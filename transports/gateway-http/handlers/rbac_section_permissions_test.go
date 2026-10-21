package handlers

import (
	"strings"
	"testing"

	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/rbac"
)

func TestIsSectionGrantedForPath_UserWorkspaceAccess(t *testing.T) {
	cases := []struct {
		granted string
		path    string
		want    bool
	}{
		{"prompt-repository", "/api/prompt-repo/folders", true},
		{"prompt-repository", "/api/logs", false},
		{"observability", "/api/logs", true},
		{"observability/dashboard", "/api/logs/stats", true},
		{"observability/llm-logs", "/api/logs", true},
		{"governance/users", "/api/session/users", true},
		{"governance/users", "/api/session/users/quota", true},
		{"governance/teams", "/api/governance/teams", true},
		{"governance/business-units", "/api/governance/business-units", true},
		{"guardrails/rules", "/api/guardrails/rules", true},
		{"guardrails/cluster-config", "/api/cluster", true},
		{"cluster-config", "/api/cluster", true},
		{"adaptive-routing", "/api/load-balancer", true},
		{"models/pricing-overrides", "/api/governance/pricing-overrides", true},
		{"plugins", "/api/plugins", true},
		{"mcp-gateway", "/api/mcp/clients", true},
		{"skills-repository", "/api/skills", true},
		{"settings", "/api/feature-flags", true},
		{"", "/api/logs", false},
	}
	for _, c := range cases {
		if got := isSectionGrantedForPath(c.granted, c.path); got != c.want {
			t.Errorf("isSectionGrantedForPath(%q, %q) = %v, want %v", c.granted, c.path, got, c.want)
		}
	}
}

func TestApplySectionPermissions_GrantsViewForUserSections(t *testing.T) {
	perms := rbac.PermissionSet{}
	applySectionPermissions(perms, "observability,governance/users,guardrails,adaptive-routing,models")
	need := []struct{ resource, op string }{
		{"Logs", "View"},
		{"Dashboard", "View"},
		{"Users", "View"},
		{"GuardrailsConfig", "View"},
		{"AdaptiveRouter", "View"},
		{"ModelProvider", "View"},
	}
	for _, n := range need {
		if !rbac.HasPermission(perms, n.resource, n.op) {
			t.Errorf("missing %s/%s after applySectionPermissions", n.resource, n.op)
		}
	}
	if rbac.HasPermission(perms, "Logs", "Create") {
		t.Error("section grants must stay view/read only")
	}
}

func TestEnsureUserRoleAllowedSections(t *testing.T) {
	if got := ensureUserRoleAllowedSections("user", ""); got != "prompt-repository" {
		t.Errorf("empty user sections = %q", got)
	}
	if got := ensureUserRoleAllowedSections("user", "observability"); !strings.Contains(got, "prompt-repository") {
		t.Errorf("user extras must keep prompt-repository, got %q", got)
	}
	if got := ensureUserRoleAllowedSections("admin", ""); got != "" {
		t.Errorf("admin must stay unrestricted empty, got %q", got)
	}
	if got := ensureUserRoleAllowedSections("sub_admin", "governance"); got != "governance" {
		t.Errorf("sub_admin must not force prompt-repository, got %q", got)
	}
}

func TestEnsureUserPromptRepositoryAccess(t *testing.T) {
	perms := rbac.PermissionSet{}
	ensureUserPromptRepositoryAccess(perms)
	for _, op := range []string{"View", "Read", "Create", "Update", "Delete", "Download"} {
		if !rbac.HasPermission(perms, "PromptRepository", op) {
			t.Errorf("built-in user must have PromptRepository/%s", op)
		}
	}
}

func TestApplyUserRolePermissions_ViewPlusRaise(t *testing.T) {
	perms := rbac.PermissionSet{}
	// Find Logs Update id from catalog.
	var updateID uint
	for _, p := range configstore.RBACPermissions() {
		if p.Resource == "Logs" && p.Operation == "Update" {
			updateID = p.ID
			break
		}
	}
	if updateID == 0 {
		t.Fatal("Logs/Update permission missing from catalog")
	}
	applyUserRolePermissions(perms, "observability", formatRbacPermissionIDs([]uint{updateID}))
	if !rbac.HasPermission(perms, "Logs", "View") {
		t.Error("section grant must still provide View")
	}
	if !rbac.HasPermission(perms, "Logs", "Update") {
		t.Error("per-user raise must grant Update")
	}
	if !rbac.HasPermission(perms, "PromptRepository", "Create") {
		t.Error("Prompt Repository must stay full for built-in user")
	}
	if rbac.HasPermission(perms, "Users", "Update") {
		t.Error("unselected sections must not gain Update")
	}
}

func TestApplySectionFullPermissions_SubAdminSelectedSections(t *testing.T) {
	perms := rbac.PermissionSet{}
	applySectionFullPermissions(perms, "observability,governance/users")
	for _, n := range []struct{ resource, op string }{
		{"Logs", "Create"},
		{"Logs", "Download"},
		{"Users", "Update"},
		{"Users", "Delete"},
	} {
		if !rbac.HasPermission(perms, n.resource, n.op) {
			t.Errorf("missing %s/%s after applySectionFullPermissions", n.resource, n.op)
		}
	}
	if rbac.HasPermission(perms, "PromptRepository", "View") {
		t.Error("unselected sections must not receive full access")
	}
}
