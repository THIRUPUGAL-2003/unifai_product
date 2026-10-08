import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { Button } from "@/components/ui/button";
import { useGetSessionsQuery, useCreateSessionMutation } from "@/lib/store/apis/promptsApi";
import { getErrorMessage, useIsAuthEnabledQuery, useGetTeamsQuery, useGetSessionUsersQuery } from "@/lib/store";
import { parseAsInteger, useQueryStates } from "nuqs";
import { toast } from "sonner";
import { Message } from "@/lib/message";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdownMenu";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scrollArea";
import { CreateSessionRequest, Folder, Prompt } from "@/lib/types/prompts";
import { cn } from "@/lib/utils";
import { DragDropProvider, useDraggable, useDroppable } from "@dnd-kit/react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
	Archive,
	Building,
	ChevronDown,
	ChevronRight,
	FileText,
	Folder as FolderIcon,
	FolderOpen,
	MoreHorizontal,
	PanelLeftClose,
	Pencil,
	Plus,
	PlusIcon,
	Search,
	ShieldCheck,
	Trash2,
	UserCheck,
	Users,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { usePromptContext } from "../context";
import { isPromptMemberRole } from "../utils/memberRole";

/** Folder ids from startId up to its root. Stops on a parent cycle in stored data instead of looping forever. */
function folderAncestry(startId: string | null | undefined, folderMap: Map<string, Folder>): string[] {
	const chain: string[] = [];
	const seen = new Set<string>();
	let curr = startId ?? undefined;
	while (curr && !seen.has(curr)) {
		seen.add(curr);
		chain.push(curr);
		curr = folderMap.get(curr)?.parent_id ?? undefined;
	}
	return chain;
}

/**
 * Renders the prompt-manager sidebar including search, folder hierarchy, root prompts, and drag-and-drop reorganization.
 *
 * The sidebar supports creating, renaming, and deleting folders and prompts (when permitted), selecting prompts, auto-expanding the folder that contains the selected prompt, filtering by search query, and dragging prompts between folders or to the root. Visual drag-over feedback and permission gating for create/update/delete actions are applied.
 *
 * @returns The sidebar React element containing the search input, folder list, root prompt drop zone, and drag-and-drop provider.
 */
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

