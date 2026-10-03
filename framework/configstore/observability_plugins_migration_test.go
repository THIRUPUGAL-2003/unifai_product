package configstore

import "testing"

func TestObservabilityPluginConfigured(t *testing.T) {
	cases := []struct {
		name   string
		plugin string
		config any
		want   bool
	}{
		{"telemetry pull only", "telemetry", map[string]any{"metrics_enabled": true}, false},
		{"telemetry push gateway", "telemetry", map[string]any{"push_gateway_enabled": true}, true},
		{"otel no profiles", "otel", map[string]any{"profiles": []any{}}, false},
		{"otel empty collector", "otel", map[string]any{"profiles": []any{map[string]any{"collector_url": ""}}}, false},
		{"otel collector string", "otel", map[string]any{"profiles": []any{map[string]any{"collector_url": "http://otel:4318"}}}, true},
		{"otel collector env var", "otel", map[string]any{"profiles": []any{map[string]any{"collector_url": map[string]any{"env_var": "env.OTEL_URL"}}}}, true},
		{"nil config", "otel", nil, false},
	}
	for _, c := range cases {
		if got := observabilityPluginConfigured(c.plugin, c.config); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
