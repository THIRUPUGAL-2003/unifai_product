/** Zip archive helpers for Prompt Repo file import. */

const MAX_ZIP_ENTRIES = 50;
const MAX_NESTED_ZIP_BYTES = 20 * 1024 * 1024;
const SKIP_NAME_RE = /(^|\/)(__MACOSX|\.DS_Store|Thumbs\.db)(\/|$)/i;

const INNER_EXT_MIME: Record<string, string> = {
	// Documents
	pdf: "application/pdf",
	txt: "text/plain",
	csv: "text/csv",
	tsv: "text/tab-separated-values",
	json: "application/json",
	jsonl: "application/x-jsonlines",
	jsonc: "application/json",
	xml: "application/xml",
	md: "text/markdown",
	markdown: "text/markdown",
	rst: "text/x-rst",
	tex: "text/x-tex",
	html: "text/html",
	htm: "text/html",
	doc: "application/msword",
	docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	xls: "application/vnd.ms-excel",
	xlsx: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	ppt: "application/vnd.ms-powerpoint",
	pptx: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	odt: "application/vnd.oasis.opendocument.text",
	ods: "application/vnd.oasis.opendocument.spreadsheet",
	odp: "application/vnd.oasis.opendocument.presentation",
	rtf: "application/rtf",
	ipynb: "application/x-ipynb+json",
	// Code & Scripts
	py: "text/x-python",
	pyw: "text/x-python",
	js: "text/javascript",
	jsx: "text/javascript",
	mjs: "text/javascript",
	cjs: "text/javascript",
	ts: "text/typescript",
	tsx: "text/typescript",
	java: "text/x-java-source",
	go: "text/x-go",
	rs: "text/x-rust",
	c: "text/x-c",
	cpp: "text/x-c++",
	cc: "text/x-c++",
	cxx: "text/x-c++",
	h: "text/x-c",
	hpp: "text/x-c++",
	cs: "text/x-csharp",
	php: "text/x-php",
	rb: "text/x-ruby",
	swift: "text/x-swift",
	kt: "text/x-kotlin",
	kts: "text/x-kotlin",
	scala: "text/x-scala",
	r: "text/x-r",
	sql: "text/x-sql",
	sh: "text/x-shellscript",
	bash: "text/x-shellscript",
	zsh: "text/x-shellscript",
	ps1: "text/x-powershell",
	bat: "text/plain",
	cmd: "text/plain",
	yaml: "text/yaml",
	yml: "text/yaml",
	toml: "text/x-toml",
	ini: "text/plain",
	env: "text/plain",
	conf: "text/plain",
	config: "text/plain",
	properties: "text/plain",
	proto: "text/plain",
	graphql: "text/plain",
	gql: "text/plain",
	css: "text/css",
	scss: "text/x-scss",
	sass: "text/x-sass",
	less: "text/x-less",
	vue: "text/plain",
	svelte: "text/plain",
	dart: "text/plain",
	lua: "text/plain",
	dockerfile: "text/plain",
	makefile: "text/plain",
	gitignore: "text/plain",
	npmrc: "text/plain",
	diff: "text/plain",
	patch: "text/plain",
	log: "text/plain",
	// Images
	png: "image/png",
	jpg: "image/jpeg",
	jpeg: "image/jpeg",
	gif: "image/gif",
	webp: "image/webp",
	bmp: "image/bmp",
	svg: "image/svg+xml",
	tiff: "image/tiff",
	tif: "image/tiff",
	avif: "image/avif",
	heic: "image/heic",
	heif: "image/heif",
	ico: "image/x-icon",
	// Audio
	mp3: "audio/mpeg",
	wav: "audio/wav",
	m4a: "audio/mp4",
	webm: "audio/webm",
	ogg: "audio/ogg",
	flac: "audio/flac",
	aac: "audio/aac",
};

const SUPPORTED_INNER_EXT = new Set(Object.keys(INNER_EXT_MIME));
const KNOWN_STANDALONE_NAMES = new Set([
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

function baseName(path: string): string {
	const parts = path.replace(/\\/g, "/").split("/");
	return parts[parts.length - 1] || path;
}

function extOf(name: string): string {
	const m = name.toLowerCase().match(/\.([a-z0-9]+)$/);
	return m?.[1] || "";
}

/**
 * Unzips a .zip archive and returns inner files that Prompt Repo can import.
 * Skips nested zips, OS junk, and caps entry count.
 */
export async function extractZipInnerFiles(zipFile: File): Promise<{
	files: File[];
	skipped: number;
	errors: string[];
}> {
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const JSzipMod: any = await import("jszip");
	const JSZip = JSzipMod.default ?? JSzipMod;
	const buf = await zipFile.arrayBuffer();
	const zip = await JSZip.loadAsync(buf);

	const entries = Object.keys(zip.files)
		.filter((name) => {
			const entry = zip.files[name];
			if (!entry || entry.dir) return false;
			if (SKIP_NAME_RE.test(name)) return false;
			const bName = baseName(name);
			const ext = extOf(bName);
			const lowerName = bName.toLowerCase();
			if (!SUPPORTED_INNER_EXT.has(ext) && !KNOWN_STANDALONE_NAMES.has(lowerName)) return false;
			if (ext === "zip") return false; // no nested zip recursion
			return true;
		})
		.sort((a, b) => a.localeCompare(b));

	const files: File[] = [];
	const errors: string[] = [];
	let skipped = Math.max(0, entries.length - MAX_ZIP_ENTRIES);

	for (const name of entries.slice(0, MAX_ZIP_ENTRIES)) {
		try {
			const entry = zip.files[name];
			const blob: Blob = await entry.async("blob");
			if (blob.size <= 0) {
				skipped += 1;
				continue;
			}
			if (blob.size > MAX_NESTED_ZIP_BYTES) {
				errors.push(`${baseName(name)} too large`);
				skipped += 1;
				continue;
			}
			const fileName = baseName(name);
			const ext = extOf(fileName);
			const mime = INNER_EXT_MIME[ext] || "application/octet-stream";
			files.push(new File([blob], fileName, { type: mime }));
		} catch (err) {
			errors.push(`${baseName(name)}: ${err instanceof Error ? err.message : String(err)}`);
			skipped += 1;
		}
	}

	return { files, skipped, errors };
}
