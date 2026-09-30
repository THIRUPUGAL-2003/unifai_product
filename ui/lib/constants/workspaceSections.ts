/** Sidebar sections/items super-admin can grant to sub-admins. Keys must stay stable in DB. */

export type WorkspaceSectionKey =
	| "observability"
	| "browser-ai"
	| "models"
	| "mcp-gateway"
	| "plugins"
	| "governance"
	| "guardrails"
	| "cluster-config"
	| "adaptive-routing"
	| "prompt-repository"
	| "skills-repository"
	| "settings";

/** Parent key (`observability`) or child grant (`observability/llm-logs`). Legacy: `observability/browser-ai` → `browser-ai`. */
export type WorkspaceGrantKey = string;

/** Old nested grant → new top-level Browser AI section. */
const LEGACY_BROWSER_AI_GRANT = "observability/browser-ai";

export type WorkspaceSectionItem = {
	key: string;
	label: string;
	path: string;
};

export type WorkspaceSection = {
	key: WorkspaceSectionKey;
	label: string;
	defaultPath: string;
	items?: readonly WorkspaceSectionItem[];
};

export const WORKSPACE_SECTIONS: readonly WorkspaceSection[] = [
	{
		key: "observability",
		label: "Observability",
		defaultPath: "/workspace/logs",
		items: [
			{ key: "dashboard", label: "Dashboard", path: "/workspace/dashboard" },
			{ key: "llm-logs", label: "LLM Logs", path: "/workspace/logs" },
			{ key: "mcp-logs", label: "MCP Logs", path: "/workspace/mcp-logs" },
			{ key: "connectors", label: "Connectors", path: "/workspace/observability" },
			{ key: "logs-settings", label: "Logs Settings", path: "/workspace/config/logging" },
		],
	},
	{
		key: "browser-ai",
		label: "Browser AI",
		defaultPath: "/workspace/browser-ai",
		items: [
			{ key: "overview", label: "Overview", path: "/workspace/browser-ai?tab=overview" },
			{ key: "targets", label: "Target Websites", path: "/workspace/browser-ai?tab=targets" },
			{ key: "rules", label: "Guard Rules", path: "/workspace/browser-ai?tab=rules" },
			{ key: "logs", label: "Prompt Logs", path: "/workspace/browser-ai?tab=logs" },
			{ key: "search-logs", label: "Search Logs", path: "/workspace/browser-ai?tab=search-logs" },
			{ key: "agents", label: "Guard Agents", path: "/workspace/browser-ai?tab=agents" },
			{ key: "telemetry", label: "Guard Insights", path: "/workspace/browser-ai?tab=telemetry" },
			{ key: "setup", label: "Setup", path: "/workspace/browser-ai?tab=setup" },
		],
	},
	{
		key: "models",
		label: "Models",
		defaultPath: "/workspace/providers",
		items: [
			{ key: "model-providers", label: "Model Providers", path: "/workspace/providers" },
			{ key: "model-catalog", label: "Model Catalog", path: "/workspace/model-catalog" },
			{ key: "budgets-limits", label: "Budgets & Limits", path: "/workspace/model-limits" },
			{ key: "complexity-router", label: "Complexity Router", path: "/workspace/complexity-router" },
			{ key: "routing-rules", label: "Routing Rules", path: "/workspace/routing-rules" },
			{ key: "circuit-breaker", label: "Circuit Breaker", path: "/workspace/circuit-breaker" },
			{ key: "pricing-overrides", label: "Pricing Overrides", path: "/workspace/custom-pricing/overrides" },
			{ key: "model-settings", label: "Model Settings", path: "/workspace/custom-pricing" },
		],
	},
	{
		key: "mcp-gateway",
		label: "MCP Gateway",
		defaultPath: "/workspace/mcp-registry",
		items: [
			{ key: "mcp-catalog", label: "MCP Catalog", path: "/workspace/mcp-registry" },
			{ key: "mcp-library", label: "MCP Library", path: "/workspace/mcp-registry/library" },
			{ key: "tool-groups", label: "Tool Groups", path: "/workspace/mcp-tool-groups" },
			{ key: "auth-sessions", label: "Auth Sessions", path: "/workspace/mcp-sessions" },
			{ key: "oauth-grants", label: "OAuth Grants", path: "/workspace/oauth-grants" },
			{ key: "mcp-settings", label: "MCP Settings", path: "/workspace/mcp-settings" },
		],
	},
	{
		key: "plugins",
		label: "Plugins",
		defaultPath: "/workspace/plugins",
	},
	{
		key: "governance",
		label: "Governance",
		defaultPath: "/workspace/governance",
		items: [
			{ key: "virtual-keys", label: "Virtual Keys", path: "/workspace/governance/virtual-keys" },
			{ key: "users", label: "Users", path: "/workspace/governance/users" },
			{ key: "teams", label: "Teams", path: "/workspace/governance/teams" },
			{ key: "business-units", label: "Business Units", path: "/workspace/governance/business-units" },
			{ key: "customers", label: "Customers", path: "/workspace/governance/customers" },
			{ key: "user-provisioning", label: "User Provisioning", path: "/workspace/scim" },
			{ key: "roles-permissions", label: "Roles & Permissions", path: "/workspace/governance/rbac" },
			{ key: "access-profiles", label: "Access Profiles", path: "/workspace/governance/access-profiles" },
			{ key: "audit-logs", label: "Audit Logs", path: "/workspace/audit-logs" },
		],
	},
	{
		key: "guardrails",
		label: "Guardrails",
		defaultPath: "/workspace/guardrails",
		items: [
			{ key: "rules", label: "Rules", path: "/workspace/guardrails/configuration" },
			{ key: "providers", label: "Rule Providers", path: "/workspace/guardrails/providers" },
			{ key: "cluster-config", label: "Cluster Config", path: "/workspace/cluster" },
		],
	},
	/** Legacy top-level key — still accepted from DB; UI nests under Guardrails. */
	{
		key: "cluster-config",
		label: "Cluster Config",
		defaultPath: "/workspace/cluster",
	},
	{
		key: "adaptive-routing",
		label: "Adaptive Routing",
		defaultPath: "/workspace/adaptive-routing",
		items: [
			{ key: "dashboard", label: "Dashboard", path: "/workspace/adaptive-routing" },
			{ key: "settings", label: "Settings", path: "/workspace/adaptive-routing/settings" },
		],
	},
	{
		key: "prompt-repository",
		label: "Prompt Repository",
		defaultPath: "/workspace/prompt-repo",
	},
	{
		key: "skills-repository",
		label: "Skills Repository",
		defaultPath: "/workspace/skills-repo",
	},
	{
		key: "settings",
		label: "Settings",
		defaultPath: "/workspace/config",
		items: [
			{ key: "client-settings", label: "Client Settings", path: "/workspace/config/client-settings" },
			{ key: "compatibility", label: "Compatibility", path: "/workspace/config/compatibility" },
			{ key: "caching", label: "Caching", path: "/workspace/config/caching" },
			{ key: "security", label: "Security", path: "/workspace/config/security" },
			{ key: "proxy", label: "Proxy", path: "/workspace/config/proxy" },
			{ key: "api-keys", label: "API Keys", path: "/workspace/config/api-keys" },
			{ key: "performance-tuning", label: "Performance Tuning", path: "/workspace/config/performance-tuning" },
			{ key: "feature-flags", label: "Feature Flags", path: "/workspace/config/feature-flags" },
		],
	},
] as const;

