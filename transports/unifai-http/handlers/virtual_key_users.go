package handlers

import (
	"encoding/json"

	"github.com/unifai/unifai/framework/configstore"
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
	links, err := ws.ListVirtualKeyUsers(ctx, vkID)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list virtual key users")
		return
	}
	users := make([]map[string]any, 0, len(links))
	for _, link := range links {
		user, err := h.configStore.GetUserByID(ctx, link.UserID)
		if err != nil || user == nil {
			continue
		}
		users = append(users, map[string]any{
			"id": user.ID, "name": user.Username, "email": user.Email,
			"role": user.Role, "created_at": user.CreatedAt, "updated_at": user.UpdatedAt,
		})
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
