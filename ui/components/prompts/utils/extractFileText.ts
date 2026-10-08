import pdfjsWorker from "pdfjs-dist/build/pdf.worker.min.mjs?url";

const MAX_EXTRACT_CHARS = 120_000;
const MAX_PDF_PAGES = 50;
const MAX_PPTX_SLIDES = 50;

let pdfjsReady: Promise<typeof import("pdfjs-dist")> | null = null;

async function getPdfJs() {
	if (!pdfjsReady) {
		pdfjsReady = import("pdfjs-dist").then((pdfjs) => {
			pdfjs.GlobalWorkerOptions.workerSrc = pdfjsWorker;
			return pdfjs;
		});
	}
	return pdfjsReady;
}

function htmlToPlainText(html: string): string {
	return (html || "")
		.replace(/<script\b[\s\S]*?<\/script>/gi, " ")
		.replace(/<style\b[\s\S]*?<\/style>/gi, " ")
		.replace(/<br\s*\/?>/gi, "\n")
		.replace(/<\/(p|div|tr|li|h[1-6])>/gi, "\n")
		.replace(/<[^>]+>/g, " ")
		.replace(/&nbsp;/g, " ")
		.replace(/&amp;/g, "&")
		.replace(/&lt;/g, "<")
		.replace(/&gt;/g, ">")
		.replace(/&quot;/g, '"')
		.replace(/\s+\n/g, "\n")
		.replace(/[ \t]{2,}/g, " ")
		.trim();
}

