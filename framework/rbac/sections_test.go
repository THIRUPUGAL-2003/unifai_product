package rbac

import "testing"

func TestSectionsAllowParentChildAndWildcard(t *testing.T) {
	cases := []struct {
		grants string
		req    []string
		want   bool
	}{
		{"observability", []string{"observability/llm-logs"}, true},
		{"observability/llm-logs", []string{"observability/llm-logs"}, true},
		{"observability/dashboard", []string{"observability/llm-logs"}, false},
		{"observability/dashboard", []string{"observability/llm-logs", "observability/dashboard"}, true},
		{"browser-ai/targets", []string{"browser-ai/*"}, true},
		{"observability/browser-ai", []string{"browser-ai/*"}, true},
		{"governance/teams", []string{"browser-ai/*"}, false},
		{"", []string{"governance/audit-logs"}, false},
		{"", nil, true},
	}
	for _, c := range cases {
		if got := SectionsAllow(c.grants, c.req); got != c.want {
			t.Errorf("SectionsAllow(%q, %v) = %v, want %v", c.grants, c.req, got, c.want)
		}
	}
}

func TestSectionRequirementFor(t *testing.T) {
	dashboardOnly := "observability/dashboard"
	llmLogsOnly := "observability/llm-logs"
	cases := []struct {
		name, method, path, grants string
		want                       bool
	}{
		{"dashboard reads charts", "GET", "/api/logs/stats", dashboardOnly, true},
		{"dashboard cannot read raw rows", "GET", "/api/logs", dashboardOnly, false},
		{"dashboard cannot open a log", "GET", "/api/logs/abc", dashboardOnly, false},
		{"llm logs reads raw rows", "GET", "/api/logs", llmLogsOnly, true},
		{"llm logs cannot read mcp rows", "GET", "/api/mcp-logs", llmLogsOnly, false},
		{"teams section cannot edit VKs", "PUT", "/api/governance/virtual-keys/vk1", "governance/teams", false},
		{"teams list stays readable as a dropdown", "GET", "/api/governance/teams", "governance/virtual-keys", true},
		{"users page assigns team members", "POST", "/api/governance/teams/t1/members", "governance/users", true},
		{"users page assigns VK users", "PUT", "/api/governance/virtual-keys/vk1/users", "governance/users", true},
		{"audit logs need their section", "GET", "/api/audit-logs", "governance/teams", false},
		{"governance parent covers audit logs", "GET", "/api/audit-logs", "governance", true},
		{"rule providers section saves providers", "PUT", "/api/guardrails/providers", "guardrails/providers", true},
		{"rule providers section cannot save rules", "PUT", "/api/guardrails/rules", "guardrails/providers", false},
		{"unmapped route is not section-gated", "GET", "/api/providers", "", true},
		{"connectors need connectors section", "GET", "/api/connectors", "observability/llm-logs", false},
		{"connectors section can list connectors", "GET", "/api/connectors", "observability/connectors", true},
		{"observability parent covers connectors", "PUT", "/api/connectors/datadog", "observability", true},
	}
	for _, c := range cases {
		if got := SectionsAllow(c.grants, SectionRequirementFor(c.method, c.path)); got != c.want {
			t.Errorf("%s: allowed = %v, want %v", c.name, got, c.want)
		}
	}
}
