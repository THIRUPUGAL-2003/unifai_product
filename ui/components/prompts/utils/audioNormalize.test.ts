import { afterEach, describe, expect, it, vi } from "vitest";
import { audioFormatFromMimeOrName, normalizeAudioToWavFile } from "./audioNormalize";

afterEach(() => {
	vi.unstubAllGlobals();
});

describe("audio normalization", () => {
	it("recognizes model-compatible mp3 and wav formats", () => {
		expect(audioFormatFromMimeOrName("audio/mpeg", "recording.mp3")).toBe("mp3");
		expect(audioFormatFromMimeOrName("audio/wav", "recording.wav")).toBe("wav");
		expect(audioFormatFromMimeOrName("audio/webm", "recording.webm")).toBe("webm");
	});

	it("mixes both channels and writes a mono 16 kHz wav", async () => {
		const channels = [new Float32Array([1, 1, -1, -1]), new Float32Array([1, 1, 1, 1])];
		const decodedAudio = {
			numberOfChannels: channels.length,
			sampleRate: 32_000,
			length: 4,
			getChannelData: (channel: number) => channels[channel],
		} as unknown as AudioBuffer;

		class MockAudioContext {
			state = "running";

			async decodeAudioData() {
				return decodedAudio;
			}

			async close() {}
		}

		vi.stubGlobal("window", { AudioContext: MockAudioContext });
		const original = new File(["webm"], "recording.webm", { type: "audio/webm" });

		const normalized = await normalizeAudioToWavFile(original);

		expect(normalized).not.toBeNull();
		expect(normalized?.name).toBe("recording.wav");
		expect(normalized?.type).toBe("audio/wav");
		const wav = new DataView(await normalized!.arrayBuffer());
		expect(wav.getUint32(24, true)).toBe(16_000);
		expect(wav.getUint16(22, true)).toBe(1);
		expect(wav.getInt16(44, true)).toBe(32_767);
		expect(wav.getInt16(46, true)).toBe(0);
	});
});
