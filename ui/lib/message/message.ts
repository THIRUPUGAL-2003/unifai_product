import { v4 as uuidv4 } from "uuid";
import {
	APIMessage,
	CompletionRequest,
	CompletionResult,
	CompletionUsage,
	MessageContent,
	MessageError,
	MessageRole,
	MessageType,
	SerializedMessage,
	ToolCall,
	ToolResult,
} from "./types";

/** Extracted file / image / voice transcript text parts live in content[] but act as attachments. */
function isEmbeddedAttachmentText(part: MessageContent): boolean {
	if (part.type !== "text" || !part.text) return false;
	return (
		part.text.startsWith("Attached file:") ||
		part.text.startsWith("Attached zip:") ||
		part.text.startsWith("Attached image:") ||
		part.text.startsWith("Voice transcript") ||
		part.text.includes("--- extracted content ---") ||
		part.text.includes("--- OCR extracted content ---")
	);
}

interface ExtractedFileInfo {
	filename: string;
	language?: string;
	category: string;
	content: string;
	kind: "extracted" | "ocr" | "voice" | "zip" | "raw";
}

function detectLanguageFromExt(ext: string): string | undefined {
	const map: Record<string, string> = {
		ts: "typescript",
		tsx: "typescript",
		mts: "typescript",
		cts: "typescript",
		js: "javascript",
		jsx: "javascript",
		mjs: "javascript",
		cjs: "javascript",
		py: "python",
		pyw: "python",
		pyi: "python",
		pyx: "python",
		json: "json",
		jsonl: "json",
		jsonc: "json",
		ndjson: "json",
		csv: "csv",
		tsv: "tsv",
		sql: "sql",
		html: "html",
		htm: "html",
		xml: "xml",
		svg: "xml",
		yaml: "yaml",
		yml: "yaml",
		css: "css",
		scss: "scss",
		sass: "sass",
		less: "less",
		sh: "bash",
		bash: "bash",
		zsh: "bash",
		fish: "bash",
		ps1: "powershell",
		psm1: "powershell",
		psd1: "powershell",
		bat: "bat",
		cmd: "bat",
		go: "go",
		rs: "rust",
		rlib: "rust",
		java: "java",
		c: "c",
		h: "c",
		cpp: "cpp",
		hpp: "cpp",
		cc: "cpp",
		cxx: "cpp",
		hh: "cpp",
		hxx: "cpp",
		cs: "csharp",
		csx: "csharp",
		fs: "fsharp",
		fsi: "fsharp",
		fsx: "fsharp",
		php: "php",
		phtml: "php",
		rb: "ruby",
		rake: "ruby",
		gemspec: "ruby",
		swift: "swift",
		kt: "kotlin",
		kts: "kotlin",
		scala: "scala",
		sc: "scala",
		r: "r",
		rmd: "r",
		m: "matlab",
		mlx: "matlab",
		jl: "julia",
		dart: "dart",
		lua: "lua",
		pl: "perl",
		pm: "perl",
		t: "perl",
		asm: "assembly",
		s: "assembly",
		f: "fortran",
		for: "fortran",
		f90: "fortran",
		f95: "fortran",
		cbl: "cobol",
		cob: "cobol",
		cpy: "cobol",
		mm: "objectivec",
		ex: "elixir",
		exs: "elixir",
		erl: "erlang",
		hrl: "erlang",
		hs: "haskell",
		lhs: "haskell",
		clj: "clojure",
		cljs: "clojure",
		cljc: "clojure",
		edn: "clojure",
		lisp: "lisp",
		lsp: "lisp",
		pro: "prolog",
		sol: "solidity",
		graphql: "graphql",
		gql: "graphql",
		proto: "protobuf",
		wat: "wat",
		vue: "vue",
		svelte: "svelte",
		md: "markdown",
		markdown: "markdown",
		mdown: "markdown",
		rst: "rst",
		tex: "latex",
		latex: "latex",
		toml: "toml",
		ini: "ini",
		cfg: "ini",
		conf: "ini",
		config: "ini",
		env: "shell",
		dockerfile: "dockerfile",
		makefile: "makefile",
		cmake: "cmake",
		gradle: "groovy",
		pom: "xml",
		sln: "text",
		csproj: "xml",
		tf: "hcl",
		tfvars: "hcl",
		j2: "jinja2",
		jinja: "jinja2",
		jinja2: "jinja2",
		diff: "diff",
		patch: "diff",
		log: "log",
		hlsl: "hlsl",
		glsl: "glsl",
		shader: "glsl",
		gcode: "gcode",
		scad: "scad",
		bib: "bibtex",
		ris: "text",
		qmd: "markdown",
		fasta: "fasta",
		fa: "fasta",
		fastq: "fastq",
		sam: "sam",
		edi: "text",
		x12: "text",
		po: "gettext",
		pot: "gettext",
		strings: "text",
	};
	return map[ext.toLowerCase()];
}

