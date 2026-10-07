package plugins

import (
	"context"
	"plugin"

	"github.com/gateway/gateway/core/schemas"
)

// DynamicPlugin is a generic dynamic plugin that can implement any combination of plugin interfaces
// It uses optional function pointers - nil pointers indicate the interface is not implemented
type DynamicPlugin struct {
	Enabled bool
	Path    string
	Config  any

	filename string
	plugin   *plugin.Plugin

	// BasePlugin (required)
	getName func() string
	cleanup func() error

	// HTTPTransportPlugin (optional)
	httpTransportPreHook         func(ctx *schemas.GatewayContext, req *schemas.HTTPRequest) (*schemas.HTTPResponse, error)
	httpTransportPostHook        func(ctx *schemas.GatewayContext, req *schemas.HTTPRequest, resp *schemas.HTTPResponse) error
	httpTransportStreamChunkHook func(ctx *schemas.GatewayContext, req *schemas.HTTPRequest, stream *schemas.GatewayStreamChunk) (*schemas.GatewayStreamChunk, error)

	// LLMPlugin (optional)
	// preRequestHook is forward-compat: new .so plugins built against LLMPlugin can export
	// PreRequestHook to participate in the per-request routing phase. Legacy plugins predating
	// PreRequestHook leave it nil and silently no-op for routing.
	preRequestHook func(ctx *schemas.GatewayContext, req *schemas.GatewayRequest) error
	preLLMHook     func(ctx *schemas.GatewayContext, req *schemas.GatewayRequest) (*schemas.GatewayRequest, *schemas.LLMPluginShortCircuit, error)
	postLLMHook    func(ctx *schemas.GatewayContext, resp *schemas.GatewayResponse, gatewayErr *schemas.GatewayError) (*schemas.GatewayResponse, *schemas.GatewayError, error)

	// MCPPlugin (optional)
	preMCPHook  func(ctx *schemas.GatewayContext, req *schemas.GatewayMCPRequest) (*schemas.GatewayMCPRequest, *schemas.MCPPluginShortCircuit, error)
	postMCPHook func(ctx *schemas.GatewayContext, resp *schemas.GatewayMCPResponse, gatewayErr *schemas.GatewayError) (*schemas.GatewayMCPResponse, *schemas.GatewayError, error)

	// MCPConnectionPlugin (optional, typed). Forward-compat: new .so plugins can
	// export PreMCPConnectionHook/PostMCPConnectionHook to receive Connect events
	// with the typed signatures. Legacy plugins (pre-MCPConnectionPlugin) leave
	// these nil and silently no-op for Connect.
	preMCPConnectionHook  func(ctx *schemas.GatewayContext, req *schemas.GatewayMCPConnectRequest) (*schemas.GatewayMCPConnectRequest, *schemas.MCPConnectionShortCircuit, error)
	postMCPConnectionHook func(ctx *schemas.GatewayContext, resp *schemas.GatewayMCPConnectResponse, gatewayErr *schemas.GatewayError) (*schemas.GatewayMCPConnectResponse, *schemas.GatewayError, error)

	// ObservabilityPlugin (optional)
	inject func(ctx context.Context, trace *schemas.Trace) error
}

// GetName returns the name of the plugin (BasePlugin interface)
func (dp *DynamicPlugin) GetName() string {
	return dp.getName()
}

// Cleanup is invoked by core/gateway.go during plugin unload, reload, and shutdown (BasePlugin interface)
func (dp *DynamicPlugin) Cleanup() error {
	return dp.cleanup()
}

// HTTPTransportPreHook intercepts HTTP requests at the transport layer before entering Gateway core (HTTPTransportPlugin interface)
func (dp *DynamicPlugin) HTTPTransportPreHook(ctx *schemas.GatewayContext, req *schemas.HTTPRequest) (*schemas.HTTPResponse, error) {
	if dp.httpTransportPreHook == nil {
		return nil, nil // No-op if not implemented
	}
	return dp.httpTransportPreHook(ctx, req)
}

// HTTPTransportPostHook intercepts HTTP responses at the transport layer after exiting Gateway core (HTTPTransportPlugin interface)
func (dp *DynamicPlugin) HTTPTransportPostHook(ctx *schemas.GatewayContext, req *schemas.HTTPRequest, resp *schemas.HTTPResponse) error {
	if dp.httpTransportPostHook == nil {
		return nil // No-op if not implemented
	}
	return dp.httpTransportPostHook(ctx, req, resp)
}

