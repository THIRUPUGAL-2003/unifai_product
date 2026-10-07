package handlers

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/gateway/gateway/framework/logstore"
	"github.com/valyala/fasthttp"
)

// getProxyPAC serves a PAC built only from monitored Target Websites (no hardcoded defaults).
// Product / VAPT: ignore client ?proxy= unless GATEWAY_PAC_ALLOW_QUERY_PROXY=1 (fleet/env only by default).
func (h *BrowserAIHandler) getProxyPAC(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	allowQuery := strings.TrimSpace(gatewayEnv("PAC_ALLOW_QUERY_PROXY"))
	allowQueryOn := allowQuery == "1" || strings.EqualFold(allowQuery, "true") || strings.EqualFold(allowQuery, "yes")

	queryProxy := strings.TrimSpace(string(ctx.QueryArgs().Peek("proxy")))
	proxyAddr := ""
	if allowQueryOn {
		proxyAddr = queryProxy
	} else if loopbackPACProxy(queryProxy) {
		// A laptop Guard asks for its own local listener; honoring it keeps
		// Guards on different installer versions (and ports) working.
		proxyAddr = queryProxy
	}
	if strings.TrimSpace(proxyAddr) == "" {
		if fleet, err := h.manager.GetFleetConfig(ctx); err == nil && fleet != nil {
			if v := strings.TrimSpace(fleet.PacAdvertiseAddr); v != "" {
				proxyAddr = v
			} else if v := strings.TrimSpace(fleet.DefaultProxyAddr); v != "" {
				proxyAddr = v
			}
		}
	}
	if strings.TrimSpace(proxyAddr) == "" {
		if v := strings.TrimSpace(gatewayEnv("PROXY_ADDR")); v != "" {
			proxyAddr = v
		} else if port := strings.TrimSpace(os.Getenv("PROXY_PORT")); port != "" {
			proxyAddr = "127.0.0.1:" + port
		}
	}
	pac, _ := h.manager.BuildProxyPAC(context.Background(), proxyAddr)
	if strings.TrimSpace(pac) == "" {
		pac, _ = h.manager.BuildProxyPAC(context.Background(), proxyAddr)
	}
	ctx.Response.Header.Set("Content-Type", "application/x-ns-proxy-autoconfig; charset=utf-8")
	ctx.Response.Header.Set("Cache-Control", "no-store, no-cache, must-revalidate")
	ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBodyString(pac)
}

// loopbackPACProxy reports whether addr is 127.0.0.1/localhost/[::1] with a valid port.
// Such a PAC can only point a browser at its own machine, so it is safe to accept from the query.
func loopbackPACProxy(addr string) bool {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return false
	}
	return host == "127.0.0.1" || host == "::1" || strings.EqualFold(host, "localhost")
}

func (h *BrowserAIHandler) getFleetConfig(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	fleet, err := h.manager.GetFleetConfig(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"fleet_config": fleet})
}

func (h *BrowserAIHandler) putFleetConfig(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	var body logstore.BrowserGuardFleetConfig
	if err := sonic.Unmarshal(ctx.PostBody(), &body); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}
	saved, err := h.manager.SaveFleetConfig(ctx, &body)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"status": "success", "fleet_config": saved})
}