function getCategoryFromExt(ext: string): string {
	const lower = ext.toLowerCase();
	if (["pdf", "epub", "mobi", "azw", "azw3", "fb2", "djvu", "xps", "oxps"].includes(lower)) return "PDF / E-Book Document";
	if (["doc", "docx", "dot", "dotx", "dotm", "odt", "rtf", "pages"].includes(lower)) return "Word Document";
	if (["ppt", "pptx", "pptm", "pot", "potx", "potm", "odp", "key"].includes(lower)) return "Presentation Slides";
	if (["xls", "xlsx", "xlsm", "xlsb", "xlt", "xltx", "xltm", "csv", "tsv", "ods", "numbers"].includes(lower)) return "Spreadsheet / Data Table";
	if (["png", "jpg", "jpeg", "jpe", "webp", "gif", "bmp", "dib", "svg", "tiff", "tif", "avif", "heic", "heif", "ico", "cur", "psd", "psb", "ai", "raw", "dng", "cr2", "cr3", "nef", "hdr", "exr"].includes(lower)) return "Image / Visual Asset";
	if (["mp3", "wav", "aac", "m4a", "flac", "ogg", "opus", "wma", "aiff", "aif", "mid", "midi", "amr", "weba", "pcm"].includes(lower)) return "Audio / Voice Recording";
	if (["mp4", "m4v", "mpeg", "mpg", "mov", "avi", "mkv", "webm", "wmv", "flv", "mts", "m2ts"].includes(lower)) return "Video Recording";
	if (["zip", "tar", "gz", "tgz", "bz2", "tbz2", "xz", "txz", "zst", "7z", "rar", "iso", "dmg", "cab", "pkg", "deb", "rpm", "apk"].includes(lower)) return "Archive Package";
	if (["py", "pyw", "pyi", "js", "ts", "tsx", "jsx", "go", "java", "rs", "cpp", "c", "cs", "php", "rb", "swift", "kt", "scala", "dart", "lua", "r", "jl", "pl", "asm", "f", "cob", "ex", "erl", "hs", "clj", "sol", "sql", "sh", "bash", "ps1", "bat", "cmd", "proto", "graphql", "wat", "vue", "svelte", "html", "css"].includes(lower)) return "Source Code";
	if (["json", "jsonl", "ndjson", "yaml", "yml", "xml", "toml", "ini", "cfg", "conf", "config", "env", "properties", "tf", "tfvars", "cmake", "gradle", "dockerfile", "makefile", "editorconfig"].includes(lower)) return "Structured Config / DevOps";
	if (["pt", "pth", "onnx", "safetensors", "gguf", "ggml", "pkl", "pickle", "joblib", "tflite", "keras", "weights", "ckpt", "model", "cbm"].includes(lower)) return "AI / ML Model Artifact";
	if (["sqlite", "sqlite3", "db", "duckdb", "parquet", "orc", "avro", "arrow", "feather", "h5", "hdf5", "dump", "bak"].includes(lower)) return "Database / Analytics Storage";
	if (["pem", "crt", "cer", "cert", "csr", "key", "pub", "asc", "sig", "sha256", "sha512", "md5", "spdx"].includes(lower)) return "Security / Cryptographic Certificate or Key";
	if (["obj", "stl", "gcode", "scad", "dxf", "step", "stp", "iges", "igs", "fbx", "gltf", "glb", "kicad_sch", "kicad_pcb", "gbr"].includes(lower)) return "CAD / 3D Model Specification";
	if (["geojson", "kml", "kmz", "gpx", "topojson", "shp", "tab"].includes(lower)) return "Geospatial / GIS Map Data";
	if (["bib", "ris", "rdf", "fasta", "fa", "fastq", "fq", "sam", "pdb", "cif", "qmd"].includes(lower)) return "Scientific / Research Data";
	if (["eml", "msg", "vcf", "ics", "ical"].includes(lower)) return "Email / Calendar / Contact Record";
	if (["exe", "dll", "so", "dylib", "wasm", "bin", "dex", "elf"].includes(lower)) return "Binary Executable / Module";
	return "Text Document";
}

