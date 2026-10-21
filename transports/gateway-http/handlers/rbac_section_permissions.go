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
		return hasGrant("browser-ai", "observability", "observability/llm-logs")

	case strings.HasPrefix(path, "/api/models"),
		strings.HasPrefix(path, "/api/providers"),
		strings.HasPrefix(path, "/api/keys"),
		strings.HasPrefix(path, "/api/governance/users"),
		strings.HasPrefix(path, "/api/governance/virtual-keys"),
		strings.HasPrefix(path, "/api/governance/providers"),
		strings.HasPrefix(path, "/api/mcp/clients"),
		strings.HasPrefix(path, "/api/skills"),
		strings.HasPrefix(path, "/api/governance/model-configs"),
		strings.HasPrefix(path, "/api/governance/pricing-overrides"),
		strings.HasPrefix(path, "/api/circuit-breaker"),
		strings.HasPrefix(path, "/api/governance/routing-rules"),
		strings.HasPrefix(path, "/api/governance/complexity-analyzer-config"),
		strings.HasPrefix(path, "/api/governance/billing-blocks"):
		return hasGrant("prompt-repository", "models", "models/model-providers", "models/model-catalog", "models/budgets-limits", "models/routing-rules", "models/circuit-breaker", "models/complexity-router", "models/pricing-overrides", "models/model-settings", "observability")

	case strings.HasPrefix(path, "/api/config"),
		strings.HasPrefix(path, "/api/core/config"),
		strings.HasPrefix(path, "/api/pricing"),
		strings.HasPrefix(path, "/api/feature-flags"),
		strings.HasPrefix(path, "/api/proxy-config"),
		strings.HasPrefix(path, "/api/smtp-config"):
		return hasGrant("settings", "settings/client-settings", "settings/compatibility", "settings/caching", "settings/security", "settings/proxy", "settings/api-keys", "settings/performance-tuning", "settings/feature-flags", "observability/logs-settings", "models")

	case strings.HasPrefix(path, "/api/session/users"),
		strings.HasPrefix(path, "/api/governance"),
		strings.HasPrefix(path, "/api/users"),
		strings.HasPrefix(path, "/api/teams"),
		strings.HasPrefix(path, "/api/customers"),
		strings.HasPrefix(path, "/api/business-units"),
		strings.HasPrefix(path, "/api/audit-logs"),
		strings.HasPrefix(path, "/api/scim"),
		strings.HasPrefix(path, "/api/access-profiles"),
		strings.HasPrefix(path, "/api/roles"),
		strings.HasPrefix(path, "/api/permissions"),
		strings.HasPrefix(path, "/api/operations"),
		strings.HasPrefix(path, "/api/resources"),
		strings.HasPrefix(path, "/api/rbac"):
		return hasGrant("governance", "governance/virtual-keys", "governance/teams", "governance/business-units", "governance/customers", "governance/users", "governance/audit-logs", "governance/access-profiles", "governance/user-provisioning", "governance/roles-permissions")

	case strings.HasPrefix(path, "/api/guardrails"),
		strings.HasPrefix(path, "/api/cluster"):
		return hasGrant("guardrails", "guardrails/rules", "guardrails/providers", "guardrails/cluster-config", "cluster-config")

	case strings.HasPrefix(path, "/api/load-balancer"):
		return hasGrant("adaptive-routing", "adaptive-routing/dashboard", "adaptive-routing/settings")

	case strings.HasPrefix(path, "/api/mcp"):
		return hasGrant("mcp-gateway", "mcp-gateway/mcp-catalog", "mcp-gateway/mcp-library", "mcp-gateway/tool-groups", "mcp-gateway/auth-sessions", "mcp-gateway/oauth-grants", "mcp-gateway/mcp-settings")

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

	if hasGrant("observability", "observability/llm-logs") {
		add("Logs")
	}
	if hasGrant("observability", "observability/dashboard") {
		add("Dashboard")
	}
	if hasGrant("observability", "observability/connectors") {
		add("Observability")
	}
	if hasGrant("observability", "observability/mcp-logs") {
		add("MCPLogs")
	}
	if hasGrant("browser-ai") {
		add("Logs")
	}
	if hasGrant("models", "models/model-providers", "models/model-catalog", "models/pricing-overrides", "models/model-settings") {
		add("ModelProvider")
	}
	if hasGrant("models", "models/budgets-limits") {
		add("Governance", "ModelProvider")
	}
	if hasGrant("models", "models/routing-rules", "models/complexity-router") {
		add("RoutingRules", "AdaptiveRouter")
	}
	if hasGrant("models", "models/circuit-breaker") {
		add("CircuitBreaker")
	}
	if hasGrant("settings", "settings/client-settings", "settings/compatibility", "settings/caching", "settings/security", "settings/proxy", "settings/api-keys", "settings/performance-tuning", "settings/feature-flags", "observability/logs-settings") {
		add("Settings", "FeatureFlags", "APIKeys")
	}
	if hasGrant("governance") {
		add("Governance", "VirtualKeys", "Teams", "Customers", "Users", "AuditLogs", "AccessProfiles", "UserProvisioning", "RBAC")
	}
	if hasGrant("governance/virtual-keys") {
		add("VirtualKeys", "Governance")
	}
	if hasGrant("governance/teams", "governance/business-units") {
		add("Teams", "Governance")
	}
	if hasGrant("governance/customers") {
		add("Customers", "Governance")
	}
	if hasGrant("governance/users") {
		add("Users", "Governance")
	}
	if hasGrant("governance/audit-logs") {
		add("AuditLogs", "Governance")
	}
	if hasGrant("governance/access-profiles") {
		add("AccessProfiles", "Governance")
	}
	if hasGrant("governance/user-provisioning") {
		add("UserProvisioning", "Governance")
	}
	if hasGrant("governance/roles-permissions") {
		add("RBAC", "Governance")
	}
	if hasGrant("guardrails", "guardrails/rules") {
		add("GuardrailsConfig", "GuardrailRules")
	}
	if hasGrant("guardrails", "guardrails/providers") {
		add("GuardrailsProviders")
	}
	if hasGrant("guardrails", "guardrails/cluster-config", "cluster-config") {
		add("Cluster")
	}
	if hasGrant("adaptive-routing", "adaptive-routing/dashboard", "adaptive-routing/settings") {
		add("AdaptiveRouter")
	}
	if hasGrant("prompt-repository") {
		add("PromptRepository")
	}
	if hasGrant("skills-repository") {
		add("SkillsRepository")
	}
	if hasGrant("mcp-gateway") {
		add("MCPGateway", "MCPToolGroups")
	}
	if hasGrant("plugins") {
		add("Plugins", "Observability")
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
