package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

type businessUnitPayload struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	TeamIDs   []string       `json:"team_ids,omitempty"`
	Budget    map[string]any `json:"budget,omitempty"`
	RateLimit map[string]any `json:"rate_limit,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

func businessUnitFromRow(row tables.TableBusinessUnit) businessUnitPayload {
	return businessUnitPayload{
		ID: row.ID, Name: row.Name, TeamIDs: row.ParsedTeamIDs,
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
			"id": row.ID, "name": row.Name, "team_count": len(row.ParsedTeamIDs),
			"team_ids": row.ParsedTeamIDs,
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
		Name string `json:"name"`
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
	row := tables.TableBusinessUnit{
		ID: newEntityID(), Name: strings.TrimSpace(body.Name),
		ParsedTeamIDs: []string{}, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateBusinessUnit(ctx, &row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save business unit")
		return
	}
	h.reloadBusinessUnitIndex(ctx)
	SendJSON(ctx, map[string]any{"business_unit": businessUnitFromRow(row)})
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
		"id": row.ID, "name": row.Name, "team_count": len(row.ParsedTeamIDs),
		"team_ids": row.ParsedTeamIDs,
		"budget": row.ParsedBudget, "rate_limit": row.ParsedRateLimit,
		"created_at": row.CreatedAt, "updated_at": row.UpdatedAt,
	})
}

func (h *WorkspaceHandler) deleteBusinessUnit(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	if err := store.DeleteBusinessUnit(ctx, pathID(ctx, "id")); isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "business unit not found")
		return
	} else if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete business unit")
		return
	}
	h.reloadBusinessUnitIndex(ctx)
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
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil || body.TeamID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "team_id is required")
		return
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
	row.ParsedBudget = body.Budget
	row.ParsedRateLimit = body.RateLimit
	row.UpdatedAt = time.Now().UTC()
	if err := store.UpdateBusinessUnit(ctx, row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to update governance")
		return
	}
	SendJSON(ctx, map[string]any{"business_unit": businessUnitFromRow(*row)})
}
