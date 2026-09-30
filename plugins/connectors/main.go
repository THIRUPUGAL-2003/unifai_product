// Package connectors exports inference traces to workspace data connectors.
package connectors

import (
	"context"

	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/connectors"
)

const PluginName = "connectors"

// Plugin forwards completed traces to enabled workspace connectors.
type Plugin struct{}

// Init creates the connectors observability plugin.
func Init() (schemas.BasePlugin, error) {
	return &Plugin{}, nil
}

// GetName returns the plugin name.
func (p *Plugin) GetName() string { return PluginName }

// Cleanup is a no-op for the connectors plugin.
func (p *Plugin) Cleanup() error { return nil }

// Inject exports the trace to Datadog, Kafka, BigQuery, and Pub/Sub when enabled.
func (p *Plugin) Inject(ctx context.Context, trace *schemas.Trace) error {
	connectors.Default.ExportTrace(ctx, trace)
	return nil
}

var _ schemas.ObservabilityPlugin = (*Plugin)(nil)
