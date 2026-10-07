package handlers

import (
	"os"
	"strings"
)

// gatewayEnv reads GATEWAY_<NAME>.
func gatewayEnv(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = strings.TrimPrefix(name, "GATEWAY_")
	return strings.TrimSpace(os.Getenv("GATEWAY_" + name))
}
