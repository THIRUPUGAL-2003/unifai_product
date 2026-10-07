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
	"github.com/gateway/gateway/core/schemas"
)

func exportNewRelic(ctx context.Context, cfg Settings, trace *schemas.Trace) error {
	return sendNewRelic(ctx, cfg, traceEvent(trace))
}

func testNewRelic(ctx context.Context, cfg Settings) error {
	return sendNewRelic(ctx, cfg, map[string]any{
		"message": "gateway connector test",
		"source":  "gateway",
	})
}

func sendNewRelic(ctx context.Context, cfg Settings, attributes map[string]any) error {
	apiKey := configValue(cfg.Config, "api_key")
	if apiKey == "" {
		return fmt.Errorf("new relic api_key is required")
	}
	accountID := configValue(cfg.Config, "account_id")
	if accountID == "" {
		return fmt.Errorf("new relic account_id is required")
	}
	region := configValue(cfg.Config, "region")
	if region == "" {
		region = "US"
	}
	host := "log-api.newrelic.com"
	if strings.EqualFold(region, "EU") {
		host = "log-api.eu.newrelic.com"
	}
	service := configValue(cfg.Config, "service")
	if service == "" {
		service = "gateway"
	}
	payload, err := sonic.Marshal([]map[string]any{{
		"common": map[string]any{
			"attributes": map[string]any{
				"service": service,
				"source":  "gateway",
			},
		},
		"logs": []map[string]any{{
			"timestamp":  time.Now().UnixMilli(),
			"message":    "gateway inference trace",
			"attributes": attributes,
		}},
	}})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://%s/log/v1", host)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", apiKey)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("new relic returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
