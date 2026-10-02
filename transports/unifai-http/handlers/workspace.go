package handlers

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/fasthttp/router"
	"github.com/google/uuid"
	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/transports/unifai-http/lib"
	"github.com/valyala/fasthttp"
)

// WorkspaceHandler serves workspace feature APIs. Persistence goes through
// configstore.WorkspaceStore; this type only does HTTP.
type WorkspaceHandler struct {
	store             *lib.Config
	workspace         configstore.WorkspaceStore
	governanceManager GovernanceManager
	promptLifecycle   *PromptLifecycleManager
}

func NewWorkspaceHandler(store *lib.Config, governanceManager ...GovernanceManager) *WorkspaceHandler {
	h := &WorkspaceHandler{store: store}
	if store != nil {
		h.workspace, _ = configstore.AsWorkspaceStore(store.ConfigStore)
		if store.ConfigStore != nil {
			h.promptLifecycle = NewPromptLifecycleManager(store.ConfigStore)
		}
	}
	if len(governanceManager) > 0 {
		h.governanceManager = governanceManager[0]
	}
	return h
}

// NewEnterpriseFeaturesHandler is kept so existing call sites compile.
func NewEnterpriseFeaturesHandler(store *lib.Config, governanceManager ...GovernanceManager) *WorkspaceHandler {
	return NewWorkspaceHandler(store, governanceManager...)
}

