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
		return []string{browserAISectionForPath(path)}
	case strings.HasPrefix(path, "/api/models/catalog"):
		return []string{"models/model-catalog"}
	case strings.HasPrefix(path, "/api/load-balancer"):
		if strings.HasPrefix(path, "/api/load-balancer/routes") {
			return []string{"adaptive-routing/dashboard"}
		}
		return []string{"adaptive-routing/settings"}
	case strings.HasPrefix(path, "/api/mcp"), strings.HasPrefix(path, "/api/oauth/config"), strings.HasPrefix(path, "/api/oauth2/sessions"):
		if section := mcpSectionForPath(path); section != "" {
			return []string{section}
		}
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
	case strings.HasPrefix(path, "/api/connectors"):
		return []string{"observability/connectors"}
	case strings.HasPrefix(path, "/api/skills"):
		if read {
			return []string{"skills-repository", "prompt-repository"}
		}
		return []string{"skills-repository"}
	case strings.HasPrefix(path, "/api/prompt-repo"), strings.HasPrefix(path, "/api/prompt-deployments"):
		return []string{"prompt-repository"}
	}

	if read {
		return nil
	}
	switch {
	case path == "/api/config" || path == "/api/config/metadata":
		if path == "/api/config/metadata" {
			return []string{"settings/client-settings"}
		}
		return []string{
			"settings/client-settings", "settings/compatibility", "settings/security", "settings/performance-tuning",
			"observability/logs-settings", "mcp-gateway/mcp-settings", "models/model-settings", "models/pricing-overrides",
		}
	case strings.HasPrefix(path, "/api/vector-store-config") || path == "/api/plugins/semantic_cache":
		return []string{"settings/caching"}
	case strings.HasPrefix(path, "/api/proxy-config"):
		return []string{"settings/proxy"}
	case strings.HasPrefix(path, "/api/smtp-config"):
		return []string{"settings/security"}
	case strings.HasPrefix(path, "/api/feature-flags"):
		return []string{"settings/feature-flags"}
	case strings.HasPrefix(path, "/api/pricing"):
		return []string{"models/pricing-overrides"}
	case strings.HasPrefix(path, "/api/cache/clear"):
		return []string{"settings/caching"}
	case strings.HasPrefix(path, "/api/cluster"):
		return []string{"guardrails/cluster-config"}
	case strings.HasPrefix(path, "/api/access-profiles"):
		return []string{"governance/access-profiles"}
	case strings.HasPrefix(path, "/api/roles"), strings.HasPrefix(path, "/api/permissions"), strings.HasPrefix(path, "/api/rbac"):
		return []string{"governance/roles-permissions"}
	case path == "/api/users" || strings.HasPrefix(path, "/api/users/"), strings.HasPrefix(path, "/api/session/users"):
		return []string{"governance/users", "governance/roles-permissions"}
	case path == "/api/guardrails/rules":
		return []string{"guardrails/rules"}
	case path == "/api/guardrails/providers":
		return []string{"guardrails/providers"}
	case strings.HasPrefix(path, "/api/guardrails"):
		return []string{"guardrails/rules", "guardrails/providers"}
	case strings.HasPrefix(path, "/api/governance/virtual-keys/") && strings.HasSuffix(path, "/users"):
		return []string{"governance/virtual-keys", "governance/users", "settings/api-keys"}
	case strings.HasPrefix(path, "/api/governance/virtual-keys"):
		return []string{"governance/virtual-keys", "settings/api-keys"}
	case strings.HasPrefix(path, "/api/governance/teams/") && strings.Contains(path, "/members"):
		return []string{"governance/teams", "governance/users"}
	case strings.HasPrefix(path, "/api/governance/teams"):
		return []string{"governance/teams"}
	case strings.HasPrefix(path, "/api/governance/customers"):
		return []string{"governance/customers"}
	case strings.HasPrefix(path, "/api/governance/business-units"):
		return []string{"governance/business-units"}
	case strings.HasPrefix(path, "/api/governance/routing-rules"):
		return []string{"models/routing-rules"}
	case strings.HasPrefix(path, "/api/governance/complexity-analyzer-config"):
		return []string{"models/complexity-router"}
	case strings.HasPrefix(path, "/api/governance/pricing-overrides"):
		return []string{"models/pricing-overrides"}
	case strings.HasPrefix(path, "/api/governance/billing-blocks"):
		return []string{"models/budgets-limits"}
	case strings.HasPrefix(path, "/api/governance/model-configs"):
		return []string{"models/model-settings"}
	case strings.HasPrefix(path, "/api/circuit-breaker"):
		return []string{"models/circuit-breaker"}
	case strings.HasPrefix(path, "/api/mcp/tool-groups"):
		return []string{"mcp-gateway/tool-groups"}
	}
	return nil
}

func browserAISectionForPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/api/browser-ai/search-logs"):
		return "browser-ai/search-logs"
	case strings.HasPrefix(path, "/api/browser-ai/logs"):
		return "browser-ai/logs"
	case strings.HasPrefix(path, "/api/browser-ai/rules"):
		return "browser-ai/rules"
	case strings.HasPrefix(path, "/api/browser-ai/targets"):
		return "browser-ai/targets"
	case strings.HasPrefix(path, "/api/browser-ai/agents"):
		return "browser-ai/agents"
	case strings.HasPrefix(path, "/api/browser-ai/setup"), strings.HasPrefix(path, "/api/browser-ai/license"),
		path == "/api/browser-ai/controls", path == "/api/browser-ai/fleet-config":
		return "browser-ai/setup"
	case strings.HasPrefix(path, "/api/browser-ai/insights"):
		return "browser-ai/telemetry"
	default:
		return "browser-ai/overview"
	}
}

func mcpSectionForPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/api/mcp/tool-groups"):
		return "mcp-gateway/tool-groups"
	case strings.HasPrefix(path, "/api/mcp/library"):
		return "mcp-gateway/mcp-library"
	case strings.HasPrefix(path, "/api/mcp/sessions"), strings.HasPrefix(path, "/api/mcp/per-user-headers"):
		return "mcp-gateway/auth-sessions"
	case strings.HasPrefix(path, "/api/mcp/clients"), strings.HasPrefix(path, "/api/mcp/client"):
		return "mcp-gateway/mcp-catalog"
	case strings.HasPrefix(path, "/api/oauth/config"), strings.HasPrefix(path, "/api/oauth2/sessions"):
		return "mcp-gateway/oauth-grants"
	case strings.HasPrefix(path, "/api/mcp"):
		return "mcp-gateway"
	default:
		return ""
	}
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
		if key == "skills" {
			key = "skills-repository"
		}
		if key == "prompt-repo" {
			key = "prompt-repository"
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
