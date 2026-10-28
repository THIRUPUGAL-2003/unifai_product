/** Client-side preview helpers for Browser AI attachment viewer. */

import pdfjsWorker from "pdfjs-dist/build/pdf.worker.min.mjs?url";
import DOMPurify from "dompurify";

export type AttachmentPreviewKind = "pdf" | "image" | "html" | "text" | "unsupported";

export type AttachmentSheetPreview = {
	name: string;
	html: string;
	rowCount: number;
};

export type AttachmentPreviewResult = {
	kind: AttachmentPreviewKind;
	blobUrl?: string;
	html?: string;
	text?: string;
	/** Multi-sheet workbooks — UI can switch / show all. */
	sheets?: AttachmentSheetPreview[];
	/** True when CSV/XLSX was capped for first paint. */
	truncated?: boolean;
	totalRows?: number;
};

const CSV_PREVIEW_ROWS = 200;
const CSV_SHOW_ALL_MAX = 5000;
const XLSX_PREVIEW_ROWS = 200;
const XLSX_SHOW_ALL_MAX = 5000;

/** Heuristic: distinguish readable extracted text from PDF-binary regex garbage. */
export function isReadableExtractedText(text: string): boolean {
	const t = (text || "").trim();
	if (t.length < 6) return false;

	const letters = (t.match(/[A-Za-z\u00C0-\u024F\u0900-\u097F\u0B80-\u0BFF]/g) || []).length;
	const digits = (t.match(/[0-9]/g) || []).length;
	const spaces = (t.match(/\s/g) || []).length;
	const printable = (t.match(/[\x20-\x7E\u00A0-\uFFFF]/g) || []).length;
	const ratio = printable / t.length;

	if (ratio < 0.75) return false;
	if (letters + digits < Math.min(12, t.length * 0.08)) return false;

	if (/[@#^*]{4,}/.test(t)) return false;
	if (/[^\w\s.,!?;:'"()\-–—/\\[\]{}%$&+=<>@#^*|`~]{10,}/.test(t)) return false;

	if (t.length > 40 && spaces < 2 && letters > 30) return false;
	if (t.startsWith("[null,") || t.startsWith("f.req=") || t.startsWith("[[[") || t.includes("/contrib service")) {
		return false;
	}

	return true;
}

/** Pick the best readable extracted text from multiple sources (client PDF parse preferred over bad metadata). */
export function pickBestExtractedText(...sources: Array<string | undefined | null>): string {
	const candidates = sources.map((s) => (s || "").trim()).filter(Boolean);
	if (!candidates.length) return "";

	const readable = candidates.filter(isReadableExtractedText);
	if (readable.length) {
		return readable.reduce((best, cur) => (cur.length > best.length ? cur : best));
	}
	return candidates.reduce((best, cur) => (cur.length > best.length ? cur : best));
}

export function attachmentExt(name?: string, contentType?: string): string {
	const n = (name || "").toLowerCase();
	const dot = n.lastIndexOf(".");
	if (dot >= 0) return n.slice(dot);
	const ct = (contentType || "").toLowerCase();
	if (ct.includes("pdf")) return ".pdf";
	if (ct.includes("png")) return ".png";
	if (ct.includes("jpeg") || ct.includes("jpg")) return ".jpg";
	if (ct.includes("gif")) return ".gif";
	if (ct.includes("webp")) return ".webp";
	if (ct.includes("wordprocessingml") || ct.includes("msword")) return ".docx";
	if (ct.includes("spreadsheetml") || ct.includes("excel")) return ".xlsx";
	if (ct.includes("text/plain")) return ".txt";
	if (ct.includes("csv")) return ".csv";
	if (ct.includes("json")) return ".json";
	return "";
}

const IMAGE_EXTS = new Set([
	".png", ".jpg", ".jpeg", ".jpe", ".webp", ".gif", ".bmp", ".dib", ".svg",
	".ico", ".cur", ".tif", ".tiff", ".avif", ".heic", ".heif", ".jxl",
]);

const TEXT_CODE_EXTS = new Set([
	".txt", ".text", ".md", ".markdown", ".mdown", ".rst", ".tex", ".latex", ".rtf",
	".json", ".jsonl", ".ndjson", ".jsonc", ".xml", ".html", ".htm", ".xhtml",
	".yaml", ".yml", ".toml", ".ini", ".env", ".conf", ".config", ".cfg", ".properties",
	".editorconfig", ".map", ".proto", ".graphql", ".gql", ".sql", ".sh", ".bash",
	".zsh", ".fish", ".ps1", ".psm1", ".psd1", ".bat", ".cmd", ".py", ".pyw", ".pyi",
	".pyx", ".jsx", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts", ".java",
	".go", ".rs", ".rlib", ".c", ".h", ".cpp", ".hpp", ".cc", ".cxx", ".hh", ".hxx",
	".cs", ".csx", ".fs", ".fsi", ".fsx", ".php", ".phtml", ".phar", ".rb", ".rake",
	".gemspec", ".swift", ".kt", ".kts", ".dart", ".scala", ".sc", ".r", ".rmd",
	".m", ".mlx", ".jl", ".pl", ".pm", ".t", ".lua", ".asm", ".s", ".f", ".for",
	".f90", ".f95", ".cbl", ".cob", ".cpy", ".mm", ".ex", ".exs", ".erl", ".hrl",
	".hs", ".lhs", ".clj", ".cljs", ".cljc", ".edn", ".lisp", ".lsp", ".cl", ".pro",
	".sol", ".wat", ".wasm", ".vue", ".svelte", ".aspx", ".ascx", ".cshtml", ".razor",
	".manifest", ".webmanifest", ".tf", ".tfvars", ".tfstate", ".j2", ".jinja",
	".jinja2", ".cmake", ".gradle", ".pom", ".sln", ".slnx", ".csproj", ".vcxproj",
	".fsproj", ".lock", ".diff", ".patch", ".log", ".csv", ".tsv", ".svg", ".srt",
	".vtt", ".ass", ".ssa", ".ttml", ".dfxp", ".prompt", ".dvc", ".pem", ".crt",
	".cer", ".cert", ".csr", ".key", ".pub", ".asc", ".sig", ".sha256", ".sha512",
	".md5", ".sha1", ".spdx", ".gpg", ".pgp", ".eml", ".vcf", ".ics", ".ical",
	".obj", ".stl", ".gcode", ".nc", ".tap", ".scad", ".dxf", ".step", ".stp",
	".iges", ".igs", ".kicad_sch", ".kicad_pcb", ".gbr", ".ger", ".gtl", ".gbl",
	".brd", ".sch", ".geojson", ".kml", ".gpx", ".tab", ".mif", ".mid", ".topojson",
	".wkt", ".prj", ".osm", ".reg", ".service", ".socket", ".timer", ".cron", ".plist",
	".bib", ".ris", ".rdf", ".fa", ".fasta", ".fna", ".fq", ".fastq", ".sam", ".cif",
	".qmd", ".nb", ".wl", ".edi", ".x12", ".hlsl", ".glsl", ".shader", ".po", ".pot",
	".mo", ".xliff", ".xlf", ".strings", ".stringsdict", ".arb", ".rc", ".res",
	".storyboard", ".xib", ".har", ".warc", ".mhtml", ".mht", ".fb2", ".x_t", ".x_b",
	".lvproj", ".slx", ".mdl", ".cnc", ".stpnc", ".schdoc", ".pcbdoc", ".logicx",
	".fcpxml", ".usda", ".x3d", ".x3dv", ".wrl", ".unity", ".godot", ".tscn", ".tres",
	".enl", ".enlx", ".ofx", ".qfx", ".qif", ".sfz", ".lottie", ".m3u8", ".mpd", ".m4s",
	".dockerfile", ".makefile", ".gitignore", ".npmrc",
]);

export function classifyAttachmentPreview(name?: string, contentType?: string): AttachmentPreviewKind {
	const ext = attachmentExt(name, contentType);
	const ct = (contentType || "").toLowerCase();
	if (ct.startsWith("image/") || IMAGE_EXTS.has(ext)) return "image";
	if (ct.includes("pdf") || ext === ".pdf") return "pdf";
	if (ext === ".docx" || ct.includes("wordprocessingml")) return "html";
	if ([".xlsx", ".xls", ".csv"].includes(ext) || ct.includes("spreadsheetml") || ct.includes("csv")) return "html";
	if (ct.startsWith("text/") || ct.includes("json") || ct.includes("xml") || ct.includes("yaml") || TEXT_CODE_EXTS.has(ext)) return "text";
	return "unsupported";
}

function escapeHtml(s: string): string {
	return s
		.replace(/&/g, "&amp;")
		.replace(/</g, "&lt;")
		.replace(/>/g, "&gt;")
		.replace(/"/g, "&quot;");
}

/** Strip script/handlers from mammoth / sheet HTML before dangerouslySetInnerHTML using DOMPurify. */
export function sanitizePreviewHtml(html: string): string {
	if (!html) return "";
	return DOMPurify.sanitize(html, {
		USE_PROFILES: { html: true },
		ADD_ATTR: ["target"],
	});
}

function csvRowsToHtml(rows: string[]): string {
	const table = rows
		.map((row, i) => {
			const cells = row.split(",").map((c) => `<td class="border border-border px-2 py-1 text-xs">${escapeHtml(c)}</td>`);
			return `<tr class="${i === 0 ? "bg-muted/40 font-medium" : ""}">${cells.join("")}</tr>`;
		})
		.join("");
	return `<div class="overflow-auto"><table class="w-full border-collapse">${table}</table></div>`;
}

function sheetAoAToHtml(aoa: unknown[][], maxRows: number): { html: string; rowCount: number; truncated: boolean } {
	const total = aoa.length;
	const slice = aoa.slice(0, maxRows);
	const rowsHtml = slice
		.map((row, i) => {
			const cells = (row as unknown[]).map(
				(c) => `<td class="border border-border px-2 py-1 text-xs">${escapeHtml(c == null ? "" : String(c))}</td>`,
			);
			return `<tr class="${i === 0 ? "bg-muted/40 font-medium" : ""}">${cells.join("")}</tr>`;
		})
		.join("");
	return {
		html: `<div class="overflow-auto"><table class="w-full border-collapse">${rowsHtml}</table></div>`,
		rowCount: total,
		truncated: total > maxRows,
	};
}

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

async function extractPdfText(bytes: Uint8Array): Promise<string> {
	try {
		const pdfjs = await getPdfJs();
		const doc = await pdfjs.getDocument({ data: bytes.slice() }).promise;
		const parts: string[] = [];
		const maxPages = Math.min(doc.numPages, 50);
		for (let i = 1; i <= maxPages; i++) {
			const page = await doc.getPage(i);
			const content = await page.getTextContent();
			const line = content.items
				.map((item) => ("str" in item ? item.str : ""))
				.join(" ")
				.replace(/\s+/g, " ")
				.trim();
			if (line) parts.push(line);
		}
		return parts.join("\n\n").trim().slice(0, 50_000);
	} catch {
		return "";
	}
}

export type BuildAttachmentPreviewOptions = {
	/** When true, expand CSV/XLSX up to SHOW_ALL_MAX rows and include every sheet. */
	showAll?: boolean;
};

export async function buildAttachmentPreview(
	blob: Blob,
	name?: string,
	contentType?: string,
	options?: BuildAttachmentPreviewOptions,
): Promise<AttachmentPreviewResult> {
	const showAll = !!options?.showAll;
	const csvLimit = showAll ? CSV_SHOW_ALL_MAX : CSV_PREVIEW_ROWS;
	const xlsxLimit = showAll ? XLSX_SHOW_ALL_MAX : XLSX_PREVIEW_ROWS;

	const buf = await blob.arrayBuffer();
	const bytes = new Uint8Array(buf);
	if (bytes.length === 0) {
		return { kind: "text", text: "File is empty — no bytes were stored for this log." };
	}
	const head = new TextDecoder("utf-8", { fatal: false }).decode(bytes.slice(0, 16));
	let i = 0;
	while (i < bytes.length && (bytes[i] === 0x20 || bytes[i] === 0x09 || bytes[i] === 0x0a || bytes[i] === 0x0d)) i++;
	const first = i < bytes.length ? bytes[i] : 0;
	const sniffedType =
		bytes.length >= 5 && head.startsWith("%PDF-")
			? "application/pdf"
			: bytes.length >= 3 && bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff
				? "image/jpeg"
				: bytes.length >= 8 && bytes[0] === 0x89 && bytes[1] === 0x50 && bytes[2] === 0x4e && bytes[3] === 0x47
					? "image/png"
					: "";

	const kind = sniffedType.includes("pdf")
		? "pdf"
		: sniffedType.startsWith("image/")
			? "image"
			: classifyAttachmentPreview(name, sniffedType || contentType || blob.type);
	const ext = attachmentExt(name, sniffedType || contentType || blob.type);
	const typedBlob = sniffedType
		? new Blob([buf], { type: sniffedType })
		: new Blob([buf], { type: blob.type || contentType || "application/octet-stream" });

	const looksJson = first === 0x7b || first === 0x5b;
	if (looksJson) {
		const text = new TextDecoder().decode(bytes);
		let pretty = text;
		try {
			pretty = JSON.stringify(JSON.parse(text), null, 2);
		} catch {
			pretty = text;
		}
		const errMatch = pretty.match(/"(?:error|message)"\s*:\s*"([^"]+)"/);
		if (errMatch?.[1] && pretty.length < 2000) {
			return { kind: "text", text: `Could not load file: ${errMatch[1]}` };
		}
		return {
			kind: "text",
			text:
				`This log stored JSON (ChatGPT request / API metadata), not the original file bytes.\n` +
				`The prompt was intercepted, but the PDF/image itself was not captured in this event.\n\n` +
				pretty.slice(0, showAll ? 200_000 : 8000),
			truncated: !showAll && pretty.length > 8000,
			totalRows: pretty.length,
		};
	}

	if (kind === "image" || kind === "pdf") {
		const text = kind === "pdf" ? await extractPdfText(bytes) : undefined;
		return { kind, blobUrl: URL.createObjectURL(typedBlob), text: text || undefined };
	}

	if (kind === "text" || ext === ".csv") {
		const text = new TextDecoder().decode(bytes);
		if (ext === ".csv") {
			const allRows = text.split(/\r?\n/).filter((l) => l.length > 0);
			const rows = allRows.slice(0, csvLimit);
			const truncated = allRows.length > csvLimit;
			return {
				kind: "html",
				html: sanitizePreviewHtml(csvRowsToHtml(rows)),
				truncated,
				totalRows: allRows.length,
				sheets: [
					{
						name: "CSV",
						html: sanitizePreviewHtml(csvRowsToHtml(rows)),
						rowCount: allRows.length,
					},
				],
			};
		}
		return {
			kind: "text",
			text: showAll ? text : text.slice(0, 50_000),
			truncated: !showAll && text.length > 50_000,
			totalRows: text.length,
		};
	}

	if (ext === ".docx" || (contentType || "").includes("wordprocessingml")) {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const mammoth: any = await import("mammoth");
		const api = mammoth.default ?? mammoth;
		const result = await api.convertToHtml({ arrayBuffer: buf });
		const html = result.value?.trim()
			? `<div class="prose prose-invert max-w-none text-sm leading-relaxed p-2">${sanitizePreviewHtml(result.value)}</div>`
			: `<p class="text-muted-foreground text-sm">Empty document.</p>`;
		let text = "";
		try {
			const textResult = await api.extractRawText({ arrayBuffer: buf });
			text = textResult.value?.trim() || "";
		} catch {
			/* fallback */
		}
		return { kind: "html", html, text: text || undefined };
	}

	if ([".xlsx", ".xls"].includes(ext) || (contentType || "").includes("spreadsheetml")) {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const XLSXmod: any = await import("xlsx");
		const XLSX = XLSXmod.default ?? XLSXmod;
		const wb = XLSX.read(buf, { type: "array" });
		if (!wb.SheetNames.length) {
			return { kind: "html", html: `<p class="text-sm text-muted-foreground">Workbook has no sheets.</p>` };
		}

		const sheets: AttachmentSheetPreview[] = [];
		let anyTruncated = false;
		let totalRows = 0;
		for (const sheetName of wb.SheetNames) {
			const sheet = wb.Sheets[sheetName];
			const aoa = XLSX.utils.sheet_to_json(sheet, { header: 1, defval: "" }) as unknown[][];
			const built = sheetAoAToHtml(aoa, xlsxLimit);
			totalRows += built.rowCount;
			if (built.truncated) anyTruncated = true;
			sheets.push({
				name: sheetName,
				html: sanitizePreviewHtml(built.html),
				rowCount: built.rowCount,
			});
		}

		const first = sheets[0];
		const moreNote =
			sheets.length > 1 ? ` · ${sheets.length} sheets` : "";
		const truncNote = anyTruncated && !showAll ? ` · showing first ${xlsxLimit} rows (Show all available)` : "";
		const wrapped = `<div class="overflow-auto text-xs">
			<p class="text-[11px] text-muted-foreground mb-2">Sheet: ${escapeHtml(first.name)}${escapeHtml(moreNote)}${escapeHtml(truncNote)}</p>
			${first.html}
		</div>`;

		return {
			kind: "html",
			html: sanitizePreviewHtml(wrapped),
			sheets,
			truncated: anyTruncated && !showAll,
			totalRows,
		};
	}

	// Fallback for any plain-text / code / script files that reached this point
	const hasNul = bytes.subarray(0, Math.min(bytes.length, 4096)).includes(0);
	if (!hasNul && bytes.length > 0) {
		try {
			const text = new TextDecoder().decode(bytes);
			if (isReadableExtractedText(text) || text.length < 1000) {
				return {
					kind: "text",
					text: showAll ? text : text.slice(0, 50_000),
					truncated: !showAll && text.length > 50_000,
					totalRows: text.length,
				};
			}
		} catch {
			/* not utf-8 text */
		}
	}

	return { kind: "unsupported" };
}
