import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdownMenu";
import { Input } from "@/components/ui/input";
import { SplitButton } from "@/components/ui/splitButton";
import { Message, MessageRole } from "@/lib/message";
import { getErrorMessage, useIsAuthEnabledQuery, useGetVirtualKeysQuery } from "@/lib/store";
import { useCreateSessionMutation, useGetSessionsQuery, useGetVersionsQuery, useRenameSessionMutation, useUpdateSessionMutation } from "@/lib/store/apis/promptsApi";
import { ModelParams, PromptSession } from "@/lib/types/prompts";
import { cn } from "@/lib/utils";
import { Check, Eye, GitCommit, MoreHorizontal, PencilIcon, Save, ShieldCheck, Trash2 } from "lucide-react";
import { parseAsInteger, useQueryStates } from "nuqs";
import { useCallback, useRef, useState, useMemo } from "react";
import { useHotkeys } from "react-hotkeys-hook";
import { toast } from "sonner";
import { usePromptContext } from "../context";
import { isPromptMemberRole } from "../utils/memberRole";
import PromptHistoryControls from "./promptHistoryControls";

export default function PromptsViewHeader() {
	const {
		selectedPrompt,
		messages,
		setMessages: onMessagesChange,
		setCommitSheet,
		setDeletePromptDialog,
		apiKeyId,
		skillId,
		modelParams,
		provider,
		model,
		variables,
		hasChanges,
		hasVersionChanges,
		hasSessionChanges,
		isStreaming,
		canUpdate,
		canDelete,
		selectedSession: fullSelectedSession,
	} = usePromptContext();

	const { data: authStatus } = useIsAuthEnabledQuery();
	const isUserRole = isPromptMemberRole(authStatus?.role);

	const { data: virtualKeysData } = useGetVirtualKeysQuery(undefined, { skip: !isUserRole });
	const selectedVK = useMemo(
		() => (virtualKeysData?.virtual_keys ?? []).find((vk) => vk.value === apiKeyId),
		[virtualKeysData, apiKeyId],
	);
	const vkScopeLabel = useMemo(() => {
		if (!selectedVK) return null;
		const cName = selectedVK.customer?.name || selectedVK.customers?.[0]?.name;
		const tName = selectedVK.team?.name || selectedVK.teams?.[0]?.name;
		if (cName) return `Customer: ${cName}`;
		if (tName) return `Team: ${tName}`;
		return null;
	}, [selectedVK]);

	const vkBudget = useMemo(() => selectedVK?.budgets?.[0], [selectedVK]);

	const committedLabel = useMemo(() => {
		const version = selectedPrompt?.latest_version;
		const p = (model && provider) || version?.provider || provider;
		const m = model || version?.model;
		if (p && m) return `${String(p).toUpperCase()} — ${m}`;
		if (m) return m;
		return "No model selected";
	}, [selectedPrompt?.latest_version, provider, model]);

	const [sessionsOpen, setSessionsOpen] = useState(false);

	const onSessionSaved = useCallback(
		(session: PromptSession) => {
			setCommitSheet({ open: true, session });
		},
		[setCommitSheet],
	);
	// UI state — persisted in URL query params
	const [{ sessionId: selectedSessionId, versionId: selectedVersionId }, setUrlState] = useQueryStates(
		{
			sessionId: parseAsInteger,
			versionId: parseAsInteger,
		},
		{ history: "replace" },
	);

	// Fetch versions and sessions for selected prompt
	const { data: versionsData } = useGetVersionsQuery(selectedPrompt?.id ?? "", { skip: !selectedPrompt?.id });
	const { data: sessionsData } = useGetSessionsQuery(selectedPrompt?.id ?? "", { skip: !selectedPrompt?.id });

	// Mutations
	const [createSession, { isLoading: isCreatingSession }] = useCreateSessionMutation();
	const [updateSession, { isLoading: isUpdatingSession }] = useUpdateSessionMutation();
	const [renameSession] = useRenameSessionMutation();

	const versions = versionsData?.versions ?? [];
	const sessions = sessionsData?.sessions ?? [];

	const handleSelectVersion = useCallback(
		(versionId: number) => {
			setUrlState({ versionId, sessionId: null });
		},
		[setUrlState],
	);

	// Build model_params with api_key_id for persistence
	const buildSaveParams = useCallback((): ModelParams => {
		const params = { ...modelParams };
		if (apiKeyId && apiKeyId !== "__auto__") {
			params.api_key_id = apiKeyId;
		}
		if (skillId.trim()) {
			params.skill_id = skillId.trim();
		}
		return params;
	}, [modelParams, apiKeyId, skillId]);

	const handleSaveSession = useCallback(async () => {
		if (!selectedPrompt || !hasChanges || isStreaming) return;
		const data = {
			messages: Message.serializeAll(messages),
			model_params: buildSaveParams(),
			provider,
			model,
			variables: Object.keys(variables).length > 0 ? variables : undefined,
		};
		try {
			if (selectedSessionId) {
				await updateSession({
					id: selectedSessionId,
					promptId: selectedPrompt.id,
					data,
				}).unwrap();
				toast.success("Session saved");
			} else {
				const result = await createSession({
					promptId: selectedPrompt.id,
					data,
				}).unwrap();
				setUrlState({ sessionId: result.session.id, versionId: null });
				toast.success("Session saved");
			}
		} catch (err) {
			toast.error("Failed to save session", { description: getErrorMessage(err) });
		}
	}, [
		selectedPrompt?.id,
		selectedSessionId,
		messages,
		buildSaveParams,
		provider,
		model,
		variables,
		createSession,
		updateSession,
		setUrlState,
		hasChanges,
		isStreaming,
	]);

	// Cmd+S / Ctrl+S to save session
	useHotkeys(
		"mod+s",
		() => handleSaveSession(),
		{
			preventDefault: true,
			enableOnFormTags: ["input", "textarea", "select"],
			enabled: !!selectedPrompt && !isCreatingSession && !isUpdatingSession && !isStreaming,
		},
		[handleSaveSession, selectedPrompt, isCreatingSession, isUpdatingSession, isStreaming],
	);

	const handleCommitVersion = useCallback(async () => {
		if (!selectedPrompt) return;
		if (!hasChanges) {
			if (fullSelectedSession && fullSelectedSession.id === selectedSessionId) {
				onSessionSaved(fullSelectedSession);
			}
			return;
		}
		try {
			// Always create a new session with current state before committing
			const result = await createSession({
				promptId: selectedPrompt.id,
				data: {
					messages: Message.serializeAll(messages),
					model_params: buildSaveParams(),
					provider,
					model,
					variables: Object.keys(variables).length > 0 ? variables : undefined,
				},
			}).unwrap();
			setUrlState({ sessionId: result.session.id, versionId: null });
			onSessionSaved(result.session);
		} catch (err) {
			toast.error("Failed to save session", { description: getErrorMessage(err) });
		}
	}, [
		selectedPrompt?.id,
		messages,
		buildSaveParams,
		provider,
		model,
		variables,
		createSession,
		setUrlState,
		onSessionSaved,
		hasChanges,
		fullSelectedSession,
		selectedSessionId,
	]);

	const handleRenameSession = useCallback(
		async (sessionId: number, name: string) => {
			if (!selectedPrompt) return;
			try {
				await renameSession({ id: sessionId, promptId: selectedPrompt.id, data: { name } }).unwrap();
			} catch (err) {
				toast.error("Failed to rename session", { description: getErrorMessage(err) });
			}
		},
		[selectedPrompt?.id, renameSession],
	);

	const handleClearConversation = useCallback(() => {
		const firstMsg = messages[0];
		if (firstMsg?.role === MessageRole.SYSTEM) {
			onMessagesChange([firstMsg]);
		} else {
			onMessagesChange([Message.system("")]);
		}
	}, [messages]);

	const selectedVersion = versions.find((v) => v.id === selectedVersionId);
	const latestVersion = versions.find((v) => v.is_latest);
	const displayVersion = selectedVersion ?? latestVersion;

	return (
		<div className="flex items-center justify-between border-b px-4 py-3">
			<div className="flex min-w-0 items-center gap-4">
				<h3 className="truncate font-semibold">
					{selectedPrompt?.name || "Playground"}
					{!isUserRole && hasChanges && <span className="text-destructive ml-1">*</span>}
				</h3>
				{isUserRole ? (
					<>
						<Badge variant="secondary" className="max-w-[280px] truncate font-normal" title={committedLabel}>
							{committedLabel}
						</Badge>
						{selectedVK && (
							<Badge variant="outline" className="border-teal-500/40 text-teal-600 dark:text-teal-400 bg-teal-500/10 gap-1 text-xs">
								VK: {selectedVK.name}{vkScopeLabel ? ` (${vkScopeLabel})` : ""}
							</Badge>
						)}
						{authStatus?.budget !== undefined && authStatus.budget > 0 && (
							<Badge variant="outline" className="font-mono text-xs border-primary/30 text-muted-foreground" title="Your Personal User Budget">
								User: ${(authStatus.budget_current_usage ?? 0).toFixed(2)} / ${authStatus.budget.toFixed(2)}
							</Badge>
						)}
						{vkBudget && vkBudget.max_limit > 0 && (
							<Badge variant="outline" className="font-mono text-xs border-emerald-500/30 text-emerald-600 dark:text-emerald-400" title="Virtual Key Budget (Shared)">
								Key: ${(vkBudget.current_usage ?? 0).toFixed(2)} / ${vkBudget.max_limit.toFixed(2)}
							</Badge>
						)}

					</>
				) : (
					<>
						{displayVersion && <Badge variant={"secondary"}>v{displayVersion.version_number}</Badge>}
						{hasVersionChanges && versions.length > 0 && <Badge variant="outline">Unpublished Changes</Badge>}
					</>
				)}
				{!canUpdate && (
					<Badge variant="outline" className="border-amber-500/40 text-amber-600 dark:text-amber-400 bg-amber-500/10 gap-1 text-xs">
						<Eye className="h-3 w-3" />
						View Only
					</Badge>
				)}
			</div>
			<div className="flex shrink-0 items-center gap-4">
				{messages.length > 1 && (
					<Button variant="ghost" size="sm" data-testid="header-clear" onClick={handleClearConversation} disabled={isStreaming}>
						<Trash2 className="h-4 w-4" />
						Clear
					</Button>
				)}
				{!isUserRole && (
					<>
						<PromptHistoryControls />
						<SplitButton
							onClick={handleSaveSession}
							disabled={isCreatingSession || isUpdatingSession || isStreaming}
							isLoading={isCreatingSession || isUpdatingSession}
							dropdownContent={{
								className: "w-72 p-0",
								open: sessionsOpen,
								onOpenChange: setSessionsOpen,
								children: (
									<Command>
										<CommandInput placeholder="Search sessions..." data-testid="header-sessions-search" />
										<CommandList>
											<CommandEmpty>No sessions found.</CommandEmpty>
											<CommandGroup>
												{sessions.map((session) => (
													<SessionItem
														key={session.id}
														session={session}
														isSelected={selectedSessionId === session.id}
														onSelect={() => {
															setUrlState({ sessionId: session.id, versionId: null });
															setSessionsOpen(false);
														}}
														onRename={(name) => handleRenameSession(session.id, name)}
													/>
												))}
											</CommandGroup>
										</CommandList>
									</Command>
								),
							}}
							variant={"outline"}
							dropdownTrigger={{
								className: cn("bg-transparent"),
							}}
							button={{
								dataTestId: "header-save-session",
								className: "bg-transparent disabled:opacity-100 disabled:text-muted-foreground",
								disabled: !hasChanges || !canUpdate,
							}}
						>
							<Save className="h-4 w-4" />
							Save Session
						</SplitButton>
						<SplitButton
							onClick={handleCommitVersion}
							disabled={isCreatingSession || isStreaming}
							dropdownContent={{
								className: "w-64 max-h-72 overflow-y-auto",
								children: (
									<>
										<DropdownMenuLabel>Versions</DropdownMenuLabel>
										<DropdownMenuSeparator />
										{versions.length === 0 ? (
											<div className="text-muted-foreground px-2 py-3 text-center text-sm">No versions yet</div>
										) : (
											versions.map((version) => (
												<DropdownMenuItem
													key={version.id}
													onClick={() => handleSelectVersion(version.id)}
													className="flex items-center justify-between gap-2"
												>
													<div className="flex min-w-0 flex-col">
														<span className="truncate text-sm">
															v{version.version_number}
															{version.is_latest && <span className="text-primary ml-1.5 text-xs">(latest)</span>}
														</span>
														<span className="text-muted-foreground truncate text-xs">{version.commit_message || "No commit message"}</span>
														<span className="text-muted-foreground text-xs">{formatSessionDate(version.created_at)}</span>
													</div>
													{selectedVersionId === version.id && <Check className="text-primary h-4 w-4 shrink-0" />}
												</DropdownMenuItem>
											))
										)}
									</>
								),
							}}
							variant={"outline"}
							dropdownTrigger={{
								className: cn("bg-transparent"),
							}}
							button={{
								dataTestId: "header-commit-version",
								className: "bg-transparent disabled:opacity-100 disabled:text-muted-foreground",
								disabled: !hasVersionChanges || !canUpdate,
							}}
						>
							<GitCommit className="h-4 w-4" />
							Commit Version
						</SplitButton>
						{canDelete && selectedPrompt && (
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button
										variant="outline"
										size="icon"
										className="h-8 w-8 bg-transparent"
										data-testid="header-prompt-actions"
										aria-label="Prompt actions"
									>
										<MoreHorizontal className="h-4 w-4" />
									</Button>
								</DropdownMenuTrigger>
								<DropdownMenuContent align="end">
									<DropdownMenuItem
										variant="destructive"
										className="cursor-pointer text-destructive focus:text-destructive"
										data-testid="header-prompt-delete"
										onClick={() => setDeletePromptDialog({ open: true, prompt: selectedPrompt })}
									>
										<Trash2 className="mr-2 h-4 w-4" />
										Delete Prompt
									</DropdownMenuItem>
								</DropdownMenuContent>
							</DropdownMenu>
						)}
					</>
				)}
			</div>
		</div>
	);
}

