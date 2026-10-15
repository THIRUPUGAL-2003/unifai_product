import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Message, type MessageContent } from "@/lib/message";
import { getErrorMessage, useGetVirtualKeyBillingBlocksQuery, useGetVirtualKeysQuery, useIsAuthEnabledQuery } from "@/lib/store";
import { AlertTriangle, Loader2, Paperclip, Play, Square } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { usePromptContext } from "../context";
import { filesToAttachments, isImageFile, attachmentNeedsAudio, attachmentNeedsVision } from "../utils/attachment";
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
		selectedPrompt,
		apiKeyId,
		virtualKeyId,
	} = usePromptContext();
	const [userInput, setUserInput] = useState("");
	const [inputRole, setInputRole] = useState<string>("user");
	const [attachments, setAttachments] = useState<MessageContent[]>([]);
	const userInputRef = useRef<HTMLTextAreaElement>(null);

	const POLL_MS = 5000;
	const [authPollMs, setAuthPollMs] = useState(POLL_MS);
	const [vkPollMs, setVkPollMs] = useState(POLL_MS);
	const [billingPollMs, setBillingPollMs] = useState(POLL_MS);
	const { data: authStatus, error: authError } = useIsAuthEnabledQuery(undefined, { pollingInterval: authPollMs });
	const { data: virtualKeysData, error: vkError, isError: vkFailed } = useGetVirtualKeysQuery(undefined, {
		pollingInterval: vkPollMs,
	});
	const isMember = Boolean(authStatus?.role && authStatus.role !== "admin" && authStatus.role !== "super_admin");
	const { data: billingBlocks, error: billingError, isError: billingFailed } = useGetVirtualKeyBillingBlocksQuery(undefined, {
		pollingInterval: isMember ? billingPollMs : 0,
		skip: !isMember,
	});
	const vkBillingQueryFailed = vkFailed || (isMember && billingFailed);
	const vkBillingQueryError = vkError || (isMember ? billingError : undefined);

	useEffect(() => {
		setAuthPollMs(authError ? 0 : POLL_MS);
	}, [authError]);
	useEffect(() => {
		setVkPollMs(vkError ? 0 : POLL_MS);
	}, [vkError]);
	useEffect(() => {
		setBillingPollMs(billingError ? 0 : POLL_MS);
	}, [billingError]);

	const budgetAlert = useMemo(() => {
		// 1. Check User Personal Budget
		const userLimit = authStatus?.budget;
		const userUsage = authStatus?.budget_current_usage ?? 0;
		if (userLimit && userLimit > 0) {
			if (userUsage >= userLimit) {
				return {
					isExhausted: true,
					message: `User Budget Limit Reached: $${userUsage.toFixed(2)} / $${userLimit.toFixed(2)} (100% used). Running prompts will be blocked until reset.`,
				};
			}
			if (userUsage / userLimit >= 0.8) {
				const pct = Math.round((userUsage / userLimit) * 100);
				return {
					isExhausted: false,
					message: `User Budget Warning: ${pct}% used ($${userUsage.toFixed(2)} / $${userLimit.toFixed(2)}). Approaching maximum limit.`,
				};
			}
		}

		// 2. Check Selected Virtual Key Budget
		if (virtualKeyId && virtualKeyId !== "__auto__" && virtualKeyId.startsWith("sk-uf-")) {
			const vks = virtualKeysData?.virtual_keys ?? [];
			const currentVk = vks.find((v) => v.value === virtualKeyId);
			const block = currentVk ? billingBlocks?.blocks?.[currentVk.id] : undefined;
			if (currentVk && block) {
				return {
					isExhausted: true,
					message: `${block.scope === "team" ? "Team" : "Customer"} budget for "${block.name}" is used up, so Virtual Key "${currentVk.name}" is blocked. Pick another key.`,
				};
			}
			if (currentVk?.budgets && currentVk.budgets.length > 0) {
				for (const b of currentVk.budgets) {
					if (b.max_limit > 0) {
						const usage = b.current_usage ?? 0;
						if (usage >= b.max_limit) {
							return {
								isExhausted: true,
								message: `Virtual Key "${currentVk.name}" Budget Limit Reached: $${usage.toFixed(2)} / $${b.max_limit.toFixed(2)} (${b.reset_duration || "monthly"}). Calls blocked.`,
							};
						}
						if (usage / b.max_limit >= 0.8) {
							const pct = Math.round((usage / b.max_limit) * 100);
							return {
								isExhausted: false,
								message: `Virtual Key "${currentVk.name}" Budget Warning: ${pct}% used ($${usage.toFixed(2)} / $${b.max_limit.toFixed(2)}).`,
							};
						}
					}
				}
			}
		}
		return null;
	}, [authStatus, virtualKeyId, virtualKeysData, billingBlocks]);

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

	interface PendingUploadItem {
		id: string;
		name: string;
		size: number;
		ext: string;
		progressText: string;
	}

	const [pendingUploads, setPendingUploads] = useState<PendingUploadItem[]>([]);

	const handleAddAttachments = useCallback((newAttachments: MessageContent[]) => {
		setAttachments((prev) => [...prev, ...newAttachments]);
	}, []);

	const processFiles = useCallback(
		async (filesList: File[]) => {
			if (!filesList || filesList.length === 0) return;
			const newPending: PendingUploadItem[] = filesList.map((f) => {
				const ext = f.name.split(".").pop()?.toLowerCase() || "";
				let progressText = "Extracting content...";
				if (ext === "pdf") progressText = "Parsing PDF pages...";
				else if (["xlsx", "xls", "csv"].includes(ext)) progressText = "Parsing sheets to tables...";
				else if (["docx", "doc"].includes(ext)) progressText = "Extracting Word document...";
				else if (["pptx", "ppt"].includes(ext)) progressText = "Extracting slides & notes...";
				else if (["zip", "tar", "gz"].includes(ext)) progressText = "Unpacking archive members...";
				else if (["png", "jpg", "jpeg", "webp", "gif"].includes(ext)) progressText = "Preparing visual image...";
				else if (["mp3", "wav", "m4a", "webm"].includes(ext)) progressText = "Transcribing with Whisper...";
				return {
					id: `upload_${Date.now()}_${Math.random().toString(36).slice(2, 7)}`,
					name: f.name,
					size: f.size,
					ext,
					progressText,
				};
			});

			setPendingUploads((prev) => [...prev, ...newPending]);

			try {
				const newAttachments = await filesToAttachments(filesList, { apiKeyId, virtualKeyId });
				if (newAttachments.length > 0) {
					handleAddAttachments(newAttachments);
				}
			} catch (err) {
				toast.error(err instanceof Error ? err.message : "Failed to process files");
			} finally {
				setPendingUploads((prev) => prev.filter((p) => !newPending.some((np) => np.id === p.id)));
			}
		},
		[apiKeyId, virtualKeyId, handleAddAttachments],
	);

	useEffect(() => {
		const handleGlobalAttach = (e: Event) => {
			const ce = e as CustomEvent<File[]>;
			if (ce.detail && Array.isArray(ce.detail) && ce.detail.length > 0) {
				processFiles(ce.detail);
			}
		};
		window.addEventListener("prompt-repo:attach-files", handleGlobalAttach);
		return () => window.removeEventListener("prompt-repo:attach-files", handleGlobalAttach);
	}, [processFiles]);

	const textBeforeRecordingRef = useRef<string>("");
	const [isVoiceRecording, setIsVoiceRecording] = useState(false);

	const handleRecordingChange = useCallback((recording: boolean) => {
		setIsVoiceRecording(recording);
		if (recording) {
			textBeforeRecordingRef.current = userInput;
		}
	}, [userInput]);

	const handleLiveTranscript = useCallback((liveText: string) => {
		const base = textBeforeRecordingRef.current.trim();
		const updated = base ? `${base} ${liveText}` : liveText;
		setUserInput(updated);
	}, []);

	const handleRecordingCancel = useCallback(() => {
		setUserInput(textBeforeRecordingRef.current);
		setIsVoiceRecording(false);
	}, []);

	const handleTextExtracted = useCallback((text: string) => {
		const base = textBeforeRecordingRef.current.trim();
		const updated = base ? `${base} ${text}` : text;
		setUserInput(updated);
		setIsVoiceRecording(false);
		setTimeout(() => userInputRef.current?.focus(), 0);
	}, []);

	const handleRun = useCallback(async () => {
		if (budgetAlert?.isExhausted) {
			toast.error("Budget Limit Reached", {
				description: budgetAlert.message,
			});
			return;
		}
		if (isStreaming) return;
		const effectiveProvider = provider || committed?.provider;
		const effectiveModel = model || committed?.model;
		if (!effectiveProvider || !effectiveModel) {
			toast.error("Provider and Model Required", {
				description: "Please select a provider and model in the Settings panel before running.",
			});
			return;
		}
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
	}, [userInput, attachments, isStreaming, inputRole, onSendMessage, provider, model, committed, missingRequiredHeaders, budgetAlert]);

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

	const handlePaste = useCallback(
		async (e: React.ClipboardEvent) => {
			if (!canAttach) return;
			const items = e.clipboardData?.items;
			if (!items) return;

			const pastedFiles: File[] = [];
			for (const item of Array.from(items)) {
				if (item.kind === "file") {
					const file = item.getAsFile();
					if (file) pastedFiles.push(file);
				}
			}
			if (pastedFiles.length === 0) return;

			e.preventDefault();
			processFiles(pastedFiles);
		},
		[canAttach, processFiles],
	);

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

			processFiles(Array.from(files));
		},
		[canAttach, processFiles],
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
				<div className="bg-background/80 border-primary absolute inset-0 z-50 flex items-center justify-center rounded-sm border-2 border-dashed backdrop-blur-sm pointer-events-none">
					<div className="text-primary flex flex-col items-center gap-1">
						<Paperclip className="h-5 w-5 animate-bounce" />
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
						onFilesSelected={processFiles}
						onTextExtracted={handleTextExtracted}
						onLiveTranscript={handleLiveTranscript}
						onRecordingChange={handleRecordingChange}
						onRecordingCancel={handleRecordingCancel}
					/>
				)}
			</div>
			{canAttach && hasImageAttachment && !supportsVision && (
				<p className="mb-2 text-xs text-amber-600 dark:text-amber-400">
					{model ? `"${model}" is not marked as vision-capable` : "No model selected"} — the image may be rejected or ignored.
					Pick a vision model (e.g. GPT-4o, Claude, Gemini) so it can answer questions about the image.
				</p>
			)}
			{canAttach && hasAudioAttachment && (
				<p className="text-muted-foreground mb-2 text-xs">
					Voice attached. Whisper will transcribe to text when possible so any chat model can answer; otherwise raw audio
					is sent.
				</p>
			)}
			{(attachments.length > 0 || pendingUploads.length > 0) && (
				<div className="mb-2 flex flex-wrap gap-2 items-center">
					{attachments.map((att, index) => (
						<AttachmentBadge key={index} attachment={att} onRemove={() => handleRemoveAttachment(index)} />
					))}
					{pendingUploads.map((pu) => (
						<div
							key={pu.id}
							className="group/pending bg-card/90 border-primary/40 text-foreground relative flex items-center gap-2 rounded-md border px-2.5 py-1 text-xs shadow-xs animate-pulse select-none"
						>
							<Loader2 className="h-3.5 w-3.5 animate-spin text-primary shrink-0" />
							<div className="flex flex-col min-w-0">
								<span className="font-medium truncate max-w-[140px] text-[11px]" title={pu.name}>
									{pu.name}
								</span>
								<span className="text-[10px] text-muted-foreground">{pu.progressText}</span>
							</div>
						</div>
					))}
				</div>
			)}
			{vkBillingQueryFailed ? (
				<QueryErrorBanner
					className="mb-2"
					testId="prompt-vk-billing-query-error"
					message={getErrorMessage(vkBillingQueryError) || "Failed to load virtual keys or billing blocks."}
				/>
			) : null}
			{budgetAlert && (
				<div
					data-testid="prompt-budget-alert-banner"
					className={`mb-2 flex items-center gap-2 rounded-md border px-3 py-2 text-xs transition-colors ${
						budgetAlert.isExhausted
							? "bg-rose-500/10 text-rose-600 dark:text-rose-400 border-rose-500/30 font-medium"
							: "bg-amber-500/10 text-amber-700 dark:text-amber-300 border-amber-500/30"
					}`}
				>
					<AlertTriangle className={`h-4 w-4 shrink-0 ${budgetAlert.isExhausted ? "text-rose-500" : "text-amber-500"}`} />
					<span className="flex-1">{budgetAlert.message}</span>
				</div>
			)}
			<div
				className={`relative rounded-md transition-all ${
					isVoiceRecording
						? "ring-1 ring-rose-500/40 bg-rose-500/[0.02] p-1.5"
						: ""
				}`}
			>
				<Textarea
					placeholder={
						isVoiceRecording
							? "Listening... Speak, and your words appear here live..."
							: canAttach
								? "Type a message or import files..."
								: "Type a message..."
					}
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
								{!canRun ? <span>Select a key and a model in Settings to run</span> : <span>Run prompt</span>}
								<kbd className="bg-primary-foreground/20 ml-1.5 rounded px-1 py-0.5 font-mono text-[10px]">↵</kbd>
							</TooltipContent>
						</Tooltip>
					)}
				</div>
			</div>
		</div>
	);
}
