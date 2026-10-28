package handlers

import (
	"strconv"
	"strings"

	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/rbac"
)

// isSectionGrantedForPath reports whether the granted allowed_sections string authorizes
// read access to the given API path.
func isSectionGrantedForPath(granted, path string) bool {
	if strings.TrimSpace(granted) == "" {
		return false
	}
	grantList := strings.Split(strings.ToLower(granted), ",")
	hasGrant := func(keys ...string) bool {
		for _, key := range keys {
			for _, g := range grantList {
				g = strings.TrimSpace(g)
				if g == key || g == "*" {
					return true
				}
				if parent, _, found := strings.Cut(key, "/"); found && g == parent {
					return true
				}
				if parent, _, found := strings.Cut(g, "/"); found && key == parent {
					return true
				}
			}
		}
		return false
	}

	switch {
	case strings.HasPrefix(path, "/api/logs"),
		strings.HasPrefix(path, "/api/mcp-logs"),
		strings.HasPrefix(path, "/api/mcp/logs"),
		strings.HasPrefix(path, "/api/mcp/stats"),
		strings.HasPrefix(path, "/api/mcp/histogram"),
		strings.HasPrefix(path, "/api/mcp/filterdata"),
		strings.HasPrefix(path, "/api/connectors"):
		return hasGrant("observability", "observability/llm-logs", "observability/dashboard", "observability/connectors", "observability/mcp-logs", "observability/logs-settings")

	case strings.HasPrefix(path, "/api/browser-ai"):
		return hasGrant("browser-ai", "browser-ai/overview", "browser-ai/targets", "browser-ai/rules", "browser-ai/logs", "browser-ai/search-logs", "browser-ai/agents", "browser-ai/telemetry", "browser-ai/setup", "observability", "observability/llm-logs")

	case strings.HasPrefix(path, "/api/governance/virtual-keys"):
		return hasGrant("governance", "governance/virtual-keys", "prompt-repository")

	case strings.HasPrefix(path, "/api/governance/users"),
		strings.HasPrefix(path, "/api/session/users"),
		strings.HasPrefix(path, "/api/users"):
		return hasGrant("governance", "governance/users", "governance/roles-permissions", "governance/virtual-keys", "governance/teams", "prompt-repository")

	case strings.HasPrefix(path, "/api/governance/teams"),
		strings.HasPrefix(path, "/api/teams"):
		return hasGrant("governance", "governance/teams", "governance/business-units")

	case strings.HasPrefix(path, "/api/governance/business-units"),
		strings.HasPrefix(path, "/api/business-units"):
		return hasGrant("governance", "governance/business-units", "governance/teams")

	case strings.HasPrefix(path, "/api/governance/customers"),
		strings.HasPrefix(path, "/api/customers"):
		return hasGrant("governance", "governance/customers")

	case strings.HasPrefix(path, "/api/audit-logs"):
		return hasGrant("governance", "governance/audit-logs")

	case strings.HasPrefix(path, "/api/scim"):
		return hasGrant("governance", "governance/user-provisioning")

	case strings.HasPrefix(path, "/api/access-profiles"):
		return hasGrant("governance", "governance/access-profiles")

	case strings.HasPrefix(path, "/api/roles"),
		strings.HasPrefix(path, "/api/permissions"),
		strings.HasPrefix(path, "/api/operations"),
		strings.HasPrefix(path, "/api/resources"),
		strings.HasPrefix(path, "/api/rbac"):
		return hasGrant("governance", "governance/roles-permissions")

	case strings.HasPrefix(path, "/api/circuit-breaker"):
		return hasGrant("models", "models/circuit-breaker", "adaptive-routing", "adaptive-routing/dashboard", "adaptive-routing/settings")

	case strings.HasPrefix(path, "/api/governance/routing-rules"),
		strings.HasPrefix(path, "/api/governance/complexity-analyzer-config"):
		return hasGrant("models", "models/routing-rules", "models/complexity-router", "adaptive-routing", "adaptive-routing/dashboard", "adaptive-routing/settings")

	case strings.HasPrefix(path, "/api/governance/pricing-overrides"),
		strings.HasPrefix(path, "/api/governance/billing-blocks"),
		strings.HasPrefix(path, "/api/governance/model-configs"):
		return hasGrant("models", "models/pricing-overrides", "models/model-settings", "models/budgets-limits", "governance", "prompt-repository")

	case strings.HasPrefix(path, "/api/models"),
		strings.HasPrefix(path, "/api/providers"),
		strings.HasPrefix(path, "/api/keys"),
		strings.HasPrefix(path, "/api/governance/providers"):
		return hasGrant("models", "models/model-providers", "models/model-catalog", "models/budgets-limits", "models/routing-rules", "models/circuit-breaker", "models/complexity-router", "models/pricing-overrides", "models/model-settings", "prompt-repository", "governance", "governance/virtual-keys", "observability", "browser-ai")

	case path == "/api/plugins/semantic_cache":
		return hasGrant("settings/caching", "plugins", "observability", "observability/connectors")

	case strings.HasPrefix(path, "/api/vector-store-config"):
		return hasGrant("settings", "settings/caching")

	case strings.HasPrefix(path, "/api/governance"):
		return hasGrant("governance", "governance/virtual-keys", "governance/teams", "governance/business-units", "governance/customers", "governance/users", "governance/audit-logs", "governance/access-profiles", "governance/user-provisioning", "governance/roles-permissions")

	case strings.HasPrefix(path, "/api/config"),
		strings.HasPrefix(path, "/api/core/config"),
		strings.HasPrefix(path, "/api/pricing"),
		strings.HasPrefix(path, "/api/feature-flags"),
		strings.HasPrefix(path, "/api/proxy-config"),
		strings.HasPrefix(path, "/api/smtp-config"):
		return hasGrant("settings", "settings/client-settings", "settings/compatibility", "settings/caching", "settings/security", "settings/proxy", "settings/api-keys", "settings/performance-tuning", "settings/feature-flags", "observability/logs-settings", "models")

	case strings.HasPrefix(path, "/api/guardrails"),
		strings.HasPrefix(path, "/api/cluster"):
		return hasGrant("guardrails", "guardrails/rules", "guardrails/providers", "guardrails/cluster-config", "cluster-config")

	case strings.HasPrefix(path, "/api/load-balancer"):
		return hasGrant("adaptive-routing", "adaptive-routing/dashboard", "adaptive-routing/settings", "models")

	case strings.HasPrefix(path, "/api/mcp"):
		return hasGrant("mcp-gateway", "mcp-gateway/mcp-catalog", "mcp-gateway/mcp-library", "mcp-gateway/tool-groups", "mcp-gateway/auth-sessions", "mcp-gateway/oauth-grants", "mcp-gateway/mcp-settings", "prompt-repository")

	case strings.HasPrefix(path, "/api/plugins"):
		return hasGrant("plugins", "observability", "observability/connectors")

	case strings.HasPrefix(path, "/api/prompt-repo"), strings.HasPrefix(path, "/api/prompt-deployments"):
		return hasGrant("prompt-repository")

	case strings.HasPrefix(path, "/api/skills"):
		return hasGrant("skills-repository", "prompt-repository")
	}

	return false
}

