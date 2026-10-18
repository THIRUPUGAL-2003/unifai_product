package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gateway/gateway/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

type businessUnitPayload struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	CustomerID *string        `json:"customer_id,omitempty"`
	TeamIDs    []string       `json:"team_ids,omitempty"`
	Budget     map[string]any `json:"budget,omitempty"`
	RateLimit  map[string]any `json:"rate_limit,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

func businessUnitFromRow(row tables.TableBusinessUnit) businessUnitPayload {
	return businessUnitPayload{
		ID: row.ID, Name: row.Name, CustomerID: row.CustomerID, TeamIDs: row.ParsedTeamIDs,
		Budget: row.ParsedBudget, RateLimit: row.ParsedRateLimit,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (h *WorkspaceHandler) reloadBusinessUnitIndex(ctx *fasthttp.RequestCtx) {
	if h == nil || h.governanceManager == nil {
		return
	}
	if reloader, ok := h.governanceManager.(interface {
		ReloadBusinessUnitTeamIndex(ctx context.Context)
	}); ok {
		reloader.ReloadBusinessUnitTeamIndex(ctx)
	}
}

func (h *WorkspaceHandler) listBusinessUnits(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	rows, err := store.ListBusinessUnits(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list business units")
		return
	}
	search := strings.ToLower(string(ctx.QueryArgs().Peek("search")))
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if search != "" && !strings.Contains(strings.ToLower(row.Name), search) {
			continue
		}
		items = append(items, map[string]any{
			"id": row.ID, "name": row.Name, "customer_id": row.CustomerID, "team_count": len(row.ParsedTeamIDs),
			"team_ids":   row.ParsedTeamIDs,
			"budget":     row.ParsedBudget,
			"rate_limit": row.ParsedRateLimit,
			"created_at": row.CreatedAt, "updated_at": row.UpdatedAt,
		})
	}
	page := queryInt(ctx, "page", 1)
	limit := queryInt(ctx, "limit", 20)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	total := len(items)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	SendJSON(ctx, map[string]any{
		"business_units": items[start:end],
		"total":          total,
		"page":           page,
		"limit":          limit,
	})
}

func (h *WorkspaceHandler) createBusinessUnit(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var body struct {
		Name       string         `json:"name"`
		CustomerID *string        `json:"customer_id"`
		Budget     map[string]any `json:"budget"`
		RateLimit  map[string]any `json:"rate_limit"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil || strings.TrimSpace(body.Name) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "name is required")
		return
	}
	existing, err := store.ListBusinessUnits(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list business units")
		return
	}
	for _, row := range existing {
		if strings.EqualFold(row.Name, body.Name) {
			SendError(ctx, fasthttp.StatusConflict, "a business unit with this name already exists")
			return
		}
	}
	now := time.Now().UTC()
	var custID *string
	if body.CustomerID != nil && strings.TrimSpace(*body.CustomerID) != "" {
		trimmed := strings.TrimSpace(*body.CustomerID)
		custID = &trimmed
	}
	row := tables.TableBusinessUnit{
		ID: newEntityID(), Name: strings.TrimSpace(body.Name), CustomerID: custID,
		ParsedTeamIDs: []string{}, CreatedAt: now, UpdatedAt: now,
	}
	applyBusinessUnitBudget(&row, body.Budget)
	applyBusinessUnitRateLimit(&row, body.RateLimit)
	if err := store.CreateBusinessUnit(ctx, &row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save business unit")
		return
	}
	h.reloadBusinessUnitIndex(ctx)
	if h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnBusinessUnitCreated(ctx, &row)
	}
	SendJSON(ctx, map[string]any{"business_unit": businessUnitFromRow(row)})
}

