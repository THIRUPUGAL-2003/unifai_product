/** Whisper / transcription helpers for Prompt Repo voice. */

const DEFAULT_TRANSCRIPTION_MODELS = [
	"openai/whisper-1",
	"whisper-1",
	"groq/whisper-large-v3",
	"groq/distil-whisper-large-v3-en",
	"whisper-large-v3",
	"openai/gpt-4o-mini-transcribe",
	"openai/gpt-4o-transcribe",
];

function buildAuthHeaders(apiKeyId?: string): Record<string, string> {
	const headers: Record<string, string> = {};
	if (apiKeyId && apiKeyId !== "__auto__") {
		if (apiKeyId.startsWith("sk-uf-")) {
			headers["Authorization"] = `Bearer ${apiKeyId}`;
		} else {
			headers["x-uf-api-key-id"] = apiKeyId;
		}
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

/**
 * Transcribe audio via UnifAI `/v1/audio/transcriptions` (Whisper-compatible).
 * Tries a few common model ids until one succeeds.
 */
export async function transcribeAudioFile(
	file: File,
	opts?: { apiKeyId?: string; models?: string[]; signal?: AbortSignal },
): Promise<{ text: string; model: string } | null> {
	const models = (opts?.models?.length ? opts.models : DEFAULT_TRANSCRIPTION_MODELS).filter(Boolean);
	const auth = buildAuthHeaders(opts?.apiKeyId);
	let lastError = "";

	for (const model of models) {
		try {
			const form = new FormData();
			form.append("file", file, file.name || "voice.wav");
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
				continue;
			}

			const contentType = response.headers.get("content-type") || "";
			if (contentType.includes("application/json")) {
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
			if (err instanceof DOMException && err.name === "AbortError") throw err;
			lastError = err instanceof Error ? err.message : String(err);
		}
	}

	if (lastError) {
		console.warn("Whisper transcription failed:", lastError);
	}
	return null;
}

export function voiceTranscriptAttachment(fileName: string, transcript: string): {
	type: "text";
	text: string;
} {
	return {
		type: "text",
		text: `Voice transcript (${fileName}):\n\n${transcript.trim()}`,
	};
}
