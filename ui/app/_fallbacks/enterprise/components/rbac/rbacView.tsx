import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
	getErrorMessage,
	useGetCustomersQuery,
	useGetSessionUsersQuery,
	useGetTeamsQuery,
	useUpdateSessionUserMutation,
	type SessionUser,
} from "@/lib/store";
import type { Customer, Team } from "@/lib/types/governance";
import {
	useAssignUserRoleMutation,
	useCreateRoleMutation,
	useDeleteRoleMutation,
	useGetPermissionsQuery,
	useGetRolePermissionsQuery,
	useGetRolesQuery,
	useUpdateRolePermissionsMutation,
} from "@enterprise/lib/store/apis/rbacApi";
import { RBACPermission, RBACRole } from "@enterprise/lib/types/workspace";
import {
	Building2,
	Check,
	ChevronDown,
	ChevronRight,
	Eye,
	Globe,
	Info,
	Landmark,
	Plus,
	Search,
	Shield,
	Trash2,
	User,
	Users,
	X,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

// Selection Target Type
type SelectionTarget =
	| { type: "role"; role: RBACRole }
	| { type: "all_users" }
	| { type: "user"; user: SessionUser }
	| { type: "all_teams" }
	| { type: "team"; team: Team }
	| { type: "all_customers" }
	| { type: "customer"; customer: Customer };

// Resource to Section Mapping for automatic sidebar permission syncing
const RESOURCE_TO_SECTION_MAP: Record<string, string[]> = {
	Logs: ["observability/llm-logs", "observability"],
	Observability: ["observability", "observability/connectors"],
	Dashboard: ["observability/dashboard"],
	MCPLogs: ["observability/mcp-logs"],
	VirtualKeys: ["governance/virtual-keys", "governance"],
	Governance: ["governance"],
	Users: ["governance/users", "governance"],
	Teams: ["governance/teams", "governance"],
	Customers: ["governance/customers", "governance"],
	UserProvisioning: ["governance/user-provisioning", "governance"],
	RBAC: ["governance/roles-permissions", "governance"],
	AccessProfiles: ["governance/access-profiles", "governance"],
	AuditLogs: ["governance/audit-logs", "governance"],
	ModelProvider: ["models/model-providers", "models"],
	RoutingRules: ["models/routing-rules", "models"],
	CircuitBreaker: ["models/circuit-breaker", "models"],
	MCPGateway: ["mcp-gateway/mcp-catalog", "mcp-gateway"],
	MCPToolGroups: ["mcp-gateway/tool-groups", "mcp-gateway"],
	Plugins: ["plugins"],
	GuardrailsConfig: ["guardrails/rules", "guardrails"],
	GuardrailsProviders: ["guardrails/providers", "guardrails"],
	Cluster: ["guardrails/cluster-config", "cluster-config"],
	Settings: ["settings"],
	FeatureFlags: ["settings/feature-flags"],
	APIKeys: ["settings/api-keys"],
	AdaptiveRouter: ["adaptive-routing"],
	PromptRepository: ["prompt-repository"],
	PromptDeploymentStrategy: ["prompt-repository"],
	SkillsRepository: ["skills-repository"],
};

// allowed_sections only scopes sub_admin and custom roles: admin is unrestricted and the
// built-in "user" role is server-locked to Prompt Repository.
function sectionScopeApplies(role?: string): boolean {
	const r = (role || "user").toLowerCase();
	return r !== "admin" && r !== "user";
}

export default function RBACView() {
	// Navigation Tree Accordion States
	const [expandedSections, setExpandedSections] = useState<{
		roles: boolean;
		users: boolean;
		teams: boolean;
		customers: boolean;
	}>({
		roles: true,
		users: true,
		teams: true,
		customers: false,
	});

	// Current Target Selection
	const [target, setTarget] = useState<SelectionTarget>({
		type: "role",
		role: { id: 0, name: "sub_admin", description: "Sub administrator with configured permissions", dac: "all-data", is_system_role: true },
	});

	// Checkbox state for the active permission matrix
	const [selectedPerms, setSelectedPerms] = useState<number[]>([]);

	// Search filters in the left pane
	const [treeSearch, setTreeSearch] = useState("");
	const [resourceSearch, setResourceSearch] = useState("");

	// Create Role Dialog
	const [createRoleOpen, setCreateRoleOpen] = useState(false);
	const [newRoleName, setNewRoleName] = useState("");
	const [newRoleDesc, setNewRoleDesc] = useState("");
	const [newRoleDac, setNewRoleDac] = useState("all-data");

	// Queries
	const { data: roleData, isLoading: rolesLoading } = useGetRolesQuery();
	const { data: permData } = useGetPermissionsQuery();
	const { data: sessionUsers = [], isLoading: usersLoading } = useGetSessionUsersQuery();
	const { data: teamsData, isLoading: teamsLoading } = useGetTeamsQuery();
	const { data: customersData, isLoading: customersLoading } = useGetCustomersQuery();

	const roles = roleData?.roles || [];
	const permissions = permData?.permissions || [];
	const teams = teamsData?.teams || [];
	const customers = customersData?.customers || [];

	// The tree hands us a snapshot; read role / allowed_sections from the live list so role
	// changes and saved sections are reflected without re-selecting the user.
	const targetUser = useMemo(
		() => (target.type === "user" ? sessionUsers.find((u) => u.id === target.user.id) ?? target.user : null),
		[target, sessionUsers],
	);

	// Determine active role ID for fetching role permissions query
	const activeRoleId = useMemo(() => {
		if (target.type === "role") return target.role.id;
		if (target.type === "all_users") {
			const userRole = roles.find((r) => r.name.toLowerCase() === "user");
			return userRole?.id ?? 0;
		}
		if (target.type === "user") {
			const matchingRole = roles.find((r) => r.name.toLowerCase() === (targetUser?.role || "user").toLowerCase());
			return matchingRole?.id ?? 0;
		}
		if (target.type === "all_teams" || target.type === "team") {
			const subAdminRole = roles.find((r) => r.name.toLowerCase() === "sub_admin");
			return subAdminRole?.id ?? 0;
		}
		return 0;
	}, [target, roles, targetUser]);

	const { currentData: rolePermData } = useGetRolePermissionsQuery(activeRoleId, { skip: activeRoleId === 0 });

	// Mutations
	const [createRole, { isLoading: isCreatingRole }] = useCreateRoleMutation();
	const [updatePerms, { isLoading: isUpdatingPerms }] = useUpdateRolePermissionsMutation();
	const [deleteRole] = useDeleteRoleMutation();
	const [assignUserRole] = useAssignUserRoleMutation();
	const [updateSessionUser] = useUpdateSessionUserMutation();

	// Initialize selected role if none set
	useEffect(() => {
		if (roles.length > 0 && target.type === "role" && target.role.id === 0) {
			const subAdmin = roles.find((r) => r.name?.toLowerCase() === "sub_admin") || roles[0];
			setTarget({ type: "role", role: subAdmin });
		}
	}, [roles, target]);

	// Sync permissions when target changes
	useEffect(() => {
		if (target.type === "role" && target.role.name?.toLowerCase() === "admin") {
			setSelectedPerms(permissions.map((p) => p.id));
			return;
		}
		if (rolePermData?.permissions) {
			let granted = rolePermData.permissions;
			// A scoped user sees their role's permissions narrowed to the sections saved for them.
			if (target.type === "user" && sectionScopeApplies(targetUser?.role) && targetUser?.allowed_sections?.trim()) {
				const allowed = new Set(
					targetUser.allowed_sections
						.split(",")
						.map((s) => s.trim())
						.filter(Boolean),
				);
				granted = granted.filter((p) => {
					const sections = RESOURCE_TO_SECTION_MAP[p.resource] || [];
					return sections.length === 0 || sections.some((s) => allowed.has(s));
				});
			}
			setSelectedPerms(granted.map((p) => p.id));
		} else if (activeRoleId === 0) {
			setSelectedPerms([]);
		}
	}, [target, targetUser, rolePermData, permissions, activeRoleId]);

	// Toggle accordion
	const toggleSection = (section: keyof typeof expandedSections) => {
		setExpandedSections((prev) => ({ ...prev, [section]: !prev[section] }));
	};

	// Group permissions by Resource
	const groupedPermissions = useMemo(() => {
		const map = new Map<string, RBACPermission[]>();
		for (const perm of permissions) {
			if (resourceSearch.trim()) {
				const q = resourceSearch.toLowerCase();
				const matchResource = perm.resource.toLowerCase().includes(q);
				const matchOp = perm.operation.toLowerCase().includes(q);
				if (!matchResource && !matchOp) continue;
			}
			const list = map.get(perm.resource) || [];
			list.push(perm);
			map.set(perm.resource, list);
		}
		return Array.from(map.entries());
	}, [permissions, resourceSearch]);

	// Filtered lists for left pane tree
	const filteredRoles = useMemo(() => {
		if (!treeSearch.trim()) return roles;
		const q = treeSearch.toLowerCase();
		return roles.filter((r) => r.name.toLowerCase().includes(q) || r.description?.toLowerCase().includes(q));
	}, [roles, treeSearch]);

	const filteredUsers = useMemo(() => {
		if (!treeSearch.trim()) return sessionUsers;
		const q = treeSearch.toLowerCase();
		return sessionUsers.filter(
			(u) =>
				u.username.toLowerCase().includes(q) ||
				u.email?.toLowerCase().includes(q) ||
				u.role?.toLowerCase().includes(q),
		);
	}, [sessionUsers, treeSearch]);

	const filteredTeams = useMemo(() => {
		if (!treeSearch.trim()) return teams;
		const q = treeSearch.toLowerCase();
		return teams.filter((t) => t.name.toLowerCase().includes(q) || t.id.toLowerCase().includes(q));
	}, [teams, treeSearch]);

	const filteredCustomers = useMemo(() => {
		if (!treeSearch.trim()) return customers;
		const q = treeSearch.toLowerCase();
		return customers.filter((c) => c.name.toLowerCase().includes(q) || c.id.toLowerCase().includes(q));
	}, [customers, treeSearch]);

	// Calculate allowed sections string based on selected permissions
	const computeAllowedSections = (permIds: number[]): string => {
		const grantedPerms = permissions.filter((p) => permIds.includes(p.id));
		const sections = new Set<string>();
		for (const p of grantedPerms) {
			if (p.operation === "Read" || p.operation === "View") {
				const mapped = RESOURCE_TO_SECTION_MAP[p.resource] || [];
				for (const s of mapped) {
					sections.add(s);
				}
			}
		}
		return Array.from(sections).join(",");
	};

	/** Returns how many user updates failed. */
	const applySectionsToUsers = async (users: SessionUser[], sectionsStr: string): Promise<number> => {
		const results = await Promise.allSettled(
			users.map((u) =>
				updateSessionUser({
					id: u.id,
					updates: { username: u.username, role: u.role, allowed_sections: sectionsStr },
				}).unwrap(),
			),
		);
		return results.filter((r) => r.status === "rejected").length;
	};

	// Save Action for current Target
	const handleSaveCurrentMatrix = async () => {
		try {
			if (target.type === "role") {
				if (target.role.name.toLowerCase() === "admin") {
					toast.info("Admin inherently possesses full unrestricted permissions.");
					return;
				}
				await updatePerms({ id: target.role.id, permission_ids: selectedPerms }).unwrap();
				toast.success(`Permissions saved for role '${target.role.name}'`);
				return;
			}

			if (target.type === "all_users") {
				const userRole = roles.find((r) => r.name.toLowerCase() === "user");
				if (!userRole) {
					toast.error("The built-in 'user' role was not found.");
					return;
				}
				await updatePerms({ id: userRole.id, permission_ids: selectedPerms }).unwrap();
				toast.success("Permissions updated for the 'user' role (applies to all users)");
				return;
			}

			if (target.type === "user") {
				const user = targetUser ?? target.user;
				if (!sectionScopeApplies(user.role)) {
					toast.info(
						user.role?.toLowerCase() === "admin"
							? "Admins always have full access."
							: "The 'user' role is limited to Prompt Repository. Assign sub_admin or a custom role to grant more sections.",
					);
					return;
				}
				// Per-user scoping can only narrow the role: a section whose API the role cannot
				// call would show in the sidebar and then fail with "insufficient permissions".
				const rolePermIds = new Set((rolePermData?.permissions || []).map((p) => p.id));
				const effective = selectedPerms.filter((id) => rolePermIds.has(id));
				if (effective.length < selectedPerms.length) {
					toast.warning(
						`${selectedPerms.length - effective.length} permission(s) are not granted by role '${user.role}' and were skipped. Grant them on the role first.`,
					);
				}
				const sectionsStr = computeAllowedSections(effective);
				await updateSessionUser({
					id: user.id,
					updates: {
						username: user.username,
						role: user.role,
						allowed_sections: sectionsStr,
					},
				}).unwrap();
				toast.success(`Permissions and sidebar access updated for user '${user.username}'`);
				return;
			}

			if (target.type === "all_teams") {
				const sectionsStr = computeAllowedSections(selectedPerms);
				const scopedUsers = sessionUsers.filter((u) => sectionScopeApplies(u.role));
				const failed = await applySectionsToUsers(scopedUsers, sectionsStr);
				if (failed > 0) {
					toast.error(`${failed} of ${scopedUsers.length} users could not be updated`);
				} else {
					toast.success(`Sidebar sections updated for ${scopedUsers.length} sub-admin / custom-role users`);
				}
				return;
			}

			if (target.type === "team") {
				toast.info("Per-team permissions are not stored yet. Use All Teams, a role, or an individual user.");
				return;
			}

			if (target.type === "all_customers" || target.type === "customer") {
				toast.info("Customer-scoped permissions are not stored yet. Use a role or an individual user.");
				return;
			}
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	// Quick Select Helpers
	const handleSelectAllReadView = () => {
		const readViewIds = permissions
			.filter((p) => p.operation === "Read" || p.operation === "View")
			.map((p) => p.id);
		setSelectedPerms((prev) => Array.from(new Set([...prev, ...readViewIds])));
		toast.info("Granted all Read & View permissions");
	};

	const handleSelectAll = () => {
		setSelectedPerms(permissions.map((p) => p.id));
		toast.info("Granted all permissions");
	};

	const handleClearAll = () => {
		setSelectedPerms([]);
		toast.info("Cleared all permissions");
	};

	// Create Role
	const handleCreateRole = async () => {
		if (!newRoleName.trim()) {
			toast.error("Role name is required");
			return;
		}
		try {
			const res = await createRole({ name: newRoleName.trim(), description: newRoleDesc.trim(), dac: newRoleDac }).unwrap();
			toast.success(`Role '${res.role.name}' created`);
			setCreateRoleOpen(false);
			setNewRoleName("");
			setNewRoleDesc("");
			setTarget({ type: "role", role: res.role });
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	// Delete Role
	const handleDeleteRole = async (role: RBACRole) => {
		if (!confirm(`Delete role '${role.name}'?`)) return;
		try {
			await deleteRole(role.id).unwrap();
			if (target.type === "role" && target.role.id === role.id) {
				const fallback = roles.find((r) => r.id !== role.id) || null;
				if (fallback) setTarget({ type: "role", role: fallback });
			}
			toast.success(`Role '${role.name}' deleted`);
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	// Direct User Role Assignment
	const handleAssignUserRole = async (userId: string, newRole: string) => {
		try {
			await assignUserRole({ id: userId, role_name: newRole }).unwrap();
			toast.success(`Role updated to '${newRole}'`);
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	// Badge Helper
	const getRoleBadgeClass = (rName: string) => {
		const r = rName.toLowerCase();
		if (r === "admin") return "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/30";
		if (r === "sub_admin" || r === "subadmin") return "bg-blue-500/15 text-blue-600 dark:text-blue-400 border-blue-500/30";
		return "bg-purple-500/15 text-purple-600 dark:text-purple-400 border-purple-500/30";
	};

	return (
		<div className="flex h-full w-full flex-col gap-3">
			{/* Top Header */}
			<div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
				<div>
					<h1 className="flex items-center gap-2 text-2xl font-bold tracking-tight">
						<Shield className="h-6 w-6 text-emerald-500" />
						Roles & Scoped Permissions (RBAC)
					</h1>
					<p className="text-muted-foreground text-xs sm:text-sm">
						Hierarchical Access Control: Configure permissions globally (mothavum) or per individual User, Team, and Customer (separate-avum).
					</p>
				</div>
				<div className="flex items-center gap-2">
					<Button onClick={() => setCreateRoleOpen(true)} className="gap-1.5 shadow-sm text-xs h-9">
						<Plus className="h-4 w-4" />
						New Role
					</Button>
				</div>
			</div>

			{/* Main Split Layout: Left Hierarchy Navigation | Right Permissions Matrix */}
			<div className="grid h-[calc(100%-4rem)] grid-cols-1 gap-4 lg:grid-cols-[340px_1fr] overflow-hidden">
				{/* ════════════════════════════════════════════════════════════════
				    LEFT COLUMN: UNIFIED HIERARCHY TREE
				════════════════════════════════════════════════════════════════ */}
				<div className="bg-card flex flex-col rounded-xl border shadow-xs overflow-hidden">
					{/* Search in left pane */}
					<div className="border-b p-3">
						<div className="relative">
							<Search className="text-muted-foreground absolute top-2.5 left-2.5 h-3.5 w-3.5" />
							<Input
								placeholder="Search roles, users, teams…"
								value={treeSearch}
								onChange={(e) => setTreeSearch(e.target.value)}
								className="h-8 pl-8 text-xs"
							/>
						</div>
					</div>

					{/* Navigation Tree Accordions */}
					<div className="no-scrollbar flex-1 space-y-2 overflow-y-auto p-2">
						{/* 1. ROLES SECTION */}
						<div className="rounded-lg border bg-muted/20">
							<button
								type="button"
								onClick={() => toggleSection("roles")}
								className="flex w-full items-center justify-between px-3 py-2 text-xs font-bold uppercase tracking-wider text-muted-foreground hover:text-foreground"
							>
								<div className="flex items-center gap-1.5">
									{expandedSections.roles ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
									<Shield className="h-3.5 w-3.5 text-emerald-500" />
									<span>Roles</span>
								</div>
								<Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
									{filteredRoles.length}
								</Badge>
							</button>

							{expandedSections.roles && (
								<div className="space-y-1 p-1 pt-0">
									{filteredRoles.map((role) => {
										const isSelected = target.type === "role" && target.role.id === role.id;
										const isSys = Boolean(
											role.is_system_role ||
												role.name?.toLowerCase() === "admin" ||
												role.name?.toLowerCase() === "user",
										);

										return (
											<div
												key={role.id}
												onClick={() => setTarget({ type: "role", role })}
												className={`flex cursor-pointer items-center justify-between rounded-md px-2.5 py-1.5 text-xs transition-colors ${
													isSelected
														? "bg-emerald-500/15 font-semibold text-emerald-900 dark:text-emerald-200"
														: "hover:bg-muted text-muted-foreground hover:text-foreground"
												}`}
											>
												<div className="flex items-center gap-2 truncate">
													<span className="truncate">{role.name}</span>
													{isSys && <span className="bg-muted text-muted-foreground rounded px-1 text-[9px]">system</span>}
												</div>

												{!isSys && (
													<Button
														size="icon"
														variant="ghost"
														className="h-5 w-5 text-muted-foreground hover:text-destructive"
														onClick={(e) => {
															e.stopPropagation();
															void handleDeleteRole(role);
														}}
													>
														<Trash2 className="h-3 w-3" />
													</Button>
												)}
											</div>
										);
									})}
								</div>
							)}
						</div>

						{/* 2. USERS SECTION (Mothavum & Separate) */}
						<div className="rounded-lg border bg-muted/20">
							<button
								type="button"
								onClick={() => toggleSection("users")}
								className="flex w-full items-center justify-between px-3 py-2 text-xs font-bold uppercase tracking-wider text-muted-foreground hover:text-foreground"
							>
								<div className="flex items-center gap-1.5">
									{expandedSections.users ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
									<Users className="h-3.5 w-3.5 text-blue-500" />
									<span>Users</span>
								</div>
								<Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
									{filteredUsers.length}
								</Badge>
							</button>

							{expandedSections.users && (
								<div className="space-y-1 p-1 pt-0">
									{/* All Users Bulk Option */}
									<div
										onClick={() => setTarget({ type: "all_users" })}
										className={`flex cursor-pointer items-center gap-2 rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors ${
											target.type === "all_users"
												? "bg-blue-500/15 text-blue-800 dark:text-blue-300 font-bold"
												: "hover:bg-muted text-muted-foreground hover:text-foreground"
										}`}
									>
										<Globe className="h-3.5 w-3.5 text-blue-500" />
										<span>All Users (Bulk Global Policy)</span>
									</div>

									{/* Individual Users List */}
									<div className="border-t pt-1 pl-2 space-y-0.5">
										{filteredUsers.map((user) => {
											const isSelected = target.type === "user" && target.user.id === user.id;

											return (
												<div
													key={user.id}
													onClick={() => setTarget({ type: "user", user })}
													className={`flex cursor-pointer items-center justify-between rounded-md px-2 py-1 text-xs transition-colors ${
														isSelected
															? "bg-blue-500/20 text-blue-900 dark:text-blue-200 font-semibold"
															: "hover:bg-muted text-muted-foreground hover:text-foreground"
													}`}
												>
													<div className="flex items-center gap-1.5 truncate">
														<User className="h-3 w-3 shrink-0 text-muted-foreground" />
														<span className="truncate">{user.username}</span>
													</div>
													<span className={`rounded border px-1 text-[10px] ${getRoleBadgeClass(user.role || "user")}`}>
														{user.role || "user"}
													</span>
												</div>
											);
										})}
									</div>
								</div>
							)}
						</div>

						{/* 3. TEAMS SECTION (Mothavum & Separate) */}
						<div className="rounded-lg border bg-muted/20">
							<button
								type="button"
								onClick={() => toggleSection("teams")}
								className="flex w-full items-center justify-between px-3 py-2 text-xs font-bold uppercase tracking-wider text-muted-foreground hover:text-foreground"
							>
								<div className="flex items-center gap-1.5">
									{expandedSections.teams ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
									<Building2 className="h-3.5 w-3.5 text-purple-500" />
									<span>Teams</span>
								</div>
								<Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
									{filteredTeams.length}
								</Badge>
							</button>

							{expandedSections.teams && (
								<div className="space-y-1 p-1 pt-0">
									{/* All Teams Bulk Option */}
									<div
										onClick={() => setTarget({ type: "all_teams" })}
										className={`flex cursor-pointer items-center gap-2 rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors ${
											target.type === "all_teams"
												? "bg-purple-500/15 text-purple-800 dark:text-purple-300 font-bold"
												: "hover:bg-muted text-muted-foreground hover:text-foreground"
										}`}
									>
										<Globe className="h-3.5 w-3.5 text-purple-500" />
										<span>All Teams (Bulk Policy)</span>
									</div>

									{/* Individual Teams */}
									<div className="border-t pt-1 pl-2 space-y-0.5">
										{filteredTeams.map((team) => {
											const isSelected = target.type === "team" && target.team.id === team.id;

											return (
												<div
													key={team.id}
													onClick={() => setTarget({ type: "team", team })}
													className={`flex cursor-pointer items-center justify-between rounded-md px-2 py-1 text-xs transition-colors ${
														isSelected
															? "bg-purple-500/20 text-purple-900 dark:text-purple-200 font-semibold"
															: "hover:bg-muted text-muted-foreground hover:text-foreground"
													}`}
												>
													<div className="flex items-center gap-1.5 truncate">
														<Building2 className="h-3 w-3 shrink-0 text-muted-foreground" />
														<span className="truncate">{team.name}</span>
													</div>
													<span className="text-[10px] text-muted-foreground">
														{team.customer?.name ? team.customer.name.slice(0, 10) : "Team"}
													</span>
												</div>
											);
										})}
									</div>
								</div>
							)}
						</div>

						{/* 4. CUSTOMERS SECTION (Mothavum & Separate) */}
						<div className="rounded-lg border bg-muted/20">
							<button
								type="button"
								onClick={() => toggleSection("customers")}
								className="flex w-full items-center justify-between px-3 py-2 text-xs font-bold uppercase tracking-wider text-muted-foreground hover:text-foreground"
							>
								<div className="flex items-center gap-1.5">
									{expandedSections.customers ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
									<Landmark className="h-3.5 w-3.5 text-amber-500" />
									<span>Customers</span>
								</div>
								<Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
									{filteredCustomers.length}
								</Badge>
							</button>

							{expandedSections.customers && (
								<div className="space-y-1 p-1 pt-0">
									<div
										onClick={() => setTarget({ type: "all_customers" })}
										className={`flex cursor-pointer items-center gap-2 rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors ${
											target.type === "all_customers"
												? "bg-amber-500/15 text-amber-800 dark:text-amber-300 font-bold"
												: "hover:bg-muted text-muted-foreground hover:text-foreground"
										}`}
									>
										<Globe className="h-3.5 w-3.5 text-amber-500" />
										<span>All Customers (Bulk Root)</span>
									</div>

									<div className="border-t pt-1 pl-2 space-y-0.5">
										{filteredCustomers.map((cust) => {
											const isSelected = target.type === "customer" && target.customer.id === cust.id;

											return (
												<div
													key={cust.id}
													onClick={() => setTarget({ type: "customer", customer: cust })}
													className={`flex cursor-pointer items-center justify-between rounded-md px-2 py-1 text-xs transition-colors ${
														isSelected
															? "bg-amber-500/20 text-amber-900 dark:text-amber-200 font-semibold"
															: "hover:bg-muted text-muted-foreground hover:text-foreground"
													}`}
												>
													<div className="flex items-center gap-1.5 truncate">
														<Landmark className="h-3 w-3 shrink-0 text-muted-foreground" />
														<span className="truncate">{cust.name}</span>
													</div>
												</div>
											);
										})}
									</div>
								</div>
							)}
						</div>
					</div>
				</div>

				{/* ════════════════════════════════════════════════════════════════
				    RIGHT COLUMN: DYNAMIC PERMISSIONS MATRIX FOR TARGET
				════════════════════════════════════════════════════════════════ */}
				<div className="bg-card flex flex-col rounded-xl border shadow-xs overflow-hidden">
					{/* Active Target Banner */}
					<div className="border-b p-4">
						<div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
							<div>
								<div className="flex items-center gap-2">
									<span className="text-xs uppercase tracking-wider text-muted-foreground font-semibold">
										Configuring Permissions For:
									</span>
									<span className="font-bold text-base">
										{target.type === "role" && `Role: ${target.role.name}`}
										{target.type === "all_users" && "All Users (Bulk Global Policy)"}
										{target.type === "user" && `User: ${target.user.username}`}
										{target.type === "all_teams" && "All Teams (Bulk Policy)"}
										{target.type === "team" && `Team: ${target.team.name}`}
										{target.type === "all_customers" && "All Customers"}
										{target.type === "customer" && `Customer: ${target.customer.name}`}
									</span>
								</div>

								{/* Target Metadata & User Role Quick Switch */}
								<div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
									{target.type === "user" && targetUser && (
										<>
											<span>Current Role:</span>
											<select
												value={targetUser.role || "user"}
												onChange={(e) => void handleAssignUserRole(targetUser.id, e.target.value)}
												className="bg-background border-input h-7 rounded border px-2 text-xs font-semibold text-foreground"
											>
												{roles.map((r) => (
													<option key={r.id} value={r.name}>
														{r.name}
													</option>
												))}
											</select>
											<span>• Email: {targetUser.email || "No email"}</span>
										</>
									)}

									{target.type === "team" && (
										<>
											<span>Scope: All members of {target.team.name} inherit granted sections</span>
										</>
									)}

									{target.type === "role" && (
										<span>
											{target.role.description || "No description"} • DAC: {target.role.dac || "all-data"}
										</span>
									)}
								</div>
							</div>

							{/* Action Buttons */}
							<div className="flex flex-wrap items-center gap-2">
								<Button variant="outline" size="sm" onClick={handleSelectAllReadView} className="h-8 text-xs">
									<Eye className="mr-1.5 h-3.5 w-3.5 text-blue-500" />
									View Only
								</Button>
								<Button variant="outline" size="sm" onClick={handleSelectAll} className="h-8 text-xs">
									<Check className="mr-1.5 h-3.5 w-3.5 text-emerald-500" />
									Select All
								</Button>
								<Button variant="outline" size="sm" onClick={handleClearAll} className="h-8 text-xs">
									<X className="mr-1.5 h-3.5 w-3.5 text-amber-500" />
									Clear
								</Button>
								<Button
									size="sm"
									onClick={() => void handleSaveCurrentMatrix()}
									disabled={isUpdatingPerms}
									className="h-8 bg-emerald-600 text-white hover:bg-emerald-700 text-xs font-semibold"
								>
									Save Permissions
								</Button>
							</div>
						</div>

						{/* Resource Filter */}
						<div className="relative mt-3">
							<Search className="text-muted-foreground absolute top-2.5 left-2.5 h-3.5 w-3.5" />
							<Input
								placeholder="Filter resources (e.g. Logs, VirtualKeys, Governance, ModelProvider)…"
								value={resourceSearch}
								onChange={(e) => setResourceSearch(e.target.value)}
								className="h-8 pl-8 text-xs"
							/>
						</div>
					</div>

					{/* Explanation Notice */}
					<div className="border-b bg-muted/30 px-4 py-2 text-xs flex items-center gap-2 text-muted-foreground">
						<Info className="h-4 w-4 shrink-0 text-emerald-500" />
						<span>
							Checked operations (`Read, View, Create, Update, Delete`) enable exact actions and sidebar sections. Unchecked items remain strictly hidden and forbidden.
						</span>
					</div>

					{/* Resource Cards Grid */}
					<div className="no-scrollbar flex-1 space-y-3 overflow-y-auto p-4">
						{groupedPermissions.length === 0 ? (
							<p className="text-muted-foreground py-8 text-center text-sm">No resources matching filter.</p>
						) : (
							groupedPermissions.map(([resource, perms]) => {
								const allGranted = perms.every((p) => selectedPerms.includes(p.id));

								const toggleGroup = () => {
									if (allGranted) {
										const removeIds = new Set(perms.map((p) => p.id));
										setSelectedPerms((prev) => prev.filter((id) => !removeIds.has(id)));
									} else {
										const addIds = perms.map((p) => p.id);
										setSelectedPerms((prev) => Array.from(new Set([...prev, ...addIds])));
									}
								};

								return (
									<div
										key={resource}
										className="bg-card/60 hover:bg-card/90 rounded-lg border p-3 transition-colors shadow-2xs"
									>
										<div className="mb-2 flex items-center justify-between">
											<div className="flex items-center gap-2">
												<span className="font-semibold text-sm">{resource}</span>
												<Badge variant="outline" className="text-[10px] py-0 px-1.5 font-normal">
													{perms.filter((p) => selectedPerms.includes(p.id)).length} / {perms.length} granted
												</Badge>
											</div>

											<button
												type="button"
												onClick={toggleGroup}
												className="text-muted-foreground hover:text-foreground text-[11px] underline-offset-2 hover:underline"
											>
												{allGranted ? "Deselect All" : "Select All"}
											</button>
										</div>

										<div className="flex flex-wrap gap-2">
											{perms.map((perm) => {
												const isChecked = selectedPerms.includes(perm.id);

												return (
													<label
														key={perm.id}
														className={`flex cursor-pointer items-center gap-2 rounded-md border px-2.5 py-1 text-xs transition-all ${
															isChecked
																? "border-emerald-500/40 bg-emerald-500/10 text-emerald-800 dark:text-emerald-300 font-medium"
																: "border-border bg-background text-muted-foreground hover:border-muted-foreground/30"
														}`}
													>
														<input
															type="checkbox"
															checked={isChecked}
															onChange={(e) => {
																setSelectedPerms((current) =>
																	e.target.checked
																		? [...current, perm.id]
																		: current.filter((id) => id !== perm.id),
																);
															}}
															className="rounded border-gray-300 text-emerald-600 focus:ring-emerald-500 h-3.5 w-3.5"
														/>
														<span>{perm.operation}</span>
													</label>
												);
											})}
										</div>
									</div>
								);
							})
						)}
					</div>
				</div>
			</div>

			{/* Create Role Modal */}
			<Dialog open={createRoleOpen} onOpenChange={setCreateRoleOpen}>
				<DialogContent className="sm:max-w-md no-scrollbar">
					<DialogHeader>
						<DialogTitle className="flex items-center gap-2">
							<Shield className="h-5 w-5 text-emerald-500" />
							Create New RBAC Role
						</DialogTitle>
					</DialogHeader>
					<div className="space-y-4 py-2">
						<div className="space-y-1.5">
							<Label htmlFor="role-name">Role Name *</Label>
							<Input
								id="role-name"
								value={newRoleName}
								onChange={(e) => setNewRoleName(e.target.value)}
								placeholder="e.g. auditor, data_engineer, operator"
								autoFocus
							/>
						</div>
						<div className="space-y-1.5">
							<Label htmlFor="role-desc">Description</Label>
							<Input
								id="role-desc"
								value={newRoleDesc}
								onChange={(e) => setNewRoleDesc(e.target.value)}
								placeholder="What does this role allow?"
							/>
						</div>
						<div className="space-y-1.5">
							<Label htmlFor="role-dac">Data Access Scope (DAC)</Label>
							<select
								id="role-dac"
								value={newRoleDac}
								onChange={(e) => setNewRoleDac(e.target.value)}
								className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
							>
								<option value="all-data">all-data (Full tenant data access)</option>
								<option value="team-data">team-data (Limited to own team's keys/prompts)</option>
								<option value="own-data">own-data (Limited to self-created keys)</option>
							</select>
						</div>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setCreateRoleOpen(false)}>
							Cancel
						</Button>
						<Button onClick={() => void handleCreateRole()} disabled={isCreatingRole} className="bg-emerald-600 text-white hover:bg-emerald-700">
							Create Role
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}
