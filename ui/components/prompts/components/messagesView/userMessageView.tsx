import { Button } from "@/components/ui/button";
import { CodeEditor } from "@/components/ui/codeEditor";
import { RichTextarea } from "@/components/ui/custom/richTextarea";
import { Message, SerializedMessage, type MessageContent } from "@/lib/message";
import { JINJA_VAR_HIGHLIGHT_PATTERNS, JINJA_VAR_REGEX } from "@/lib/message/constant";
import { isJson } from "@/lib/utils/validation";
import { Paperclip, PencilIcon, Send, XIcon } from "lucide-react";
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState, type ComponentProps } from "react";
import { usePromptContext } from "../../context";
import { filesToAttachments, PROMPT_FILE_ACCEPT } from "../../utils/attachment";
import { AttachmentDisplay } from "./attachmentViews";
import MessageRoleSwitcher from "./messageRoleSwitcher";

const LazyMarkdown = lazy(() => import("@/components/ui/markdown").then((m) => ({ default: m.Markdown })));
const Markdown = (props: ComponentProps<typeof LazyMarkdown>) => (
	<Suspense fallback={null}>
		<LazyMarkdown {...props} />
	</Suspense>
);

/**
 * Render an interactive user message block that supports viewing and editing content, role switching, file attachments (via picker or drag-and-drop), and special handling for JSON and Jinja-variable content.
 *
 * @param message - The message model to render and edit; its updates are emitted via `onChange`.
 * @param disabled - When true, disables editing and attachment interactions.
 * @param allowAttachments - When true, enables attaching files (images, audio, documents) and drag-and-drop attachments.
 * @param onChange - Called with the message's serialized form whenever the message is modified (content, role, or attachments).
 * @param onRemove - Optional callback invoked when the message's delete action is triggered.
 * @param onSend - Optional callback to save and re-run conversation from this edited message (ChatGPT style).
 * @returns The JSX element that renders the user message view and its interactive controls.
 */
