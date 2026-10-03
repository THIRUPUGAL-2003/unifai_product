import { type MessageContent } from "@/lib/message";
import { toast } from "sonner";
import { audioFormatFromMimeOrName, normalizeAudioToWavFile } from "./audioNormalize";
import { extractPromptFileText } from "./extractFileText";
import { extractZipInnerFiles } from "./extractZip";
import { isThinExtractedText, ocrImageFile, ocrPdfFile, ocrTextAttachment } from "./ocrImage";
import { transcribeAudioFile, voiceTranscriptAttachment } from "./transcribeAudio";

/** Accepted file types for prompt repository attachments */
export const PROMPT_FILE_ACCEPT =
	"image/*,.png,.jpg,.jpeg,.webp,.gif,.bmp,.svg,.tiff,.tif,.avif,.heic,.heif,.ico,audio/*,.pdf,.txt,.csv,.json,.xml,.md,.html,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.zip,.mp3,.wav,.m4a,.webm,.ogg";

export const PROMPT_FILE_ACCEPT_LABEL =
	"Images (PNG, JPG, WEBP, GIF, SVG, etc.), PDF, Zip, Excel, Word, audio, voice, and more";

export const MAX_PROMPT_ATTACHMENT_BYTES = 20 * 1024 * 1024; // 20 MB

const EXTENSION_MIME: Record<string, string> = {
	pdf: "application/pdf",
	txt: "text/plain",
	csv: "text/csv",
	json: "application/json",
	xml: "application/xml",
	md: "text/markdown",
	html: "text/html",
	doc: "application/msword",
	docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	xls: "application/vnd.ms-excel",
	xlsx: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	ppt: "application/vnd.ms-powerpoint",
	pptx: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	zip: "application/zip",
	mp3: "audio/mpeg",
	wav: "audio/wav",
	m4a: "audio/mp4",
	webm: "audio/webm",
	ogg: "audio/ogg",
	png: "image/png",
	jpg: "image/jpeg",
	jpeg: "image/jpeg",
	webp: "image/webp",
	gif: "image/gif",
	bmp: "image/bmp",
	svg: "image/svg+xml",
	tiff: "image/tiff",
	tif: "image/tiff",
	avif: "image/avif",
	heic: "image/heic",
	heif: "image/heif",
	ico: "image/x-icon",
};

export function resolveFileMimeType(file: File): string {
	if (file.type && file.type !== "application/octet-stream") {
		return file.type;
	}
	const ext = file.name.split(".").pop()?.toLowerCase() || "";
	return EXTENSION_MIME[ext] || "application/octet-stream";
}

export function isImageFile(file: File, mimeType?: string): boolean {
	const effectiveMime = mimeType || resolveFileMimeType(file);
	if (effectiveMime.startsWith("image/")) return true;
	const ext = file.name.split(".").pop()?.toLowerCase() || "";
	return [
		"png",
		"jpg",
		"jpeg",
		"webp",
		"gif",
		"bmp",
		"svg",
		"tiff",
		"tif",
		"avif",
		"heic",
		"heif",
		"ico",
	].includes(ext);
}

export function validatePromptAttachmentFile(file: File): string | null {
	if (file.size > MAX_PROMPT_ATTACHMENT_BYTES) {
		return `"${file.name}" is too large (max ${Math.round(MAX_PROMPT_ATTACHMENT_BYTES / (1024 * 1024))} MB)`;
	}
	return null;
}

export function fileToBase64(file: File): Promise<string> {
	return new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = () => resolve(reader.result as string);
		reader.onerror = reject;
		reader.readAsDataURL(file);
	});
}

/** Formats every vision provider (OpenAI, Anthropic, Gemini, Bedrock) accepts as inline images. */
const MODEL_IMAGE_MIMES = new Set(["image/png", "image/jpeg", "image/webp", "image/gif"]);
/** Anthropic rejects inline images above 5 MB; stay under it with base64 overhead in mind. */
const MODEL_IMAGE_MAX_BYTES = 3.75 * 1024 * 1024;
const MODEL_IMAGE_MAX_DIMENSION = 2048;

