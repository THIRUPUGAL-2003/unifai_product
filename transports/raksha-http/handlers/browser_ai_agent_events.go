package handlers

import (
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/valyala/fasthttp"
)

const (
	guardEventUninstall = "uninstall"
	guardEventRebuild   = "rebuild"

	// guardCommandWait stays under common reverse-proxy read timeouts (60s).
	guardCommandWait = 25 * time.Second
)

// guardEventHub wakes Guards parked on /agents/wait-command so admin actions (remote
// uninstall, Rebuild & Publish) reach laptops in about a second instead of on the next
// 30s heartbeat. Heartbeat stays the source of truth; an event only triggers one early.
type guardEventHub struct {
	mu      sync.Mutex
	waiters map[string]map[chan string]struct{}
}

var guardEvents = &guardEventHub{waiters: map[string]map[chan string]struct{}{}}

func (g *guardEventHub) subscribe(agentID string) chan string {
	ch := make(chan string, 1)
	g.mu.Lock()
	if g.waiters[agentID] == nil {
		g.waiters[agentID] = map[chan string]struct{}{}
	}
	g.waiters[agentID][ch] = struct{}{}
	g.mu.Unlock()
	return ch
}

func (g *guardEventHub) unsubscribe(agentID string, ch chan string) {
	g.mu.Lock()
	if set := g.waiters[agentID]; set != nil {
		delete(set, ch)
		if len(set) == 0 {
			delete(g.waiters, agentID)
		}
	}
	g.mu.Unlock()
}

func (g *guardEventHub) notify(agentID, event string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for ch := range g.waiters[agentID] {
		select {
		case ch <- event:
		default:
		}
	}
}

func (g *guardEventHub) broadcast(event string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, set := range g.waiters {
		for ch := range set {
			select {
			case ch <- event:
			default:
			}
		}
	}
}

// agentWaitCommand long-polls until an admin action targets this Guard (or the fleet),
// returning {"event": "uninstall"|"rebuild"} — or {"event": ""} after guardCommandWait.
func (h *BrowserAIHandler) agentWaitCommand(ctx *fasthttp.RequestCtx) {
	if !h.verifyGuardSecurity(ctx) {
		return
	}
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := sonic.Unmarshal(ctx.PostBody(), &req); err != nil || strings.TrimSpace(req.AgentID) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "agent_id is required")
		return
	}
	agentID := strings.TrimSpace(req.AgentID)

	ch := guardEvents.subscribe(agentID)
	defer guardEvents.unsubscribe(agentID, ch)

	h.ensureDB(ctx)
	if h.manager != nil {
		if agent, err := h.manager.GetAgent(ctx, agentID); err == nil && remoteUninstallAuthorized(agent, time.Now()) {
			SendJSON(ctx, map[string]any{"event": guardEventUninstall})
			return
		}
	}

	timer := time.NewTimer(guardCommandWait)
	defer timer.Stop()
	select {
	case event := <-ch:
		SendJSON(ctx, map[string]any{"event": event})
	case <-timer.C:
		SendJSON(ctx, map[string]any{"event": ""})
	case <-ctx.Done():
	}
}