func (h *WorkspaceHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.UnifAIHTTPMiddleware) {
	wrap := func(fn fasthttp.RequestHandler) fasthttp.RequestHandler {
		return lib.ChainMiddlewares(fn, middlewares...)
	}

	r.GET("/api/circuit-breaker/policies", wrap(h.listCircuitBreakerPolicies))
	r.POST("/api/circuit-breaker/policies", wrap(h.createCircuitBreakerPolicy))
	r.PUT("/api/circuit-breaker/policies/{name}", wrap(h.updateCircuitBreakerPolicy))
	r.DELETE("/api/circuit-breaker/policies/{name}", wrap(h.deleteCircuitBreakerPolicy))
	r.GET("/api/circuit-breaker/state", wrap(h.getCircuitBreakerState))
	r.POST("/api/circuit-breaker/policies/{name}/reset", wrap(h.resetCircuitBreakerPolicy))

	r.GET("/api/access-profiles", wrap(h.listAccessProfiles))
	r.POST("/api/access-profiles", wrap(h.createAccessProfile))
	r.GET("/api/access-profiles/{id}", wrap(h.getAccessProfile))
	r.PUT("/api/access-profiles/{id}", wrap(h.updateAccessProfile))
	r.DELETE("/api/access-profiles/{id}", wrap(h.deleteAccessProfile))
	r.POST("/api/access-profiles/{id}/activate", wrap(h.activateAccessProfile))
	r.POST("/api/access-profiles/{id}/deactivate", wrap(h.deactivateAccessProfile))
	r.POST("/api/access-profiles/{id}/clone", wrap(h.cloneAccessProfile))
	r.GET("/api/users/{target_user_id}/access-profiles", wrap(h.listUserAccessProfiles))

	r.GET("/api/roles", wrap(h.listRoles))
	r.POST("/api/roles", wrap(h.createRole))
	r.GET("/api/roles/{id}", wrap(h.getRole))
	r.PUT("/api/roles/{id}", wrap(h.updateRole))
	r.DELETE("/api/roles/{id}", wrap(h.deleteRole))
	r.GET("/api/roles/{id}/permissions", wrap(h.getRolePermissions))
	r.PUT("/api/roles/{id}/permissions", wrap(h.updateRolePermissions))
	r.GET("/api/resources", wrap(h.listRBACResources))
	r.GET("/api/operations", wrap(h.listRBACOperations))
	r.GET("/api/permissions", wrap(h.listRBACPermissions))
	r.GET("/api/rbac/me/permissions", wrap(h.getMyRBACPermissions))
	r.PUT("/api/users/{id}/role", wrap(h.assignUserRole))

	r.GET("/api/governance/business-units", wrap(h.listBusinessUnits))
	r.POST("/api/governance/business-units", wrap(h.createBusinessUnit))
	r.GET("/api/governance/business-units/{id}", wrap(h.getBusinessUnit))
	r.DELETE("/api/governance/business-units/{id}", wrap(h.deleteBusinessUnit))
	r.GET("/api/governance/business-units/{id}/teams", wrap(h.listBusinessUnitTeams))
	r.POST("/api/governance/business-units/{id}/teams", wrap(h.assignBusinessUnitTeam))
	r.DELETE("/api/governance/business-units/{id}/teams/{team_id}", wrap(h.removeBusinessUnitTeam))
	r.PUT("/api/governance/business-units/{id}/governance", wrap(h.updateBusinessUnitGovernance))

	r.GET("/api/mcp/tool-groups", wrap(h.listMCPToolGroups))
	r.POST("/api/mcp/tool-groups", wrap(h.createMCPToolGroup))
	r.GET("/api/mcp/tool-groups/{id}", wrap(h.getMCPToolGroup))
	r.PUT("/api/mcp/tool-groups/{id}", wrap(h.updateMCPToolGroup))
	r.DELETE("/api/mcp/tool-groups/{id}", wrap(h.deleteMCPToolGroup))

	r.GET("/api/cluster", wrap(h.getClusterConfig))
	r.PUT("/api/cluster", wrap(h.updateClusterConfig))
	r.GET("/api/load-balancer", wrap(h.getLoadBalancerConfig))
	r.PUT("/api/load-balancer", wrap(h.updateLoadBalancerConfig))
	r.GET("/api/load-balancer/routes", wrap(h.getLoadBalancerRoutes))

	r.GET("/api/scim/config", wrap(h.getSCIMConfig))
	r.PUT("/api/scim/config", wrap(h.updateSCIMConfig))
	r.GET("/api/scim/providers", wrap(h.listSCIMProviders))
	// OAuth discovery/logout are auth-middleware whitelisted (browser popup + logout).
	r.GET("/api/scim/oauth/config", lib.ChainMiddlewares(h.getSCIMOAuthConfig, middlewares...))
	r.POST("/api/scim/oauth/callback", lib.ChainMiddlewares(h.scimOAuthCallback, middlewares...))
	r.POST("/api/scim/oauth/refresh", lib.ChainMiddlewares(h.scimOAuthRefresh, middlewares...))
	r.POST("/api/scim/oauth/logout", lib.ChainMiddlewares(h.scimOAuthLogout, middlewares...))

	r.GET("/api/alert-channels", wrap(h.listAlertChannels))
	r.POST("/api/alert-channels", wrap(h.createAlertChannel))
	r.PUT("/api/alert-channels/{id}", wrap(h.updateAlertChannel))
	r.DELETE("/api/alert-channels/{id}", wrap(h.deleteAlertChannel))
	r.POST("/api/alert-channels/{id}/test", wrap(h.testAlertChannel))

	r.GET("/api/audit-logs", wrap(h.listAuditLogs))
	r.GET("/api/audit-logs/export", wrap(h.exportAuditLogs))
	r.GET("/api/audit-logs/settings", wrap(h.getAuditSettings))
	r.PUT("/api/audit-logs/settings", wrap(h.updateAuditSettings))

	r.GET("/api/prompt-deployments", wrap(h.listPromptDeployments))
	r.POST("/api/prompt-deployments", wrap(h.createPromptDeployment))
	r.PUT("/api/prompt-deployments/{id}", wrap(h.updatePromptDeployment))
	r.DELETE("/api/prompt-deployments/{id}", wrap(h.deletePromptDeployment))

	r.GET("/api/connectors", wrap(h.listConnectors))
	r.GET("/api/connectors/{name}", wrap(h.getConnector))
	r.PUT("/api/connectors/{name}", wrap(h.updateConnector))
	r.DELETE("/api/connectors/{name}", wrap(h.deleteConnector))
	r.POST("/api/connectors/{name}/test", wrap(h.testConnector))

	// Cluster peer KV replication (no dashboard RBAC — peers authenticate via header).
	r.POST("/internal/cluster/kv", h.clusterKVReplicate)

	scim := h.scimMiddleware()
	r.GET("/scim/v2/ServiceProviderConfig", scim(h.scimServiceProviderConfig))
	r.GET("/scim/v2/Schemas", scim(h.scimSchemas))
	r.GET("/scim/v2/Users", scim(h.scimListUsers))
	r.POST("/scim/v2/Users", scim(h.scimCreateUser))
	r.GET("/scim/v2/Users/{id}", scim(h.scimGetUser))
	r.PUT("/scim/v2/Users/{id}", scim(h.scimPutUser))
	r.PATCH("/scim/v2/Users/{id}", scim(h.scimPatchUser))
	r.DELETE("/scim/v2/Users/{id}", scim(h.scimDeleteUser))

	r.GET("/scim/v2/Groups", scim(h.scimListGroups))
	r.POST("/scim/v2/Groups", scim(h.scimCreateGroup))
	r.GET("/scim/v2/Groups/{id}", scim(h.scimGetGroup))
	r.PUT("/scim/v2/Groups/{id}", scim(h.scimPutGroup))
	r.PATCH("/scim/v2/Groups/{id}", scim(h.scimPatchGroup))
	r.DELETE("/scim/v2/Groups/{id}", scim(h.scimDeleteGroup))
}

