import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Message } from "@/lib/message";
import { MAX_TEXT_EXTRACTION_BYTES } from "./extractFileText";

const { normalizeAudioToWavFile, transcribeAudioFile, ocrImageFile, toastError, toastLoading, toastMessage, toastSuccess, toastWarning } =
	vi.hoisted(() => ({
		normalizeAudioToWavFile: vi.fn(),
		transcribeAudioFile: vi.fn(),
		ocrImageFile: vi.fn(),
		toastError: vi.fn(),
		toastLoading: vi.fn(),
		toastMessage: vi.fn(),
		toastSuccess: vi.fn(),
		toastWarning: vi.fn(),
	}));

vi.mock("sonner", () => ({
	toast: { error: toastError, loading: toastLoading, message: toastMessage, success: toastSuccess, warning: toastWarning },
}));
vi.mock("./audioNormalize", () => ({
	audioFormatFromMimeOrName: (mimeType: string, fileName: string) => {
		const lower = `${mimeType} ${fileName}`.toLowerCase();
		if (lower.includes("mpeg") || lower.includes("mp3")) return "mp3";
		if (lower.includes("wav")) return "wav";
		return fileName.split(".").pop()?.toLowerCase() || "wav";
	},
	normalizeAudioToWavFile,
}));
vi.mock("./transcribeAudio", () => ({
	MAX_AUDIO_TRANSCRIPTION_BYTES: 25 * 1024 * 1024,
	transcribeAudioFile,
	voiceTranscriptAttachment: (fileName: string, transcript: string) => ({
		type: "text",
		text: `Voice transcript (${fileName}):\n\n${transcript.trim()}`,
	}),
}));
vi.mock("./ocrImage", () => ({
	isThinExtractedText: () => false,
	ocrImageFile,
	ocrPdfFile: vi.fn(),
	ocrTextAttachment: (fileName: string, text: string) => ({
		type: "text",
		text: `Attached file: ${fileName}\n\n--- OCR extracted content ---\n${text.trim()}`,
	}),
}));

import { fileToAttachment, filesToAttachments, normalizeImageForModel } from "./attachment";

