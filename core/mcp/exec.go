package mcp

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/client"
	"github.com/raksha/raksha/core/schemas"
)

// ============================================================================
// MCP REQUEST POOL
// ============================================================================
//
// Pool for RakshaMCPRequest objects. Owned by the mcp package because these
// requests are only used inside this package — the Raksha public API just
// delegates to MCPManager's Execute* methods.

var mcpRequestPool = sync.Pool{
	New: func() any {
		return &schemas.RakshaMCPRequest{}
	},
}

// resetMCPRequest zeroes a RakshaMCPRequest for reuse. Must be kept in sync with
// the fields defined on the request struct.
func resetMCPRequest(req *schemas.RakshaMCPRequest) {
	req.RequestType = ""
	req.ClientName = ""
	req.RakshaMCPPingRequest = nil
	req.RakshaMCPListToolsRequest = nil
	req.RakshaMCPExecuteToolRequest = nil
	req.ChatAssistantMessageToolCall = nil
	req.ResponsesToolMessage = nil
}

func getMCPRequest() *schemas.RakshaMCPRequest {
	return mcpRequestPool.Get().(*schemas.RakshaMCPRequest)
}

func releaseMCPRequest(req *schemas.RakshaMCPRequest) {
	resetMCPRequest(req)
	mcpRequestPool.Put(req)
}

// ============================================================================
// EXECUTE-TOOL GATE (matches the pattern of connect/ping/list_tools gates)
// ============================================================================

// executeToolWithHooks runs an MCP tool call through the plugin gate. It is the
// execute-tool counterpart to the connect/ping/list_tools gates. Mirrors the
// short-circuit + PostHook semantics of all other gates by delegating to
// runWithPluginPipeline, then adds two execute-specific touches on the returned RakshaError:
//
//   - stamps ExtraFields.RequestType from the caller-provided RequestType
//   - preserves MCPUserOAuthRequiredError so agent-mode detection still works
//
// requestType is the raksha-side RequestType (ChatCompletionRequest / ResponsesRequest)
// that error metadata should carry — it isn't the same as request.RequestType.
func (m *MCPManager) executeToolWithHooks(
	ctx *schemas.RakshaContext,
	request *schemas.RakshaMCPRequest,
	requestType schemas.RequestType,
) (*schemas.RakshaMCPResponse, *schemas.RakshaError) {
	if request == nil {
		return nil, &schemas.RakshaError{
			IsRakshaError: false,
			Error:          &schemas.ErrorField{Message: "request cannot be nil"},
			ExtraFields:    schemas.RakshaErrorExtraFields{RequestType: requestType},
		}
	}

	// Populate top-level ClientName from the prefixed tool name so the gate can
	// attribute short-circuit responses without depending on prefix parsing.
	if request.ClientName == "" {
		if toolName := request.GetToolName(); toolName != "" {
			if idx := strings.IndexByte(toolName, '-'); idx > 0 {
				request.ClientName = toolName[:idx]
			}
		}
	}

	// Resolve the upstream client and acquire its connection BEFORE the plugin
	// gate runs. Connection lifecycle is the orchestrator's concern, not the
	// plugin op's — the plugin pipeline only wraps the actual CallTool. When
	// AcquireClientConn fails (e.g. *MCPAuthRequiredError for per-user
	// clients that need re-auth or headers submission), the plugin gate is
	// never invoked.
	state, conn, release, prepErr := m.prepareToolExecution(ctx, request)
	if prepErr != nil {
		rakshaErr := &schemas.RakshaError{
			IsRakshaError: false,
			Error:          &schemas.ErrorField{Message: prepErr.Error()},
			ExtraFields:    schemas.RakshaErrorExtraFields{RequestType: requestType, MCPRequestType: request.RequestType},
		}
		var authRequiredErr *schemas.MCPAuthRequiredError
		if errors.As(prepErr, &authRequiredErr) {
			rakshaErr.ExtraFields.MCPAuthRequired = authRequiredErr
		}
		return nil, rakshaErr
	}
	defer release()

	// state == nil signals a code-mode tool: pass nil conn/config/mapping and
	// ToolsManager.ExecuteTool routes directly to CodeMode.
	var executionConfig *schemas.MCPClientConfig
	var toolNameMapping map[string]string
	if state != nil {
		executionConfig = state.ExecutionConfig
		toolNameMapping = state.ToolNameMapping
	}

	resp, rakshaErr := m.RunWithPluginPipeline(ctx, request, func(preReq *schemas.RakshaMCPRequest) (*schemas.RakshaMCPResponse, error) {
		result, opErr := m.toolsManager.ExecuteTool(ctx, preReq, conn, executionConfig, toolNameMapping)
		if opErr != nil {
			return nil, opErr
		}
		if result == nil {
			return nil, fmt.Errorf("tool execution returned nil result")
		}
		return result, nil
	})

	if rakshaErr != nil {
		rakshaErr.ExtraFields.RequestType = requestType
		return nil, rakshaErr
	}
	return resp, nil
}

