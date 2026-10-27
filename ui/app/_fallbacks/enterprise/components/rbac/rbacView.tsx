import { QueryErrorBanner } from "@/components/queryErrorBanner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
	getErrorMessage,
	useGetCustomersQuery,
	useGetSessionUsersQuery,
	useGetTeamsQuery,
	useGetUserTeamsQuery,
	useIsAuthEnabledQuery,
	useUpdateSessionUserMutation,
} from "@/lib/store";
import { RbacOperation, RbacResource, useRbac } from "@enterprise/lib";
import type { SessionUser } from "@/lib/store";
import { allowedSectionsToString } from "@/lib/constants/workspaceSections";
import type { Customer, Team } from "@/lib/types/governance";
import {
	useAssignUserRoleMutation,
	useCreateRoleMutation,
	useDeleteRoleMutation,
	useGetPermissionsQuery,
	useGetRBACScopeGrantsQuery,
	useGetRolePermissionsQuery,
	useGetRolesQuery,
	useUpdateRBACScopeGrantMutation,
	useUpdateRolePermissionsMutation,
} from "@enterprise/lib/store/apis/rbacApi";
import { RBACPermission, RBACRole, RBACScopeGrant, RBACScopeGrants, RBACScopeType } from "@enterprise/lib/types/workspace";
import {
	Building2,
	Check,
	ChevronDown,
	ChevronRight,
	Edit2,
	Eye,
	Globe,
	Info,
	Landmark,
	Plus,
	Search,
	Shield,
	ShieldCheck,
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
	| { type: "all_admins" }
	| { type: "all_users" }
	| { type: "all_sub_admins" }
	| { type: "user"; user: SessionUser }
	| { type: "all_teams" }
	| { type: "team"; team: Team }
	| { type: "all_customers" }
	| { type: "customer"; customer: Customer };

// Resource → the sidebar entries (keys from WORKSPACE_SECTIONS) its API backs. Only the
// specific children are listed: a parent section is granted when all its children are
// (see allowedSectionsToString), never because one child was selected.
// Resource → the sidebar entries (keys from WORKSPACE_SECTIONS) its API backs.
const RESOURCE_TO_SECTION_MAP: Record<string, string[]> = {
	// Observability
	Dashboard: ["observability/dashboard"],
	Logs: ["observability/llm-logs"],
	MCPLogs: ["observability/mcp-logs"],
	Observability: ["observability/connectors"],
	LogsSettings: ["observability/logs-settings"],

	// Browser AI
	BrowserAIOverview: ["browser-ai/overview", "browser-ai"],
	BrowserAITargets: ["browser-ai/targets"],
	BrowserAIRules: ["browser-ai/rules"],
	BrowserAILogs: ["browser-ai/logs"],
	BrowserAISearchLogs: ["browser-ai/search-logs"],
	BrowserAISetup: ["browser-ai/setup"],
	BrowserAIAgents: ["browser-ai/agents"],
	BrowserAIInsights: ["browser-ai/telemetry"],

	// Models
	ModelProvider: ["models/model-providers"],
	ModelCatalog: ["models/model-catalog"],
	BudgetsLimits: ["models/budgets-limits"],
	ComplexityRouter: ["models/complexity-router"],
	RoutingRules: ["models/routing-rules"],
	CircuitBreaker: ["models/circuit-breaker"],
	PricingOverrides: ["models/pricing-overrides"],
	ModelSettings: ["models/model-settings"],

	// MCP Gateway
	MCPCatalog: ["mcp-gateway/mcp-catalog"],
	MCPLibrary: ["mcp-gateway/mcp-library"],
	MCPToolGroups: ["mcp-gateway/tool-groups"],
	MCPAuthSessions: ["mcp-gateway/auth-sessions"],
	MCPOAuthGrants: ["mcp-gateway/oauth-grants"],
	MCPSettings: ["mcp-gateway/mcp-settings"],
	Plugins: ["plugins"],

	// Governance
	VirtualKeys: ["governance/virtual-keys"],
	Users: ["governance/users"],
	Teams: ["governance/teams"],
	BusinessUnits: ["governance/business-units"],
	Customers: ["governance/customers"],
	UserProvisioning: ["governance/user-provisioning"],
	RBAC: ["governance/roles-permissions"],
	AccessProfiles: ["governance/access-profiles"],
	AuditLogs: ["governance/audit-logs"],

	// Guardrails
	GuardrailRules: ["guardrails/rules"],
	GuardrailsProviders: ["guardrails/providers"],
	Cluster: ["guardrails/cluster-config"],

	// Adaptive Routing
	AdaptiveRoutingDashboard: ["adaptive-routing/dashboard", "adaptive-routing"],
	AdaptiveRoutingSettings: ["adaptive-routing/settings"],
	PromptRepository: ["prompt-repository"],
	SkillsRepository: ["skills-repository"],

	// Settings
	ClientSettings: ["settings/client-settings"],
	Compatibility: ["settings/compatibility"],
	Caching: ["settings/caching"],
	Security: ["settings/security"],
	APIKeys: ["settings/api-keys"],
	PerformanceTuning: ["settings/performance-tuning"],
	FeatureFlags: ["settings/feature-flags"],

	// Aliases & legacy mappings for backward compatibility
	Governance: ["governance"],
	GuardrailsConfig: ["guardrails/rules"],
	Settings: ["settings"],
	AdaptiveRouter: ["adaptive-routing"],
	PromptDeploymentStrategy: ["prompt-repository"],
	Inference: ["prompt-repository"],
	Metrics: ["observability/dashboard"],
};

/** Sidebar-aligned groups matching the 8 primary workspace categories. */
const RESOURCE_GROUPS: { title: string; resources: string[] }[] = [
	{
		title: "Observability",
		resources: ["Dashboard", "Logs", "MCPLogs", "Observability", "LogsSettings"],
	},
	{
		title: "Browser AI",
		resources: [
			"BrowserAIOverview",
			"BrowserAITargets",
			"BrowserAIRules",
			"BrowserAILogs",
			"BrowserAISearchLogs",
			"BrowserAISetup",
			"BrowserAIAgents",
			"BrowserAIInsights",
		],
	},
	{
		title: "Models",
		resources: [
			"ModelProvider",
			"ModelCatalog",
			"BudgetsLimits",
			"ComplexityRouter",
			"RoutingRules",
			"CircuitBreaker",
			"PricingOverrides",
			"ModelSettings",
		],
	},
	{
		title: "MCP Gateway",
		resources: [
			"MCPCatalog",
			"MCPLibrary",
			"MCPToolGroups",
			"MCPAuthSessions",
			"MCPOAuthGrants",
			"MCPSettings",
			"Plugins",
		],
	},
	{
		title: "Governance",
		resources: [
			"VirtualKeys",
			"Users",
			"Teams",
			"BusinessUnits",
			"Customers",
			"UserProvisioning",
			"RBAC",
			"AccessProfiles",
			"AuditLogs",
		],
	},
	{
		title: "Guardrails",
		resources: ["GuardrailRules", "GuardrailsProviders", "Cluster"],
	},
	{
		title: "Adaptive Routing",
		resources: [
			"AdaptiveRoutingDashboard",
			"AdaptiveRoutingSettings",
			"PromptRepository",
			"SkillsRepository",
		],
	},
	{
		title: "Settings",
		resources: [
			"ClientSettings",
			"Compatibility",
			"Caching",
			"Security",
			"APIKeys",
			"PerformanceTuning",
			"FeatureFlags",
		],
	},
];

const RESOURCE_LABELS: Record<string, string> = {
	// Observability
	Dashboard: "Dashboard",
	Logs: "LLM Log",
	MCPLogs: "MCP Logs",
	Observability: "Connectors",
	LogsSettings: "Logs Settings",

	// Browser AI
	BrowserAIOverview: "Overview",
	BrowserAITargets: "Target Websites",
	BrowserAIRules: "Guard Rules",
	BrowserAILogs: "Prompt Logs",
	BrowserAISearchLogs: "Search Logs",
	BrowserAISetup: "Setup",
	BrowserAIAgents: "Guard Agents",
	BrowserAIInsights: "Guard Insights",

	// Models
	ModelProvider: "Model Providers",
	ModelCatalog: "Model Catalog",
	BudgetsLimits: "Budgets & Limits",
	ComplexityRouter: "Complexity Router",
	RoutingRules: "Routing Rules",
	CircuitBreaker: "Circuit Breaker",
	PricingOverrides: "Pricing Overrides",
	ModelSettings: "Model Settings",

	// MCP Gateway
	MCPCatalog: "MCP Catalog",
	MCPLibrary: "MCP Library",
	MCPToolGroups: "Tool Groups",
	MCPAuthSessions: "Auth Sessions",
	MCPOAuthGrants: "OAuth Grants",
	MCPSettings: "MCP Settings",
	Plugins: "Plugins",

	// Governance
	VirtualKeys: "Virtual Keys",
	Users: "Users",
	Teams: "Teams",
	BusinessUnits: "Business Units",
	Customers: "Customers",
	UserProvisioning: "User Provisioning",
	RBAC: "Roles & Permissions",
	AccessProfiles: "Access Profiles",
	AuditLogs: "Audit Logs",

	// Guardrails
	GuardrailRules: "Rules",
	GuardrailsProviders: "Rule Providers",
	Cluster: "Cluster Config",

	// Adaptive Routing
	AdaptiveRoutingDashboard: "Dashboard",
	AdaptiveRoutingSettings: "Settings",
	PromptRepository: "Prompt Repository",
	SkillsRepository: "Skills Repository",

	// Settings
	ClientSettings: "Client Settings",
	Compatibility: "Compatibility",
	Caching: "Caching",
	Security: "Security",
	APIKeys: "API Keys",
	PerformanceTuning: "Performance Tuning",
	FeatureFlags: "Feature Flags",

	// Legacy aliases
	Metrics: "Metrics",
	Governance: "Governance (all)",
	GuardrailsConfig: "Guardrails Config",
	AdaptiveRouter: "Adaptive Routing",
	PromptDeploymentStrategy: "Prompt Deployments",
	Inference: "Inference",
};

// A saved section grants a mapped entry directly or through its parent section.
function sectionGranted(allowed: Set<string>, section: string): boolean {
	if (allowed.has(section)) return true;
	const parent = section.split("/")[0];
	if (parent !== section && allowed.has(parent)) return true;
	return section === "guardrails/cluster-config" && allowed.has("cluster-config");
}

function isAdminRole(role?: string): boolean {
	return (role || "").toLowerCase() === "admin";
}

function isSubAdminRole(role?: string): boolean {
	return (role || "").toLowerCase() === "sub_admin";
}

// "user", "sub_admin", and "admin" have their own tree sections.
function hasOwnTreeSection(role: RBACRole): boolean {
	const r = role.name?.toLowerCase();
	return r === "user" || r === "sub_admin" || r === "admin";
}

type TargetScope = { type: RBACScopeType; id?: string; label: string; grant?: RBACScopeGrant };

// Teams and customers are saved as scope grants that their members inherit.
function targetScope(target: SelectionTarget, grants?: RBACScopeGrants): TargetScope | null {
	switch (target.type) {
		case "all_teams":
			return { type: "all_teams", label: "All Teams", grant: grants?.all_teams };
		case "team":
			return { type: "team", id: target.team.id, label: `team '${target.team.name}'`, grant: grants?.teams?.[target.team.id] };
		case "all_customers":
			return { type: "all_customers", label: "All Customers", grant: grants?.all_customers };
		case "customer":
			return {
				type: "customer",
				id: target.customer.id,
				label: `customer '${target.customer.name}'`,
				grant: grants?.customers?.[target.customer.id],
			};
		default:
			return null;
	}
}

function countSections(list?: string): number {
	return (list || "")
		.split(",")
		.map((s) => s.trim())
		.filter(Boolean).length;
}

export default function RBACView() {
	// Navigation Tree Accordion States
	const [expandedSections, setExpandedSections] = useState<{
		roles: boolean;
		admins: boolean;
		subAdmins: boolean;
		users: boolean;
		teams: boolean;
		customers: boolean;
	}>({
		roles: true,
		admins: true,
		subAdmins: true,
		users: true,
		teams: true,
		customers: false,
	});

	// Current Target Selection
	const [target, setTarget] = useState<SelectionTarget>({ type: "all_sub_admins" });

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
	const { data: authStatus } = useIsAuthEnabledQuery();
	const isSuperAdmin = authStatus?.role === "admin";
	const canUpdateRbac = useRbac(RbacResource.RBAC, RbacOperation.Update) || isSuperAdmin;
	const canCreateRole = useRbac(RbacResource.RBAC, RbacOperation.Create) || isSuperAdmin;
	const canDeleteRole = useRbac(RbacResource.RBAC, RbacOperation.Delete) || isSuperAdmin;

	const { data: roleData, isError: rolesFailed, error: rolesError } = useGetRolesQuery();
	const { data: permData, isError: permsFailed, error: permsError } = useGetPermissionsQuery();
	const { data: sessionUsersData, isError: usersFailed, error: usersError } = useGetSessionUsersQuery();
	const sessionUsers = useMemo(() => sessionUsersData ?? [], [sessionUsersData]);
	const { data: teamsData, isError: teamsFailed, error: teamsError } = useGetTeamsQuery();
	const { data: customersData, isError: customersFailed, error: customersError } = useGetCustomersQuery();
	const { data: scopeGrants, isError: scopeGrantsFailed, error: scopeGrantsError } = useGetRBACScopeGrantsQuery();

	const roles = useMemo(() => roleData?.roles ?? [], [roleData]);
	const rawPermissions = useMemo(() => permData?.permissions ?? [], [permData]);
	const permissions = useMemo(() => {
		const existingKeys = new Set(rawPermissions.map((p) => `${p.resource}:${p.operation}`));
		const extended = [...rawPermissions];
		let syntheticId = 3000;
		const ops = ["Read", "View", "Create", "Update", "Delete", "Download"];
		for (const group of RESOURCE_GROUPS) {
			for (const res of group.resources) {
				for (const op of ops) {
					const key = `${res}:${op}`;
					if (!existingKeys.has(key)) {
						extended.push({ id: syntheticId++, resource: res, operation: op });
						existingKeys.add(key);
					}
				}
			}
		}
		return extended;
	}, [rawPermissions]);

	const teams = useMemo(() => teamsData?.teams ?? [], [teamsData]);
	const customers = useMemo(() => customersData?.customers ?? [], [customersData]);

	// The tree hands us a snapshot; read role / allowed_sections from the live list so role
	// changes and saved sections are reflected without re-selecting the user.
	const targetUser = useMemo(
		() => (target.type === "user" ? sessionUsers.find((u) => u.id === target.user.id) ?? target.user : null),
		[target, sessionUsers],
	);

	// Check if current target is a regular User role
	const isTargetUserRole = useMemo(() => {
		if (target.type === "all_users") return true;
		if (target.type === "user") {
			const uRole = (targetUser?.role || target.user.role || "").toLowerCase();
			return uRole !== "admin" && !isSubAdminRole(uRole);
		}
		if (target.type === "role" && target.role.name?.toLowerCase() === "user") return true;
		return false;
	}, [target, targetUser]);

	// Determine active role ID for fetching role permissions query
	const activeRoleId = useMemo(() => {
		if (target.type === "role") return target.role.id;
		if (target.type === "all_admins") {
			const adminRole = roles.find((r) => r.name.toLowerCase() === "admin");
			return adminRole?.id ?? 0;
		}
		if (target.type === "all_users") {
			const userRole = roles.find((r) => r.name.toLowerCase() === "user");
			return userRole?.id ?? 0;
		}
		if (target.type === "user") {
			const matchingRole = roles.find((r) => r.name.toLowerCase() === (targetUser?.role || "user").toLowerCase());
			return matchingRole?.id ?? 0;
		}
		if (target.type === "all_sub_admins") {
			const subAdminRole = roles.find((r) => r.name.toLowerCase() === "sub_admin");
			return subAdminRole?.id ?? 0;
		}
		return 0;
	}, [target, roles, targetUser]);

	const { currentData: rolePermData, isError: rolePermsFailed, error: rolePermsError } = useGetRolePermissionsQuery(activeRoleId, {
		skip: activeRoleId === 0,
	});

	const scope = useMemo(() => targetScope(target, scopeGrants), [target, scopeGrants]);

	const userUsesSectionScope = Boolean(targetUser);
	const {
		data: targetUserTeams,
		isError: targetUserTeamsFailed,
		error: targetUserTeamsError,
	} = useGetUserTeamsQuery(targetUser?.id ?? "", { skip: !userUsesSectionScope });

	const rbacListQueryFailed =
		rolesFailed ||
		permsFailed ||
		usersFailed ||
		teamsFailed ||
		customersFailed ||
		scopeGrantsFailed ||
		rolePermsFailed ||
		targetUserTeamsFailed;
	const rbacListQueryError =
		rolesError ||
		permsError ||
		usersError ||
		teamsError ||
		customersError ||
		scopeGrantsError ||
		rolePermsError ||
		targetUserTeamsError;

	// Mirrors effectiveAllowedSections on the server: what the selected user gets from
	// their teams and those teams' customers, on top of their own sections.
	const inheritedForUser = useMemo(() => {
		if (!userUsesSectionScope || !scopeGrants) return null;
		const teamsList = targetUserTeams?.teams ?? [];
		if (teamsList.length === 0) return null;
		const sources: string[] = [];
		const sections = new Set<string>();
		const add = (grant: RBACScopeGrant | undefined, label: string) => {
			if (!grant) return;
			sources.push(label);
			for (const s of grant.allowed_sections.split(",")) {
				if (s.trim()) sections.add(s.trim());
			}
		};
		let inCustomer = false;
		for (const t of teamsList) {
			const teamId = t.id || t.team_id || "";
			add(scopeGrants.teams?.[teamId], `team ${t.name}`);
			if (t.customer_id) {
				inCustomer = true;
				add(scopeGrants.customers?.[t.customer_id], `customer ${t.customer_name || t.customer_id}`);
			}
		}
		add(scopeGrants.all_teams, "All Teams");
		if (inCustomer) add(scopeGrants.all_customers, "All Customers");
		return sources.length ? { sources: Array.from(new Set(sources)), count: sections.size } : null;
	}, [userUsesSectionScope, scopeGrants, targetUserTeams]);

	// Mutations
	const [createRole, { isLoading: isCreatingRole }] = useCreateRoleMutation();
	const [updatePerms, { isLoading: isUpdatingPerms }] = useUpdateRolePermissionsMutation();
	const [updateScopeGrant, { isLoading: isSavingScope }] = useUpdateRBACScopeGrantMutation();
	const [deleteRole, { isLoading: isDeletingRole }] = useDeleteRoleMutation();
	const [deletingRole, setDeletingRole] = useState<RBACRole | null>(null);
	const [assignUserRole] = useAssignUserRoleMutation();
	const [updateSessionUser, { isLoading: isUpdatingUser }] = useUpdateSessionUserMutation();

	// Track the active target key to synchronize permissions without wiping unsaved user selections on background re-renders
	const activeTargetKey = useMemo(() => {
		switch (target.type) {
			case "role":
				return `role:${target.role.id}`;
			case "all_admins":
				return "all_admins";
			case "all_users":
				return "all_users";
			case "all_sub_admins":
				return "all_sub_admins";
			case "user":
				return `user:${target.user.id}:${targetUser?.role ?? ""}:${targetUser?.allowed_sections ?? ""}:${targetUser?.rbac_permission_ids ?? ""}`;
			case "all_teams":
				return `all_teams:${scopeGrants?.all_teams?.updated_at ?? ""}:${scopeGrants?.all_teams?.allowed_sections ?? ""}`;
			case "team":
				return `team:${target.team.id}:${scopeGrants?.teams?.[target.team.id]?.updated_at ?? ""}:${scopeGrants?.teams?.[target.team.id]?.allowed_sections ?? ""}`;
			case "all_customers":
				return `all_customers:${scopeGrants?.all_customers?.updated_at ?? ""}:${scopeGrants?.all_customers?.allowed_sections ?? ""}`;
			case "customer":
				return `customer:${target.customer.id}:${scopeGrants?.customers?.[target.customer.id]?.updated_at ?? ""}:${scopeGrants?.customers?.[target.customer.id]?.allowed_sections ?? ""}`;
			default:
				return "";
		}
	}, [target, targetUser?.role, targetUser?.allowed_sections, targetUser?.rbac_permission_ids, scopeGrants]);

	const [lastSyncedKey, setLastSyncedKey] = useState<string>("");

	// Sync permissions when target changes or when relevant query data arrives
	useEffect(() => {
		if (activeTargetKey === lastSyncedKey) {
			return;
		}

		if ((target.type === "role" && target.role.name?.toLowerCase() === "admin") || target.type === "all_admins") {
			if (permissions.length > 0) {
				setSelectedPerms(permissions.map((p) => p.id));
				setLastSyncedKey(activeTargetKey);
			}
			return;
		}

		if (scope) {
			setSelectedPerms(scope.grant?.permission_ids ?? []);
			setLastSyncedKey(activeTargetKey);
			return;
		}

		if (target.type === "user" && targetUser) {
			const uRole = (targetUser.role || "user").toLowerCase();
			const savedRaiseIds = (targetUser.rbac_permission_ids || "")
				.split(",")
				.map((s) => Number(s.trim()))
				.filter((n) => Number.isFinite(n) && n > 0);

			// If customized permissions were saved for this user (admin, sub_admin, or user), restore them!
			if (savedRaiseIds.length > 0) {
				setSelectedPerms(savedRaiseIds);
				setLastSyncedKey(activeTargetKey);
				return;
			}

			// Role defaults when no custom RBAC permissions have been saved:
			// 1. Admin: full access across all platform permissions
			if (uRole === "admin") {
				if (permissions.length > 0) {
					setSelectedPerms(permissions.map((p) => p.id));
				}
				setLastSyncedKey(activeTargetKey);
				return;
			}

			const userSections = (targetUser.allowed_sections || "")
				.split(",")
				.map((s) => s.trim())
				.filter(Boolean);
			const allowed = new Set(userSections);

			// 2. Sub-Admin: full access (View, Edit, Delete) for their selected features only
			if (isSubAdminRole(uRole)) {
				if (userSections.length > 0) {
					const subAdminPermIds: number[] = [];
					for (const p of permissions) {
						const sections = RESOURCE_TO_SECTION_MAP[p.resource] || [];
						if (sections.some((s) => sectionGranted(allowed, s))) {
							subAdminPermIds.push(p.id);
						}
					}
					setSelectedPerms(subAdminPermIds);
				} else if (rolePermData?.permissions) {
					setSelectedPerms(rolePermData.permissions.map((p) => p.id));
				} else {
					setSelectedPerms([]);
				}
				setLastSyncedKey(activeTargetKey);
				return;
			}

			// 3. User: View-only for granted Workspace Access sections; Prompt Repo always full access
			const userPermIds: number[] = [];
			for (const p of permissions) {
				if (p.resource === "PromptRepository") {
					userPermIds.push(p.id);
					continue;
				}
				if (userSections.length > 0) {
					const sections = RESOURCE_TO_SECTION_MAP[p.resource] || [];
					if (sections.some((s) => sectionGranted(allowed, s))) {
						if (p.operation === "Read" || p.operation === "View") {
							userPermIds.push(p.id);
						}
					}
				}
			}
			setSelectedPerms(userPermIds);
			setLastSyncedKey(activeTargetKey);
			return;
		}

		if (rolePermData?.permissions) {
			setSelectedPerms(rolePermData.permissions.map((p) => p.id));
			setLastSyncedKey(activeTargetKey);
			return;
		}

		if (activeRoleId === 0) {
			setSelectedPerms([]);
			setLastSyncedKey(activeTargetKey);
		}
	}, [activeTargetKey, lastSyncedKey, target, targetUser, rolePermData, permissions, activeRoleId, scope]);

	// Toggle accordion
	const toggleSection = (section: keyof typeof expandedSections) => {
		setExpandedSections((prev) => ({ ...prev, [section]: !prev[section] }));
	};

	// Group permissions by Resource, ordered like the workspace sidebar (all features visible).
	const groupedPermissions = useMemo(() => {
		const map = new Map<string, RBACPermission[]>();
		for (const perm of permissions) {
			if (resourceSearch.trim()) {
				const q = resourceSearch.toLowerCase();
				const label = (RESOURCE_LABELS[perm.resource] || perm.resource).toLowerCase();
				const matchResource = perm.resource.toLowerCase().includes(q) || label.includes(q);
				const matchOp = perm.operation.toLowerCase().includes(q);
				const matchTier =
					(q === "view" && (perm.operation === "View" || perm.operation === "Read")) ||
					(q === "edit" && (perm.operation === "Create" || perm.operation === "Update")) ||
					(q === "delete" && perm.operation === "Delete") ||
					q.includes("full") ||
					q.includes("access");
				if (!matchResource && !matchOp && !matchTier) continue;
			}
			const list = map.get(perm.resource) || [];
			list.push(perm);
			map.set(perm.resource, list);
		}

		const used = new Set<string>();
		const ordered: [string, RBACPermission[]][] = [];
		for (const group of RESOURCE_GROUPS) {
			for (const name of group.resources) {
				const perms = map.get(name);
				if (!perms) continue;
				ordered.push([name, perms]);
				used.add(name);
			}
		}
		for (const [name, perms] of map.entries()) {
			if (used.has(name)) continue;
			ordered.push([name, perms]);
		}
		return ordered;
	}, [permissions, resourceSearch]);

	const groupedBySection = useMemo(() => {
		const byResource = new Map(groupedPermissions);
		const sections: { title: string; items: [string, RBACPermission[]][] }[] = [];
		const used = new Set<string>();
		for (const group of RESOURCE_GROUPS) {
			const items: [string, RBACPermission[]][] = [];
			for (const name of group.resources) {
				const perms = byResource.get(name);
				if (!perms) continue;
				items.push([name, perms]);
				used.add(name);
			}
			if (items.length > 0) sections.push({ title: group.title, items });
		}
		const other: [string, RBACPermission[]][] = [];
		for (const [name, perms] of groupedPermissions) {
			if (used.has(name)) continue;
			other.push([name, perms]);
		}
		if (other.length > 0) sections.push({ title: "Other", items: other });
		return sections;
	}, [groupedPermissions]);

	// Filtered lists for left pane tree
	const filteredRoles = useMemo(() => {
		const listed = roles.filter((r) => !hasOwnTreeSection(r));
		if (!treeSearch.trim()) return listed;
		const q = treeSearch.toLowerCase();
		return listed.filter((r) => r.name.toLowerCase().includes(q) || r.description?.toLowerCase().includes(q));
	}, [roles, treeSearch]);

	const searchedUsers = useMemo(() => {
		if (!treeSearch.trim()) return sessionUsers;
		const q = treeSearch.toLowerCase();
		return sessionUsers.filter(
			(u) =>
				u.username.toLowerCase().includes(q) ||
				u.email?.toLowerCase().includes(q) ||
				u.role?.toLowerCase().includes(q),
		);
	}, [sessionUsers, treeSearch]);

	const filteredAdmins = useMemo(() => searchedUsers.filter((u) => isAdminRole(u.role)), [searchedUsers]);
	const filteredSubAdmins = useMemo(() => searchedUsers.filter((u) => isSubAdminRole(u.role)), [searchedUsers]);
	const filteredUsers = useMemo(
		() => searchedUsers.filter((u) => !isAdminRole(u.role) && !isSubAdminRole(u.role)),
		[searchedUsers],
	);

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
			const mapped = RESOURCE_TO_SECTION_MAP[p.resource] || [];
			for (const s of mapped) {
				sections.add(s);
			}
		}
		return allowedSectionsToString(sections);
	};

	// Save Action for current Target
	const handleSaveCurrentMatrix = async () => {
		if (!canUpdateRbac) {
			toast.error("You have view-only access to roles and permissions.");
			return;
		}
		try {
			if (target.type === "role") {
				await updatePerms({ id: target.role.id, permission_ids: selectedPerms }).unwrap();
				setLastSyncedKey("");
				toast.success(`Permissions saved for role '${target.role.name}'`);
				return;
			}

			if (target.type === "all_admins") {
				const adminRole = roles.find((r) => r.name.toLowerCase() === "admin");
				if (!adminRole) {
					toast.error("The built-in 'admin' role was not found.");
					return;
				}
				await updatePerms({ id: adminRole.id, permission_ids: selectedPerms }).unwrap();
				setLastSyncedKey("");
				toast.success("Permissions updated for the 'admin' role policy");
				return;
			}

			if (target.type === "all_users") {
				const userRole = roles.find((r) => r.name.toLowerCase() === "user");
				if (!userRole) {
					toast.error("The built-in 'user' role was not found.");
					return;
				}
				await updatePerms({ id: userRole.id, permission_ids: selectedPerms }).unwrap();
				setLastSyncedKey("");
				toast.success("Permissions updated for the 'user' role (applies to all users)");
				return;
			}

			if (target.type === "all_sub_admins") {
				const subAdminRole = roles.find((r) => isSubAdminRole(r.name));
				if (!subAdminRole) {
					toast.error("The built-in 'sub_admin' role was not found.");
					return;
				}
				await updatePerms({ id: subAdminRole.id, permission_ids: selectedPerms }).unwrap();
				setLastSyncedKey("");
				toast.success("Permissions updated for the 'sub_admin' role (applies to all sub admins)");
				return;
			}

			if (target.type === "user") {
				const user = targetUser ?? target.user;
				const uRole = (user.role || "").toLowerCase();
				const sectionsStr = computeAllowedSections(selectedPerms);
				const updates: {
					username: string;
					role: string;
					allowed_sections: string;
					rbac_permission_ids?: string;
				} = {
					username: user.username,
					role: user.role,
					allowed_sections: sectionsStr,
					rbac_permission_ids: selectedPerms.join(","),
				};
				await updateSessionUser({
					id: user.id,
					updates,
				}).unwrap();
				setLastSyncedKey("");
				toast.success(
					`Permissions saved for ${uRole === "admin" ? "admin" : isSubAdminRole(uRole) ? "sub admin" : "user"} '${user.username}'`,
				);
				return;
			}

			if (scope) {
				await updateScopeGrant({
					scope_type: scope.type,
					scope_id: scope.id,
					permission_ids: selectedPerms,
					allowed_sections: computeAllowedSections(selectedPerms),
				}).unwrap();
				setLastSyncedKey("");
				toast.success(
					selectedPerms.length > 0
						? `Permissions saved for ${scope.label}. Members inherit these sections on their next page load.`
						: `Permissions cleared for ${scope.label}.`,
				);
				return;
			}
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	// Quick Select Helpers
	const handleSelectAll = () => {
		if (!canUpdateRbac) return;
		setSelectedPerms(permissions.map((p) => p.id));
		toast.info("Granted all permissions");
	};

	const handleClearAll = () => {
		if (!canUpdateRbac) return;
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
	const handleDeleteRole = (role: RBACRole) => {
		setDeletingRole(role);
	};

	// Direct User Role Assignment
	const handleAssignUserRole = async (userId: string, newRole: string) => {
		if (!canUpdateRbac) {
			toast.error("You have view-only access to roles and permissions.");
			return;
		}
		try {
			await assignUserRole({ id: userId, role_name: newRole }).unwrap();
			setLastSyncedKey("");
			toast.success(`Role updated to '${newRole}'`);
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const roleLabel = (rName: string) => {
		const r = (rName || "user").toLowerCase();
		if (r === "admin") return "Admin";
		if (r === "sub_admin" || r === "subadmin") return "Sub-Admin";
		if (r === "user") return "User";
		return rName;
	};

	// Badge Helper (monochrome enterprise styling, no rainbow colors)
	const getRoleBadgeClass = (rName: string) => {
		const r = rName.toLowerCase();
		if (r === "admin") return "border-border/80 bg-muted/60 text-foreground font-semibold";
		if (r === "sub_admin" || r === "subadmin") return "border-border/70 bg-muted/40 text-foreground/80 font-medium";
		return "border-border/60 bg-muted/20 text-muted-foreground font-normal";
	};

	return (
		<div className="flex h-full w-full flex-col gap-3">
			{/* Top Header */}
			<div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
				<div>
					<h1 className="flex items-center gap-2 text-2xl font-bold tracking-tight">
						<Shield className="h-6 w-6 text-foreground" />
						Roles & Scoped Permissions (RBAC)
					</h1>
					<p className="text-muted-foreground text-xs sm:text-sm">
						Hierarchical Access Control: Configure permissions globally (mothavum) or per individual User, Team, and Customer (separate-avum).
					</p>
				</div>
			</div>

			{rbacListQueryFailed ? (
				<QueryErrorBanner
					testId="rbac-list-query-error"
					message={
						getErrorMessage(rbacListQueryError) ||
						"Failed to load roles, permissions, scope grants, users, teams, or customers."
					}
				/>
			) : null}

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
								placeholder="Search users, teams, customers…"
								value={treeSearch}
								onChange={(e) => setTreeSearch(e.target.value)}
								className="h-8 pl-8 text-xs"
							/>
						</div>
					</div>

					{/* Navigation Tree Accordions */}
					<div className="no-scrollbar flex-1 space-y-2 overflow-y-auto p-2">
						{/* ADMINS SECTION (role policy + individual admins) */}
						<div className="rounded-lg border bg-muted/20">
							<button
								type="button"
								onClick={() => toggleSection("admins")}
								className="flex w-full items-center justify-between px-3 py-2 text-xs font-bold uppercase tracking-wider text-muted-foreground hover:text-foreground"
								data-testid="rbac-tree-admins"
							>
								<div className="flex items-center gap-1.5">
									{expandedSections.admins ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
									<Shield className="h-3.5 w-3.5 text-muted-foreground" />
									<span>Admins</span>
								</div>
								<Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
									{filteredAdmins.length}
								</Badge>
							</button>

							{expandedSections.admins && (
								<div className="space-y-1 p-1 pt-0">
									<div
										onClick={() => setTarget({ type: "all_admins" })}
										className={`flex cursor-pointer items-center gap-2 rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors ${
											target.type === "all_admins"
												? "bg-muted text-foreground font-semibold ring-1 ring-border shadow-2xs"
												: "hover:bg-muted text-muted-foreground hover:text-foreground"
										}`}
									>
										<Globe className="h-3.5 w-3.5 text-muted-foreground" />
										<span>All Admins (Default Full Access)</span>
									</div>

									<div className="border-t pt-1 pl-2 space-y-0.5">
										{filteredAdmins.length === 0 ? (
											<p className="text-muted-foreground px-2 py-1 text-[11px]">No admins yet</p>
										) : (
											filteredAdmins.map((user) => {
												const isSelected = target.type === "user" && target.user.id === user.id;
												return (
													<div
														key={user.id}
														onClick={() => setTarget({ type: "user", user })}
														className={`flex cursor-pointer items-center justify-between rounded-md px-2 py-1 text-xs transition-colors ${
															isSelected
																? "bg-muted text-foreground font-semibold ring-1 ring-border shadow-2xs"
																: "hover:bg-muted text-muted-foreground hover:text-foreground"
														}`}
													>
														<div className="flex items-center gap-1.5 truncate">
															<User className="h-3 w-3 shrink-0 text-muted-foreground" />
															<span className="truncate">{user.username}</span>
														</div>
														<span className={`rounded border px-1 text-[10px] ${getRoleBadgeClass("admin")}`}>
															admin
														</span>
													</div>
												);
											})
										)}
									</div>
								</div>
							)}
						</div>

						{/* SUB ADMINS SECTION (role policy + individual sub admins) */}
						<div className="rounded-lg border bg-muted/20">
							<button
								type="button"
								onClick={() => toggleSection("subAdmins")}
								className="flex w-full items-center justify-between px-3 py-2 text-xs font-bold uppercase tracking-wider text-muted-foreground hover:text-foreground"
								data-testid="rbac-tree-sub-admins"
							>
								<div className="flex items-center gap-1.5">
									{expandedSections.subAdmins ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
									<Shield className="h-3.5 w-3.5 text-muted-foreground" />
									<span>Sub Admins</span>
								</div>
								<Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
									{filteredSubAdmins.length}
								</Badge>
							</button>

							{expandedSections.subAdmins && (
								<div className="space-y-1 p-1 pt-0">
									<div
										onClick={() => setTarget({ type: "all_sub_admins" })}
										className={`flex cursor-pointer items-center gap-2 rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors ${
											target.type === "all_sub_admins"
												? "bg-muted text-foreground font-semibold ring-1 ring-border shadow-2xs"
												: "hover:bg-muted text-muted-foreground hover:text-foreground"
										}`}
									>
										<Globe className="h-3.5 w-3.5 text-muted-foreground" />
										<span>All Sub Admins (Role Policy)</span>
									</div>

									<div className="border-t pt-1 pl-2 space-y-0.5">
										{filteredSubAdmins.length === 0 ? (
											<p className="text-muted-foreground px-2 py-1 text-[11px]">No sub admins yet</p>
										) : (
											filteredSubAdmins.map((user) => {
												const isSelected = target.type === "user" && target.user.id === user.id;
												return (
													<div
														key={user.id}
														onClick={() => setTarget({ type: "user", user })}
														className={`flex cursor-pointer items-center justify-between rounded-md px-2 py-1 text-xs transition-colors ${
															isSelected
																? "bg-muted text-foreground font-semibold ring-1 ring-border shadow-2xs"
																: "hover:bg-muted text-muted-foreground hover:text-foreground"
														}`}
													>
														<div className="flex items-center gap-1.5 truncate">
															<User className="h-3 w-3 shrink-0 text-muted-foreground" />
															<span className="truncate">{user.username}</span>
														</div>
														<span className={`rounded border px-1 text-[10px] ${getRoleBadgeClass(user.role || "sub_admin")}`}>
															{user.role}
														</span>
													</div>
												);
											})
										)}
									</div>
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
									<Users className="h-3.5 w-3.5 text-muted-foreground" />
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
												? "bg-muted text-foreground font-semibold ring-1 ring-border shadow-2xs"
												: "hover:bg-muted text-muted-foreground hover:text-foreground"
										}`}
									>
										<Globe className="h-3.5 w-3.5 text-muted-foreground" />
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
															? "bg-muted text-foreground font-semibold ring-1 ring-border shadow-2xs"
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
									<Building2 className="h-3.5 w-3.5 text-muted-foreground" />
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
												? "bg-muted text-foreground font-semibold ring-1 ring-border shadow-2xs"
												: "hover:bg-muted text-muted-foreground hover:text-foreground"
										}`}
									>
										<Globe className="h-3.5 w-3.5 text-muted-foreground" />
										<span>All Teams</span>
										{scopeGrants?.all_teams && <Check className="ml-auto h-3 w-3 text-foreground" />}
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
															? "bg-muted text-foreground font-semibold ring-1 ring-border shadow-2xs"
															: "hover:bg-muted text-muted-foreground hover:text-foreground"
													}`}
												>
													<div className="flex items-center gap-1.5 truncate">
														<Building2 className="h-3 w-3 shrink-0 text-muted-foreground" />
														<span className="truncate">{team.name}</span>
													</div>
													<span className="flex items-center gap-1 text-[10px] text-muted-foreground">
														{team.customer?.name ? team.customer.name.slice(0, 10) : "Team"}
														{scopeGrants?.teams?.[team.id] && <Check className="h-3 w-3 text-foreground" />}
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
									<Landmark className="h-3.5 w-3.5 text-muted-foreground" />
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
												? "bg-muted text-foreground font-semibold ring-1 ring-border shadow-2xs"
												: "hover:bg-muted text-muted-foreground hover:text-foreground"
										}`}
									>
										<Globe className="h-3.5 w-3.5 text-muted-foreground" />
										<span>All Customers</span>
										{scopeGrants?.all_customers && <Check className="ml-auto h-3 w-3 text-foreground" />}
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
															? "bg-muted text-foreground font-semibold ring-1 ring-border shadow-2xs"
															: "hover:bg-muted text-muted-foreground hover:text-foreground"
													}`}
												>
													<div className="flex items-center gap-1.5 truncate">
														<Landmark className="h-3 w-3 shrink-0 text-muted-foreground" />
														<span className="truncate">{cust.name}</span>
													</div>
													{scopeGrants?.customers?.[cust.id] && <Check className="h-3 w-3 shrink-0 text-foreground" />}
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
										{target.type === "all_admins" && "All Admins (Default Full Access Policy)"}
										{target.type === "all_users" && "All Users (Bulk Global Policy)"}
										{target.type === "all_sub_admins" && "All Sub Admins (sub_admin role)"}
										{target.type === "user" &&
											`${isAdminRole(targetUser?.role ?? target.user.role) ? "Admin" : isSubAdminRole(targetUser?.role ?? target.user.role) ? "Sub Admin" : "User"}: ${target.user.username}`}
										{target.type === "all_teams" && "All Teams"}
										{target.type === "team" && `Team: ${target.team.name}`}
										{target.type === "all_customers" && "All Customers"}
										{target.type === "customer" && `Customer: ${target.customer.name}`}
									</span>
								</div>

								{/* Target Metadata */}
								<div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
									{target.type === "user" && targetUser && (
										<>
											<span className="font-medium text-foreground">Role:</span>
											<span
												className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${
													(targetUser.role || "user").toLowerCase() === "admin"
														? "border-teal-500/30 bg-teal-500/10 text-teal-400"
														: (targetUser.role || "user").toLowerCase() === "sub_admin"
															? "border-purple-500/30 bg-purple-500/10 text-purple-400"
															: "border-blue-500/30 bg-blue-500/10 text-blue-400"
												}`}
											>
												{roleLabel(targetUser.role || "user")}
											</span>
											<span>• Email: {targetUser.email || "No email"}</span>
											{inheritedForUser && (
												<span className="text-muted-foreground">
													• Also inherits {inheritedForUser.count} section{inheritedForUser.count === 1 ? "" : "s"} from{" "}
													{inheritedForUser.sources.join(", ")}
												</span>
											)}
										</>
									)}

									{scope && (
										<span>
											{target.type === "all_teams" && "Every member who is in any team inherits these sections."}
											{target.type === "team" && `Members of ${target.team.name} inherit these sections.`}
											{target.type === "all_customers" &&
												"Members of any team that belongs to a customer inherit these sections."}
											{target.type === "customer" && `Members of teams under ${target.customer.name} inherit these sections.`}{" "}
											Their own sections are kept, and API access still follows each member's role. •{" "}
											{scope.grant
												? `Saved: ${countSections(scope.grant.allowed_sections)} section(s)${
														scope.grant.updated_at ? ` on ${new Date(scope.grant.updated_at).toLocaleString()}` : ""
													}`
												: "Nothing saved yet"}
										</span>
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
								<Button variant="outline" size="sm" onClick={handleSelectAll} disabled={!canUpdateRbac} className="h-8 text-xs font-medium">
									Full Access (All)
								</Button>
								<Button variant="outline" size="sm" onClick={handleClearAll} disabled={!canUpdateRbac} className="h-8 text-xs font-medium">
									Clear
								</Button>
								<Button
									size="sm"
									onClick={() => void handleSaveCurrentMatrix()}
									disabled={!canUpdateRbac || isUpdatingPerms || isSavingScope || isUpdatingUser}
									className="h-8 bg-emerald-600 text-white hover:bg-emerald-700 text-xs font-semibold px-4 disabled:opacity-50"
								>
									{isUpdatingPerms || isSavingScope || isUpdatingUser ? "Saving..." : "Save Permissions"}
								</Button>
							</div>
						</div>

						{!canUpdateRbac && (
							<div className="flex items-center gap-2 rounded-md bg-muted/50 border border-border px-3 py-2 text-xs text-muted-foreground font-medium mt-3" data-testid="rbac-view-only-banner">
								<Eye className="h-4 w-4 shrink-0" />
								<span>View-only access: roles and permissions cannot be modified.</span>
							</div>
						)}

						{/* Resource Filter */}
						<div className="relative mt-3">
							<Search className="text-muted-foreground absolute top-2.5 left-2.5 h-3.5 w-3.5" />
							<Input
								placeholder="Filter resources (e.g. Logs, VirtualKeys, Governance, or type 'edit', 'delete')…"
								value={resourceSearch}
								onChange={(e) => setResourceSearch(e.target.value)}
								className="h-8 pl-8 text-xs"
							/>
						</div>
					</div>

					{/* Explanation Notice */}
					<div className="border-b bg-muted/30 px-4 py-2.5 text-xs flex items-center gap-2 text-muted-foreground">
						<Info className="h-4 w-4 shrink-0 text-muted-foreground" />
						<span>
							Permissions are configured into 4 clear levels: <strong>View</strong> (read-only & exports), <strong>Create</strong> (add new records), <strong>Edit</strong> (modify existing data), <strong>Delete</strong> (remove records), or <strong>Full Access</strong>. Showing{" "}
							<strong>{groupedPermissions.length}</strong> workspace features across 8 primary categories.
							{isTargetUserRole && (
								<span className="ml-1 text-foreground/80 font-medium">
									• User role defaults to View-only across workspace features (Full Access on Prompt Repository). Admins and sub-admins can edit and raise permissions.
								</span>
							)}
						</span>
					</div>

					{/* Resource Cards Grid — grouped like the sidebar so every feature is discoverable */}
					<div className="no-scrollbar flex-1 space-y-5 overflow-y-auto p-4">
						{groupedBySection.length === 0 ? (
							<p className="text-muted-foreground py-8 text-center text-sm">No resources matching filter.</p>
						) : (
							groupedBySection.map((section) => (
								<div key={section.title} className="space-y-3">
									<div className="sticky top-0 z-10 -mx-1 flex items-center justify-between rounded-md bg-background/95 px-1 py-1.5 backdrop-blur-sm">
										<h3 className="text-muted-foreground text-xs font-semibold tracking-wider uppercase">
											{section.title}
										</h3>
										<span className="text-muted-foreground text-[10px] tabular-nums">
											{section.items.length} feature{section.items.length === 1 ? "" : "s"}
										</span>
									</div>
									{section.items.map(([resource, perms]) => {
										const resourceLabel = RESOURCE_LABELS[resource] || resource;

										const viewPerms = perms.filter((p) => p.operation === "Read" || p.operation === "View" || p.operation === "Download");
										const createPerms = perms.filter((p) => p.operation === "Create");
										const editPerms = perms.filter((p) => p.operation === "Update");
										const deletePerms = perms.filter((p) => p.operation === "Delete");

										const allPermIds = perms.map((p) => p.id);
										const viewIds = viewPerms.map((p) => p.id);
										const createIds = createPerms.map((p) => p.id);
										const editIds = editPerms.map((p) => p.id);
										const deleteIds = deletePerms.map((p) => p.id);

										const isViewActive = viewIds.length > 0 && viewIds.some((id) => selectedPerms.includes(id));
										const isCreateActive = createIds.length > 0 && createIds.some((id) => selectedPerms.includes(id));
										const isEditActive = editIds.length > 0 && editIds.some((id) => selectedPerms.includes(id));
										const isDeleteActive = deleteIds.length > 0 && deleteIds.some((id) => selectedPerms.includes(id));
										const isFullAccessActive = allPermIds.length > 0 && allPermIds.every((id) => selectedPerms.includes(id));

										const toggleView = () => {
											if (isViewActive) {
												const toRemove = new Set(allPermIds);
												setSelectedPerms((prev) => prev.filter((id) => !toRemove.has(id)));
											} else {
												setSelectedPerms((prev) => Array.from(new Set([...prev, ...viewIds])));
											}
										};

										const toggleCreate = () => {
											if (isCreateActive) {
												const toRemove = new Set(createIds);
												setSelectedPerms((prev) => prev.filter((id) => !toRemove.has(id)));
											} else {
												setSelectedPerms((prev) => Array.from(new Set([...prev, ...viewIds, ...createIds])));
											}
										};

										const toggleEdit = () => {
											if (isEditActive) {
												const toRemove = new Set(editIds);
												setSelectedPerms((prev) => prev.filter((id) => !toRemove.has(id)));
											} else {
												setSelectedPerms((prev) => Array.from(new Set([...prev, ...viewIds, ...editIds])));
											}
										};

										const toggleDelete = () => {
											if (isDeleteActive) {
												const toRemove = new Set(deleteIds);
												setSelectedPerms((prev) => prev.filter((id) => !toRemove.has(id)));
											} else {
												setSelectedPerms((prev) => Array.from(new Set([...prev, ...viewIds, ...deleteIds])));
											}
										};

										const toggleFullAccess = () => {
											if (isFullAccessActive) {
												const toRemove = new Set(allPermIds);
												setSelectedPerms((prev) => prev.filter((id) => !toRemove.has(id)));
											} else {
												setSelectedPerms((prev) => Array.from(new Set([...prev, ...allPermIds])));
											}
										};

										const renderStatusBadge = () => {
											if (isFullAccessActive) {
												return (
													<Badge variant="outline" className="bg-foreground/10 text-foreground border-foreground/30 text-[11px] font-semibold gap-1">
														<Check className="h-3 w-3 text-foreground" />
														Full Access
													</Badge>
												);
											}
											if (isCreateActive && isEditActive && isDeleteActive) {
												return (
													<Badge variant="outline" className="bg-muted/60 text-foreground border-border/80 text-[11px] font-medium">
														Create, Edit & Delete
													</Badge>
												);
											}
											if (isCreateActive && isEditActive) {
												return (
													<Badge variant="outline" className="bg-muted/60 text-foreground border-border/80 text-[11px] font-medium">
														Create & Edit
													</Badge>
												);
											}
											if (isEditActive && isDeleteActive) {
												return (
													<Badge variant="outline" className="bg-muted/60 text-foreground border-border/80 text-[11px] font-medium">
														Edit & Delete
													</Badge>
												);
											}
											if (isCreateActive) {
												return (
													<Badge variant="outline" className="bg-muted/60 text-foreground border-border/80 text-[11px] font-medium">
														View & Create
													</Badge>
												);
											}
											if (isEditActive) {
												return (
													<Badge variant="outline" className="bg-muted/60 text-foreground border-border/80 text-[11px] font-medium">
														View & Edit
													</Badge>
												);
											}
											if (isDeleteActive) {
												return (
													<Badge variant="outline" className="bg-muted/60 text-foreground border-border/80 text-[11px] font-medium">
														View & Delete
													</Badge>
												);
											}
											if (isViewActive) {
												return (
													<Badge variant="outline" className="bg-muted/60 text-foreground border-border/80 text-[11px] font-medium">
														View Only
													</Badge>
												);
											}
											return (
												<Badge variant="outline" className="text-muted-foreground/60 text-[11px] font-normal border-border/50">
													No Access
												</Badge>
											);
										};

										return (
											<div
												key={resource}
												className="bg-card/70 hover:bg-card/95 rounded-xl border p-3.5 transition-all shadow-2xs hover:shadow-xs"
											>
												{/* Resource Header */}
												<div className="mb-2.5 flex items-center justify-between">
													<div className="flex min-w-0 items-center gap-2">
														<span className="truncate font-semibold text-sm tracking-tight text-foreground" title={resource}>
															{resourceLabel}
														</span>
														{renderStatusBadge()}
													</div>
												</div>

												{/* 5-tier clean toggle buttons: View, Create, Edit, Delete, Full Access */}
												<div className="grid grid-cols-2 sm:grid-cols-5 gap-2 pt-1">
													{/* 1. VIEW */}
													<button
														type="button"
														disabled={!canUpdateRbac}
														onClick={canUpdateRbac ? toggleView : undefined}
														className={`h-8 rounded-lg border text-xs font-medium transition-all duration-150 flex items-center justify-center px-2.5 ${
															!canUpdateRbac ? "cursor-not-allowed opacity-75" : "cursor-pointer"
														} ${
															isViewActive
																? "border-foreground/30 bg-foreground/10 text-foreground font-semibold shadow-2xs ring-1 ring-foreground/15"
																: "border-border/70 bg-card hover:bg-muted/50 text-muted-foreground hover:text-foreground"
														}`}
														title="View data, filter and download records"
													>
														View
													</button>

													{/* 2. CREATE */}
													<button
														type="button"
														disabled={!canUpdateRbac}
														onClick={canUpdateRbac ? toggleCreate : undefined}
														className={`h-8 rounded-lg border text-xs font-medium transition-all duration-150 flex items-center justify-center px-2.5 ${
															!canUpdateRbac ? "cursor-not-allowed opacity-75" : "cursor-pointer"
														} ${
															isCreateActive
																? "border-foreground/30 bg-foreground/10 text-foreground font-semibold shadow-2xs ring-1 ring-foreground/15"
																: "border-border/70 bg-card hover:bg-muted/50 text-muted-foreground hover:text-foreground"
														}`}
														title="Create new records"
													>
														Create
													</button>

													{/* 3. EDIT */}
													<button
														type="button"
														disabled={!canUpdateRbac}
														onClick={canUpdateRbac ? toggleEdit : undefined}
														className={`h-8 rounded-lg border text-xs font-medium transition-all duration-150 flex items-center justify-center px-2.5 ${
															!canUpdateRbac ? "cursor-not-allowed opacity-75" : "cursor-pointer"
														} ${
															isEditActive
																? "border-foreground/30 bg-foreground/10 text-foreground font-semibold shadow-2xs ring-1 ring-foreground/15"
																: "border-border/70 bg-card hover:bg-muted/50 text-muted-foreground hover:text-foreground"
														}`}
														title="Modify existing records"
													>
														Edit
													</button>

													{/* 4. DELETE */}
													<button
														type="button"
														disabled={!canUpdateRbac}
														onClick={canUpdateRbac ? toggleDelete : undefined}
														className={`h-8 rounded-lg border text-xs font-medium transition-all duration-150 flex items-center justify-center px-2.5 ${
															!canUpdateRbac ? "cursor-not-allowed opacity-75" : "cursor-pointer"
														} ${
															isDeleteActive
																? "border-foreground/30 bg-foreground/10 text-foreground font-semibold shadow-2xs ring-1 ring-foreground/15"
																: "border-border/70 bg-card hover:bg-muted/50 text-muted-foreground hover:text-foreground"
														}`}
														title="Delete records"
													>
														Delete
													</button>

													{/* 5. FULL ACCESS */}
													<button
														type="button"
														disabled={!canUpdateRbac}
														onClick={canUpdateRbac ? toggleFullAccess : undefined}
														className={`h-8 rounded-lg border text-xs font-semibold transition-all duration-150 flex items-center justify-center px-2.5 ${
															!canUpdateRbac ? "cursor-not-allowed opacity-75" : "cursor-pointer"
														} ${
															isFullAccessActive
																? "border-foreground bg-foreground text-background font-semibold shadow-xs"
																: "border-border/70 bg-card hover:bg-muted/50 text-muted-foreground hover:text-foreground"
														}`}
														title="Grant full access (View, Create, Edit, Delete)"
													>
														Full Access
													</button>
												</div>
											</div>
										);
									})}
								</div>
							))
						)}
					</div>
				</div>
			</div>

			{/* Create Role Modal */}
			<Dialog open={createRoleOpen} onOpenChange={setCreateRoleOpen}>
				<DialogContent className="sm:max-w-md no-scrollbar">
					<DialogHeader>
						<DialogTitle className="flex items-center gap-2">
							<Shield className="h-5 w-5 text-foreground" />
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

			<Dialog open={!!deletingRole} onOpenChange={(open) => { if (!open && !isDeletingRole) setDeletingRole(null); }}>
				<DialogContent className="sm:max-w-md">
					<DialogHeader>
						<DialogTitle>Delete Role</DialogTitle>
						<DialogDescription>
							Are you sure you want to delete role <span className="font-semibold text-foreground">{deletingRole?.name}</span>? This action cannot be undone.
						</DialogDescription>
					</DialogHeader>
					<DialogFooter className="gap-2 sm:gap-0">
						<Button variant="outline" disabled={isDeletingRole} onClick={() => setDeletingRole(null)}>
							Cancel
						</Button>
						<Button
							variant="destructive"
							disabled={isDeletingRole}
							onClick={async () => {
								if (!deletingRole) return;
								try {
									await deleteRole(deletingRole.id).unwrap();
									if (target.type === "role" && target.role.id === deletingRole.id) {
										setTarget({ type: "all_sub_admins" });
									}
									toast.success(`Role '${deletingRole.name}' deleted`);
									setDeletingRole(null);
								} catch (err) {
									toast.error(getErrorMessage(err));
								}
							}}
						>
							{isDeletingRole ? "Deleting..." : "Delete"}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}
