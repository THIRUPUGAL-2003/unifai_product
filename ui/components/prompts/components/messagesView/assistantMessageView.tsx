import { Textarea } from "@/components/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Message, SerializedMessage } from "@/lib/message";
import { InfoIcon, PencilIcon, XIcon, FileDown, FileText, FileSpreadsheet, Code2, FolderArchive, Sparkles, ChevronDown } from "lucide-react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdownMenu";
import { lazy, Suspense, useEffect, useMemo, useRef, useState, type ComponentProps } from "react";
import MessageRoleSwitcher from "./messageRoleSwitcher";
import { isJson } from "@/lib/utils/validation";
import { CodeEditor } from "@/components/ui/codeEditor";
import {
	detectGeneratableFiles,
	exportToPdf,
	exportToWordDocx,
	exportToExcel,
	exportCodeFile,
	exportAllCodeFilesZip,
} from "../../utils/generateFiles";
import { lastFile } from "../../utils/fileStudio";

const LazyMarkdown = lazy(() => import("@/components/ui/markdown").then((m) => ({ default: m.Markdown })));
const Markdown = (props: ComponentProps<typeof LazyMarkdown>) => (
	<Suspense fallback={null}>
		<LazyMarkdown {...props} />
	</Suspense>
);

/**
 * Renders the assistant message UI including role switcher, usage tooltip, edit/delete controls, and editable or view-only content.
 *
 * The component allows inline editing of plain text or JSON content (JSON edits are buffered and committed on blur), toggling role, and removing the message. Clicking outside the component exits edit mode.
 *
 * @param message - The message model to display and edit; updates are emitted via `onChange` as the message's serialized form.
 * @param disabled - When true, disables editing, role changes, and delete action.
 * @param isStreaming - When true, shows streaming state (loading indicator) and prevents entering edit mode.
 * @param onChange - Called when the message is modified; receives the message's serialized representation.
 * @param onRemove - Optional callback invoked when the delete action is triggered.
 * @returns The rendered assistant message element.
 */