export function PromptSidebar() {
	const {
		folders,
		prompts,
		selectedPromptId,
		handleSelectPrompt: onSelectPrompt,
		setFolderSheet,
		setDeleteFolderDialog,
		setPromptSheet,
		setDeletePromptDialog,
		setPromptAccessDialog,
		promptFilters,
		handleMovePrompt: onMovePrompt,
		canCreate,
		canUpdate,
		canDelete,
		selectedPrompt,
		toggleSidebar,
	} = usePromptContext();

	const { data: authStatus } = useIsAuthEnabledQuery();
	const isUserRole = isPromptMemberRole(authStatus?.role);

	const { data: teamsData, isError: teamsFailed, error: teamsError } = useGetTeamsQuery(undefined, { skip: isUserRole });
	const { data: sessionUsersData, isError: sessionUsersFailed, error: sessionUsersError } = useGetSessionUsersQuery(undefined, {
		skip: isUserRole,
	});

	const {
		data: sessionsData,
		isError: sessionsFailed,
		error: sessionsError,
	} = useGetSessionsQuery(selectedPrompt?.id ?? "", { skip: !selectedPrompt?.id });
	const sessions = sessionsData?.sessions ?? [];
	const [createSession] = useCreateSessionMutation();

	const [{ sessionId: selectedSessionId, versionId: selectedVersionId }, setUrlState] = useQueryStates(
		{
			sessionId: parseAsInteger,
			versionId: parseAsInteger,
		},
		{ history: "replace" },
	);

	const onManageAccess = useCallback(
		(prompt: Prompt) => {
			setPromptAccessDialog({ open: true, prompt });
		},
		[setPromptAccessDialog],
	);

	const onCreateFolder = useCallback((parentId?: string) => setFolderSheet({ open: true, parentId }), [setFolderSheet]);
	const onEditFolder = useCallback((folder: Folder) => setFolderSheet({ open: true, folder }), [setFolderSheet]);
	const onDeleteFolder = useCallback((folder: Folder) => setDeleteFolderDialog({ open: true, folder }), [setDeleteFolderDialog]);
	const onCreatePrompt = useCallback((folderId?: string) => setPromptSheet({ open: true, folderId }), [setPromptSheet]);
	const onEditPrompt = useCallback((prompt: Prompt) => setPromptSheet({ open: true, prompt }), [setPromptSheet]);
	const onDeletePrompt = useCallback((prompt: Prompt) => setDeletePromptDialog({ open: true, prompt }), [setDeletePromptDialog]);
	const [expandedFolders, setExpandedFolders] = useState<Set<string>>(new Set());
	const [searchQuery, setSearchQuery] = useState("");
	const [dragOverTarget, setDragOverTarget] = useState<string | null>(null);

	// Group prompts by folder, root prompts have no folder_id
	const { promptsByFolder, rootPrompts } = useMemo(() => {
		const map = new Map<string, Prompt[]>();
		const root: Prompt[] = [];
		for (const prompt of prompts) {
			if (!prompt.folder_id) {
				root.push(prompt);
			} else {
				const list = map.get(prompt.folder_id) || [];
				list.push(prompt);
				map.set(prompt.folder_id, list);
			}
		}
		return { promptsByFolder: map, rootPrompts: root };
	}, [prompts]);

	// Organize folders and subfolders
	const { rootFolders, childFoldersByParent, folderMap } = useMemo<{
		rootFolders: Folder[];
		childFoldersByParent: Map<string, Folder[]>;
		folderMap: Map<string, Folder>;
	}>(() => {
		const root: Folder[] = [];
		const childrenMap = new Map<string, Folder[]>();
		const fMap = new Map<string, Folder>();

		for (const folder of folders) {
			fMap.set(folder.id, folder);
			if (!folder.parent_id) {
				root.push(folder);
			} else {
				const list = childrenMap.get(folder.parent_id) || [];
				list.push(folder);
				childrenMap.set(folder.parent_id, list);
			}
		}

		// Sort root folders: Customers (1), Teams (2), Users (3), custom folders (5), Removed archives (10-12)
		const order = (f: Folder) => {
			if (f.name === "Customers" || f.type === "system_customers_root") return 1;
			if (f.name === "Teams" || f.type === "system_teams_root") return 2;
			if (f.name === "Users" || f.type === "system_users_root") return 3;
			if (f.name === "Removed Customers" || f.type === "system_removed_customers_root") return 10;
			if (f.name === "Removed Users" || f.type === "system_removed_users_root") return 11;
			if (f.name === "Removed Teams" || f.type === "system_removed_teams_root") return 12;
			if (f.name.startsWith("Removed") || f.type?.startsWith("archived_")) return 13;
			return 5;
		};
		root.sort((a, b) => order(a) - order(b) || a.name.localeCompare(b.name));

		return { rootFolders: root, childFoldersByParent: childrenMap, folderMap: fMap };
	}, [folders]);

	// Auto-expand the folder and all its parent folders containing the selected prompt
	useEffect(() => {
		if (!selectedPromptId) return;
		const prompt = prompts.find((p) => p.id === selectedPromptId);
		if (prompt?.folder_id) {
			const toExpand = new Set(folderAncestry(prompt.folder_id, folderMap));
			setExpandedFolders((prev) => {
				let changed = false;
				const next = new Set(prev);
				for (const id of toExpand) {
					if (!next.has(id)) {
						next.add(id);
						changed = true;
					}
				}
				return changed ? next : prev;
			});
		}
	}, [selectedPromptId, prompts, folderMap]);

	const toggleFolder = useCallback((folderId: string) => {
		setExpandedFolders((prev) => {
			const next = new Set(prev);
			if (next.has(folderId)) {
				next.delete(folderId);
			} else {
				next.add(folderId);
			}
			return next;
		});
	}, []);

	const hasActiveFilters = useMemo(() => {
		return (
			(promptFilters?.customer_ids?.length || 0) > 0 ||
			(promptFilters?.team_ids?.length || 0) > 0 ||
			(promptFilters?.user_ids?.length || 0) > 0
		);
	}, [promptFilters]);

	const isPromptMatchingFilters = useCallback(
		(p: Prompt) => {
			if (!hasActiveFilters || !promptFilters) return true;

			const cFilter = promptFilters.customer_ids || [];
			const tFilter = promptFilters.team_ids || [];
			const uFilter = promptFilters.user_ids || [];

			// 1. Match Customer
			if (cFilter.length > 0) {
				let matchesC = false;
				const pCusts = (p.customer_ids || "").split(",").map((s) => s.trim()).filter(Boolean);
				if (pCusts.some((cid) => cFilter.includes(cid))) {
					matchesC = true;
				}
				if (!matchesC && p.folder_id) {
					for (const fid of folderAncestry(p.folder_id, folderMap)) {
						const f = folderMap.get(fid);
						if (f && f.entity_id && cFilter.includes(f.entity_id)) {
							matchesC = true;
							break;
						}
					}
				}
				if (!matchesC && p.team_ids) {
					const pTeams = p.team_ids.split(",").map((s) => s.trim());
					for (const tid of pTeams) {
						const t = teamsData?.teams?.find((team) => team.id === tid);
						if (t?.customer_id && cFilter.includes(t.customer_id)) {
							matchesC = true;
							break;
						}
					}
				}
				if (!matchesC) return false;
			}

			// 2. Match Team
			if (tFilter.length > 0) {
				let matchesT = false;
				const pTeams = (p.team_ids || "").split(",").map((s) => s.trim()).filter(Boolean);
				if (pTeams.some((tid) => tFilter.includes(tid))) {
					matchesT = true;
				}
				if (!matchesT && p.folder_id) {
					for (const fid of folderAncestry(p.folder_id, folderMap)) {
						const f = folderMap.get(fid);
						if (f && f.entity_id && tFilter.includes(f.entity_id)) {
							matchesT = true;
							break;
						}
					}
				}
				if (!matchesT) return false;
			}

			// 3. Match User
			if (uFilter.length > 0) {
				let matchesU = false;
				const pUsers = (p.user_ids || "").split(",").map((s) => s.trim()).filter(Boolean);
				if (pUsers.some((uid) => uFilter.includes(uid))) {
					matchesU = true;
				}
				if (!matchesU && p.owner_user_id && uFilter.includes(p.owner_user_id)) {
					matchesU = true;
				}
				if (!matchesU && sessionUsersData) {
					for (const uid of uFilter) {
						const u = sessionUsersData.find((user) => user.id === uid);
						if (u?.allowed_prompt_repos) {
							const repos = u.allowed_prompt_repos.split(",").map((s) => s.trim());
							if (repos.includes(p.id)) {
								matchesU = true;
								break;
							}
						}
					}
				}
				if (!matchesU && p.folder_id) {
					for (const fid of folderAncestry(p.folder_id, folderMap)) {
						const f = folderMap.get(fid);
						if (f && f.entity_id && uFilter.includes(f.entity_id)) {
							matchesU = true;
							break;
						}
					}
				}
				if (!matchesU) return false;
			}

			return true;
		},
		[hasActiveFilters, promptFilters, folderMap, teamsData, sessionUsersData],
	);

	// Filter folders and prompts based on search and active filters
	const filteredData = useMemo(() => {
		if (!searchQuery.trim() && !hasActiveFilters) {
			return { folders, rootFolders, childFoldersByParent, promptsByFolder, rootPrompts };
		}

		const query = searchQuery.trim().toLowerCase();
		const matchedFolderIds = new Set<string>();
		const filteredPromptsByFolder = new Map<string, Prompt[]>();
		const filteredRootPrompts: Prompt[] = [];

		for (const prompt of prompts) {
			if (!isPromptMatchingFilters(prompt)) continue;
			if (query && !prompt.name.toLowerCase().includes(query)) continue;

			if (!prompt.folder_id) {
				filteredRootPrompts.push(prompt);
			} else {
				for (const id of folderAncestry(prompt.folder_id, folderMap)) matchedFolderIds.add(id);
				const list = filteredPromptsByFolder.get(prompt.folder_id) || [];
				list.push(prompt);
				filteredPromptsByFolder.set(prompt.folder_id, list);
			}
		}

		if (query) {
			for (const folder of folders) {
				if (folder.name.toLowerCase().includes(query)) {
					for (const id of folderAncestry(folder.id, folderMap)) matchedFolderIds.add(id);
				}
			}
		}

		const filteredFolders = folders.filter((folder) => matchedFolderIds.has(folder.id));
		const filteredRootFolders = rootFolders.filter((folder) => matchedFolderIds.has(folder.id));

		return {
			folders: filteredFolders,
			rootFolders: filteredRootFolders,
			childFoldersByParent,
			promptsByFolder: filteredPromptsByFolder,
			rootPrompts: filteredRootPrompts,
		};
	}, [
		folders,
		rootFolders,
		childFoldersByParent,
		folderMap,
		prompts,
		promptsByFolder,
		rootPrompts,
		searchQuery,
		hasActiveFilters,
		isPromptMatchingFilters,
	]);

	// Prompt lookup for drag events
	const promptMap = useMemo(() => {
		const map = new Map<string, Prompt>();
		for (const p of prompts) map.set(p.id, p);
		return map;
	}, [prompts]);

	if (isUserRole) {
		return (
			<div className="flex h-full flex-col">
				<div className="flex items-center justify-between border-b p-3">
					<div>
						<span className="text-sm font-semibold">Your prompts</span>
						<p className="text-muted-foreground mt-0.5 text-xs">Select a prompt assigned by your admin to start chatting.</p>
					</div>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="ghost"
								size="icon"
								className="h-7 w-7 shrink-0 text-muted-foreground hover:text-foreground"
								onClick={toggleSidebar}
								data-testid="user-sidebar-collapse-btn"
								aria-label="Collapse sidebar"
							>
								<PanelLeftClose className="h-4 w-4" />
							</Button>
						</TooltipTrigger>
						<TooltipContent side="right">Collapse sidebar</TooltipContent>
					</Tooltip>
				</div>
				<ScrollArea className="max-h-[45%] shrink-0 overflow-y-auto border-b" viewportClassName="no-table">
					<div className="flex flex-col gap-1 p-2 px-3">
						{prompts.length === 0 ? (
							<div className="text-muted-foreground py-6 text-center text-xs">
								No prompts assigned. Ask your admin to allow prompt repositories on your user.
							</div>
						) : (
							prompts.map((prompt) => {
								const folder = prompt.folder_id ? folderMap.get(prompt.folder_id) : undefined;
								const parentFolder = folder?.parent_id ? folderMap.get(folder.parent_id) : undefined;
								const folderPath = parentFolder && !parentFolder.type?.startsWith("system_")
									? `${parentFolder.name} / ${folder?.name}`
									: folder?.name;
								return (
									<button
										key={prompt.id}
										type="button"
										onClick={() => onSelectPrompt(prompt.id)}
										data-testid={`user-prompt-${prompt.id}`}
										className={cn(
											"flex w-full items-center justify-between gap-2 rounded-md px-3 py-2 text-left text-sm transition-all hover:bg-accent",
											selectedPromptId === prompt.id ? "bg-accent font-medium text-foreground" : "text-muted-foreground",
										)}
									>
										<div className="flex min-w-0 items-center gap-2">
											<FileText className="size-4 shrink-0" />
											<span className="truncate">{prompt.name}</span>
										</div>
										{folderPath && (
											<span className="bg-muted text-muted-foreground shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium max-w-[120px] truncate" title={folderPath}>
												{folderPath}
											</span>
										)}
									</button>
								);
							})
						)}
					</div>
				</ScrollArea>

				<div className="flex items-center justify-between border-b p-3">
					<span className="truncate text-sm font-semibold">Chat History</span>
					<Button
						variant="outline"
						size="sm"
						disabled={!selectedPrompt}
						onClick={async () => {
							if (!selectedPrompt) return;
							try {
								const latestVersion = selectedPrompt.latest_version;
								const sessionData: CreateSessionRequest = latestVersion?.id
									? // Server copies provider/model/params/messages from the committed version.
										{ version_id: latestVersion.id }
									: {
											messages: Message.serializeForStorage([
												Message.fromLegacyAll((latestVersion?.messages ?? []).map((m) => m.message)).find(
													(m) => m.role === "system",
												) || Message.system(""),
											]),
											model_params: { stream: true },
											provider: latestVersion?.provider ?? "",
											model: latestVersion?.model ?? "",
										};

								const result = await createSession({
									promptId: selectedPrompt.id,
									data: sessionData,
								}).unwrap();
								setUrlState({ sessionId: result.session.id, versionId: null });
								toast.success("New chat started");
							} catch (err) {
								toast.error("Failed to start new chat");
							}
						}}
						className="h-8 gap-1.5"
					>
						<Plus className="h-4 w-4" />
						New Chat
					</Button>
				</div>
				<ScrollArea className="grow overflow-y-auto" viewportClassName="no-table viewport-table-height-full">
					<div className="flex flex-col gap-1 p-2 px-3">
						{sessionsFailed ? (
							<QueryErrorBanner
								className="mb-2"
								testId="prompt-sidebar-sessions-query-error"
								message={getErrorMessage(sessionsError) || "Failed to load chat sessions."}
							/>
						) : null}
						{!selectedPrompt ? (
							<div className="text-muted-foreground py-8 text-center text-sm">Select a prompt above first</div>
						) : sessions.length === 0 && !sessionsFailed ? (
							<div className="text-muted-foreground py-8 text-center text-sm">No chat history yet</div>
						) : (
							[...sessions].reverse().map((session) => (
								<button
									key={session.id}
									type="button"
									onClick={() => setUrlState({ sessionId: session.id, versionId: null })}
									className={cn(
										"flex w-full items-center justify-between rounded-md px-3 py-2 text-left text-sm transition-all hover:bg-accent",
										selectedSessionId === session.id ? "bg-accent font-medium text-foreground" : "text-muted-foreground",
									)}
								>
									<span className="max-w-[150px] truncate">{session.name || formatSessionDate(session.created_at)}</span>
								</button>
							))
						)}
					</div>
				</ScrollArea>
			</div>
		);
	}

	return (
		<DragDropProvider
			onDragOver={(event) => {
				if (!canUpdate) return;
				const targetId = event.operation.target?.id as string | undefined;
				setDragOverTarget(targetId ?? null);
			}}
			onDragEnd={(event) => {
				setDragOverTarget(null);
				if (!canUpdate) return;
				if (event.canceled || !onMovePrompt) return;

				const sourceId = event.operation.source?.id as string | undefined;
				const targetId = event.operation.target?.id as string | undefined;
				if (!sourceId || !targetId) return;

				const promptId = sourceId.startsWith("prompt-") ? sourceId.slice(7) : null;
				if (!promptId) return;

				const prompt = promptMap.get(promptId);
				if (!prompt) return;

				let targetFolderId: string | null = null;
				if (targetId === "root-drop-zone") {
					targetFolderId = null;
				} else if (targetId.startsWith("folder-")) {
					targetFolderId = targetId.slice(7);
				} else {
					return;
				}
				if ((prompt.folder_id ?? null) === targetFolderId) return;
				onMovePrompt(promptId, targetFolderId);
			}}
		>
			<div className="flex h-full flex-col">
				{/* Search */}
				<div className="flex items-center gap-2 border-b p-3">
					<div className="relative grow">
						<Search className="text-muted-foreground absolute top-1/2 left-2.5 h-4 w-4 -translate-y-1/2" />
						<Input
							placeholder="Search prompts..."
							value={searchQuery}
							onChange={(e) => setSearchQuery(e.target.value)}
							data-testid="sidebar-search"
							className="h-8 pl-8"
						/>
					</div>
					{canCreate && (
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									variant="outline"
									className="h-8 w-8 shrink-0 bg-transparent"
									data-testid="sidebar-create-menu"
									aria-label="Create prompt or folder"
								>
									<PlusIcon className="h-3.5 w-3.5" />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								<DropdownMenuItem
									data-testid="sidebar-create-prompt"
									onClick={(e) => {
										e.stopPropagation();
										onCreatePrompt();
									}}
								>
									New Prompt
								</DropdownMenuItem>
								<DropdownMenuItem
									data-testid="sidebar-create-folder"
									onClick={(e) => {
										e.stopPropagation();
										onCreateFolder();
									}}
								>
									New Folder
								</DropdownMenuItem>
							</DropdownMenuContent>
						</DropdownMenu>
					)}
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="ghost"
								size="icon"
								className="h-8 w-8 shrink-0 text-muted-foreground hover:text-foreground"
								onClick={toggleSidebar}
								data-testid="admin-sidebar-collapse-btn"
								aria-label="Collapse sidebar"
							>
								<PanelLeftClose className="h-4 w-4" />
							</Button>
						</TooltipTrigger>
						<TooltipContent side="right">Collapse sidebar</TooltipContent>
					</Tooltip>
				</div>

				<ScrollArea className="grow overflow-y-auto" viewportClassName="no-table viewport-table-height-full">
					<div className="flex flex-col p-2 px-3">
						{hasActiveFilters && !isUserRole && (teamsFailed || sessionUsersFailed) ? (
							<QueryErrorBanner
								className="mb-2"
								testId="prompt-sidebar-filter-query-error"
								message={
									getErrorMessage(teamsError || sessionUsersError) ||
									"Failed to load team or user data for filters. Results may be incomplete."
								}
							/>
						) : null}
						{filteredData.folders.length === 0 && filteredData.rootPrompts.length === 0 ? (
							<div className="text-muted-foreground py-8 text-center text-sm">{searchQuery ? "No results found" : "No prompts yet"}</div>
						) : (
							<>
								{filteredData.rootFolders.map((folder) => (
									<DroppableFolder
										key={folder.id}
										folder={folder}
										prompts={filteredData.promptsByFolder.get(folder.id) || promptsByFolder.get(folder.id) || []}
										childFolders={filteredData.childFoldersByParent.get(folder.id) || []}
										allChildFoldersByParent={filteredData.childFoldersByParent}
										allPromptsByFolder={filteredData.promptsByFolder}
										expandedFolders={expandedFolders}
										searchQuery={searchQuery}
										dragOverTarget={dragOverTarget}
										selectedPromptId={selectedPromptId}
										onToggle={toggleFolder}
										onSelectPrompt={onSelectPrompt}
										onEdit={onEditFolder}
										onDelete={onDeleteFolder}
										onCreateFolder={onCreateFolder}
										onCreatePrompt={onCreatePrompt}
										onEditPrompt={onEditPrompt}
										onDeletePrompt={onDeletePrompt}
										onManageAccess={onManageAccess}
										canCreate={canCreate}
										canUpdate={canUpdate}
										canDelete={canDelete}
										isUserRole={isUserRole}
									/>
								))}
								<RootDropZone
									isDragOver={dragOverTarget === "root-drop-zone"}
									rootPrompts={filteredData.rootPrompts}
									selectedPromptId={selectedPromptId}
									onSelectPrompt={onSelectPrompt}
									onEditPrompt={onEditPrompt}
									onDeletePrompt={onDeletePrompt}
									onManageAccess={onManageAccess}
									canUpdate={canUpdate}
									canDelete={canDelete}
									isUserRole={isUserRole}
								/>
							</>
						)}
					</div>
				</ScrollArea>
			</div>
		</DragDropProvider>
	);
}