function formatSessionDate(dateStr: string): string {
	const date = new Date(dateStr);
	const month = date.toLocaleString("en-US", { month: "short" });
	const day = date.getDate();
	const hours = date.getHours();
	const minutes = date.getMinutes().toString().padStart(2, "0");
	const ampm = hours >= 12 ? "pm" : "am";
	const displayHours = (hours % 12 || 12).toString().padStart(2, "0");
	return `${month} ${day}, ${displayHours}:${minutes}${ampm}`;
}

function SessionItem({
	session,
	isSelected,
	onSelect,
	onRename,
}: {
	session: PromptSession;
	isSelected: boolean;
	onSelect: () => void;
	onRename: (name: string) => void;
}) {
	const [isEditing, setIsEditing] = useState(false);
	const inputRef = useRef<HTMLInputElement>(null);

	const handleRenameSubmit = () => {
		const newName = inputRef.current?.value.trim() ?? "";
		if (!newName || newName === session.name) {
			setIsEditing(false);
			return;
		}
		onRename(newName);
		setIsEditing(false);
	};

	const dateLabel = formatSessionDate(session.created_at);

	if (isEditing) {
		return (
			<div className="flex items-center gap-2 rounded-sm px-2 py-1.5" onKeyDown={(e) => e.stopPropagation()}>
				<Input
					ref={inputRef}
					defaultValue={session.name}
					placeholder="Session name"
					className="h-auto border-none bg-transparent p-0 text-sm shadow-none focus-visible:border-none focus-visible:ring-0"
					data-testid="session-rename-input"
					autoFocus
					onKeyDown={(e) => {
						if (e.key === "Enter") handleRenameSubmit();
						if (e.key === "Escape") setIsEditing(false);
					}}
					onBlur={handleRenameSubmit}
				/>
			</div>
		);
	}

	return (
		<CommandItem
			value={`${session.id}-${dateLabel}-${session.name}-${session.user_id || ""}`}
			onSelect={onSelect}
			className="group/item flex items-center justify-between gap-2 py-1"
		>
			<div className="flex min-w-0 flex-col">
				<span className="truncate text-sm flex items-center gap-1.5">
					<span className="text-muted-foreground">{dateLabel}</span>
					{session.name && <span className="font-medium">{session.name}</span>}
					{session.user_id && (
						<span
							className="text-[10px] bg-muted px-1.5 py-0.5 rounded text-muted-foreground max-w-[120px] truncate"
							title={`User: ${session.user_id}`}
						>
							{session.user_id}
						</span>
					)}
				</span>
			</div>
			<div className="flex shrink-0 items-center gap-1">
				<button
					type="button"
					aria-label="Rename session"
					data-testid="session-rename"
					onPointerDown={(e) => {
						e.preventDefault();
						e.stopPropagation();
					}}
					onClick={(e) => {
						e.preventDefault();
						e.stopPropagation();
						setIsEditing(true);
					}}
					className="hover:bg-muted focus:bg-muted rounded-sm p-1 opacity-0 transition-opacity group-hover/item:opacity-100 focus:opacity-100"
				>
					<PencilIcon className="text-muted-foreground hover:text-foreground h-3.5 w-3.5 cursor-pointer" />
				</button>
				{isSelected && <Check className="text-primary h-4 w-4" />}
			</div>
		</CommandItem>
	);
}