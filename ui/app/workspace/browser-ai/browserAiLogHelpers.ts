import { pickBestExtractedText } from "./attachmentPreview";
import type { SecurityVerdict, SiteBlockLogFields } from "./browserAiTypes";
import type { BrowserAILogEntry } from "@/lib/store/apis/browserAiApi";

export function isSiteBlockLog(log: SiteBlockLogFields): boolean {
	const preview = `${log.user_prompt_preview || ""} ${log.user_prompt_full || ""}`.toUpperCase();
	if (preview.includes("[SITE BLOCKED]")) return true;
	if ((log.predicted_category || "").toUpperCase() === "SITE_BLOCK") return true;
	return (log.rule_triggered || "").toLowerCase() === "block entire website";
}

export function predictReasonLabel(log: BrowserAILogEntry): string {
	const status = (log.status || "").trim();
	if (status) return status;
	if (log.action === "Blocked" && log.rule_triggered) return `Blocked (${log.rule_triggered})`;
	if ((log.action === "Redacted" || log.action === "Warned") && log.rule_triggered) {
		return `Redacted (${log.rule_triggered})`;
	}
	if (log.action === "Allowed") return "Allowed (no guard rule matched)";
	return log.action || "—";
}

export function parseBrowserAiLogMetadata(log: BrowserAILogEntry): Record<string, unknown> {
	try {
		return JSON.parse(log.metadata || "{}") as Record<string, unknown>;
	} catch {
		return {};
	}
}

export function logFileStatusLine(log: BrowserAILogEntry): string {
	const full = (log.user_prompt_full || log.user_prompt_preview || "").trim();
	if (!full) return "";
	const pipe = full.indexOf(" | ");
	const head = pipe >= 0 ? full.slice(0, pipe) : full;
	if (head.startsWith("[FILE UPLOAD]") || head.startsWith("[VOICE UPLOAD]")) return head;
	return head;
}

export function logExtractedTextFromPrompt(log: BrowserAILogEntry): string {
	const full = (log.user_prompt_full || log.user_prompt_preview || "").trim();
	const pipe = full.indexOf(" | ");
	if (pipe >= 0) return full.slice(pipe + 3).trim();
	return "";
}

export function logExtractedText(log: BrowserAILogEntry, attachmentText = ""): string {
	const meta = parseBrowserAiLogMetadata(log);
	const fromMeta = typeof meta.extracted_text === "string" ? meta.extracted_text.trim() : "";
	const legacy = logExtractedTextFromPrompt(log);
	// Prefer client-side PDF parse (pdfjs) over regex garbage stored in metadata.
	return pickBestExtractedText(attachmentText, fromMeta, legacy);
}

export function isFileUploadLog(log: BrowserAILogEntry | null | undefined): boolean {
	if (!log) return false;
	if (log.attachment_name || log.attachment_stored_name) return true;
	const p = (log.user_prompt_full || log.user_prompt_preview || "").trim();
	if (p.startsWith("[FILE UPLOAD]") || p.startsWith("[VOICE UPLOAD]")) return true;
	const meta = parseBrowserAiLogMetadata(log);
	return meta.upload_scan === true;
}

export function logHasStoredAttachment(log: BrowserAILogEntry | null | undefined): boolean {
	return !!(log?.attachment_stored_name);
}

export function logUserCaption(log: BrowserAILogEntry): string {
	const meta = parseBrowserAiLogMetadata(log);
	const fromMeta = typeof meta.user_caption === "string" ? meta.user_caption.trim() : "";
	if (fromMeta && fromMeta.length <= 2000) {
		if (/^(asset_pointer|image_asset_pointer|audio_asset_pointer|content_type|file_id)$/i.test(fromMeta)) {
			/* wire junk — fall through */
		} else {
			return fromMeta;
		}
	}

	const full = (log.user_prompt_full || log.user_prompt_preview || "").trim();
	// Proxy embeds Claude-style: "[FILE UPLOAD] name.pdf -- caption — Allowed"
	const dashCaption = full.match(
		/^\[(?:FILE|VOICE) UPLOAD\]\s+.+?\s+--\s+(.+?)(?:\s+[—–-]\s+(?:Allowed|Blocked|Redacted|Warned).*)?$/i,
	);
	if (dashCaption?.[1]) {
		let after = dashCaption[1].trim();
		after = after.replace(/\s+[—–-]\s+(Allowed|Blocked|Redacted|Warned).*$/i, "").trim();
		if (
			after &&
			!after.startsWith("[FILE") &&
			!after.startsWith("[VOICE") &&
			!/^attachment(-\d+)?$/i.test(after) &&
			!/^(asset_pointer|image_asset_pointer|audio_asset_pointer|content_type|file_id)$/i.test(after)
		) {
			return after.slice(0, 500);
		}
	}
	const pipe = full.indexOf(" | ");
	if (pipe < 0) return "";
	let after = full.slice(pipe + 3).trim();
	// Strip trailing status: — Allowed / -- Blocked (rule)
	after = after.replace(/\s+[—–-]\s+(Allowed|Blocked|Redacted|Warned).*$/i, "").trim();
	// Ignore if after looks like another file status line
	if (!after || after.startsWith("[FILE") || after.startsWith("[VOICE")) return "";
	if (/^attachment(-\d+)?$/i.test(after)) return "";
	if (/^(asset_pointer|image_asset_pointer|audio_asset_pointer|content_type|file_id)$/i.test(after)) return "";
	return after.slice(0, 500);
}

