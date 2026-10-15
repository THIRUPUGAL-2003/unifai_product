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

function buildAuthHeaders(apiKeyId?: string, virtualKeyId?: string): Record<string, string> {
	const headers: Record<string, string> = {
		"Content-Type": "application/json",
	};
	if (virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId.startsWith("sk-uf-")) {
		headers["Authorization"] = `Bearer ${virtualKeyId}`;
	}
	if (apiKeyId && apiKeyId !== "__auto__" && !apiKeyId.startsWith("sk-uf-")) {
		headers["x-uf-api-key-id"] = apiKeyId;
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
 * OCR via gateway `POST /v1/ocr` (Mistral OCR).
 * Supports image_url (photos) and document_url (PDF / scanned docs).
 */
export async function ocrDocument(
	document: OcrDocumentInput,
	opts?: { apiKeyId?: string; virtualKeyId?: string; models?: string[]; signal?: AbortSignal; pages?: number[] },
): Promise<{ text: string; model: string } | null> {
	const url =
		document.type === "image_url"
			? document.image_url
			: document.document_url;
	if (!url?.startsWith("data:")) {
		return null;
	}

	const models = (opts?.models?.length ? opts.models : DEFAULT_OCR_MODELS).filter(Boolean);
	const auth = buildAuthHeaders(opts?.apiKeyId, opts?.virtualKeyId);
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

/**
 * Run client-side OCR on an image data URL using in-browser Tesseract.js.
 * Zero external API dependencies, zero rate limits, works completely offline.
 */
export async function clientSideOcr(dataUrl: string): Promise<string> {
	if (!dataUrl?.startsWith("data:image/")) return "";
	try {
		const { default: Tesseract } = await import("tesseract.js");
		const res = await Tesseract.recognize(dataUrl, "eng");
		return res?.data?.text?.trim() || "";
	} catch (err) {
		console.warn("Client Tesseract OCR failed:", err);
		return "";
	}
}

/** OCR an image data URL (cloud first, in-browser Tesseract fallback). */
export async function ocrImageFile(
	dataUrl: string,
	opts?: { apiKeyId?: string; virtualKeyId?: string; models?: string[]; signal?: AbortSignal },
): Promise<{ text: string; model: string } | null> {
	// 1. Try cloud OCR via gateway /v1/ocr
	const cloud = await ocrDocument({ type: "image_url", image_url: dataUrl }, opts);
	if (cloud?.text?.trim()) {
		return cloud;
	}

	// 2. Client-side Tesseract.js fallback
	const clientText = await clientSideOcr(dataUrl);
	if (clientText) {
		return { text: clientText, model: "tesseract-ocr" };
	}

	return null;
}

/** OCR a PDF data URL — used for scanned / image-only PDFs (cloud first, in-browser canvas render + Tesseract fallback). */
export async function ocrPdfFile(
	dataUrl: string,
	opts?: { apiKeyId?: string; virtualKeyId?: string; models?: string[]; signal?: AbortSignal; pages?: number[] },
): Promise<{ text: string; model: string } | null> {
	// 1. Try cloud OCR via gateway /v1/ocr
	const cloud = await ocrDocument({ type: "document_url", document_url: dataUrl }, opts);
	if (cloud?.text?.trim()) {
		return cloud;
	}

	// 2. Client-side PDF OCR via pdfjs-dist + Tesseract.js
	try {
		const base64Match = dataUrl.match(/^data:application\/pdf;base64,(.+)$/);
		if (base64Match) {
			const raw = atob(base64Match[1]);
			const bytes = new Uint8Array(raw.length);
			for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i);

			const { default: pdfjsWorker } = await import("pdfjs-dist/build/pdf.worker.min.mjs?url");
			const pdfjs = await import("pdfjs-dist");
			pdfjs.GlobalWorkerOptions.workerSrc = pdfjsWorker;

			const doc = await pdfjs.getDocument({ data: bytes }).promise;
			const pagesToScan = Math.min(doc.numPages, 5);
			const { default: Tesseract } = await import("tesseract.js");
			const pageTexts: string[] = [];

			for (let p = 1; p <= pagesToScan; p++) {
				const page = await doc.getPage(p);
				const viewport = page.getViewport({ scale: 1.5 });
				const canvas = document.createElement("canvas");
				canvas.width = viewport.width;
				canvas.height = viewport.height;
				const ctx = canvas.getContext("2d");
				if (ctx) {
					// eslint-disable-next-line @typescript-eslint/no-explicit-any
					await (page.render as any)({ canvasContext: ctx, viewport }).promise;
					const res = await Tesseract.recognize(canvas, "eng");
					const txt = res?.data?.text?.trim();
					if (txt) {
						pageTexts.push(pagesToScan > 1 ? `[Page ${p}]\n${txt}` : txt);
					}
				}
			}

			if (pageTexts.length > 0) {
				return { text: pageTexts.join("\n\n").trim(), model: "tesseract-ocr" };
			}
		}
	} catch (pdfOcrErr) {
		console.warn("Client PDF OCR fallback failed:", pdfOcrErr);
	}

	return null;
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
