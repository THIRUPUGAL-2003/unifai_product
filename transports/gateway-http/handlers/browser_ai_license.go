package handlers

import (
	"crypto/ed25519"
	"io"
	"os"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/gateway/gateway/framework/logstore"
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

// getServerHardwareID returns the deterministic hardware identifier of this host machine.
func (h *BrowserAIHandler) getServerHardwareID(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	installID := ""
	if h.manager != nil {
		installID, _ = h.manager.GetOrCreateInstallID(ctx)
	}
	SendJSON(ctx, map[string]any{
		"status":             "success",
		"server_hardware_id": logstore.GetServerHardwareID(),
		"install_id":         installID,
	})
}

// licenseAuthorityStatus is served only on the vendor Gateway.
// Client Gateways call it with LICENSE_AUTHORITY_URL. The reply is signed with the vendor private key.
func (h *BrowserAIHandler) licenseAuthorityStatus(ctx *fasthttp.RequestCtx) {
	regPath := strings.TrimSpace(os.Getenv("LICENSE_AUTHORITY_REGISTRY"))
	keyPath := strings.TrimSpace(os.Getenv("LICENSE_AUTHORITY_PRIVATE_KEY"))
	if regPath == "" || keyPath == "" {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.SetContentType("application/json")
		ctx.SetBodyString(`{"error":"license authority is not configured on this server"}`)
		return
	}
	var req struct {
		LicenseID        string `json:"license_id"`
		InstallID        string `json:"install_id"`
		ServerHardwareID string `json:"server_hardware_id"`
		Revision         int    `json:"revision"`
	}
	if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil || strings.TrimSpace(req.LicenseID) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "license_id, install_id, server_hardware_id and revision are required")
		return
	}
	registry, err := os.ReadFile(regPath)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "license registry unreadable")
		return
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "license authority key unreadable")
		return
	}
	priv, err := logstore.ParseLicenseAuthorityPrivateKey(keyPEM)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "license authority key invalid")
		return
	}
	decision := logstore.DecideLicenseAuthority(registry, req.LicenseID, req.InstallID, req.ServerHardwareID, req.Revision)
	payloadB64, sigB64, err := logstore.SignLicenseAuthorityDecision(ed25519.PrivateKey(priv), decision)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "could not sign authority response")
		return
	}
	SendJSON(ctx, map[string]any{
		"payload_b64": payloadB64,
		"signature":   sigB64,
	})
}
