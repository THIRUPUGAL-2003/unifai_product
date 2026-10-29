import { exportToExcel, exportToPdf, exportToWordDocx, triggerBlobDownload } from "./generateFiles";

export type StudioFileKind = "pdf" | "docx" | "xlsx" | "csv" | "html" | "json" | "md" | "txt" | "svg";

export type StudioEvent = {
	phase: "open" | "code" | "run" | "done" | "error" | "close";
	title?: string;
	language?: string;
	kind?: StudioFileKind;
	code?: string;
	previewMarkdown?: string;
	previewCsv?: string;
	previewHtml?: string;
	previewSvg?: string;
	previewUrl?: string;
	log?: string;
};

const KIND_EXT: Record<StudioFileKind, string> = {
	pdf: "pdf",
	docx: "docx",
	xlsx: "xlsx",
	csv: "csv",
	html: "html",
	json: "json",
	md: "md",
	txt: "txt",
	svg: "svg",
};

const WORD_TO_KIND: Record<string, StudioFileKind> = {
	pdf: "pdf",
	word: "docx",
	doc: "docx",
	docx: "docx",
	excel: "xlsx",
	xlsx: "xlsx",
	spreadsheet: "xlsx",
	csv: "csv",
	html: "html",
	json: "json",
	markdown: "md",
	md: "md",
	txt: "txt",
	text: "txt",
	svg: "svg",
};

const LAST_FILE_KEY = "prompt-repo:last-file";

export type LastFile = {
	kind: StudioFileKind;
	title: string;
	markdown: string;
	csv: string;
};

export function rememberFile(doc: LastFile): void {
	try {
		sessionStorage.setItem(LAST_FILE_KEY, JSON.stringify(doc));
	} catch {
		/* ignore quota */
	}
}

export function lastFile(): LastFile | null {
	try {
		const raw = sessionStorage.getItem(LAST_FILE_KEY);
		if (!raw) return null;
		const parsed = JSON.parse(raw) as LastFile;
		if (!parsed?.kind || !parsed.markdown) return null;
		return parsed;
	} catch {
		return null;
	}
}

export function emitStudio(detail: StudioEvent): void {
	if (typeof window === "undefined") return;
	window.dispatchEvent(new CustomEvent("prompt-repo:studio", { detail }));
}

/** User asked for a real file, not an explanation of how to make one. */
export function extractFileGeneration(text: string): { kind: StudioFileKind; instruction: string } | null {
	const t = (text || "").trim();
	if (!t || t.length > 8000) return null;
	if (/\b(how do i|how to|explain|what is|tutorial|example code|in python|in javascript|in react)\b/i.test(t)) return null;
	const verb = /\b(generate|create|make|write|export|build|compile|download|பண்ணு|உருவாக்கு|தயார்)\b/i.test(t);
	if (!verb) return null;
	const found = t.toLowerCase().match(/\b(pdf|word|docx|doc|excel|xlsx|spreadsheet|csv|html|json|markdown|md|txt|text|svg)\b/);
	if (!found) return null;
	const kind = WORD_TO_KIND[found[1]];
	if (!kind) return null;
	return { kind, instruction: t };
}

/** A follow-up edit ("modify this", "alter the excel") updates the last file instead of starting a new chat answer. */
export function extractFileRequest(text: string): { kind: StudioFileKind; instruction: string; revise: boolean } | null {
	const direct = extractFileGeneration(text);
	if (direct) return { ...direct, revise: /\b(alter|modify|update|change|revise|edit|மாற்று|திருத்து)\b/i.test(text) && !!lastFile() };
	const prev = lastFile();
	if (!prev) return null;
	if (!/\b(alter|modify|update|change|revise|edit|மாற்று|திருத்து)\b/i.test(text || "")) return null;
	return { kind: prev.kind, instruction: text.trim(), revise: true };
}

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

function isAbortError(error: unknown): boolean {
	return Boolean(error && typeof error === "object" && "name" in error && error.name === "AbortError");
}

