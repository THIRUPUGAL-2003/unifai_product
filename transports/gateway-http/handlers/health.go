package handlers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/router"
	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/transports/gateway-http/lib"
	"github.com/valyala/fasthttp"
)

// HealthHandler manages HTTP requests for health checks.
type HealthHandler struct {
	config *lib.Config
}

// NewHealthHandler creates a new health handler instance.
func NewHealthHandler(config *lib.Config) *HealthHandler {
	return &HealthHandler{
		config: config,
	}
}

// RegisterRoutes registers the health-related routes.
func (h *HealthHandler) RegisterRoutes(r *router.Router, middlewares ...schemas.GatewayHTTPMiddleware) {
	r.GET("/health", lib.ChainMiddlewares(h.getHealth, middlewares...))
	r.GET("/api/health", lib.ChainMiddlewares(h.getHealth, middlewares...))
	r.GET("/api/governance/debug/health", lib.ChainMiddlewares(h.getHealth, middlewares...))
	r.GET("/api/branding", lib.ChainMiddlewares(h.getBranding, middlewares...))
	r.GET("/api/system/branding", lib.ChainMiddlewares(h.getBranding, middlewares...))
}

// getHealth handles GET /api/health - Get the health status of the server.
func (h *HealthHandler) getHealth(ctx *fasthttp.RequestCtx) {
	// If DB pings are disabled, just return OK
	if h.config.ClientConfig.DisableDBPingsInHealth {
		SendJSON(ctx, map[string]any{"status": "ok", "components": map[string]any{"db_pings": "disabled"}})
		return
	}
	// Pinging config store
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var errors []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	if h.config.ConfigStore != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.config.ConfigStore.Ping(reqCtx); err != nil {
				mu.Lock()
				errors = append(errors, "config store not available")
				mu.Unlock()
			}
		}()
	}

	// Pinging log store
	if h.config.LogsStore != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.config.LogsStore.Ping(reqCtx); err != nil {
				mu.Lock()
				errors = append(errors, "log store not available")
				mu.Unlock()
			}
		}()
	}

	// Pinging vector store
	if h.config.VectorStore != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.config.VectorStore.Ping(reqCtx); err != nil {
				mu.Lock()
				errors = append(errors, "vector store not available")
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if len(errors) > 0 {
		SendError(ctx, fasthttp.StatusServiceUnavailable, errors[0])
		return
	}
	SendJSON(ctx, map[string]any{
		"status":     "healthy",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"components": map[string]any{"db_pings": "ok"},
		"checks": map[string]any{
			"database": map[string]string{"status": "healthy"},
		},
	})
}

// getBranding returns live branding configuration loaded from environment variables.
func (h *HealthHandler) getBranding(ctx *fasthttp.RequestCtx) {
	// Home-page brand mark (operator product name). In-app / email copy uses product_name.
	brandName := strings.TrimSpace(gatewayEnv("BRAND_NAME"))
	if brandName == "" {
		brandName = "Gateway"
	}
	prodName := strings.TrimSpace(gatewayEnv("PRODUCT_NAME"))
	if prodName == "" {
		prodName = "Gateway"
	}
	prodSubtitle := strings.TrimSpace(gatewayEnv("PRODUCT_SUBTITLE"))
	if prodSubtitle == "" {
		prodSubtitle = "Real-time AI Knowledge Screening & Hazard Audit"
	}
	companyName := strings.TrimSpace(gatewayEnv("COMPANY_NAME"))
	if companyName == "" {
		companyName = "YesPanchi Group of Companies"
	}
	companyShortName := strings.TrimSpace(gatewayEnv("COMPANY_SHORT_NAME"))
	if companyShortName == "" {
		parts := strings.Fields(companyName)
		if len(parts) > 0 {
			companyShortName = parts[0]
		} else {
			companyShortName = "YesPanchi"
		}
	}
	companyLogo := strings.TrimSpace(gatewayEnv("COMPANY_LOGO"))
	if companyLogo == "" {
		companyLogo = "/yes-panchi-logo.png"
	}
	footerCopyright := strings.TrimSpace(gatewayEnv("FOOTER_COPYRIGHT"))
	footerSubtitle := strings.TrimSpace(gatewayEnv("FOOTER_SUBTITLE"))
	if footerSubtitle == "" {
		footerSubtitle = "Gateway - Real-time AI Knowledge Screening & Hazard Audit"
	}
	if footerCopyright == "" {
		footerCopyright = fmt.Sprintf("© %d %s. All rights reserved. %s", time.Now().Year(), companyName, footerSubtitle)
	}

	SendJSON(ctx, map[string]any{
		"brand_name":         brandName,
		"product_name":       prodName,
		"product_subtitle":   prodSubtitle,
		"company_name":       companyName,
		"company_short_name": companyShortName,
		"company_logo":       companyLogo,
		"footer_copyright":   footerCopyright,
		"footer_subtitle":    footerSubtitle,
	})
}