// HTTPTransportStreamChunkHook intercepts streaming chunks before they are written to the client
func (dp *DynamicPlugin) HTTPTransportStreamChunkHook(ctx *schemas.GatewayContext, req *schemas.HTTPRequest, stream *schemas.GatewayStreamChunk) (*schemas.GatewayStreamChunk, error) {
	if dp.httpTransportStreamChunkHook == nil {
		return stream, nil // No-op if not implemented
	}
	return dp.httpTransportStreamChunkHook(ctx, req, stream)
}

// PreRequestHook is invoked once per top-level request to decide provider/model/fallbacks
// (LLMPlugin interface). Defaults to a no-op passthrough for legacy plugins that don't
// export PreRequestHook.
func (dp *DynamicPlugin) PreRequestHook(ctx *schemas.GatewayContext, req *schemas.GatewayRequest) error {
	if dp.preRequestHook == nil {
		return nil
	}
	return dp.preRequestHook(ctx, req)
}

// PreLLMHook is invoked before LLM provider calls (LLMPlugin interface)
func (dp *DynamicPlugin) PreLLMHook(ctx *schemas.GatewayContext, req *schemas.GatewayRequest) (*schemas.GatewayRequest, *schemas.LLMPluginShortCircuit, error) {
	if dp.preLLMHook == nil {
		return req, nil, nil // No-op if not implemented
	}
	return dp.preLLMHook(ctx, req)
}

// PostLLMHook is invoked after LLM provider calls (LLMPlugin interface)
func (dp *DynamicPlugin) PostLLMHook(ctx *schemas.GatewayContext, resp *schemas.GatewayResponse, gatewayErr *schemas.GatewayError) (*schemas.GatewayResponse, *schemas.GatewayError, error) {
	if dp.postLLMHook == nil {
		return resp, gatewayErr, nil // No-op if not implemented
	}
	return dp.postLLMHook(ctx, resp, gatewayErr)
}

// PreMCPHook is invoked before MCP calls (MCPPlugin interface)
func (dp *DynamicPlugin) PreMCPHook(ctx *schemas.GatewayContext, req *schemas.GatewayMCPRequest) (*schemas.GatewayMCPRequest, *schemas.MCPPluginShortCircuit, error) {
	if dp.preMCPHook == nil {
		return req, nil, nil // No-op if not implemented
	}
	return dp.preMCPHook(ctx, req)
}

// PostMCPHook is invoked after MCP calls (MCPPlugin interface)
func (dp *DynamicPlugin) PostMCPHook(ctx *schemas.GatewayContext, resp *schemas.GatewayMCPResponse, gatewayErr *schemas.GatewayError) (*schemas.GatewayMCPResponse, *schemas.GatewayError, error) {
	if dp.postMCPHook == nil {
		return resp, gatewayErr, nil // No-op if not implemented
	}
	return dp.postMCPHook(ctx, resp, gatewayErr)
}

// PreMCPConnectionHook satisfies MCPConnectionPlugin for dynamically-loaded plugins.
// If the .so exported PreMCPConnectionHook, dispatch to it. Otherwise default to
// a no-op passthrough — legacy plugins predating MCPConnectionPlugin keep working
// as MCPPlugin (via PreMCPHook/PostMCPHook) and silently skip Connect events.
func (dp *DynamicPlugin) PreMCPConnectionHook(ctx *schemas.GatewayContext, req *schemas.GatewayMCPConnectRequest) (*schemas.GatewayMCPConnectRequest, *schemas.MCPConnectionShortCircuit, error) {
	if dp.preMCPConnectionHook == nil {
		return req, nil, nil
	}
	return dp.preMCPConnectionHook(ctx, req)
}

// PostMCPConnectionHook satisfies MCPConnectionPlugin for dynamically-loaded plugins.
// Same dispatch as PreMCPConnectionHook: typed symbol if exported, else no-op.
func (dp *DynamicPlugin) PostMCPConnectionHook(ctx *schemas.GatewayContext, resp *schemas.GatewayMCPConnectResponse, gatewayErr *schemas.GatewayError) (*schemas.GatewayMCPConnectResponse, *schemas.GatewayError, error) {
	if dp.postMCPConnectionHook == nil {
		return resp, gatewayErr, nil
	}
	return dp.postMCPConnectionHook(ctx, resp, gatewayErr)
}

// Inject receives completed traces for observability backends (ObservabilityPlugin interface)
func (dp *DynamicPlugin) Inject(ctx context.Context, trace *schemas.Trace) error {
	if dp.inject == nil {
		return nil // No-op if not implemented
	}
	return dp.inject(ctx, trace)
}