function fromJsonObject(raw: string): { title: string; body: string; csv: string; svg: string } | null {
	const start = raw.indexOf("{");
	const end = raw.lastIndexOf("}");
	if (start < 0 || end <= start) return null;
	try {
		const obj = JSON.parse(raw.slice(start, end + 1)) as Record<string, unknown>;
		const body = String(obj.content || obj.markdown || obj.CONTENT || "").trim();
		if (!body && !obj.csv && !obj.svg) return null;
		return {
			title: String(obj.title || obj.TITLE || "").trim(),
			body,
			csv: String(obj.csv || obj.CSV || "").trim(),
			svg: String(obj.svg || obj.SVG || "").trim(),
		};
	} catch {
		return null;
	}
}

/** Accept JSON, Python, or plain text so a smaller model can still produce the file. */
function documentFromModel(raw: string): { title: string; body: string; csv: string; svg: string } {
	const json = fromJsonObject(raw);
	if (json?.body || json?.csv || json?.svg) return json;
	const python = contentFromModelCode(raw);
	if (python.body || python.csv || python.svg) return python;
	const fenced = raw.match(/```(?:markdown|md|text)?\s*([\s\S]*?)```/);
	const text = (fenced?.[1] || raw)
		.replace(/^```[\w-]*\s*/, "")
		.replace(/```$/, "")
		.trim();
	if (text.length > 40) return { title: "", body: text, csv: "", svg: "" };
	return { title: "", body: "", csv: "", svg: "" };
}

/** Pull the document body out of the Python the model wrote. */
function contentFromModelCode(code: string): { title: string; body: string; csv: string; svg: string } {
	const titleMatch = code.match(/TITLE\s*=\s*["']([^"'\n]{1,80})["']/);
	const block = code.match(/CONTENT\s*=\s*(?:"""|''')([\s\S]*?)(?:"""|''')/);
	const csv = code.match(/CSV\s*=\s*(?:"""|''')([\s\S]*?)(?:"""|''')/);
	const svg = code.match(/SVG\s*=\s*(?:"""|''')([\s\S]*?)(?:"""|''')/);
	return {
		title: titleMatch?.[1]?.trim() || "",
		body: block?.[1]?.trim() || "",
		csv: csv?.[1]?.trim() || "",
		svg: svg?.[1]?.trim() || "",
	};
}

function safeSvg(svg: string): string | null {
	const trimmed = svg.trim();
	if (!/^<svg\b[\s\S]*<\/svg>$/i.test(trimmed)) return null;
	if (
		/<script\b|<foreignObject\b|<(?:iframe|object|embed|audio|video)\b|\bon[a-z][\w:-]*\s*=|javascript:|vbscript:|data:text\/html/i.test(
			trimmed,
		)
	) {
		return null;
	}
	return trimmed;
}