interface RootDropZoneProps {
	isDragOver: boolean;
	rootPrompts: Prompt[];
	selectedPromptId?: string | null;
	onSelectPrompt: (promptId: string) => void;
	onEditPrompt: (prompt: Prompt) => void;
	onDeletePrompt: (prompt: Prompt) => void;
	onManageAccess?: (prompt: Prompt) => void;
	canUpdate: boolean;
	canDelete: boolean;
	isUserRole?: boolean;
}

function RootDropZone({
	isDragOver,
	rootPrompts,
	selectedPromptId,
	onSelectPrompt,
	onEditPrompt,
	onDeletePrompt,
	onManageAccess,
	canUpdate,
	canDelete,
	isUserRole,
}: RootDropZoneProps) {
	const { ref } = useDroppable({ id: "root-drop-zone" });

	return (
		<div ref={ref} className={cn("min-h-[8px] grow rounded-sm transition-colors", isDragOver && "bg-primary/10 ring-primary/30 ring-1")}>
			{rootPrompts.map((prompt) => (
				<DraggablePromptItem
					key={prompt.id}
					prompt={prompt}
					isSelected={selectedPromptId === prompt.id}
					onSelect={() => onSelectPrompt(prompt.id)}
					onEdit={() => onEditPrompt(prompt)}
					onDelete={() => onDeletePrompt(prompt)}
					onManageAccess={onManageAccess}
					canUpdate={canUpdate}
					canDelete={canDelete}
					isUserRole={isUserRole}
				/>
			))}
		</div>
	);
}

