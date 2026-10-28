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

	// 3. Settings / Config endpoints: GET /api/config is available to all authenticated roles
	reqConfigGet := PathRequirementFor("GET", "/api/config")
	if reqConfigGet != nil {
		t.Fatalf("Expected nil requirement for GET /api/config, got: %+v", reqConfigGet)
	}
	reqProxyGet := PathRequirementFor("GET", "/api/proxy-config")
	if reqProxyGet == nil || reqProxyGet.Resource != "Proxy" || reqProxyGet.Operation != "View" {
		t.Fatalf("Unexpected requirement for GET /api/proxy-config: %+v", reqProxyGet)
	}
	reqConfigPut := PathRequirementFor("PUT", "/api/config")
	if reqConfigPut == nil || reqConfigPut.Resource != "Settings" || reqConfigPut.Operation != "Update" {
		t.Fatalf("Unexpected requirement for PUT /api/config: %+v", reqConfigPut)
	}
	if !reqConfigPut.Allowed(PermissionSet{"Compatibility": {"Update": true}}) {
		t.Fatal("Compatibility Update must authorize the shared core config endpoint")
	}
	if reqConfigPut.Allowed(PermissionSet{"Compatibility": {"View": true}}) {
		t.Fatal("Compatibility View must not authorize a core config update")
	}
	if !PathRequirementFor("PUT", "/api/proxy-config").Allowed(PermissionSet{"Proxy": {"Update": true}}) {
		t.Fatal("Proxy Update must authorize the proxy config endpoint")
	}
	if !PathRequirementFor("DELETE", "/api/cache/clear/cache-1").Allowed(PermissionSet{"Caching": {"Delete": true}}) {
		t.Fatal("Caching Delete must authorize clearing a cache")
	}
	if PathRequirementFor("DELETE", "/api/cache/clear/cache-1").Allowed(PermissionSet{"Caching": {"Update": true}}) {
		t.Fatal("Caching Update must not authorize deleting a cache")
	}
	if !PathRequirementFor("POST", "/api/browser-ai/rules").Allowed(PermissionSet{"BrowserAIRules": {"Create": true}}) {
		t.Fatal("Browser AI Rules Create must authorize creating a rule")
	}
	if PathRequirementFor("POST", "/api/browser-ai/rules").Allowed(PermissionSet{"BrowserAIRules": {"Update": true}}) {
		t.Fatal("Browser AI Rules Update must not authorize creating a rule")
	}
	if !PathRequirementFor("DELETE", "/api/browser-ai/targets/target-1").Allowed(PermissionSet{"BrowserAITargets": {"Delete": true}}) {
		t.Fatal("Browser AI Targets Delete must authorize deleting a target")
	}

	// 4. Logs endpoint
	reqLogsGet := PathRequirementFor("GET", "/api/logs")
	if reqLogsGet == nil || reqLogsGet.Resource != "Logs" || reqLogsGet.Operation != "Read" {
		t.Fatalf("Unexpected requirement for GET /api/logs: %+v", reqLogsGet)
	}

	// 4b. Browser AI rules use their dedicated resource and Create permission.
	reqImport := PathRequirementFor("POST", "/api/browser-ai/rules/import")
	if reqImport == nil || reqImport.Resource != "BrowserAIRules" || reqImport.Operation != "Create" {
		t.Fatalf("Unexpected requirement for POST /api/browser-ai/rules/import: %+v", reqImport)
	}
	reqRulesGet := PathRequirementFor("GET", "/api/browser-ai/rules")
	if reqRulesGet == nil || reqRulesGet.Resource != "BrowserAIRules" || reqRulesGet.Operation != "View" {
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
		{"PUT", "/api/governance/complexity-analyzer-config", "Update", []string{"ComplexityRouter", "RoutingRules", "AdaptiveRouter", "Governance"}},
		{"GET", "/api/governance/pricing-overrides", "View", []string{"PricingOverrides", "Governance"}},
		{"GET", "/api/mcp/clients", "View", []string{"MCPCatalog", "MCPGateway"}},
		{"POST", "/api/mcp/client", "Create", []string{"MCPCatalog", "MCPGateway"}},
		{"PUT", "/api/mcp/client/client-1", "Update", []string{"MCPCatalog", "MCPGateway"}},
		{"POST", "/api/mcp/client/client-1/reconnect", "Update", []string{"MCPCatalog", "MCPGateway"}},
		{"GET", "/api/mcp/library", "View", []string{"MCPLibrary", "MCPGateway"}},
		{"POST", "/api/mcp/library/force-sync", "Update", []string{"MCPLibrary", "MCPGateway"}},
		{"POST", "/api/mcp/library", "Create", []string{"MCPLibrary", "MCPGateway"}},
		{"DELETE", "/api/mcp/library/entry-1", "Delete", []string{"MCPLibrary", "MCPGateway"}},
		{"GET", "/api/mcp/sessions", "View", []string{"MCPAuthSessions", "MCPGateway"}},
		{"POST", "/api/mcp/sessions/session-1/reauth", "Update", []string{"MCPAuthSessions", "MCPGateway"}},
		{"DELETE", "/api/mcp/sessions/session-1", "Delete", []string{"MCPAuthSessions", "MCPGateway"}},
		{"GET", "/api/oauth/config/oauth-1/status", "View", []string{"MCPOAuthGrants", "MCPGateway"}},
		{"DELETE", "/api/oauth/config/oauth-1", "Delete", []string{"MCPOAuthGrants", "MCPGateway"}},
		{"GET", "/api/oauth2/sessions", "View", []string{"MCPOAuthGrants", "MCPGateway"}},
		{"GET", "/api/mcp-logs", "Read", []string{"MCPLogs", "Logs"}},
		{"DELETE", "/api/mcp-logs", "Delete", []string{"MCPLogs", "Logs"}},
		{"GET", "/api/plugins", "View", []string{"Plugins", "Observability"}},
		{"PUT", "/api/plugins/otel", "Update", []string{"Plugins", "Observability"}},
		{"GET", "/api/feature-flags", "View", []string{"FeatureFlags", "Settings"}},
		{"PUT", "/api/feature-flags/f1", "Update", []string{"FeatureFlags", "Settings"}},
		{"GET", "/api/logs/stats", "Read", []string{"Logs", "Dashboard"}},
		{"GET", "/api/logs/histogram/tokens", "Read", []string{"Logs", "Dashboard"}},
		{"GET", "/api/logs/rankings", "Read", []string{"Logs", "Dashboard"}},
		{"GET", "/api/logs/filterdata", "Read", []string{"Logs", "Dashboard"}},
		{"GET", "/api/mcp-logs/filterdata", "Read", []string{"MCPLogs", "Logs", "Dashboard"}},
		{"GET", "/api/mcp-logs/histogram/top-tools", "Read", []string{"MCPLogs", "Logs", "Dashboard"}},
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

func TestMCPFeaturePermissionsStayScoped(t *testing.T) {
	cases := []struct {
		name, method, path, resource, op string
		want                             bool
	}{
		{"catalog can view clients", "GET", "/api/mcp/clients", "MCPCatalog", "View", true},
		{"library cannot view clients", "GET", "/api/mcp/clients", "MCPLibrary", "View", false},
		{"library can create entry", "POST", "/api/mcp/library", "MCPLibrary", "Create", true},
		{"catalog cannot create library entry", "POST", "/api/mcp/library", "MCPCatalog", "Create", false},
		{"auth sessions can revoke session", "DELETE", "/api/mcp/sessions/s1", "MCPAuthSessions", "Delete", true},
		{"oauth grants cannot revoke auth session", "DELETE", "/api/mcp/sessions/s1", "MCPOAuthGrants", "Delete", false},
		{"oauth grants can revoke grant", "DELETE", "/api/oauth/config/o1", "MCPOAuthGrants", "Delete", true},
		{"catalog cannot revoke grant", "DELETE", "/api/oauth/config/o1", "MCPCatalog", "Delete", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := PathRequirementFor(c.method, c.path)
			got := req != nil && req.Allowed(PermissionSet{c.resource: {c.op: true}})
			if got != c.want {
				t.Fatalf("Allowed = %v, want %v (requirement: %+v)", got, c.want, req)
			}
		})
	}
}

