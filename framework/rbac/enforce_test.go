package rbac

import (
	"testing"
)

func TestPathRequirementFor(t *testing.T) {
	// 1. Self permission endpoint must be open for any authenticated user
	if req := PathRequirementFor("GET", "/api/rbac/me/permissions"); req != nil {
		t.Fatalf("Expected nil requirement for /api/rbac/me/permissions, got %+v", req)
	}

	// 2. Virtual Keys endpoints
	reqVKGet := PathRequirementFor("GET", "/api/governance/virtual-keys")
	if reqVKGet == nil || reqVKGet.Resource != "VirtualKeys" || reqVKGet.Operation != "View" {
		t.Fatalf("Unexpected requirement for GET virtual-keys: %+v", reqVKGet)
	}
	reqVKPost := PathRequirementFor("POST", "/api/governance/virtual-keys")
	if reqVKPost == nil || reqVKPost.Resource != "VirtualKeys" || reqVKPost.Operation != "Create" {
		t.Fatalf("Unexpected requirement for POST virtual-keys: %+v", reqVKPost)
	}

	// 3. Settings / Config endpoints
	reqConfigGet := PathRequirementFor("GET", "/api/config")
	if reqConfigGet == nil || reqConfigGet.Resource != "Settings" || reqConfigGet.Operation != "View" {
		t.Fatalf("Unexpected requirement for GET /api/config: %+v", reqConfigGet)
	}
	reqConfigPut := PathRequirementFor("PUT", "/api/config")
	if reqConfigPut == nil || reqConfigPut.Resource != "Settings" || reqConfigPut.Operation != "Update" {
		t.Fatalf("Unexpected requirement for PUT /api/config: %+v", reqConfigPut)
	}

	// 4. Logs endpoint
	reqLogsGet := PathRequirementFor("GET", "/api/logs")
	if reqLogsGet == nil || reqLogsGet.Resource != "Logs" || reqLogsGet.Operation != "Read" {
		t.Fatalf("Unexpected requirement for GET /api/logs: %+v", reqLogsGet)
	}

	// 4b. Browser AI rules import (must allow sub_admin Logs Update)
	reqImport := PathRequirementFor("POST", "/api/browser-ai/rules/import")
	if reqImport == nil || reqImport.Resource != "Logs" || reqImport.Operation != "Update" {
		t.Fatalf("Unexpected requirement for POST /api/browser-ai/rules/import: %+v", reqImport)
	}
	reqRulesGet := PathRequirementFor("GET", "/api/browser-ai/rules")
	if reqRulesGet == nil || reqRulesGet.Resource != "Logs" || reqRulesGet.Operation != "View" {
		t.Fatalf("Unexpected requirement for GET /api/browser-ai/rules: %+v", reqRulesGet)
	}

	// 5. Roles endpoint (Readable by Governance & RBAC)
	reqRolesGet := PathRequirementFor("GET", "/api/roles")
	if reqRolesGet == nil || reqRolesGet.Resource != "RBAC" || reqRolesGet.Operation != "View" {
		t.Fatalf("Unexpected requirement for GET /api/roles: %+v", reqRolesGet)
	}

	// 6. User role assignment (Requires RBAC:Update)
	reqUserRolePut := PathRequirementFor("PUT", "/api/users/123/role")
	if reqUserRolePut == nil || reqUserRolePut.Resource != "RBAC" || reqUserRolePut.Operation != "Update" {
		t.Fatalf("Unexpected requirement for PUT /api/users/123/role: %+v", reqUserRolePut)
	}
}

