package handlers

import (
	"reflect"
	"testing"
)

func TestRenameClientInToolList(t *testing.T) {
	tools := []any{
		"github-search",
		"github-*",
		"gitlab-search",
		map[string]any{"name": "github-issues"},
		map[string]any{"mcp_client_name": "github", "tool_names": []any{"a", "b"}},
		map[string]any{"mcp_client_name": "githubx", "tool_names": []any{"c"}},
	}
	out, changed := renameClientInToolList(tools, "github", "gh")
	if !changed {
		t.Fatalf("expected change")
	}
	list := out.([]any)
	if list[0] != "gh-search" || list[1] != "gh-*" || list[2] != "gitlab-search" {
		t.Fatalf("string entries not renamed correctly: %#v", list[:3])
	}
	if list[3].(map[string]any)["name"] != "gh-issues" {
		t.Fatalf("flat map entry not renamed: %#v", list[3])
	}
	if list[4].(map[string]any)["mcp_client_name"] != "gh" {
		t.Fatalf("bundle entry not renamed: %#v", list[4])
	}
	if list[5].(map[string]any)["mcp_client_name"] != "githubx" {
		t.Fatalf("unrelated client renamed: %#v", list[5])
	}

	if _, changed := renameClientInToolList([]any{"other-tool"}, "github", "gh"); changed {
		t.Fatalf("unexpected change for unrelated entries")
	}
}

func TestConnectorSecretsRedactAndRestore(t *testing.T) {
	stored := map[string]any{"config": map[string]any{"api_key": "sk-live", "site": "datadoghq.com"}}
	view := map[string]any{"config": map[string]any{"api_key": "sk-live", "site": "datadoghq.com"}}
	redactConnectorPayload(view)
	cfg := view["config"].(map[string]any)
	if cfg["api_key"] != connectorSecretPlaceholder || cfg["site"] != "datadoghq.com" {
		t.Fatalf("unexpected redaction: %#v", cfg)
	}

	incoming := map[string]any{"config": map[string]any{"api_key": connectorSecretPlaceholder, "site": "us5.datadoghq.com"}}
	restoreConnectorSecrets(incoming, stored)
	want := map[string]any{"api_key": "sk-live", "site": "us5.datadoghq.com"}
	if !reflect.DeepEqual(incoming["config"], want) {
		t.Fatalf("restore mismatch: got %#v want %#v", incoming["config"], want)
	}
}

func TestNormalizeClusterPeer(t *testing.T) {
	valid := map[string]string{
		"10.0.0.12:8080":              "10.0.0.12:8080",
		" node-2:8080 ":               "node-2:8080",
		"http://node-2:8080/":         "http://node-2:8080",
		"https://node-3.internal":     "https://node-3.internal",
		"https://node-3.internal:443": "https://node-3.internal:443",
	}
	for in, want := range valid {
		got, err := normalizeClusterPeer(in)
		if err != nil || got != want {
			t.Errorf("normalizeClusterPeer(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"node-2", "ftp://node-2:21", "http://node-2:8080/internal", "", ":8080"} {
		if _, err := normalizeClusterPeer(in); err == nil {
			t.Errorf("normalizeClusterPeer(%q) should fail", in)
		}
	}
}

func TestClientConfigFieldPresent(t *testing.T) {
	if !clientConfigFieldPresent([]byte(`{"client_config":{"async_job_result_ttl":0}}`), "async_job_result_ttl") {
		t.Fatal("explicit 0 must count as present")
	}
	if clientConfigFieldPresent([]byte(`{"client_config":{"drop_excess_requests":true}}`), "async_job_result_ttl") {
		t.Fatal("missing field must not count as present")
	}
	if clientConfigFieldPresent([]byte(`not json`), "async_job_result_ttl") {
		t.Fatal("invalid body must not count as present")
	}
}

func TestAuditMiddlewareTargetAndAction(t *testing.T) {
	// Test Actions
	if act := auditActionForMethod("POST", "/api/roles"); act != "create" {
		t.Errorf("Expected 'create', got %s", act)
	}
	if act := auditActionForMethod("PUT", "/api/roles/1/permissions"); act != "update" {
		t.Errorf("Expected 'update', got %s", act)
	}
	if act := auditActionForMethod("DELETE", "/api/roles/1"); act != "delete" {
		t.Errorf("Expected 'delete', got %s", act)
	}
	if act := auditActionForMethod("POST", "/api/access-profiles/1/activate"); act != "update" {
		t.Errorf("Expected 'update' for activate, got %s", act)
	}
	if act := auditActionForMethod("POST", "/api/access-profiles/1/deactivate"); act != "update" {
		t.Errorf("Expected 'update' for deactivate, got %s", act)
	}
	if act := auditActionForMethod("POST", "/api/access-profiles/1/clone"); act != "create" {
		t.Errorf("Expected 'create' for clone, got %s", act)
	}

	// Test Targets
	if trg := auditTargetForRequest("/api/roles", "POST", map[string]any{"name": "Developer"}); trg != "Role: Developer" {
		t.Errorf("Expected 'Role: Developer', got %s", trg)
	}
	if trg := auditTargetForRequest("/api/roles/5/permissions", "PUT", nil); trg != "Role: #5 (Permissions)" {
		t.Errorf("Expected 'Role: #5 (Permissions)', got %s", trg)
	}
	if trg := auditTargetForRequest("/api/access-profiles", "POST", map[string]any{"name": "Tier-1"}); trg != "Access Profile: Tier-1" {
		t.Errorf("Expected 'Access Profile: Tier-1', got %s", trg)
	}
	if trg := auditTargetForRequest("/api/access-profiles/8/activate", "POST", nil); trg != "Access Profile: #8 (activate)" {
		t.Errorf("Expected 'Access Profile: #8 (activate)', got %s", trg)
	}
	if trg := auditTargetForRequest("/api/users/usr-42/role", "PUT", nil); trg != "User: usr-42 (Role)" {
		t.Errorf("Expected 'User: usr-42 (Role)', got %s", trg)
	}
	if trg := auditTargetForRequest("/api/rbac/scope-grants", "PUT", map[string]any{"scope_type": "all_teams"}); trg != "RBAC Scope: all_teams" {
		t.Errorf("Expected 'RBAC Scope: all_teams', got %s", trg)
	}
	if trg := auditTargetForRequest("/api/feature-flags/routing.circuit_breaker", "PUT", nil); trg != "Feature Flag: routing.circuit_breaker" {
		t.Errorf("Expected 'Feature Flag: routing.circuit_breaker', got %s", trg)
	}
	if trg := auditTargetForRequest("/api/config", "PUT", nil); trg != "Configuration" {
		t.Errorf("Expected 'Configuration', got %s", trg)
	}
	if trg := auditTargetForRequest("/api/vector-store-config", "PUT", nil); trg != "Vector Store Config" {
		t.Errorf("Expected 'Vector Store Config', got %s", trg)
	}
}
