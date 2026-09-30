import { isPromptWarningMessage, formatPromptWarningMessage } from "../../utils/errorDisplay";
import { Message } from "@/lib/message";
import { cn } from "@/lib/utils";
import { AlertTriangle, XIcon } from "lucide-react";

/**
 * Render a styled warning message block with an optional delete control.
 * Provider and rate-limit failures use soft amber styling instead of destructive error UI.
 */
export default function ErrorMessageView({ message, disabled, onRemove }: { message: Message; disabled?: boolean; onRemove?: () => void }) {
	const isWarning = isPromptWarningMessage(message.content);
	const text = isWarning ? formatPromptWarningMessage(message.content) : message.content;

	return (
		<div
			className={cn(
				"group rounded-sm border border-transparent px-3 py-2 transition-colors",
				isWarning ? "hover:border-amber-500/30 focus-within:border-amber-500/30" : "hover:border-destructive/30 focus-within:border-destructive/30",
			)}
		>
			{isWarning ? (
				<div className="flex items-start gap-2 rounded-md border border-amber-500/25 bg-amber-500/10 px-2.5 py-1.5">
					<AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-600 dark:text-amber-400" />
					<div className="min-w-0 flex-1">
						<p className="text-xs font-medium text-amber-800 dark:text-amber-200">Warning</p>
						<p className="mt-0.5 text-xs leading-snug text-amber-900/90 dark:text-amber-100/90">{text}</p>
					</div>
					{!disabled && onRemove && (
						<button
							type="button"
							aria-label="Delete message"
							data-testid="error-msg-delete"
							onClick={onRemove}
							className="hover:bg-amber-500/15 focus:bg-amber-500/15 rounded-sm p-0.5 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100 focus:opacity-100"
						>
							<XIcon className="text-muted-foreground hover:text-foreground size-3 shrink-0 cursor-pointer" />
						</button>
					)}
				</div>
			) : (
				<>
					<div className="mb-1 flex h-5 items-center">
						<span className="text-destructive flex items-center gap-1 py-0.5 text-xs font-medium uppercase">
							<AlertTriangle className="size-3" />
							Error
						</span>
						<div className="ml-auto">
							{!disabled && onRemove && (
								<button
									type="button"
									aria-label="Delete message"
									data-testid="error-msg-delete"
									onClick={onRemove}
									className="hover:bg-muted focus:bg-muted rounded-sm p-1 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100 focus:opacity-100"
								>
									<XIcon className="text-muted-foreground hover:text-foreground size-3 shrink-0 cursor-pointer" />
								</button>
							)}
						</div>
					</div>
					<div className="bg-destructive/10 rounded-sm px-2.5 py-1.5">
						<p className="text-muted-foreground text-sm whitespace-pre-wrap">{text}</p>
					</div>
				</>
			)}
		</div>
	);
}