func (h *WorkspaceHandler) requireStore(ctx *fasthttp.RequestCtx) configstore.WorkspaceStore {
	if h.workspace == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
		return nil
	}
	return h.workspace
}

func (h *WorkspaceHandler) sessionActor(ctx *fasthttp.RequestCtx) string {
	if h.store == nil || h.store.ConfigStore == nil {
		return "system"
	}
	return auditInitiator(h.store.ConfigStore, ctx)
}

func pathID(ctx *fasthttp.RequestCtx, name string) string {
	value := ctx.UserValue(name)
	if value == nil {
		return ""
	}
	var raw string
	switch typed := value.(type) {
	case string:
		raw = typed
	case []byte:
		raw = string(typed)
	default:
		raw = fmt.Sprint(typed)
	}
	// Routers may leave %20 (etc.) encoded; decode so names with spaces match DB rows
	// (e.g. circuit-breaker policies "Batch lane breaker").
	if decoded, err := url.PathUnescape(raw); err == nil {
		return decoded
	}
	return raw
}

func pathUint(ctx *fasthttp.RequestCtx, name string) (uint, bool) {
	raw := pathID(ctx, name)
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(n), true
}

func queryInt(ctx *fasthttp.RequestCtx, name string, fallback int) int {
	raw := string(ctx.QueryArgs().Peek(name))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func newEntityID() string {
	return uuid.NewString()
}

func isStoreNotFound(err error) bool {
	return errors.Is(err, configstore.ErrNotFound)
}

func specStringSlice(spec map[string]any, key string) []string {
	raw, _ := spec[key].([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		if typed, ok := spec[key].([]string); ok {
			return typed
		}
	}
	return out
}

func specUintSlice(spec map[string]any, key string) []uint {
	raw, ok := spec[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []uint:
		return typed
	case []any:
		out := make([]uint, 0, len(typed))
		for _, item := range typed {
			switch n := item.(type) {
			case float64:
				out = append(out, uint(n))
			case int:
				out = append(out, uint(n))
			case uint:
				out = append(out, n)
			}
		}
		return out
	default:
		return nil
	}
}

func specMapSlice(spec map[string]any, key string) []map[string]any {
	raw, ok := spec[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []map[string]any:
		return typed
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func specMap(spec map[string]any, key string) map[string]any {
	raw, _ := spec[key].(map[string]any)
	return raw
}
