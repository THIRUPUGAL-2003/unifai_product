import { afterEach, describe, expect, it, vi } from "vitest";
import { transcribeAudioFile } from "./transcribeAudio";

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

	it("preserves cancellation instead of converting it into a transcription error", async () => {
		vi.spyOn(globalThis, "fetch").mockRejectedValue(new DOMException("Aborted", "AbortError"));

		await expect(
			transcribeAudioFile(new File(["audio"], "speech.wav"), { models: ["first-model"] }),
		).rejects.toMatchObject({ name: "AbortError" });
	});
});