export function AssistantMessageView({
	message,
	disabled,
	isStreaming,
	onChange,
	onRemove,
}: {
	message: Message;
	disabled?: boolean;
	isStreaming?: boolean;
	onChange: (serialized: SerializedMessage) => void;
	onRemove?: () => void;
}) {
	const [editMode, setEditMode] = useState(false);
	const containerRef = useRef<HTMLDivElement>(null);
	const content = message.content;
	const isEmpty = !content;
	const usage = message.usage;
	const jsonBufferRef = useRef<string | null>(null);
	const contentIsJson = useMemo(() => !isEmpty && !isStreaming && isJson(content), [content, isEmpty, isStreaming]);
	const formattedJson = useMemo(() => {
		if (!contentIsJson) return "";
		try {
			return JSON.stringify(JSON.parse(content), null, 2);
		} catch {
			return content;
		}
	}, [content, contentIsJson]);

	const flushJsonBuffer = () => {
		if (jsonBufferRef.current !== null) {
			const clone = message.clone();
			clone.content = jsonBufferRef.current;
			onChange(clone.serialized);
			jsonBufferRef.current = null;
		}
	};

	const generatable = useMemo(() => {
		if (isEmpty || isStreaming) return null;
		return detectGeneratableFiles(content);
	}, [content, isEmpty, isStreaming]);

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
		const clone = message.clone();
		clone.role = role as any;
		onChange(clone.serialized);
	};

	return (
		<div
			className="group hover:border-border focus-within:border-border rounded-sm border border-transparent px-3 py-2 transition-colors"
			ref={containerRef}
		>
			<div className="mb-1 flex items-center">
				<MessageRoleSwitcher role={message.role ?? ""} disabled={disabled} onRoleChange={handleRoleChange} />
				<div className="ml-auto flex h-5 items-center gap-0.5">
					{usage && (
						<Tooltip>
							<TooltipTrigger className="hover:bg-muted focus:bg-muted rounded-sm p-1 focus:opacity-100">
								<InfoIcon className="text-muted-foreground hover:text-foreground size-3 shrink-0 cursor-pointer opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100" />
							</TooltipTrigger>
							<TooltipContent side="bottom">
								<div className="flex flex-col gap-0.5 text-xs tabular-nums">
									<span>
										<span className="inline-block w-12">Input:</span> {usage.prompt_tokens} tokens
									</span>
									<span>
										<span className="inline-block w-12">Output:</span> {usage.completion_tokens} tokens
									</span>
									<span>
										<span className="inline-block w-12">Total:</span> {usage.total_tokens} tokens
									</span>
								</div>
							</TooltipContent>
						</Tooltip>
					)}
					{!disabled && !isStreaming && (
						<button
							type="button"
							aria-label="Edit message"
							data-testid="assistant-msg-edit"
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
							data-testid="assistant-msg-delete"
							onClick={onRemove}
							className="hover:bg-muted focus:bg-muted rounded-sm p-1 opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100 focus:opacity-100"
						>
							<XIcon className="text-muted-foreground hover:text-foreground size-3 shrink-0 cursor-pointer" />
						</button>
					)}
				</div>
			</div>

			<div>
				{isStreaming && isEmpty ? (
					<div className="flex items-center gap-1 py-1">
						<span className="bg-muted-foreground h-1.5 w-1.5 animate-bounce rounded-full opacity-60" style={{ animationDelay: "0ms" }} />
						<span className="bg-muted-foreground h-1.5 w-1.5 animate-bounce rounded-full opacity-60" style={{ animationDelay: "150ms" }} />
						<span className="bg-muted-foreground h-1.5 w-1.5 animate-bounce rounded-full opacity-60" style={{ animationDelay: "300ms" }} />
					</div>
				) : editMode ? (
					<Textarea
						autoFocus
						value={content}
						className="text-muted-foreground min-h-[20px] resize-none rounded-none border-0 bg-transparent p-0 text-sm shadow-none focus-visible:ring-0 focus-visible:ring-offset-0 dark:bg-transparent"
						disabled={disabled}
						onChange={(e) => {
							const clone = message.clone();
							clone.content = e.target.value;
							onChange(clone.serialized);
						}}
						onFocus={(e) => {
							const target = e.target;
							requestAnimationFrame(() => {
								target.selectionStart = target.value.length;
								target.selectionEnd = target.value.length;
							});
						}}
						onBlur={() => {
							if (content.trim().length > 0) setEditMode(false);
						}}
					/>
				) : isEmpty ? (
					<div className="text-muted-foreground min-h-[20px] text-sm italic">Enter assistant message...</div>
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
						onBlur={flushJsonBuffer}
						options={{
							showIndentLines: false,
							disableHover: true,
						}}
					/>
				) : (
					<div
						className={!disabled && !isStreaming ? "cursor-text" : undefined}
						onClick={(e) => {
							if (disabled || isStreaming || editMode) return;
							if ((e.target as HTMLElement).closest("button, a, [role='button']")) return;
							setEditMode(true);
						}}
					>
						<Markdown content={content} isStreaming={isStreaming} className="text-muted-foreground" />
					</div>
				)}

				{!isEmpty && !isStreaming && !editMode && generatable && (generatable.canPdf || generatable.canExcel || generatable.canCode) && (
					<div className="mt-2.5 pt-2 border-t border-border/40 flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground select-none">
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<button
									type="button"
									className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-sm bg-muted/70 hover:bg-muted hover:text-foreground text-[11px] font-medium border border-border/50 transition-colors cursor-pointer"
								>
									<FileDown className="size-3.5 text-primary" />
									Download
									<ChevronDown className="size-3 opacity-70" />
								</button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="start" side="bottom" className="min-w-[180px]">
								{generatable.canPdf && (
									<DropdownMenuItem onSelect={() => exportToPdf(content)} className="gap-2 text-xs cursor-pointer">
										<FileText className="size-3.5 text-rose-500" />
										PDF
									</DropdownMenuItem>
								)}
								{generatable.canDocx && (
									<DropdownMenuItem onSelect={() => exportToWordDocx(content)} className="gap-2 text-xs cursor-pointer">
										<FileText className="size-3.5 text-blue-500" />
										Word (.docx)
									</DropdownMenuItem>
								)}
								{generatable.canExcel && (
									<DropdownMenuItem onSelect={() => exportToExcel(content)} className="gap-2 text-xs cursor-pointer">
										<FileSpreadsheet className="size-3.5 text-emerald-500" />
										Excel (.xlsx)
									</DropdownMenuItem>
								)}
								{generatable.canCode && generatable.codeBlocks.length > 1 && (
									<DropdownMenuItem onSelect={() => exportAllCodeFilesZip(generatable.codeBlocks)} className="gap-2 text-xs cursor-pointer">
										<FolderArchive className="size-3.5 text-amber-500" />
										Project ZIP ({generatable.codeBlocks.length} files)
									</DropdownMenuItem>
								)}
								{generatable.canCode &&
									generatable.codeBlocks.map((b, idx) => (
										<DropdownMenuItem key={idx} onSelect={() => exportCodeFile(b)} className="gap-2 text-xs cursor-pointer">
											<Code2 className="size-3.5 text-amber-500" />
											{b.filename}
										</DropdownMenuItem>
									))}
							</DropdownMenuContent>
						</DropdownMenu>

						<button
							type="button"
							onClick={() => {
								const prevFile = lastFile();
								const target = prevFile
									? `${prevFile.kind} file`
									: generatable.canCode
										? "code"
										: generatable.tablesCount
											? "Excel table"
											: "document";
								window.dispatchEvent(
									new CustomEvent("prompt-repo:set-input", {
										detail: `Please alter and update the generated ${target} with the following changes: `,
									})
								);
							}}
							className="ml-auto inline-flex items-center gap-1 px-2 py-0.5 rounded-sm bg-primary/10 hover:bg-primary/20 text-primary text-[11px] font-medium transition-colors cursor-pointer"
							title="Instruct model to alter/modify this generated file"
						>
							<Sparkles className="size-3" />
							Alter / Modify
						</button>
					</div>
				)}
			</div>
		</div>
	);
}