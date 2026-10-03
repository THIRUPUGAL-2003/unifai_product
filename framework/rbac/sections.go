package rbac

import "strings"

// SectionRequirementFor returns the sidebar section grants (any of) a section-scoped session
// needs for an API route, or nil when the route is not tied to a page. Keys use the UI's
// grant format: "parent" or "parent/child"; "parent/*" accepts the parent or any child.
//
// Shared lookups (lists used as dropdowns on other pages) are deliberately not mapped, so
// only the owning page's raw data and its writes are section-gated.
func SectionRequirementFor(method, path string) []string {
	method = strings.ToUpper(method)
	read := method == "GET" || method == "HEAD" || method == "OPTIONS"

	switch {
	case strings.HasPrefix(path, "/api/audit-logs"):
		return []string{"governance/audit-logs"}
	case strings.HasPrefix(path, "/api/scim"):
		return []string{"governance/user-provisioning"}
	case strings.HasPrefix(path, "/api/browser-ai"):
		return []string{"browser-ai/*"}
	case strings.HasPrefix(path, "/api/mcp-logs"):
		switch {
		case read && isDashboardMCPAggregatePath(path):
			return []string{"observability/mcp-logs", "observability/dashboard"}
		case read:
			return []string{"observability/mcp-logs"}
		default:
			return []string{"observability/mcp-logs", "observability/logs-settings"}
		}
	case strings.HasPrefix(path, "/api/logs"):
		switch {
		case read && isDashboardAggregatePath(path):
			return []string{"observability/llm-logs", "observability/dashboard", "models/model-catalog"}
		case read:
			return []string{"observability/llm-logs"}
		default:
			return []string{"observability/llm-logs", "observability/logs-settings"}
		}
	}

	if read {
		return nil
	}
	switch {
	case strings.HasPrefix(path, "/api/access-profiles"):
		return []string{"governance/access-profiles"}
	case strings.HasPrefix(path, "/api/roles"), strings.HasPrefix(path, "/api/permissions"), strings.HasPrefix(path, "/api/rbac"):
		return []string{"governance/roles-permissions"}
	case strings.HasPrefix(path, "/api/users/"), strings.HasPrefix(path, "/api/session/users"):
		return []string{"governance/users", "governance/roles-permissions"}
	case path == "/api/guardrails/rules":
		return []string{"guardrails/rules"}
	case path == "/api/guardrails/providers":
		return []string{"guardrails/providers"}
	case strings.HasPrefix(path, "/api/guardrails"):
		return []string{"guardrails/rules", "guardrails/providers"}
	case strings.HasPrefix(path, "/api/governance/virtual-keys/") && strings.HasSuffix(path, "/users"):
		return []string{"governance/virtual-keys", "governance/users"}
	case strings.HasPrefix(path, "/api/governance/virtual-keys"):
		return []string{"governance/virtual-keys"}
	case strings.HasPrefix(path, "/api/governance/teams/") && strings.Contains(path, "/members"):
		return []string{"governance/teams", "governance/users"}
	case strings.HasPrefix(path, "/api/governance/teams"):
		return []string{"governance/teams"}
	case strings.HasPrefix(path, "/api/governance/customers"):
		return []string{"governance/customers"}
	case strings.HasPrefix(path, "/api/governance/business-units"):
		return []string{"governance/business-units"}
	case strings.HasPrefix(path, "/api/governance/routing-rules"), strings.HasPrefix(path, "/api/governance/complexity-analyzer-config"):
		return []string{"models/routing-rules", "models/complexity-router"}
	case strings.HasPrefix(path, "/api/circuit-breaker"):
		return []string{"models/circuit-breaker"}
	case strings.HasPrefix(path, "/api/mcp/tool-groups"):
		return []string{"mcp-gateway/tool-groups"}
	}
	return nil
}

// SectionsAllow reports whether a comma-separated grant list satisfies any required key.
// A parent grant ("governance") covers all of its children.
func SectionsAllow(allowedSections string, required []string) bool {
	if len(required) == 0 {
		return true
	}
	grants := map[string]bool{}
	for _, part := range strings.Split(allowedSections, ",") {
		key := strings.ToLower(strings.TrimSpace(part))
		if key == "observability/browser-ai" {
			key = "browser-ai"
		}
		if key != "" {
			grants[key] = true
		}
	}
	for _, want := range required {
		if parent, ok := strings.CutSuffix(want, "/*"); ok {
			if grants[parent] {
				return true
			}
			for g := range grants {
				if strings.HasPrefix(g, parent+"/") {
					return true
				}
			}
			continue
		}
		if grants[want] {
			return true
		}
		if parent, _, found := strings.Cut(want, "/"); found && grants[parent] {
			return true
		}
		if want == "guardrails/cluster-config" && grants["cluster-config"] {
			return true
		}
	}
	return false
}