interface DroppableFolderProps {
	folder: Folder;
	prompts: Prompt[];
	childFolders?: Folder[];
	allChildFoldersByParent: Map<string, Folder[]>;
	allPromptsByFolder: Map<string, Prompt[]>;
	expandedFolders: Set<string>;
	searchQuery: string;
	dragOverTarget: string | null;
	selectedPromptId?: string | null;
	level?: number;
	onToggle: (folderId: string) => void;
	onSelectPrompt: (promptId: string) => void;
	onEdit: (folder: Folder) => void;
	onDelete: (folder: Folder) => void;
	onCreateFolder: (parentId?: string) => void;
	onCreatePrompt: (folderId?: string) => void;
	onEditPrompt: (prompt: Prompt) => void;
	onDeletePrompt: (prompt: Prompt) => void;
	onManageAccess?: (prompt: Prompt) => void;
	canCreate: boolean;
	canUpdate: boolean;
	canDelete: boolean;
	isUserRole?: boolean;
}

function getRecursivePromptCount(
	folderId: string,
	promptsMap: Map<string, Prompt[]>,
	childrenMap: Map<string, Folder[]>,
): number {
	let count = (promptsMap.get(folderId) || []).length;
	const children = childrenMap.get(folderId) || [];
	for (const child of children) {
		count += getRecursivePromptCount(child.id, promptsMap, childrenMap);
	}
	return count;
}

