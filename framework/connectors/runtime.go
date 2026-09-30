package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/configstore"
)

// TestResult reports whether a connector can reach its destination.
type TestResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// Runtime holds live connector settings loaded from workspace_settings.
type Runtime struct {
	mu      sync.RWMutex
	byName  map[string]Settings
	enabled map[string]bool
}

// Default is the process-wide connector runtime.
var Default = &Runtime{
	byName:  make(map[string]Settings),
	enabled: make(map[string]bool),
}

// ReloadFromStore refreshes connector settings from the workspace DB.
func ReloadFromStore(ctx context.Context, store configstore.WorkspaceStore) error {
	if store == nil {
		Default.mu.Lock()
		Default.byName = make(map[string]Settings)
		Default.enabled = make(map[string]bool)
		Default.mu.Unlock()
		return nil
	}
	loaded := make(map[string]Settings, len(ConnectorNames))
	enabled := make(map[string]bool, len(ConnectorNames))
	for _, name := range ConnectorNames {
		row, err := store.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingConnector(name))
		if err != nil {
			if err == configstore.ErrNotFound {
				continue
			}
			return err
		}
		var payload struct {
			Enabled bool           `json:"enabled"`
			Config  map[string]any   `json:"config"`
		}
		if err := json.Unmarshal([]byte(row.Data), &payload); err != nil {
			return fmt.Errorf("parse connector %s: %w", name, err)
		}
		cfg := map[string]string{}
		for k, v := range payload.Config {
			if s, ok := v.(string); ok {
				cfg[k] = s
			}
		}
		loaded[name] = Settings{Name: name, Enabled: payload.Enabled, Config: cfg}
		enabled[name] = payload.Enabled
	}
	Default.mu.Lock()
	Default.byName = loaded
	Default.enabled = enabled
	Default.mu.Unlock()
	return nil
}

func (r *Runtime) settings(name string) (Settings, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byName[name]
	return s, ok
}

// ExportTrace forwards a completed trace to all enabled connectors.
func (r *Runtime) ExportTrace(ctx context.Context, trace *schemas.Trace) {
	if trace == nil {
		return
	}
	for _, name := range ConnectorNames {
		cfg, ok := r.settings(name)
		if !ok || !cfg.Enabled {
			continue
		}
		var err error
		switch name {
		case "datadog":
			err = exportDatadog(ctx, cfg, trace)
		case "kafka":
			err = exportKafka(ctx, cfg, trace)
		case "bigquery":
			err = exportBigQuery(ctx, cfg, trace)
		case "pubsub":
			err = exportPubSub(ctx, cfg, trace)
		case "newrelic":
			err = exportNewRelic(ctx, cfg, trace)
		}
		if err != nil {
			// Best-effort export; do not fail the request pipeline.
			_ = err
		}
	}
}

// Test validates connectivity for a connector by name using current or supplied settings.
func (r *Runtime) Test(ctx context.Context, name string, cfg Settings) TestResult {
	if !cfg.Enabled {
		return TestResult{OK: true}
	}
	var err error
	switch name {
	case "datadog":
		err = testDatadog(ctx, cfg)
	case "kafka":
		err = testKafka(ctx, cfg)
	case "bigquery":
		err = testBigQuery(ctx, cfg)
	case "pubsub":
		err = testPubSub(ctx, cfg)
	case "newrelic":
		err = testNewRelic(ctx, cfg)
	default:
		err = fmt.Errorf("unknown connector %s", name)
	}
	if err != nil {
		return TestResult{OK: false, Error: err.Error()}
	}
	return TestResult{OK: true}
}

// SettingsFor returns the loaded settings for a connector name.
func (r *Runtime) SettingsFor(name string) (Settings, bool) {
	return r.settings(name)
}

// ApplySettings updates one connector in the in-memory runtime (after save).
func (r *Runtime) ApplySettings(cfg Settings) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byName[cfg.Name] = cfg
	r.enabled[cfg.Name] = cfg.Enabled
}

// Remove clears one connector from the in-memory runtime (after delete).
func (r *Runtime) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byName, name)
	delete(r.enabled, name)
}
