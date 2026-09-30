package handlers

import (
	"encoding/json"

	"github.com/unifai/unifai/framework/configstore"
	"github.com/valyala/fasthttp"
)

// getTeamMembers handles GET /api/governance/teams/{team_id}/members
func (h *GovernanceHandler) getTeamMembers(ctx *fasthttp.RequestCtx) {
	teamID := pathID(ctx, "team_id")
	if teamID == "" {
		teamID = pathID(ctx, "id")
	}
	if teamID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid team id")
		return
	}
	ws, ok := configstore.AsWorkspaceStore(h.configStore)
	if !ok || ws == nil {
		SendJSON(ctx, map[string]any{"members": []any{}})
		return
	}
	if _, err := h.configStore.GetTeam(ctx, teamID); err != nil {
		SendError(ctx, fasthttp.StatusNotFound, "team not found")
		return
	}
	links, err := ws.ListTeamMembers(ctx, teamID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list team members")
		return
	}
	members := make([]map[string]any, 0, len(links))
	for _, link := range links {
		user, err := h.configStore.GetUserByID(ctx, link.UserID)
		if err != nil || user == nil {
			continue
		}
		members = append(members, map[string]any{
			"id":         user.ID,
			"user_id":    user.ID,
			"username":   user.Username,
			"email":      user.Email,
			"role":       user.Role,
			"created_at": link.CreatedAt,
			"updated_at": link.UpdatedAt,
		})
	}
	SendJSON(ctx, map[string]any{"members": members})
}

// addTeamMember handles POST /api/governance/teams/{team_id}/members
// Also accepts legacy POST /api/teams/{id}/members (migration-cli).
func (h *GovernanceHandler) addTeamMember(ctx *fasthttp.RequestCtx) {
	teamID := pathID(ctx, "team_id")
	if teamID == "" {
		teamID = pathID(ctx, "id")
	}
	if teamID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid team id")
		return
	}
	var body struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil || body.UserID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "user_id is required")
		return
	}
	ws, ok := configstore.AsWorkspaceStore(h.configStore)
	if !ok || ws == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "workspace store is not available")
		return
	}
	if _, err := h.configStore.GetTeam(ctx, teamID); err != nil {
		SendError(ctx, fasthttp.StatusNotFound, "team not found")
		return
	}
	if user, err := h.configStore.GetUserByID(ctx, body.UserID); err != nil || user == nil {
		SendError(ctx, fasthttp.StatusNotFound, "user not found")
		return
	}
	if err := ws.AddTeamMember(ctx, teamID, body.UserID); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to add team member: "+err.Error())
		return
	}
	h.getTeamMembers(ctx)
}

// removeTeamMember handles DELETE /api/governance/teams/{team_id}/members/{user_id}
func (h *GovernanceHandler) removeTeamMember(ctx *fasthttp.RequestCtx) {
	teamID := pathID(ctx, "team_id")
	userID := pathID(ctx, "user_id")
	if teamID == "" || userID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "team_id and user_id are required")
		return
	}
	ws, ok := configstore.AsWorkspaceStore(h.configStore)
	if !ok || ws == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "workspace store is not available")
		return
	}
	if err := ws.RemoveTeamMember(ctx, teamID, userID); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to remove team member: "+err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"ok": true})
}

// getUserTeams handles GET /api/governance/users/{user_id}/teams
func (h *GovernanceHandler) getUserTeams(ctx *fasthttp.RequestCtx) {
	userID := pathID(ctx, "user_id")
	if userID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid user id")
		return
	}
	ws, ok := configstore.AsWorkspaceStore(h.configStore)
	if !ok || ws == nil {
		SendJSON(ctx, map[string]any{"teams": []any{}})
		return
	}
	links, err := ws.ListTeamsForUser(ctx, userID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list user teams")
		return
	}
	teams := make([]map[string]any, 0, len(links))
	for _, link := range links {
		team, err := h.configStore.GetTeam(ctx, link.TeamID)
		if err != nil || team == nil {
			continue
		}
		teams = append(teams, map[string]any{
			"id":         team.ID,
			"name":       team.Name,
			"team_id":    team.ID,
			"created_at": link.CreatedAt,
		})
	}
	SendJSON(ctx, map[string]any{"teams": teams})
}

// getUserVirtualKeys handles GET /api/governance/users/{user_id}/virtual-keys
func (h *GovernanceHandler) getUserVirtualKeys(ctx *fasthttp.RequestCtx) {
	userID := pathID(ctx, "user_id")
	if userID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid user id")
		return
	}
	ws, ok := configstore.AsWorkspaceStore(h.configStore)
	if !ok || ws == nil {
		SendJSON(ctx, map[string]any{"virtual_keys": []any{}})
		return
	}
	links, err := ws.ListVirtualKeysForUser(ctx, userID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list user virtual keys")
		return
	}
	keys := make([]map[string]any, 0, len(links))
	for _, link := range links {
		vk, err := h.configStore.GetVirtualKey(ctx, link.VirtualKeyID)
		if err != nil || vk == nil {
			continue
		}
		keys = append(keys, map[string]any{
			"id":         vk.ID,
			"name":       vk.Name,
			"is_active":  vk.IsActiveValue(),
			"created_at": link.CreatedAt,
		})
	}
	SendJSON(ctx, map[string]any{"virtual_keys": keys})
}
