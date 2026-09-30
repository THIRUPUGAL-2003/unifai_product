/** Convert browser-recorded audio (often webm/ogg) to PCM WAV for chat input_audio. */

function writeString(view: DataView, offset: number, value: string) {
	for (let i = 0; i < value.length; i++) {
		view.setUint8(offset + i, value.charCodeAt(i));
	}
}

function encodeWav(samples: Float32Array, sampleRate: number): Blob {
	const numChannels = 1;
	const bitsPerSample = 16;
	const blockAlign = (numChannels * bitsPerSample) / 8;
	const byteRate = sampleRate * blockAlign;
	const dataSize = samples.length * blockAlign;
	const buffer = new ArrayBuffer(44 + dataSize);
	const view = new DataView(buffer);

	writeString(view, 0, "RIFF");
	view.setUint32(4, 36 + dataSize, true);
	writeString(view, 8, "WAVE");
	writeString(view, 12, "fmt ");
	view.setUint32(16, 16, true);
	view.setUint16(20, 1, true);
	view.setUint16(22, numChannels, true);
	view.setUint32(24, sampleRate, true);
	view.setUint32(28, byteRate, true);
	view.setUint16(32, blockAlign, true);
	view.setUint16(34, bitsPerSample, true);
	writeString(view, 36, "data");
	view.setUint32(40, dataSize, true);

	let offset = 44;
	for (let i = 0; i < samples.length; i++) {
		const s = Math.max(-1, Math.min(1, samples[i]));
		view.setInt16(offset, s < 0 ? s * 0x8000 : s * 0x7fff, true);
		offset += 2;
	}

	return new Blob([buffer], { type: "audio/wav" });
}

function downsampleToMono(buffer: AudioBuffer, targetRate = 16000): Float32Array {
	const channel = buffer.numberOfChannels > 0 ? buffer.getChannelData(0) : new Float32Array(0);
	if (buffer.sampleRate === targetRate) {
		return channel.slice();
	}
	const ratio = buffer.sampleRate / targetRate;
	const newLength = Math.max(1, Math.floor(channel.length / ratio));
	const result = new Float32Array(newLength);
	for (let i = 0; i < newLength; i++) {
		result[i] = channel[Math.min(channel.length - 1, Math.floor(i * ratio))] || 0;
	}
	return result;
}

/**
 * Decode any playable audio blob and re-encode as 16 kHz mono WAV.
 * Returns null if Web Audio cannot decode the format.
 */
export async function normalizeAudioToWavFile(file: File): Promise<File | null> {
	const mime = (file.type || "").toLowerCase();
	const name = file.name.toLowerCase();
	if (mime.includes("wav") || name.endsWith(".wav")) {
		return file;
	}

	if (typeof window === "undefined") {
		return null;
	}

	const AudioCtx = window.AudioContext || (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
	if (!AudioCtx) {
		return null;
	}

	try {
		const ctx = new AudioCtx();
		try {
			const arrayBuffer = await file.arrayBuffer();
			const audioBuffer = await ctx.decodeAudioData(arrayBuffer.slice(0));
			const mono = downsampleToMono(audioBuffer, 16000);
			const wavBlob = encodeWav(mono, 16000);
			const base = file.name.replace(/\.[^.]+$/, "") || "voice";
			return new File([wavBlob], `${base}.wav`, { type: "audio/wav" });
		} finally {
			void ctx.close();
		}
	} catch (error) {
		console.warn("Audio normalize failed:", file.name, error);
		return null;
	}
}

export function audioFormatFromMimeOrName(mimeType: string, fileName: string): string {
	const mime = (mimeType || "").toLowerCase();
	const name = (fileName || "").toLowerCase();
	if (mime.includes("wav") || name.endsWith(".wav")) return "wav";
	if (mime.includes("mpeg") || mime.includes("mp3") || name.endsWith(".mp3")) return "mp3";
	if (mime.includes("mp4") || mime.includes("m4a") || name.endsWith(".m4a") || name.endsWith(".mp4")) return "mp4";
	if (mime.includes("ogg") || name.endsWith(".ogg")) return "ogg";
	if (mime.includes("webm") || name.endsWith(".webm")) return "webm";
	const ext = fileName.split(".").pop()?.toLowerCase();
	return ext || "wav";
}
