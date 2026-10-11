package lib

import (
	"context"

	"github.com/gateway/gateway/core/schemas"
)

// LargePayloadRuntime is the live large-payload policy from Client Settings.
type LargePayloadRuntime struct {
	Enabled           bool
	RequestThreshold  int64
	ResponseThreshold int64
	Prefetch          int
	MaxPayload        int64
	TruncatedLog      int
}

// ApplyLargePayloadMetadata installs the large_payload object stored in client metadata.
// A nil or disabled value clears the live policy.
func (c *Config) ApplyLargePayloadMetadata(raw any) {
	if c == nil {
		return
	}
	parsed := parseLargePayload(raw)
	if parsed == nil || !parsed.Enabled {
		c.largePayload.Store(nil)
		c.StreamingDecompressThreshold = 0
		return
	}
	c.largePayload.Store(parsed)
	if parsed.RequestThreshold > 0 {
		c.StreamingDecompressThreshold = parsed.RequestThreshold
	}
}

// LargePayload returns the live policy, or nil when large-payload handling is off.
func (c *Config) LargePayload() *LargePayloadRuntime {
	if c == nil {
		return nil
	}
	return c.largePayload.Load()
}

func (c *Config) applyStoredLargePayload(ctx context.Context) {
	if c == nil || c.ConfigStore == nil {
		return
	}
	metadata, err := c.ConfigStore.GetClientMetadata(ctx)
	if err != nil || metadata == nil {
		return
	}
	c.ApplyLargePayloadMetadata(metadata["large_payload"])
}

func parseLargePayload(raw any) *LargePayloadRuntime {
	obj, ok := raw.(map[string]any)
	if !ok || obj == nil {
		return nil
	}
	return &LargePayloadRuntime{
		Enabled:           boolValue(obj["enabled"]),
		RequestThreshold:  int64Value(obj["request_threshold_bytes"]),
		ResponseThreshold: int64Value(obj["response_threshold_bytes"]),
		Prefetch:          int(int64Value(obj["prefetch_size_bytes"])),
		MaxPayload:        int64Value(obj["max_payload_bytes"]),
		TruncatedLog:      int(int64Value(obj["truncated_log_bytes"])),
	}
}

func boolValue(v any) bool {
	b, _ := v.(bool)
	return b
}

func int64Value(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}

// StampLargePayload copies the live policy onto the request context the providers read.
func StampLargePayload(config *Config, ctx *schemas.GatewayContext) {
	if config == nil || ctx == nil {
		return
	}
	lp := config.LargePayload()
	if lp == nil || !lp.Enabled {
		return
	}
	if lp.RequestThreshold > 0 {
		ctx.SetValue(schemas.GatewayContextKeyLargePayloadRequestThreshold, lp.RequestThreshold)
	}
	if lp.Prefetch > 0 {
		ctx.SetValue(schemas.GatewayContextKeyLargePayloadPrefetchSize, lp.Prefetch)
	}
}