function DroppableFolder({
	folder,
	prompts,
	childFolders,
	allChildFoldersByParent,
	allPromptsByFolder,
	expandedFolders,
	searchQuery,
	dragOverTarget,
	selectedPromptId,
	level = 0,
	onToggle,
	onSelectPrompt,
	onEdit,
	onDelete,
	onCreateFolder,
	onCreatePrompt,
	onEditPrompt,
	onDeletePrompt,
	onManageAccess,
	canCreate,
	canUpdate,
	canDelete,
	isUserRole,
}: DroppableFolderProps) {
	const { ref } = useDroppable({ id: `folder-${folder.id}` });
	const isExpanded = expandedFolders.has(folder.id) || !!searchQuery;
	const isDragOver = dragOverTarget === `folder-${folder.id}`;

	const isSystemFolder =
		folder.type?.startsWith("system_") ||
		folder.name === "Users" ||
		folder.name === "Customers" ||
		folder.name === "Teams" ||
		folder.name === "Removed Users" ||
		folder.name === "Removed Customers" ||
		folder.name === "Removed Teams";

	const isArchiveFolder = folder.type?.startsWith("archived_") || folder.name.startsWith("Removed");
	const isCustomersFolder = folder.type === "system_customers_root" || folder.name === "Customers";
	const isTeamsFolder = folder.type === "system_teams_root" || folder.name === "Teams";
	const isUsersFolder = folder.type === "system_users_root" || folder.name === "Users";

	const showActions = canCreate || canUpdate || canDelete;
	const totalCount = getRecursivePromptCount(folder.id, allPromptsByFolder, allChildFoldersByParent);

	return (
		<div ref={ref} className="mb-0.5 last:mb-0">
			<div
				className={cn(
					"hover:bg-muted/50 group relative flex h-[30px] cursor-pointer items-center gap-1 rounded-sm px-2 transition-colors",
					isDragOver && "bg-primary/10 ring-primary/30 ring-1",
					isArchiveFolder && "opacity-80 text-muted-foreground",
				)}
				onClick={() => onToggle(folder.id)}
				data-testid={`sidebar-folder-${folder.id}`}
			>
				<button
					type="button"
					className="flex shrink-0 items-center"
					aria-label="Toggle folder"
					onClick={(e) => {
						e.stopPropagation();
						onToggle(folder.id);
					}}
				>
					{isExpanded ? (
						<ChevronDown className="text-muted-foreground h-4 w-4" />
					) : (
						<ChevronRight className="text-muted-foreground h-4 w-4" />
					)}
				</button>
				{isArchiveFolder ? (
					<Archive className="text-muted-foreground h-4 w-4 shrink-0" />
				) : isCustomersFolder ? (
					<Building className="text-primary h-4 w-4 shrink-0" />
				) : isTeamsFolder ? (
					<Users className="text-primary h-4 w-4 shrink-0" />
				) : isUsersFolder ? (
					<UserCheck className="text-primary h-4 w-4 shrink-0" />
				) : isExpanded ? (
					<FolderOpen className="text-muted-foreground h-4 w-4 shrink-0" />
				) : (
					<FolderIcon className="text-muted-foreground h-4 w-4 shrink-0" />
				)}
				<span className="flex-1 truncate text-sm font-medium">{folder.name}</span>
				<span className="text-muted-foreground mr-1 shrink-0 text-xs">{totalCount}</span>
				{showActions && (
					<DropdownMenu>
						<DropdownMenuTrigger asChild onClick={(e) => e.stopPropagation()} className="bg-card absolute top-1/2 right-2 -translate-y-1/2">
							<Button
								variant="ghost"
								size="icon"
								className="h-6 w-6 shrink-0 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 focus-visible:opacity-100"
								data-testid={`sidebar-folder-actions-${folder.id}`}
								aria-label="Folder actions"
							>
								<MoreHorizontal className="h-4 w-4" />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							{canCreate && !isArchiveFolder && (
								<>
									<DropdownMenuItem
										data-testid="folder-create-prompt"
										onClick={(e) => {
											e.stopPropagation();
											onCreatePrompt(folder.id);
										}}
									>
										<Plus className="mr-2 h-4 w-4" />
										New Prompt
									</DropdownMenuItem>
									<DropdownMenuItem
										data-testid="folder-create-subfolder"
										onClick={(e) => {
											e.stopPropagation();
											onCreateFolder(folder.id);
										}}
									>
										<FolderIcon className="mr-2 h-4 w-4" />
										New Subfolder
									</DropdownMenuItem>
								</>
							)}
							{(canUpdate || canDelete) && <DropdownMenuSeparator />}
							{canUpdate && (
								<DropdownMenuItem
									data-testid="folder-action-edit"
									onClick={(e) => {
										e.stopPropagation();
										onEdit(folder);
									}}
								>
									<Pencil className="mr-2 h-4 w-4" />
									Edit Folder
								</DropdownMenuItem>
							)}
							{canDelete && (
								<DropdownMenuItem
									variant="destructive"
									data-testid="folder-action-delete"
									onClick={(e) => {
										e.stopPropagation();
										onDelete(folder);
									}}
								>
									<Trash2 className="mr-2 h-4 w-4" />
									Delete Folder
								</DropdownMenuItem>
							)}
						</DropdownMenuContent>
					</DropdownMenu>
				)}
			</div>

			{isExpanded && (
				<div className={cn("ml-3 border-l border-border/40 pl-1.5 flex flex-col gap-0.5", level > 0 && "ml-2.5 pl-1")}>
					{childFolders && childFolders.map((child) => (
						<DroppableFolder
							key={child.id}
							folder={child}
							prompts={allPromptsByFolder.get(child.id) || []}
							childFolders={allChildFoldersByParent.get(child.id) || []}
							allChildFoldersByParent={allChildFoldersByParent}
							allPromptsByFolder={allPromptsByFolder}
							expandedFolders={expandedFolders}
							searchQuery={searchQuery}
							dragOverTarget={dragOverTarget}
							selectedPromptId={selectedPromptId}
							level={level + 1}
							onToggle={onToggle}
							onSelectPrompt={onSelectPrompt}
							onEdit={onEdit}
							onDelete={onDelete}
							onCreateFolder={onCreateFolder}
							onCreatePrompt={onCreatePrompt}
							onEditPrompt={onEditPrompt}
							onDeletePrompt={onDeletePrompt}
							onManageAccess={onManageAccess}
							canCreate={canCreate}
							canUpdate={canUpdate}
							canDelete={canDelete}
							isUserRole={isUserRole}
						/>
					))}
					{prompts.map((prompt) => (
						<DraggablePromptItem
							key={prompt.id}
							prompt={prompt}
							isSelected={selectedPromptId === prompt.id}
							onSelect={() => onSelectPrompt(prompt.id)}
							onEdit={() => onEditPrompt(prompt)}
							onDelete={() => onDeletePrompt(prompt)}
							onManageAccess={onManageAccess}
							canUpdate={canUpdate}
							canDelete={canDelete}
							isUserRole={isUserRole}
						/>
					))}
					{(!childFolders || childFolders.length === 0) && prompts.length === 0 && (
						<div className="text-muted-foreground py-1 pl-4 text-xs">{isDragOver ? "Drop here" : "Empty"}</div>
					)}
				</div>
			)}
		</div>
	);
}

