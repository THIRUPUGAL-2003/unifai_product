import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Message, type MessageContent, extractVariablesFromMessages, mergeVariables } from "@/lib/message";
import { Paperclip, Play, Plus, Square } from "lucide-react";
import { useCallback, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { usePromptContext } from "../context";
import { filesToAttachments, attachmentNeedsAudio, attachmentNeedsVision } from "../utils/attachment";
import { AttachmentBadge } from "./messagesView/attachmentViews";
import MessageRoleSwitcher from "./messagesView/messageRoleSwitcher";
import { PromptFileImportBar } from "./promptFileImportBar";

export function NewMessageInputView() {
	const {
		messages,
		setMessages: onUpdateMessages,
		handleSendMessage: onSendMessage,
		handleStopStreaming: onStopStreaming,
		isStreaming,
		supportsVision,
		provider,
		model,
		requiredHeaders,
		customHeaders,
		setVariables,
		selectedPrompt,
	} = usePromptContext();
	const [userInput, setUserInput] = useState("");
	const [inputRole, setInputRole] = useState<string>("user");
	const [attachments, setAttachments] = useState<MessageContent[]>([]);
	const userInputRef = useRef<HTMLTextAreaElement>(null);

	const missingRequiredHeaders = useMemo(
		() => requiredHeaders.filter((name) => !(customHeaders[name] ?? "").trim()),
		[requiredHeaders, customHeaders],
	);

	const canAttach = inputRole === "user";

	const committed = selectedPrompt?.latest_version;
	const canRun = !!(
		(provider && model) ||
		(committed?.provider && committed?.model)
	);

	const handleAddAttachments = useCallback((newAttachments: MessageContent[]) => {
		setAttachments((prev) => [...prev, ...newAttachments]);
	}, []);

	const handleAddMessage = useCallback(() => {
		if (isStreaming) return;
		const input = userInput.trim();
		const currentAttachments = attachments.length > 0 ? [...attachments] : undefined;
		if (!input && !currentAttachments) {
			toast.error("Type a message or attach a file before adding");
			return;
		}
		setUserInput("");
		setAttachments([]);
		let msg: Message;
		if (inputRole === "user") {
			msg = Message.request(input, 0, currentAttachments);
		} else if (inputRole === "system") {
			msg = Message.system(input);
		} else {
			msg = Message.response(input);
		}
		onUpdateMessages((prev) => {
			const next = [...prev, msg];
			const varNames = extractVariablesFromMessages(next);
			setVariables((vars) => mergeVariables(vars, varNames));
			return next;
		});
		toast.success("Message added");
		setTimeout(() => userInputRef.current?.focus(), 0);
	}, [userInput, attachments, isStreaming, inputRole, onUpdateMessages, setVariables]);

	const handleRun = useCallback(async () => {
		const effectiveProvider = provider || committed?.provider;
		const effectiveModel = model || committed?.model;
		if (isStreaming || !effectiveProvider || !effectiveModel) return;
		if (missingRequiredHeaders.length > 0) {
			toast.error("Fill required headers in Settings before running", {
				description: missingRequiredHeaders.join(", "),
			});
			return;
		}
		const input = userInput.trim();
		const currentAttachments = attachments.length > 0 ? [...attachments] : undefined;
		if (input || currentAttachments) {
			setUserInput("");
			setAttachments([]);
		}
		let pendingMessage: Message | undefined;
		if (input || currentAttachments) {
			if (inputRole === "system") {
				pendingMessage = Message.system(input);
			} else if (inputRole === "user") {
				pendingMessage = Message.request(input, 0, currentAttachments);
			} else {
				pendingMessage = Message.response(input);
			}
		}
		await onSendMessage(pendingMessage);
		setTimeout(() => {
			userInputRef.current?.focus();
		}, 100);
	}, [userInput, attachments, isStreaming, inputRole, onSendMessage, provider, model, committed, missingRequiredHeaders]);

	const handleKeyDown = useCallback(
		(e: React.KeyboardEvent) => {
			if (e.key === "Enter" && !e.shiftKey) {
				e.preventDefault();
				handleRun();
			}
		},
		[handleRun],
	);

	const handleRemoveAttachment = useCallback((index: number) => {
		setAttachments((prev) => prev.filter((_, i) => i !== index));
	}, []);

	const handlePaste = useCallback(async (e: React.ClipboardEvent) => {
		if (!canAttach) return;
		const items = e.clipboardData?.items;
		if (!items) return;

		const imageFiles: File[] = [];
		for (const item of Array.from(items)) {
			if (item.type.startsWith("image/")) {
				const file = item.getAsFile();
				if (file) imageFiles.push(file);
			}
		}
		if (imageFiles.length === 0) return;

		e.preventDefault();
		const newAttachments = await filesToAttachments(imageFiles);
		if (newAttachments.length > 0) {
			handleAddAttachments(newAttachments);
		}
	}, [canAttach, handleAddAttachments]);

	const [isDragging, setIsDragging] = useState(false);
	const dragCounterRef = useRef(0);

	const handleDragEnter = useCallback(
		(e: React.DragEvent) => {
			if (!canAttach) return;
			e.preventDefault();
			e.stopPropagation();
			dragCounterRef.current++;
			if (e.dataTransfer.types.includes("Files")) {
				setIsDragging(true);
			}
		},
		[canAttach],
	);

	const handleDragLeave = useCallback((e: React.DragEvent) => {
		e.preventDefault();
		e.stopPropagation();
		dragCounterRef.current--;
		if (dragCounterRef.current === 0) {
			setIsDragging(false);
		}
	}, []);

	const handleDragOver = useCallback((e: React.DragEvent) => {
		e.preventDefault();
		e.stopPropagation();
	}, []);

	const handleDrop = useCallback(
		async (e: React.DragEvent) => {
			if (!canAttach) return;
			e.preventDefault();
			e.stopPropagation();
			dragCounterRef.current = 0;
			setIsDragging(false);

			const files = e.dataTransfer.files;
			if (!files || files.length === 0) return;

			const newAttachments = await filesToAttachments(files);
			if (newAttachments.length > 0) {
				handleAddAttachments(newAttachments);
			}
		},
		[canAttach, handleAddAttachments],
	);

	const hasImageAttachment = attachmentNeedsVision(attachments);
	const hasAudioAttachment = attachmentNeedsAudio(attachments);

	return (
		<div
			className="group relative max-h-[500px] shrink-0 overflow-y-auto border-t px-4 py-2"
			onDragEnter={handleDragEnter}
			onDragLeave={handleDragLeave}
			onDragOver={handleDragOver}
			onDrop={handleDrop}
		>
			{canAttach && isDragging && (
				<div className="bg-background/80 border-primary absolute inset-0 z-50 flex items-center justify-center rounded-sm border-2 border-dashed backdrop-blur-sm">
					<div className="text-primary flex flex-col items-center gap-1">
						<Paperclip className="h-5 w-5" />
						<span className="text-xs font-medium">Drop files to attach</span>
					</div>
				</div>
			)}
			<div className="mb-1 flex items-center gap-2">
				<MessageRoleSwitcher
					role={inputRole}
					disabled={isStreaming}
					onRoleChange={(role) => {
						setInputRole(role);
						if (role !== "user") setAttachments([]);
					}}
					restrictedRoles={["system", "tool"]}
				/>
				{canAttach && (
					<PromptFileImportBar
						className="ml-auto shrink-0"
						disabled={isStreaming}
						onAttachmentsAdded={handleAddAttachments}
					/>
				)}
			</div>
			{canAttach && hasImageAttachment && !supportsVision && (
				<p className="text-muted-foreground mb-2 text-xs">
					Image attached. Select a vision-capable model if the provider should process images.
				</p>
			)}
			{canAttach && hasAudioAttachment && (
				<p className="text-muted-foreground mb-2 text-xs">
					Voice attached. Whisper will transcribe to text when possible so any chat model can answer; otherwise raw audio
					is sent.
				</p>
			)}
			{attachments.length > 0 && (
				<div className="mb-2 flex flex-wrap gap-2">
					{attachments.map((att, index) => (
						<AttachmentBadge key={index} attachment={att} onRemove={() => handleRemoveAttachment(index)} />
					))}
				</div>
			)}
			<div className="relative">
				<Textarea
					placeholder={canAttach ? "Type a message or import files..." : "Type a message..."}
					value={userInput}
					ref={userInputRef}
					onChange={(e) => setUserInput(e.target.value)}
					onKeyDown={handleKeyDown}
					onPaste={handlePaste}
					data-testid="new-message-textarea"
					className="text-muted-foreground min-h-[60px] resize-none rounded-none border-0 bg-transparent p-0 pr-16 text-sm shadow-none focus-visible:ring-0 focus-visible:ring-offset-0 dark:bg-transparent"
					disabled={isStreaming}
				/>
				<div className="absolute right-0 bottom-0 flex items-center gap-1">
					<Button
						onClick={handleAddMessage}
						disabled={isStreaming}
						variant={"ghost"}
						data-testid="new-message-add"
						className="text-muted-foreground hover:text-foreground flex items-center gap-1 rounded px-1.5 py-1 text-xs disabled:pointer-events-none disabled:opacity-50"
					>
						<Plus className="h-3.5 w-3.5" />
						Add
					</Button>
					{isStreaming ? (
						<Button
							onClick={onStopStreaming}
							variant={"ghost"}
							data-testid="new-message-stop"
							className="text-destructive hover:text-destructive hover:bg-destructive/10 flex items-center gap-1 rounded px-1.5 py-1 text-xs"
						>
							<Square className="!h-3 !w-3 fill-current" />
							Stop
						</Button>
					) : (
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									onClick={handleRun}
									disabled={!canRun}
									variant={"ghost"}
									data-testid="new-message-run"
									className="text-muted-foreground hover:text-foreground flex items-center gap-1 rounded px-1.5 py-1 text-xs disabled:pointer-events-none disabled:opacity-50"
								>
									<Play className="h-3.5 w-3.5" />
									Run
								</Button>
							</TooltipTrigger>
							<TooltipContent side="top">
								{!canRun ? <span>Select a prompt with a committed model to run</span> : <span>Run prompt</span>}
								<kbd className="bg-primary-foreground/20 ml-1.5 rounded px-1 py-0.5 font-mono text-[10px]">↵</kbd>
							</TooltipContent>
						</Tooltip>
					)}
				</div>
			</div>
		</div>
	);
}