async function saveDocument(kind: StudioFileKind, title: string, plan: { markdown?: string; csv?: string; svg?: string }): Promise<string> {
	const markdown = (plan.markdown || title).trim();
	const extension = `.${KIND_EXT[kind]}`;
	const normalizedTitle = title.replace(/[\\/:*?"<>|]+/g, "_").trim();
	const baseTitle = normalizedTitle.toLowerCase().endsWith(extension) ? normalizedTitle.slice(0, -extension.length) : normalizedTitle;
	const safeTitle = baseTitle.slice(0, 50) || "file";
	if (kind === "pdf") {
		if (!(await exportToPdf(markdown, safeTitle))) throw new Error("The PDF could not be generated. Please try again.");
		return `${safeTitle}.pdf`;
	}
	if (kind === "docx") {
		if (!(await exportToWordDocx(markdown, safeTitle))) throw new Error("The Word document could not be generated. Please try again.");
		return `${safeTitle}.docx`;
	}
	if (kind === "xlsx") {
		if (!(await exportToExcel(plan.csv || markdown, safeTitle)))
			throw new Error("The Excel file could not be generated. Please try again.");
		return `${safeTitle}.xlsx`;
	}
	const filename = `${safeTitle}.${KIND_EXT[kind]}`;
	if (kind === "svg") {
		const svg = safeSvg(plan.svg || "");
		if (!svg) throw new Error("The model did not return a valid SVG.");
		triggerBlobDownload(new Blob([svg], { type: "image/svg+xml" }), filename);
		return filename;
	}
	const body = kind === "csv" ? plan.csv || markdown : kind === "json" ? plan.markdown || "{}" : markdown;
	const type = kind === "html" ? "text/html" : kind === "json" ? "application/json" : kind === "csv" ? "text/csv" : "text/plain";
	triggerBlobDownload(new Blob([body], { type: `${type};charset=utf-8` }), filename);
	return filename;
}

export async function runDocumentStudio(opts: {
	kind: StudioFileKind;
	instruction: string;
	context: string;
	revise?: boolean;
	previous?: LastFile | null;
	askModel: (system: string, user: string) => Promise<string>;
}): Promise<string> {
	const titleGuess = opts.instruction.replace(/\s+/g, " ").slice(0, 48) || "Document";
	emitStudio({
		phase: "open",
		title: `Writing ${KIND_EXT[opts.kind]}`,
		kind: opts.kind,
		language: "json",
		code: "",
		log: "Preparing the file content…",
	});

	const revising = Boolean(opts.revise && opts.previous?.markdown);
	const system = [
		"Return one JSON object and nothing else.",
		'{"title":"short file name","content":"the full document in markdown","csv":"comma separated rows, only for a spreadsheet","svg":""}',
		revising
			? "This is an edit. content must be the complete document after the change. Keep every part the user did not ask to change."
			: "content must be the finished document, with headings and a markdown table when the data is tabular.",
		"Do not describe the file. Do not use asterisks around words.",
	].join(" ");
	const user = [
		`File type: ${opts.kind}`,
		`Request: ${opts.instruction}`,
		revising ? `Current title: ${opts.previous?.title || ""}` : "",
		revising ? `Current document:\n${opts.previous?.markdown || ""}` : "",
		revising && opts.previous?.csv ? `Current spreadsheet:\n${opts.previous.csv}` : "",
		`Conversation:\n${opts.context || "(none)"}`,
	]
		.filter(Boolean)
		.join("\n\n");

	let code = "";
	let pulled = { title: "", body: "", csv: "", svg: "" };
	let generationError: unknown;
	try {
		code = (await opts.askModel(system, user)).trim();
		pulled = documentFromModel(code);
		if (!pulled.body && !pulled.csv) {
			code = (await opts.askModel("Return only the JSON object with title and content. content is the full document.", user)).trim();
			pulled = documentFromModel(code);
		}
	} catch (error) {
		if (isAbortError(error)) throw error;
		generationError = error;
		code = "";
	}
	if (!pulled.body && !pulled.csv && !pulled.svg && revising && opts.previous) {
		pulled = { title: opts.previous.title, body: opts.previous.markdown, csv: opts.previous.csv, svg: "" };
	}
	if (!pulled.body && !pulled.csv && !pulled.svg) {
		if (generationError) {
			const reason = generationError instanceof Error ? generationError.message : String(generationError);
			throw new Error(`The file content could not be generated: ${reason}`);
		}
		throw new Error("The model did not return usable file content. Please try again or make the request more specific.");
	}
	if (!code) code = JSON.stringify({ title: pulled.title || titleGuess, content: pulled.body, csv: pulled.csv, svg: pulled.svg }, null, 2);

	let shown = "";
	const lines = code.split("\n");
	for (let i = 0; i < lines.length; i++) {
		shown += (shown ? "\n" : "") + lines[i];
		if (i % 4 === 0 || i === lines.length - 1) {
			emitStudio({ phase: "code", language: "json", code: shown, log: "Writing file content…" });
			await sleep(40);
		}
	}

	const plan = {
		title: pulled.title || titleGuess,
		markdown: pulled.body || opts.instruction,
		csv: pulled.csv,
		svg: pulled.svg,
	};
	emitStudio({
		phase: "run",
		title: plan.title,
		kind: opts.kind,
		code,
		previewMarkdown: plan.markdown,
		previewCsv: plan.csv,
		previewHtml: opts.kind === "html" ? plan.markdown : undefined,
		previewSvg: opts.kind === "svg" ? plan.svg : undefined,
		log: revising ? "Updating the file…" : "Generating the file…",
	});
	const filename = await saveDocument(opts.kind, plan.title || "Document", plan);
	rememberFile({
		kind: opts.kind,
		title: plan.title || "Document",
		markdown: plan.markdown,
		csv: plan.csv,
	});
	emitStudio({ phase: "done", title: filename, code, log: `Saved ${filename}` });
	return filename;
}
