package handlers

import (
	"testing"
	"time"

	"github.com/raksha/raksha/framework/logstore"
)

// Dashboard "Turn Off" → laptop uninstaller must skip the key prompt; a manual
// uninstall on a normal active Guard must still ask for the key.
func TestRemoteUninstallAuthorization(t *testing.T) {
	now := time.Now()
	recent := now.Add(-2 * time.Minute)
	old := now.Add(-remoteUninstallGrace - time.Minute)
	cases := []struct {
		name  string
		agent *logstore.BrowserAIAgent
		want  bool
	}{
		{"admin requested, Guard not yet acked", &logstore.BrowserAIAgent{Status: logstore.AgentStatusUninstallPending, UninstallRequested: true}, true},
		{"Guard acked moments ago (uninstaller running)", &logstore.BrowserAIAgent{Status: logstore.AgentStatusUninstalled, UninstalledAt: &recent}, true},
		{"acked long ago", &logstore.BrowserAIAgent{Status: logstore.AgentStatusUninstalled, UninstalledAt: &old}, false},
		{"active Guard, employee uninstalls manually", &logstore.BrowserAIAgent{Status: logstore.AgentStatusActive}, false},
		{"unknown Guard", nil, false},
	}
	for _, c := range cases {
		if got := remoteUninstallAuthorized(c.agent, now); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
