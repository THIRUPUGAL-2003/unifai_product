import { useEffect, useRef, useState } from "react";
import {
	Users,
	Plus,
	Search,
	Edit2,
	Trash2,
	Shield,
	Check,
	X,
	Clock,
	Key,
	DollarSign,
	Activity,
	ChevronDown,
	ChevronRight,
	Loader2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { toast } from "sonner";
import {
	WORKSPACE_ACCESS_SECTIONS,
	adminSectionsFromStorage,
	allowedSectionsToString,
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
	useGetPromptsQuery,
	useGetSessionUsersQuery,
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
import type { VirtualKey } from "@/lib/types/governance";

function IndeterminateCheckbox({
	checked,
	indeterminate,
	onChange,
}: {
	checked: boolean;
	indeterminate: boolean;
	onChange: (checked: boolean) => void;
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
			onChange={(e) => onChange(e.target.checked)}
			className="border-border rounded text-teal-500 focus:ring-teal-500/50"
		/>
	);
}

// Server-side: admin/sub_admin see every prompt; all other roles are filtered by allowed_prompt_repos.
function isWorkspaceAdminRole(role: string): boolean {
	return role === "admin" || role === "sub_admin";
}

// Built-in "user" is server-locked to Prompt Repository and admins are unrestricted,
// so section grants never apply to either.
function sectionGrantsApply(role: string): boolean {
	return role !== "user" && role !== "admin";
}

function UserTeamCell({ userId }: { userId: string }) {
	const { data } = useGetUserTeamsQuery(userId);
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

// Direct, team and customer keys: team/customer keys reach the user without a direct assignment.
function UserVirtualKeysCell({ userId, allVirtualKeys }: { userId: string; allVirtualKeys: VirtualKey[] }) {
	const { data } = useGetUserVirtualKeysQuery(userId);
	const keys = (data?.virtual_keys ?? []).filter((vk) => vk.is_active !== false);
	if (keys.length === 0) {
		return <span className="text-muted-foreground text-xs italic">No key</span>;
	}
	return (
		<div className="flex flex-col gap-1">
			{keys.map((vk) => {
				const budgets = (allVirtualKeys.find((k) => k.id === vk.id)?.budgets ?? []).filter((b) => b.max_limit > 0);
				return (
					<div key={vk.id} className="flex flex-col">
						<span className="text-sm">
							{vk.name}
							{vk.origin === "team" || vk.origin === "customer" ? (
								<span className="text-muted-foreground text-xs">
									{" "}
									({vk.origin === "team" ? "Team" : "Customer"}: {vk.origin_name})
								</span>
							) : null}
						</span>
						{budgets.map((b) => (
							<span
								key={b.id || b.reset_duration}
								className={cn(
									"font-mono text-[11px]",
									(b.current_usage ?? 0) >= b.max_limit ? "text-rose-500" : "text-muted-foreground",
								)}
							>
								${(b.current_usage ?? 0).toFixed(2)} / ${b.max_limit.toFixed(2)} ({b.reset_duration})
							</span>
						))}
					</div>
				);
			})}
		</div>
	);
}

function passwordPolicyFailures(password: string): string[] {
	const fails: string[] = [];
	if (password.length < 8) fails.push("at least 8 characters");
	if (!/[A-Z]/.test(password)) fails.push("one uppercase letter");
	if (!/[a-z]/.test(password)) fails.push("one lowercase letter");
	if (!/\d/.test(password)) fails.push("one number");
	if (!/[^A-Za-z0-9]/.test(password)) fails.push("one special character");
	return fails;
}

export default function UsersView() {
	const [searchQuery, setSearchQuery] = useState("");
	const [roleFilter, setRoleFilter] = useState<"all" | "user" | "admin">("all");
	const [actionBusyId, setActionBusyId] = useState<string | null>(null);
	const hasCreateAccess = useRbac(RbacResource.Users, RbacOperation.Create);
	const hasUpdateAccess = useRbac(RbacResource.Users, RbacOperation.Update);
	const hasDeleteAccess = useRbac(RbacResource.Users, RbacOperation.Delete);

	const {
		data: users = [],
		isLoading: loading,
		isError: usersError,
		error: usersErrorDetail,
		refetch: refetchUsers,
	} = useGetSessionUsersQuery();
	const { data: promptsData } = useGetPromptsQuery();
	const { data: teamsData } = useGetTeamsQuery({ limit: 500, offset: 0 });
	const teams = teamsData?.teams || [];
	const { data: virtualKeysData } = useGetVirtualKeysQuery({ limit: 500, offset: 0 });
	const virtualKeys = (virtualKeysData?.virtual_keys || []).filter((vk) => vk.is_active !== false);
	const { data: rolesData } = useGetRolesQuery();
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
	const [password, setPassword] = useState("");
	const [confirmPassword, setConfirmPassword] = useState("");
	const [role, setRole] = useState("user");
	const [teamId, setTeamId] = useState("");
	const [initialTeamId, setInitialTeamId] = useState("");
	const [virtualKeyId, setVirtualKeyId] = useState("");
	const [initialVirtualKeyId, setInitialVirtualKeyId] = useState("");
	const [budget, setBudget] = useState(0);
	const [rateLimit, setRateLimit] = useState(0);
	const [allowedPromptRepos, setAllowedPromptRepos] = useState("");
	const [allowedSections, setAllowedSections] = useState<Set<WorkspaceGrantKey>>(new Set());
	const [expandedSections, setExpandedSections] = useState<Set<WorkspaceSectionKey>>(new Set());
	const [autoCreatePrompt, setAutoCreatePrompt] = useState(false);

	const { data: editUserTeams } = useGetUserTeamsQuery(selectedUser?.id || "", {
		skip: !selectedUser?.id || !isEditOpen,
	});
	const { data: editUserVKs } = useGetUserVirtualKeysQuery(selectedUser?.id || "", {
		skip: !selectedUser?.id || !isEditOpen,
	});

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
		setAllowedSections((prev) => {
			const next = new Set(prev);
			clearSectionGrants(next, section);
			if (checked) {
				next.add(section.key);
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
		role !== "admin" ? (
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
		role !== "admin" ? (
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

	const workspaceAccessPicker =
		sectionGrantsApply(role) ? (
			<div className="space-y-2">
				<label className="text-muted-foreground text-sm font-medium">Workspace Access</label>
				<p className="text-muted-foreground text-xs">
					Expand a section to pick pages. Parent tick = whole section. Nothing is granted by default —
					unchecked sections stay hidden for this user.
				</p>
				<div className="border-border/50 bg-muted/10 max-h-64 space-y-1 overflow-y-auto rounded-lg border p-2">
					{WORKSPACE_ACCESS_SECTIONS.map((section) => {
						const hasChildren = !!section.items?.length;
						const expanded = expandedSections.has(section.key);
						const sel = sectionSelectionState(section, allowedSections);
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
									<label className="text-foreground flex min-w-0 flex-1 cursor-pointer items-center gap-2 text-sm select-none">
										<IndeterminateCheckbox
											checked={sel === "all"}
											indeterminate={sel === "some"}
											onChange={(checked) => toggleSection(section, checked)}
										/>
										<span className="truncate font-medium">{section.label}</span>
									</label>
								</div>
								{hasChildren && expanded ? (
									<div className="border-border/40 ml-6 space-y-0.5 border-l py-1 pl-3">
										{section.items!.map((item) => (
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
					})}
				</div>
			</div>
		) : null;

	const promptReposPicker =
		!isWorkspaceAdminRole(role) ? (
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

	const userPayload = () => ({
		username,
		email: email.trim() || undefined,
		password: password || undefined,
		role,
		budget,
		rate_limit: rateLimit,
		allowed_prompt_repos: !isWorkspaceAdminRole(role) ? sanitizeAllowedPromptRepos(allowedPromptRepos) : "",
		allowed_sections: sectionGrantsApply(role) ? allowedSectionsToString(allowedSections) : "",
		auto_create_prompt: autoCreatePrompt,
	});

	const handleCreateUser = async (e: React.FormEvent) => {
		e.preventDefault();
		if (isCreating) return;
		if (!username || !password) {
			toast.error("Username and password are required");
			return;
		}
		if (password !== confirmPassword) {
			toast.error("Passwords do not match");
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
		const policyFails = passwordPolicyFailures(password);
		if (policyFails.length > 0) {
			toast.error("Password must include " + policyFails.join(", "));
			return;
		}
		setIsCreating(true);
		try {
			// The create endpoint validates and stores the role (incl. admin-only checks).
			const created = await createUser({ ...userPayload(), password }).unwrap();
			if (created?.id && role !== "admin") {
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
				toast.success("User created — welcome email sent. Share the password with the user separately.");
			} else if (created?.email_error) {
				toast.warning(`User created, but email failed: ${created.email_error}`);
			} else {
				toast.success("User created successfully (no welcome email — check SMTP / Email on user create)");
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
		if (password) {
			if (password !== confirmPassword) {
				toast.error("Passwords do not match");
				return;
			}
			const policyFails = passwordPolicyFailures(password);
			if (policyFails.length > 0) {
				toast.error("Password must include " + policyFails.join(", "));
				return;
			}
		}
		setIsUpdating(true);
		try {
			await updateUser({ id: selectedUser.id, updates: userPayload() }).unwrap();
			if (role !== "admin") {
				await syncUserTeam(selectedUser.id, teamId, initialTeamId);
				await syncUserVirtualKey(selectedUser.id, virtualKeyId, initialVirtualKeyId);
				if (!virtualKeyId && role === "user" && inheritedVirtualKeys.length === 0) {
					toast.warning("Saved without a Virtual Key — Prompt Repository chat stays blocked until you assign one.");
				}
			} else if (initialTeamId) {
				await syncUserTeam(selectedUser.id, "", initialTeamId);
			}
			if (role === "admin" && initialVirtualKeyId) {
				await syncUserVirtualKey(selectedUser.id, "", initialVirtualKeyId);
			}
			toast.success("User updated successfully");
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

	const resetForm = () => {
		setUsername("");
		setEmail("");
		setPassword("");
		setConfirmPassword("");
		setRole("user");
		setTeamId("");
		setInitialTeamId("");
		setVirtualKeyId("");
		setInitialVirtualKeyId("");
		setBudget(0);
		setRateLimit(0);
		setAllowedPromptRepos("");
		setAllowedSections(new Set());
		setExpandedSections(new Set());
		setAutoCreatePrompt(false);
		setSelectedUser(null);
	};

	const openEditModal = (user: SessionUser) => {
		setSelectedUser(user);
		setUsername(user.username);
		setEmail(user.email || "");
		setPassword("");
		setConfirmPassword("");
		const nextRole = (user.role || "user").trim() || "user";
		setRole(nextRole);
		setTeamId("");
		setInitialTeamId("");
		setVirtualKeyId("");
		setInitialVirtualKeyId("");
		setBudget(user.budget);
		setRateLimit(user.rate_limit);
		setAllowedPromptRepos(sanitizeAllowedPromptRepos(user.allowed_prompt_repos || ""));
		const grants = sectionGrantsApply(nextRole) ? adminSectionsFromStorage(user.allowed_sections) : new Set<WorkspaceGrantKey>();
		setAllowedSections(grants);
		setExpandedSections(
			new Set(
				WORKSPACE_ACCESS_SECTIONS.filter(
					(s) => s.items?.length && sectionSelectionState(s, grants) !== "none",
				).map((s) => s.key),
			),
		);
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

	const isAdminRole = (u: SessionUser) => {
		const r = (u.role || "user").trim();
		return r === "admin" || r === "sub_admin";
	};

	const matchesRole = (u: SessionUser) => {
		if (roleFilter === "admin") return isAdminRole(u);
		if (roleFilter === "user") return !isAdminRole(u);
		return true;
	};

	// Disabled = deactivated by the identity provider (SCIM); listed with a badge, cannot sign in.
	const approvedUsers = users.filter((u) => ["approved", "disabled"].includes(u.status || "approved"));
	const roleCounts = {
		all: approvedUsers.length,
		user: approvedUsers.filter((u) => !isAdminRole(u)).length,
		admin: approvedUsers.filter(isAdminRole).length,
	};
	const roleFilterOptions: { value: "all" | "user" | "admin"; label: string }[] = [
		{ value: "all", label: "All" },
		{ value: "user", label: "User" },
		{ value: "admin", label: "Admin" },
	];

	// email_unverified sign-ups are listed too (badged) so admins can see and clean up stuck requests.
	const pendingUsers = users.filter(
		(u) => ["pending", "email_unverified"].includes(u.status || "approved") && matchesSearch(u) && matchesRole(u),
	);
	const activeUsers = approvedUsers.filter((u) => matchesSearch(u) && matchesRole(u));
	const activeUsersTitle =
		roleFilter === "admin" ? "Active admins" : roleFilter === "user" ? "Active users (role: User)" : "Active users";

	return (
		<div className="text-foreground bg-background border-border/40 flex w-full flex-col gap-6 rounded-lg border p-6 shadow-xl backdrop-blur-sm">
			{/* Header Section */}
			<div className="border-border/40 flex flex-col items-start justify-between gap-4 border-b pb-6 md:flex-row md:items-center">
				<div>
					<h1 className="text-primary flex items-center gap-2 text-2xl font-semibold tracking-tight">
						<Users className="h-6 w-6 text-teal-400" />
						User Governance
					</h1>
				</div>
				<Button
					disabled={!hasCreateAccess}
					onClick={() => {
						resetForm();
						setIsCreateOpen(true);
					}}
					className="flex items-center gap-2 rounded-lg bg-teal-500 px-4 py-2 font-medium text-white shadow-lg shadow-teal-500/20 transition-all duration-200 hover:bg-teal-600 active:scale-95"
				>
					<Plus className="h-4 w-4" /> Add New User
				</Button>
			</div>

			{/* Search + role filter */}
			<div className="flex w-full flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
				<div className="relative w-full max-w-sm">
					<Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
					<Input
						placeholder="Search by name, email or role..."
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
					className="bg-muted/30 border-border/50 inline-flex shrink-0 items-center gap-1 rounded-lg border p-1"
				>
					{roleFilterOptions.map((opt) => {
						const active = roleFilter === opt.value;
						return (
							<button
								key={opt.value}
								type="button"
								onClick={() => setRoleFilter(opt.value)}
								aria-pressed={active}
								data-testid={`users-role-filter-${opt.value}`}
								className={`inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
									active
										? "bg-teal-500 text-white shadow-sm"
										: "text-muted-foreground hover:bg-muted hover:text-foreground"
								}`}
							>
								{opt.value === "admin" ? <Shield className="h-3.5 w-3.5" /> : opt.value === "user" ? <Users className="h-3.5 w-3.5" /> : null}
								{opt.label}
								<span
									className={`rounded-full px-1.5 text-[10px] font-semibold ${
										active ? "bg-white/20 text-white" : "bg-muted text-muted-foreground"
									}`}
								>
									{roleCounts[opt.value]}
								</span>
							</button>
						);
					})}
				</div>
			</div>

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
			) : (
				<>
					{/* Pending registrations */}
					<div className="space-y-3">
						<div className="flex items-center gap-2">
							<Clock className="h-4 w-4 text-amber-400" />
							<h2 className="text-sm font-semibold text-amber-300">
								Pending approvals ({pendingUsers.length})
							</h2>
						</div>
						{pendingUsers.length === 0 ? (
							<p className="text-muted-foreground text-sm pl-6">No registration requests waiting.</p>
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
														className={`inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-medium ${
															user.role === "admin"
																? "border-teal-500/20 bg-teal-500/10 text-teal-400"
																: "border-blue-500/20 bg-blue-500/10 text-blue-400"
														}`}
													>
														<Shield className="h-3 w-3" />
														{user.role}
													</span>
												</TableCell>
												<TableCell className="text-muted-foreground text-xs">
													{new Date(user.created_at).toLocaleString()}
												</TableCell>
												<TableCell className="text-right">
													<div className="flex justify-end gap-2">
														<Button
															size="sm"
															disabled={actionBusyId === user.id || !hasUpdateAccess}
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

					{/* Active users */}
					<div className="space-y-3">
						<h2 className="text-sm font-semibold text-foreground/90">
							{activeUsersTitle} ({activeUsers.length})
						</h2>
						{activeUsers.length === 0 ? (
							<div className="border-border/40 bg-muted/10 flex flex-col items-center justify-center rounded-xl border border-dashed py-16 text-center">
								<Users className="text-muted-foreground/60 mb-3 h-12 w-12" />
								<p className="text-foreground/80 text-base font-medium">No Active Users</p>
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
										{activeUsers.map((user) => (
											<TableRow key={user.id} className="hover:bg-muted/20 transition-colors">
												<TableCell className="flex items-center gap-2 font-medium">
													<div className="flex h-8 w-8 items-center justify-center rounded-full bg-teal-500/10 text-xs font-bold text-teal-400 uppercase">
														{user.username.slice(0, 2)}
													</div>
													{user.username}
													{user.status === "disabled" && (
														<span className="rounded-full border border-rose-500/30 bg-rose-500/10 px-2 py-0.5 text-[10px] font-medium text-rose-400">
															Disabled
														</span>
													)}
												</TableCell>
												<TableCell className="text-sm text-muted-foreground">{user.email || "—"}</TableCell>
												<TableCell>
													<span
														className={`inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-medium ${
															user.role === "admin"
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
													{user.role === "admin" ? (
														<span className="text-muted-foreground text-sm">—</span>
													) : (
														<UserTeamCell userId={user.id} />
													)}
												</TableCell>
												<TableCell>
													{user.role === "admin" ? (
														<span className="text-muted-foreground text-sm">—</span>
													) : (
														<UserVirtualKeysCell userId={user.id} allVirtualKeys={virtualKeys} />
													)}
												</TableCell>
												<TableCell className="font-mono text-xs">
													{user.budget > 0 ? (
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
												<TableCell className="font-mono text-xs">{user.rate_limit > 0 ? `${user.rate_limit} RPM` : "Unlimited"}</TableCell>
												<TableCell className="max-w-[200px] truncate text-xs" title={resolveAllowedPromptNames(user.allowed_prompt_repos)}>
													{isWorkspaceAdminRole(user.role) ? "All" : resolveAllowedPromptNames(user.allowed_prompt_repos)}
												</TableCell>
												<TableCell className="text-muted-foreground text-xs">{new Date(user.created_at).toLocaleDateString()}</TableCell>
												<TableCell className="text-right">
													<div className="flex justify-end gap-2">
														<Button
															size="icon"
															variant="ghost"
															disabled={!hasUpdateAccess}
															onClick={() => openEditModal(user)}
															className="text-muted-foreground h-8 w-8 rounded-lg transition-colors hover:text-teal-400"
														>
															<Edit2 className="h-4 w-4" />
														</Button>
														<Button
															size="icon"
															variant="ghost"
															disabled={!hasDeleteAccess}
															onClick={() => openDeleteModal(user)}
															className="text-muted-foreground h-8 w-8 rounded-lg transition-colors hover:text-red-400"
														>
															<Trash2 className="h-4 w-4" />
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
				</>
			)}

			{/* Create User Dialog */}
			<Dialog open={isCreateOpen} onOpenChange={setIsCreateOpen}>
				<DialogContent className="bg-background border-border/80 text-foreground sm:max-w-[520px]">
					<DialogHeader>
						<DialogTitle className="flex items-center gap-2 text-teal-400">
							<Users className="h-5 w-5" /> Add New User
						</DialogTitle>
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
							<label className="text-muted-foreground text-sm font-medium">Password</label>
							<Input
								type="password"
								required
								value={password}
								onChange={(e) => setPassword(e.target.value)}
								placeholder="Min 8 chars, 1 uppercase, 1 symbol"
								className="bg-muted/20 border-border/50 focus:border-teal-500/50"
							/>
						</div>
						<div className="space-y-2">
							<label className="text-muted-foreground text-sm font-medium">Confirm Password</label>
							<Input
								type="password"
								required
								value={confirmPassword}
								onChange={(e) => setConfirmPassword(e.target.value)}
								placeholder="Re-enter password"
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
									} else if (nextRole === "sub_admin") {
										setAllowedPromptRepos("");
									}
									if (!sectionGrantsApply(nextRole)) {
										setAllowedSections(new Set());
										setExpandedSections(new Set());
									}
								}}
								className="bg-muted/20 border-border/50 text-foreground w-full rounded-lg border p-2.5 text-sm focus:border-teal-500/50 focus:outline-none"
								data-testid="user-role-select-create"
							>
								{roleOptions.length === 0 ? (
									<>
										<option value="user">User (Prompt Repository only)</option>
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
														? "User (Prompt Repository only)"
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
						<div className="border-border/50 bg-muted/10 flex items-center gap-2 rounded-lg border p-3">
							<input
								id="auto-create-prompt"
								type="checkbox"
								checked={autoCreatePrompt}
								onChange={(e) => setAutoCreatePrompt(e.target.checked)}
								className="border-border rounded text-teal-500 focus:ring-teal-500/50"
							/>
							<div className="flex flex-col">
								<label htmlFor="auto-create-prompt" className="text-foreground cursor-pointer text-sm font-medium">
									Auto-create Prompt Repository workspace (Optional)
								</label>
								<span className="text-muted-foreground text-xs">
									Optionally generates a personal prompt repository for this user with pre-configured model parameters.
								</span>
							</div>
						</div>
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
							<label className="text-muted-foreground text-sm font-medium">New Password (Leave blank to keep same)</label>
							<Input
								type="password"
								value={password}
								onChange={(e) => setPassword(e.target.value)}
								placeholder="••••••••"
								className="bg-muted/20 border-border/50 focus:border-teal-500/50"
							/>
						</div>
						{password && (
							<div className="space-y-2">
								<label className="text-muted-foreground text-sm font-medium">Confirm New Password</label>
								<Input
									type="password"
									value={confirmPassword}
									onChange={(e) => setConfirmPassword(e.target.value)}
									placeholder="Re-enter new password"
									className="bg-muted/20 border-border/50 focus:border-teal-500/50"
								/>
							</div>
						)}
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
									} else if (nextRole === "sub_admin") {
										setAllowedPromptRepos("");
									}
									if (!sectionGrantsApply(nextRole)) {
										setAllowedSections(new Set());
										setExpandedSections(new Set());
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
										<option value="user">User (Prompt Repository only)</option>
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
														? "User (Prompt Repository only)"
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
		</div>
	);
}