function parseAttachmentPart(part: MessageContent): ExtractedFileInfo | null {
	if (part.type !== "text" || !part.text) return null;
	const text = part.text.trim();

	// Check OCR
	if (text.includes("--- OCR extracted content ---")) {
		const lines = text.split("\n");
		const fileLine = lines.find((l) => l.startsWith("Attached file:")) || "";
		const filename = fileLine.replace(/^Attached file:\s*/i, "").trim() || "image_ocr";
		const splitIdx = text.indexOf("--- OCR extracted content ---");
		const content = text.slice(splitIdx + "--- OCR extracted content ---".length).trim();
		return { filename, category: "Image OCR", content, kind: "ocr" };
	}

	// Check normal extracted file
	if (text.startsWith("Attached file:") && text.includes("--- extracted content ---")) {
		const splitIdx = text.indexOf("--- extracted content ---");
		const header = text.slice(0, splitIdx).trim();
		const filename = header.replace(/^Attached file:\s*/i, "").trim() || "file";
		const content = text.slice(splitIdx + "--- extracted content ---".length).trim();
		const ext = filename.split(".").pop()?.toLowerCase() || "";
		const lang = detectLanguageFromExt(ext);
		const category = getCategoryFromExt(ext);
		return { filename, language: lang, category, content, kind: "extracted" };
	}

	// Check Voice transcript
	if (text.startsWith("Voice transcript") || text.includes("voice recording attached") || text.includes("[Voice Audio:")) {
		const match = text.match(/^Voice transcript\s*(?:\(([^)]+)\))?:\s*([\s\S]*)$/i);
		const filename = match?.[1] || "voice_recording.wav";
		const content = (match?.[2] || text).trim();
		return { filename, category: "Voice Recording", content, kind: "voice" };
	}

	// Check Attached zip
	if (text.startsWith("Attached zip:")) {
		const match = text.match(/^Attached zip:\s*([^\n]+)\n\n([\s\S]*)$/i);
		const filename = match?.[1]?.trim() || "archive.zip";
		const content = (match?.[2] || "").trim();
		return { filename, category: "Zip Archive", content, kind: "zip" };
	}

	// Generic attached text fallback
	if (text.startsWith("Attached file:") || text.startsWith("Attached image:") || text.startsWith("[File:")) {
		const lines = text.split("\n");
		const first = lines[0];
		let filename = "attachment";
		const m = first.match(/^(?:Attached file|Attached image|\[File):\s*([^()\n]+)/i);
		if (m?.[1]) filename = m[1].trim();
		const content = lines.slice(1).join("\n").trim() || text;
		const ext = filename.split(".").pop()?.toLowerCase() || "";
		const lang = detectLanguageFromExt(ext);
		const category = getCategoryFromExt(ext);
		return { filename, language: lang, category, content, kind: "raw" };
	}

	return null;
}

