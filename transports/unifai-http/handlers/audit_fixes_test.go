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