/** Sections shown in the Workspace Access picker (hide legacy duplicate cluster-config row). */
export const WORKSPACE_ACCESS_SECTIONS = WORKSPACE_SECTIONS.filter((s) => s.key !== "cluster-config");

export const DEFAULT_USER_SECTIONS = "prompt-repository";

export const SECTION_KEY_BY_TITLE: Record<string, WorkspaceSectionKey> = Object.fromEntries(
	WORKSPACE_SECTIONS.map((s) => [s.label, s.key]),
) as Record<string, WorkspaceSectionKey>;

export function itemGrantKey(sectionKey: WorkspaceSectionKey, itemKey: string): WorkspaceGrantKey {
	return `${sectionKey}/${itemKey}`;
}

export function pathMatches(pathname: string, prefix: string): boolean {
	return pathname === prefix || pathname.startsWith(`${prefix}/`);
}

function pathsForSection(section: WorkspaceSection): string[] {
	const paths = new Set<string>([section.defaultPath]);
	for (const i of section.items ?? []) {
		paths.add(i.path);
	}
	return Array.from(paths);
}

function normalizeGrants(grants: Set<WorkspaceGrantKey>): Set<WorkspaceGrantKey> {
	if (!grants.has(LEGACY_BROWSER_AI_GRANT)) return grants;
	const next = new Set(grants);
	next.delete(LEGACY_BROWSER_AI_GRANT);
	next.add("browser-ai");
	return next;
}