function loadImageElement(file: File): Promise<HTMLImageElement> {
	return new Promise((resolve, reject) => {
		const url = URL.createObjectURL(file);
		const img = new Image();
		img.onload = () => {
			URL.revokeObjectURL(url);
			resolve(img);
		};
		img.onerror = () => {
			URL.revokeObjectURL(url);
			reject(new Error(`Could not decode image "${file.name}"`));
		};
		img.src = url;
	});
}

/**
 * Returns a data URL a vision model will accept: supported format, at most
 * MODEL_IMAGE_MAX_DIMENSION px per side and under MODEL_IMAGE_MAX_BYTES.
 * Returns null when the browser cannot decode the image (e.g. HEIC outside Safari).
 */
export async function normalizeImageForModel(file: File, mimeType: string): Promise<string | null> {
	const supported = MODEL_IMAGE_MIMES.has(mimeType);
	if (supported && file.size <= MODEL_IMAGE_MAX_BYTES) {
		if (mimeType === "image/gif") return fileToBase64(file);
		try {
			const img = await loadImageElement(file);
			if (Math.max(img.naturalWidth, img.naturalHeight) <= MODEL_IMAGE_MAX_DIMENSION) {
				return fileToBase64(file);
			}
		} catch {
			return fileToBase64(file);
		}
	}

	let img: HTMLImageElement;
	try {
		img = await loadImageElement(file);
	} catch {
		return null;
	}
	const width = img.naturalWidth || 1024;
	const height = img.naturalHeight || 1024;
	let scale = Math.min(1, MODEL_IMAGE_MAX_DIMENSION / Math.max(width, height));
	const keepAlpha = mimeType === "image/png" || mimeType === "image/svg+xml" || mimeType === "image/x-icon" || mimeType === "image/webp";

	for (let attempt = 0; attempt < 5; attempt++) {
		const canvas = document.createElement("canvas");
		canvas.width = Math.max(1, Math.round(width * scale));
		canvas.height = Math.max(1, Math.round(height * scale));
		const g = canvas.getContext("2d");
		if (!g) return null;
		const asPng = keepAlpha && attempt === 0;
		if (!asPng) {
			g.fillStyle = "#ffffff";
			g.fillRect(0, 0, canvas.width, canvas.height);
		}
		g.drawImage(img, 0, 0, canvas.width, canvas.height);
		const dataUrl = asPng ? canvas.toDataURL("image/png") : canvas.toDataURL("image/jpeg", attempt <= 1 ? 0.9 : 0.8);
		const bytes = Math.ceil(((dataUrl.length - dataUrl.indexOf(",") - 1) * 3) / 4);
		if (bytes <= MODEL_IMAGE_MAX_BYTES) {
			if (!supported || scale < 1) {
				toast.message(`"${file.name}" converted for model compatibility`, {
					description: `${canvas.width}×${canvas.height} ${dataUrl.startsWith("data:image/png") ? "PNG" : "JPEG"}`,
				});
			}
			return dataUrl;
		}
		scale *= 0.75;
	}
	return null;
}

function isZipFile(file: File, mimeType: string): boolean {
	const lower = file.name.toLowerCase();
	return lower.endsWith(".zip") || mimeType === "application/zip" || mimeType === "application/x-zip-compressed";
}

function isPdfFile(file: File, mimeType: string): boolean {
	const lower = file.name.toLowerCase();
	return lower.endsWith(".pdf") || mimeType.includes("pdf");
}

/**
 * Convert one or more files into message attachments.
 * Zip archives are expanded and each supported inner file is imported.
 */
