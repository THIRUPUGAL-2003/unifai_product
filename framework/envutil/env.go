package envutil

import (
	"os"
	"strings"
)

// Get reads GATEWAY_<NAME>.
func Get(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = strings.TrimPrefix(name, "GATEWAY_")
	return strings.TrimSpace(os.Getenv("GATEWAY_" + name))
}