func (h *WorkspaceHandler) updateBusinessUnit(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id := pathID(ctx, "id")
	row, err := store.GetBusinessUnit(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "business unit not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load business unit")
		return
	}
	var body struct {
		Name       string          `json:"name"`
		CustomerID *string         `json:"customer_id"`
		Budget     *map[string]any `json:"budget"`
		RateLimit  *map[string]any `json:"rate_limit"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil || strings.TrimSpace(body.Name) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "name is required")
		return
	}
	name := strings.TrimSpace(body.Name)
	existing, err := store.ListBusinessUnits(ctx)
	if err == nil {
		for _, r := range existing {
			if r.ID != id && strings.EqualFold(r.Name, name) {
				SendError(ctx, fasthttp.StatusConflict, "a business unit with this name already exists")
				return
			}
		}
	}
	oldName := row.Name
	oldCustomerID := row.CustomerID
	row.Name = name
	if body.CustomerID != nil {
		if strings.TrimSpace(*body.CustomerID) != "" {
			trimmed := strings.TrimSpace(*body.CustomerID)
			row.CustomerID = &trimmed
		} else {
			row.CustomerID = nil
		}
	}
	if body.Budget != nil {
		applyBusinessUnitBudget(row, *body.Budget)
	}
	if body.RateLimit != nil {
		applyBusinessUnitRateLimit(row, *body.RateLimit)
	}
	row.UpdatedAt = time.Now().UTC()
	if err := store.UpdateBusinessUnit(ctx, row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to update business unit")
		return
	}
	h.reloadBusinessUnitIndex(ctx)
	if h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnBusinessUnitUpdated(ctx, row, oldName, oldCustomerID)
	}
	SendJSON(ctx, map[string]any{"business_unit": businessUnitFromRow(*row)})
}

func (h *WorkspaceHandler) getBusinessUnit(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	row, err := store.GetBusinessUnit(ctx, pathID(ctx, "id"))
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "business unit not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load business unit")
		return
	}
	SendJSON(ctx, map[string]any{
		"id": row.ID, "name": row.Name, "customer_id": row.CustomerID, "team_count": len(row.ParsedTeamIDs),
		"team_ids": row.ParsedTeamIDs,
		"budget":   row.ParsedBudget, "rate_limit": row.ParsedRateLimit,
		"created_at": row.CreatedAt, "updated_at": row.UpdatedAt,
	})
}

func (h *WorkspaceHandler) deleteBusinessUnit(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id := pathID(ctx, "id")
	row, _ := store.GetBusinessUnit(ctx, id)
	if err := store.DeleteBusinessUnit(ctx, id); isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "business unit not found")
		return
	} else if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete business unit")
		return
	}
	h.reloadBusinessUnitIndex(ctx)
	if h.promptLifecycle != nil && row != nil {
		_ = h.promptLifecycle.OnBusinessUnitDeleted(ctx, row)
	}
	SendJSON(ctx, map[string]string{"message": "deleted"})
}

func (h *WorkspaceHandler) listBusinessUnitTeams(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	row, err := store.GetBusinessUnit(ctx, pathID(ctx, "id"))
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "business unit not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load business unit")
		return
	}
	teams := []map[string]any{}
	if h.store != nil && h.store.ConfigStore != nil {
		allTeams, err := h.store.ConfigStore.GetTeams(ctx, "")
		if err == nil {
			wanted := map[string]bool{}
			for _, id := range row.ParsedTeamIDs {
				wanted[id] = true
			}
			for _, team := range allTeams {
				if wanted[team.ID] {
					teams = append(teams, map[string]any{"id": team.ID, "name": team.Name})
				}
			}
		}
	}
	SendJSON(ctx, map[string]any{"teams": teams, "total": len(teams)})
}

func (h *WorkspaceHandler) assignBusinessUnitTeam(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id := pathID(ctx, "id")
	row, err := store.GetBusinessUnit(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "business unit not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load business unit")
		return
	}
	var body struct {
		TeamID string `json:"team_id"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil || strings.TrimSpace(body.TeamID) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "team_id is required")
		return
	}
	body.TeamID = strings.TrimSpace(body.TeamID)
	if h.store != nil && h.store.ConfigStore != nil {
		team, err := h.store.ConfigStore.GetTeam(ctx, body.TeamID)
		if err != nil || team == nil {
			if err == nil || isStoreNotFound(err) {
				SendError(ctx, fasthttp.StatusNotFound, "team not found")
				return
			}
			SendError(ctx, fasthttp.StatusInternalServerError, "failed to load team")
			return
		}
	}
	for _, existing := range row.ParsedTeamIDs {
		if existing == body.TeamID {
			SendJSON(ctx, map[string]string{"message": "team assigned"})
			return
		}
	}
	row.ParsedTeamIDs = append(row.ParsedTeamIDs, body.TeamID)
	row.UpdatedAt = time.Now().UTC()
	if err := store.UpdateBusinessUnit(ctx, row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to assign team")
		return
	}
	h.reloadBusinessUnitIndex(ctx)
	if h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnTeamAssignedToBusinessUnit(ctx, body.TeamID, row.ID)
	}
	SendJSON(ctx, map[string]string{"message": "team assigned"})
}