// ensureUserRoleAllowedSections guarantees Prompt Repository is always present in
// allowed_sections for the built-in user role (create/update payloads).
func ensureUserRoleAllowedSections(role, sections string) string {
	if strings.ToLower(strings.TrimSpace(role)) != "user" {
		return sections
	}
	parts := strings.Split(sections, ",")
	for _, p := range parts {
		if strings.EqualFold(strings.TrimSpace(p), "prompt-repository") {
			return sections
		}
	}
	trimmed := strings.TrimSpace(sections)
	if trimmed == "" {
		return "prompt-repository"
	}
	return trimmed + ",prompt-repository"
}

// ensureUserPromptRepositoryAccess gives the built-in user role full Prompt Repository
// playground access (view + chat + edit own sessions/prompts). Other workspace sections
// stay view-only via applySectionPermissions unless the role matrix grants more.
func ensureUserPromptRepositoryAccess(perms rbac.PermissionSet) {
	if perms == nil {
		return
	}
	ops := perms["PromptRepository"]
	if ops == nil {
		ops = map[string]bool{}
		perms["PromptRepository"] = ops
	}
	for _, op := range []string{"View", "Read", "Create", "Update", "Delete", "Download"} {
		ops[op] = true
	}
}

// resourcesForGrantedSections maps Workspace Access keys to RBAC resource names.
func resourcesForGrantedSections(grantedSections string) []string {
	if strings.TrimSpace(grantedSections) == "" {
		return nil
	}
	grantList := strings.Split(strings.ToLower(grantedSections), ",")
	hasGrant := func(keys ...string) bool {
		for _, key := range keys {
			for _, g := range grantList {
				g = strings.TrimSpace(g)
				if g == key || g == "*" {
					return true
				}
				if parent, _, found := strings.Cut(key, "/"); found && g == parent {
					return true
				}
				if parent, _, found := strings.Cut(g, "/"); found && key == parent {
					return true
				}
			}
		}
		return false
	}
	seen := map[string]bool{}
	var out []string
	add := func(resources ...string) {
		for _, resource := range resources {
			if resource == "" || seen[resource] {
				continue
			}
			seen[resource] = true
			out = append(out, resource)
		}
	}

	// 1. Observability (5 features)
	if hasGrant("observability", "observability/dashboard") {
		add("Dashboard", "Metrics")
	}
	if hasGrant("observability", "observability/llm-logs") {
		add("Logs")
	}
	if hasGrant("observability", "observability/mcp-logs") {
		add("MCPLogs")
	}
	if hasGrant("observability", "observability/connectors") {
		add("Observability")
	}
	if hasGrant("observability", "observability/logs-settings") {
		add("LogsSettings", "Settings")
	}

	// 2. Browser AI (8 features)
	if hasGrant("browser-ai", "browser-ai/overview") {
		add("BrowserAIOverview", "Logs")
	}
	if hasGrant("browser-ai", "browser-ai/targets") {
		add("BrowserAITargets", "Logs")
	}
	if hasGrant("browser-ai", "browser-ai/rules") {
		add("BrowserAIRules", "Logs")
	}
	if hasGrant("browser-ai", "browser-ai/logs") {
		add("BrowserAILogs", "Logs")
	}
	if hasGrant("browser-ai", "browser-ai/search-logs") {
		add("BrowserAISearchLogs", "Logs")
	}
	if hasGrant("browser-ai", "browser-ai/setup") {
		add("BrowserAISetup", "Logs")
	}
	if hasGrant("browser-ai", "browser-ai/agents") {
		add("BrowserAIAgents", "Logs")
	}
	if hasGrant("browser-ai", "browser-ai/telemetry") {
		add("BrowserAIInsights", "Logs")
	}

	// 3. Models (8 features)
	if hasGrant("models", "models/model-providers") {
		add("ModelProvider")
	}
	if hasGrant("models", "models/model-catalog") {
		add("ModelCatalog", "ModelProvider")
	}
	if hasGrant("models", "models/budgets-limits") {
		add("BudgetsLimits", "Governance", "ModelProvider")
	}
	if hasGrant("models", "models/complexity-router") {
		add("ComplexityRouter", "RoutingRules", "AdaptiveRouter")
	}
	if hasGrant("models", "models/routing-rules") {
		add("RoutingRules", "AdaptiveRouter")
	}
	if hasGrant("models", "models/circuit-breaker") {
		add("CircuitBreaker")
	}
	if hasGrant("models", "models/pricing-overrides") {
		add("PricingOverrides", "ModelProvider")
	}
	if hasGrant("models", "models/model-settings") {
		add("ModelSettings", "ModelProvider", "Settings")
	}

	// 4. MCP Gateway (7 features)
	if hasGrant("mcp-gateway", "mcp-gateway/mcp-catalog") {
		add("MCPCatalog", "MCPGateway")
	}
	if hasGrant("mcp-gateway", "mcp-gateway/mcp-library") {
		add("MCPLibrary", "MCPGateway")
	}
	if hasGrant("mcp-gateway", "mcp-gateway/tool-groups") {
		add("MCPToolGroups", "MCPGateway")
	}
	if hasGrant("mcp-gateway", "mcp-gateway/auth-sessions") {
		add("MCPAuthSessions", "MCPGateway")
	}
	if hasGrant("mcp-gateway", "mcp-gateway/oauth-grants") {
		add("MCPOAuthGrants", "MCPGateway")
	}
	if hasGrant("mcp-gateway", "mcp-gateway/mcp-settings") {
		add("MCPSettings", "MCPGateway")
	}
	if hasGrant("plugins") {
		add("Plugins", "Observability")
	}

	// 5. Governance (9 features)
	if hasGrant("governance") {
		add("Governance", "VirtualKeys", "Users", "Teams", "BusinessUnits", "Customers", "UserProvisioning", "RBAC", "AccessProfiles", "AuditLogs")
	}
	if hasGrant("governance/virtual-keys") {
		add("VirtualKeys", "Governance")
	}
	if hasGrant("governance/users") {
		add("Users", "Governance")
	}
	if hasGrant("governance/teams") {
		add("Teams", "Governance")
	}
	if hasGrant("governance/business-units") {
		add("BusinessUnits", "Teams", "Governance")
	}
	if hasGrant("governance/customers") {
		add("Customers", "Governance")
	}
	if hasGrant("governance/user-provisioning") {
		add("UserProvisioning", "Governance")
	}
	if hasGrant("governance/roles-permissions") {
		add("RBAC", "Governance")
	}
	if hasGrant("governance/access-profiles") {
		add("AccessProfiles", "Governance")
	}
	if hasGrant("governance/audit-logs") {
		add("AuditLogs", "Governance")
	}

	// 6. Guardrails (3 features)
	if hasGrant("guardrails", "guardrails/rules") {
		add("GuardrailRules", "GuardrailsConfig")
	}
	if hasGrant("guardrails", "guardrails/providers") {
		add("GuardrailsProviders", "GuardrailsConfig")
	}
	if hasGrant("guardrails", "guardrails/cluster-config", "cluster-config") {
		add("Cluster")
	}

	// 7. Adaptive Routing (4 features)
	if hasGrant("adaptive-routing", "adaptive-routing/dashboard") {
		add("AdaptiveRoutingDashboard", "AdaptiveRouter")
	}
	if hasGrant("adaptive-routing", "adaptive-routing/settings") {
		add("AdaptiveRoutingSettings", "AdaptiveRouter")
	}
	if hasGrant("prompt-repository") {
		add("PromptRepository", "PromptDeploymentStrategy", "Inference")
	}
	if hasGrant("skills-repository") {
		add("SkillsRepository")
	}

	// 8. Settings (7 features)
	if hasGrant("settings") {
		add("Settings", "ClientSettings", "Compatibility", "Caching", "Security", "APIKeys", "PerformanceTuning", "FeatureFlags")
	}
	if hasGrant("settings/client-settings") {
		add("ClientSettings", "Settings")
	}
	if hasGrant("settings/compatibility") {
		add("Compatibility", "Settings")
	}
	if hasGrant("settings/caching") {
		add("Caching", "Settings")
	}
	if hasGrant("settings/security") {
		add("Security", "Settings")
	}
	if hasGrant("settings/api-keys") {
		add("APIKeys", "Settings")
	}
	if hasGrant("settings/proxy") {
		add("Settings")
	}
	if hasGrant("settings/performance-tuning") {
		add("PerformanceTuning", "Settings")
	}
	if hasGrant("settings/feature-flags") {
		add("FeatureFlags", "Settings")
	}

	return out
}

