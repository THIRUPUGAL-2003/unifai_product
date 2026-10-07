package rbac

import (
	"context"
	"strings"

	"github.com/gateway/gateway/framework/configstore"
)

// PermissionSet maps resource -> operation -> allowed.
type PermissionSet map[string]map[string]bool

// PathRequirement is the RBAC check for an HTTP route.
// When AnyOfResources is set, permission on any listed resource (same Operation) is enough.
type PathRequirement struct {
	Resource       string
	Operation      string
	AnyOfResources []string
	// AllOfResources requires the Operation on every listed resource (takes precedence over AnyOf).
	AllOfResources []string
}

// Allowed reports whether perms satisfy the requirement.
func (r *PathRequirement) Allowed(perms PermissionSet) bool {
	if r == nil {
		return true
	}
	if len(r.AllOfResources) > 0 {
		for _, resource := range r.AllOfResources {
			if !HasPermission(perms, resource, r.Operation) {
				return false
			}
		}
		return true
	}
	if len(r.AnyOfResources) > 0 {
		for _, resource := range r.AnyOfResources {
			if HasPermission(perms, resource, r.Operation) {
				return true
			}
		}
		return false
	}
	return HasPermission(perms, r.Resource, r.Operation)
}

// ResolvePermissions loads the permission set for a role name.
func ResolvePermissions(ctx context.Context, store configstore.WorkspaceStore, roleName string) (PermissionSet, error) {
	perms := make(PermissionSet)
	if store == nil {
		return perms, nil
	}
	if err := store.EnsureRBACRoles(ctx); err != nil {
		return nil, err
	}
	roleName = strings.ToLower(strings.TrimSpace(roleName))
	if roleName == "" || roleName == "admin" {
		return allowAll(), nil
	}
	rows, err := store.ListRBACRoles(ctx)
	if err != nil {
		return nil, err
	}
	var rolePerms []uint
	for _, row := range rows {
		if strings.EqualFold(row.Name, roleName) {
			rolePerms = row.ParsedPermissionIDs
			break
		}
	}
	if len(rolePerms) == 0 {
		return perms, nil
	}
	wanted := map[uint]bool{}
	for _, id := range rolePerms {
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
	return perms, nil
}

func allowAll() PermissionSet {
	out := make(PermissionSet)
	for _, resource := range configstore.RBACResourceNames {
		out[resource] = map[string]bool{}
		for _, op := range configstore.RBACOperationNames {
			out[resource][op] = true
		}
	}
	return out
}

// HasPermission reports whether the set allows a resource operation.
// An empty set means deny (unknown/unseeded role) — admin uses allowAll() via ResolvePermissions.
func HasPermission(set PermissionSet, resource, operation string) bool {
	if len(set) == 0 {
		return false
	}
	ops, ok := set[resource]
	if !ok {
		return false
	}
	if ops[operation] {
		return true
	}
	// View and Read are interchangeable for UI gates.
	if operation == "View" && ops["Read"] {
		return true
	}
	if operation == "Read" && ops["View"] {
		return true
	}
	return false
}

// PathRequirementFor maps dashboard API routes to RBAC requirements.
// Returns nil when the route is not RBAC-gated.
func PathRequirementFor(method, path string) *PathRequirement {
	method = strings.ToUpper(method)
	if method == "GET" || method == "HEAD" || method == "OPTIONS" {
		return readRequirement(path)
	}
	return writeRequirement(method, path)
}

func readRequirement(path string) *PathRequirement {
	// Allow any authenticated session to query their own assigned permissions
	if path == "/api/rbac/me/permissions" {
		return nil
	}
	// Core system configuration (is_db_connected, env, auth status, version) is required
	// by all dashboard shells and clients; write operations still require Settings Update.
	if path == "/api/config" {
		return nil
	}
	switch {
	case isDashboardAggregatePath(path):
		return &PathRequirement{Resource: "Logs", Operation: "Read", AnyOfResources: []string{"Logs", "Dashboard"}}
	case isDashboardMCPAggregatePath(path):
		return &PathRequirement{Resource: "MCPLogs", Operation: "Read", AnyOfResources: []string{"MCPLogs", "Logs", "Dashboard"}}
	case strings.HasPrefix(path, "/api/mcp-logs"):
		return &PathRequirement{Resource: "MCPLogs", Operation: "Read", AnyOfResources: []string{"MCPLogs", "Logs"}}
	case strings.HasPrefix(path, "/api/logs"):
		return &PathRequirement{Resource: "Logs", Operation: "Read"}
	case strings.HasPrefix(path, "/api/browser-ai/logs"):
		return &PathRequirement{Resource: "Logs", Operation: "Read"}
	case strings.HasPrefix(path, "/api/browser-ai"):
		return &PathRequirement{Resource: "Logs", Operation: "View"}
	case strings.HasPrefix(path, "/api/plugins"):
		return &PathRequirement{Resource: "Plugins", Operation: "View", AnyOfResources: []string{"Plugins", "Observability"}}
	case strings.HasPrefix(path, "/api/connectors"):
		return &PathRequirement{Resource: "Observability", Operation: "View"}
	case strings.HasPrefix(path, "/api/feature-flags"):
		return &PathRequirement{Resource: "FeatureFlags", Operation: "View", AnyOfResources: []string{"FeatureFlags", "Settings"}}
	case strings.HasPrefix(path, "/api/load-balancer"):
		return &PathRequirement{Resource: "AdaptiveRouter", Operation: "View"}
	case strings.HasPrefix(path, "/api/cluster"):
		return &PathRequirement{Resource: "Cluster", Operation: "View"}
	case strings.HasPrefix(path, "/api/scim"):
		return &PathRequirement{Resource: "UserProvisioning", Operation: "View"}
	case strings.HasPrefix(path, "/api/audit-logs"):
		return &PathRequirement{Resource: "AuditLogs", Operation: "View"}
	case strings.HasPrefix(path, "/api/access-profiles"):
		return &PathRequirement{Resource: "AccessProfiles", Operation: "View"}
	case strings.HasPrefix(path, "/api/roles"):
		return &PathRequirement{
			Resource:       "RBAC",
			Operation:      "View",
			AnyOfResources: []string{"RBAC", "Governance"},
		}
	case strings.HasPrefix(path, "/api/permissions"), strings.HasPrefix(path, "/api/resources"), strings.HasPrefix(path, "/api/operations"), strings.HasPrefix(path, "/api/rbac"):
		return &PathRequirement{Resource: "RBAC", Operation: "View"}
	case strings.HasPrefix(path, "/api/circuit-breaker"):
		return &PathRequirement{Resource: "CircuitBreaker", Operation: "View"}
	case strings.HasPrefix(path, "/api/mcp/tool-groups"):
		return &PathRequirement{Resource: "MCPToolGroups", Operation: "View"}
	case strings.HasPrefix(path, "/api/mcp"):
		return &PathRequirement{Resource: "MCPGateway", Operation: "View"}
	case strings.HasPrefix(path, "/api/governance/virtual-keys"):
		return &PathRequirement{
			Resource:       "VirtualKeys",
			Operation:      "View",
			AnyOfResources: []string{"VirtualKeys", "Governance"},
		}
	case strings.HasPrefix(path, "/api/governance"):
		return governanceRequirement(path, "View")
	case strings.HasPrefix(path, "/api/guardrails"):
		// Shared /api/guardrails/config blob; Rules UI uses GuardrailsConfig, Providers UI uses GuardrailsProviders.
		return &PathRequirement{
			Resource:       "GuardrailsConfig",
			Operation:      "View",
			AnyOfResources: []string{"GuardrailsConfig", "GuardrailsProviders"},
		}
	case strings.HasPrefix(path, "/api/prompt-repo"):
		return &PathRequirement{Resource: "PromptRepository", Operation: "View"}
	case strings.HasPrefix(path, "/api/prompt-deployments"):
		return &PathRequirement{Resource: "PromptDeploymentStrategy", Operation: "View"}
	case strings.HasPrefix(path, "/api/skills"):
		return &PathRequirement{Resource: "SkillsRepository", Operation: "View"}
	case strings.HasPrefix(path, "/api/providers"), strings.HasPrefix(path, "/api/keys"), strings.HasPrefix(path, "/api/models"):
		return &PathRequirement{Resource: "ModelProvider", Operation: "View"}
	case strings.HasPrefix(path, "/api/proxy-config"), strings.HasPrefix(path, "/api/vector-store-config"), strings.HasPrefix(path, "/api/smtp-config"):
		return &PathRequirement{Resource: "Settings", Operation: "View"}
	default:
		return nil
	}
}

// isDashboardAggregatePath covers the chart/stat endpoints the Dashboard page reads.
// Raw request rows (/api/logs, /api/logs/sessions/...) still require Logs.
func isDashboardAggregatePath(path string) bool {
	return path == "/api/logs/stats" ||
		path == "/api/logs/filterdata" ||
		path == "/api/logs/dashboard" ||
		strings.HasPrefix(path, "/api/logs/histogram") ||
		strings.HasPrefix(path, "/api/logs/rankings")
}

// isOwnPlaygroundSessionPath matches creating, saving, renaming and deleting a playground
// session: POST /api/prompt-repo/prompts/{id}/sessions and /api/prompt-repo/sessions/{id}[/rename].
func isOwnPlaygroundSessionPath(path string) bool {
	if rest, ok := strings.CutPrefix(path, "/api/prompt-repo/prompts/"); ok {
		id, tail, found := strings.Cut(rest, "/")
		return found && id != "" && tail == "sessions"
	}
	if rest, ok := strings.CutPrefix(path, "/api/prompt-repo/sessions/"); ok {
		id, tail, _ := strings.Cut(rest, "/")
		return id != "" && (tail == "" || tail == "rename")
	}
	return false
}

// isDashboardMCPAggregatePath covers the Dashboard MCP tab's charts and filter options.
// Raw MCP tool-call rows still require MCPLogs or Logs.
func isDashboardMCPAggregatePath(path string) bool {
	return path == "/api/mcp-logs/stats" ||
		path == "/api/mcp-logs/filterdata" ||
		strings.HasPrefix(path, "/api/mcp-logs/histogram")
}

// governanceRequirement lets the dedicated resource shown in the sidebar (Teams,
// Customers, RoutingRules, Settings) reach its governance sub-API, while roles
// holding the broader Governance permission keep working.
func governanceRequirement(path, op string) *PathRequirement {
	dedicated := ""
	switch {
	case strings.HasPrefix(path, "/api/governance/teams"):
		dedicated = "Teams"
	case strings.HasPrefix(path, "/api/governance/customers"):
		dedicated = "Customers"
	case strings.HasPrefix(path, "/api/governance/routing-rules"),
		strings.HasPrefix(path, "/api/governance/complexity-analyzer-config"):
		dedicated = "RoutingRules"
	case strings.HasPrefix(path, "/api/governance/pricing-overrides"):
		dedicated = "Settings"
	}
	if dedicated == "" {
		return &PathRequirement{Resource: "Governance", Operation: op}
	}
	return &PathRequirement{Resource: dedicated, Operation: op, AnyOfResources: []string{dedicated, "Governance"}}
}

// isBrowserAILogDelete matches prompt-log / search-log clears, single deletes and bulk deletes.
func isBrowserAILogDelete(method, path string) bool {
	for _, base := range []string{"/api/browser-ai/logs", "/api/browser-ai/search-logs"} {
		if method == "DELETE" && (path == base || strings.HasPrefix(path, base+"/")) {
			return true
		}
		if method == "POST" && path == base+"/bulk-delete" {
			return true
		}
	}
	return false
}

func writeRequirement(method, path string) *PathRequirement {
	op := "Update"
	switch method {
	case "POST":
		op = "Create"
	case "DELETE":
		op = "Delete"
	}
	switch {
	case strings.HasPrefix(path, "/api/mcp-logs"):
		return &PathRequirement{Resource: "MCPLogs", Operation: "Delete", AnyOfResources: []string{"MCPLogs", "Logs"}}
	case strings.HasPrefix(path, "/api/logs"):
		return &PathRequirement{Resource: "Logs", Operation: "Delete"}
	case isBrowserAILogDelete(method, path):
		return &PathRequirement{Resource: "Logs", Operation: "Delete"}
	case strings.HasPrefix(path, "/api/browser-ai"):
		return &PathRequirement{Resource: "Logs", Operation: "Update"}
	case strings.HasPrefix(path, "/api/plugins"):
		return &PathRequirement{Resource: "Plugins", Operation: "Update", AnyOfResources: []string{"Plugins", "Observability"}}
	case strings.HasPrefix(path, "/api/connectors"):
		return &PathRequirement{Resource: "Observability", Operation: "Update"}
	case strings.HasPrefix(path, "/api/feature-flags"):
		return &PathRequirement{Resource: "FeatureFlags", Operation: "Update", AnyOfResources: []string{"FeatureFlags", "Settings"}}
	case strings.HasPrefix(path, "/api/load-balancer"):
		return &PathRequirement{Resource: "AdaptiveRouter", Operation: "Update"}
	case strings.HasPrefix(path, "/api/cluster"):
		return &PathRequirement{Resource: "Cluster", Operation: "Update"}
	case strings.HasPrefix(path, "/api/scim"):
		return &PathRequirement{Resource: "UserProvisioning", Operation: "Update"}
	case strings.HasPrefix(path, "/api/audit-logs"):
		return &PathRequirement{Resource: "AuditLogs", Operation: "Update"}
	case strings.HasPrefix(path, "/api/access-profiles"):
		return &PathRequirement{Resource: "AccessProfiles", Operation: op}
	case strings.HasPrefix(path, "/api/roles"), strings.HasPrefix(path, "/api/permissions"), strings.HasPrefix(path, "/api/rbac"), strings.HasPrefix(path, "/api/users"):
		return &PathRequirement{Resource: "RBAC", Operation: op}
	case strings.HasPrefix(path, "/api/circuit-breaker"):
		return &PathRequirement{Resource: "CircuitBreaker", Operation: op}
	case strings.HasPrefix(path, "/api/mcp/tool-groups"):
		return &PathRequirement{Resource: "MCPToolGroups", Operation: op}
	case strings.HasPrefix(path, "/api/mcp"):
		return &PathRequirement{Resource: "MCPGateway", Operation: op}
	case strings.HasPrefix(path, "/api/governance/virtual-keys"):
		return &PathRequirement{
			Resource:       "VirtualKeys",
			Operation:      op,
			AnyOfResources: []string{"VirtualKeys", "Governance"},
		}
	case strings.HasPrefix(path, "/api/governance"):
		return governanceRequirement(path, op)
	case path == "/api/guardrails/rules":
		return &PathRequirement{Resource: "GuardrailsConfig", Operation: op}
	case path == "/api/guardrails/providers":
		return &PathRequirement{Resource: "GuardrailsProviders", Operation: op}
	case strings.HasPrefix(path, "/api/guardrails"):
		// The combined config blob carries both rules and providers.
		return &PathRequirement{
			Resource:       "GuardrailsConfig",
			Operation:      op,
			AllOfResources: []string{"GuardrailsConfig", "GuardrailsProviders"},
		}
	case isOwnPlaygroundSessionPath(path):
		// Playground chats belong to the caller (handlers enforce ownership), so members with
		// view access can keep their own history. Committing a session still needs write access.
		return &PathRequirement{Resource: "PromptRepository", Operation: "View"}
	case strings.HasPrefix(path, "/api/prompt-repo"):
		return &PathRequirement{Resource: "PromptRepository", Operation: op}
	case strings.HasPrefix(path, "/api/prompt-deployments"):
		return &PathRequirement{Resource: "PromptDeploymentStrategy", Operation: op}
	case strings.HasPrefix(path, "/api/skills"):
		return &PathRequirement{Resource: "SkillsRepository", Operation: op}
	case strings.HasPrefix(path, "/api/providers"), strings.HasPrefix(path, "/api/keys"):
		return &PathRequirement{Resource: "ModelProvider", Operation: op}
	case strings.HasPrefix(path, "/api/config"), strings.HasPrefix(path, "/api/proxy-config"), strings.HasPrefix(path, "/api/vector-store-config"), strings.HasPrefix(path, "/api/smtp-config"):
		return &PathRequirement{Resource: "Settings", Operation: "Update"}
	default:
		return nil
	}
}