export function logAttachmentLabel(log: BrowserAILogEntry): string {
	const name = (log.attachment_name || "").trim();
	const full = (log.user_prompt_full || log.user_prompt_preview || "").trim();
	const meta = parseBrowserAiLogMetadata(log);

	const isVoice =
		full.startsWith("[VOICE UPLOAD]") ||
		/\.(webm|wav|mp3|ogg|m4a|aac|flac)$/i.test(name) ||
		(typeof meta.content_type === "string" && meta.content_type.startsWith("audio/"));

	const isImage =
		meta.is_image === true ||
		meta.kind === "image" ||
		/\.(png|jpe?g|gif|webp|svg|bmp|tiff|heic)$/i.test(name) ||
		(typeof meta.content_type === "string" && meta.content_type.startsWith("image/")) ||
		/image/i.test(String(meta.predicted_category || ""));

	const defaultLabel = isVoice ? "Voice Note" : isImage ? "Image" : "Document";

	const looksLikeSiteOrTab = (label: string) => {
		const n = (label || "").trim();
		if (!n) return true;
		const compact = n.replace(/\s+/g, "");
		if (/^(?:www\.)?(?:chatgpt|chat\.openai|openai|claude|gemini|copilot|perplexity)(?:\.(?:com|ai|google\.com))?$/i.test(compact)) {
			return true;
		}
		if (/^(?:[a-z0-9-]+\.)+(?:com|ai|org|net|io|dev|app)$/i.test(compact)) {
			return true;
		}
		// Extensionless chat-tab titles like "Greeting response"
		if (!/\.[A-Za-z0-9]{1,8}$/.test(n) && !/^\.[A-Za-z0-9._-]+$/.test(n)) {
			const words = n.split(/\s+/).filter(Boolean);
			if (words.length >= 2 && words.length <= 6 && words.every((w) => /^[A-Za-z]+$/.test(w)) && !/[\d()_\-./\\]/.test(n)) {
				return true;
			}
		}
		return false;
	};

	// [FILE UPLOAD] resumes.zip [3 files: a.pdf, b.docx] (1/2) | caption — Allowed
	const m = full.match(/^\[(?:FILE|VOICE) UPLOAD\]\s+(.+?)(?:\s+[—–-]\s+|\s+--\s+|$)/i);
	if (m?.[1]) {
		let rawPart = m[1].trim();
		// Strip off leaked caption or wire garbage after pipe
		if (rawPart.includes(" | ")) {
			rawPart = rawPart.split(" | ")[0].trim();
		}
		// Match multi-file count indicator if present, e.g. (1/2) or (2/2)
		const mCount = rawPart.match(/\s+(\(\d+\/\d+\))$/);
		const counter = mCount ? ` ${mCount[1]}` : "";
		const baseLabel = rawPart.replace(/\s+\(\d+\/\d+\)\s*$/, "").trim();
		if (
			baseLabel &&
			!/^attachment(-\d+)?$/i.test(baseLabel) &&
			baseLabel.toLowerCase() !== "document.pdf" &&
			!baseLabel.startsWith("[null,") &&
			!looksLikeSiteOrTab(baseLabel)
		) {
			return `${baseLabel}${counter}`;
		}
		if (/^attachment(-\d+)?$/i.test(baseLabel) || baseLabel.toLowerCase() === "document.pdf") {
			return `${defaultLabel}${counter}`;
		}
	}
	if (
		name &&
		!/^attachment(-\d+)?$/i.test(name) &&
		name.toLowerCase() !== "document.pdf" &&
		!name.startsWith("[null,") &&
		!looksLikeSiteOrTab(name)
	) {
		return name;
	}
	if (isVoice) return "Voice Note";
	if (name && !name.startsWith("[null,") && !looksLikeSiteOrTab(name)) return name;
	return defaultLabel;
}

