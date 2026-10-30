import React from "react";
import { Paperclip, ClipboardPaste, Copy, ArrowDownToLine, Camera, Download, ShieldAlert } from "lucide-react";
import type { BrowserAILogEntry } from "@/lib/store/apis/browserAiApi";
import { oneLinePreview } from "./browserAiFormat";
import {
	isClipboardLog,
	isClipboardCopyLog,
	logClipboardPromptText,
	isDragDropLog,
	logDragDropPromptText,
	isScreenshotLog,
	logScreenshotPromptText,
	isDownloadLog,
	logDownloadPromptText,
	isCorporateDomainLog,
	logCorporatePromptText,
	isFileUploadLog,
	logAttachmentLabel,
	logUserCaption,
} from "./browserAiLogHelpers";

export function LogPromptPreviewCell({ log }: { log: BrowserAILogEntry }) {
	if (isCorporateDomainLog(log)) {
		const text = logCorporatePromptText(log);
		const label = "Personal Account Blocked";
		const title = text ? `${label} -- ${text}` : label;
		return (
			<div className="flex min-w-0 items-center gap-1.5" title={title}>
				<ShieldAlert className="h-3.5 w-3.5 shrink-0 text-red-500" />
				<span className="truncate font-mono text-xs">
					<span className="font-semibold text-red-500">{label}</span>
					{text ? (
						<>
							<span className="text-muted-foreground"> -- </span>
							<span className="text-foreground/90 font-medium">{oneLinePreview(text)}</span>
						</>
					) : null}
				</span>
			</div>
		);
	}
	if (isScreenshotLog(log)) {
		const text = logScreenshotPromptText(log);
		const label = "Screen Capture Restricted";
		const title = text ? `${label} -- ${text}` : label;
		return (
			<div className="flex min-w-0 items-center gap-1.5" title={title}>
				<Camera className="h-3.5 w-3.5 shrink-0 text-rose-500" />
				<span className="truncate font-mono text-xs">
					<span className="font-semibold text-rose-500">{label}</span>
					{text ? (
						<>
							<span className="text-muted-foreground"> -- </span>
							<span className="text-foreground/90 font-medium">{oneLinePreview(text)}</span>
						</>
					) : null}
				</span>
			</div>
		);
	}

	if (isDownloadLog(log)) {
		const text = logDownloadPromptText(log);
		const label = "Download Blocked";
		const title = text ? `${label} -- ${text}` : label;
		return (
			<div className="flex min-w-0 items-center gap-1.5" title={title}>
				<Download className="h-3.5 w-3.5 shrink-0 text-sky-400" />
				<span className="truncate font-mono text-xs">
					<span className="font-semibold text-sky-400">{label}</span>
					{text ? (
						<>
							<span className="text-muted-foreground"> -- </span>
							<span className="text-foreground/90 font-medium">{oneLinePreview(text)}</span>
						</>
					) : null}
				</span>
			</div>
		);
	}
	if (isClipboardLog(log)) {
		const text = logClipboardPromptText(log);
		const isCopy = isClipboardCopyLog(log);
		const label = isCopy ? "Clipboard Copy" : "Clipboard Paste";
		const title = text ? `${label} -- ${text}` : label;
		return (
			<div className="flex min-w-0 items-center gap-1.5" title={title}>
				{isCopy ? (
					<Copy className="h-3.5 w-3.5 shrink-0 text-amber-500" />
				) : (
					<ClipboardPaste className="h-3.5 w-3.5 shrink-0 text-amber-500" />
				)}
				<span className="truncate font-mono text-xs">
					<span className="font-semibold text-amber-500">{label}</span>
					{text ? (
						<>
							<span className="text-muted-foreground"> -- </span>
							<span className="text-foreground/90 font-medium">{oneLinePreview(text)}</span>
						</>
					) : null}
				</span>
			</div>
		);
	}

	if (isDragDropLog(log)) {
		const text = logDragDropPromptText(log);
		const label = "Drag & Drop";
		const title = text ? `${label} -- ${text}` : label;
		return (
			<div className="flex min-w-0 items-center gap-1.5" title={title}>
				<ArrowDownToLine className="h-3.5 w-3.5 shrink-0 text-purple-400" />
				<span className="truncate font-mono text-xs">
					<span className="font-semibold text-purple-400">{label}</span>
					{text ? (
						<>
							<span className="text-muted-foreground"> -- </span>
							<span className="text-foreground/90 font-medium">{oneLinePreview(text)}</span>
						</>
					) : null}
				</span>
			</div>
		);
	}

	if (isFileUploadLog(log)) {
		const label = logAttachmentLabel(log);
		const caption = logUserCaption(log);
		const title = caption ? `${label} -- ${caption}` : label;
		return (
			<div className="flex min-w-0 items-center gap-1.5" title={title}>
				<Paperclip className="h-3.5 w-3.5 shrink-0 text-sky-400" />
				<span className="truncate font-mono text-xs">
					{caption ? (
						<>
							<span className="text-foreground font-medium">{oneLinePreview(caption)}</span>
							<span className="text-muted-foreground"> ({label})</span>
						</>
					) : (
						label
					)}
				</span>
			</div>
		);
	}

	const meta = parseBrowserAiLogMetadata(log);
	const origPrompt = (typeof meta.original_prompt === "string" && meta.original_prompt.trim()) || (typeof meta.prompt === "string" && meta.prompt.trim()) || "";
	const rawPreview = (log.user_prompt_preview || "").trim();
	const fullPrompt = (log.user_prompt_full || "").trim();
	const displayTxt = (rawPreview && !rawPreview.toUpperCase().startsWith("[REDACTED]"))
		? rawPreview
		: ((fullPrompt && !fullPrompt.toUpperCase().startsWith("[REDACTED]"))
			? fullPrompt
			: (origPrompt || rawPreview || fullPrompt));
	const preview = oneLinePreview(displayTxt);
	return (
		<div className="truncate font-mono text-xs" title={preview || displayTxt || ""}>
			{preview || "—"}
		</div>
	);
}
