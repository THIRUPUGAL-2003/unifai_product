package connectors

import (
	"os"
	"strings"
)

func resolveValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "env.") {
		if v, ok := os.LookupEnv(strings.TrimPrefix(raw, "env.")); ok {
			return v
		}
		return ""
	}
	return raw
}

func configValue(cfg map[string]string, key string) string {
	if cfg == nil {
		return ""
	}
	return resolveValue(cfg[key])
}
