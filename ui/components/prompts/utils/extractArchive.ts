/** Unpack zip, tar, gzip, 7z, and rar archives for Prompt Repo import. */

import { extractZipInnerFiles } from "./extractZip";

const MAX_ARCHIVE_ENTRIES = 1000;
const SKIP_NAME_RE = /(^|\/)(__MACOSX|\.DS_Store|Thumbs\.db)(\/|$)/i;

function baseName(path: string): string {
	const parts = path.replace(/\\/g, "/").split("/");
	return parts[parts.length - 1] || path;
}

function isZipName(name: string): boolean {
	return name.toLowerCase().endsWith(".zip");
}

type ArchiveCtor = {
	init: (opts: { workerUrl: string }) => void;
	open: (file: File) => Promise<{ extractFiles: () => Promise<unknown> }>;
};

let archiveLoader: Promise<ArchiveCtor> | null = null;

async function loadArchiveReader(): Promise<ArchiveCtor> {
	if (!archiveLoader) {
		archiveLoader = (async () => {
			const workerMod = await import("libarchive.js/dist/worker-bundle.js?raw");
			const wasmMod = await import("libarchive.js/dist/libarchive.wasm?url");
			const wasmUrl = new URL(wasmMod.default, window.location.href).toString();
			const source = String(workerMod.default).replaceAll(
				'new URL("libarchive.wasm",import.meta.url).href',
				JSON.stringify(wasmUrl),
			);
			if (source === String(workerMod.default)) {
				throw new Error("Archive reader could not locate its decoder");
			}
			const workerUrl = URL.createObjectURL(new Blob([source], { type: "text/javascript" }));
			const mod = (await import("libarchive.js")) as { Archive: ArchiveCtor };
			mod.Archive.init({ workerUrl });
			return mod.Archive;
		})();
	}
	return archiveLoader;
}

function flattenExtracted(node: unknown, out: File[]) {
	if (!node || typeof node !== "object") return;
	if (typeof File !== "undefined" && node instanceof File) {
		const name = node.name || "file";
		if (!baseName(name) || SKIP_NAME_RE.test(name)) return;
		out.push(node);
		return;
	}
	for (const value of Object.values(node as Record<string, unknown>)) {
		flattenExtracted(value, out);
	}
}

async function extractWithLibarchive(file: File): Promise<{ files: File[]; skipped: number; errors: string[] }> {
	const Archive = await loadArchiveReader();
	const opened = await Archive.open(file);
	const tree = await opened.extractFiles();
	const flat: File[] = [];
	flattenExtracted(tree, flat);
	const files: File[] = [];
	const errors: string[] = [];
	let skipped = 0;
	for (const inner of flat) {
		if (files.length >= MAX_ARCHIVE_ENTRIES) {
			skipped += 1;
			continue;
		}
		if (inner.size <= 0) {
			skipped += 1;
			continue;
		}
		files.push(inner);
	}
	return { files, skipped, errors };
}

export function isPromptArchive(file: File, mimeType: string): boolean {
	const name = file.name.toLowerCase();
	const mime = (mimeType || "").toLowerCase();
	return (
		name.endsWith(".zip") ||
		name.endsWith(".tar") ||
		name.endsWith(".tar.gz") ||
		name.endsWith(".tgz") ||
		name.endsWith(".gz") ||
		name.endsWith(".gzip") ||
		name.endsWith(".bz2") ||
		name.endsWith(".xz") ||
		name.endsWith(".7z") ||
		name.endsWith(".rar") ||
		name.endsWith(".tbz2") ||
		name.endsWith(".txz") ||
		name.endsWith(".cab") ||
		name.endsWith(".iso") ||
		name.endsWith(".jar") ||
		name.endsWith(".war") ||
		name.endsWith(".apk") ||
		name.endsWith(".ear") ||
		name.endsWith(".zst") ||
		name.endsWith(".tzst") ||
		mime === "application/zip" ||
		mime === "application/x-zip-compressed" ||
		mime.includes("gzip") ||
		mime.includes("x-tar") ||
		mime.includes("x-7z") ||
		mime.includes("rar") ||
		mime.includes("bzip") ||
		mime.includes("x-xz") ||
		mime.includes("java-archive") ||
		mime.includes("vnd.android.package")
	);
}

/** Inner files from a zip (JSZip) or from tar/gzip/7z/rar (libarchive). */
export async function extractArchiveInnerFiles(file: File, detectedMimeType?: string): Promise<{
	files: File[];
	skipped: number;
	errors: string[];
}> {
	const lower = file.name.toLowerCase();
	const mime = (detectedMimeType || file.type || "").toLowerCase();
	const zipPackage = /\.(zip|jar|war|apk|ear|whl|nupkg)$/i.test(lower);
	if (zipPackage || mime === "application/zip" || mime === "application/x-zip-compressed" || mime.includes("java-archive")) {
		return extractZipInnerFiles(file);
	}
	return extractWithLibarchive(file);
}
