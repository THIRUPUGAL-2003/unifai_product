package handlers

import (
	"strings"

	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/framework/featureflags"
	"github.com/valyala/fasthttp"
)

// sectionAPIRule ties an /api prefix to the feature flag that owns that product area.
// Settings, session, and the flag API stay reachable so an admin can turn a section back on.
type sectionAPIRule struct {
	prefix string
	flagID string
}

var sectionAPIRules = []sectionAPIRule{
	{prefix: "/api/browser-ai", flagID: "section.browser-ai"},
	{prefix: "/api/logs", flagID: "section.observability"},
	{prefix: "/api/mcp-logs", flagID: "section.observability"},
	{prefix: "/api/governance", flagID: "section.governance"},
	{prefix: "/api/plugins", flagID: "section.plugins"},
	{prefix: "/api/guardrails", flagID: "section.guardrails"},
	{prefix: "/api/cluster", flagID: "section.guardrails"},
	{prefix: "/api/providers", flagID: "section.models"},
	{prefix: "/api/circuit-breaker", flagID: "section.models"},
	{prefix: "/api/mcp", flagID: "section.mcp-gateway"},
	{prefix: "/api/prompt-repo", flagID: "section.prompt-repository"},
	{prefix: "/api/prompt-deployments", flagID: "section.prompt-repository"},
	{prefix: "/api/skills", flagID: "section.skills-repository"},
	{prefix: "/api/load-balancer", flagID: "section.adaptive-routing"},
}

// SectionFlagForPath returns the section flag that gates this API path.
func SectionFlagForPath(path string) (string, bool) {
	path = strings.SplitN(path, "?", 2)[0]
	var flagID string
	best := 0
	for _, rule := range sectionAPIRules {
		if path != rule.prefix && !strings.HasPrefix(path, rule.prefix+"/") {
			continue
		}
		if len(rule.prefix) > best {
			flagID = rule.flagID
			best = len(rule.prefix)
		}
	}
	if flagID == "" {
		return "", false
	}
	return flagID, true
}

// SectionFlagMiddleware rejects API calls for a workspace section whose feature flag is off.
// The toggle in Settings → Feature Flags takes effect on the next request.
func SectionFlagMiddleware() schemas.GatewayHTTPMiddleware {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			flagID, ok := SectionFlagForPath(string(ctx.Path()))
			if ok && !featureflags.ActiveEnabled(flagID) {
				SendError(ctx, fasthttp.StatusForbidden, "This section is turned off in Settings → Feature Flags")
				return
			}
			next(ctx)
		}
	}
}