describe("prompt attachments", () => {
	afterEach(() => {
		vi.unstubAllGlobals();
		vi.restoreAllMocks();
	});

	beforeEach(() => {
		toastError.mockClear();
		toastLoading.mockClear();
		toastMessage.mockClear();
		toastSuccess.mockClear();
		toastWarning.mockClear();
		normalizeAudioToWavFile.mockReset().mockResolvedValue(null);
		transcribeAudioFile.mockReset().mockResolvedValue({ text: "", model: "", error: "provider unavailable" });
		ocrImageFile.mockReset().mockResolvedValue(null);
	});

	it("never labels an unconverted webm recording as wav", async () => {
		const webmAudio = new File([new Uint8Array([0x1a, 0x45, 0xdf, 0xa3, 0x00])], "speech.webm", { type: "audio/webm" });
		const attachment = await fileToAttachment(webmAudio, {
			silent: true,
		});

		expect(attachment).toMatchObject({
			type: "text",
			text: expect.stringContaining("[Transcription unavailable:"),
		});
		expect(attachment?.type === "input_audio" ? attachment.input_audio?.format : "").not.toBe("wav");
	});

	it("reuses a real normalized wav for raw-audio fallback", async () => {
		const normalized = new File(["wav-audio"], "speech.wav", { type: "audio/wav" });
		normalizeAudioToWavFile.mockResolvedValue(normalized);
		vi.stubGlobal(
			"FileReader",
			class {
				result: string | null = null;
				onload: null | (() => void) = null;
				onerror: null | (() => void) = null;
				readAsDataURL(file: File) {
					void file.arrayBuffer().then((buffer) => {
						const bytes = new Uint8Array(buffer);
						let binary = "";
						for (const byte of bytes) binary += String.fromCharCode(byte);
						this.result = `data:${file.type};base64,${btoa(binary)}`;
						this.onload?.();
					});
				}
			},
		);

		const webmAudio = new File([new Uint8Array([0x1a, 0x45, 0xdf, 0xa3, 0x00])], "speech.webm", { type: "audio/webm" });
		const attachment = await fileToAttachment(webmAudio, {
			silent: true,
		});

		expect(attachment).toMatchObject({ type: "input_audio", input_audio: { format: "wav" } });
	});

	it("does not call transcription again when the caller already attempted it", async () => {
		const webmAudio = new File([new Uint8Array([0x1a, 0x45, 0xdf, 0xa3, 0x00])], "speech.webm", { type: "audio/webm" });
		const attachment = await fileToAttachment(webmAudio, {
			silent: true,
			skipAudioTranscription: true,
		});

		expect(transcribeAudioFile).not.toHaveBeenCalled();
		expect(attachment).toMatchObject({ type: "text", text: expect.stringContaining("Transcription unavailable") });
	});

	it("transcribes supported webm audio without converting the original first", async () => {
		const webmAudio = new File([new Uint8Array([0x1a, 0x45, 0xdf, 0xa3, 0x00])], "speech.webm", { type: "audio/webm" });
		transcribeAudioFile.mockResolvedValue({ text: "hello", model: "test-model" });

		const attachment = await fileToAttachment(webmAudio, { silent: true });

		expect(normalizeAudioToWavFile).not.toHaveBeenCalled();
		expect(transcribeAudioFile).toHaveBeenCalledWith(webmAudio, expect.any(Object));
		expect(attachment).toMatchObject({ type: "text", text: expect.stringContaining("hello") });
	});

	it("does not send a mislabeled text file as raw mp3 audio", async () => {
		const attachment = await fileToAttachment(new File(["this is plain text"], "broken.mp3", { type: "audio/mpeg" }), {
			silent: true,
		});

		expect(transcribeAudioFile).not.toHaveBeenCalled();
		expect(attachment).toMatchObject({ type: "text", text: expect.stringContaining("Transcription unavailable") });
	});

	it("passes the extracted voice transcript and question into the chat request", () => {
		const message = Message.request("What time does the meeting start?", 0, [
			{ type: "text", text: "Voice transcript (meeting.wav):\n\nThe meeting starts at 10 AM." },
		]);
		const [apiMessage] = Message.toAPIMessages([message], "gpt-4o-mini", "openai");

		expect(apiMessage?.role).toBe("user");
		expect(apiMessage?.content).toContain("What time does the meeting start?");
		expect(apiMessage?.content).toContain("The meeting starts at 10 AM.");
	});

	it("does not forward a corrupt image as if it were a valid picture", async () => {
		class BrokenImage {
			naturalWidth = 0;
			naturalHeight = 0;
			onload: (() => void) | null = null;
			onerror: (() => void) | null = null;

			set src(_value: string) {
				queueMicrotask(() => this.onerror?.());
			}
		}
		vi.stubGlobal("Image", BrokenImage);
		vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:broken-image");
		vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});

		const dataUrl = await normalizeImageForModel(new File(["not an image"], "broken.png", { type: "image/png" }), "image/png");

		expect(dataUrl).toBeNull();
	});

	it("extracts a valid TIFF through cloud OCR when the browser cannot decode it", async () => {
		class BrokenImage {
			naturalWidth = 0;
			naturalHeight = 0;
			onload: (() => void) | null = null;
			onerror: (() => void) | null = null;

			set src(_value: string) {
				queueMicrotask(() => this.onerror?.());
			}
		}
		vi.stubGlobal("Image", BrokenImage);
		vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:valid-tiff");
		vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
		vi.stubGlobal(
			"FileReader",
			class {
				result: string | null = null;
				onload: null | (() => void) = null;
				onerror: null | (() => void) = null;
				readAsDataURL(file: File) {
					void file.arrayBuffer().then((buffer) => {
						const bytes = new Uint8Array(buffer);
						let binary = "";
						for (const byte of bytes) binary += String.fromCharCode(byte);
						this.result = `data:${file.type};base64,${btoa(binary)}`;
						this.onload?.();
					});
				}
			},
		);
		ocrImageFile.mockResolvedValue({ text: "Invoice total: 42", model: "mistral-ocr-latest" });

		const tiff = new File([new Uint8Array([0x49, 0x49, 0x2a, 0x00, 0x08, 0x00, 0x00, 0x00])], "invoice.tiff", {
			type: "image/tiff",
		});
		const attachment = await fileToAttachment(tiff, { silent: true }, "image/tiff");

		expect(ocrImageFile).toHaveBeenCalledWith(expect.stringMatching(/^data:image\/tiff;base64,/), expect.any(Object));
		expect(attachment).toMatchObject({ type: "text", text: expect.stringContaining("Invoice total: 42") });
	});

	it("does not send an invalid image payload to OCR when decoding fails", async () => {
		class BrokenImage {
			naturalWidth = 0;
			naturalHeight = 0;
			onload: (() => void) | null = null;
			onerror: (() => void) | null = null;

			set src(_value: string) {
				queueMicrotask(() => this.onerror?.());
			}
		}
		vi.stubGlobal("Image", BrokenImage);
		vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:invalid-image");
		vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});

		const invalid = new File(["not an image"], "broken.png", { type: "image/png" });
		await fileToAttachment(invalid, { silent: true }, "image/png");

		expect(ocrImageFile).not.toHaveBeenCalled();
	});

	it("does not turn an oversized image metadata stub into successful OCR", async () => {
		const createObjectURL = vi.spyOn(URL, "createObjectURL");
		const file = {
			name: "large-scan.png",
			size: MAX_TEXT_EXTRACTION_BYTES + 1,
		} as File;

		const attachment = await fileToAttachment(file, {}, "image/png");

		expect(attachment?.type).toBe("text");
		expect(attachment?.text).toContain("Extraction skipped:");
		expect(attachment?.text).not.toContain("OCR extracted content");
		expect(createObjectURL).not.toHaveBeenCalled();
		expect(toastError).toHaveBeenCalledWith(
			'Could not read image "large-scan.png"',
			expect.objectContaining({ description: expect.stringContaining("safe in-browser extraction limit") }),
		);
	});

	it("reports unreadable files in a bulk import instead of claiming every file succeeded", async () => {
		const unreadable = new File([new Uint8Array(64)], "opaque.bin");
		const readable = [1, 2, 3, 4].map((index) => new File([`readable file ${index}`], `notes-${index}.txt`, { type: "text/plain" }));

		await filesToAttachments([unreadable, ...readable]);

		expect(toastWarning).toHaveBeenCalledWith(
			expect.stringContaining("1 file(s) failed"),
			expect.objectContaining({ description: expect.stringContaining("opaque.bin") }),
		);
		expect(toastSuccess).not.toHaveBeenCalledWith(expect.stringContaining("Successfully imported all"), expect.anything());
	});
});