/** Expand stored grants into concrete path prefixes (longest first for matching). */
export function expandGrantsToPaths(grants: Set<WorkspaceGrantKey>): string[] {
	const normalized = normalizeGrants(grants);
	const paths = new Set<string>();

	for (const section of WORKSPACE_SECTIONS) {
		if (normalized.has(section.key)) {
			for (const p of pathsForSection(section)) paths.add(p);
			continue;
		}
		if (!section.items) continue;
		for (const item of section.items) {
			if (normalized.has(itemGrantKey(section.key, item.key))) {
				paths.add(item.path);
			}
		}
	}

	// Legacy: cluster-config parent key → cluster path (also covered by guardrails/cluster-config)
	if (normalized.has("cluster-config")) {
		paths.add("/workspace/cluster");
	}

	return Array.from(paths).sort((a, b) => b.length - a.length);
}

export function parseAllowedSections(raw?: string | null): Set<WorkspaceGrantKey> {
	const trimmed = (raw || "").trim();
	if (!trimmed) {
		return new Set([DEFAULT_USER_SECTIONS]);
	}
	const keys = trimmed
		.split(",")
		.map((s) => s.trim())
		.filter(Boolean);
	return normalizeGrants(new Set(keys.length > 0 ? keys : [DEFAULT_USER_SECTIONS]));
}

/** Sub-admin: empty/null stored value = full workspace access. Non-empty = limited grants. */
export function parseAdminAllowedSections(raw?: string | null): Set<WorkspaceGrantKey> | null {
	const trimmed = (raw || "").trim();
	if (!trimmed) {
		return null;
	}
	const keys = trimmed
		.split(",")
		.map((s) => s.trim())
		.filter(Boolean);
	return keys.length > 0 ? normalizeGrants(new Set(keys)) : null;
}

/** Form state when editing an admin — unchecked = full access. */
export function adminSectionsFromStorage(raw?: string | null): Set<WorkspaceGrantKey> {
	const trimmed = (raw || "").trim();
	if (!trimmed) {
		return new Set();
	}
	return normalizeGrants(
		new Set(
			trimmed
				.split(",")
				.map((s) => s.trim())
				.filter(Boolean),
		),
	);
}

/** Normalize UI selection: all children checked → store parent key only. */
export function allowedSectionsToString(sections: Set<WorkspaceGrantKey>): string {
	const out = new Set<string>();

	for (const section of WORKSPACE_ACCESS_SECTIONS) {
		if (sections.has(section.key)) {
			out.add(section.key);
			continue;
		}
		if (!section.items?.length) continue;
		const selectedChildren = section.items.filter((item) => sections.has(itemGrantKey(section.key, item.key)));
		if (selectedChildren.length === 0) continue;
		if (selectedChildren.length === section.items.length) {
			out.add(section.key);
		} else {
			for (const item of selectedChildren) {
				out.add(itemGrantKey(section.key, item.key));
			}
		}
	}

	// Preserve unknown legacy keys (e.g. bare cluster-config) if still present and not covered
	for (const key of sections) {
		if (key === "cluster-config" && !out.has("guardrails") && !out.has("guardrails/cluster-config")) {
			out.add("cluster-config");
		}
	}

	return Array.from(out).join(",");
}

/** Longest matching catalog path for a URL (avoids /custom-pricing catching /overrides). */
function findBestCatalogMatch(
	pathname: string,
): { sectionKey: WorkspaceSectionKey; itemKey?: string; path: string } | null {
	let best: { sectionKey: WorkspaceSectionKey; itemKey?: string; path: string } | null = null;
	for (const section of WORKSPACE_SECTIONS) {
		const candidates: { sectionKey: WorkspaceSectionKey; itemKey?: string; path: string }[] = [
			{ sectionKey: section.key, path: section.defaultPath },
		];
		for (const i of section.items ?? []) {
			candidates.push({ sectionKey: section.key, itemKey: i.key, path: i.path });
		}
		for (const c of candidates) {
			if (!pathMatches(pathname, c.path)) continue;
			if (!best || c.path.length > best.path.length) {
				best = c;
			}
		}
	}
	return best;
}