function formatUnifiedMultiFileContent(
	userQuery: string,
	files: ExtractedFileInfo[],
	images: { filename?: string }[]
): string {
	const sections: string[] = [];
	const totalItems = files.length + images.length;

	// 1. User Instruction
	const trimmedQuery = userQuery.trim();
	if (trimmedQuery) {
		sections.push(
			`[USER QUESTION / INSTRUCTION]\n${trimmedQuery}\n\nPlease thoroughly analyze all the attached files, extracted content, and voice transcripts provided below to answer this question completely and accurately.`
		);
	} else if (totalItems > 1) {
		sections.push(
			"Please thoroughly review and analyze all the attached files and voice recordings below. Provide a clear summary of each item, compare or cross-reference their key details, and highlight any notable patterns, findings, or discrepancies."
		);
	} else if (totalItems === 1) {
		sections.push(
			"Please thoroughly review and analyze the attached file or voice recording below. Provide a comprehensive summary, explain its key points, and highlight any significant findings or insights."
		);
	}

	// 2. Multi-item Manifest
	if (totalItems > 1 || (totalItems === 1 && files.length > 0)) {
		const manifestLines: string[] = [];
		manifestLines.push(`[ATTACHED ITEMS OVERVIEW - ${totalItems} item(s) provided for analysis]`);
		let idx = 1;
		for (const f of files) {
			manifestLines.push(`• Item ${idx}: "${f.filename}" (${f.category})`);
			idx++;
		}
		for (const img of images) {
			manifestLines.push(`• Item ${idx}: "${img.filename || "Image"}" (Image - visual content attached below)`);
			idx++;
		}
		sections.push(manifestLines.join("\n"));
	}

	// 3. Document / Code Content Blocks
	let fileIdx = 1;
	for (const f of files) {
		const fence = f.language ? "```" + f.language + "\n" : "";
		const closeFence = f.language ? "\n```" : "";
		sections.push(
`================================================================================
<<< FILE ${fileIdx} OF ${files.length}: "${f.filename}" (${f.category}) >>>
================================================================================
${fence}${f.content}${closeFence}
================================================================================
<<< END OF FILE ${fileIdx}: "${f.filename}" >>>
================================================================================`
		);
		fileIdx++;
	}

	// 4. Image Visual Reference
	if (images.length > 0) {
		const imgLines: string[] = [];
		imgLines.push(`[ATTACHED IMAGES FOR VISUAL ANALYSIS]`);
		imgLines.push(`The following ${images.length} visual image(s) are attached in order:`);
		images.forEach((img, i) => {
			imgLines.push(`  - Image ${i + 1}: "${img.filename || `Image ${i + 1}`}"`);
		});
		imgLines.push(`Please analyze these images in combination with the prompt and any text files above.`);
		sections.push(imgLines.join("\n"));
	}

	return sections.join("\n\n");
}

export class Message {
	readonly id: string;
	readonly index: number;
	private readonly originalType: MessageType;
	private currentType: MessageType;
	private _payload?: CompletionRequest | CompletionResult | ToolResult;
	readonly error?: MessageError;

	constructor(
		id: string,
		index: number,
		type: MessageType,
		payload?: CompletionRequest | CompletionResult | ToolResult,
		error?: MessageError,
	) {
		this.id = id;
		this.index = index;
		this.originalType = type;
		this.currentType = type;
		this._payload = payload;
		this.error = error;
	}

	// Convenience factory methods

	static system(content: string, index = 0): Message {
		return new Message(uuidv4(), index, MessageType.CompletionRequest, {
			role: MessageRole.SYSTEM,
			content,
		} as CompletionRequest);
	}

	static request(content: string, index = 0, attachments?: MessageContent[]): Message {
		if (attachments && attachments.length > 0) {
			const parts: MessageContent[] = [{ type: "text", text: content }, ...attachments];
			return new Message(uuidv4(), index, MessageType.CompletionRequest, {
				role: MessageRole.USER,
				content: parts,
			} as CompletionRequest);
		}
		return new Message(uuidv4(), index, MessageType.CompletionRequest, {
			role: MessageRole.USER,
			content,
		} as CompletionRequest);
	}

	static response(content: string, index = 0, usage?: CompletionUsage): Message {
		return new Message(uuidv4(), index, MessageType.CompletionResult, {
			id: uuidv4(),
			choices: [{ index: 0, message: { role: MessageRole.ASSISTANT, content } }],
			usage,
		} as CompletionResult);
	}

	static toolCallResponse(content: string, toolCalls: ToolCall[], index = 0, usage?: CompletionUsage): Message {
		return new Message(uuidv4(), index, MessageType.CompletionResult, {
			id: uuidv4(),
			choices: [{ index: 0, message: { role: MessageRole.ASSISTANT, content, tool_calls: toolCalls }, finish_reason: "tool_calls" }],
			usage,
		} as CompletionResult);
	}

	static error(content: string, index = 0): Message {
		return new Message(uuidv4(), index, MessageType.CompletionError, undefined, {
			code: "error",
			message: content,
		});
	}

	public get payload() {
		return this._payload;
	}

	public get type(): MessageType {
		return this.currentType;
	}

	// Serialization