interface DraggablePromptItemProps {
	prompt: Prompt;
	isSelected: boolean;
	onSelect: () => void;
	onEdit: () => void;
	onDelete: () => void;
	onManageAccess?: (prompt: Prompt) => void;
	canUpdate: boolean;
	canDelete: boolean;
	isUserRole?: boolean;
}

function DraggablePromptItem({
	prompt,
	isSelected,
	onSelect,
	onEdit,
	onDelete,
	onManageAccess,
	canUpdate,
	canDelete,
	isUserRole,
}: DraggablePromptItemProps) {
	const { ref, isDragging } = useDraggable({
		id: `prompt-${prompt.id}`,
		disabled: !canUpdate,
	});
	const showActions = canUpdate || canDelete || (!isUserRole && !!onManageAccess);

	return (
		<div
			ref={ref}
			data-testid={`sidebar-prompt-${prompt.id}`}
			className={cn(
				"group mb-1 flex h-[30px] cursor-pointer items-center gap-2 rounded-sm px-2 last:mb-0",
				isSelected ? "bg-primary/10 text-primary" : "hover:bg-muted/50",
				isDragging && "opacity-50",
			)}
			onClick={() => {
				// Don't navigate if this was a drag
				if (isDragging) return;
				onSelect();
			}}
		>
			<FileText className="h-4 w-4 shrink-0" />
			<span className="flex-1 truncate text-sm">{prompt.name}</span>
			{showActions && (
				<DropdownMenu>
					<DropdownMenuTrigger asChild onClick={(e) => e.stopPropagation()}>
						<Button
							variant="ghost"
							size="icon"
							className="h-6 w-6 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 focus-visible:opacity-100"
							data-testid={`sidebar-prompt-actions-${prompt.id}`}
							aria-label="Prompt actions"
						>
							<MoreHorizontal className="h-4 w-4" />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						{!isUserRole && onManageAccess && (
							<DropdownMenuItem
								className="cursor-pointer"
								data-testid={`prompt-action-access-${prompt.id}`}
								onClick={(e) => {
									e.stopPropagation();
									onManageAccess(prompt);
								}}
							>
								<ShieldCheck className="h-4 w-4" />
								Manage Access
							</DropdownMenuItem>
						)}
						{canUpdate && (
							<DropdownMenuItem
								className="cursor-pointer"
								data-testid="prompt-action-rename"
								onClick={(e) => {
									e.stopPropagation();
									onEdit();
								}}
							>
								<Pencil className="h-4 w-4" />
								Rename
							</DropdownMenuItem>
						)}
						{canDelete && (
							<DropdownMenuItem
								variant="destructive"
								className="cursor-pointer"
								data-testid="prompt-action-delete"
								onClick={(e) => {
									e.stopPropagation();
									onDelete();
								}}
							>
								<Trash2 className="h-4 w-4" />
								Delete
							</DropdownMenuItem>
						)}
					</DropdownMenuContent>
				</DropdownMenu>
			)}
		</div>
	);
}