export function logAttachmentList(log: BrowserAILogEntry): string[] {
	const rawName = (log.attachment_name || "").trim();
	const full = (log.user_prompt_full || log.user_prompt_preview || "").trim();
	const meta = parseBrowserAiLogMetadata(log);

	const items = new Set<string>();

	// 1. Check metadata files array
	if (Array.isArray(meta.files)) {
		for (const f of meta.files) {
			if (typeof f === "string" && f.trim()) items.add(f.trim());
			else if (f && typeof f === "object" && typeof (f as { name?: string }).name === "string") {
				items.add((f as { name: string }).name.trim());
			}
		}
	}

	// 2. Extract from [FILE UPLOAD] header
	const m = full.match(/^\[(?:FILE|VOICE) UPLOAD\]\s+(.+?)(?:\s+[—–-]\s+|\s+--\s+|$)/i);
	if (m?.[1]) {
		let part = m[1].trim();
		if (part.includes(" | ")) {
			part = part.split(" | ")[0].trim();
		}
		part = part.replace(/\s+\(\d+\/\d+\)\s*$/, "").trim();

		const split = part.split(",").map((s) => s.trim()).filter(Boolean);
		for (const s of split) {
			if (s && !s.startsWith("[null,")) {
				items.add(s);
			}
		}
	}

	// 3. Extract from attachment_name
	if (rawName && !rawName.startsWith("[null,")) {
		const split = rawName.split(",").map((s) => s.trim()).filter(Boolean);
		for (const s of split) {
			if (s && !s.startsWith("[null,")) {
				items.add(s);
			}
		}
	}

	const list = Array.from(items).filter((name) => {
		const lower = name.toLowerCase();
		return !/^attachment(-\d+)?$/i.test(lower) && lower !== "document.pdf";
	});

	if (list.length > 0) return list;

	const single = logAttachmentLabel(log);
	return single ? [single] : [];
}

/** Clear security pass/fail wording for Prompt Details (AI Guard Bot + categories). */
export function securityVerdictFromLog(log: BrowserAILogEntry): SecurityVerdict {
	const cat = (log.predicted_category || "").toUpperCase();
	const status = (log.status || "").toLowerCase();
	const action = (log.action || "").toLowerCase();
	if (cat === "AI_GUARD_BOT_CLEAR" || status.includes("security ok")) {
		return {
			title: "Security OK — policy met",
			detail: "AI Guard Bot analysed this prompt and found no security-policy violation.",
			tone: "ok",
		};
	}
	if (cat === "AI_GUARD_BOT_VIOLATION") {
		return {
			title: "Security NOT met — blocked",
			detail: `AI Guard Bot found a policy violation${log.rule_triggered ? ` (${log.rule_triggered})` : ""}.`,
			tone: "bad",
		};
	}
	if (cat === "AI_GUARD_BOT_REDACT" || cat === "AI_GUARD_BOT_WARNING") {
		return {
			title: "Security notice — allowed with redaction",
			detail: `AI Guard Bot flagged this prompt${log.rule_triggered ? ` (${log.rule_triggered})` : ""}.`,
			tone: "warn",
		};
	}
	if (cat === "AI_GUARD_BOT_EVAL_ERROR" || cat === "AI_GUARD_BOT_MISCONFIGURED") {
		return {
			title: "Security check failed",
			detail: status || "AI Guard Bot could not finish evaluation.",
			tone: "bad",
		};
	}
	if (action === "blocked" || cat === "SECURITY_POLICY_VIOLATION") {
		return {
			title: "Security NOT met — blocked",
			detail: log.rule_triggered ? `Blocked by rule: ${log.rule_triggered}` : (log.status || "Blocked by Guard policy."),
			tone: "bad",
		};
	}
	if (action === "redacted" || action === "warned") {
		return {
			title: "Security redaction",
			detail: log.rule_triggered ? `Redacted by rule: ${log.rule_triggered}` : (log.status || "Redacted by Guard policy."),
			tone: "warn",
		};
	}
	if (action === "allowed") {
		return {
			title: "Allowed",
			detail: status.includes("security ok")
				? "AI Guard Bot: security OK."
				: "No blocking Guard rule matched this prompt.",
			tone: "ok",
		};
	}
	return {
		title: log.action || "Logged",
		detail: log.status || "",
		tone: "neutral",
	};
}
