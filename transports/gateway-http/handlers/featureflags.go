package handlers

import (
	"errors"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/router"
	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/featureflags"
	"github.com/gateway/gateway/transports/gateway-http/lib"
	"github.com/valyala/fasthttp"
)

// FeatureFlagsHandler serves the toggle UI/API. It mediates between the
// HTTP layer, the in-memory featureflags.Store, and the configstore where
// overrides are persisted. The store is the source of truth for the
// effective value; the configstore exists only so toggles survive restarts.
type FeatureFlagsHandler struct {
	store       *featureflags.Store
	configStore configstore.ConfigStore
}

// NewFeatureFlagsHandler wires the handler to its dependencies. Both must
// be non-nil at server boot; the handler intentionally does not lazily
// resolve them because feature flag state is needed during request
// dispatch and a missing store would cause silent off-by-default behavior.
func NewFeatureFlagsHandler(store *featureflags.Store, configStore configstore.ConfigStore) *FeatureFlagsHandler {
	return &FeatureFlagsHandler{store: store, configStore: configStore}
}

// RegisterRoutes mounts the feature flag endpoints. Supports listing,
// creating dynamic flags, toggling existing flags, and deleting overrides.
func (h *FeatureFlagsHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.GatewayHTTPMiddleware) {
	r.GET("/api/feature-flags", lib.ChainMiddlewares(h.listFlags, middlewares...))
	r.POST("/api/feature-flags", lib.ChainMiddlewares(h.createFlag, middlewares...))
	r.PUT("/api/feature-flags/{id}", lib.ChainMiddlewares(h.updateFlag, middlewares...))
	r.DELETE("/api/feature-flags/{id}", lib.ChainMiddlewares(h.deleteFlag, middlewares...))
}


// featureFlagsListResponse keeps the wire format flexible: wrapping the
// array in an object lets us add pagination / counts later without breaking
// existing UI clients.
type featureFlagsListResponse struct {
	Flags []featureflags.FlagStatus `json:"flags"`
}

func (h *FeatureFlagsHandler) listFlags(ctx *fasthttp.RequestCtx) {
	SendJSON(ctx, featureFlagsListResponse{Flags: h.store.List()})
}

type updateFlagRequest struct {
	// Pointer so that a missing field decodes as nil rather than false;
	// otherwise a PUT with an empty body silently disables the flag.
	Enabled *bool `json:"enabled"`
}

func (h *FeatureFlagsHandler) updateFlag(ctx *fasthttp.RequestCtx) {
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid flag id")
		return
	}

	var req updateFlagRequest
	if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}
	if req.Enabled == nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Missing required field: enabled")
		return
	}
	enabled := *req.Enabled

	// Capture prior in-memory state verbatim so a failed DB write below
	// can be rolled back without corrupting metadata. Snapshot's second
	// return distinguishes "had an override" from "was at code default";
	// Restore uses that to delete vs. write-back, preserving the original
	// source (default / db / file) and timestamps exactly.
	priorSnap, priorHad := h.store.Snapshot(id)

	status, err := h.store.Set(ctx, id, enabled)
	switch {
	case errors.Is(err, featureflags.ErrFlagLocked):
		SendError(ctx, fasthttp.StatusConflict, "Feature flag is locked by config.json / Helm")
		return
	case errors.Is(err, featureflags.ErrFlagEnterpriseOnly):
		SendError(ctx, fasthttp.StatusForbidden, "Feature flag is enterprise-only")
		return
	case errors.Is(err, featureflags.ErrFlagUnregistered):
		SendError(ctx, fasthttp.StatusNotFound, "Feature flag is not registered")
		return
	case err != nil:
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to update feature flag: "+err.Error())
		return
	}

	// Persist after the in-memory toggle succeeds. If the DB write fails
	// we roll back the in-memory change to keep the two layers consistent;
	// otherwise a subsequent restart would silently revert the operator's
	// toggle and they would have no way to know.
	if h.configStore != nil {
		if err := h.configStore.UpsertFeatureFlag(ctx, id, enabled, status.UpdatedAt); err != nil {
			h.store.Restore(id, priorSnap, priorHad)
			SendError(ctx, fasthttp.StatusInternalServerError, "Failed to persist feature flag: "+err.Error())
			return
		}
	}

	SendJSON(ctx, status)
}

type createFlagRequest struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Default     bool   `json:"default"`
	Enabled     *bool  `json:"enabled"`
}

func (h *FeatureFlagsHandler) createFlag(ctx *fasthttp.RequestCtx) {
	var req createFlagRequest
	if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid request payload")
		return
	}

	req.ID = strings.TrimSpace(req.ID)
	if req.ID == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Feature flag id is required")
		return
	}

	if _, exists := featureflags.LookupDef(req.ID); exists {
		SendError(ctx, fasthttp.StatusConflict, "Feature flag is already registered")
		return
	}

	if strings.TrimSpace(req.DisplayName) == "" {
		req.DisplayName = req.ID
	}

	def := featureflags.FlagDef{
		ID:          req.ID,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Default:     req.Default,
	}

	if err := featureflags.Register(def); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid flag ID: must contain lowercase letters, numbers, and .-_ separators (e.g. custom.my_feature)")
		return
	}

	initialEnabled := req.Default
	if req.Enabled != nil {
		initialEnabled = *req.Enabled
	}

	status, err := h.store.Set(ctx, req.ID, initialEnabled)
	if err != nil {
		featureflags.Unregister(req.ID)
		SendError(ctx, fasthttp.StatusInternalServerError, "Failed to initialize feature flag: "+err.Error())
		return
	}

	if h.configStore != nil {
		if err := h.configStore.UpsertFeatureFlag(ctx, req.ID, initialEnabled, status.UpdatedAt); err != nil {
			h.store.Restore(req.ID, featureflags.EntrySnapshot{}, false)
			featureflags.Unregister(req.ID)
			SendError(ctx, fasthttp.StatusInternalServerError, "Failed to persist feature flag: "+err.Error())
			return
		}
	}

	ctx.SetStatusCode(fasthttp.StatusCreated)
	SendJSON(ctx, status)
}

func (h *FeatureFlagsHandler) deleteFlag(ctx *fasthttp.RequestCtx) {
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid flag id")
		return
	}

	priorSnap, _ := h.store.Snapshot(id)
	status, err := h.store.Status(id)
	if err != nil {
		SendError(ctx, fasthttp.StatusNotFound, "Feature flag not found")
		return
	}
	if status.Locked {
		SendError(ctx, fasthttp.StatusConflict, "Cannot delete feature flag locked by config.json / Helm")
		return
	}

	if h.configStore != nil {
		if err := h.configStore.DeleteFeatureFlag(ctx, id); err != nil {
			SendError(ctx, fasthttp.StatusInternalServerError, "Failed to delete feature flag from database: "+err.Error())
			return
		}
	}

	h.store.Restore(id, priorSnap, false)
	featureflags.Unregister(id)

	SendJSON(ctx, map[string]any{"status": "deleted", "id": id})
}