	public get serialized(): SerializedMessage {
		const s: SerializedMessage = {
			id: this.id,
			index: this.index,
			originalType: this.originalType,
			currentType: this.currentType,
			payload: this._payload ? JSON.parse(JSON.stringify(this._payload)) : undefined,
		};
		if (this.error) {
			s.error = this.error;
		}
		return s;
	}

	public static deserialize(serialized: SerializedMessage): Message {
		const message = new Message(
			serialized.id,
			serialized.index,
			serialized.originalType,
			serialized.payload ? JSON.parse(JSON.stringify(serialized.payload)) : undefined,
			serialized.error,
		);
		message.currentType = serialized.currentType;
		return message;
	}

	public withIndex(index: number): Message {
		if (this.index === index) return this;
		const m = new Message(this.id, index, this.originalType, this._payload, this.error);
		m.currentType = this.currentType;
		return m;
	}

	public clone(): Message {
		return Message.deserialize(this.serialized);
	}

	// Role

	public get role(): MessageRole | undefined {
		switch (this.originalType) {
			case MessageType.CompletionRequest:
				return (this._payload as CompletionRequest)?.role;
			case MessageType.CompletionResult:
				return (this._payload as CompletionResult)?.choices?.[0]?.message?.role;
			case MessageType.CompletionError:
				return MessageRole.ASSISTANT;
			case MessageType.ToolResult:
				return (this._payload as ToolResult)?.role ?? MessageRole.TOOL;
			default:
				return undefined;
		}
	}

	public set role(role: MessageRole) {
		switch (this.originalType) {
			case MessageType.CompletionRequest:
				(this._payload as CompletionRequest).role = role;
				break;
			case MessageType.CompletionResult:
				(this._payload as CompletionResult).choices?.forEach((choice) => {
					choice.message.role = role;
				});
				break;
			case MessageType.ToolResult:
				(this._payload as any).role = role;
				break;
		}
		switch (role) {
			case MessageRole.ASSISTANT:
				this.currentType = MessageType.CompletionResult;
				break;
			case MessageRole.USER:
			case MessageRole.SYSTEM:
			case MessageRole.DEVELOPER:
				this.currentType = MessageType.CompletionRequest;
				break;
			case MessageRole.TOOL:
				this.currentType = MessageType.ToolResult;
				break;
			default:
				this.currentType = MessageType.CompletionResult;
				break;
		}
	}

	// Content

	public get content(): string {
		switch (this.originalType) {
			case MessageType.CompletionRequest: {
				const payload = this._payload as CompletionRequest;
				if (!payload?.content) return "";
				if (typeof payload.content === "string") return payload.content;
				// User-typed text only — file/voice extracts are treated as attachments.
				return payload.content
					.filter((c) => c.type === "text" && !isEmbeddedAttachmentText(c))
					.map((c) => c.text || "")
					.join("\n\n");
			}
			case MessageType.CompletionResult:
				return (this._payload as CompletionResult)?.choices?.[0]?.message?.content ?? "";
			case MessageType.ToolResult:
				return (this._payload as ToolResult)?.content ?? "";
			default:
				return this.error?.message || "";
		}
	}

	public set content(content: string) {
		switch (this.originalType) {
			case MessageType.CompletionRequest: {
				const payload = this._payload as CompletionRequest;
				if (typeof payload.content === "string" || payload.content === null) {
					this._payload = { ...payload, content } as CompletionRequest;
				} else if (Array.isArray(payload.content)) {
					const updated = JSON.parse(JSON.stringify(payload.content)) as MessageContent[];
					const userIdx = updated.findIndex((c) => c.type === "text" && !isEmbeddedAttachmentText(c));
					if (userIdx >= 0) {
						updated[userIdx] = { ...updated[userIdx], text: content };
					} else {
						updated.unshift({ type: "text", text: content });
					}
					this._payload = { ...payload, content: updated } as CompletionRequest;
				}
				break;
			}
			case MessageType.ToolResult:
				this._payload = { ...this._payload, content } as ToolResult;
				break;
			case MessageType.CompletionResult: {
				const result = this._payload as CompletionResult;
				const choices = result?.choices?.map((c) => ({ ...c })) ?? [];
				if (choices[0]) {
					choices[0].message = { ...choices[0].message, content };
				}
				this._payload = { ...result, choices } as CompletionResult;
				break;
			}
		}
	}

