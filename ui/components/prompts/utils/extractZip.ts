/** Zip archive helpers for Prompt Repo file import. */

const MAX_ZIP_ENTRIES = 25;
const MAX_NESTED_ZIP_BYTES = 20 * 1024 * 1024;
const SKIP_NAME_RE = /(^|\/)(__MACOSX|\.DS_Store|Thumbs\.db)(\/|$)/i;

const INNER_EXT_MIME: Record<string, string> = {
	pdf: "application/pdf",
	txt: "text/plain",
	csv: "text/csv",
	json: "application/json",
	xml: "application/xml",
	md: "text/markdown",
	html: "text/html",
	htm: "text/html",
	doc: "application/msword",
	docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	xls: "application/vnd.ms-excel",
	xlsx: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	ppt: "application/vnd.ms-powerpoint",
	pptx: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	png: "image/png",
	jpg: "image/jpeg",
	jpeg: "image/jpeg",
	gif: "image/gif",
	webp: "image/webp",
	bmp: "image/bmp",
	mp3: "audio/mpeg",
	wav: "audio/wav",
	m4a: "audio/mp4",
	webm: "audio/webm",
	ogg: "audio/ogg",
};

const SUPPORTED_INNER_EXT = new Set(Object.keys(INNER_EXT_MIME));

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
			const ext = extOf(baseName(name));
			if (!ext || !SUPPORTED_INNER_EXT.has(ext)) return false;
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
