package connectors

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/unifai/unifai/core/schemas"
)

func exportDatadog(ctx context.Context, cfg Settings, trace *schemas.Trace) error {
	apiKey := configValue(cfg.Config, "api_key")
	if apiKey == "" {
		return fmt.Errorf("datadog api_key is required")
	}
	site := configValue(cfg.Config, "site")
	if site == "" {
		site = "datadoghq.com"
	}
	service := configValue(cfg.Config, "service")
	if service == "" {
		service = "unifai"
	}
	event := traceEvent(trace)
	payload, err := sonic.Marshal([]map[string]any{{
		"ddsource": "unifai",
		"service":  service,
		"message":  "unifai inference trace",
		"status":   "info",
		"trace_id": trace.TraceID,
		"attributes": event,
	}})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://http-intake.logs.%s/api/v2/logs", strings.TrimPrefix(site, "https://"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("datadog intake returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func testDatadog(ctx context.Context, cfg Settings) error {
	apiKey := configValue(cfg.Config, "api_key")
	if apiKey == "" {
		return fmt.Errorf("api_key is required")
	}
	site := configValue(cfg.Config, "site")
	if site == "" {
		site = "datadoghq.com"
	}
	service := configValue(cfg.Config, "service")
	if service == "" {
		service = "unifai"
	}
	payload, _ := sonic.Marshal([]map[string]any{{
		"ddsource": "unifai",
		"service":  service,
		"message":  "unifai connector test",
		"status":   "info",
	}})
	url := fmt.Sprintf("https://http-intake.logs.%s/api/v2/logs", strings.TrimPrefix(site, "https://"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("datadog returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