func TestGuardrailsRulesAndProvidersNeedTheirOwnPermission(t *testing.T) {
	providersOnly := PermissionSet{"GuardrailsProviders": {"View": true, "Update": true}}
	rulesOnly := PermissionSet{"GuardrailsConfig": {"View": true, "Update": true}}
	both := PermissionSet{"GuardrailsProviders": {"Update": true}, "GuardrailsConfig": {"Update": true}}

	cases := []struct {
		name   string
		method string
		path   string
		perms  PermissionSet
		want   bool
	}{
		{"providers role saves providers", "PUT", "/api/guardrails/providers", providersOnly, true},
		{"providers role cannot save rules", "PUT", "/api/guardrails/rules", providersOnly, false},
		{"rules role saves rules", "PUT", "/api/guardrails/rules", rulesOnly, true},
		{"rules role cannot save providers", "PUT", "/api/guardrails/providers", rulesOnly, false},
		{"combined save needs both (providers only)", "PUT", "/api/guardrails/config", providersOnly, false},
		{"combined save needs both (rules only)", "PUT", "/api/guardrails/config", rulesOnly, false},
		{"combined save with both", "PUT", "/api/guardrails/config", both, true},
		{"either can read config", "GET", "/api/guardrails/config", providersOnly, true},
	}
	for _, c := range cases {
		if got := PathRequirementFor(c.method, c.path).Allowed(c.perms); got != c.want {
			t.Errorf("%s: Allowed = %v, want %v", c.name, got, c.want)
		}
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
