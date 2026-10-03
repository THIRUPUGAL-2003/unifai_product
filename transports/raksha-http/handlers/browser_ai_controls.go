package handlers

import (
	"context"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/raksha/raksha/framework/logstore"
	"github.com/valyala/fasthttp"
)

var (
	browserAILogRetentionOnce sync.Once
	browserAILogRetentionStop chan struct{}
)

// startBrowserAILogRetentionCleanup runs background auto-delete for Prompt Logs
// and Search Logs when retention controls are enabled (1d|7d|30d|90d|180d|365d).
func startBrowserAILogRetentionCleanup(manager *logstore.BrowserAIManager) {
	if manager == nil {
		return
	}
	browserAILogRetentionOnce.Do(func() {
		browserAILogRetentionStop = make(chan struct{})
		go func() {
			// Initial sweep shortly after boot so stale rows do not wait for UI traffic.
			runBrowserAILogRetentionSweep(manager)
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					runBrowserAILogRetentionSweep(manager)
				case <-browserAILogRetentionStop:
					return
				}
			}
		}()
	})
}

func runBrowserAILogRetentionSweep(manager *logstore.BrowserAIManager) {
	if manager == nil || manager.GetDB() == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if cutoff := manager.ApplySearchLogAutoDelete(ctx); cutoff != nil {
		purgeInMemorySearchLogsBefore(*cutoff)
	}
	_ = manager.ApplyPromptLogAutoDelete(ctx)
}

func (h *BrowserAIHandler) getControls(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	ctrl, err := h.manager.GetControls(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"controls": ctrl})
}

func (h *BrowserAIHandler) updateControls(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	var updates map[string]any
	if err := sonic.Unmarshal(ctx.PostBody(), &updates); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}
	ctrl, err := h.manager.UpdateControls(ctx, updates)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	if ctrl != nil && ctrl.SearchLogAutoDelete {
		if cutoff := h.manager.ApplySearchLogAutoDelete(ctx); cutoff != nil {
			purgeInMemorySearchLogsBefore(*cutoff)
		}
	}
	if ctrl != nil && ctrl.PromptLogAutoDelete {
		h.manager.ApplyPromptLogAutoDelete(ctx)
	}
	SendJSON(ctx, map[string]any{"status": "success", "controls": ctrl})
}
