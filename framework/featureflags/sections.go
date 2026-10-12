package featureflags

// Section flags gate workspace menus and the APIs behind them. They start
// enabled so a fresh install shows every section. Turning one off in
// Settings → Feature Flags hides that menu and rejects its /api calls
// until the flag is turned back on.
func init() {
	defs := []FlagDef{
		{ID: "section.observability", DisplayName: "Observability", Description: "Dashboard, LLM logs, MCP logs, connectors, and log settings.", Default: true},
		{ID: "section.browser-ai", DisplayName: "Browser AI", Description: "Guard overview, rules, prompt logs, agents, and setup.", Default: true},
		{ID: "section.models", DisplayName: "Models", Description: "Providers, catalog, budgets, routing rules, circuit breaker, and pricing.", Default: true},
		{ID: "section.mcp-gateway", DisplayName: "MCP Gateway", Description: "MCP catalog, library, tool groups, auth sessions, and OAuth grants.", Default: true},
		{ID: "section.plugins", DisplayName: "Plugins", Description: "Plugin list and plugin order.", Default: true},
		{ID: "section.governance", DisplayName: "Governance", Description: "Virtual keys, users, teams, customers, provisioning, and roles.", Default: true},
		{ID: "section.guardrails", DisplayName: "Guardrails", Description: "Guardrail rules, rule providers, and cluster config.", Default: true},
		{ID: "section.adaptive-routing", DisplayName: "Adaptive Routing", Description: "Adaptive routing dashboard and settings.", Default: true},
		{ID: "section.prompt-repository", DisplayName: "Prompt Repository", Description: "Saved prompts and folders.", Default: true},
		{ID: "section.skills-repository", DisplayName: "Skills Repository", Description: "Skill files stored for this workspace.", Default: true},
		{ID: "section.cluster", DisplayName: "Cluster Config", Description: "Cluster nodes, load balancer routing, and high availability.", Default: true},
		{ID: "section.scim", DisplayName: "User Provisioning (SCIM)", Description: "SCIM directory integrations, Active Directory sync, and automated user provisioning.", Default: true},
		{ID: "section.audit-logs", DisplayName: "Audit Logs", Description: "Governance audit trail for admin actions, virtual keys, and system mutations.", Default: true},
		{ID: "section.access-profiles", DisplayName: "Access Profiles", Description: "Access profile policies and multi-entity permission grants.", Default: true},
		{ID: "section.business-units", DisplayName: "Business Units", Description: "Business unit organizational grouping and team management.", Default: true},
	}
	for _, def := range defs {
		MustRegister(def)
	}
}
