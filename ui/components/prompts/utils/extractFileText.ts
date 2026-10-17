import pdfjsWorker from "pdfjs-dist/build/pdf.worker.min.mjs?url";

const MAX_EXTRACT_CHARS = 10_000_000; // 10M characters (full context for long books & documents)
const MAX_PDF_PAGES = 10_000; // Unlimited PDF pages
const MAX_PPTX_SLIDES = 5_000; // Unlimited PowerPoint slides

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
	let visitedSlides = 0;
	for (const name of slideNames) {
		visitedSlides += 1;
		const xml = await zip.files[name].async("string");
		const texts = [...xml.matchAll(/<a:t[^>]*>([\s\S]*?)<\/a:t>/g)].map((m) => decodeXmlEntities(m[1] || "").trim());
		const slideText = texts.filter(Boolean).join(" ").replace(/\s+/g, " ").trim();
		if (slideText) {
			const num = name.match(/slide(\d+)/i)?.[1] || "?";
			parts.push(`## Slide ${num}\n${slideText}`);
			if (parts.join("\n\n").length >= MAX_EXTRACT_CHARS) break;
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

	let text = parts.join("\n\n").trim();
	const totalSlides = Object.keys(zip.files).filter((n: string) => /^ppt\/slides\/slide\d+\.xml$/i.test(n)).length;
	if (visitedSlides > 0 && visitedSlides < totalSlides) {
		text += `\n\n[Truncated: showed first ${visitedSlides} of ${totalSlides} slides]`;
	}
	return text;
}

async function extractPdfText(bytes: Uint8Array): Promise<string> {
	const pdfjs = await getPdfJs();
	const doc = await pdfjs.getDocument({ data: bytes.slice() }).promise;
	const parts: string[] = [];
	const maxPages = Math.min(doc.numPages, MAX_PDF_PAGES);
	let reached = 0;
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
		reached = i;
		if (trimmedPage) {
			parts.push(doc.numPages > 1 ? `[Page ${i} of ${doc.numPages}]\n${trimmedPage}` : trimmedPage);
			if (parts.join("\n\n").length >= MAX_EXTRACT_CHARS) break;
		}
	}
	let text = parts.join("\n\n").trim();
	if (reached > 0 && reached < doc.numPages) {
		text += `\n\n[Truncated: showed first ${reached} of ${doc.numPages} pages]`;
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
	const names: string[] = wb.SheetNames || [];
	let visited = 0;
	for (const sheetName of names) {
		if (chunks.join("\n\n").length >= MAX_EXTRACT_CHARS) break;
		visited += 1;
		const sheet = wb.Sheets[sheetName];
		const csv = XLSX.utils.sheet_to_csv(sheet);
		if (csv?.trim()) {
			chunks.push(`## Sheet: ${sheetName}\n${csv.trim()}`);
		}
	}
	let text = chunks.join("\n\n");
	if (visited < names.length) {
		text += `\n\n[Truncated: showed first ${visited} of ${names.length} sheets]`;
	}
	return text;
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

async function extractEpubText(buf: ArrayBuffer): Promise<string> {
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const JSzipMod: any = await import("jszip");
	const JSZip = JSzipMod.default ?? JSzipMod;
	const zip = await JSZip.loadAsync(buf);
	const names = Object.keys(zip.files)
		.filter((n: string) => /\.(xhtml|html|htm)$/i.test(n) && !zip.files[n].dir)
		.sort();
	const parts: string[] = [];
	for (const name of names.slice(0, 80)) {
		const html = await zip.files[name].async("string");
		const text = htmlToPlainText(html);
		if (text) parts.push(text);
		if (parts.join("\n\n").length > MAX_EXTRACT_CHARS) break;
	}
	return parts.join("\n\n").trim();
}

const TEXT_CODE_EXT_RE =
	/\.(txt|text|md|markdown|mdown|rst|tex|latex|json|jsonl|ndjson|jsonc|xml|html|htm|xhtml|yaml|yml|toml|ini|env|conf|config|cfg|properties|editorconfig|map|proto|graphql|gql|sql|sh|bash|zsh|fish|ps1|psm1|psd1|bat|cmd|py|pyw|pyi|pyx|jsx|js|mjs|cjs|ts|tsx|mts|cts|java|go|rs|rlib|c|h|cpp|hpp|cc|cxx|hh|hxx|cs|csx|fs|fsi|fsx|php|phtml|phar|rb|rake|gemspec|swift|kt|kts|dart|scala|sc|r|rmd|m|mlx|jl|pl|pm|t|lua|asm|s|f|for|f90|f95|cbl|cob|cpy|mm|ex|exs|erl|hrl|hs|lhs|clj|cljs|cljc|edn|lisp|lsp|cl|pro|sol|wat|wasm|vue|svelte|aspx|ascx|cshtml|razor|manifest|webmanifest|tf|tfvars|tfstate|j2|jinja|jinja2|cmake|gradle|pom|sln|slnx|csproj|vcxproj|fsproj|lock|diff|patch|log|csv|tsv|svg|srt|vtt|ass|ssa|ttml|dfxp|prompt|dvc|pem|crt|cer|cert|csr|key|pub|asc|sig|sha256|sha512|md5|sha1|spdx|gpg|pgp|eml|vcf|ics|ical|obj|stl|gcode|nc|tap|scad|dxf|step|stp|iges|igs|kicad_sch|kicad_pcb|gbr|ger|gtl|gbl|brd|sch|geojson|kml|gpx|tab|mif|mid|topojson|wkt|prj|osm|reg|service|socket|timer|cron|plist|bib|ris|rdf|fa|fasta|fna|fq|fastq|sam|cif|qmd|nb|wl|edi|x12|hlsl|glsl|shader|po|pot|mo|xliff|xlf|strings|stringsdict|arb|rc|res|storyboard|xib|har|warc|mhtml|mht|fb2|dart|x_t|x_b|lvproj|slx|mdl|cnc|stpnc|schdoc|pcbdoc|logicx|fcpxml|usda|x3d|x3dv|wrl|unity|godot|tscn|tres|shader|enl|enlx|zarr|ofx|qfx|qif|sfz|lottie|lnk|m3u8|mpd|m4s|mts|qtl|aup3|band|als|flp|ptx|dockerfile|makefile|gitignore|npmrc|ipynb)$/i;

const TEXT_STANDALONE_NAMES = new Set([
	"dockerfile",
	".dockerignore",
	"makefile",
	"jenkinsfile",
	"procfile",
	"gemfile",
	"rakefile",
	"justfile",
	"brewfile",
	"vagrantfile",
	"license",
	"license.txt",
	"licence",
	"readme",
	"readme.txt",
	"readme.md",
	"changelog",
	"contributing",
	"codeowners",
	"notice",
	".gitignore",
	".gitattributes",
	".editorconfig",
	".env",
	".env.local",
	".env.production",
	".env.development",
	".npmrc",
	".pypirc",
	".bashrc",
	".zshrc",
	".profile",
	".nvmrc",
	".node-version",
	"package.json",
	"package-lock.json",
	"yarn.lock",
	"pnpm-lock.yaml",
	"requirements.txt",
	"pyproject.toml",
	"setup.py",
	"setup.cfg",
	"cargo.toml",
	"cargo.lock",
	"go.mod",
	"go.sum",
	"pom.xml",
	"cmakelists.txt",
	"config",
	"authorized_keys",
	"known_hosts",
	"id_rsa",
	"id_ed25519",
	"robots.txt",
	"sitemap.xml",
	"manifest.json",
	"site.webmanifest",
	"compose.yaml",
	"compose.yml",
	"docker-compose.yml",
	"chart.yaml",
	"values.yaml",
	"mlmodel",
	"config.json",
	"tokenizer.json",
	"tokenizer_config.json",
	"vocab.json",
	"merges.txt",
]);

function clipExtract(text: string | null | undefined): string | null {
	const t = (text || "").trim();
	if (!t) return null;
	return t.slice(0, MAX_EXTRACT_CHARS);
}

/** Try each reader. One failure must not skip the readers after it. */
async function firstExtracted(steps: Array<() => Promise<string | null | undefined>>): Promise<string | null> {
	for (const step of steps) {
		try {
			const text = clipExtract(await step());
			if (!text) continue;
			if (isUsefulExtractedText(text) || text.length >= 4) return text;
		} catch (error) {
			console.warn("Prompt file extract step failed:", error);
		}
	}
	return null;
}

/** Best-effort plain text for prompt-repo attachments (PDF / Office / Code / Text). */
export async function extractPromptFileText(file: File, mimeType: string): Promise<string | null> {
	const buf = await file.arrayBuffer();
	const bytes = new Uint8Array(buf);
	if (bytes.length === 0) {
		return null;
	}

	const name = file.name.toLowerCase();
	const baseName = name.split("/").pop()?.split("\\").pop() || name;
	const mime = (mimeType || file.type || "").toLowerCase();

	const isPdf = mime.includes("pdf") || name.endsWith(".pdf") || (bytes.length >= 5 && new TextDecoder().decode(bytes.slice(0, 5)) === "%PDF-");
	const isZip = looksLikeZipOffice(bytes);

	try {
		const steps: Array<() => Promise<string | null | undefined>> = [];

		// PDF
		if (isPdf) {
			steps.push(() => extractPdfText(bytes));
		}

		const wordOoxml = /\.(docx|docm|dotx|dotm)$/i.test(name) || mime.includes("wordprocessingml");
		const sheetOoxml =
			/\.(xlsx|xlsm|xlsb|xltx|xltm|xlt|xls|ods|ots)$/i.test(name) ||
			mime.includes("spreadsheetml") ||
			mime.includes("opendocument.spreadsheet") ||
			mime.includes("excel");
		const pptOoxml =
			/\.(pptx|pptm|ppsx|ppsm|potx|potm)$/i.test(name) ||
			mime.includes("presentationml") ||
			(looksLikeZipOffice(bytes) && mime.includes("ms-powerpoint"));
		const odfText =
			/\.(odt|ott|odg|odp|otp)$/i.test(name) ||
			mime.includes("opendocument.text") ||
			mime.includes("opendocument.presentation") ||
			mime.includes("opendocument.graphics");

		// Word OOXML: .docx .docm .dotx .dotm — mammoth reads the document XML.
		if (wordOoxml || (isZip && /\.(docx|docm|dotx|dotm)$/i.test(name))) {
			steps.push(() => extractDocxText(buf));
		}

		// Legacy Word .doc is an OLE compound file, not a zip.
		if (!isZip && (name.endsWith(".doc") || name.endsWith(".dot") || mime.includes("msword") || (looksLikeOleCompound(bytes) && (name.endsWith(".doc") || name.endsWith(".dot"))))) {
			steps.push(async () => {
				const text = await extractDocText(buf);
				return text && isUsefulExtractedText(text) ? text : null;
			});
		}

		// PowerPoint OOXML, including macro and template packages.
		if (pptOoxml) {
			steps.push(() => extractPptxText(buf));
		}

		// Legacy PowerPoint .ppt / .pps is OLE. A zip package must not take this path.
		if (
			!isZip &&
			(name.endsWith(".ppt") || name.endsWith(".pps") || name.endsWith(".pot") || (mime.includes("ms-powerpoint") && !mime.includes("macroenabled")))
		) {
			steps.push(async () => {
				const text = await extractPptBinaryText(buf);
				return text && isUsefulExtractedText(text) ? text : null;
			});
		}

		// OpenDocument text, presentation, and drawing share content.xml.
		if (odfText) {
			steps.push(() => extractOdfText(buf));
		}

		// Spreadsheets: SheetJS for workbooks, plain decode for CSV/TSV.
		if (
			sheetOoxml ||
			name.endsWith(".csv") ||
			name.endsWith(".tsv") ||
			mime.includes("csv") ||
			mime.includes("tab-separated")
		) {
			if (name.endsWith(".csv") || name.endsWith(".tsv") || mime.includes("csv") || mime.includes("tab-separated")) {
				steps.push(async () => new TextDecoder().decode(bytes));
			} else {
				steps.push(() => extractSpreadsheetText(buf));
			}
		}

		// Jupyter Notebook (.ipynb)
		if (name.endsWith(".ipynb")) {
			steps.push(async () => extractJupyterNotebookText(new TextDecoder().decode(bytes)));
		}

		// Rich Text Format (.rtf)
		if (name.endsWith(".rtf") || mime.includes("rtf") || (bytes.length >= 5 && new TextDecoder().decode(bytes.slice(0, 5)) === "{\\rtf")) {
			steps.push(async () => extractRtfText(new TextDecoder().decode(bytes)));
		}

		// EPUB is a zip of HTML/XHTML chapters.
		if (name.endsWith(".epub") || mime.includes("epub")) {
			steps.push(() => extractEpubText(buf));
		}

		// Magic fallbacks when the extension was wrong or the first reader returned nothing.
		if (isZip && !name.endsWith(".epub")) {
			steps.push(() => extractDocxText(buf));
			steps.push(() => extractSpreadsheetText(buf));
			steps.push(() => extractPptxText(buf));
			steps.push(() => extractOdfText(buf));
			steps.push(() => extractEpubText(buf));
		}
		if (looksLikeOleCompound(bytes)) {
			steps.push(async () => {
				const text = await extractFromOleCompound(buf, []);
				return text && isUsefulExtractedText(text) ? text : null;
			});
		}

		// Code, Markdown, Config, Markup, Text files
		const isKnownTextExt = TEXT_CODE_EXT_RE.test(name) || TEXT_STANDALONE_NAMES.has(baseName) || TEXT_STANDALONE_NAMES.has(name);
		const isTextMime =
			mime.startsWith("text/") ||
			mime.includes("json") ||
			mime.includes("xml") ||
			mime.includes("yaml") ||
			mime.includes("javascript") ||
			mime.includes("typescript") ||
			mime.includes("markdown");

		const hasNul = bytes.subarray(0, Math.min(bytes.length, 8192)).includes(0);
		if (!hasNul && (isKnownTextExt || isTextMime || isTextBytes(bytes))) {
			steps.push(async () => {
				let text = new TextDecoder().decode(bytes);
				if (name.endsWith(".json") || name.endsWith(".jsonc") || mime.includes("json")) {
					try {
						text = JSON.stringify(JSON.parse(text), null, 2);
					} catch {
						/* keep raw text */
					}
				}
				if (
					name.endsWith(".html") ||
					name.endsWith(".htm") ||
					name.endsWith(".xhtml") ||
					name.endsWith(".fb2") ||
					mime.includes("html")
				) {
					text = htmlToPlainText(text);
				}
				return text;
			});
		}

		const extracted = await firstExtracted(steps);
		if (extracted) return extracted;

		// ── Binary / unknown fallback ─────────────────────────────────────────
		// For AI models, databases, executables, CAD binary, fonts, game assets,
		// VM images and any other format that reached this point: harvest printable
		// string runs (≥5 chars) so guardrails still have something to scan and
		// the user gets a structured metadata attachment instead of an error.
		const binaryStrings = extractBinaryOfficeStrings(bytes);
		const ext = name.split(".").pop() || "";
		const sizekb = (file.size / 1024).toFixed(1);
		const header = `[Binary File: ${file.name} | Type: ${ext.toUpperCase() || "unknown"} | Size: ${sizekb} KB]`;
		if (binaryStrings && binaryStrings.trim().length >= 8) {
			return `${header}\n\n--- Extracted Strings & Metadata ---\n${binaryStrings.slice(0, MAX_EXTRACT_CHARS - header.length - 50)}`;
		}
		// Nothing readable at all — still return a stub so callers don't show "No text extractor" error
		return `${header}\n\n[No readable text content found in binary data]`;
	} catch (error) {
		console.warn("Failed to extract text from attachment:", file.name, error);
		// Even on hard error, return a metadata stub so attachment doesn't silently disappear
		const sizekb = (file.size / 1024).toFixed(1);
		return `[File: ${file.name} | Size: ${sizekb} KB | Extraction error: ${error instanceof Error ? error.message : String(error)}]`;
	}

	return null;
}
