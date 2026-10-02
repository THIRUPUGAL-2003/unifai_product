package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/valyala/fasthttp"
)

func (h *WorkspaceHandler) scimMiddleware() func(fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
		return func(ctx *fasthttp.RequestCtx) {
			if h.workspace == nil {
				scimError(ctx, fasthttp.StatusServiceUnavailable, "config store is not available")
				return
			}
			token := scimBearerTokenFromRequest(ctx)
			if token == "" {
				scimError(ctx, fasthttp.StatusUnauthorized, "missing scim bearer token")
				return
			}
			expected, enabled, err := h.scimProvisioningToken(ctx)
			if err != nil {
				scimError(ctx, fasthttp.StatusInternalServerError, "failed to load scim config")
				return
			}
			if !enabled || expected == "" {
				scimError(ctx, fasthttp.StatusForbidden, "scim provisioning is disabled")
				return
			}
			if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
				scimError(ctx, fasthttp.StatusUnauthorized, "invalid scim bearer token")
				return
			}
			next(ctx)
		}
	}
}

// scimError writes an RFC 7644 §3.12 error response; IdPs parse status/detail/scimType.
func scimError(ctx *fasthttp.RequestCtx, status int, detail string, scimType ...string) {
	body := map[string]any{
		"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
		"status":  strconv.Itoa(status),
		"detail":  detail,
	}
	if len(scimType) > 0 && scimType[0] != "" {
		body["scimType"] = scimType[0]
	}
	raw, _ := json.Marshal(body)
	ctx.SetStatusCode(status)
	ctx.SetContentType("application/scim+json")
	ctx.SetBody(raw)
}

func scimBearerTokenFromRequest(ctx *fasthttp.RequestCtx) string {
	auth := strings.TrimSpace(string(ctx.Request.Header.Peek("Authorization")))
	if len(auth) >= 7 && strings.EqualFold(auth[:7], "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return strings.TrimSpace(string(ctx.Request.Header.Peek("X-SCIM-Token")))
}

func (h *WorkspaceHandler) scimProvisioningToken(ctx context.Context) (string, bool, error) {
	if h.workspace == nil {
		return "", false, nil
	}
	row, err := h.workspace.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingSCIM)
	if err != nil {
		if err == configstore.ErrNotFound {
			return "", false, nil
		}
		return "", false, err
	}
	var payload scimConfigPayload
	if err := json.Unmarshal([]byte(row.Data), &payload); err != nil {
		return "", false, err
	}
	token := ""
	if payload.BearerToken != "" {
		token = payload.BearerToken
	} else if payload.Config != nil {
		if v, ok := payload.Config["bearer_token"].(string); ok {
			token = v
		} else if v, ok := payload.Config["provisioningToken"].(string); ok {
			token = v
		}
	}
	return token, payload.Enabled, nil
}

func ensureSCIMBearerToken(payload *scimConfigPayload) {
	if payload == nil {
		return
	}
	if payload.BearerToken != "" {
		// Keep the legacy config copy identical so every reader sees the same token.
		if payload.Config == nil {
			payload.Config = map[string]any{}
		}
		payload.Config["bearer_token"] = payload.BearerToken
		return
	}
	if payload.Config != nil {
		if v, ok := payload.Config["bearer_token"].(string); ok && v != "" {
			payload.BearerToken = v
			return
		}
	}
	if payload.Enabled {
		payload.BearerToken = uuid.NewString()
		if payload.Config == nil {
			payload.Config = map[string]any{}
		}
		payload.Config["bearer_token"] = payload.BearerToken
	}
}