export async function filesToAttachments(files: FileList | File[]): Promise<MessageContent[]> {
	const attachments: MessageContent[] = [];
	for (const file of Array.from(files)) {
		const error = validatePromptAttachmentFile(file);
		if (error) {
			toast.error(error);
			continue;
		}
		try {
			const mimeType = resolveFileMimeType(file);
			if (isZipFile(file, mimeType)) {
				const fromZip = await attachmentsFromZip(file);
				attachments.push(...fromZip);
				continue;
			}
			const attachment = await fileToAttachment(file);
			if (attachment) {
				attachments.push(attachment);
			}
		} catch (err) {
			const message = err instanceof Error ? err.message : `Failed to import "${file.name}"`;
			toast.error(message);
		}
	}
	return attachments;
}

async function attachmentsFromZip(zipFile: File): Promise<MessageContent[]> {
	toast.message(`Unpacking ${zipFile.name}…`);
	const { files, skipped, errors } = await extractZipInnerFiles(zipFile);
	if (files.length === 0) {
		toast.error(`No supported files found in "${zipFile.name}"`, {
			description: "Include PDF, Office, text, image, or audio files inside the zip.",
		});
		return [];
	}
	toast.message(`Importing ${files.length} file(s) from ${zipFile.name}…`);
	const out: MessageContent[] = [];
	for (const inner of files) {
		const error = validatePromptAttachmentFile(inner);
		if (error) {
			toast.error(error);
			continue;
		}
		try {
			const attachment = await fileToAttachment(inner);
			if (attachment) out.push(attachment);
		} catch (err) {
			toast.error(err instanceof Error ? err.message : `Failed to import "${inner.name}"`);
		}
	}
	if (out.length > 0) {
		const extra =
			skipped > 0 || errors.length > 0
				? ` (${skipped} skipped${errors.length ? `; ${errors[0]}` : ""})`
				: "";
		toast.success(`Imported ${out.length} file(s) from ${zipFile.name}${extra}`);
	}
	return out;
}

function textAttachmentFromExtract(fileName: string, extracted: string): MessageContent {
	return {
		type: "text",
		text: `Attached file: ${fileName}\n\n--- extracted content ---\n${extracted.trim()}`,
	};
}

async function tryPdfOcr(file: File): Promise<MessageContent | null> {
	toast.message(`Running OCR on scanned PDF ${file.name}…`);
	const dataUrl = await fileToBase64(file);
	const ocr = await ocrPdfFile(dataUrl);
	if (ocr?.text) {
		toast.success(`OCR extracted text from ${file.name} (${ocr.model})`);
		return ocrTextAttachment(file.name, ocr.text);
	}
	return null;
}

