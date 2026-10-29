import { afterEach, describe, expect, it, vi } from "vitest";
import { MAX_AUDIO_TRANSCRIPTION_BYTES, transcribeAudioFile } from "./transcribeAudio";

afterEach(() => {
	vi.restoreAllMocks();
});

describe("transcribeAudioFile", () => {
	it("does not send an empty audio file", async () => {
		const fetchMock = vi.spyOn(globalThis, "fetch");
		const result = await transcribeAudioFile(new File([], "empty.wav", { type: "audio/wav" }));

		expect(result).toMatchObject({ text: "", error: "The audio file is empty." });
		expect(fetchMock).not.toHaveBeenCalled();
	});

	it("rejects files over the gateway limit before sending a request", async () => {
		const fetchMock = vi.spyOn(globalThis, "fetch");
		const oversizedFile = { size: MAX_AUDIO_TRANSCRIPTION_BYTES + 1 } as File;

		const result = await transcribeAudioFile(oversizedFile);

		expect(result).toMatchObject({ text: "", error: "Audio is larger than the 25 MB transcription limit." });
		expect(fetchMock).not.toHaveBeenCalled();
	});

	it("tries the next model after a model-specific error and returns its transcript", async () => {
		const fetchMock = vi
			.spyOn(globalThis, "fetch")
			.mockResolvedValueOnce(
				new Response(JSON.stringify({ error: { message: "unknown model" } }), {
					status: 400,
					headers: { "content-type": "application/json" },
				}),
			)
			.mockResolvedValueOnce(
				new Response(JSON.stringify({ text: "  hello from audio  " }), {
					status: 200,
					headers: { "content-type": "application/json" },
				}),
			);
		const file = new File(["audio-bytes"], "speech.wav", { type: "audio/wav" });

		const result = await transcribeAudioFile(file, { models: ["bad-model", "working-model"] });

		expect(fetchMock).toHaveBeenCalledTimes(2);
		expect(result).toEqual({ text: "hello from audio", model: "working-model" });
	});

	it("stops retrying after a request-size rejection", async () => {
		const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
			new Response(JSON.stringify({ error: { message: "audio upload too large" } }), {
				status: 413,
				headers: { "content-type": "application/json" },
			}),
		);

		const result = await transcribeAudioFile(new File(["audio"], "speech.wav"), {
			models: ["first-model", "second-model", "third-model"],
		});

		expect(fetchMock).toHaveBeenCalledTimes(1);
		expect(result).toMatchObject({ text: "", error: expect.stringContaining("audio upload too large") });
	});

	it("stops retrying when the gateway reports its 25 MB upload limit", async () => {
		const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
			new Response(JSON.stringify({ error: { message: "file size exceeds maximum limit of 25 MB" } }), {
				status: 400,
				headers: { "content-type": "application/json" },
			}),
		);

		await transcribeAudioFile(new File(["audio"], "speech.wav"), {
			models: ["first-model", "second-model", "third-model"],
		});

		expect(fetchMock).toHaveBeenCalledTimes(1);
	});

	it("does not repeat requests when credentials are rejected", async () => {
		const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
			new Response(JSON.stringify({ error: { message: "Invalid API credentials" } }), {
				status: 401,
				headers: { "content-type": "application/json" },
			}),
		);

		const result = await transcribeAudioFile(new File(["audio"], "speech.wav"), {
			models: ["first-model", "second-model"],
		});

		expect(fetchMock).toHaveBeenCalledTimes(1);
		expect(result).toMatchObject({ text: "", error: expect.stringContaining("Invalid API credentials") });
	});

	it("preserves cancellation instead of converting it into a transcription error", async () => {
		vi.spyOn(globalThis, "fetch").mockRejectedValue(Object.assign(new Error("Aborted"), { name: "AbortError" }));

		await expect(transcribeAudioFile(new File(["audio"], "speech.wav"), { models: ["first-model"] })).rejects.toMatchObject({
			name: "AbortError",
		});
	});
});
