package handlers

import (
	"encoding/json"

	"github.com/unifai/unifai/framework/configstore"
	configstoreTables "github.com/unifai/unifai/framework/configstore/tables"
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
		var budgetUsage float64
		if user.BudgetID != nil && *user.BudgetID != "" {
			if h.governanceManager != nil {
				data := h.governanceManager.GetGovernanceData(ctx)
				if data != nil && data.Budgets != nil {
					if b, ok := data.Budgets[*user.BudgetID]; ok && b != nil {
						budgetUsage = b.CurrentUsage
					}
				}
			}
			if budgetUsage == 0 {
				if b, err := h.configStore.GetBudget(ctx, *user.BudgetID); err == nil && b != nil {
					budgetUsage = b.CurrentUsage
				}
			}
		}
		members = append(members, map[string]any{
			"id":                   user.ID,
			"user_id":              user.ID,
			"username":             user.Username,
			"email":                user.Email,
			"role":                 user.Role,
			"budget":               user.Budget,
			"budget_current_usage": budgetUsage,
			"created_at":           link.CreatedAt,
			"updated_at":           link.UpdatedAt,
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
	if h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnTeamMemberAdded(ctx, teamID, body.UserID)
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
	if h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnTeamMemberRemoved(ctx, teamID, userID)
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
		item := map[string]any{
			"id":          team.ID,
			"name":        team.Name,
			"customer_id": team.CustomerID,
			"created_at":  link.CreatedAt,
		}
		if team.CustomerID != nil && *team.CustomerID != "" {
			if cust, err := h.configStore.GetCustomer(ctx, *team.CustomerID); err == nil && cust != nil {
				item["customer_name"] = cust.Name
			}
		}
		teams = append(teams, item)
	}
	SendJSON(ctx, map[string]any{"teams": teams})
}

// getUserVirtualKeys handles GET /api/governance/users/{user_id}/virtual-keys
// Returns direct keys, team keys, and cascading Customer keys!
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
	seenVK := make(map[string]bool)

	// 1. Direct Virtual Keys
	for _, link := range links {
		vk, err := h.configStore.GetVirtualKey(ctx, link.VirtualKeyID)
		if err != nil || vk == nil {
			continue
		}
		if seenVK[vk.ID] {
			continue
		}
		seenVK[vk.ID] = true
		keys = append(keys, map[string]any{
			"id":          vk.ID,
			"name":        vk.Name,
			"is_active":   vk.IsActiveValue(),
			"created_at":  link.CreatedAt,
			"origin":      "direct",
			"origin_name": "Direct Assignment",
		})
	}

	// 2. Inherited Virtual Keys via Teams and Customers
	if teamMemberships, err := ws.ListTeamsForUser(ctx, userID); err == nil {
		for _, tm := range teamMemberships {
			team, err := h.configStore.GetTeam(ctx, tm.TeamID)
			if err != nil || team == nil {
				continue
			}

			// Team-assigned Virtual Keys
			var teamVKs []configstoreTables.TableVirtualKey
			_ = h.configStore.DB().WithContext(ctx).
				Joins("LEFT JOIN governance_virtual_key_teams ON governance_virtual_key_teams.virtual_key_id = governance_virtual_keys.id").
				Where("governance_virtual_keys.team_id = ? OR governance_virtual_key_teams.team_id = ?", team.ID, team.ID).
				Find(&teamVKs).Error

			for _, tvk := range teamVKs {
				if seenVK[tvk.ID] {
					continue
				}
				seenVK[tvk.ID] = true
				keys = append(keys, map[string]any{
					"id":          tvk.ID,
					"name":        tvk.Name,
					"is_active":   tvk.IsActiveValue(),
					"created_at":  tvk.CreatedAt,
					"origin":      "team",
					"origin_name": team.Name,
				})
			}

			// Customer-assigned Virtual Keys (Customer -> Team -> User cascade!)
			if team.CustomerID != nil && *team.CustomerID != "" {
				customer, err := h.configStore.GetCustomer(ctx, *team.CustomerID)
				if err == nil && customer != nil {
					var custVKs []configstoreTables.TableVirtualKey
					_ = h.configStore.DB().WithContext(ctx).
						Joins("LEFT JOIN governance_virtual_key_customers ON governance_virtual_key_customers.virtual_key_id = governance_virtual_keys.id").
						Where("governance_virtual_keys.customer_id = ? OR governance_virtual_key_customers.customer_id = ?", customer.ID, customer.ID).
						Find(&custVKs).Error

					for _, cvk := range custVKs {
						if seenVK[cvk.ID] {
							continue
						}
						seenVK[cvk.ID] = true
						keys = append(keys, map[string]any{
							"id":          cvk.ID,
							"name":        cvk.Name,
							"is_active":   cvk.IsActiveValue(),
							"created_at":  cvk.CreatedAt,
							"origin":      "customer",
							"origin_name": customer.Name,
						})
					}
				}
			}
		}
	}

	SendJSON(ctx, map[string]any{"virtual_keys": keys})
}