export async function fileToAttachment(file: File): Promise<MessageContent | null> {
	const mimeType = resolveFileMimeType(file);

	if (isZipFile(file, mimeType)) {
		// Prefer filesToAttachments (keeps each inner file separate). Single-return path combines text.
		const many = await attachmentsFromZip(file);
		if (many.length === 0) return null;
		if (many.length === 1) return many[0];
		const combined = many
			.map((a) => {
				if (a.type === "text" && a.text) return a.text;
				if (a.type === "image_url") return "[Image attachment]";
				if (a.type === "input_audio") return "[Audio attachment]";
				if (a.type === "file") return `[File: ${a.file?.filename || "unknown"}]`;
				return "";
			})
			.filter(Boolean)
			.join("\n\n---\n\n");
		return {
			type: "text",
			text: `Attached zip: ${file.name}\n\n${combined}`,
		};
	}

	if (isImageFile(file, mimeType)) {
		const dataUrl = await normalizeImageForModel(file, mimeType);
		if (dataUrl) {
			toast.success(`Image attached: ${file.name}`);
			return {
				type: "image_url",
				image_url: { url: dataUrl, detail: "auto" },
			};
		}
		const raw = await fileToBase64(file);
		const ocr = await ocrImageFile(raw);
		if (ocr?.text) {
			toast.message(`"${file.name}" could not be decoded by the browser — attached OCR text instead`);
			return ocrTextAttachment(file.name, ocr.text);
		}
		toast.error(`Unsupported image "${file.name}"`, {
			description: "Convert it to PNG or JPG and attach again.",
		});
		return null;
	}

	if (mimeType.startsWith("audio/")) {
		const normalized = (await normalizeAudioToWavFile(file)) || file;
		toast.message("Transcribing voice with Whisper…");
		const transcript = await transcribeAudioFile(normalized);
		if (transcript?.text) {
			toast.success(`Voice transcribed (${transcript.model})`);
			return voiceTranscriptAttachment(file.name, transcript.text);
		}
		toast.message("Whisper unavailable — attaching raw audio", {
			description: "Configure an OpenAI Whisper key/model, or use an audio-capable chat model.",
		});
		const dataUrl = await fileToBase64(normalized);
		const base64Data = dataUrl.split(",")[1] || "";
		const format = audioFormatFromMimeOrName(normalized.type || mimeType, normalized.name || file.name);
		if (format === "webm" || format === "ogg") {
			toast.message(`"${file.name}" kept as ${format}`, {
				description: "Some models only accept wav/mp3. If run fails, export as WAV/MP3 and re-import.",
			});
		} else if (normalized !== file) {
			toast.success("Voice converted to WAV for model compatibility");
		}
		return {
			type: "input_audio",
			input_audio: { data: base64Data, format },
		};
	}

	const extracted = await extractPromptFileText(file, mimeType);
	const pdf = isPdfFile(file, mimeType);

	// Scanned / image-only PDF: text layer empty or thin → OCR document_url
	if (pdf && isThinExtractedText(extracted)) {
		const ocrAtt = await tryPdfOcr(file);
		if (ocrAtt) return ocrAtt;
		if (extracted && extracted.trim()) {
			toast.message(`Using thin PDF text layer for ${file.name}`, {
				description: "OCR unavailable. Configure Mistral OCR for better scan results.",
			});
			return textAttachmentFromExtract(file.name, extracted);
		}
		toast.error(`Could not extract text from scanned PDF "${file.name}"`, {
			description: "Configure a Mistral OCR key/model, or use a text-based PDF.",
		});
		return null;
	}

	if (extracted && extracted.trim()) {
		toast.success(`Extracted text from ${file.name}`);
		return textAttachmentFromExtract(file.name, extracted);
	}

	const lower = file.name.toLowerCase();
	if (
		/\.(pdf|docx|doc|xlsx|xls|csv|txt|md|json|xml|html|htm|pptx|ppt|ppsx|pps)$/i.test(lower) ||
		mimeType.startsWith("text/")
	) {
		toast.error(`Could not extract readable text from "${file.name}"`, {
			description: "File may be empty, image-only, or corrupted. Try re-exporting or paste the content.",
		});
		return null;
	}

	const dataUrl = await fileToBase64(file);
	toast.message(`Attached "${file.name}" as raw file`, {
		description: "This model may ignore raw file blocks. Prefer PDF/DOCX/XLSX/TXT when possible.",
	});
	return {
		type: "file",
		file: {
			file_data: dataUrl,
			filename: file.name,
			file_type: mimeType,
		},
	};
}

export function getAttachmentDisplayName(attachment: MessageContent): string {
	if (attachment.type === "image_url") return "Image";
	if (attachment.type === "input_audio") return attachment.input_audio?.format?.toUpperCase() || "Voice";
	if (attachment.type === "text" && attachment.text?.startsWith("Voice transcript")) return "Voice transcript";
	if (attachment.type === "text" && attachment.text?.includes("--- OCR extracted content ---")) {
		const firstLine = attachment.text.split("\n")[0] || "";
		return `OCR: ${firstLine.replace(/^Attached file:\s*/i, "").trim() || "Image"}`;
	}
	if (attachment.type === "text" && attachment.text?.startsWith("Attached file:")) {
		const firstLine = attachment.text.split("\n")[0] || "";
		return firstLine.replace(/^Attached file:\s*/i, "").trim() || "File";
	}
	return attachment.file?.filename || "File";
}

export function attachmentNeedsVision(attachments: MessageContent[]): boolean {
	return attachments.some((a) => a.type === "image_url");
}

export function attachmentNeedsAudio(attachments: MessageContent[]): boolean {
	return attachments.some((a) => a.type === "input_audio");
}