func (h *WorkspaceHandler) removeBusinessUnitTeam(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	row, err := store.GetBusinessUnit(ctx, pathID(ctx, "id"))
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "business unit not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load business unit")
		return
	}
	teamID := pathID(ctx, "team_id")
	next := row.ParsedTeamIDs[:0]
	for _, existing := range row.ParsedTeamIDs {
		if existing != teamID {
			next = append(next, existing)
		}
	}
	row.ParsedTeamIDs = next
	row.UpdatedAt = time.Now().UTC()
	if err := store.UpdateBusinessUnit(ctx, row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to remove team")
		return
	}
	h.reloadBusinessUnitIndex(ctx)
	if h.promptLifecycle != nil {
		_ = h.promptLifecycle.OnTeamRemovedFromBusinessUnit(ctx, teamID, row.ID)
	}
	SendJSON(ctx, map[string]string{"message": "team removed"})
}

func (h *WorkspaceHandler) updateBusinessUnitGovernance(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	row, err := store.GetBusinessUnit(ctx, pathID(ctx, "id"))
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "business unit not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load business unit")
		return
	}
	var body struct {
		Budget    map[string]any `json:"budget"`
		RateLimit map[string]any `json:"rate_limit"`
	}
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	if len(body.Budget) > 0 {
		applyBusinessUnitBudget(row, body.Budget)
	} else {
		row.ParsedBudget = nil
	}
	applyBusinessUnitRateLimit(row, body.RateLimit)
	row.UpdatedAt = time.Now().UTC()
	if err := store.UpdateBusinessUnit(ctx, row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to update governance")
		return
	}
	h.reloadBusinessUnitIndex(ctx)
	SendJSON(ctx, map[string]any{"business_unit": businessUnitFromRow(*row)})
}

func applyBusinessUnitBudget(row *tables.TableBusinessUnit, budget map[string]any) {
	if row == nil {
		return
	}
	maxLimit, ok := businessUnitNumber(budget["max_limit"])
	if !ok || maxLimit <= 0 {
		row.ParsedBudget = nil
		return
	}
	reset, _ := budget["reset_duration"].(string)
	switch reset {
	case "1h", "1d", "1w", "1M":
	default:
		reset = "1M"
	}
	current := 0.0
	if row.ParsedBudget != nil {
		if used, ok := businessUnitNumber(row.ParsedBudget["current_usage"]); ok {
			current = used
		}
	}
	row.ParsedBudget = map[string]any{
		"max_limit":      maxLimit,
		"current_usage":  current,
		"reset_duration": reset,
	}
}

func applyBusinessUnitRateLimit(row *tables.TableBusinessUnit, rate map[string]any) {
	if row == nil {
		return
	}
	tokenMax, tokenOK := businessUnitNumber(rate["token_max_limit"])
	requestMax, requestOK := businessUnitNumber(rate["request_max_limit"])
	if (!tokenOK || tokenMax <= 0) && (!requestOK || requestMax <= 0) {
		row.ParsedRateLimit = nil
		return
	}
	out := map[string]any{}
	if row.ParsedRateLimit != nil {
		if used, ok := businessUnitNumber(row.ParsedRateLimit["token_current_usage"]); ok {
			out["token_current_usage"] = used
		}
		if used, ok := businessUnitNumber(row.ParsedRateLimit["request_current_usage"]); ok {
			out["request_current_usage"] = used
		}
	}
	if tokenOK && tokenMax > 0 {
		reset, _ := rate["token_reset_duration"].(string)
		if _, err := tables.ParseDuration(reset); err != nil {
			reset = "1h"
		}
		out["token_max_limit"] = tokenMax
		out["token_reset_duration"] = reset
		if _, ok := out["token_current_usage"]; !ok {
			out["token_current_usage"] = 0.0
		}
	}
	if requestOK && requestMax > 0 {
		reset, _ := rate["request_reset_duration"].(string)
		if _, err := tables.ParseDuration(reset); err != nil {
			reset = "1h"
		}
		out["request_max_limit"] = requestMax
		out["request_reset_duration"] = reset
		if _, ok := out["request_current_usage"]; !ok {
			out["request_current_usage"] = 0.0
		}
	}
	row.ParsedRateLimit = out
}

func businessUnitNumber(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