// prepareToolExecution resolves the tool to its owning MCP client and
// acquires a connection. Returns (state, conn, release, err):
//   - For regular MCP tools: state non-nil, conn is the live transport, release
//     must be called by the caller (defer).
//   - For code-mode tools: state nil, conn nil, release is a no-op. The caller
//     forwards nil conn/config/mapping to ToolsManager.ExecuteTool which
//     dispatches via the CodeMode implementation.
//
// Errors here mean the call should NOT run — neither the envelope plugin
// gate nor the wire op. Typed errors (e.g. *MCPUserOAuthRequiredError)
// propagate so the caller can stamp RakshaError.ExtraFields.
func (m *MCPManager) prepareToolExecution(ctx *schemas.RakshaContext, request *schemas.RakshaMCPRequest) (*schemas.MCPClientState, *client.Client, func(), error) {
	toolName := request.GetToolName()
	if toolName == "" {
		return nil, nil, nil, fmt.Errorf("tool call missing function name")
	}

	// Code-mode tools have no upstream client — skip client lookup.
	codeMode := m.toolsManager.GetCodeMode()
	if codeMode != nil && codeMode.IsCodeModeTool(toolName) {
		return nil, nil, func() {}, nil
	}

	state := m.GetClientForTool(toolName)
	if state == nil {
		return nil, nil, nil, fmt.Errorf("tool '%s' is not available or not permitted", toolName)
	}
	clientName := state.ExecutionConfig.Name
	// Enforce the same filters that GetToolPerClient applies for tool
	// discovery, in the same order. Without these a caller could invoke a
	// tool by name that was deliberately hidden from the tool list.
	//
	//  1. Client lifecycle — a disabled client is not usable.
	//  2. Client allow-list — request-context MCPContextKeyIncludeClients.
	//  3. Tool allow-list   — client-level ToolsToExecute (most restrictive).
	//  4. Tool narrowing    — request-context MCPContextKeyIncludeTools.
	if state.State == schemas.MCPConnectionStateDisabled {
		return nil, nil, nil, fmt.Errorf("tool '%s' is not permitted (client %s is disabled)", toolName, clientName)
	}
	var includeClients []string
	if v, ok := ctx.Value(schemas.MCPContextKeyIncludeClients).([]string); ok {
		includeClients = v
	}
	if !shouldIncludeClient(clientName, includeClients, m.logger) {
		return nil, nil, nil, fmt.Errorf("tool '%s' is not permitted (client %s is not in request-context include list)", toolName, clientName)
	}
	if shouldSkipToolForConfig(toolName, state.ExecutionConfig) {
		return nil, nil, nil, fmt.Errorf("tool '%s' is not permitted (not in client's ToolsToExecute allow-list)", toolName)
	}
	if shouldSkipToolForRequest(ctx, clientName, toolName) {
		return nil, nil, nil, fmt.Errorf("tool '%s' is not permitted (filtered by request context)", toolName)
	}
	conn, release, err := m.AcquireClientConn(ctx, state)
	if err != nil {
		return nil, nil, nil, err
	}
	return state, conn, release, nil
}

