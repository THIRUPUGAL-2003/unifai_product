/** Whisper / transcription helpers for Prompt Repo voice. */

const DEFAULT_TRANSCRIPTION_MODELS = [
	"openai/whisper-1",
	"whisper-1",
	"groq/whisper-large-v3",
	"whisper-large-v3",
	"mistral/voxtral-mini-latest",
	"voxtral-mini-latest",
	"voxtral-mini-2602",
	"openai/gpt-4o-mini-transcribe",
	"openai/gpt-4o-transcribe",
];

/** Keep browser uploads aligned with the gateway transcription endpoint's request limit. */
export const MAX_AUDIO_TRANSCRIPTION_BYTES = 25 * 1024 * 1024;

function buildAuthHeaders(apiKeyId?: string, virtualKeyId?: string): Record<string, string> {
	const headers: Record<string, string> = {};
	if (virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId.startsWith("sk-uf-")) {
		headers["Authorization"] = `Bearer ${virtualKeyId}`;
	}
	if (apiKeyId && apiKeyId !== "__auto__" && !apiKeyId.startsWith("sk-uf-")) {
		headers["x-uf-api-key-id"] = apiKeyId;
	}
	return headers;
}

function parseTranscriptionText(data: unknown): string {
	if (!data || typeof data !== "object") return "";
	const root = data as Record<string, unknown>;
	if (typeof root.text === "string" && root.text.trim()) return root.text.trim();
	if (root.TranscriptionResponse && typeof root.TranscriptionResponse === "object") {
		const nested = root.TranscriptionResponse as Record<string, unknown>;
		if (typeof nested.text === "string" && nested.text.trim()) return nested.text.trim();
	}
	return "";
}

function isAbortError(error: unknown): boolean {
	return Boolean(error && typeof error === "object" && "name" in error && error.name === "AbortError");
}

function shouldTryNextModel(status: number, message: string): boolean {
	if (status === 408 || status === 429 || status >= 500) return true;
	return (status === 400 || status === 404) && /model/i.test(message);
}

/**
 * Transcribe audio via gateway `/v1/audio/transcriptions` (Whisper-compatible).
 * Tries a few common model ids until one succeeds.
 */
export type TranscriptionResult = { text: string; model: string } | { text: ""; model: ""; error: string };

export async function transcribeAudioFile(
	file: File,
	opts?: { apiKeyId?: string; virtualKeyId?: string; models?: string[]; signal?: AbortSignal },
): Promise<TranscriptionResult | null> {
	if (file.size === 0) return { text: "", model: "", error: "The audio file is empty." };
	if (file.size > MAX_AUDIO_TRANSCRIPTION_BYTES) {
		return { text: "", model: "", error: "Audio is larger than the 25 MB transcription limit." };
	}
	const models = (opts?.models?.length ? opts.models : DEFAULT_TRANSCRIPTION_MODELS).filter(Boolean);
	const auth = buildAuthHeaders(opts?.apiKeyId, opts?.virtualKeyId);
	let lastError = "";

	for (const model of models) {
		try {
			const form = new FormData();
			form.append("file", file, file.name);
			form.append("model", model);
			form.append("response_format", "json");

			const response = await fetch("/v1/audio/transcriptions", {
				method: "POST",
				credentials: "include",
				headers: auth,
				body: form,
				signal: opts?.signal,
			});

			if (!response.ok) {
				let msg = `HTTP ${response.status}`;
				try {
					const data = await response.json();
					msg =
						(data?.error?.message as string) ||
						(data?.error?.error as string) ||
						(typeof data?.error === "string" ? data.error : "") ||
						msg;
				} catch {
					/* ignore */
				}
				lastError = `${model}: ${msg}`;
				// These errors apply to the upload/request itself, so trying every model only repeats the same failure.
				if (
					response.status === 413 ||
					/unsupported file format|invalid file type|file size exceeds maximum limit|file too large/i.test(msg)
				) {
					break;
				}
				if (!shouldTryNextModel(response.status, msg)) break;
				continue;
			}

			const contentType = response.headers.get("content-type") || "";
			if (/json/i.test(contentType)) {
				const data = await response.json();
				const text = parseTranscriptionText(data);
				if (text) return { text, model };
				lastError = `${model}: empty transcript`;
				continue;
			}

			const text = (await response.text()).trim();
			if (text) return { text, model };
			lastError = `${model}: empty transcript`;
		} catch (err) {
			if (isAbortError(err)) throw err;
			lastError = err instanceof Error ? err.message : String(err);
			if (err instanceof TypeError) break;
		}
	}

	if (lastError) {
		console.warn("Whisper transcription failed:", lastError);
		return { text: "", model: "", error: lastError };
	}
	return null;
}

export function voiceTranscriptAttachment(
	fileName: string,
	transcript: string,
): {
	type: "text";
	text: string;
} {
	return {
		type: "text",
		text: `Voice transcript (${fileName}):\n\n${transcript.trim()}`,
	};
}
