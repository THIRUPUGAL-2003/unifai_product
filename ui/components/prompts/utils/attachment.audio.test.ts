import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Message } from "@/lib/message";

const { normalizeAudioToWavFile, transcribeAudioFile } = vi.hoisted(() => ({
	normalizeAudioToWavFile: vi.fn(),
	transcribeAudioFile: vi.fn(),
}));

vi.mock("sonner", () => ({ toast: { error: vi.fn(), message: vi.fn(), success: vi.fn(), warning: vi.fn() } }));
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
	transcribeAudioFile,
	voiceTranscriptAttachment: (fileName: string, transcript: string) => ({
		type: "text",
		text: `Voice transcript (${fileName}):\n\n${transcript.trim()}`,
	}),
}));

import { fileToAttachment } from "./attachment";

describe("audio file attachments", () => {
	afterEach(() => {
		vi.unstubAllGlobals();
		vi.restoreAllMocks();
	});

	beforeEach(() => {
		normalizeAudioToWavFile.mockReset().mockResolvedValue(null);
		transcribeAudioFile.mockReset().mockResolvedValue({ text: "", model: "", error: "provider unavailable" });
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

	it("passes the extracted voice transcript and question into the chat request", () => {
		const message = Message.request("What time does the meeting start?", 0, [
			{ type: "text", text: "Voice transcript (meeting.wav):\n\nThe meeting starts at 10 AM." },
		]);
		const [apiMessage] = Message.toAPIMessages([message], "gpt-4o-mini", "openai");

		expect(apiMessage?.role).toBe("user");
		expect(apiMessage?.content).toContain("What time does the meeting start?");
		expect(apiMessage?.content).toContain("The meeting starts at 10 AM.");
	});
});
