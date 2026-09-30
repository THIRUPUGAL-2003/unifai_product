package handlers

import "testing"

func TestCustomRolePathDelegable(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		{"GET", "/api/cluster", true},
		{"GET", "/api/browser-ai/rules", true},
		{"POST", "/api/browser-ai/rules", true},
		{"PUT", "/api/load-balancer", true},
		{"GET", "/api/roles", true},
		{"POST", "/api/roles", false},
		{"PUT", "/api/roles/3/permissions", false},
		{"PUT", "/api/users/7/role", false},
		{"PUT", "/api/config", false},
		{"PUT", "/api/scim/config", false},
		{"POST", "/api/session/users", false},
		{"DELETE", "/api/session/users/1", false},
		{"GET", "/api/session/users", false},
	}
	for _, c := range cases {
		if got := customRolePathDelegable(c.method, c.path); got != c.want {
			t.Errorf("%s %s = %v, want %v", c.method, c.path, got, c.want)
		}
	}
	if customRoleMayReach(nil, "qa_viewer", "GET", "/api/cluster") {
		t.Error("without a workspace store RBAC cannot be enforced, so custom roles must stay blocked")
	}
	for _, role := range []string{"user", "User", " user ", ""} {
		if roleUsesRBACDelegation(role) {
			t.Errorf("role %q must stay locked to Prompt Repository, not reach workspace logs", role)
		}
	}
	for _, role := range []string{"qa_viewer", "Auditor", "security_team"} {
		if !roleUsesRBACDelegation(role) {
			t.Errorf("custom role %q should be enforced by its RBAC permissions", role)
		}
	}
}
