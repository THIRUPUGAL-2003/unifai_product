package handlers

import "testing"

// Setup packages embed guard_secret, so they must never be anonymously downloadable.
func TestGuardSetupDownloadsNeedGuardKeyOrSession(t *testing.T) {
	for _, path := range []string{
		"/api/browser-ai/setup/download.zip",
		"/api/browser-ai/setup/download-windows.zip",
		"/api/browser-ai/setup/download-mac.zip",
	} {
		for _, method := range []string{"GET", "HEAD"} {
			if isPublicBrowserAIRoute(method, path) {
				t.Errorf("%s %s must not be public", method, path)
			}
			if !isGuardKeyBrowserAIRoute(method, path) {
				t.Errorf("%s %s must accept the Guard key for auto-update", method, path)
			}
		}
	}
	for _, path := range []string{
		"/api/browser-ai/agents/guard-1/uninstall-key",
		"/api/browser-ai/agents/guard-1/uninstall-key/rotate",
		"/api/browser-ai/setup/rebuild",
		"/api/browser-ai/setup/rebuild-history",
		"/api/browser-ai/agents/guard-1/contact-email",
	} {
		for _, method := range []string{"GET", "POST"} {
			if isPublicBrowserAIRoute(method, path) || isGuardKeyBrowserAIRoute(method, path) {
				t.Errorf("%s %s must require an admin session", method, path)
			}
		}
	}
}