// executeToolForAgent is the agent-mode-facing helper. The agent loop expects a
// plain (response, error) signature and doesn't need rich RakshaError fields,
// so we collapse them. MCPUserOAuthRequiredError is returned directly when present
// so agent mode can detect it via errors.As.
func (m *MCPManager) executeToolForAgent(ctx *schemas.RakshaContext, request *schemas.RakshaMCPRequest) (*schemas.RakshaMCPResponse, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}
	if request == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	// Derive raksha RequestType from the MCP request type (only execute-tool variants
	// are valid in the agent loop).
	var requestType schemas.RequestType
	switch request.RequestType {
	case schemas.MCPRequestTypeChatToolCall:
		requestType = schemas.ChatCompletionRequest
	case schemas.MCPRequestTypeResponsesToolCall:
		requestType = schemas.ResponsesRequest
	default:
		return nil, fmt.Errorf("unsupported MCP request type for agent: %s", request.RequestType)
	}

	resp, rakshaErr := m.executeToolWithHooks(ctx, request, requestType)
	if rakshaErr != nil {
		// Surface the typed OAuth error so agent mode can react to it.
		if rakshaErr.ExtraFields.MCPAuthRequired != nil {
			return nil, rakshaErr.ExtraFields.MCPAuthRequired
		}
		return nil, fmt.Errorf("tool execution failed: %s", rakshaErr.GetErrorString())
	}
	return resp, nil
}

// ============================================================================
// PUBLIC EXECUTE-TOOL ENTRY POINTS
// ============================================================================

// ExecuteChatTool executes an MCP tool call and returns the result as a chat message.
// This is the canonical entry point for manual MCP tool execution in Chat format.
// Raksha.ExecuteChatMCPTool delegates here.
func (m *MCPManager) ExecuteChatTool(ctx *schemas.RakshaContext, toolCall *schemas.ChatAssistantMessageToolCall) (*schemas.ChatMessage, *schemas.RakshaError) {
	if toolCall == nil {
		return nil, &schemas.RakshaError{
			IsRakshaError: false,
			Error:          &schemas.ErrorField{Message: "toolCall cannot be nil"},
			ExtraFields:    schemas.RakshaErrorExtraFields{RequestType: schemas.ChatCompletionRequest},
		}
	}

	mcpRequest := getMCPRequest()
	mcpRequest.RequestType = schemas.MCPRequestTypeChatToolCall
	mcpRequest.ChatAssistantMessageToolCall = toolCall
	defer releaseMCPRequest(mcpRequest)

	result, rakshaErr := m.executeToolWithHooks(ctx, mcpRequest, schemas.ChatCompletionRequest)
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	if result == nil || result.ChatMessage == nil {
		return nil, &schemas.RakshaError{
			IsRakshaError: false,
			Error:          &schemas.ErrorField{Message: "MCP tool execution returned nil chat message"},
			ExtraFields:    schemas.RakshaErrorExtraFields{RequestType: schemas.ChatCompletionRequest},
		}
	}
	return result.ChatMessage, nil
}

// ExecuteResponsesTool executes an MCP tool call and returns the result as a responses
// message. Raksha.ExecuteResponsesMCPTool delegates here.
func (m *MCPManager) ExecuteResponsesTool(ctx *schemas.RakshaContext, toolCall *schemas.ResponsesToolMessage) (*schemas.ResponsesMessage, *schemas.RakshaError) {
	if toolCall == nil {
		return nil, &schemas.RakshaError{
			IsRakshaError: false,
			Error:          &schemas.ErrorField{Message: "toolCall cannot be nil"},
			ExtraFields:    schemas.RakshaErrorExtraFields{RequestType: schemas.ResponsesRequest},
		}
	}

	mcpRequest := getMCPRequest()
	mcpRequest.RequestType = schemas.MCPRequestTypeResponsesToolCall
	mcpRequest.ResponsesToolMessage = toolCall
	defer releaseMCPRequest(mcpRequest)

	result, rakshaErr := m.executeToolWithHooks(ctx, mcpRequest, schemas.ResponsesRequest)
	if rakshaErr != nil {
		return nil, rakshaErr
	}
	if result == nil || result.ResponsesMessage == nil {
		return nil, &schemas.RakshaError{
			IsRakshaError: false,
			Error:          &schemas.ErrorField{Message: "MCP tool execution returned nil responses message"},
			ExtraFields:    schemas.RakshaErrorExtraFields{RequestType: schemas.ResponsesRequest},
		}
	}
	return result.ResponsesMessage, nil
}
