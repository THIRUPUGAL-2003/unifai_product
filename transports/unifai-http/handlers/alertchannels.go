package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/unifai/unifai/framework/alerts"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

type alertChannelPayload struct {
	ID        uint           `json:"id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Enabled   bool           `json:"enabled"`
	Config    map[string]any `json:"config"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

const alertSecretPlaceholder = "<redacted>"

// Webhook URLs (Slack/generic) embed their auth token, so they are secrets like routing keys.
var alertSecretKeys = []string{"url", "webhook_url", "routing_key", "integration_key"}

func alertChannelFromRow(row tables.TableAlertChannel) alertChannelPayload {
	cfg := make(map[string]any, len(row.ParsedConfig))
	for k, v := range row.ParsedConfig {
		cfg[k] = v
	}
	for _, k := range alertSecretKeys {
		if s, ok := cfg[k].(string); ok && s != "" {
			cfg[k] = alertSecretPlaceholder
		}
	}
	return alertChannelPayload{
		ID: row.ID, Name: row.Name, Type: row.Type, Enabled: row.Enabled,
		Config: cfg, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (h *WorkspaceHandler) listAlertChannels(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	rows, err := store.ListAlertChannels(ctx)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to list alert channels")
		return
	}
	items := make([]alertChannelPayload, 0, len(rows))
	for _, row := range rows {
		items = append(items, alertChannelFromRow(row))
	}
	SendJSON(ctx, map[string]any{"channels": items, "count": len(items)})
}

func (h *WorkspaceHandler) createAlertChannel(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var payload alertChannelPayload
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.Type) == "" {
		SendError(ctx, fasthttp.StatusBadRequest, "name and type are required")
		return
	}
	switch payload.Type {
	case "webhook", "slack", "email", "pagerduty":
	default:
		SendError(ctx, fasthttp.StatusBadRequest, "type must be webhook, slack, email, or pagerduty")
		return
	}
	now := time.Now().UTC()
	if payload.Config == nil {
		payload.Config = map[string]any{}
	}
	row := tables.TableAlertChannel{
		Name: payload.Name, Type: payload.Type, Enabled: payload.Enabled,
		ParsedConfig: payload.Config, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateAlertChannel(ctx, &row); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save alert channel")
		return
	}
	SendJSONWithStatus(ctx, alertChannelFromRow(row), fasthttp.StatusCreated)
}

func (h *WorkspaceHandler) updateAlertChannel(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid alert channel id")
		return
	}
	existing, err := store.GetAlertChannel(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "alert channel not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load alert channel")
		return
	}
	var patch alertChannelPayload
	if err := json.Unmarshal(ctx.PostBody(), &patch); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	var present map[string]json.RawMessage
	_ = json.Unmarshal(ctx.PostBody(), &present)
	if patch.Name != "" {
		existing.Name = patch.Name
	}
	if patch.Type != "" {
		switch patch.Type {
		case "webhook", "slack", "email", "pagerduty":
		default:
			SendError(ctx, fasthttp.StatusBadRequest, "type must be webhook, slack, email, or pagerduty")
			return
		}
		existing.Type = patch.Type
	}
	if patch.Config != nil {
		for k, v := range patch.Config {
			if s, ok := v.(string); ok && s == alertSecretPlaceholder {
				if prev, had := existing.ParsedConfig[k]; had {
					patch.Config[k] = prev
				} else {
					delete(patch.Config, k)
				}
			}
		}
		existing.ParsedConfig = patch.Config
	}
	if _, ok := present["enabled"]; ok {
		existing.Enabled = patch.Enabled
	}
	existing.UpdatedAt = time.Now().UTC()
	if err := store.UpdateAlertChannel(ctx, existing); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to update alert channel")
		return
	}
	SendJSON(ctx, alertChannelFromRow(*existing))
}

func (h *WorkspaceHandler) deleteAlertChannel(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid alert channel id")
		return
	}
	if err := store.DeleteAlertChannel(ctx, id); isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "alert channel not found")
		return
	} else if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to delete alert channel")
		return
	}
	SendJSON(ctx, map[string]string{"message": "deleted"})
}

func (h *WorkspaceHandler) testAlertChannel(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	id, ok := pathUint(ctx, "id")
	if !ok {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid alert channel id")
		return
	}
	row, err := store.GetAlertChannel(ctx, id)
	if isStoreNotFound(err) {
		SendError(ctx, fasthttp.StatusNotFound, "alert channel not found")
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load alert channel")
		return
	}
	var cs configstore.ConfigStore
	if h.store != nil {
		cs = h.store.ConfigStore
	}
	if err := sendAlertChannelTest(ctx, cs, row); err != nil {
		SendJSON(ctx, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("test to %s channel %q failed: %v", row.Type, row.Name, err),
		})
		return
	}
	SendJSON(ctx, map[string]any{
		"ok":      true,
		"message": fmt.Sprintf("test alert delivered to %s channel %q", row.Type, row.Name),
	})
}