	// Attachments (non-user-text content parts, including extracted file / voice transcript text)

	public get attachments(): MessageContent[] {
		if (this.originalType !== MessageType.CompletionRequest) return [];
		const payload = this._payload as CompletionRequest;
		if (!Array.isArray(payload?.content)) return [];
		return payload.content.filter((c) => c.type !== "text" || isEmbeddedAttachmentText(c));
	}

	public set attachments(parts: MessageContent[]) {
		if (this.originalType !== MessageType.CompletionRequest) return;
		const payload = this._payload as CompletionRequest;
		const text = this.content;
		if (parts.length === 0) {
			this._payload = { ...payload, content: text } as CompletionRequest;
		} else {
			this._payload = {
				...payload,
				content: [{ type: "text", text } as MessageContent, ...parts],
			} as CompletionRequest;
		}
	}

	// Tool calls

	public get toolCalls(): ToolCall[] | undefined {
		if (this.originalType === MessageType.CompletionResult) {
			const calls = (this._payload as CompletionResult)?.choices?.map((c) => c.message.tool_calls ?? []).flat();
			return calls && calls.length > 0 ? calls : undefined;
		}
		if (this.originalType === MessageType.CompletionRequest) {
			const calls = (this._payload as CompletionRequest)?.tool_calls;
			return calls && calls.length > 0 ? calls : undefined;
		}
		return undefined;
	}

	public get toolCallId(): string | undefined {
		if (this.originalType === MessageType.ToolResult || this.currentType === MessageType.ToolResult) {
			return (this._payload as ToolResult)?.tool_call_id;
		}
		if (this.originalType === MessageType.CompletionRequest) {
			return (this._payload as CompletionRequest)?.tool_call_id;
		}
		return undefined;
	}

	public set toolCallId(id: string) {
		if (this.originalType === MessageType.ToolResult) {
			(this._payload as ToolResult).tool_call_id = id;
		} else if (this.originalType === MessageType.CompletionRequest) {
			(this._payload as CompletionRequest).tool_call_id = id;
		}
	}

	public get isToolCallError(): boolean | undefined {
		if (this.originalType === MessageType.ToolResult || this.currentType === MessageType.ToolResult) {
			return !!(this._payload as ToolResult)?.isError;
		}
		return undefined;
	}

	public setToolCallError(isError: boolean | undefined): void {
		if (this.originalType === MessageType.ToolResult || this.currentType === MessageType.ToolResult) {
			this._payload = { ...this._payload, isError } as ToolResult;
		}
	}

	public get finishReasons(): string | undefined {
		if (this.originalType === MessageType.CompletionResult) {
			return (this._payload as CompletionResult)?.choices?.find((c) => c.finish_reason)?.finish_reason;
		}
		return undefined;
	}

	// Usage

	public get usage(): CompletionUsage | undefined {
		if (this.originalType === MessageType.CompletionResult) {
			return (this._payload as CompletionResult)?.usage;
		}
		return undefined;
	}

	public set usage(usage: CompletionUsage | undefined) {
		if (this.originalType === MessageType.CompletionResult) {
			(this._payload as CompletionResult).usage = usage;
		}
	}

	// Batch helpers

	static serializeAll(messages: Message[]): SerializedMessage[] {
		return messages.map((m) => m.serialized);
	}

	static deserializeAll(data: SerializedMessage[]): Message[] {
		return data.map((d) => Message.deserialize(d));
	}

	/**
	 * Serialize messages for persistent storage (sessions / database), ensuring that
	 * raw file/image/audio binaries (base64 data URLs) are purged and only the
	 * extracted text content is preserved.
	 */
	static serializeForStorage(messages: Message[]): SerializedMessage[] {
		const serialized = Message.serializeAll(messages);
		return Message.sanitizeSerializedForStorage(serialized);
	}

