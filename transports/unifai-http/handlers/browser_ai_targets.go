package handlers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/unifai/unifai/framework/logstore"
	"github.com/valyala/fasthttp"
)

func safeCtx(ctx *fasthttp.RequestCtx) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return context.Background()
}

func (h *BrowserAIHandler) getTargets(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	targets, err := h.manager.GetTargets(safeCtx(ctx))
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	forAgent := strings.EqualFold(string(ctx.QueryArgs().Peek("for")), "agent")
	if forAgent {
		out := make([]map[string]any, 0, len(targets))
		for _, t := range targets {
			out = append(out, map[string]any{
				"id":            t.ID,
				"domain":        t.Domain,
				"platform_name": t.PlatformName,
				"monitored":     t.Monitored,
				"block_site":    t.BlockSite,
				"parent_id":     t.ParentID,
				"host_role":     t.HostRole,
			})
		}
		SendJSON(ctx, map[string]any{"targets": out})
		return
	}
	SendJSON(ctx, map[string]any{"targets": targets})
}

func (h *BrowserAIHandler) createTarget(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	// Check if this is a bulk import payload: {"targets": [...], "overwrite": bool}
	var peek map[string]any
	if err := sonic.Unmarshal(ctx.PostBody(), &peek); err == nil {
		if _, ok := peek["targets"]; ok {
			h.importTargets(ctx)
			return
		}
	}

	var target logstore.BrowserTargetWebsite
	if err := sonic.Unmarshal(ctx.PostBody(), &target); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.manager.CreateTarget(safeCtx(ctx), &target); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"status": "success", "target": target})
}

func isReservedBrowserAITargetPathID(id string) bool {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "import":
		return true
	default:
		return false
	}
}

func (h *BrowserAIHandler) updateTarget(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Missing target ID")
		return
	}
	if isReservedBrowserAITargetPathID(id) {
		SendError(ctx, fasthttp.StatusMethodNotAllowed, "Use POST /api/browser-ai/targets/import for bulk import")
		return
	}
	var updates map[string]any
	if err := sonic.Unmarshal(ctx.PostBody(), &updates); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.manager.UpdateTarget(safeCtx(ctx), id, updates); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"status": "success"})
}

func (h *BrowserAIHandler) deleteTarget(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	id, ok := ctx.UserValue("id").(string)
	if !ok || id == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "Missing target ID")
		return
	}
	if isReservedBrowserAITargetPathID(id) {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid target ID")
		return
	}
	if err := h.manager.DeleteTarget(safeCtx(ctx), id); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	SendJSON(ctx, map[string]any{"status": "success"})
}

type importTargetsPayload struct {
	Targets   []importTargetItem `json:"targets"`
	Overwrite bool               `json:"overwrite"`
}

type importTargetItem struct {
	Domain       string `json:"domain"`
	PlatformName string `json:"platform_name"`
	HostRole     string `json:"host_role"`
	Monitored    *bool  `json:"monitored"`
	BlockSite    *bool  `json:"block_site"`
	Status       string `json:"status"`
}