// Each sidebar item's own permission must reach the API that page loads,
// while the broader legacy permission keeps working for existing roles.
func TestSidebarResourcesReachTheirAPIs(t *testing.T) {
	cases := []struct {
		method, path, op string
		anyOf            []string
	}{
		{"GET", "/api/governance/teams", "View", []string{"Teams", "Governance"}},
		{"POST", "/api/governance/teams", "Create", []string{"Teams", "Governance"}},
		{"GET", "/api/governance/customers", "View", []string{"Customers", "Governance"}},
		{"DELETE", "/api/governance/customers/c1", "Delete", []string{"Customers", "Governance"}},
		{"GET", "/api/governance/routing-rules", "View", []string{"RoutingRules", "Governance"}},
		{"PUT", "/api/governance/complexity-analyzer-config", "Update", []string{"RoutingRules", "Governance"}},
		{"GET", "/api/governance/pricing-overrides", "View", []string{"Settings", "Governance"}},
		{"GET", "/api/mcp-logs", "Read", []string{"MCPLogs", "Logs"}},
		{"DELETE", "/api/mcp-logs", "Delete", []string{"MCPLogs", "Logs"}},
		{"GET", "/api/plugins", "View", []string{"Plugins", "Observability"}},
		{"PUT", "/api/plugins/otel", "Update", []string{"Plugins", "Observability"}},
		{"GET", "/api/feature-flags", "View", []string{"FeatureFlags", "Settings"}},
		{"PUT", "/api/feature-flags/f1", "Update", []string{"FeatureFlags", "Settings"}},
		{"GET", "/api/logs/stats", "Read", []string{"Logs", "Dashboard"}},
		{"GET", "/api/logs/histogram/tokens", "Read", []string{"Logs", "Dashboard"}},
		{"GET", "/api/logs/rankings", "Read", []string{"Logs", "Dashboard"}},
	}
	for _, c := range cases {
		req := PathRequirementFor(c.method, c.path)
		if req == nil || req.Operation != c.op || len(req.AnyOfResources) != len(c.anyOf) {
			t.Errorf("%s %s: got %+v, want %s on any of %v", c.method, c.path, req, c.op, c.anyOf)
			continue
		}
		for i, r := range c.anyOf {
			if req.AnyOfResources[i] != r {
				t.Errorf("%s %s: AnyOfResources = %v, want %v", c.method, c.path, req.AnyOfResources, c.anyOf)
				break
			}
		}
	}

	// Raw request rows and generic governance stay on their original resources.
	if req := PathRequirementFor("GET", "/api/logs"); req == nil || req.Resource != "Logs" || len(req.AnyOfResources) != 0 {
		t.Errorf("GET /api/logs must require Logs only, got %+v", req)
	}
	if req := PathRequirementFor("GET", "/api/governance/budgets"); req == nil || req.Resource != "Governance" || len(req.AnyOfResources) != 0 {
		t.Errorf("GET /api/governance/budgets must require Governance only, got %+v", req)
	}
}

func TestHasPermission(t *testing.T) {
	// 1. Admin allows all
	adminPerms := allowAll()
	if !HasPermission(adminPerms, "Settings", "Update") {
		t.Fatalf("Admin should have Settings:Update")
	}
	if !HasPermission(adminPerms, "VirtualKeys", "Create") {
		t.Fatalf("Admin should have VirtualKeys:Create")
	}

	// 2. Sub-Admin simulation
	subAdminPerms := PermissionSet{
		"VirtualKeys": {
			"Create": true,
			"Read":   true,
			"View":   true,
			"Update": true,
			"Delete": true,
		},
		"PromptRepository": {
			"Create": true,
			"Read":   true,
			"View":   true,
			"Update": true,
		},
		"Logs": {
			"Read":   true,
			"View":   true,
			"Create": true,
			"Update": true,
			"Delete": true,
		},
		"Governance": {
			"View":   true,
			"Read":   true,
			"Update": true,
		},
	}

	// Sub-Admin CAN do:
	if !HasPermission(subAdminPerms, "VirtualKeys", "Create") {
		t.Fatalf("Sub-Admin should be allowed to create VirtualKeys")
	}
	if !HasPermission(subAdminPerms, "PromptRepository", "Create") {
		t.Fatalf("Sub-Admin should be allowed to create Prompts")
	}
	if !HasPermission(subAdminPerms, "Logs", "Update") {
		t.Fatalf("Sub-Admin should be allowed to update Logs (Browser AI rules import)")
	}
	if !HasPermission(subAdminPerms, "Logs", "Read") {
		t.Fatalf("Sub-Admin should be allowed to read Logs")
	}
	if !HasPermission(subAdminPerms, "Governance", "Update") {
		t.Fatalf("Sub-Admin should be allowed to update Governance budgets")
	}

	// Sub-Admin CANNOT do (Security restrictions):
	if HasPermission(subAdminPerms, "Settings", "Update") {
		t.Fatalf("Sub-Admin should NOT be allowed to update Settings")
	}
	if HasPermission(subAdminPerms, "Settings", "View") {
		t.Fatalf("Sub-Admin should NOT be allowed to view Settings")
	}
	if HasPermission(subAdminPerms, "RBAC", "Update") {
		t.Fatalf("Sub-Admin should NOT be allowed to update RBAC roles")
	}
	if HasPermission(subAdminPerms, "Cluster", "Update") {
		t.Fatalf("Sub-Admin should NOT be allowed to update Cluster")
	}
}