	/**
	 * Strips raw base64 data URLs from file_data, image_url, and input_audio,
	 * retaining only extracted plain text and human-readable references.
	 */
	static sanitizeSerializedForStorage(serialized: SerializedMessage[]): SerializedMessage[] {
		return serialized.map((msg) => {
			if (!msg.payload) return msg;
			const clone = JSON.parse(JSON.stringify(msg)) as SerializedMessage;
			const payload = clone.payload as any;
			if (!payload) return clone;

			if (Array.isArray(payload.content)) {
				payload.content = payload.content.map((item: any) => {
					if (!item || typeof item !== "object") return item;

					// If it's a file attachment, keep extracted text or metadata only - purge raw file_data
					if (item.type === "file") {
						const filename = item.file?.filename || "file";
						return {
							type: "text",
							text: `Attached file: ${filename}\n\n[Extracted text preserved — raw binary file purged]`,
						};
					}

					// If it's an image, keep filename and note - purge giant base64 data URL
					if (item.type === "image_url") {
						const filename = item.image_url?.filename || "image";
						return {
							type: "text",
							text: `Attached image: ${filename}\n\n[Extracted visual text preserved — raw image binary purged]`,
						};
					}

					// If it's an audio attachment, purge raw audio data
					if (item.type === "input_audio") {
						return {
							type: "text",
							text: `Attached voice audio\n\n[Transcribed audio text preserved — raw audio binary purged]`,
						};
					}

					return item;
				});
			}

			return clone;
		});
	}

	/**
	 * Convert to OpenAI-compatible API format for chat completions.
	 * Excludes error messages and unaccepted requests that immediately failed.
	 */
	static toAPIMessages(messages: Message[]): APIMessage[] {
		const validMessages: Message[] = [];
		for (let i = 0; i < messages.length; i++) {
			const m = messages[i];
			if (m.type === MessageType.CompletionError) {
				continue;
			}
			// If a CompletionRequest was rejected (immediately followed by a CompletionError),
			// do not include the rejected request in subsequent API context.
			if (
				m.type === MessageType.CompletionRequest &&
				i + 1 < messages.length &&
				messages[i + 1].type === MessageType.CompletionError
			) {
				continue;
			}
			validMessages.push(m);
		}

		return validMessages.map((m): APIMessage => {
				const p = m._payload as CompletionRequest;
				let rawContent: string | MessageContent[] | null;

				if (m.currentType !== m.originalType) {
					rawContent = Array.isArray(p?.content) ? p.content : m.content;
				} else if (m.originalType === MessageType.CompletionRequest) {
					rawContent = p.content;
				} else if (m.originalType === MessageType.CompletionResult) {
					const choice = (m._payload as CompletionResult)?.choices?.[0]?.message;
					const msg: APIMessage = {
						role: choice?.role ?? MessageRole.ASSISTANT,
						content: choice?.content ?? "",
					};
					if (choice?.tool_calls && choice.tool_calls.length > 0) {
						msg.tool_calls = choice.tool_calls;
					}
					return msg;
				} else if (m.originalType === MessageType.ToolResult) {
					const tp = m._payload as ToolResult;
					return {
						role: MessageRole.TOOL,
						content: tp.content,
						tool_call_id: tp.tool_call_id,
					};
				} else {
					return { role: MessageRole.ASSISTANT, content: m.content };
				}

				if (Array.isArray(rawContent)) {
					const userTextParts: string[] = [];
					const extractedFiles: ExtractedFileInfo[] = [];
					const cleanedImageParts: MessageContent[] = [];
					const imageInfos: { filename?: string }[] = [];
					const otherParts: MessageContent[] = [];

					for (const part of rawContent) {
						if (!part) continue;
						if (part.type === "text") {
							if (isEmbeddedAttachmentText(part)) {
								const parsed = parseAttachmentPart(part);
								if (parsed) {
									extractedFiles.push(parsed);
								} else if (part.text?.trim()) {
									userTextParts.push(part.text.trim());
								}
							} else if (part.text?.trim()) {
								userTextParts.push(part.text.trim());
							}
						} else if (part.type === "image_url" && part.image_url) {
							const filename = part.image_url.filename || "image";
							imageInfos.push({ filename });
							cleanedImageParts.push({
								type: "image_url" as const,
								image_url: {
									url: part.image_url.url,
									detail: part.image_url.detail || "auto",
								},
							});
						} else {
							otherParts.push(part);
						}
					}

					const userQuery = userTextParts.join("\n\n");

					// If attachments are present (files or images)
					if (extractedFiles.length > 0 || imageInfos.length > 0) {
						const formattedUnifiedText = formatUnifiedMultiFileContent(userQuery, extractedFiles, imageInfos);
						if (cleanedImageParts.length === 0 && otherParts.length === 0) {
							// Text-only with file extractions: return single clean string for maximum model accuracy
							const msg: APIMessage = { role: m.role ?? MessageRole.USER, content: formattedUnifiedText };
							if (p?.tool_calls && p.tool_calls.length > 0) msg.tool_calls = p.tool_calls;
							if (p?.tool_call_id) msg.tool_call_id = p.tool_call_id;
							return msg;
						} else {
							// Multimodal (images or audio): 1 unified text block + image blocks
							const finalParts: MessageContent[] = [
								{ type: "text", text: formattedUnifiedText },
								...cleanedImageParts,
								...otherParts,
							];
							const msg: APIMessage = { role: m.role ?? MessageRole.USER, content: finalParts as any };
							if (p?.tool_calls && p.tool_calls.length > 0) msg.tool_calls = p.tool_calls;
							if (p?.tool_call_id) msg.tool_call_id = p.tool_call_id;
							return msg;
						}
					}

					// No attachments: ordinary multi-part text or empty
					if (cleanedImageParts.length === 0 && otherParts.length === 0) {
						const msg: APIMessage = { role: m.role ?? MessageRole.USER, content: userQuery || "" };
						if (p?.tool_calls && p.tool_calls.length > 0) msg.tool_calls = p.tool_calls;
						if (p?.tool_call_id) msg.tool_call_id = p.tool_call_id;
						return msg;
					}

					const fallbackParts: MessageContent[] = [];
					if (userQuery) fallbackParts.push({ type: "text", text: userQuery });
					fallbackParts.push(...cleanedImageParts, ...otherParts);
					const msg: APIMessage = { role: m.role ?? MessageRole.USER, content: fallbackParts as any };
					if (p?.tool_calls && p.tool_calls.length > 0) msg.tool_calls = p.tool_calls;
					if (p?.tool_call_id) msg.tool_call_id = p.tool_call_id;
					return msg;
				}

				const msg: APIMessage = { role: m.role ?? MessageRole.USER, content: rawContent ?? "" };
				if (p?.tool_calls && p.tool_calls.length > 0) msg.tool_calls = p.tool_calls;
				if (p?.tool_call_id) msg.tool_call_id = p.tool_call_id;
				return msg;
			});
	}

