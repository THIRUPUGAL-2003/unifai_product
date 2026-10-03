package credstore

import "github.com/raksha/raksha/core/schemas"

// identityForMCPAuthMode returns the identity string to look up by, given the
// derived mode. Mirrors the priority used by ctx.MCPAuthMode().
//
// Used by every resolver that keys persisted state by (mode, identity,
// mcp_client) — currently per-user OAuth and per-user headers. Lives in its
// own file so both resolvers can call it without duplication or accidental
// drift.
func identityForMCPAuthMode(ctx *schemas.RakshaContext, mode schemas.MCPAuthMode) string {
	switch mode {
	case schemas.MCPAuthModeUser:
		if v, _ := ctx.Value(schemas.RakshaContextKeyUserID).(string); v != "" {
			return v
		}
	case schemas.MCPAuthModeVK:
		if v, _ := ctx.Value(schemas.RakshaContextKeyGovernanceVirtualKeyID).(string); v != "" {
			return v
		}
	case schemas.MCPAuthModeSession:
		if v, _ := ctx.Value(schemas.RakshaContextKeyMCPSessionID).(string); v != "" {
			return v
		}
	}
	return ""
}