export function isPathAllowedForUser(pathname: string, allowedSections: Set<WorkspaceGrantKey>): boolean {
	const grants = normalizeGrants(allowedSections);
	// Prefer exact catalog match including ?tab=… (Browser AI child grants).
	const best = findBestCatalogMatch(pathname);
	if (!best) {
		return expandGrantsToPaths(grants).some((prefix) => pathMatches(pathname, prefix));
	}
	if (grants.has(best.sectionKey)) return true;
	if (best.itemKey && grants.has(itemGrantKey(best.sectionKey, best.itemKey))) return true;
	// Section root URL with only child grants — allow landing on default path if any child granted
	const section = WORKSPACE_SECTIONS.find((s) => s.key === best.sectionKey);
	if (!best.itemKey && section && best.path === section.defaultPath) {
		if (section.items?.some((item) => grants.has(itemGrantKey(section.key, item.key)))) {
			const pathOnly = pathname.split("?")[0];
			return pathOnly === section.defaultPath;
		}
	}
	if (best.sectionKey === "guardrails" && best.itemKey === "cluster-config" && grants.has("cluster-config")) {
		return true;
	}
	return false;
}

export function getDefaultPathForSections(allowedSections: Set<WorkspaceGrantKey>): string {
	const grants = normalizeGrants(allowedSections);
	const prefixes = expandGrantsToPaths(grants);
	if (prefixes.length > 0) {
		// Prefer first section's default among granted
		for (const section of WORKSPACE_ACCESS_SECTIONS) {
			if (grants.has(section.key)) {
				return section.defaultPath;
			}
			if (section.items?.some((item) => grants.has(itemGrantKey(section.key, item.key)))) {
				const first = section.items.find((item) => grants.has(itemGrantKey(section.key, item.key)));
				if (first) return first.path;
			}
		}
		return prefixes[prefixes.length - 1]; // shortest / first-ish
	}
	return "/workspace/prompt-repo";
}

/** True if this sidebar section title should appear for the grant set. */
export function isSectionGranted(sectionKey: WorkspaceSectionKey, grants: Set<WorkspaceGrantKey>): boolean {
	const normalized = normalizeGrants(grants);
	if (normalized.has(sectionKey)) return true;
	const section = WORKSPACE_SECTIONS.find((s) => s.key === sectionKey);
	if (!section?.items) return false;
	if (section.items.some((item) => normalized.has(itemGrantKey(sectionKey, item.key)))) return true;
	// Legacy cluster-config unlocks Guardrails section visibility for that item
	if (sectionKey === "guardrails" && normalized.has("cluster-config")) return true;
	return false;
}

/** True if a sidebar sub-item path is covered by grants. */
export function isSidebarItemGranted(
	sectionKey: WorkspaceSectionKey,
	itemPath: string,
	grants: Set<WorkspaceGrantKey>,
): boolean {
	const normalized = normalizeGrants(grants);
	if (normalized.has(sectionKey)) return true;
	const section = WORKSPACE_SECTIONS.find((s) => s.key === sectionKey);
	if (!section) return false;
	if (!section.items?.length) {
		return normalized.has(sectionKey) && pathMatches(itemPath, section.defaultPath);
	}
	let best: WorkspaceSectionItem | null = null;
	for (const i of section.items) {
		if (itemPath === i.path || pathMatches(itemPath, i.path)) {
			if (!best || i.path.length > best.path.length) best = i;
		}
	}
	if (!best) return false;
	if (normalized.has(itemGrantKey(sectionKey, best.key))) return true;
	if (sectionKey === "guardrails" && best.key === "cluster-config" && normalized.has("cluster-config")) return true;
	return false;
}

export function sectionSelectionState(
	section: WorkspaceSection,
	grants: Set<WorkspaceGrantKey>,
): "all" | "some" | "none" {
	const normalized = normalizeGrants(grants);
	if (normalized.has(section.key)) return "all";
	if (!section.items?.length) return "none";
	const selected = section.items.filter((item) => normalized.has(itemGrantKey(section.key, item.key)));
	if (section.key === "guardrails" && normalized.has("cluster-config")) {
		const cluster = section.items.find((i) => i.key === "cluster-config");
		if (cluster && !selected.includes(cluster)) selected.push(cluster);
	}
	if (selected.length === 0) return "none";
	if (selected.length === section.items.length) return "all";
	return "some";
}