func alertConfigString(cfg map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := cfg[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// sendAlertChannelTest delivers a real test message so "Test" proves the channel works.
func sendAlertChannelTest(ctx *fasthttp.RequestCtx, store configstore.ConfigStore, row *tables.TableAlertChannel) error {
	event := alerts.Event{
		Kind:     "test",
		Severity: alerts.SeverityInfo,
		Title:    "RAKSHA test alert",
		Message:  fmt.Sprintf("Test alert from channel %q.", row.Name),
		Time:     time.Now().UTC(),
	}
	err := deliverAlert(ctx, store, row, event)
	if row.Type == "email" {
		recordEmailAudit(store, ctx, alertConfigString(row.ParsedConfig, "address", "email", "to"), "RAKSHA test alert", err)
	}
	return err
}

func alertText(e alerts.Event) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %s\n%s\n", strings.ToUpper(e.Severity), e.Title, e.Message)
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v := e.Fields[k]; v != "" {
			fmt.Fprintf(&b, "%s: %s\n", k, v)
		}
	}
	fmt.Fprintf(&b, "time: %s", e.Time.UTC().Format(time.RFC3339))
	return b.String()
}

func pagerDutySeverity(sev string) string {
	switch sev {
	case alerts.SeverityCritical:
		return "critical"
	case alerts.SeverityWarning:
		return "warning"
	default:
		return "info"
	}
}

// deliverAlert sends one event to one channel.
func deliverAlert(ctx context.Context, store configstore.ConfigStore, row *tables.TableAlertChannel, e alerts.Event) error {
	cfg := row.ParsedConfig
	if cfg == nil {
		cfg = map[string]any{}
	}
	text := alertText(e)
	switch row.Type {
	case "email":
		to := alertConfigString(cfg, "address", "email", "to")
		if to == "" {
			return fmt.Errorf("no email address configured")
		}
		return sendSMTPEmail(ctx, store, to, "RAKSHA alert: "+e.Title, text+"\n")
	case "pagerduty":
		key := alertConfigString(cfg, "routing_key", "integration_key")
		if key == "" {
			return fmt.Errorf("no PagerDuty routing key configured")
		}
		body := map[string]any{
			"routing_key":  key,
			"event_action": "trigger",
			"payload": map[string]any{
				"summary":        e.Title + ": " + e.Message,
				"source":         "raksha-gateway",
				"severity":       pagerDutySeverity(e.Severity),
				"custom_details": e.Fields,
			},
		}
		if e.DedupeKey != "" {
			body["dedup_key"] = e.DedupeKey
		}
		return postAlertJSON("https://events.pagerduty.com/v2/enqueue", body)
	case "slack":
		url := alertConfigString(cfg, "url", "webhook_url")
		if url == "" {
			return fmt.Errorf("no Slack webhook URL configured")
		}
		return postAlertJSON(url, map[string]any{"text": text})
	case "webhook":
		url := alertConfigString(cfg, "url", "webhook_url")
		if url == "" {
			return fmt.Errorf("no webhook URL configured")
		}
		return postAlertJSON(url, map[string]any{
			"event":    e.Kind,
			"severity": e.Severity,
			"title":    e.Title,
			"message":  e.Message,
			"fields":   e.Fields,
			"time":     e.Time.UTC().Format(time.RFC3339),
			"channel":  row.Name,
		})
	default:
		return fmt.Errorf("unsupported channel type %q", row.Type)
	}
}

// StartAlertDispatcher routes gateway incidents (circuit breaker trips, budget exhaustion,
// rate limiting) to every enabled alert channel.
func StartAlertDispatcher(store configstore.ConfigStore) {
	ws, ok := configstore.AsWorkspaceStore(store)
	if !ok || ws == nil {
		alerts.SetSink(nil)
		return
	}
	alerts.SetSink(func(e alerts.Event) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		rows, err := ws.ListAlertChannels(ctx)
		if err != nil {
			logger.Warn("alert dispatch: failed to list channels: %v", err)
			return
		}
		for i := range rows {
			row := rows[i]
			if !row.Enabled {
				continue
			}
			if err := deliverAlert(ctx, store, &row, e); err != nil {
				logger.Warn("alert dispatch: %s channel %q failed: %v", row.Type, row.Name, err)
			}
		}
	})
}

func postAlertJSON(url string, body any) error {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return fmt.Errorf("URL must start with http:// or https://")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)
	req.SetRequestURI(url)
	req.Header.SetMethod(fasthttp.MethodPost)
	req.Header.SetContentType("application/json")
	req.SetBody(raw)
	client := &fasthttp.Client{ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	if err := client.DoTimeout(req, resp, 10*time.Second); err != nil {
		return err
	}
	if code := resp.StatusCode(); code < 200 || code >= 300 {
		return fmt.Errorf("endpoint returned HTTP %d", code)
	}
	return nil
}