func grantResourceOps(perms rbac.PermissionSet, resources []string, ops ...string) {
	if perms == nil {
		return
	}
	for _, resource := range resources {
		if perms[resource] == nil {
			perms[resource] = map[string]bool{}
		}
		for _, op := range ops {
			perms[resource][op] = true
		}
	}
}

// applySectionPermissions grants View and Read only for Workspace Access sections
// (built-in "user" extras). Prompt Repository full access is handled separately.
func applySectionPermissions(perms rbac.PermissionSet, grantedSections string) {
	grantResourceOps(perms, resourcesForGrantedSections(grantedSections), "View", "Read")
}

// applySectionFullPermissions grants View/Read/Create/Update/Delete/Download for
// selected sections (sub_admin and customized admin Workspace Access).
func applySectionFullPermissions(perms rbac.PermissionSet, grantedSections string) {
	grantResourceOps(perms, resourcesForGrantedSections(grantedSections),
		"View", "Read", "Create", "Update", "Delete", "Download")
}

// parseRbacPermissionIDs parses a comma-separated list of RBAC catalog permission IDs.
func parseRbacPermissionIDs(raw string) []uint {
	var out []uint
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil || n == 0 {
			continue
		}
		out = append(out, uint(n))
	}
	return out
}

func formatRbacPermissionIDs(ids []uint) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ids))
	seen := map[uint]bool{}
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		parts = append(parts, strconv.FormatUint(uint64(id), 10))
	}
	return strings.Join(parts, ",")
}

// applyRbacPermissionIDs merges catalog permission IDs into the set (per-user RBAC raises).
func applyRbacPermissionIDs(perms rbac.PermissionSet, ids []uint) {
	if perms == nil || len(ids) == 0 {
		return
	}
	wanted := map[uint]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	for _, perm := range configstore.RBACPermissions() {
		if !wanted[perm.ID] {
			continue
		}
		if perms[perm.Resource] == nil {
			perms[perm.Resource] = map[string]bool{}
		}
		perms[perm.Resource][perm.Operation] = true
	}
}

// applyUserRolePermissions builds the effective matrix for built-in "user":
// role policy + View/Read for Workspace Access sections + Prompt Repo full +
// any per-user RBAC raises (Edit/Delete) saved on the user row.
func applyUserRolePermissions(perms rbac.PermissionSet, grantedSections, rbacPermissionIDs string) {
	if perms == nil {
		return
	}
	applySectionPermissions(perms, grantedSections)
	ensureUserPromptRepositoryAccess(perms)
	applyRbacPermissionIDs(perms, parseRbacPermissionIDs(rbacPermissionIDs))
}
