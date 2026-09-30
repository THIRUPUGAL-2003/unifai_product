package handlers

import (
	"encoding/json"
	"time"

	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

type promptDeploymentPayload struct {
	ID            uint      `json:"id"`
	PromptID      string    `json:"prompt_id"`
	PromptName    string    `json:"prompt_name,omitempty"`
	VersionNumber int       `json:"version_number"`
	Environment   string    `json:"environment"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func promptDeploymentFromRow(row tables.TablePromptDeployment) promptDeploymentPayload {
	return promptDeploymentPayload{
		ID: row.ID, PromptID: row.PromptID, PromptName: row.PromptName,
		VersionNumber: row.VersionNumber, Environment: row.Environment, Enabled: row.Enabled,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (h *WorkspaceHandler) listPromptDeployments(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	rows, err := store.ListPromptDeployments(ctx, string(ctx.QueryArgs().Peek("prompt_id")))
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list deployments")
		return
	}
	items := make([]promptDeploymentPayload, 0, len(rows))
	for _, row := range rows {
		items = append(items, promptDeploymentFromRow(row))
	}
	SendJSON(ctx, map[string]any{"deployments": items, "count": len(items)})
}

func (h *WorkspaceHandler) createPromptDeployment(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var payload promptDeploymentPayload
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	if payload.PromptID == "" || payload.Environment == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "prompt_id and environment are required")
		return
	}
	now := time.Now().UTC()
	row := tables.TablePromptDeployment{
		PromptID: payload.PromptID, PromptName: payload.PromptName,
		VersionNumber: payload.VersionNumber, Environment: payload.Environment,
		Enabled: payload.Enabled, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreatePromptDeployment(ctx, &row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save deployment")
		return
	}
	SendJSONWithStatus(ctx, promptDeploymentFromRow(row), fasthttp.StatusCreated)
}

func (h *WorkspaceHandler) updatePromptDeployment(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid deployment id")
		return
	}
	existing, err := store.GetPromptDeployment(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "deployment not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load deployment")
		return
	}
	var patch promptDeploymentPayload
	if err := json.Unmarshal(ctx.PostBody(), &patch); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	if patch.Environment != "" {
		existing.Environment = patch.Environment
	}
	if patch.PromptID != "" {
		existing.PromptID = patch.PromptID
	}
	if patch.PromptName != "" {
		existing.PromptName = patch.PromptName
	}
	if patch.VersionNumber != 0 {
		existing.VersionNumber = patch.VersionNumber
	}
	existing.Enabled = patch.Enabled
	existing.UpdatedAt = time.Now().UTC()
	if err := store.UpdatePromptDeployment(ctx, existing); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to update deployment")
		return
	}
	SendJSON(ctx, promptDeploymentFromRow(*existing))
}

func (h *WorkspaceHandler) deletePromptDeployment(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid deployment id")
		return
	}
	if err := store.DeletePromptDeployment(ctx, id); isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "deployment not found")
		return
	} else if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete deployment")
		return
	}
	SendJSON(ctx, map[string]string{"message": "deleted"})
}
