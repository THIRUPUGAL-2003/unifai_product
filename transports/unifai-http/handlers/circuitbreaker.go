package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/unifai/unifai/framework/circuitbreaker"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

type circuitBreakerPolicyPayload struct {
	Name             string   `json:"name"`
	Enabled          *bool    `json:"enabled,omitempty"`
	PrimaryProvider  string   `json:"primary_provider"`
	PrimaryModel     string   `json:"primary_model"`
	PrimaryKeyIDs    []string `json:"primary_key_ids,omitempty"`
	FallbackProvider string   `json:"fallback_provider"`
	FallbackModel    string   `json:"fallback_model"`
	Condition        struct {
		Operator string `json:"operator"`
		Signals  []struct {
			Source         string `json:"source"`
			HeaderName     string `json:"header_name"`
			HeaderValue    string `json:"header_value,omitempty"`
			HeaderContains string `json:"header_contains,omitempty"`
		} `json:"signals"`
	} `json:"condition"`
	DefaultCooldown string `json:"default_cooldown,omitempty"`
	CooldownHeader  string `json:"cooldown_header,omitempty"`
}

func policyEnabled(policy circuitBreakerPolicyPayload) bool {
	return policy.Enabled == nil || *policy.Enabled
}

func circuitPolicyFromRow(row tables.TableCircuitBreakerPolicy) circuitBreakerPolicyPayload {
	enabled := row.Enabled
	payload := circuitBreakerPolicyPayload{
		Name: row.Name, Enabled: &enabled,
		PrimaryProvider: row.PrimaryProvider, PrimaryModel: row.PrimaryModel,
		PrimaryKeyIDs:    row.ParsedPrimaryKeyIDs,
		FallbackProvider: row.FallbackProvider, FallbackModel: row.FallbackModel,
		DefaultCooldown: row.DefaultCooldown, CooldownHeader: row.CooldownHeader,
	}
	raw, _ := json.Marshal(row.ParsedCondition)
	_ = json.Unmarshal(raw, &payload.Condition)
	return payload
}

func (p circuitBreakerPolicyPayload) toRow() tables.TableCircuitBreakerPolicy {
	condition := map[string]any{}
	raw, _ := json.Marshal(p.Condition)
	_ = json.Unmarshal(raw, &condition)
	now := time.Now().UTC()
	return tables.TableCircuitBreakerPolicy{
		Name: p.Name, Enabled: policyEnabled(p),
		PrimaryProvider: p.PrimaryProvider, PrimaryModel: p.PrimaryModel,
		FallbackProvider: p.FallbackProvider, FallbackModel: p.FallbackModel,
		DefaultCooldown: p.DefaultCooldown, CooldownHeader: p.CooldownHeader,
		ParsedCondition: condition, ParsedPrimaryKeyIDs: p.PrimaryKeyIDs,
		CreatedAt: now, UpdatedAt: now,
	}
}

func validateCircuitBreakerPolicy(policy *circuitBreakerPolicyPayload) string {
	if strings.TrimSpace(policy.Name) == "" {
		return "name is required"
	}
	if policy.PrimaryProvider == "" || policy.PrimaryModel == "" {
		return "primary_provider and primary_model are required"
	}
	if policy.FallbackProvider == "" || policy.FallbackModel == "" {
		return "fallback_provider and fallback_model are required"
	}
	if len(policy.Condition.Signals) == 0 {
		return "condition.signals is required"
	}
	for i, sig := range policy.Condition.Signals {
		if strings.TrimSpace(sig.HeaderName) == "" {
			return fmt.Sprintf("condition.signals[%d].header_name is required", i)
		}
	}
	if policy.Condition.Operator == "" {
		policy.Condition.Operator = "OR"
	}
	if policy.DefaultCooldown == "" {
		policy.DefaultCooldown = "30s"
	}
	return ""
}

func (h *WorkspaceHandler) listCircuitBreakerPolicies(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	rows, err := store.ListCircuitBreakerPolicies(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list policies")
		return
	}
	policies := make([]circuitBreakerPolicyPayload, 0, len(rows))
	for _, row := range rows {
		policies = append(policies, circuitPolicyFromRow(row))
	}
	SendJSON(ctx, map[string]any{"policies": policies, "count": len(policies)})
}

func (h *WorkspaceHandler) createCircuitBreakerPolicy(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var payload circuitBreakerPolicyPayload
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	if msg := validateCircuitBreakerPolicy(&payload); msg != "" {
		SendError(ctx, fasthttp.StatusBadRequest, msg)
		return
	}
	if _, err := store.GetCircuitBreakerPolicy(ctx, payload.Name); err == nil {
		SendError(ctx, fasthttp.StatusConflict, "a policy with this name already exists")
		return
	}
	row := payload.toRow()
	if err := store.CreateCircuitBreakerPolicy(ctx, &row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save policy")
		return
	}
	reloadCircuitBreakerPolicies(store)
	SendJSONWithStatus(ctx, payload, fasthttp.StatusCreated)
}

func (h *WorkspaceHandler) updateCircuitBreakerPolicy(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	name := pathID(ctx, "name")
	existing, err := store.GetCircuitBreakerPolicy(ctx, name)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "policy not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load policy")
		return
	}
	var payload circuitBreakerPolicyPayload
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	if payload.Name == "" {
		payload.Name = name
	}
	if payload.Name != name {
		SendError(ctx, fasthttp.StatusBadRequest, "name in body must match the url")
		return
	}
	if msg := validateCircuitBreakerPolicy(&payload); msg != "" {
		SendError(ctx, fasthttp.StatusBadRequest, msg)
		return
	}
	row := payload.toRow()
	row.CreatedAt = existing.CreatedAt
	row.UpdatedAt = time.Now().UTC()
	if err := store.UpdateCircuitBreakerPolicy(ctx, &row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to update policy")
		return
	}
	reloadCircuitBreakerPolicies(store)
	SendJSON(ctx, payload)
}

func (h *WorkspaceHandler) deleteCircuitBreakerPolicy(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	name := pathID(ctx, "name")
	if err := store.DeleteCircuitBreakerPolicy(ctx, name); isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "policy not found")
		return
	} else if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete policy")
		return
	}
	circuitbreaker.Default.DeletePolicy(name)
	reloadCircuitBreakerPolicies(store)
	SendJSON(ctx, map[string]string{"message": "deleted"})
}

func (h *WorkspaceHandler) getCircuitBreakerState(ctx *fasthttp.RequestCtx) {
	SendJSON(ctx, map[string]any{"circuits": circuitbreaker.Default.ListStates()})
}

func (h *WorkspaceHandler) resetCircuitBreakerPolicy(ctx *fasthttp.RequestCtx) {
	circuitbreaker.Default.Reset(pathID(ctx, "name"))
	SendJSON(ctx, map[string]string{"message": "reset"})
}

func reloadCircuitBreakerPolicies(store configstore.WorkspaceStore) {
	if store == nil {
		return
	}
	rows, err := store.ListCircuitBreakerPolicies(context.Background())
	if err != nil {
		return
	}
	circuitbreaker.Default.LoadPolicies(rows)
}

// ReloadCircuitBreakerPoliciesFromStore refreshes runtime policies (server startup).
func ReloadCircuitBreakerPoliciesFromStore(store configstore.WorkspaceStore) {
	reloadCircuitBreakerPolicies(store)
}
