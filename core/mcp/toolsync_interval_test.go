package mcp

import (
	"testing"
	"time"

	"github.com/unifai/unifai/core/schemas"
)

func TestSetGlobalIntervalRestartsOnlyGlobalSyncers(t *testing.T) {
	tsm := NewToolSyncManager(5 * time.Minute)
	m := &MCPManager{clientMap: map[string]*schemas.MCPClientState{}}

	global := NewClientToolSyncer(m, "global", "global", tsm.GetGlobalInterval(), nil)
	global.usesGlobal = true
	override := NewClientToolSyncer(m, "override", "override", time.Hour, nil)
	tsm.StartSyncing(global)
	tsm.StartSyncing(override)
	t.Cleanup(tsm.StopAll)

	tsm.SetGlobalInterval(2 * time.Minute)

	if got := tsm.GetGlobalInterval(); got != 2*time.Minute {
		t.Fatalf("global interval = %v, want 2m", got)
	}
	tsm.mu.RLock()
	defer tsm.mu.RUnlock()
	if s := tsm.syncers["global"]; s == global || s.interval != 2*time.Minute || !s.usesGlobal {
		t.Fatalf("global syncer was not restarted with the new interval: %+v", s)
	}
	if s := tsm.syncers["override"]; s != override || s.interval != time.Hour {
		t.Fatalf("per-client override syncer must be left alone: %+v", s)
	}
}

func TestSetGlobalIntervalNonPositiveUsesDefault(t *testing.T) {
	tsm := NewToolSyncManager(5 * time.Minute)
	tsm.SetGlobalInterval(0)
	if got := tsm.GetGlobalInterval(); got != DefaultToolSyncInterval {
		t.Fatalf("global interval = %v, want default %v", got, DefaultToolSyncInterval)
	}
}
