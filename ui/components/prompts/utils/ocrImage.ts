/** OCR helpers for Prompt Repo (Mistral OCR via /v1/ocr). */

const DEFAULT_OCR_MODELS = [
	"mistral/mistral-ocr-latest",
	"mistral-ocr-latest",
	"mistral/mistral-ocr-2505-22",
	"mistral-ocr-2505-22",
];

export type OcrDocumentInput =
	| { type: "image_url"; image_url: string }
	| { type: "document_url"; document_url: string };

function buildAuthHeaders(apiKeyId?: string): Record<string, string> {
	const headers: Record<string, string> = {
		"Content-Type": "application/json",
	};
	if (apiKeyId && apiKeyId !== "__auto__") {
		if (apiKeyId.startsWith("sk-uf-")) {
			headers["Authorization"] = `Bearer ${apiKeyId}`;
		} else {
			headers["x-uf-api-key-id"] = apiKeyId;
		}
	}
	return headers;
}

function parseOcrMarkdown(data: unknown): string {
	if (!data || typeof data !== "object") return "";
	const root = data as Record<string, unknown>;
	const pages = root.pages;
	if (!Array.isArray(pages) || pages.length === 0) return "";
	const parts: string[] = [];
	for (const page of pages) {
		if (!page || typeof page !== "object") continue;
		const markdown = (page as Record<string, unknown>).markdown;
		if (typeof markdown === "string" && markdown.trim()) {
			parts.push(markdown.trim());
		}
	}
	return parts.join("\n\n").trim();
}

/**
 * OCR via Raksha `POST /v1/ocr` (Mistral OCR).
 * Supports image_url (photos) and document_url (PDF / scanned docs).
 */
export async function ocrDocument(
	document: OcrDocumentInput,
	opts?: { apiKeyId?: string; models?: string[]; signal?: AbortSignal; pages?: number[] },
): Promise<{ text: string; model: string } | null> {
	const url =
		document.type === "image_url"
			? document.image_url
			: document.document_url;
	if (!url?.startsWith("data:")) {
		return null;
	}

	const models = (opts?.models?.length ? opts.models : DEFAULT_OCR_MODELS).filter(Boolean);
	const auth = buildAuthHeaders(opts?.apiKeyId);
	let lastError = "";

	const bodyDocument =
		document.type === "image_url"
			? { type: "image_url", image_url: document.image_url }
			: { type: "document_url", document_url: document.document_url };

	for (const model of models) {
		try {
			const payload: Record<string, unknown> = {
				model,
				document: bodyDocument,
				include_image_base64: false,
			};
			if (opts?.pages?.length) {
				payload.pages = opts.pages;
			}

			const response = await fetch("/v1/ocr", {
				method: "POST",
				credentials: "include",
				headers: auth,
				signal: opts?.signal,
				body: JSON.stringify(payload),
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

			const data = await response.json();
			const text = parseOcrMarkdown(data);
			if (text) return { text, model };
			lastError = `${model}: empty OCR text`;
		} catch (err) {
			if (err instanceof DOMException && err.name === "AbortError") throw err;
			lastError = err instanceof Error ? err.message : String(err);
		}
	}

	if (lastError) {
		console.warn("OCR failed:", lastError);
	}
	return null;
}

/** OCR an image data URL. */
export async function ocrImageFile(
	dataUrl: string,
	opts?: { apiKeyId?: string; models?: string[]; signal?: AbortSignal },
): Promise<{ text: string; model: string } | null> {
	return ocrDocument({ type: "image_url", image_url: dataUrl }, opts);
}

/** OCR a PDF (or other document) data URL — used for scanned / image-only PDFs. */
export async function ocrPdfFile(
	dataUrl: string,
	opts?: { apiKeyId?: string; models?: string[]; signal?: AbortSignal; pages?: number[] },
): Promise<{ text: string; model: string } | null> {
	return ocrDocument({ type: "document_url", document_url: dataUrl }, opts);
}

export function ocrTextAttachment(fileName: string, ocrText: string): {
	type: "text";
	text: string;
} {
	return {
		type: "text",
		text: `Attached file: ${fileName}\n\n--- OCR extracted content ---\n${ocrText.trim()}`,
	};
}

/** True when extracted text looks too thin for a real document (likely scan/image PDF). */
export function isThinExtractedText(text: string | null | undefined): boolean {
	const t = (text || "").trim();
	if (t.length < 40) return true;
	const letters = (t.match(/[A-Za-z\u00C0-\u024F\u0900-\u097F\u0B80-\u0BFF]/g) || []).length;
	const digits = (t.match(/[0-9]/g) || []).length;
	return letters + digits < 30;
}
