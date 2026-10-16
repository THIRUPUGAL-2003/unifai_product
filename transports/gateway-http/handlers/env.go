package handlers

import (
	"os"
	"strings"
)

// gatewayEnv reads GATEWAY_<NAME> or <NAME> from environment.
func gatewayEnv(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	clean := strings.TrimPrefix(name, "GATEWAY_")
	if v := strings.TrimSpace(os.Getenv("GATEWAY_" + clean)); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv(clean)); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv(name))
}
