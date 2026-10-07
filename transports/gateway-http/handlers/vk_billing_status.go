package handlers

import (
	"context"

	"github.com/raksha/raksha/core/schemas"
	"github.com/raksha/raksha/framework/configstore"
	"github.com/valyala/fasthttp"
)

// exhaustedBilledEntityResolver is implemented by the server bridge to the governance plugin.
type exhaustedBilledEntityResolver interface {
	ExhaustedBilledEntity(ctx context.Context, vkID string, userTeamIDs []string) (string, string)
}

// getVirtualKeyBillingBlocks handles GET /api/governance/virtual-keys/billing-blocks.
// For the signed-in user it lists keys governance would reject because the VK, team, or
// customer budget is used up, so the UI can steer them to another key or wait for admin reset.
func (h *GovernanceHandler) getVirtualKeyBillingBlocks(ctx *fasthttp.RequestCtx) {
	blocks := map[string]map[string]string{}
	resolver, ok := h.governanceManager.(exhaustedBilledEntityResolver)
	if !ok || h.configStore == nil {
		SendJSON(ctx, map[string]any{"blocks": blocks})
		return
	}

	var userTeamIDs []string
	if userID := h.sessionUserID(ctx); userID != "" {
		if ws, ok := configstore.AsWorkspaceStore(h.configStore); ok && ws != nil {
			if links, err := ws.ListTeamsForUser(ctx, userID); err == nil {
				for _, link := range links {
					if link.TeamID != "" {
						userTeamIDs = append(userTeamIDs, link.TeamID)
					}
				}
			}
		}
	}

	var vkIDs []string
	if allowed, filter := h.allowedVKIDsForCaller(ctx); filter {
		for id := range allowed {
			vkIDs = append(vkIDs, id)
		}
	} else if data := h.governanceManager.GetGovernanceData(ctx); data != nil {
		for _, vk := range data.VirtualKeys {
			if vk != nil {
				vkIDs = append(vkIDs, vk.ID)
			}
		}
	}

	for _, id := range vkIDs {
		if scope, name := resolver.ExhaustedBilledEntity(ctx, id, userTeamIDs); scope != "" {
			blocks[id] = map[string]string{"scope": scope, "name": name}
		}
	}
	SendJSON(ctx, map[string]any{"blocks": blocks})
}

// sessionUserID resolves the dashboard session's user row ID, or "" without one.
func (h *GovernanceHandler) sessionUserID(ctx *fasthttp.RequestCtx) string {
	token, _ := ctx.UserValue(schemas.RakshaContextKeySessionToken).(string)
	if token == "" {
		token = sessionToken(ctx)
	}
	if token == "" {
		return ""
	}
	session, err := h.configStore.GetSession(ctx, token)
	if err != nil || session == nil {
		return ""
	}
	user, err := h.configStore.GetUserByUsername(ctx, session.Username)
	if err != nil || user == nil {
		return ""
	}
	return user.ID
}
