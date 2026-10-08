package featureflags

// Section flags gate the workspace menus that already exist. They start
// enabled so a fresh install shows every section. Turning one off in
// Settings → Feature Flags hides that menu and its pages.
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
	}
	for _, def := range defs {
		MustRegister(def)
	}
}