func (h *BrowserAIHandler) importTargets(ctx *fasthttp.RequestCtx) {
	h.ensureDB(ctx)
	var payload importTargetsPayload
	if err := sonic.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if len(payload.Targets) == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "No targets provided for import")
		return
	}

	existingTargets, err := h.manager.GetTargets(safeCtx(ctx))
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	existingByDomain := make(map[string]logstore.BrowserTargetWebsite, len(existingTargets))
	hasChildren := map[string]bool{}
	for _, t := range existingTargets {
		existingByDomain[strings.ToLower(strings.TrimSpace(t.Domain))] = t
		if pid := strings.TrimSpace(t.ParentID); pid != "" {
			hasChildren[pid] = true
		}
	}

	imported := 0
	updated := 0
	skipped := 0
	alreadyExists := 0
	duplicates := 0
	nested := 0
	failed := 0
	var errorDetails []string
	seenInFile := map[string]bool{}

	// Parents before children: main (ui) hosts first, then fewer labels, so "ab.chatgpt.com"
	// and ChatGPT's chat/file hosts can nest under "chatgpt.com" from the same file.
	order := make([]int, len(payload.Targets))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		ta, tb := payload.Targets[order[a]], payload.Targets[order[b]]
		ua := logstore.NormalizeHostRole(ta.HostRole) == "ui"
		ub := logstore.NormalizeHostRole(tb.HostRole) == "ui"
		if ua != ub {
			return ua
		}
		return strings.Count(logstore.NormalizeDomain(ta.Domain), ".") < strings.Count(logstore.NormalizeDomain(tb.Domain), ".")
	})

	for _, i := range order {
		item := payload.Targets[i]
		rawDomain := strings.TrimSpace(item.Domain)
		if rawDomain == "" {
			skipped++
			errorDetails = append(errorDetails, fmt.Sprintf("Row %d: Missing domain", i+1))
			continue
		}

		domain := logstore.NormalizeDomain(rawDomain)
		if domain == "" {
			skipped++
			errorDetails = append(errorDetails, fmt.Sprintf("Row %d (%s): Invalid domain format", i+1, rawDomain))
			continue
		}

		platformName := strings.TrimSpace(item.PlatformName)
		if platformName == "" {
			platformName = domain
		}

		hostRole := logstore.NormalizeHostRole(item.HostRole)

		monitored := true
		if item.Monitored != nil {
			monitored = *item.Monitored
		}

		blockSite := false
		if item.BlockSite != nil {
			blockSite = *item.BlockSite
		}

		status := "MONITORED"
		if blockSite {
			status = "BLOCKED"
		} else if !monitored {
			status = "PAUSED"
		}

		domainKey := strings.ToLower(domain)
		if seenInFile[domainKey] {
			skipped++
			duplicates++
			errorDetails = append(errorDetails, fmt.Sprintf("Row %d (%s): Duplicate domain in the file — skipped", i+1, domain))
			continue
		}
		seenInFile[domainKey] = true

		if existing, exists := existingByDomain[domainKey]; exists {
			// A row imported before nesting existed may still sit at the top level; attach it now.
			relinkTo := ""
			if strings.TrimSpace(existing.ParentID) == "" && !hasChildren[existing.ID] {
				role := hostRole
				if !payload.Overwrite {
					role = existing.HostRole
				}
				relinkTo = importParentID(domainKey, platformName, role, existingByDomain)
			}
			if payload.Overwrite {
				updateFields := map[string]any{
					"platform_name": platformName,
					"host_role":     hostRole,
					"monitored":     monitored,
					"block_site":    blockSite,
					"status":        status,
				}
				if relinkTo != "" {
					updateFields["parent_id"] = relinkTo
				}
				if err := h.manager.UpdateTarget(safeCtx(ctx), existing.ID, updateFields); err != nil {
					failed++
					errorDetails = append(errorDetails, fmt.Sprintf("Failed to update '%s': %v", domain, err))
					continue
				}
				updated++
			} else {
				skipped++
				alreadyExists++
				if relinkTo != "" {
					if err := h.manager.UpdateTarget(safeCtx(ctx), existing.ID, map[string]any{"parent_id": relinkTo}); err != nil {
						errorDetails = append(errorDetails, fmt.Sprintf("'%s' already exists but could not be moved under its main domain: %v", domain, err))
						relinkTo = ""
					}
				}
			}
			if relinkTo != "" {
				nested++
				existing.ParentID = relinkTo
				existingByDomain[domainKey] = existing
				hasChildren[relinkTo] = true
			}
		} else {
			newTarget := logstore.BrowserTargetWebsite{
				Domain:       domain,
				PlatformName: platformName,
				HostRole:     hostRole,
				Monitored:    monitored,
				BlockSite:    blockSite,
				Status:       status,
				ParentID:     importParentID(domainKey, platformName, hostRole, existingByDomain),
			}
			if err := h.manager.CreateTarget(safeCtx(ctx), &newTarget); err != nil {
				failed++
				errorDetails = append(errorDetails, fmt.Sprintf("Failed to create '%s': %v", domain, err))
				continue
			}
			// CreateTarget always starts top-level sites monitored; honour "Monitored = FALSE" rows.
			if newTarget.ParentID == "" && !monitored && newTarget.Monitored {
				pause := map[string]any{"monitored": false}
				if !newTarget.BlockSite {
					pause["status"] = "PAUSED"
				}
				if err := h.manager.UpdateTarget(safeCtx(ctx), newTarget.ID, pause); err != nil {
					errorDetails = append(errorDetails, fmt.Sprintf("Imported '%s' but could not pause it: %v", domain, err))
				} else {
					newTarget.Monitored = false
					if !newTarget.BlockSite {
						newTarget.Status = "PAUSED"
					}
				}
			}
			existingByDomain[domainKey] = newTarget
			if newTarget.ParentID != "" {
				hasChildren[newTarget.ParentID] = true
			}
			imported++
		}
	}

	resp := map[string]any{
		"status":             "success",
		"imported":           imported,
		"updated":            updated,
		"skipped":            skipped,
		"already_exists":     alreadyExists,
		"duplicates_in_file": duplicates,
		"nested":             nested,
		"failed":             failed,
		"total":              len(payload.Targets),
	}
	if len(errorDetails) > 0 {
		resp["errors"] = errorDetails
	}
	SendJSON(ctx, resp)
}

// importParentID picks the row a host should nest under (same grouping as the Targets table),
// so pausing / blocking the parent also covers it:
//  1. the closest listed parent domain ("ab.chatgpt.com" → "chatgpt.com");
//  2. otherwise, for a non-main host (chat / file role), the main (ui) site with the same
//     Platform Name ("chat.openai.com", ChatGPT → "chatgpt.com").
func importParentID(domain, platformName, hostRole string, byDomain map[string]logstore.BrowserTargetWebsite) string {
	rootOf := func(t logstore.BrowserTargetWebsite) string {
		if pid := strings.TrimSpace(t.ParentID); pid != "" {
			return pid
		}
		return t.ID
	}
	best := ""
	bestLen := 0
	for d, t := range byDomain {
		if d == "" || d == domain || !strings.HasSuffix(domain, "."+d) || len(d) <= bestLen {
			continue
		}
		best = rootOf(t)
		bestLen = len(d)
	}
	if best != "" {
		return best
	}

	platform := strings.ToLower(strings.TrimSpace(platformName))
	if platform == "" || platform == domain || logstore.NormalizeHostRole(hostRole) == "ui" {
		return ""
	}
	bestDomain := ""
	for d, t := range byDomain {
		if d == "" || d == domain || strings.TrimSpace(t.ParentID) != "" || t.HostRole != "ui" {
			continue
		}
		if strings.ToLower(strings.TrimSpace(t.PlatformName)) != platform {
			continue
		}
		if bestDomain == "" || len(d) < len(bestDomain) || (len(d) == len(bestDomain) && d < bestDomain) {
			best, bestDomain = t.ID, d
		}
	}
	return best
}