function decodeXmlEntities(s: string): string {
	return s
		.replace(/&amp;/g, "&")
		.replace(/&lt;/g, "<")
		.replace(/&gt;/g, ">")
		.replace(/&quot;/g, '"')
		.replace(/&apos;/g, "'")
		.replace(/&#(\d+);/g, (_, n) => String.fromCharCode(Number(n)))
		.replace(/&#x([0-9a-fA-F]+);/g, (_, n) => String.fromCharCode(Number.parseInt(n, 16)));
}

function isUsefulExtractedText(text: string): boolean {
	const t = (text || "").trim();
	if (t.length < 8) return false;
	const letters = (t.match(/[A-Za-z\u00C0-\u024F\u0900-\u097F\u0B80-\u0BFF]/g) || []).length;
	const digits = (t.match(/[0-9]/g) || []).length;
	const spaces = (t.match(/\s/g) || []).length;
	if (letters + digits < 12) return false;
	if (t.length > 40 && spaces < 2 && letters > 30) return false;
	return true;
}

/** Harvest printable ASCII + UTF-16LE runs from binary Office streams (.doc / .ppt). */
function extractBinaryOfficeStrings(bytes: Uint8Array): string {
	const parts: string[] = [];

	// ASCII / Latin-1 runs
	let ascii = "";
	for (let i = 0; i < bytes.length; i++) {
		const b = bytes[i];
		if (b >= 0x20 && b <= 0x7e) {
			ascii += String.fromCharCode(b);
		} else if (b === 0x0a || b === 0x0d || b === 0x09) {
			ascii += " ";
		} else {
			if (ascii.trim().length >= 5) parts.push(ascii.trim());
			ascii = "";
		}
	}
	if (ascii.trim().length >= 5) parts.push(ascii.trim());

	// UTF-16LE runs (common in OLE Word/PowerPoint)
	let utf16 = "";
	for (let i = 0; i + 1 < bytes.length; i += 2) {
		const code = bytes[i] | (bytes[i + 1] << 8);
		if (code >= 0x20 && code <= 0xfffd && code !== 0xfeff) {
			utf16 += String.fromCharCode(code);
		} else if (code === 0x0a || code === 0x0d || code === 0x09) {
			utf16 += " ";
		} else {
			if (utf16.trim().length >= 5) parts.push(utf16.trim());
			utf16 = "";
		}
	}
	if (utf16.trim().length >= 5) parts.push(utf16.trim());

	const cleaned = parts
		.map((p) => p.replace(/\s+/g, " ").trim())
		.filter((p) => p.length >= 5)
		.filter((p) => /[A-Za-z\u00C0-\u024F\u0900-\u097F\u0B80-\u0BFF0-9]/.test(p))
		// Drop obvious OLE/binary junk tokens
		.filter((p) => !/^(Root Entry|WordDocument|PowerPoint Document|SummaryInformation|CompObj|Ole|ObjectPool)/i.test(p))
		.filter((p) => !/^[\x00-\x1f]+$/.test(p));

	// Prefer longer unique lines
	const seen = new Set<string>();
	const unique: string[] = [];
	for (const line of cleaned.sort((a, b) => b.length - a.length)) {
		const key = line.toLowerCase();
		if (seen.has(key)) continue;
		// Skip lines that are mostly the same char
		if (/^(.)\1{8,}$/.test(line.replace(/\s/g, ""))) continue;
		seen.add(key);
		unique.push(line);
		if (unique.join("\n").length > MAX_EXTRACT_CHARS) break;
	}

	return unique.join("\n").trim();
}

async function extractFromOleCompound(buf: ArrayBuffer, preferredStreams: string[]): Promise<string> {
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const CFBmod: any = await import("cfb");
	const CFB = CFBmod.default ?? CFBmod;
	const cfb = CFB.read(new Uint8Array(buf), { type: "array" });

	const chunks: string[] = [];
	for (const name of preferredStreams) {
		try {
			const entry = CFB.find(cfb, name);
			if (!entry?.content) continue;
			const content = entry.content as Uint8Array | number[];
			const bytes = content instanceof Uint8Array ? content : Uint8Array.from(content);
			const text = extractBinaryOfficeStrings(bytes);
			if (text) chunks.push(text);
		} catch {
			/* try next stream */
		}
	}

	// Also scan all streams and pick the richest text
	try {
		const paths: string[] = Array.isArray(cfb.FullPaths) ? cfb.FullPaths : [];
		for (const path of paths) {
			if (!path || path.endsWith("/")) continue;
			if (preferredStreams.some((p) => path.toLowerCase().includes(p.toLowerCase().replace(/^\//, "")))) {
				continue;
			}
			try {
				const entry = CFB.find(cfb, path);
				if (!entry?.content) continue;
				const content = entry.content as Uint8Array | number[];
				const bytes = content instanceof Uint8Array ? content : Uint8Array.from(content);
				if (bytes.length < 64 || bytes.length > 8_000_000) continue;
				const text = extractBinaryOfficeStrings(bytes);
				if (text && text.length > 40) chunks.push(text);
			} catch {
				/* skip */
			}
		}
	} catch {
		/* FullPaths may be unavailable */
	}

	// Whole-file fallback
	const whole = extractBinaryOfficeStrings(new Uint8Array(buf));
	if (whole) chunks.push(whole);

	const best = chunks.reduce((a, b) => (b.length > a.length ? b : a), "");
	return best.trim();
}

async function extractDocText(buf: ArrayBuffer): Promise<string> {
	return extractFromOleCompound(buf, ["/WordDocument", "WordDocument", "/1Table", "1Table", "/0Table", "0Table"]);
}

async function extractPptBinaryText(buf: ArrayBuffer): Promise<string> {
	return extractFromOleCompound(buf, [
		"/PowerPoint Document",
		"PowerPoint Document",
		"/Current User",
		"Current User",
	]);
}

async function extractPptxText(buf: ArrayBuffer): Promise<string> {
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const JSzipMod: any = await import("jszip");
	const JSZip = JSzipMod.default ?? JSzipMod;
	const zip = await JSZip.loadAsync(buf);
	const slideNames = Object.keys(zip.files)
		.filter((n: string) => /^ppt\/slides\/slide\d+\.xml$/i.test(n))
		.sort((a: string, b: string) => {
			const na = Number(a.match(/slide(\d+)/i)?.[1] || 0);
			const nb = Number(b.match(/slide(\d+)/i)?.[1] || 0);
			return na - nb;
		})
		.slice(0, MAX_PPTX_SLIDES);

	const parts: string[] = [];
	for (const name of slideNames) {
		const xml = await zip.files[name].async("string");
		const texts = [...xml.matchAll(/<a:t[^>]*>([\s\S]*?)<\/a:t>/g)].map((m) => decodeXmlEntities(m[1] || "").trim());
		const slideText = texts.filter(Boolean).join(" ").replace(/\s+/g, " ").trim();
		if (slideText) {
			const num = name.match(/slide(\d+)/i)?.[1] || "?";
			parts.push(`## Slide ${num}\n${slideText}`);
		}
	}

	// Notes
	const noteNames = Object.keys(zip.files).filter((n: string) => /^ppt\/notesSlides\/notesSlide\d+\.xml$/i.test(n));
	for (const name of noteNames.slice(0, MAX_PPTX_SLIDES)) {
		const xml = await zip.files[name].async("string");
		const texts = [...xml.matchAll(/<a:t[^>]*>([\s\S]*?)<\/a:t>/g)].map((m) => decodeXmlEntities(m[1] || "").trim());
		const noteText = texts.filter(Boolean).join(" ").replace(/\s+/g, " ").trim();
		if (noteText) {
			const num = name.match(/notesSlide(\d+)/i)?.[1] || "?";
			parts.push(`## Notes ${num}\n${noteText}`);
		}
	}

	return parts.join("\n\n").trim();
}

async function extractPdfText(bytes: Uint8Array): Promise<string> {
	const pdfjs = await getPdfJs();
	const doc = await pdfjs.getDocument({ data: bytes.slice() }).promise;
	const parts: string[] = [];
	const maxPages = Math.min(doc.numPages, MAX_PDF_PAGES);
	for (let i = 1; i <= maxPages; i++) {
		const page = await doc.getPage(i);
		const content = await page.getTextContent();
		let pageText = "";
		let lastY: number | null = null;
		for (const rawItem of content.items) {
			if (!("str" in rawItem)) continue;
			const item = rawItem as { str: string; transform?: number[]; hasEOL?: boolean };
			const str = item.str;
			if (!str) continue;
			const y = Array.isArray(item.transform) ? item.transform[5] : null;
			if (lastY !== null && y !== null && Math.abs(y - lastY) > 5) {
				pageText += "\n";
			} else if (item.hasEOL) {
				pageText += "\n";
			} else if (pageText && !pageText.endsWith("\n") && !pageText.endsWith(" ")) {
				pageText += " ";
			}
			pageText += str;
			if (y !== null) lastY = y;
		}
		const trimmedPage = pageText.trim();
		if (trimmedPage) {
			parts.push(maxPages > 1 ? `[Page ${i} of ${doc.numPages}]\n${trimmedPage}` : trimmedPage);
		}
	}
	const text = parts.join("\n\n").trim();
	if (doc.numPages > maxPages) {
		return `${text}\n\n[Truncated: showed first ${maxPages} of ${doc.numPages} pages]`;
	}
	return text;
}

async function extractDocxText(buf: ArrayBuffer): Promise<string> {
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const mammoth: any = await import("mammoth");
	const api = mammoth.default ?? mammoth;
	const result = await api.extractRawText({ arrayBuffer: buf });
	const text = String(result?.value || "").trim();
	if (text) return text;
	const htmlResult = await api.convertToHtml({ arrayBuffer: buf });
	return htmlToPlainText(String(htmlResult?.value || ""));
}

async function extractSpreadsheetText(buf: ArrayBuffer): Promise<string> {
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const XLSXmod: any = await import("xlsx");
	const XLSX = XLSXmod.default ?? XLSXmod;
	const wb = XLSX.read(buf, { type: "array" });
	const chunks: string[] = [];
	for (const sheetName of wb.SheetNames.slice(0, 10)) {
		const sheet = wb.Sheets[sheetName];
		const csv = XLSX.utils.sheet_to_csv(sheet);
		if (csv?.trim()) {
			chunks.push(`## Sheet: ${sheetName}\n${csv.trim()}`);
		}
	}
	return chunks.join("\n\n");
}

function looksLikeZipOffice(bytes: Uint8Array): boolean {
	return bytes.length >= 4 && bytes[0] === 0x50 && bytes[1] === 0x4b;
}

function looksLikeOleCompound(bytes: Uint8Array): boolean {
	// D0 CF 11 E0 A1 B1 1A E1
	return (
		bytes.length >= 8 &&
		bytes[0] === 0xd0 &&
		bytes[1] === 0xcf &&
		bytes[2] === 0x11 &&
		bytes[3] === 0xe0 &&
		bytes[4] === 0xa1 &&
		bytes[5] === 0xb1 &&
		bytes[6] === 0x1a &&
		bytes[7] === 0xe1
	);
}

function isTextBytes(bytes: Uint8Array): boolean {
	if (bytes.length === 0) return false;
	const checkLen = Math.min(bytes.length, 8192);
	let nonPrintable = 0;
	for (let i = 0; i < checkLen; i++) {
		const b = bytes[i];
		if (b === 0x00) return false;
		if (b < 0x09 || (b > 0x0d && b < 0x20)) {
			nonPrintable++;
		}
	}
	return nonPrintable / checkLen < 0.05;
}

function extractJupyterNotebookText(raw: string): string {
	try {
		const nb = JSON.parse(raw);
		if (!Array.isArray(nb.cells)) return raw;
		const parts: string[] = [];
		for (const cell of nb.cells) {
			const src = Array.isArray(cell.source) ? cell.source.join("") : String(cell.source || "");
			if (!src.trim()) continue;
			if (cell.cell_type === "markdown") {
				parts.push(src.trim());
			} else if (cell.cell_type === "code") {
				parts.push("```python\n" + src.trim() + "\n```");
			} else {
				parts.push(src.trim());
			}
		}
		return parts.join("\n\n");
	} catch {
		return raw;
	}
}

async function extractOdfText(buf: ArrayBuffer): Promise<string> {
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const JSzipMod: any = await import("jszip");
	const JSZip = JSzipMod.default ?? JSzipMod;
	const zip = await JSZip.loadAsync(buf);
	const contentXml = zip.files["content.xml"];
	if (!contentXml) return "";
	const xml = await contentXml.async("string");
	const matches = [...xml.matchAll(/<text:(?:p|h)[^>]*>([\s\S]*?)<\/text:(?:p|h)>/g)];
	const lines = matches.map((m) => htmlToPlainText(m[1] || "")).filter(Boolean);
	return lines.join("\n\n").trim();
}

function extractRtfText(rtf: string): string {
	return rtf
		.replace(/\\par[d]?/g, "\n")
		.replace(/\\tab/g, "\t")
		.replace(/\\line/g, "\n")
		.replace(/\\'[0-9a-fA-F]{2}/g, " ")
		.replace(/\\[a-zA-Z]+-?\d* ?/g, "")
		.replace(/[{}]/g, "")
		.replace(/\n\s*\n+/g, "\n\n")
		.trim();
}

const TEXT_CODE_EXT_RE =
	/\.(txt|md|markdown|rst|tex|latex|json|jsonl|jsonc|xml|html|htm|xhtml|yaml|yml|toml|ini|env|conf|config|properties|proto|graphql|gql|sql|sh|bash|zsh|fish|ps1|bat|cmd|py|pyw|js|jsx|mjs|cjs|ts|tsx|mts|cts|java|go|rs|c|h|cpp|hpp|cc|cxx|hh|hxx|cs|php|rb|swift|kt|kts|scala|r|m|mm|css|scss|sass|less|vue|svelte|dart|lua|pl|pm|tcl|dockerfile|makefile|gitignore|npmrc|diff|patch|log|csv|tsv|svg)$/i;

const TEXT_STANDALONE_NAMES = new Set([
	"dockerfile",
	"makefile",
	"license",
	"licence",
	"readme",
	"changelog",
	"gemfile",
	"procfile",
	"vagrantfile",
	"jenkinsfile",
]);

/** Best-effort plain text for prompt-repo attachments (PDF / Office / Code / Text). */
export async function extractPromptFileText(file: File, mimeType: string): Promise<string | null> {
	const buf = await file.arrayBuffer();
	const bytes = new Uint8Array(buf);
	if (bytes.length === 0) {
		return null;
	}

	const name = file.name.toLowerCase();
	const mime = (mimeType || file.type || "").toLowerCase();

	try {
		// PDF
		if (mime.includes("pdf") || name.endsWith(".pdf") || (bytes.length >= 5 && new TextDecoder().decode(bytes.slice(0, 5)) === "%PDF-")) {
			const text = await extractPdfText(bytes);
			return text.trim() ? text.slice(0, MAX_EXTRACT_CHARS) : null;
		}

		// Word .docx
		if (mime.includes("wordprocessingml") || name.endsWith(".docx") || (looksLikeZipOffice(bytes) && name.endsWith(".docx"))) {
			const text = await extractDocxText(buf);
			return text.trim() ? text.slice(0, MAX_EXTRACT_CHARS) : null;
		}

		// Legacy Word .doc (OLE)
		if (mime.includes("msword") || name.endsWith(".doc") || (looksLikeOleCompound(bytes) && name.endsWith(".doc"))) {
			const text = await extractDocText(buf);
			if (text && isUsefulExtractedText(text)) return text.slice(0, MAX_EXTRACT_CHARS);
			return text.trim() ? text.slice(0, MAX_EXTRACT_CHARS) : null;
		}

		// PowerPoint OOXML (.pptx, .ppsx)
		if (
			mime.includes("presentationml") ||
			name.endsWith(".pptx") ||
			name.endsWith(".ppsx") ||
			(looksLikeZipOffice(bytes) && (name.endsWith(".pptx") || name.endsWith(".ppsx")))
		) {
			const text = await extractPptxText(buf);
			return text.trim() ? text.slice(0, MAX_EXTRACT_CHARS) : null;
		}

		// Legacy PowerPoint .ppt (OLE)
		if (mime.includes("ms-powerpoint") || name.endsWith(".ppt") || name.endsWith(".pps") || (looksLikeOleCompound(bytes) && name.endsWith(".ppt"))) {
			const text = await extractPptBinaryText(buf);
			if (text && isUsefulExtractedText(text)) return text.slice(0, MAX_EXTRACT_CHARS);
			return text.trim() ? text.slice(0, MAX_EXTRACT_CHARS) : null;
		}

		// OpenDocument Text (.odt)
		if (name.endsWith(".odt") || mime.includes("opendocument.text")) {
			const text = await extractOdfText(buf);
			return text.trim() ? text.slice(0, MAX_EXTRACT_CHARS) : null;
		}

		// OpenDocument Presentation (.odp)
		if (name.endsWith(".odp") || mime.includes("opendocument.presentation")) {
			const text = await extractOdfText(buf);
			return text.trim() ? text.slice(0, MAX_EXTRACT_CHARS) : null;
		}

		// Spreadsheets (.xlsx, .xls, .ods, .csv, .tsv)
		if (
			mime.includes("spreadsheetml") ||
			mime.includes("excel") ||
			name.endsWith(".xlsx") ||
			name.endsWith(".xls") ||
			name.endsWith(".ods") ||
			name.endsWith(".csv") ||
			name.endsWith(".tsv") ||
			mime.includes("csv") ||
			mime.includes("tab-separated")
		) {
			if (name.endsWith(".csv") || name.endsWith(".tsv") || mime.includes("csv") || mime.includes("tab-separated")) {
				const text = new TextDecoder().decode(bytes);
				return text.trim() ? text.slice(0, MAX_EXTRACT_CHARS) : null;
			}
			const text = await extractSpreadsheetText(buf);
			return text.trim() ? text.slice(0, MAX_EXTRACT_CHARS) : null;
		}

		// Jupyter Notebook (.ipynb)
		if (name.endsWith(".ipynb")) {
			const raw = new TextDecoder().decode(bytes);
			const text = extractJupyterNotebookText(raw);
			return text.trim() ? text.slice(0, MAX_EXTRACT_CHARS) : null;
		}

		// Rich Text Format (.rtf)
		if (name.endsWith(".rtf") || mime.includes("rtf")) {
			const raw = new TextDecoder().decode(bytes);
			const text = extractRtfText(raw);
			return text.trim() ? text.slice(0, MAX_EXTRACT_CHARS) : null;
		}

		// Code, Markdown, Config, Markup, Text files
		const isKnownTextExt = TEXT_CODE_EXT_RE.test(name) || TEXT_STANDALONE_NAMES.has(name);
		const isTextMime =
			mime.startsWith("text/") ||
			mime.includes("json") ||
			mime.includes("xml") ||
			mime.includes("yaml") ||
			mime.includes("javascript") ||
			mime.includes("typescript") ||
			mime.includes("markdown");

		if (isKnownTextExt || isTextMime || isTextBytes(bytes)) {
			let text = new TextDecoder().decode(bytes);
			if (name.endsWith(".json") || name.endsWith(".jsonc") || mime.includes("json")) {
				try {
					text = JSON.stringify(JSON.parse(text), null, 2);
				} catch {
					/* keep raw text */
				}
			}
			if (name.endsWith(".html") || name.endsWith(".htm") || name.endsWith(".xhtml") || mime.includes("html")) {
				text = htmlToPlainText(text);
			}
			return text.trim() ? text.slice(0, MAX_EXTRACT_CHARS) : null;
		}

		// Generic OLE fallback (unknown extension but compound file)
		if (looksLikeOleCompound(bytes)) {
			const text = await extractFromOleCompound(buf, []);
			return text.trim() ? text.slice(0, MAX_EXTRACT_CHARS) : null;
		}
	} catch (error) {
		console.warn("Failed to extract text from attachment:", file.name, error);
		return null;
	}

	return null;
}
