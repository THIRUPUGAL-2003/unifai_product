package handlers

import (
	"io"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/valyala/fasthttp"
)

// getLicense returns active enterprise on-premise license details and seat usage.
func (h *BrowserAIHandler) getLicense(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	if h.manager == nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "manager not initialized")
		return
	}

	info, err := h.manager.GetActiveLicense(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}

	SendJSON(ctx, map[string]any{
		"status":  "success",
		"license": info,
	})
}

// activateLicense verifies the Ed25519 digital signature of an uploaded license and activates it.
func (h *BrowserAIHandler) activateLicense(ctx *fasthttp.RequestCtx) {
	if !h.requireGuardAdmin(ctx, "Admin role required to activate enterprise license") {
		return
	}
	h.ensureDB(ctx)

	// Can receive either raw license text, JSON envelope, or multipart form file
	var rawData []byte

	// 1. Check if multipart file upload
	if fileHeader, err := ctx.FormFile("license_file"); err == nil && fileHeader != nil {
		file, fErr := fileHeader.Open()
		if fErr == nil {
			buf, readErr := io.ReadAll(file)
			_ = file.Close()
			if readErr == nil {
				rawData = buf
			}
		}
	}

	// 2. Check JSON payload { "license_key": "..." } or raw string
	if len(rawData) == 0 {
		body := ctx.PostBody()
		var req struct {
			LicenseKey string `json:"license_key"`
			UpdatedBy  string `json:"updated_by"`
		}
		if sonic.Unmarshal(body, &req) == nil && strings.TrimSpace(req.LicenseKey) != "" {
			rawData = []byte(req.LicenseKey)
		} else {
			rawData = body
		}
	}

	if len(strings.TrimSpace(string(rawData))) == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "No license data provided. Upload a .lic file or paste the license key.")
		return
	}

	updatedBy := "admin"
	if u := string(ctx.Request.Header.Peek("X-User-Email")); u != "" {
		updatedBy = u
	}

	info, err := h.manager.ActivateLicense(ctx, rawData, updatedBy)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "License verification failed: "+err.Error())
		return
	}

	SendJSON(ctx, map[string]any{
		"status":  "success",
		"message": "Enterprise license activated successfully!",
		"license": info,
	})
}
