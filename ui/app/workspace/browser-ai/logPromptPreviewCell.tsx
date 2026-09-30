import React from "react";
import { Paperclip } from "lucide-react";
import type { BrowserAILogEntry } from "@/lib/store/apis/browserAiApi";
import { oneLinePreview } from "./browserAiFormat";
import { isFileUploadLog, logAttachmentLabel, logUserCaption } from "./browserAiLogHelpers";

export function LogPromptPreviewCell({ log }: { log: BrowserAILogEntry }) {
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
							{label}
							<span className="text-muted-foreground"> -- </span>
							<span className="text-foreground/90">{oneLinePreview(caption)}</span>
						</>
					) : (
						label
					)}
				</span>
			</div>
		);
	}
	const preview = oneLinePreview(log.user_prompt_preview);
	return (
		<div className="truncate font-mono text-xs" title={preview || log.user_prompt_preview || ""}>
			{preview || "—"}
		</div>
	);
}
