import { useEffect, useMemo, useRef, useState } from "react";
import {
	Users,
	User,
	Plus,
	Search,
	Edit2,
	Trash2,
	Shield,
	ShieldCheck,
	Check,
	X,
	Clock,
	Key,
	DollarSign,
	Activity,
	ChevronDown,
	ChevronRight,
	Loader2,
	UserX,
	Ban,
	Eye,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { DataTablePagination } from "@/components/table/dataTablePagination";
import { Checkbox } from "@/components/ui/checkbox";
import { toast } from "sonner";
import {
	WORKSPACE_ACCESS_SECTIONS,
	adminSectionsForForm,
	adminSectionsFromStorage,
	allWorkspaceParentGrants,
	allowedSectionsToString,
	isUnrestrictedAdminSelection,
	itemGrantKey,
	sectionSelectionState,
	type WorkspaceGrantKey,
	type WorkspaceSection,
	type WorkspaceSectionKey,
} from "@/lib/constants/workspaceSections";
import {
	getErrorMessage,
	useApproveSessionUserMutation,
	useCreateSessionUserMutation,
	useDeleteSessionUserMutation,
	useBlockSessionUserMutation,
	useUnblockSessionUserMutation,
	useGetPromptsQuery,
	useGetSessionUsersQuery,
	useGetProductUserQuotaQuery,
	useIsAuthEnabledQuery,
	useRejectSessionUserMutation,
	useUpdateSessionUserMutation,
	useGetTeamsQuery,
	useGetUserTeamsQuery,
	useAddTeamMemberMutation,
	useRemoveTeamMemberMutation,
	useGetVirtualKeysQuery,
	type SessionUser,
} from "@/lib/store";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import { useGetRolesQuery } from "@enterprise/lib/store/apis/rbacApi";
import {
	useDeleteVirtualKeyUserMutation,
	useGetUserVirtualKeysQuery,
	useSetVirtualKeyUserMutation,
} from "@enterprise/lib/store/apis/virtualKeyUsersApi";
import { cn } from "@/lib/utils";
import { describeTeamVirtualKey, virtualKeysForTeam } from "@/lib/utils/governance";

function IndeterminateCheckbox({
	checked,
	indeterminate,
	onChange,
	disabled,
}: {
	checked: boolean;
	indeterminate: boolean;
	onChange: (checked: boolean) => void;
	disabled?: boolean;
}) {
	const ref = useRef<HTMLInputElement>(null);
	useEffect(() => {
		if (ref.current) {
			ref.current.indeterminate = indeterminate && !checked;
		}
	}, [indeterminate, checked]);
	return (
		<input
			ref={ref}
			type="checkbox"
			checked={checked}
			disabled={disabled}
			onChange={(e) => onChange(e.target.checked)}
			className="border-border rounded text-teal-500 focus:ring-teal-500/50 disabled:cursor-not-allowed disabled:opacity-70"
		/>
	);
}

// Server-side: admin sees every prompt; sub_admin and user can have scoped virtual keys and prompt repos.
function isWorkspaceAdminRole(role: string): boolean {
	return role === "admin";
}

// Section grants apply to all roles (user, sub_admin, admin)
function sectionGrantsApply(_role: string): boolean {
	return true;
}

function isUserRole(role: string): boolean {
	return role === "user";
}

function canAssignTeamAndKey(role: string): boolean {
	return role !== "admin";
}

function UserTeamCell({ userId }: { userId: string }) {
	const { data, isError } = useGetUserTeamsQuery(userId);
	if (isError) {
		return <span className="text-destructive text-xs italic">Failed to load</span>;
	}
	const team = data?.teams?.[0];
	if (!team) {
		return <span className="text-muted-foreground text-xs italic">No Team (Standalone)</span>;
	}
	return (
		<div className="flex flex-col">
			<span className="text-sm font-medium">{team.name}</span>
			{team.customer_name ? (
				<span className="text-muted-foreground text-xs">Customer: {team.customer_name}</span>
			) : (
				<span className="text-muted-foreground text-[11px]">Direct Team</span>
			)}
		</div>
	);
}

function virtualKeySourceLabel(vk: { origin?: string; origin_name?: string }) {
	if ((vk.origin === "team" || vk.origin === "customer") && vk.origin_name) {
		return `${vk.origin === "team" ? "Team" : "Customer"}: ${vk.origin_name}`;
	}
	return null;
}

function VirtualKeyName({ vk }: { vk: { name: string; origin?: string; origin_name?: string } }) {
	const source = virtualKeySourceLabel(vk);
	return (
		<span className="text-sm">
			{vk.name}
			{source ? <span className="text-muted-foreground text-xs"> ({source})</span> : null}
		</span>
	);
}

// One key stays on the row. Several keys open a scrollable list of name and customer/team only.
function UserVirtualKeysCell({ userId }: { userId: string }) {
	const { data, isError } = useGetUserVirtualKeysQuery(userId);
	if (isError) {
		return <span className="text-destructive text-xs italic">Failed to load</span>;
	}
	const keys = (data?.virtual_keys ?? []).filter((vk) => vk.is_active !== false);
	if (keys.length === 0) {
		return <span className="text-muted-foreground text-xs italic">No key</span>;
	}
	if (keys.length === 1) {
		return <VirtualKeyName vk={keys[0]} />;
	}
	return (
		<Popover>
			<PopoverTrigger asChild>
				<button
					type="button"
					className="text-foreground hover:text-teal-500 cursor-pointer text-sm underline-offset-2 hover:underline"
					data-testid={`user-vk-list-${userId}`}
				>
					{keys.length} keys
				</button>
			</PopoverTrigger>
			<PopoverContent align="start" className="w-80 p-2">
				<div className="max-h-48 space-y-1.5 overflow-y-auto pr-1">
					{keys.map((vk) => (
						<div key={vk.id} className="px-1 py-1">
							<VirtualKeyName vk={vk} />
						</div>
					))}
				</div>
			</PopoverContent>
		</Popover>
	);
}

export type TabFilter = "allowed" | "all" | "user" | "admin" | "sub_admin" | "pending" | "rejected" | "blocked";

export default function UsersView() {
	const [searchQuery, setSearchQuery] = useState("");
	const [roleFilter, setRoleFilter] = useState<TabFilter>("all");
	const [actionBusyId, setActionBusyId] = useState<string | null>(null);
	const hasCreateAccess = useRbac(RbacResource.Users, RbacOperation.Create);
	const hasUpdateAccess = useRbac(RbacResource.Users, RbacOperation.Update);
	const hasDeleteAccess = useRbac(RbacResource.Users, RbacOperation.Delete);

	const [usersPollMs, setUsersPollMs] = useState(15_000);
	const {
		data: users = [],
		isLoading: loading,
		isError: usersError,
		error: usersErrorDetail,
		refetch: refetchUsers,
	} = useGetSessionUsersQuery(undefined, { pollingInterval: usersPollMs, refetchOnFocus: true });
	const { data: authStatus } = useIsAuthEnabledQuery();
	const currentUserId = authStatus?.user_id;
	const currentUsername = (authStatus?.username || "").trim().toLowerCase();
	const currentUserEmail = (authStatus?.email || "").trim().toLowerCase();
	const { data: userQuota } = useGetProductUserQuotaQuery(undefined, { pollingInterval: usersPollMs, refetchOnFocus: true });
	useEffect(() => {
		setUsersPollMs(usersError ? 0 : 15_000);
	}, [usersError]);
	const { data: promptsData, isError: promptsFailed, error: promptsError } = useGetPromptsQuery();
	const { data: teamsData, isError: teamsFailed, error: teamsError } = useGetTeamsQuery({ limit: 500, offset: 0 });
	const teams = teamsData?.teams || [];
	const { data: virtualKeysData, isError: vkFailed, error: vkError } = useGetVirtualKeysQuery({ limit: 500, offset: 0 });
	const virtualKeys = (virtualKeysData?.virtual_keys || []).filter((vk) => vk.is_active !== false);
	const { data: rolesData, isError: rolesFailed, error: rolesError } = useGetRolesQuery();
	const helperQueryFailed = teamsFailed || vkFailed || rolesFailed || promptsFailed;
	const helperQueryError = teamsError || vkError || rolesError || promptsError;
	const roleOptions = (rolesData?.roles || []).slice().sort((a, b) => {
		const rank = (name: string) => (name === "admin" ? 0 : name === "user" ? 1 : 2);
		const diff = rank(a.name) - rank(b.name);
		return diff !== 0 ? diff : a.name.localeCompare(b.name);
	});
	const [createUser] = useCreateSessionUserMutation();
	const [updateUser] = useUpdateSessionUserMutation();
	const [deleteUser] = useDeleteSessionUserMutation();
	const [approveUser] = useApproveSessionUserMutation();
	const [rejectUser] = useRejectSessionUserMutation();
	const [blockUser] = useBlockSessionUserMutation();
	const [unblockUser] = useUnblockSessionUserMutation();
	const [addTeamMember] = useAddTeamMemberMutation();
	const [removeTeamMember] = useRemoveTeamMemberMutation();
	const [setVirtualKeyUser] = useSetVirtualKeyUserMutation();
	const [deleteVirtualKeyUser] = useDeleteVirtualKeyUserMutation();
	const allPrompts = promptsData?.prompts || [];

	// Dialog states
	const [isCreateOpen, setIsCreateOpen] = useState(false);
	const [isEditOpen, setIsEditOpen] = useState(false);
	const [isDeleteOpen, setIsDeleteOpen] = useState(false);
	const [selectedUser, setSelectedUser] = useState<SessionUser | null>(null);
	const [isCreating, setIsCreating] = useState(false);
	const [isUpdating, setIsUpdating] = useState(false);
	const [isDeleting, setIsDeleting] = useState(false);

	// Form states — Role (permission) and Team (org membership) are separate.
	const [username, setUsername] = useState("");
	const [email, setEmail] = useState("");
	const [sendTemporaryPassword, setSendTemporaryPassword] = useState(false);
	const [role, setRole] = useState("user");
	const [teamId, setTeamId] = useState("");
	const [initialTeamId, setInitialTeamId] = useState("");
	const [virtualKeyId, setVirtualKeyId] = useState("");
	const [initialVirtualKeyId, setInitialVirtualKeyId] = useState("");
	const [budget, setBudget] = useState(0);
	const [rateLimit, setRateLimit] = useState(0);
	const [allowedPromptRepos, setAllowedPromptRepos] = useState("");
	const [allowedSections, setAllowedSections] = useState<Set<WorkspaceGrantKey>>(
		() => new Set<WorkspaceGrantKey>(["prompt-repository"]),
	);
	const [expandedSections, setExpandedSections] = useState<Set<WorkspaceSectionKey>>(new Set());
	const [workspaceSearch, setWorkspaceSearch] = useState("");

	const {
		data: editUserTeams,
		isError: editTeamsFailed,
		error: editTeamsError,
	} = useGetUserTeamsQuery(selectedUser?.id || "", {
		skip: !selectedUser?.id || !isEditOpen,
	});
	const {
		data: editUserVKs,
		isError: editVksFailed,
		error: editVksError,
	} = useGetUserVirtualKeysQuery(selectedUser?.id || "", {
		skip: !selectedUser?.id || !isEditOpen,
	});
	const editDialogQueryFailed = editTeamsFailed || editVksFailed;
	const editDialogQueryError = editTeamsError || editVksError;

	useEffect(() => {
		if (!isEditOpen || !selectedUser?.id) return;
		const current = editUserTeams?.teams?.[0]?.id || "";
		setTeamId(current);
		setInitialTeamId(current);
	}, [isEditOpen, selectedUser?.id, editUserTeams]);

	useEffect(() => {
		if (!isEditOpen || !selectedUser?.id) return;
		// Team/customer keys are inherited, not assigned to the user — preselecting one would
		// make "No Virtual Key" look like it removes a key it can't remove.
		const current = editUserVKs?.virtual_keys?.find((vk) => (vk.origin ?? "direct") === "direct")?.id || "";
		setVirtualKeyId(current);
		setInitialVirtualKeyId(current);
	}, [isEditOpen, selectedUser?.id, editUserVKs]);

	// The picker edits one direct key; any further direct keys are left untouched on save.
	const otherDirectVirtualKeys = isEditOpen
		? (editUserVKs?.virtual_keys ?? []).filter((vk) => (vk.origin ?? "direct") === "direct" && vk.id !== initialVirtualKeyId)
		: [];

	const selectedTeam = teams.find((t) => t.id === teamId);
	const inheritedVirtualKeys = selectedTeam ? virtualKeysForTeam(virtualKeys, selectedTeam) : [];

	const toggleExpanded = (key: WorkspaceSectionKey) => {
		setExpandedSections((prev) => {
			const next = new Set(prev);
			if (next.has(key)) next.delete(key);
			else next.add(key);
			return next;
		});
	};

	const clearSectionGrants = (next: Set<WorkspaceGrantKey>, section: WorkspaceSection) => {
		next.delete(section.key);
		for (const item of section.items ?? []) {
			next.delete(itemGrantKey(section.key, item.key));
		}
		if (section.key === "guardrails") next.delete("cluster-config");
	};

	const toggleSection = (section: WorkspaceSection, checked: boolean) => {
		// Built-in users always keep Prompt Repository — cannot deselect.
		if (isUserRole(role) && section.key === "prompt-repository" && !checked) {
			return;
		}
		setAllowedSections((prev) => {
			const next = new Set(prev);
			clearSectionGrants(next, section);
			if (checked) {
				next.add(section.key);
			}
			if (isUserRole(role)) {
				next.add("prompt-repository");
			}
			return next;
		});
	};

	const toggleItem = (section: WorkspaceSection, itemKey: string, checked: boolean) => {
		setAllowedSections((prev) => {
			const next = new Set(prev);
			const grant = itemGrantKey(section.key, itemKey);
			// Expand parent-all into individual children before toggling one
			if (next.has(section.key) && section.items?.length) {
				next.delete(section.key);
				for (const item of section.items) {
					next.add(itemGrantKey(section.key, item.key));
				}
			}
			if (checked) {
				next.add(grant);
				if (section.key === "guardrails" && itemKey === "cluster-config") {
					next.delete("cluster-config");
				}
				const allSelected = section.items?.every(
					(item) => next.has(itemGrantKey(section.key, item.key)),
				);
				if (allSelected && section.items?.length) {
					clearSectionGrants(next, section);
					next.add(section.key);
				}
			} else {
				next.delete(grant);
				if (section.key === "guardrails" && itemKey === "cluster-config") {
					next.delete("cluster-config");
				}
			}
			return next;
		});
	};

	const isItemChecked = (section: WorkspaceSection, itemKey: string) => {
		if (allowedSections.has(section.key)) return true;
		if (allowedSections.has(itemGrantKey(section.key, itemKey))) return true;
		if (section.key === "guardrails" && itemKey === "cluster-config" && allowedSections.has("cluster-config")) {
			return true;
		}
		return false;
	};

	const syncUserTeam = async (userId: string, nextTeamId: string, prevTeamId: string) => {
		if (prevTeamId && prevTeamId !== nextTeamId) {
			await removeTeamMember({ teamId: prevTeamId, userId }).unwrap();
		}
		if (nextTeamId && nextTeamId !== prevTeamId) {
			await addTeamMember({ teamId: nextTeamId, userId }).unwrap();
		}
	};

	const syncUserVirtualKey = async (userId: string, nextVkId: string, prevVkId: string) => {
		if (prevVkId && prevVkId !== nextVkId) {
			await deleteVirtualKeyUser({ vkId: prevVkId, user_id: userId }).unwrap();
		}
		if (nextVkId && nextVkId !== prevVkId) {
			await setVirtualKeyUser({ vkId: nextVkId, user_id: userId }).unwrap();
		}
	};

	const teamPicker =
		canAssignTeamAndKey(role) ? (
			<div className="space-y-2">
				<label className="text-muted-foreground text-sm font-medium">Team</label>
				<p className="text-muted-foreground text-xs">
					Assign this user to one team (e.g. Developer, Testing). Team is separate from Role.
				</p>
				<select
					value={teamId || "__none__"}
					onChange={(e) => setTeamId(e.target.value === "__none__" ? "" : e.target.value)}
					className="bg-muted/20 border-border/50 text-foreground w-full rounded-lg border p-2.5 text-sm focus:border-teal-500/50 focus:outline-none"
					data-testid="user-team-select"
				>
					<option value="__none__">No team</option>
					{teams.map((t) => (
						<option key={t.id} value={t.id}>
							{t.name}
						</option>
					))}
				</select>
				{teams.length === 0 && (
					<p className="text-muted-foreground text-xs">No teams yet — create teams under Governance → Teams first.</p>
				)}
			</div>
		) : null;

	const virtualKeyPicker =
		canAssignTeamAndKey(role) ? (
			<div className="space-y-2">
				<label className="text-muted-foreground text-sm font-medium">
					{inheritedVirtualKeys.length > 0 ? "Direct Virtual Key (optional)" : "Virtual Key (required for Prompt Repository chat)"}
				</label>
				{inheritedVirtualKeys.length > 0 ? (
					<p className="text-xs text-teal-500" data-testid="user-inherited-virtual-keys">
						Access via team/customer: {inheritedVirtualKeys.map(describeTeamVirtualKey).join(", ")}. No direct key needed.
					</p>
				) : (
					<p className="text-muted-foreground text-xs">
						Without a Virtual Key, this user can open prompts but cannot send messages.
					</p>
				)}
				<select
					value={virtualKeyId || "__none__"}
					onChange={(e) => setVirtualKeyId(e.target.value === "__none__" ? "" : e.target.value)}
					className="bg-muted/20 border-border/50 text-foreground w-full rounded-lg border p-2.5 text-sm focus:border-teal-500/50 focus:outline-none"
					data-testid="user-virtual-key-select"
				>
					<option value="__none__">No Virtual Key</option>
					{virtualKeys.map((vk) => (
						<option key={vk.id} value={vk.id}>
							{vk.name}
						</option>
					))}
				</select>
				{virtualKeys.length === 0 && (
					<p className="text-muted-foreground text-xs">
						No active Virtual Keys — create one under Governance → Virtual Keys first.
					</p>
				)}
				{otherDirectVirtualKeys.length > 0 && (
					<p className="text-muted-foreground text-xs">
						Also directly assigned: {otherDirectVirtualKeys.map((vk) => vk.name).join(", ")}. These stay unchanged — manage them under
						Governance → Virtual Keys.
					</p>
				)}
			</div>
		) : null;

	const filteredWorkspaceSections = useMemo(() => {
		if (!workspaceSearch.trim()) return WORKSPACE_ACCESS_SECTIONS;
		const q = workspaceSearch.trim().toLowerCase();
		return WORKSPACE_ACCESS_SECTIONS.filter((section) => {
			const matchSection = section.label.toLowerCase().includes(q) || section.key.toLowerCase().includes(q);
			const matchItems = section.items?.some(
				(item) => item.label.toLowerCase().includes(q) || item.key.toLowerCase().includes(q),
			);
			return matchSection || matchItems;
		});
	}, [workspaceSearch]);

	const isSubAdmin = role === "sub_admin";
	const isAdmin = role === "admin";

	const workspaceAccessPicker = sectionGrantsApply(role) ? (
		<div className="space-y-2">
			<label className="text-muted-foreground text-sm font-medium">
				Workspace Access{" "}
				{isAdmin
					? "(Full Access — deselect to limit)"
					: isSubAdmin
						? "(Full Access for Selected Features)"
						: "(View Only for Selected Features + Prompt Repository always on)"}
			</label>
			<p className="text-muted-foreground text-xs">
				{isAdmin
					? "By default every section is selected (unrestricted). Deselect sections to limit this admin. Selected sections get Full Access in Roles & Permissions (RBAC)."
					: isSubAdmin
						? "Select workspace sections for this Sub-Admin. Those sections get Full Access (View, Edit, Delete, Download) automatically in Roles & Permissions (RBAC)."
						: "Prompt Repository is always enabled with full access. Any other section you select is View-only in RBAC by default; an admin can later raise Edit/Download for those same features in Roles & Permissions."}
			</p>
			<div className="relative">
				<Search className="text-muted-foreground absolute left-2.5 top-2.5 size-3.5" />
				<Input
					type="text"
					placeholder="Search workspace pages or sections..."
					value={workspaceSearch}
					onChange={(e) => setWorkspaceSearch(e.target.value)}
					className="h-8 pl-8 pr-7 text-xs bg-muted/20 border-border/50"
				/>
				{workspaceSearch && (
					<button
						type="button"
						onClick={() => setWorkspaceSearch("")}
						className="text-muted-foreground hover:text-foreground absolute right-2.5 top-2.5 size-3.5 flex items-center justify-center cursor-pointer"
					>
						<X className="size-3.5" />
					</button>
				)}
			</div>
			<div className="border-border/50 bg-muted/10 max-h-64 space-y-1 overflow-y-auto rounded-lg border p-2">
				{filteredWorkspaceSections.length === 0 ? (
					<p className="text-muted-foreground py-4 text-center text-xs">No workspace sections match "{workspaceSearch}"</p>
				) : (
					filteredWorkspaceSections.map((section) => {
						const hasChildren = !!section.items?.length;
						const isSearching = !!workspaceSearch.trim();
						const expanded = isSearching || expandedSections.has(section.key);
						const sel = sectionSelectionState(section, allowedSections);
						const q = workspaceSearch.trim().toLowerCase();
						const visibleItems = (section.items ?? []).filter((item) => {
							if (!isSearching) return true;
							if (section.label.toLowerCase().includes(q) || section.key.toLowerCase().includes(q)) return true;
							return item.label.toLowerCase().includes(q) || item.key.toLowerCase().includes(q);
						});
						return (
							<div key={section.key} className="rounded-md">
								<div className="hover:bg-muted/40 flex items-center gap-1 rounded-md px-1 py-1">
									{hasChildren ? (
										<button
											type="button"
											onClick={() => toggleExpanded(section.key)}
											className="text-muted-foreground hover:text-foreground flex size-6 shrink-0 items-center justify-center rounded"
											aria-label={expanded ? `Collapse ${section.label}` : `Expand ${section.label}`}
										>
											{expanded ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
										</button>
									) : (
										<span className="size-6 shrink-0" />
									)}
									<label
										className={`text-foreground flex min-w-0 flex-1 items-center gap-2 text-sm select-none ${
											isUserRole(role) && section.key === "prompt-repository" ? "cursor-default opacity-90" : "cursor-pointer"
										}`}
									>
										<IndeterminateCheckbox
											checked={isUserRole(role) && section.key === "prompt-repository" ? true : sel === "all"}
											indeterminate={isUserRole(role) && section.key === "prompt-repository" ? false : sel === "some"}
											onChange={(checked) => toggleSection(section, checked)}
											disabled={isUserRole(role) && section.key === "prompt-repository"}
										/>
										<span className="truncate font-medium">
											{section.label}
											{isUserRole(role) && section.key === "prompt-repository" ? (
												<span className="text-muted-foreground ml-1 font-normal">(always on)</span>
											) : null}
										</span>
									</label>
								</div>
								{hasChildren && expanded ? (
									<div className="border-border/40 ml-6 space-y-0.5 border-l py-1 pl-3">
										{visibleItems.map((item) => (
											<label
												key={item.key}
												className="text-foreground hover:bg-muted/40 flex cursor-pointer items-center gap-2 rounded-md px-1 py-1 text-sm select-none"
											>
												<input
													type="checkbox"
													checked={isItemChecked(section, item.key)}
													onChange={(e) => toggleItem(section, item.key, e.target.checked)}
													className="border-border rounded text-teal-500 focus:ring-teal-500/50"
												/>
												<span className="truncate">{item.label}</span>
											</label>
										))}
									</div>
								) : null}
							</div>
						);
					})
				)}
			</div>
		</div>
	) : null;

	const promptReposPicker =
		canAssignTeamAndKey(role) ? (
			<div className="space-y-2">
				<label className="text-muted-foreground text-sm font-medium">Allowed Prompt Repositories (Optional)</label>
				<p className="text-muted-foreground text-xs">
					Optional: Pick which prompt repositories this member may access. Leave unselected if not needed.
				</p>
				<div className="border-border/50 bg-muted/10 max-h-40 space-y-2 overflow-y-auto rounded-lg border p-3">
					{allPrompts.map((p) => {
						const isChecked = allowedPromptRepos
							.split(",")
							.map((id) => id.trim())
							.includes(p.id);
						return (
							<label key={p.id} className="text-foreground flex cursor-pointer items-center gap-2 text-sm select-none">
								<input
									type="checkbox"
									checked={isChecked}
									onChange={(e) => {
										let ids = allowedPromptRepos
											.split(",")
											.map((id) => id.trim())
											.filter(Boolean);
										if (e.target.checked) {
											ids.push(p.id);
										} else {
											ids = ids.filter((id) => id !== p.id);
										}
										setAllowedPromptRepos(ids.join(","));
									}}
									className="border-border mr-1 rounded text-teal-500 focus:ring-teal-500/50"
								/>
								{p.name}
							</label>
						);
					})}
					{allPrompts.length === 0 && <p className="text-muted-foreground text-xs">No prompt repositories found</p>}
				</div>
			</div>
		) : null;

	const resolveAllowedPromptNames = (raw?: string) => {
		const ids = (raw || "")
			.split(",")
			.map((id) => id.trim())
			.filter(Boolean);
		const names = ids
			.map((id) => allPrompts.find((p) => p.id === id)?.name)
			.filter((name): name is string => Boolean(name));
		return names.length > 0 ? names.join(", ") : "None";
	};

	const sanitizeAllowedPromptRepos = (raw?: string) => {
		const ids = (raw || "")
			.split(",")
			.map((id) => id.trim())
			.filter(Boolean);
		// Until the prompt list has loaded every id would look stale; keep them instead of
		// wiping the user's prompt access on save.
		if (!promptsData) return ids.join(",");
		const validIds = new Set(allPrompts.map((p) => p.id));
		return ids.filter((id) => validIds.has(id)).join(",");
	};

	const userPayload = () => {
		const effectiveSections = new Set(allowedSections);
		if (isUserRole(role)) {
			effectiveSections.add("prompt-repository");
		}
		let sectionsOut = "";
		if (role === "admin") {
			// All parents selected (or empty UI) → unrestricted. Partial deselect → store list.
			sectionsOut =
				effectiveSections.size === 0 || isUnrestrictedAdminSelection(effectiveSections)
					? ""
					: allowedSectionsToString(effectiveSections);
		} else if (sectionGrantsApply(role)) {
			sectionsOut = allowedSectionsToString(effectiveSections);
		}
		return {
			username,
			email: email.trim() || undefined,
			role,
			budget: canAssignTeamAndKey(role) ? budget : 0,
			rate_limit: canAssignTeamAndKey(role) ? rateLimit : 0,
			allowed_prompt_repos: canAssignTeamAndKey(role) ? sanitizeAllowedPromptRepos(allowedPromptRepos) : "",
			allowed_sections: sectionsOut,
			auto_create_prompt: isUserRole(role),
		};
	};

	const handleCreateUser = async (e: React.FormEvent) => {
		e.preventDefault();
		if (isCreating) return;
		if (!username) {
			toast.error("Username is required");
			return;
		}
		if (!email.trim()) {
			toast.error("Email is required so welcome mail / password-reset OTP can work");
			return;
		}
		if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim())) {
			toast.error("Please enter a valid email address");
			return;
		}
		setIsCreating(true);
		try {
			// The server generates a temporary password, emails it, and forces a change on first login.
			const created = await createUser(userPayload()).unwrap();
			if (created?.id && canAssignTeamAndKey(role)) {
				try {
					await syncUserTeam(created.id, teamId, "");
				} catch (teamErr) {
					toast.warning(`User created, but team assign failed: ${getErrorMessage(teamErr)}`);
					setIsCreateOpen(false);
					resetForm();
					return;
				}
				try {
					await syncUserVirtualKey(created.id, virtualKeyId, "");
				} catch (vkErr) {
					toast.warning(
						`User created, but Virtual Key assign failed: ${getErrorMessage(vkErr)}. Assign under Virtual Keys or edit user.`,
					);
					setIsCreateOpen(false);
					resetForm();
					return;
				}
				if (!virtualKeyId && inheritedVirtualKeys.length === 0) {
					toast.warning("User created without a Virtual Key — Prompt Repository chat will stay blocked until you assign one.");
				}
			}
			if (created?.email_sent) {
				toast.success("User created. A temporary password was emailed. First login must set a new password.");
			} else if (created?.email_error) {
				toast.warning(`User created, but the temporary password email failed: ${created.email_error}`);
			} else {
				toast.warning("User created, but the temporary password was not emailed. Check SMTP under Security settings.");
			}
			setIsCreateOpen(false);
			resetForm();
		} catch (err) {
			toast.error(getErrorMessage(err));
		} finally {
			setIsCreating(false);
		}
	};

	const handleEditUser = async (e: React.FormEvent) => {
		e.preventDefault();
		if (isUpdating) return;
		if (!selectedUser) return;
		if (!email.trim()) {
			toast.error("Email is required so welcome mail / password-reset OTP can work");
			return;
		}
		if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim())) {
			toast.error("Please enter a valid email address");
			return;
		}
		setIsUpdating(true);
		try {
			const updated = await updateUser({
				id: selectedUser.id,
				updates: { ...userPayload(), send_temporary_password: sendTemporaryPassword },
			}).unwrap();
			if (sendTemporaryPassword) {
				const mailed = updated as { email_sent?: boolean; email_error?: string };
				if (mailed?.email_sent) {
					toast.success("A new temporary password was emailed. They must set a new password on next login.");
				} else if (mailed?.email_error) {
					toast.warning(`Saved, but the temporary password email failed: ${mailed.email_error}`);
				} else {
					toast.warning("Saved, but the temporary password was not emailed. Check SMTP under Security settings.");
				}
			}
			if (canAssignTeamAndKey(role)) {
				await syncUserTeam(selectedUser.id, teamId, initialTeamId);
				await syncUserVirtualKey(selectedUser.id, virtualKeyId, initialVirtualKeyId);
				if (!virtualKeyId && inheritedVirtualKeys.length === 0) {
					toast.warning("Saved without a Virtual Key — Prompt Repository chat stays blocked until you assign one.");
				}
			} else {
				if (initialTeamId) {
					await syncUserTeam(selectedUser.id, "", initialTeamId);
				}
				if (initialVirtualKeyId) {
					await syncUserVirtualKey(selectedUser.id, "", initialVirtualKeyId);
				}
			}
			if (!sendTemporaryPassword) {
				toast.success("User updated successfully");
			}
			setIsEditOpen(false);
			resetForm();
		} catch (err) {
			toast.error(getErrorMessage(err));
		} finally {
			setIsUpdating(false);
		}
	};

	const handleDeleteUser = async () => {
		if (!selectedUser || isDeleting) return;
		setIsDeleting(true);
		try {
			await deleteUser(selectedUser.id).unwrap();
			toast.success("User deleted successfully");
			setIsDeleteOpen(false);
			setSelectedUser(null);
		} catch (err) {
			toast.error(getErrorMessage(err));
		} finally {
			setIsDeleting(false);
		}
	};

	const handleApprove = async (user: SessionUser) => {
		setActionBusyId(user.id);
		try {
			const result = await approveUser(user.id).unwrap();
			if (result?.email_sent) {
				toast.success(`${user.username} approved — notification email sent`);
			} else if (result?.email_error) {
				toast.warning(`${user.username} approved, but email failed: ${result.email_error}`);
			} else if (!user.email?.trim()) {
				toast.success(`${user.username} approved (no email on account — notification not sent)`);
			} else {
				toast.success(`${user.username} approved (no email — enable SMTP in Security settings)`);
			}
		} catch (err) {
			toast.error(getErrorMessage(err));
		} finally {
			setActionBusyId(null);
		}
	};

	const handleReject = async (user: SessionUser) => {
		setActionBusyId(user.id);
		try {
			const result = await rejectUser(user.id).unwrap();
			if (result?.email_sent) {
				toast.success(`${user.username} denied — notification email sent`);
			} else if (result?.email_error) {
				toast.warning(`${user.username} denied, but email failed: ${result.email_error}`);
			} else if (!user.email?.trim()) {
				toast.success(`${user.username} denied (no email on account — notification not sent)`);
			} else {
				toast.success(`${user.username} denied (no email — enable SMTP in Security settings)`);
			}
		} catch (err) {
			toast.error(getErrorMessage(err));
		} finally {
			setActionBusyId(null);
		}
	};

	const handleBlock = async (user: SessionUser) => {
		setActionBusyId(user.id);
		try {
			const result = await blockUser(user.id).unwrap();
			if (result?.email_sent) {
				toast.success(`${user.username} blocked — security notification email sent`);
			} else if (result?.email_error) {
				toast.warning(`${user.username} blocked, but notification email failed: ${result.email_error}`);
			} else {
				toast.success(`${user.username} blocked`);
			}
		} catch (err) {
			toast.error(getErrorMessage(err));
		} finally {
			setActionBusyId(null);
		}
	};

	const handleUnblock = async (user: SessionUser) => {
		setActionBusyId(user.id);
		try {
			const result = await unblockUser(user.id).unwrap();
			if (result?.email_sent) {
				toast.success(`${user.username} unblocked — notification email sent`);
			} else if (result?.email_error) {
				toast.warning(`${user.username} unblocked, but notification email failed: ${result.email_error}`);
			} else {
				toast.success(`${user.username} unblocked`);
			}
		} catch (err) {
			toast.error(getErrorMessage(err));
		} finally {
			setActionBusyId(null);
		}
	};

	const resetForm = () => {
		setUsername("");
		setEmail("");
		setSendTemporaryPassword(false);
		setRole("user");
		setTeamId("");
		setInitialTeamId("");
		setVirtualKeyId("");
		setInitialVirtualKeyId("");
		setBudget(0);
		setRateLimit(0);
		setAllowedPromptRepos("");
		// Create defaults to role=user: Prompt Repository is always on.
		setAllowedSections(new Set<WorkspaceGrantKey>(["prompt-repository"]));
		setExpandedSections(new Set());
		setSelectedUser(null);
		setWorkspaceSearch("");
	};

	const openEditModal = (user: SessionUser) => {
		setSelectedUser(user);
		setUsername(user.username);
		setEmail(user.email || "");
		setSendTemporaryPassword(false);
		const nextRole = (user.role || "user").trim() || "user";
		setRole(nextRole);
		setTeamId("");
		setInitialTeamId("");
		setVirtualKeyId("");
		setInitialVirtualKeyId("");
		setBudget(user.budget);
		setRateLimit(user.rate_limit);
		setAllowedPromptRepos(sanitizeAllowedPromptRepos(user.allowed_prompt_repos || ""));
		const grants =
			nextRole === "admin" ? adminSectionsForForm(user.allowed_sections) : adminSectionsFromStorage(user.allowed_sections);
		setAllowedSections(grants);
		setExpandedSections(
			new Set(
				WORKSPACE_ACCESS_SECTIONS.filter(
					(s) => s.items?.length && sectionSelectionState(s, grants) !== "none",
				).map((s) => s.key),
			),
		);
		setWorkspaceSearch("");
		setIsEditOpen(true);
	};

	const roleLabel = (roleName: string) => {
		const normalized = (roleName || "user").trim();
		if (normalized === "admin") return "Admin";
		if (normalized === "sub_admin") return "Sub-Admin";
		if (normalized === "user") return "User";
		return normalized;
	};

	const openDeleteModal = (user: SessionUser) => {
		setSelectedUser(user);
		setIsDeleteOpen(true);
	};

	const matchesSearch = (u: SessionUser) => {
		const q = searchQuery.trim().toLowerCase();
		if (!q) return true;
		return (
			u.username.toLowerCase().includes(q) ||
			(u.email || "").toLowerCase().includes(q) ||
			u.id.toLowerCase().includes(q) ||
			roleLabel(u.role).toLowerCase().includes(q)
		);
	};

	const isUserRoleOnly = (u: SessionUser) => {
		const r = (u.role || "user").trim().toLowerCase();
		return r === "user" || (r !== "admin" && r !== "sub_admin" && r !== "sub-admin");
	};

	const isAdminRoleOnly = (u: SessionUser) => {
		const r = (u.role || "user").trim().toLowerCase();
		return r === "admin";
	};

	const isSubAdminRoleOnly = (u: SessionUser) => {
		const r = (u.role || "user").trim().toLowerCase();
		return r === "sub_admin" || r === "sub-admin";
	};

	const matchesRole = (u: SessionUser) => {
		if (roleFilter === "admin") return isAdminRoleOnly(u);
		if (roleFilter === "sub_admin") return isSubAdminRoleOnly(u);
		if (roleFilter === "user") return isUserRoleOnly(u);
		return true;
	};

	const approvedUsers = users.filter((u) => (u.status || "approved") === "approved");
	const unapprovedUsers = users.filter((u) => ["pending", "email_unverified"].includes(u.status || ""));
	const rejectedUsers = users.filter((u) => u.status === "rejected");
	const blockedUsers = users.filter((u) => u.status === "disabled");

	const allowedDisplay = userQuota
		? userQuota.limited
			? userQuota.max
			: "Unlimited"
		: "—";

	const roleCounts: Record<TabFilter, string | number> = {
		allowed: allowedDisplay,
		all: approvedUsers.length,
		user: approvedUsers.filter(isUserRoleOnly).length,
		admin: approvedUsers.filter(isAdminRoleOnly).length,
		sub_admin: approvedUsers.filter(isSubAdminRoleOnly).length,
		pending: unapprovedUsers.length,
		rejected: rejectedUsers.length,
		blocked: blockedUsers.length,
	};

	const roleFilterOptions: { value: TabFilter; label: string; icon: React.ReactNode }[] = [
		{ value: "allowed", label: "Allowed Users", icon: <Key className="h-3.5 w-3.5 text-emerald-400" /> },
		{ value: "all", label: "Active Users", icon: <Users className="h-3.5 w-3.5" /> },
		{ value: "user", label: "User", icon: <User className="h-3.5 w-3.5" /> },
		{ value: "admin", label: "Admin", icon: <Shield className="h-3.5 w-3.5" /> },
		{ value: "sub_admin", label: "Sub Admin", icon: <ShieldCheck className="h-3.5 w-3.5" /> },
		{ value: "pending", label: "Pending Approvals", icon: <Clock className="h-3.5 w-3.5" /> },
		{ value: "rejected", label: "Rejected Users", icon: <UserX className="h-3.5 w-3.5 text-rose-400" /> },
		{ value: "blocked", label: "Blocked Accounts", icon: <Ban className="h-3.5 w-3.5 text-amber-400" /> },
	];

	// Filtered sets matching search
	const pendingUsers = unapprovedUsers.filter(matchesSearch);
	const filteredRejectedUsers = rejectedUsers.filter(matchesSearch);
	const filteredBlockedUsers = blockedUsers.filter(matchesSearch);
	const activeUsers = approvedUsers.filter((u) => matchesSearch(u) && matchesRole(u));
	const [activeUsersOffset, setActiveUsersOffset] = useState(0);
	const [activeUsersLimit, setActiveUsersLimit] = useState(10);

	useEffect(() => {
		setActiveUsersOffset(0);
	}, [searchQuery, roleFilter]);

	const pagedActiveUsers = useMemo(
		() => activeUsers.slice(activeUsersOffset, activeUsersOffset + activeUsersLimit),
		[activeUsers, activeUsersOffset, activeUsersLimit],
	);

	const [selectedUserIds, setSelectedUserIds] = useState<Set<string>>(new Set());
	const [isBulkDeleteUsersOpen, setIsBulkDeleteUsersOpen] = useState(false);
	const [isBulkDeletingUsers, setIsBulkDeletingUsers] = useState(false);

	const visibleUserIds = useMemo(() => pagedActiveUsers.map((u) => u.id), [pagedActiveUsers]);
	const selectedVisibleUserIds = useMemo(
		() => visibleUserIds.filter((id) => selectedUserIds.has(id)),
		[selectedUserIds, visibleUserIds],
	);
	const selectedUserCount = selectedUserIds.size;
	const allVisibleUsersSelected =
		visibleUserIds.length > 0 && selectedVisibleUserIds.length === visibleUserIds.length;
	const someVisibleUsersSelected =
		selectedVisibleUserIds.length > 0 && selectedVisibleUserIds.length < visibleUserIds.length;

	const toggleSelectAllVisibleUsers = (checked: boolean) => {
		setSelectedUserIds((prev) => {
			const next = new Set(prev);
			for (const id of visibleUserIds) {
				if (currentUserId && id === currentUserId) continue;
				if (checked) {
					next.add(id);
				} else {
					next.delete(id);
				}
			}
			return next;
		});
	};

	const toggleSelectUser = (userId: string, checked: boolean) => {
		if (currentUserId && userId === currentUserId) return;
		setSelectedUserIds((prev) => {
			const next = new Set(prev);
			if (checked) {
				next.add(userId);
			} else {
				next.delete(userId);
			}
			return next;
		});
	};

	const handleBulkDeleteUsers = async () => {
		if (selectedUserCount === 0) return;
		setIsBulkDeletingUsers(true);
		let successCount = 0;
		let failCount = 0;
		const ids = Array.from(selectedUserIds);
		for (const id of ids) {
			if (currentUserId && id === currentUserId) continue;
			try {
				await deleteUser(id).unwrap();
				successCount++;
			} catch {
				failCount++;
			}
		}
		setIsBulkDeletingUsers(false);
		setIsBulkDeleteUsersOpen(false);
		setSelectedUserIds(new Set());
		refetchUsers();
		if (successCount > 0) {
			toast.success(`Successfully deleted ${successCount} user${successCount > 1 ? "s" : ""}`);
		}
		if (failCount > 0) {
			toast.error(`Failed to delete ${failCount} user${failCount > 1 ? "s" : ""}`);
		}
	};

	return (
		<div className="text-foreground bg-background border-border/40 flex w-full flex-col gap-6 rounded-lg border p-6 shadow-xl backdrop-blur-sm">
			{/* Header Section */}
			<div className="border-border/40 flex flex-col items-start justify-between gap-4 border-b pb-6 md:flex-row md:items-center">
				<div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:gap-3">
					<h1 className="text-primary flex items-center gap-2 text-2xl font-semibold tracking-tight">
						<Users className="h-6 w-6 text-teal-400" />
						User Governance
					</h1>
					{!hasUpdateAccess && (
						<Badge variant="outline" className="border-amber-500/40 text-amber-600 dark:text-amber-400 bg-amber-500/10 gap-1 text-xs" data-testid="users-view-only-badge">
							<Eye className="h-3 w-3" />
							View Only
						</Badge>
					)}
					{userQuota?.limited && (
						<span
							className={cn(
								"inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium border shadow-sm",
								userQuota.remaining <= 0
									? "border-red-500/40 bg-red-500/10 text-red-400"
									: "border-teal-500/30 bg-teal-500/10 text-teal-300",
							)}
							title={`License allocation: ${userQuota.used}/${userQuota.max} users in use`}
						>
							{userQuota.remaining <= 0 ? (
								<Shield className="h-3.5 w-3.5 text-red-400" />
							) : (
								<ShieldCheck className="h-3.5 w-3.5 text-teal-400" />
							)}
							License Quota: {userQuota.used}/{userQuota.max} Users
							{userQuota.remaining <= 0 ? " (Capacity Reached)" : ` (${userQuota.remaining} left)`}
						</span>
					)}
				</div>
				<div className="flex items-center gap-2">
					{selectedUserCount > 0 && (
						<Button
							variant="destructive"
							data-testid="users-bulk-delete-btn"
							onClick={() => setIsBulkDeleteUsersOpen(true)}
							disabled={!hasDeleteAccess || isBulkDeletingUsers}
							className="flex items-center gap-2 rounded-lg bg-rose-600 px-4 py-2 font-medium text-white shadow-lg shadow-rose-600/20 hover:bg-rose-700 cursor-pointer"
						>
							<Trash2 className="h-4 w-4" /> Delete Selected ({selectedUserCount})
						</Button>
					)}
					<Button
						disabled={!hasCreateAccess || (!!userQuota?.limited && userQuota.remaining <= 0)}
						onClick={() => {
							resetForm();
							setIsCreateOpen(true);
						}}
						title={
							userQuota?.limited && userQuota.remaining <= 0
								? `License limit reached (${userQuota.used}/${userQuota.max} users). Contact YesPanchi to upgrade seats.`
								: undefined
						}
						className="flex items-center gap-2 rounded-lg bg-teal-500 px-4 py-2 font-medium text-white shadow-lg shadow-teal-500/20 transition-all duration-200 hover:bg-teal-600 active:scale-95 disabled:opacity-50 disabled:cursor-not-allowed"
					>
						<Plus className="h-4 w-4" /> Add New User
					</Button>
				</div>
			</div>

			{helperQueryFailed ? (
				<QueryErrorBanner
					testId="users-helper-query-error"
					message={getErrorMessage(helperQueryError) || "Failed to load teams, virtual keys, roles, or prompts."}
				/>
			) : null}

			{/* Search + role filter */}
			<div className="flex w-full flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
				<div className="relative w-full max-w-sm">
					<Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
					<Input
						placeholder={
							roleFilter === "pending"
								? "Search pending requests by name, email or role..."
								: "Search by name, email or role..."
						}
						value={searchQuery}
						onChange={(e) => setSearchQuery(e.target.value)}
						className="bg-muted/30 border-border/50 rounded-lg pl-9 pr-9 focus:border-teal-500/50"
						data-testid="users-search-input"
					/>
					{searchQuery ? (
						<button
							type="button"
							onClick={() => setSearchQuery("")}
							className="text-muted-foreground hover:text-foreground absolute top-1/2 right-3 -translate-y-1/2"
							aria-label="Clear search"
						>
							<X className="h-4 w-4" />
						</button>
					) : null}
				</div>
				<div
					role="group"
					aria-label="Filter by role"
					className="bg-muted/30 border-border/50 inline-flex flex-wrap shrink-0 items-center gap-1 rounded-lg border p-1"
				>
					{roleFilterOptions.map((opt) => {
						const active = roleFilter === opt.value;
						const isPendingTab = opt.value === "pending";
						const isAllowedTab = opt.value === "allowed";
						const isRejectedTab = opt.value === "rejected";
						const isBlockedTab = opt.value === "blocked";
						const hasPendingAlert = isPendingTab && (Number(roleCounts.pending) || 0) > 0;
						const hasRejectedAlert = isRejectedTab && (Number(roleCounts.rejected) || 0) > 0;
						return (
							<button
								key={opt.value}
								type="button"
								onClick={() => setRoleFilter(opt.value)}
								aria-pressed={active}
								data-testid={`users-role-filter-${opt.value}`}
								title={
									isAllowedTab && userQuota
										? userQuota.limited
											? `License Key Limit: ${userQuota.used}/${userQuota.max} users (${userQuota.remaining} seats left)`
											: "Unlimited users allowed by license"
										: undefined
								}
								className={`inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${active
										? isPendingTab
											? "bg-amber-500 text-white shadow-sm"
											: isAllowedTab
												? "bg-emerald-600 text-white shadow-sm"
												: isRejectedTab
													? "bg-rose-600 text-white shadow-sm"
													: isBlockedTab
														? "bg-amber-600 text-white shadow-sm"
														: "bg-teal-500 text-white shadow-sm"
										: hasPendingAlert
											? "text-amber-400 hover:bg-amber-500/10 hover:text-amber-300 font-semibold"
											: hasRejectedAlert
												? "text-rose-400 hover:bg-rose-500/10 hover:text-rose-300 font-semibold"
												: isAllowedTab
													? "text-emerald-400 hover:bg-emerald-500/10 hover:text-emerald-300"
													: "text-muted-foreground hover:bg-muted hover:text-foreground"
									}`}
							>
								{opt.icon}
								{opt.label}
								<span
									className={`rounded-full px-1.5 text-[10px] font-semibold ${active
											? "bg-white/20 text-white"
											: hasPendingAlert
												? "bg-amber-500/20 text-amber-300 border border-amber-500/30"
												: hasRejectedAlert
													? "bg-rose-500/20 text-rose-300 border border-rose-500/30"
													: isAllowedTab
														? "bg-emerald-500/20 text-emerald-300 border border-emerald-500/30"
														: "bg-muted text-muted-foreground"
										}`}
								>
									{roleCounts[opt.value]}
								</span>
							</button>
						);
					})}
				</div>
			</div>

			{roleFilter === "allowed" && userQuota ? (
				<div className="flex items-center gap-2 rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-4 py-2.5 text-xs text-emerald-300">
					<Key className="h-4 w-4 text-emerald-400 shrink-0" />
					<span>
						<strong>License Key Capacity:</strong> {userQuota.limited ? `${userQuota.max} users allowed` : "Unlimited users allowed"} — Currently {userQuota.used} active ({userQuota.remaining} remaining seats available).
					</span>
				</div>
			) : null}

			{loading ? (
				<div className="flex items-center justify-center py-20">
					<div className="h-8 w-8 animate-spin rounded-full border-t-2 border-b-2 border-teal-400"></div>
				</div>
			) : usersError ? (
				<div
					className="flex flex-col items-center justify-center gap-3 rounded-xl border border-red-500/30 bg-red-500/5 py-16 text-center"
					data-testid="users-load-error"
				>
					<p className="text-base font-medium text-red-400">Could not load users</p>
					<p className="text-muted-foreground max-w-md text-sm">{getErrorMessage(usersErrorDetail)}</p>
					<Button variant="outline" size="sm" onClick={() => refetchUsers()}>
						Retry
					</Button>
				</div>
			) : roleFilter === "pending" ? (
				/* Pending registrations view only when Pending Approvals tab is active */
				<div className="space-y-4">
					<div className="flex flex-col gap-2 rounded-xl border border-amber-500/20 bg-amber-500/5 p-4 sm:flex-row sm:items-center sm:justify-between">
						<div className="space-y-1">
							<div className="flex items-center gap-2">
								<Clock className="h-5 w-5 text-amber-400" />
								<h2 className="text-base font-semibold text-amber-300">
									Pending Approvals ({pendingUsers.length}{searchQuery && pendingUsers.length !== unapprovedUsers.length ? ` of ${unapprovedUsers.length}` : ""})
								</h2>
							</div>
							<p className="text-muted-foreground text-xs">
								Accept activates the account (after email verification). Deny blocks sign-in and removes the request from this list. Admins also receive Accept/Deny email links when SMTP is enabled.
							</p>
						</div>
					</div>

					{pendingUsers.length === 0 ? (
						<div className="border-border/40 bg-muted/10 flex flex-col items-center justify-center rounded-xl border border-dashed py-16 text-center">
							<Clock className="text-muted-foreground/60 mb-3 h-12 w-12" />
							<p className="text-foreground/80 text-base font-medium">No Pending Requests</p>
							<p className="text-muted-foreground mt-1 max-w-xs text-sm">
								{searchQuery ? "No pending requests match your search query." : "No registration requests waiting."}
							</p>
						</div>
					) : (
						<div className="border-amber-500/20 bg-amber-500/5 overflow-hidden rounded-xl border shadow-sm">
							<Table>
								<TableHeader className="bg-muted/30">
									<TableRow>
										<TableHead className="text-foreground/90 font-semibold">ID</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Username</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Email</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Requested role</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Requested At</TableHead>
										<TableHead className="text-foreground/90 text-right font-semibold">Actions</TableHead>
									</TableRow>
								</TableHeader>
								<TableBody>
									{pendingUsers.map((user) => (
										<TableRow key={user.id} className="hover:bg-muted/20 transition-colors">
											<TableCell className="font-mono text-[11px] text-muted-foreground max-w-[140px] truncate" title={user.id}>
												{user.id}
											</TableCell>
											<TableCell className="font-medium">{user.username}</TableCell>
											<TableCell className="text-sm text-muted-foreground">
												{user.email || "—"}
												{user.status === "email_unverified" && (
													<span className="ml-2 inline-flex rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[10px] font-medium text-amber-400">
														Email not verified
													</span>
												)}
											</TableCell>
											<TableCell>
												<span
													className={`inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-medium ${user.role === "admin"
															? "border-teal-500/20 bg-teal-500/10 text-teal-400"
															: user.role === "sub_admin"
																? "border-purple-500/20 bg-purple-500/10 text-purple-400"
																: "border-blue-500/20 bg-blue-500/10 text-blue-400"
														}`}
												>
													{user.role === "admin" ? <Shield className="h-3 w-3" /> : user.role === "sub_admin" ? <ShieldCheck className="h-3 w-3" /> : <Users className="h-3 w-3" />}
													{roleLabel(user.role)}
												</span>
											</TableCell>
											<TableCell className="text-muted-foreground text-xs">
												{new Date(user.created_at).toLocaleString()}
											</TableCell>
											<TableCell className="text-right">
												<div className="flex justify-end gap-2">
													<Button
														size="sm"
														disabled={
															actionBusyId === user.id ||
															!hasUpdateAccess ||
															user.status === "email_unverified"
														}
														title={
															user.status === "email_unverified"
																? "Applicant must verify email before Accept"
																: "Accept — activate account"
														}
														onClick={() => handleApprove(user)}
														className="h-8 gap-1 bg-emerald-600 hover:bg-emerald-500 text-white"
														data-testid="user-registration-accept"
													>
														<Check className="h-3.5 w-3.5" />
														Accept
													</Button>
													<Button
														size="sm"
														variant="outline"
														disabled={actionBusyId === user.id || !hasUpdateAccess}
														onClick={() => handleReject(user)}
														className="h-8 gap-1 border-red-500/40 text-red-400 hover:bg-red-950/40"
														data-testid="user-registration-deny"
													>
														<X className="h-3.5 w-3.5" />
														Deny
													</Button>
												</div>
											</TableCell>
										</TableRow>
									))}
								</TableBody>
							</Table>
						</div>
					)}
				</div>
			) : roleFilter === "rejected" ? (
				/* Rejected users view */
				<div className="space-y-4">
					<div className="flex flex-col gap-2 rounded-xl border border-rose-500/20 bg-rose-500/5 p-4 sm:flex-row sm:items-center sm:justify-between">
						<div className="space-y-1">
							<div className="flex items-center gap-2">
								<UserX className="h-5 w-5 text-rose-400" />
								<h2 className="text-base font-semibold text-rose-300">
									Rejected Users ({filteredRejectedUsers.length}{searchQuery && filteredRejectedUsers.length !== rejectedUsers.length ? ` of ${rejectedUsers.length}` : ""})
								</h2>
							</div>
							<p className="text-muted-foreground text-xs">
								Accounts that were previously denied. Admins can Accept (restores access and sends login details) or Block / Delete them.
							</p>
						</div>
					</div>

					{filteredRejectedUsers.length === 0 ? (
						<div className="border-border/40 bg-muted/10 flex flex-col items-center justify-center rounded-xl border border-dashed py-16 text-center">
							<UserX className="text-muted-foreground/60 mb-3 h-12 w-12" />
							<p className="text-foreground/80 text-base font-medium">No Rejected Users</p>
							<p className="text-muted-foreground mt-1 max-w-xs text-sm">
								{searchQuery ? "No rejected users match your search query." : "No accounts currently in rejected state."}
							</p>
						</div>
					) : (
						<div className="border-rose-500/20 bg-rose-500/5 overflow-hidden rounded-xl border shadow-sm">
							<Table>
								<TableHeader className="bg-muted/30">
									<TableRow>
										<TableHead className="text-foreground/90 font-semibold">ID</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Username</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Email</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Role</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Rejected / Reviewed At</TableHead>
										<TableHead className="text-foreground/90 text-right font-semibold">Actions</TableHead>
									</TableRow>
								</TableHeader>
								<TableBody>
									{filteredRejectedUsers.map((user) => {
										const isSelf = Boolean(
											(currentUserId && user.id === currentUserId) ||
											(currentUsername && user.username?.trim().toLowerCase() === currentUsername) ||
											(currentUserEmail && user.email?.trim().toLowerCase() === currentUserEmail)
										);

										return (
											<TableRow key={user.id} className="hover:bg-muted/20 transition-colors">
												<TableCell className="font-mono text-[11px] text-muted-foreground max-w-[140px] truncate" title={user.id}>
													{user.id}
												</TableCell>
												<TableCell className="font-medium">
													<div className="flex items-center gap-2">
														<div className="flex h-7 w-7 items-center justify-center rounded-full bg-rose-500/10 text-xs font-bold text-rose-400 uppercase">
															{user.username.slice(0, 2)}
														</div>
														{user.username}
													</div>
												</TableCell>
												<TableCell className="text-sm text-muted-foreground">
													{user.email || "—"}
												</TableCell>
												<TableCell>
													<span
														className={`inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-medium ${user.role === "admin"
																? "border-teal-500/20 bg-teal-500/10 text-teal-400"
																: user.role === "sub_admin"
																	? "border-purple-500/20 bg-purple-500/10 text-purple-400"
																	: "border-blue-500/20 bg-blue-500/10 text-blue-400"
															}`}
													>
														{user.role === "admin" ? <Shield className="h-3 w-3" /> : user.role === "sub_admin" ? <ShieldCheck className="h-3 w-3" /> : <Users className="h-3 w-3" />}
														{roleLabel(user.role)}
													</span>
												</TableCell>
												<TableCell className="text-muted-foreground text-xs">
													{user.reviewed_at ? new Date(user.reviewed_at).toLocaleString() : new Date(user.created_at).toLocaleString()}
												</TableCell>
												<TableCell className="text-right">
													<div className="flex justify-end gap-2">
														<Button
															size="sm"
															disabled={actionBusyId === user.id || !hasUpdateAccess}
															onClick={() => handleApprove(user)}
															className="h-8 gap-1 bg-emerald-600 hover:bg-emerald-500 text-white cursor-pointer"
															title="Re-accept user and send approval email with login link"
															data-testid="rejected-user-accept"
														>
															<Check className="h-3.5 w-3.5" />
															Accept
														</Button>
														<Button
															size="sm"
															variant="outline"
															disabled={actionBusyId === user.id || !hasUpdateAccess || isSelf}
															onClick={() => handleBlock(user)}
															className={cn(
																"h-8 gap-1 border-amber-500/40 text-amber-400 hover:bg-amber-950/40",
																isSelf ? "opacity-30 cursor-not-allowed" : "cursor-pointer"
															)}
															title={isSelf ? "You cannot block your own account" : "Block user account and send security email"}
															data-testid="rejected-user-block"
														>
															<Ban className="h-3.5 w-3.5" />
															Block
														</Button>
														<Button
															size="sm"
															variant="outline"
															disabled={actionBusyId === user.id || !hasDeleteAccess || isSelf}
															onClick={() => openDeleteModal(user)}
															className={cn(
																"h-8 gap-1 border-rose-500/40 text-rose-400 hover:bg-rose-950/40",
																isSelf ? "opacity-30 cursor-not-allowed" : "cursor-pointer"
															)}
															title={isSelf ? "You cannot delete your own account" : "Permanently delete user record"}
															data-testid="rejected-user-delete"
														>
															<Trash2 className="h-3.5 w-3.5" />
															Delete
														</Button>
													</div>
												</TableCell>
											</TableRow>
										);
									})}
								</TableBody>
							</Table>
						</div>
					)}
				</div>
			) : roleFilter === "blocked" ? (
				/* Blocked accounts view */
				<div className="space-y-4">
					<div className="flex flex-col gap-2 rounded-xl border border-amber-500/20 bg-amber-500/5 p-4 sm:flex-row sm:items-center sm:justify-between">
						<div className="space-y-1">
							<div className="flex items-center gap-2">
								<Ban className="h-5 w-5 text-amber-400" />
								<h2 className="text-base font-semibold text-amber-300">
									Blocked Accounts ({filteredBlockedUsers.length}{searchQuery && filteredBlockedUsers.length !== blockedUsers.length ? ` of ${blockedUsers.length}` : ""})
								</h2>
							</div>
							<p className="text-muted-foreground text-xs">
								Accounts that have been blocked or disabled by an administrator. Admins can unblock accounts anytime to restore access, or permanently delete them.
							</p>
						</div>
					</div>

					{filteredBlockedUsers.length === 0 ? (
						<div className="border-border/40 bg-muted/10 flex flex-col items-center justify-center rounded-xl border border-dashed py-16 text-center">
							<Ban className="text-muted-foreground/60 mb-3 h-12 w-12" />
							<p className="text-foreground/80 text-base font-medium">No Blocked Accounts</p>
							<p className="text-muted-foreground mt-1 max-w-xs text-sm">
								{searchQuery ? "No blocked accounts match your search query." : "No accounts are currently blocked."}
							</p>
						</div>
					) : (
						<div className="border-amber-500/20 bg-amber-500/5 overflow-hidden rounded-xl border shadow-sm">
							<Table>
								<TableHeader className="bg-muted/30">
									<TableRow>
										<TableHead className="text-foreground/90 font-semibold">ID</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Username</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Email</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Role</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Updated At</TableHead>
										<TableHead className="text-foreground/90 text-right font-semibold">Actions</TableHead>
									</TableRow>
								</TableHeader>
								<TableBody>
									{filteredBlockedUsers.map((user) => (
										<TableRow key={user.id} className="hover:bg-muted/20 transition-colors">
											<TableCell className="font-mono text-[11px] text-muted-foreground max-w-[140px] truncate" title={user.id}>
												{user.id}
											</TableCell>
											<TableCell className="font-medium">
												<div className="flex items-center gap-2">
													<div className="flex h-7 w-7 items-center justify-center rounded-full bg-amber-500/10 text-xs font-bold text-amber-400 uppercase">
														{user.username.slice(0, 2)}
													</div>
													{user.username}
													<span className="rounded-full border border-rose-500/30 bg-rose-500/10 px-2 py-0.5 text-[10px] font-medium text-rose-400">
														Blocked
													</span>
												</div>
											</TableCell>
											<TableCell className="text-sm text-muted-foreground">
												{user.email || "—"}
											</TableCell>
											<TableCell>
												<span
													className={`inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-medium ${user.role === "admin"
															? "border-teal-500/20 bg-teal-500/10 text-teal-400"
															: user.role === "sub_admin"
																? "border-purple-500/20 bg-purple-500/10 text-purple-400"
																: "border-blue-500/20 bg-blue-500/10 text-blue-400"
														}`}
												>
													{user.role === "admin" ? <Shield className="h-3 w-3" /> : user.role === "sub_admin" ? <ShieldCheck className="h-3 w-3" /> : <Users className="h-3 w-3" />}
													{roleLabel(user.role)}
												</span>
											</TableCell>
											<TableCell className="text-muted-foreground text-xs">
												{new Date(user.updated_at || user.created_at).toLocaleString()}
											</TableCell>
											<TableCell className="text-right">
												<div className="flex justify-end gap-2">
													<Button
														size="sm"
														disabled={actionBusyId === user.id || !hasUpdateAccess}
														onClick={() => handleUnblock(user)}
														className="h-8 gap-1 bg-teal-600 hover:bg-teal-500 text-white"
														title="Unblock account and send notification email"
														data-testid="blocked-user-unblock"
													>
														<Check className="h-3.5 w-3.5" />
														Unblock
													</Button>
													<Button
														size="sm"
														variant="outline"
														disabled={actionBusyId === user.id || !hasDeleteAccess}
														onClick={() => openDeleteModal(user)}
														className="h-8 gap-1 border-rose-500/40 text-rose-400 hover:bg-rose-950/40"
														title="Permanently delete user record"
														data-testid="blocked-user-delete"
													>
														<Trash2 className="h-3.5 w-3.5" />
														Delete
													</Button>
												</div>
											</TableCell>
										</TableRow>
									))}
								</TableBody>
							</Table>
						</div>
					)}
				</div>
			) : (
				/* Active users */
				<div className="space-y-3">
					{activeUsers.length === 0 ? (
						<div className="border-border/40 bg-muted/10 flex flex-col items-center justify-center rounded-xl border border-dashed py-16 text-center">
							<Users className="text-muted-foreground/60 mb-3 h-12 w-12" />
							<p className="text-foreground/80 text-base font-medium">No Users Found</p>
							<p className="text-muted-foreground mt-1 max-w-xs text-sm">
								{searchQuery
									? "No users match your search query."
									: roleFilter === "admin"
										? "No admins yet."
										: roleFilter === "user"
											? "No users with the User role yet."
											: "Approve a registration or click Add New User."}
							</p>
						</div>
					) : (
						<div className="border-border/40 bg-muted/5 overflow-hidden rounded-xl border shadow-sm">
							<Table>
								<TableHeader className="bg-muted/30">
									<TableRow>
										<TableHead className="w-[48px]">
											<Checkbox
												checked={allVisibleUsersSelected || (someVisibleUsersSelected ? "indeterminate" : false)}
												onCheckedChange={(checked) => toggleSelectAllVisibleUsers(checked === true)}
												aria-label="Select all users on this page"
												data-testid="users-select-all-checkbox"
											/>
										</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Username</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Email</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Role</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Team</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Virtual Keys</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Budget (USD)</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Rate Limit (RPM)</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Allowed Repositories</TableHead>
										<TableHead className="text-foreground/90 font-semibold">Created At</TableHead>
										<TableHead className="text-foreground/90 text-right font-semibold">Actions</TableHead>
									</TableRow>
								</TableHeader>
								<TableBody>
									{pagedActiveUsers.map((user: SessionUser) => {
										const isUserBudgetExhausted =
											!isWorkspaceAdminRole(user.role) &&
											user.budget > 0 &&
											(user.budget_current_usage ?? 0) >= user.budget;

										const isSelf = Boolean(
											(currentUserId && user.id === currentUserId) ||
											(currentUsername && user.username?.trim().toLowerCase() === currentUsername) ||
											(currentUserEmail && user.email?.trim().toLowerCase() === currentUserEmail)
										);

										return (
											<TableRow
												key={user.id}
												className={cn(
													"transition-colors",
													isUserBudgetExhausted
														? "bg-red-500/10 border-l-4 border-l-red-500 hover:bg-red-500/15 dark:bg-red-950/20"
														: "hover:bg-muted/20",
												)}
											>
												<TableCell className="w-[48px]">
													<Checkbox
														checked={selectedUserIds.has(user.id)}
														onCheckedChange={(checked) => toggleSelectUser(user.id, checked === true)}
														disabled={isSelf}
														aria-label={`Select ${user.username}`}
														data-testid={`user-select-checkbox-${user.id}`}
													/>
												</TableCell>
												<TableCell className="flex items-center gap-2 font-medium">
													<div className="flex h-8 w-8 items-center justify-center rounded-full bg-teal-500/10 text-xs font-bold text-teal-400 uppercase">
														{user.username.slice(0, 2)}
													</div>
													{user.username}
													{isUserBudgetExhausted && (
														<span className="rounded-full border border-red-500/30 bg-red-600 px-2 py-0.5 text-[10px] font-semibold text-white shadow-xs">
															Budget Exhausted
														</span>
													)}
													{user.status === "disabled" && (
														<span className="rounded-full border border-rose-500/30 bg-rose-500/10 px-2 py-0.5 text-[10px] font-medium text-rose-400">
															Disabled
														</span>
													)}
												</TableCell>
												<TableCell className="text-sm text-muted-foreground">{user.email || "—"}</TableCell>
												<TableCell>
													<span
														className={`inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-medium ${user.role === "admin"
																? "border-teal-500/20 bg-teal-500/10 text-teal-400"
																: user.role === "sub_admin"
																	? "border-purple-500/20 bg-purple-500/10 text-purple-400"
																	: "border-blue-500/20 bg-blue-500/10 text-blue-400"
															}`}
													>
														<Shield className="h-3 w-3" />
														{roleLabel(user.role)}
													</span>
												</TableCell>
												<TableCell>
													{isWorkspaceAdminRole(user.role) ? (
														<span className="text-muted-foreground text-sm">—</span>
													) : (
														<UserTeamCell userId={user.id} />
													)}
												</TableCell>
												<TableCell>
													{isWorkspaceAdminRole(user.role) ? (
														<span className="text-muted-foreground text-sm">—</span>
													) : (
														<UserVirtualKeysCell userId={user.id} />
													)}
												</TableCell>
												<TableCell className="font-mono text-xs">
													{isWorkspaceAdminRole(user.role) ? (
														<span className="text-muted-foreground text-sm">—</span>
													) : user.budget > 0 ? (
														<div className="flex min-w-[110px] flex-col gap-1">
															<div className="flex justify-between text-xs">
																<span className="font-semibold text-foreground">
																	${(user.budget_current_usage ?? 0).toFixed(2)}
																</span>
																<span className="text-muted-foreground">/ ${user.budget.toFixed(2)}</span>
															</div>
															<div className="bg-muted h-1.5 w-full overflow-hidden rounded-full">
																<div
																	className={cn(
																		"h-full rounded-full transition-all",
																		(user.budget_current_usage ?? 0) >= user.budget
																			? "bg-red-500"
																			: (user.budget_current_usage ?? 0) / user.budget > 0.8
																				? "bg-amber-500"
																				: "bg-teal-500",
																	)}
																	style={{
																		width: `${Math.min(100, Math.max(0, ((user.budget_current_usage ?? 0) / user.budget) * 100))}%`,
																	}}
																/>
															</div>
														</div>
													) : (
														<span className="text-muted-foreground">
															{user.budget_current_usage != null && user.budget_current_usage > 0
																? `$${user.budget_current_usage.toFixed(2)} (Spent) / Unlimited`
																: "Unlimited"}
														</span>
													)}
												</TableCell>
												<TableCell className="font-mono text-xs">
													{isWorkspaceAdminRole(user.role) ? (
														<span className="text-muted-foreground text-sm">—</span>
													) : user.rate_limit > 0 ? (
														`${user.rate_limit} RPM`
													) : (
														"Unlimited"
													)}
												</TableCell>
												<TableCell className="max-w-[200px] truncate text-xs" title={resolveAllowedPromptNames(user.allowed_prompt_repos)}>
													{isWorkspaceAdminRole(user.role) ? "All" : resolveAllowedPromptNames(user.allowed_prompt_repos)}
												</TableCell>
												<TableCell className="text-muted-foreground text-xs">{new Date(user.created_at).toLocaleDateString()}</TableCell>
												<TableCell className="text-right">
													<div className="flex justify-end gap-2">
														<Button
															size="icon"
															variant="ghost"
															disabled={!hasUpdateAccess || isSelf || actionBusyId === user.id}
															onClick={() => handleBlock(user)}
															title={isSelf ? "You cannot block your own account" : "Block user account"}
															className={cn(
																"text-muted-foreground h-8 w-8 rounded-lg transition-colors",
																isSelf ? "opacity-30 cursor-not-allowed" : "hover:text-amber-400 cursor-pointer",
															)}
															data-testid={`user-block-${user.id}`}
														>
															<Ban className="h-4 w-4" />
														</Button>
														<Button
															size="icon"
															variant="ghost"
															disabled={!hasUpdateAccess}
															onClick={() => openEditModal(user)}
															title={!hasUpdateAccess ? "View-only access: editing is disabled" : "Edit user"}
															className={cn(
																"text-muted-foreground h-8 w-8 rounded-lg transition-colors",
																!hasUpdateAccess ? "opacity-30 cursor-not-allowed" : "hover:text-teal-400 cursor-pointer",
															)}
														>
															<Edit2 className="h-4 w-4" />
														</Button>
														<Button
															size="icon"
															variant="ghost"
															disabled={!hasDeleteAccess || isSelf || actionBusyId === user.id}
															onClick={() => openDeleteModal(user)}
															title={
																isSelf
																	? "You cannot delete your own account"
																	: !hasDeleteAccess
																		? "Delete permission is required"
																		: "Delete user account"
															}
															className={cn(
																"text-muted-foreground h-8 w-8 rounded-lg transition-colors",
																isSelf || !hasDeleteAccess ? "opacity-30 cursor-not-allowed" : "hover:text-red-400 cursor-pointer",
															)}
														>
															<Trash2 className="h-4 w-4" />
														</Button>
													</div>
												</TableCell>
											</TableRow>
										);
									})}
								</TableBody>
							</Table>
							<DataTablePagination
								offset={activeUsersOffset}
								limit={activeUsersLimit}
								totalCount={activeUsers.length}
								onOffsetChange={setActiveUsersOffset}
								onLimitChange={(newLimit) => {
									setActiveUsersLimit(newLimit);
									setActiveUsersOffset(0);
								}}
								itemLabel="users"
								perPageLabel="Users per page"
								dataTestId="users-pagination"
							/>
						</div>
					)}
				</div>
			)}

			{/* Create User Dialog */}
			<Dialog open={isCreateOpen} onOpenChange={setIsCreateOpen}>
				<DialogContent className="bg-background border-border/80 text-foreground sm:max-w-[520px]">
					<DialogHeader>
						<DialogTitle className="flex items-center gap-2 text-teal-400">
							<Users className="h-5 w-5" /> Add New User
						</DialogTitle>
						{userQuota?.limited && (
							<div
								className={cn(
									"mt-2 rounded-lg border p-2.5 text-xs",
									userQuota.remaining <= 0
										? "border-red-500/40 bg-red-500/10 text-red-400"
										: "border-teal-500/30 bg-teal-500/10 text-teal-300",
								)}
							>
								<strong>Enterprise License Quota:</strong> {userQuota.used} of {userQuota.max} accounts allocated (
								{userQuota.remaining > 0 ? `${userQuota.remaining} remaining` : "Capacity full"}).
							</div>
						)}
					</DialogHeader>
					<form onSubmit={handleCreateUser} className="space-y-4 py-4">
						<div className="space-y-2">
							<label className="text-muted-foreground text-sm font-medium">Username</label>
							<Input
								required
								value={username}
								onChange={(e) => setUsername(e.target.value)}
								placeholder="e.g. janesmith"
								className="bg-muted/20 border-border/50 focus:border-teal-500/50"
							/>
						</div>
						<div className="space-y-2">
							<label className="text-muted-foreground text-sm font-medium">Email (required for welcome mail / OTP)</label>
							<Input
								type="email"
								required
								value={email}
								onChange={(e) => setEmail(e.target.value)}
								placeholder="e.g. janesmith@company.com"
								className="bg-muted/20 border-border/50 focus:border-teal-500/50"
							/>
						</div>
						<div className="space-y-2">
							<label className="text-muted-foreground text-sm font-medium">Role</label>
							<p className="text-muted-foreground text-xs">
								Permission role from Roles &amp; Permissions. Team membership is chosen separately below.{" "}
								<a href="/workspace/governance/rbac" className="text-teal-400 underline-offset-2 hover:underline">
									Manage roles
								</a>
							</p>
							<select
								value={role}
								onChange={(e) => {
									const nextRole = e.target.value;
									setRole(nextRole);
									if (nextRole === "admin") {
										setAllowedPromptRepos("");
										setTeamId("");
										setVirtualKeyId("");
										setAllowedSections(allWorkspaceParentGrants());
									} else if (nextRole === "user") {
										setAllowedSections(new Set<WorkspaceGrantKey>(["prompt-repository"]));
									} else if (nextRole === "sub_admin") {
										setAllowedSections(new Set());
									}
								}}
								className="bg-muted/20 border-border/50 text-foreground w-full rounded-lg border p-2.5 text-sm focus:border-teal-500/50 focus:outline-none"
								data-testid="user-role-select-create"
							>
								{roleOptions.length === 0 ? (
									<>
										<option value="user">User</option>
										<option value="sub_admin">Sub-Admin (scoped workspace access)</option>
										<option value="admin">Admin (full workspace access)</option>
									</>
								) : (
									roleOptions.map((r) => (
										<option key={r.id} value={r.name}>
											{r.name === "admin"
												? "Admin (full workspace access)"
												: r.name === "sub_admin"
													? "Sub-Admin (scoped workspace access)"
													: r.name === "user"
														? "User"
														: `${r.name}${r.description ? ` — ${r.description}` : ""}`}
										</option>
									))
								)}
							</select>
						</div>
						{teamPicker}
						{virtualKeyPicker}
						{workspaceAccessPicker}
						{promptReposPicker}
						{canAssignTeamAndKey(role) ? (
							<>
								<div className="grid grid-cols-2 gap-4">
									<div className="space-y-2">
										<label className="text-muted-foreground flex items-center gap-1 text-sm font-medium">
											<DollarSign className="text-muted-foreground/60 h-3.5 w-3.5" /> Budget Limit
										</label>
										<Input
											type="number"
											step="0.01"
											value={budget || ""}
											onChange={(e) => setBudget(parseFloat(e.target.value) || 0)}
											placeholder="USD / Month"
											className="bg-muted/20 border-border/50 focus:border-teal-500/50"
										/>
									</div>
									<div className="space-y-2">
										<label className="text-muted-foreground flex items-center gap-1 text-sm font-medium">
											<Activity className="text-muted-foreground/60 h-3.5 w-3.5" /> Rate Limit
										</label>
										<Input
											type="number"
											value={rateLimit || ""}
											onChange={(e) => setRateLimit(parseInt(e.target.value) || 0)}
											placeholder="RPM Limit"
											className="bg-muted/20 border-border/50 focus:border-teal-500/50"
										/>
									</div>
								</div>
							</>
						) : null}
						<DialogFooter className="pt-4">
							<Button
								type="button"
								variant="outline"
								disabled={isCreating}
								onClick={() => setIsCreateOpen(false)}
								className="border-border/80 text-foreground hover:bg-muted"
							>
								Cancel
							</Button>
							<Button
								type="submit"
								disabled={isCreating || !hasCreateAccess}
								className="bg-teal-500 font-medium text-white hover:bg-teal-600 disabled:opacity-50"
							>
								{isCreating ? (
									<>
										<Loader2 className="mr-2 h-4 w-4 animate-spin" />
										Creating...
									</>
								) : (
									"Create User"
								)}
							</Button>
						</DialogFooter>
					</form>
				</DialogContent>
			</Dialog>

			{/* Edit User Dialog */}
			<Dialog open={isEditOpen} onOpenChange={setIsEditOpen}>
				<DialogContent className="bg-background border-border/80 text-foreground sm:max-w-[520px]">
					<DialogHeader>
						<DialogTitle className="flex items-center gap-2 text-teal-400">
							<Key className="h-5 w-5" /> Edit User Settings
						</DialogTitle>
					</DialogHeader>
					<form onSubmit={handleEditUser} className="space-y-4 py-4">
						{editDialogQueryFailed ? (
							<QueryErrorBanner
								testId="users-edit-dialog-query-error"
								message={getErrorMessage(editDialogQueryError) || "Failed to load team or virtual key assignments."}
							/>
						) : null}
						<div className="space-y-2">
							<label className="text-muted-foreground text-sm font-medium">Username</label>
							<Input
								required
								value={username}
								onChange={(e) => setUsername(e.target.value)}
								placeholder="e.g. janesmith"
								className="bg-muted/20 border-border/50 focus:border-teal-500/50"
							/>
						</div>
						<div className="space-y-2">
							<label className="text-muted-foreground text-sm font-medium">Email (required for welcome mail / OTP)</label>
							<Input
								type="email"
								required
								value={email}
								onChange={(e) => setEmail(e.target.value)}
								placeholder="e.g. janesmith@company.com"
								className="bg-muted/20 border-border/50 focus:border-teal-500/50"
							/>
						</div>
						<div className="border-border/50 bg-muted/10 flex items-center gap-2 rounded-lg border p-3">
							<input
								id="send-temporary-password"
								type="checkbox"
								checked={sendTemporaryPassword}
								onChange={(e) => setSendTemporaryPassword(e.target.checked)}
								className="border-border rounded text-teal-500 focus:ring-teal-500/50"
							/>
							<label htmlFor="send-temporary-password" className="text-foreground cursor-pointer text-sm">
								Email a new temporary password. Their next login must set a new password.
							</label>
						</div>
						<div className="space-y-2">
							<label className="text-muted-foreground text-sm font-medium">Role</label>
							<p className="text-muted-foreground text-xs">
								Permission role from Roles &amp; Permissions. Team membership is chosen separately below.{" "}
								<a href="/workspace/governance/rbac" className="text-teal-400 underline-offset-2 hover:underline">
									Manage roles
								</a>
							</p>
							<select
								value={role}
								onChange={(e) => {
									const nextRole = e.target.value;
									setRole(nextRole);
									if (nextRole === "admin") {
										setAllowedPromptRepos("");
										setTeamId("");
										setVirtualKeyId("");
										setAllowedSections(allWorkspaceParentGrants());
									} else if (nextRole === "user") {
										setAllowedSections((prev) => {
											const next = new Set(prev);
											next.add("prompt-repository");
											return next;
										});
									}
								}}
								className="bg-muted/20 border-border/50 text-foreground w-full rounded-lg border p-2.5 text-sm focus:border-teal-500/50 focus:outline-none"
								data-testid="user-role-select-edit"
							>
								{!roleOptions.some((r) => r.name === role) && role ? (
									<option value={role}>{roleLabel(role)} (current)</option>
								) : null}
								{roleOptions.length === 0 ? (
									<>
										<option value="user">User</option>
										<option value="sub_admin">Sub-Admin (scoped workspace access)</option>
										<option value="admin">Admin (full workspace access)</option>
									</>
								) : (
									roleOptions.map((r) => (
										<option key={r.id} value={r.name}>
											{r.name === "admin"
												? "Admin (full workspace access)"
												: r.name === "sub_admin"
													? "Sub-Admin (scoped workspace access)"
													: r.name === "user"
														? "User"
														: `${r.name}${r.description ? ` — ${r.description}` : ""}`}
										</option>
									))
								)}
							</select>
						</div>
						{teamPicker}
						{virtualKeyPicker}
						{workspaceAccessPicker}
						{promptReposPicker}
						{canAssignTeamAndKey(role) ? (
							<div className="grid grid-cols-2 gap-4">
								<div className="space-y-2">
									<label className="text-muted-foreground flex items-center gap-1 text-sm font-medium">
										<DollarSign className="text-muted-foreground/60 h-3.5 w-3.5" /> Budget Limit
									</label>
									<Input
										type="number"
										step="0.01"
										value={budget || ""}
										onChange={(e) => setBudget(parseFloat(e.target.value) || 0)}
										placeholder="USD / Month"
										className="bg-muted/20 border-border/50 focus:border-teal-500/50"
									/>
								</div>
								<div className="space-y-2">
									<label className="text-muted-foreground flex items-center gap-1 text-sm font-medium">
										<Activity className="text-muted-foreground/60 h-3.5 w-3.5" /> Rate Limit
									</label>
									<Input
										type="number"
										value={rateLimit || ""}
										onChange={(e) => setRateLimit(parseInt(e.target.value) || 0)}
										placeholder="RPM Limit"
										className="bg-muted/20 border-border/50 focus:border-teal-500/50"
									/>
								</div>
							</div>
						) : null}
						<DialogFooter className="pt-4">
							<Button
								type="button"
								variant="outline"
								disabled={isUpdating}
								onClick={() => setIsEditOpen(false)}
								className="border-border/80 text-foreground hover:bg-muted"
							>
								Cancel
							</Button>
							<Button
								type="submit"
								disabled={isUpdating || !hasUpdateAccess}
								className="bg-teal-500 font-medium text-white hover:bg-teal-600 disabled:opacity-50"
							>
								{isUpdating ? (
									<>
										<Loader2 className="mr-2 h-4 w-4 animate-spin" />
										Saving...
									</>
								) : (
									"Save Changes"
								)}
							</Button>
						</DialogFooter>
					</form>
				</DialogContent>
			</Dialog>

			{/* Delete User Confirm Dialog */}
			<Dialog open={isDeleteOpen} onOpenChange={setIsDeleteOpen}>
				<DialogContent className="bg-background border-border/80 text-foreground sm:max-w-[400px]">
					<DialogHeader>
						<DialogTitle className="flex items-center gap-2 text-red-400">
							<Trash2 className="h-5 w-5" /> Delete User Account
						</DialogTitle>
					</DialogHeader>
					<div className="py-4">
						<p className="text-muted-foreground text-sm">
							Are you sure you want to permanently delete the user account{" "}
							<span className="text-foreground font-semibold">"{selectedUser?.username}"</span>? This action cannot be undone and they will
							lose access immediately.
						</p>
					</div>
					<DialogFooter>
						<Button
							variant="outline"
							disabled={isDeleting}
							onClick={() => setIsDeleteOpen(false)}
							className="border-border/80 text-foreground hover:bg-muted"
						>
							Cancel
						</Button>
						<Button
							onClick={handleDeleteUser}
							disabled={isDeleting || !hasDeleteAccess}
							className="bg-red-500 font-medium text-white hover:bg-red-600 disabled:opacity-50"
						>
							{isDeleting ? (
								<>
									<Loader2 className="mr-2 h-4 w-4 animate-spin" />
									Deleting...
								</>
							) : (
								"Delete Account"
							)}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* Bulk Delete Users Confirm Dialog */}
			<Dialog open={isBulkDeleteUsersOpen} onOpenChange={setIsBulkDeleteUsersOpen}>
				<DialogContent className="bg-background border-border/80 text-foreground sm:max-w-[400px]">
					<DialogHeader>
						<DialogTitle className="flex items-center gap-2 text-red-400">
							<Trash2 className="h-5 w-5" /> Delete Selected Users
						</DialogTitle>
					</DialogHeader>
					<div className="py-4">
						<p className="text-muted-foreground text-sm">
							Are you sure you want to permanently delete {selectedUserCount} selected user account{selectedUserCount > 1 ? "s" : ""}? They will lose access immediately. This action cannot be undone.
						</p>
					</div>
					<DialogFooter>
						<Button
							variant="outline"
							disabled={isBulkDeletingUsers}
							onClick={() => setIsBulkDeleteUsersOpen(false)}
							className="border-border/80 text-foreground hover:bg-muted"
						>
							Cancel
						</Button>
						<Button
							onClick={handleBulkDeleteUsers}
							disabled={isBulkDeletingUsers || selectedUserCount === 0 || !hasDeleteAccess}
							className="bg-red-500 font-medium text-white hover:bg-red-600 disabled:opacity-50"
							data-testid="users-bulk-delete-confirm-btn"
						>
							{isBulkDeletingUsers ? (
								<>
									<Loader2 className="mr-2 h-4 w-4 animate-spin" />
									Deleting...
								</>
							) : (
								"Delete Selected"
							)}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}