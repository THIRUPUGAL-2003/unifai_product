package handlers

import (
	"encoding/json"

	"github.com/unifai/unifai/framework/configstore"
	tables "github.com/unifai/unifai/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

func (h *GovernanceHandler) getVirtualKeyUsers(ctx *fasthttp.RequestCtx) {
	vkID := pathID(ctx, "vk_id")
	if vkID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid virtual key id")
		return
	}
	if allowed, filter := h.allowedVKIDsForCaller(ctx); filter {
		if !allowed[vkID] {
			SendError(ctx, fasthttp.StatusNotFound, "Virtual key not found")
			return
		}
	}
	ws, ok := configstore.AsWorkspaceStore(h.configStore)
	if !ok || ws == nil {
		SendJSON(ctx, map[string]any{"users": []any{}})
		return
	}
	vk, err := h.configStore.GetVirtualKey(ctx, vkID)
	if err != nil || vk == nil {
		SendError(ctx, fasthttp.StatusNotFound, "Virtual key not found")
		return
	}

	seenUserIDs := make(map[string]bool)
	users := make([]map[string]any, 0)

	appendUser := func(userID string, origin string, originName string) {
		if userID == "" || seenUserIDs[userID] {
			return
		}
		seenUserIDs[userID] = true
		user, err := h.configStore.GetUserByID(ctx, userID)
		if err != nil || user == nil {
			return
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
		users = append(users, map[string]any{
			"id":                   user.ID,
			"name":                 user.Username,
			"email":                user.Email,
			"role":                 user.Role,
			"budget":               user.Budget,
			"budget_current_usage": budgetUsage,
			"origin":               origin,
			"origin_name":          originName,
			"created_at":           user.CreatedAt,
			"updated_at":           user.UpdatedAt,
		})
	}

	// 1. Direct Virtual Key Users
	links, _ := ws.ListVirtualKeyUsers(ctx, vkID)
	for _, link := range links {
		appendUser(link.UserID, "direct", "Direct Assignment")
	}

	// 2. Users from Teams assigned to this VK
	var teamIDs []string
	if vk.TeamID != nil && *vk.TeamID != "" {
		teamIDs = append(teamIDs, *vk.TeamID)
	}
	for _, t := range vk.Teams {
		teamIDs = append(teamIDs, t.ID)
	}
	var teamLinks []struct {
		TeamID string
	}
	_ = h.configStore.DB().WithContext(ctx).Table("governance_virtual_key_teams").
		Select("team_id").Where("virtual_key_id = ?", vkID).Scan(&teamLinks).Error
	for _, tl := range teamLinks {
		teamIDs = append(teamIDs, tl.TeamID)
	}

	for _, tid := range teamIDs {
		team, err := h.configStore.GetTeam(ctx, tid)
		teamName := "Team"
		if err == nil && team != nil {
			teamName = team.Name
		}
		if members, err := ws.ListTeamMembers(ctx, tid); err == nil {
			for _, m := range members {
				appendUser(m.UserID, "team", teamName)
			}
		}
	}

	// 3. Users from Customers assigned to this VK (Customer -> Teams -> Members)
	var customerIDs []string
	if vk.CustomerID != nil && *vk.CustomerID != "" {
		customerIDs = append(customerIDs, *vk.CustomerID)
	}
	for _, c := range vk.Customers {
		customerIDs = append(customerIDs, c.ID)
	}
	var custLinks []struct {
		CustomerID string
	}
	_ = h.configStore.DB().WithContext(ctx).Table("governance_virtual_key_customers").
		Select("customer_id").Where("virtual_key_id = ?", vkID).Scan(&custLinks).Error
	for _, cl := range custLinks {
		customerIDs = append(customerIDs, cl.CustomerID)
	}

	for _, cid := range customerIDs {
		cust, err := h.configStore.GetCustomer(ctx, cid)
		custName := "Customer"
		if err == nil && cust != nil {
			custName = cust.Name
		}
		var custTeams []tables.TableTeam
		_ = h.configStore.DB().WithContext(ctx).Where("customer_id = ?", cid).Find(&custTeams).Error
		for _, ct := range custTeams {
			if members, err := ws.ListTeamMembers(ctx, ct.ID); err == nil {
				for _, m := range members {
					appendUser(m.UserID, "customer", custName+" / "+ct.Name)
				}
			}
		}
	}

	SendJSON(ctx, map[string]any{"users": users})
}

func (h *GovernanceHandler) setVirtualKeyUser(ctx *fasthttp.RequestCtx) {
	vkID := pathID(ctx, "vk_id")
	if vkID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid virtual key id")
		return
	}
	var body struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request body")
		return
	}
	ws, ok := configstore.AsWorkspaceStore(h.configStore)
	if !ok || ws == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "workspace store is not available")
		return
	}
	if _, err := h.configStore.GetVirtualKey(ctx, vkID); err != nil {
		SendError(ctx, fasthttp.StatusNotFound, "virtual key not found")
		return
	}
	if body.UserID == "" || body.UserID == "__unassigned__" {
		targetUserID := string(ctx.QueryArgs().Peek("user_id"))
		if targetUserID != "" {
			if err := ws.RemoveVirtualKeyUser(ctx, vkID, targetUserID); err != nil {
				SendError(ctx, fasthttp.StatusInternalServerError, "failed to unassign user from virtual key: "+err.Error())
				return
			}
		} else {
			if err := ws.DeleteVirtualKeyUser(ctx, vkID); err != nil {
				SendError(ctx, fasthttp.StatusInternalServerError, "failed to unassign user from virtual key: "+err.Error())
				return
			}
		}
		SendJSON(ctx, map[string]any{"users": []any{}})
		return
	}
	if user, err := h.configStore.GetUserByID(ctx, body.UserID); err != nil || user == nil {
		SendError(ctx, fasthttp.StatusNotFound, "user not found")
		return
	}
	if err := ws.SetVirtualKeyUser(ctx, vkID, body.UserID); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to assign user to virtual key: "+err.Error())
		return
	}
	h.getVirtualKeyUsers(ctx)
}

func (h *GovernanceHandler) deleteVirtualKeyUser(ctx *fasthttp.RequestCtx) {
	vkID := pathID(ctx, "vk_id")
	if vkID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid virtual key id")
		return
	}
	ws, ok := configstore.AsWorkspaceStore(h.configStore)
	if !ok || ws == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "workspace store is not available")
		return
	}
	targetUserID := string(ctx.QueryArgs().Peek("user_id"))
	if targetUserID == "" {
		var body struct {
			UserID string `json:"user_id"`
		}
		_ = json.Unmarshal(ctx.PostBody(), &body)
		targetUserID = body.UserID
	}
	if targetUserID != "" {
		if err := ws.RemoveVirtualKeyUser(ctx, vkID, targetUserID); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "failed to unassign user from virtual key: "+err.Error())
			return
		}
	} else {
		if err := ws.DeleteVirtualKeyUser(ctx, vkID); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "failed to unassign user from virtual key: "+err.Error())
			return
		}
	}
	h.getVirtualKeyUsers(ctx)
}
