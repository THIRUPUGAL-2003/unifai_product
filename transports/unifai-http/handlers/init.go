package handlers

import (
	"strings"

	"github.com/unifai/unifai/core/schemas"
)

var version string
var logger schemas.Logger

// SetLogger sets the logger for the application.
func SetLogger(l schemas.Logger) {
	logger = l
}

// SetVersion sets the version for the application.
func SetVersion(v string) {
	version = normalizeAppVersion(v)
}

func GetVersion() string {
	return version
}

func normalizeAppVersion(v string) string {
	value := strings.TrimSpace(v)
	switch strings.ToLower(value) {
	case "", "v", "unknown", "vunknown":
		return ""
	default:
		return value
	}
}