	/**
	 * Backward-compatible deserialization for old { role, content } format.
	 * Detects whether data is the new serialized format or old flat format.
	 */
	static fromLegacy(
		data: SerializedMessage | { role: string; content: string | null; tool_calls?: ToolCall[]; tool_call_id?: string },
		index: number,
	): Message {
		// New format has originalType
		if ("originalType" in data && data.originalType) {
			return Message.deserialize(data as SerializedMessage);
		}

		// Legacy { role, content } format
		const legacy = data as { role: string; content: string | null; tool_calls?: ToolCall[]; tool_call_id?: string };

		if (legacy.tool_calls && legacy.tool_calls.length > 0) {
			return new Message(uuidv4(), index, MessageType.CompletionResult, {
				id: uuidv4(),
				choices: [
					{
						index: 0,
						message: {
							role: MessageRole.ASSISTANT,
							content: (legacy.content as string) ?? "",
							tool_calls: legacy.tool_calls,
						},
					},
				],
			});
		}

		if (legacy.role === "tool" && legacy.tool_call_id) {
			return new Message(uuidv4(), index, MessageType.ToolResult, {
				role: MessageRole.TOOL,
				content: (legacy.content as string) ?? "",
				tool_call_id: legacy.tool_call_id,
			});
		}

		const role = (legacy.role as MessageRole) ?? MessageRole.USER;
		if (role === MessageRole.ASSISTANT) {
			return new Message(uuidv4(), index, MessageType.CompletionResult, {
				id: uuidv4(),
				choices: [{ index: 0, message: { role: MessageRole.ASSISTANT, content: (legacy.content as string) ?? "" } }],
			});
		}

		return new Message(uuidv4(), index, MessageType.CompletionRequest, {
			role,
			content: legacy.content,
		} as CompletionRequest);
	}

	static fromLegacyAll(data: any[]): Message[] {
		return data.map((d, i) => Message.fromLegacy(d, i));
	}
}