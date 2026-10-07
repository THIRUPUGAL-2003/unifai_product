package main

import (
	"os"
	"strings"
)

// GatewayEnv reads GATEWAY_<NAME>.
func GatewayEnv(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = strings.TrimPrefix(name, "GATEWAY_")
	return strings.TrimSpace(os.Getenv("GATEWAY_" + name))
}