export function UserMessageView({
	message,
	disabled,
	allowAttachments = true,
	onChange,
	onRemove,
	onSend,
}: {
	message: Message;
	disabled?: boolean;
	allowAttachments?: boolean;
	onChange: (serialized: SerializedMessage) => void;
	onRemove?: () => void;
	onSend?: (updatedMessage: Message) => void;
}) {
	const { apiKeyId, virtualKeyId } = usePromptContext();
	const content = message.content;
	const [editMode, setEditMode] = useState(false);
	const [draftContent, setDraftContent] = useState(content);
	const containerRef = useRef<HTMLDivElement>(null);
	const fileInputRef = useRef<HTMLInputElement>(null);
	const messageRef = useRef(message);
	messageRef.current = message;
	const pendingCursorRef = useRef<number | null>(null);

	useEffect(() => {
		if (!editMode) {
			setDraftContent(message.content);
		}
	}, [message.content, editMode]);

	const handleSaveAndSend = useCallback(() => {
		const clone = message.clone();
		clone.content = draftContent;
		onChange(clone.serialized);
		setEditMode(false);
		if (onSend) {
			onSend(clone);
		}
	}, [message, draftContent, onChange, onSend]);

	const handleCancel = useCallback(() => {
		setDraftContent(message.content);
		setEditMode(false);
	}, [message.content]);
	const isEmpty = !content;
	const messageAttachments = message.attachments;
	const canAttach = allowAttachments && !disabled;
	const hasVariables = JINJA_VAR_REGEX.test(content);
	JINJA_VAR_REGEX.lastIndex = 0;
	const jsonBufferRef = useRef<string | null>(null);
	const contentIsJson = useMemo(() => !isEmpty && isJson(content), [content, isEmpty]);
	const formattedJson = useMemo(() => {
		if (!contentIsJson) return "";
		try {
			return JSON.stringify(JSON.parse(content), null, 2);
		} catch {
			return content;
		}
	}, [content, contentIsJson]);

	const applyPendingJsonBuffer = (msg: Message): Message => {
		if (jsonBufferRef.current !== null) {
			const clone = msg.clone();
			clone.content = jsonBufferRef.current;
			jsonBufferRef.current = null;
			return clone;
		}
		return msg;
	};

	const flushJsonBuffer = () => {
		const updated = applyPendingJsonBuffer(messageRef.current);
		if (updated !== messageRef.current) {
			onChange(updated.serialized);
		}
	};

	useEffect(() => {
		const handleClick = (e: MouseEvent) => {
			if (!containerRef.current?.contains(e.target as Node)) {
				setEditMode(false);
			}
		};
		document.addEventListener("mousedown", handleClick);
		return () => document.removeEventListener("mousedown", handleClick);
	}, []);

	const handleRoleChange = (role: string) => {
		const latest = applyPendingJsonBuffer(messageRef.current);
		const clone = latest.clone();
		clone.role = role as any;
		onChange(clone.serialized);
	};

	const addAttachments = useCallback(
		(newAttachments: MessageContent[]) => {
			const latest = applyPendingJsonBuffer(messageRef.current);
			const clone = latest.clone();
			clone.attachments = [...latest.attachments, ...newAttachments];
			onChange(clone.serialized);
		},
		[onChange],
	);

	const handleRemoveAttachment = useCallback(
		(index: number) => {
			const latest = applyPendingJsonBuffer(messageRef.current);
			const clone = latest.clone();
			clone.attachments = latest.attachments.filter((_, i) => i !== index);
			onChange(clone.serialized);
		},
		[message, onChange],
	);

	const handleFileSelect = useCallback(
		async (e: React.ChangeEvent<HTMLInputElement>) => {
			const files = e.target.files;
			if (!files) return;
			const attachments = await filesToAttachments(files, { apiKeyId, virtualKeyId });
			if (attachments.length > 0) addAttachments(attachments);
			e.target.value = "";
		},
		[addAttachments, apiKeyId, virtualKeyId],
	);

	// Drag & drop state
	const [isDragging, setIsDragging] = useState(false);
	const dragCounterRef = useRef(0);

	const handleDragEnter = useCallback((e: React.DragEvent) => {
		e.preventDefault();
		e.stopPropagation();
		dragCounterRef.current++;
		if (e.dataTransfer.types.includes("Files")) setIsDragging(true);
	}, []);

	const handleDragLeave = useCallback((e: React.DragEvent) => {
		e.preventDefault();
		e.stopPropagation();
		dragCounterRef.current--;
		if (dragCounterRef.current === 0) setIsDragging(false);
	}, []);

	const handleDragOver = useCallback((e: React.DragEvent) => {
		e.preventDefault();
		e.stopPropagation();
	}, []);

	const handleDrop = useCallback(
		async (e: React.DragEvent) => {
			e.preventDefault();
			e.stopPropagation();
			dragCounterRef.current = 0;
			setIsDragging(false);
			const files = e.dataTransfer.files;
			if (!files || files.length === 0) return;
			const attachments = await filesToAttachments(files, { apiKeyId, virtualKeyId });
			if (attachments.length > 0) addAttachments(attachments);
		},
		[addAttachments, apiKeyId, virtualKeyId],
	);

	const handleReadOnlyClick = (e: React.MouseEvent<HTMLTextAreaElement>) => {
		if (disabled) return;
		const target = e.target as HTMLTextAreaElement;
		pendingCursorRef.current = target.selectionStart ?? 0;
		setEditMode(true);
	};

	const handleEditFocus = (e: React.FocusEvent<HTMLTextAreaElement>) => {
		const pos = pendingCursorRef.current;
		pendingCursorRef.current = null;
		const target = e.target;
		requestAnimationFrame(() => {
			const cursorPos = pos ?? target.value.length;
			target.selectionStart = cursorPos;
			target.selectionEnd = cursorPos;
		});
	};

	return (
		<div
			className="group hover:border-border focus-within:border-border relative rounded-sm border border-transparent px-3 py-2 transition-colors"
			ref={containerRef}
			{...(canAttach
				? {
						onDragEnter: handleDragEnter,
						onDragLeave: handleDragLeave,
						onDragOver: handleDragOver,
						onDrop: handleDrop,
					}
				: {})}
		>
			{canAttach && isDragging && (
				<div className="bg-background/80 border-primary absolute inset-0 z-50 flex items-center justify-center rounded-sm border-2 border-dashed backdrop-blur-sm">
					<div className="text-primary flex flex-col items-center gap-1">
						<Paperclip className="h-5 w-5" />
						<span className="text-xs font-medium">Drop files to attach</span>
					</div>
				</div>
			)}
			<div className="mb-1 flex items-center">
				<MessageRoleSwitcher role={message.role ?? ""} disabled={disabled} onRoleChange={handleRoleChange} />
				<div className="ml-auto flex h-5 items-center gap-0.5">
					{canAttach && (
						<>
							<input
								ref={fileInputRef}
								type="file"
								multiple
								accept={PROMPT_FILE_ACCEPT}
								className="hidden"
								onChange={handleFileSelect}
							/>
							<button
								type="button"
								aria-label="Attach file"
								data-testid="user-msg-attach"
								onClick={() => fileInputRef.current?.click()}
								className="hover:bg-muted focus:bg-muted rounded-sm p-1 opacity-100"
							>
								<Paperclip className="text-muted-foreground hover:text-foreground size-3 shrink-0 cursor-pointer" />
							</button>
						</>
					)}
					{!disabled && (
						<button
							type="button"
							aria-label="Edit message"
							data-testid="user-msg-edit"
							onClick={() => setEditMode(true)}
							className="hover:bg-muted focus:bg-muted rounded-sm p-1 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100 focus:opacity-100"
						>
							<PencilIcon className="text-muted-foreground hover:text-foreground size-3 shrink-0 cursor-pointer" />
						</button>
					)}
					{!disabled && onRemove && (
						<button
							type="button"
							aria-label="Delete message"
							data-testid="user-msg-delete"
							onClick={onRemove}
							className="hover:bg-muted focus:bg-muted rounded-sm p-1 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100 focus:opacity-100"
						>
							<XIcon className="text-muted-foreground hover:text-foreground size-3 shrink-0 cursor-pointer" />
						</button>
					)}
				</div>
			</div>

			<div>
				{editMode ? (
					<div className="flex flex-col gap-2 pt-1">
						<RichTextarea
							autoFocus
							value={draftContent}
							className="text-foreground min-h-[50px] resize-y rounded-md border border-border/70 bg-background/50 p-2 text-sm shadow-xs focus-visible:ring-1 focus-visible:ring-primary dark:bg-background/30"
							textAreaClassName="rounded-md p-1 border-none"
							disabled={disabled}
							onChange={(e) => {
								setDraftContent(e.target.value);
							}}
							onKeyDown={(e) => {
								if (e.key === "Enter" && !e.shiftKey) {
									e.preventDefault();
									handleSaveAndSend();
								} else if (e.key === "Escape") {
									e.preventDefault();
									handleCancel();
								}
							}}
							onFocus={handleEditFocus}
							highlightPatterns={JINJA_VAR_HIGHLIGHT_PATTERNS}
						/>
						<div className="flex items-center justify-end gap-1.5">
							<Button
								type="button"
								variant="ghost"
								size="sm"
								onClick={handleCancel}
								className="h-7 px-2.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer"
							>
								Cancel
							</Button>
							<Button
								type="button"
								size="sm"
								onClick={handleSaveAndSend}
								disabled={disabled || (!draftContent.trim() && messageAttachments.length === 0)}
								className="h-7 px-3 text-xs gap-1.5 bg-primary text-primary-foreground hover:bg-primary/90 cursor-pointer shadow-xs"
							>
								<Send className="size-3" />
								<span>Send</span>
								<kbd className="text-[10px] opacity-75 ml-0.5">↵</kbd>
							</Button>
						</div>
					</div>
				) : isEmpty && messageAttachments.length === 0 ? (
					<div className="text-muted-foreground min-h-[20px] text-sm italic">Enter user message...</div>
				) : contentIsJson ? (
					<CodeEditor
						wrap
						code={formattedJson}
						lang="json"
						readonly={disabled}
						autoResize
						maxHeight={400}
						onChange={(value) => {
							jsonBufferRef.current = value ?? "";
						}}
						options={{
							showIndentLines: false,
							disableHover: true,
						}}
						onBlur={flushJsonBuffer}
					/>
				) : hasVariables ? (
					<RichTextarea
						readOnly
						value={content}
						className="text-muted-foreground min-h-[20px] resize-none rounded-none border-0 bg-transparent p-0 text-sm shadow-none focus-visible:ring-0 focus-visible:ring-offset-0 dark:bg-transparent"
						textAreaClassName="rounded-none p-0 border-none cursor-text"
						onClick={handleReadOnlyClick}
						highlightPatterns={JINJA_VAR_HIGHLIGHT_PATTERNS}
					/>
				) : (
					<div
						className={!disabled ? "cursor-text" : undefined}
						onClick={(e) => {
							if (disabled || editMode) return;
							if ((e.target as HTMLElement).closest("button, a, [role='button']")) return;
							setEditMode(true);
						}}
					>
						<Markdown content={content} className="text-muted-foreground" />
					</div>
				)}
			</div>

			{messageAttachments.length > 0 && (
				<AttachmentDisplay attachments={messageAttachments} editable={canAttach} onRemoveAttachment={handleRemoveAttachment} />
			)}
		</div>
	);
}