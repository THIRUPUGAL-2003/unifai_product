// Package integrations provides a generic router framework for handling different LLM provider APIs.
//
// CENTRALIZED STREAMING ARCHITECTURE:
//
// This package implements a centralized streaming approach where all stream handling logic
// is consolidated in the GenericRouter, eliminating the need for provider-specific StreamHandler
// implementations. The key components are:
//
// 1. StreamConfig: Defines streaming configuration for each route, including:
//   - ResponseConverter: Converts GatewayResponse to provider-specific streaming format
//   - ErrorConverter: Converts GatewayError to provider-specific streaming error format
//
// 2. Centralized Stream Processing: The GenericRouter handles all streaming logic:
//   - SSE header management
//   - Stream channel processing
//   - Error handling and conversion
//   - Response formatting and flushing
//   - Stream closure (handled automatically by provider implementation)
//
// 3. Provider-Specific Type Conversion: Integration types.go files only handle type conversion:
//   - Derive{Provider}StreamFromGatewayResponse: Convert responses to streaming format
//   - Derive{Provider}StreamFromGatewayError: Convert errors to streaming error format
//
// BENEFITS:
// - Eliminates code duplication across provider-specific stream handlers
// - Centralizes streaming logic for consistency and maintainability
// - Separates concerns: routing logic vs type conversion
// - Automatic stream closure management by provider implementations
// - Consistent error handling across all providers
//
// USAGE EXAMPLE:
//
//	routes := []RouteConfig{
//	  {
//	    Path: "/openai/chat/completions",
//	    Method: "POST",
//	    // ... other configs ...
//	    StreamConfig: &StreamConfig{
//	      ResponseConverter: func(resp *schemas.GatewayResponse) (interface{}, error) {
//	        return DeriveOpenAIStreamFromGatewayResponse(resp), nil
//	      },
//	      ErrorConverter: func(err *schemas.GatewayError) interface{} {
//	        return DeriveOpenAIStreamFromGatewayError(err)
//	      },
//	    },
//	  },
//	}
package integrations

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/bytedance/sonic"
	"github.com/fasthttp/router"
	gateway "github.com/gateway/gateway/core"
	"github.com/gateway/gateway/core/providers/bedrock"
	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/framework/logstore"
	"github.com/gateway/gateway/framework/modelcatalog"
	"github.com/gateway/gateway/transports/gateway-http/lib"
	"github.com/valyala/fasthttp"
)

// ExtensionRouter defines the interface that all integration routers must implement
// to register their routes with the main HTTP router.
type ExtensionRouter interface {
	RegisterRoutes(r *router.Router, middlewares ...schemas.GatewayHTTPMiddleware)
}

// StreamingRequest interface for requests that support streaming
type StreamingRequest interface {
	IsStreamingRequested() bool
}

// RequestWithSettableExtraParams is implemented by request types that accept
// provider-specific extra parameters via the extra_params JSON key. The
// integration router extracts extra_params from the raw request body and
// passes them through so they propagate to the downstream provider.
type RequestWithSettableExtraParams interface {
	SetExtraParams(params map[string]interface{})
}

// BatchRequest wraps a Gateway batch request with its type information.
type BatchRequest struct {
	Type            schemas.RequestType
	CreateRequest   *schemas.GatewayBatchCreateRequest
	ListRequest     *schemas.GatewayBatchListRequest
	RetrieveRequest *schemas.GatewayBatchRetrieveRequest
	CancelRequest   *schemas.GatewayBatchCancelRequest
	DeleteRequest   *schemas.GatewayBatchDeleteRequest
	ResultsRequest  *schemas.GatewayBatchResultsRequest
}

// FileRequest wraps a Gateway file request with its type information.
type FileRequest struct {
	Type            schemas.RequestType
	UploadRequest   *schemas.GatewayFileUploadRequest
	ListRequest     *schemas.GatewayFileListRequest
	RetrieveRequest *schemas.GatewayFileRetrieveRequest
	DeleteRequest   *schemas.GatewayFileDeleteRequest
	ContentRequest  *schemas.GatewayFileContentRequest
}

// ContainerRequest wraps a Gateway container request with its type information.
type ContainerRequest struct {
	Type            schemas.RequestType
	CreateRequest   *schemas.GatewayContainerCreateRequest
	ListRequest     *schemas.GatewayContainerListRequest
	RetrieveRequest *schemas.GatewayContainerRetrieveRequest
	DeleteRequest   *schemas.GatewayContainerDeleteRequest
}

// ContainerFileRequest is a wrapper for Gateway container file requests.
type ContainerFileRequest struct {
	Type            schemas.RequestType
	CreateRequest   *schemas.GatewayContainerFileCreateRequest
	ListRequest     *schemas.GatewayContainerFileListRequest
	RetrieveRequest *schemas.GatewayContainerFileRetrieveRequest
	ContentRequest  *schemas.GatewayContainerFileContentRequest
	DeleteRequest   *schemas.GatewayContainerFileDeleteRequest
}

// CachedContentRequest wraps a Gateway cached content request with its type information.
// Used by Gemini and Vertex AI integrations for the named cached content lifecycle.
type CachedContentRequest struct {
	Type            schemas.RequestType
	CreateRequest   *schemas.GatewayCachedContentCreateRequest
	ListRequest     *schemas.GatewayCachedContentListRequest
	RetrieveRequest *schemas.GatewayCachedContentRetrieveRequest
	UpdateRequest   *schemas.GatewayCachedContentUpdateRequest
	DeleteRequest   *schemas.GatewayCachedContentDeleteRequest
}

// BatchRequestConverter is a function that converts integration-specific batch requests to Gateway format.
type BatchRequestConverter func(ctx *schemas.GatewayContext, req interface{}) (*BatchRequest, error)

// FileRequestConverter is a function that converts integration-specific file requests to Gateway format.
type FileRequestConverter func(ctx *schemas.GatewayContext, req interface{}) (*FileRequest, error)

// ContainerRequestConverter is a function that converts integration-specific container requests to Gateway format.
type ContainerRequestConverter func(ctx *schemas.GatewayContext, req interface{}) (*ContainerRequest, error)

// ContainerFileRequestConverter is a function that converts integration-specific container file requests to Gateway format.
type ContainerFileRequestConverter func(ctx *schemas.GatewayContext, req interface{}) (*ContainerFileRequest, error)

// CachedContentRequestConverter is a function that converts integration-specific cached content requests to Gateway format.
type CachedContentRequestConverter func(ctx *schemas.GatewayContext, req interface{}) (*CachedContentRequest, error)

// CachedContentCreateResponseConverter converts GatewayCachedContentCreateResponse to integration format.
type CachedContentCreateResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayCachedContentCreateResponse) (interface{}, error)

// CachedContentListResponseConverter converts GatewayCachedContentListResponse to integration format.
type CachedContentListResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayCachedContentListResponse) (interface{}, error)

// CachedContentRetrieveResponseConverter converts GatewayCachedContentRetrieveResponse to integration format.
type CachedContentRetrieveResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayCachedContentRetrieveResponse) (interface{}, error)

// CachedContentUpdateResponseConverter converts GatewayCachedContentUpdateResponse to integration format.
type CachedContentUpdateResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayCachedContentUpdateResponse) (interface{}, error)

// CachedContentDeleteResponseConverter converts GatewayCachedContentDeleteResponse to integration format.
type CachedContentDeleteResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayCachedContentDeleteResponse) (interface{}, error)

// RequestConverter is a function that converts integration-specific requests to Gateway format.
// It takes the parsed request object and returns a GatewayRequest ready for processing.
type RequestConverter func(ctx *schemas.GatewayContext, req interface{}) (*schemas.GatewayRequest, error)

// ListModelsResponseConverter is a function that converts GatewayListModelsResponse to integration-specific format.
// It takes a GatewayListModelsResponse and returns the format expected by the specific integration.
type ListModelsResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayListModelsResponse) (interface{}, error)

// TextResponseConverter is a function that converts GatewayTextCompletionResponse to integration-specific format.
// It takes a GatewayTextCompletionResponse and returns the format expected by the specific integration.
type TextResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayTextCompletionResponse) (interface{}, error)

// ChatResponseConverter is a function that converts GatewayChatResponse to integration-specific format.
// It takes a GatewayChatResponse and returns the format expected by the specific integration.
type ChatResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayChatResponse) (interface{}, error)

// AsyncChatResponseConverter is a function that converts an async job response to an integration-specific format.
// It takes an async job response and a method to convert the chat response, and returns the integration-specific format, extra headers, and an error.
type AsyncChatResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.AsyncJobResponse, chatResponseConverter ChatResponseConverter) (interface{}, map[string]string, error)

// ResponsesResponseConverter is a function that converts GatewayResponsesResponse to integration-specific format.
// It takes a GatewayResponsesResponse and returns the format expected by the specific integration.
type ResponsesResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayResponsesResponse) (interface{}, error)

// AsyncResponsesResponseConverter is a function that converts an async job response to an integration-specific format.
// It takes an async job response and a method to convert the responses response, and returns the integration-specific format, extra headers, and an error.
type AsyncResponsesResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.AsyncJobResponse, responsesResponseConverter ResponsesResponseConverter) (interface{}, map[string]string, error)

// EmbeddingResponseConverter is a function that converts GatewayEmbeddingResponse to integration-specific format.
// It takes a GatewayEmbeddingResponse and returns the format expected by the specific integration.
type EmbeddingResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayEmbeddingResponse) (interface{}, error)

// RerankResponseConverter is a function that converts GatewayRerankResponse to integration-specific format.
// It takes a GatewayRerankResponse and returns the format expected by the specific integration.
type RerankResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayRerankResponse) (interface{}, error)

// OCRResponseConverter is a function that converts GatewayOCRResponse to integration-specific format.
// It takes a GatewayOCRResponse and returns the format expected by the specific integration.
type OCRResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayOCRResponse) (interface{}, error)

// SpeechResponseConverter is a function that converts GatewaySpeechResponse to integration-specific format.
// It takes a GatewaySpeechResponse and returns the format expected by the specific integration.
type SpeechResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewaySpeechResponse) (interface{}, error)

// TranscriptionResponseConverter is a function that converts GatewayTranscriptionResponse to integration-specific format.
// It takes a GatewayTranscriptionResponse and returns the format expected by the specific integration.
type TranscriptionResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayTranscriptionResponse) (interface{}, error)

// BatchCreateResponseConverter is a function that converts GatewayBatchCreateResponse to integration-specific format.
// It takes a GatewayBatchCreateResponse and returns the format expected by the specific integration.
type BatchCreateResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayBatchCreateResponse) (interface{}, error)

// BatchListResponseConverter is a function that converts GatewayBatchListResponse to integration-specific format.
// It takes a GatewayBatchListResponse and returns the format expected by the specific integration.
type BatchListResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayBatchListResponse) (interface{}, error)

// BatchRetrieveResponseConverter is a function that converts GatewayBatchRetrieveResponse to integration-specific format.
// It takes a GatewayBatchRetrieveResponse and returns the format expected by the specific integration.
type BatchRetrieveResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayBatchRetrieveResponse) (interface{}, error)

// BatchCancelResponseConverter is a function that converts GatewayBatchCancelResponse to integration-specific format.
// It takes a GatewayBatchCancelResponse and returns the format expected by the specific integration.
type BatchCancelResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayBatchCancelResponse) (interface{}, error)

// BatchResultsResponseConverter is a function that converts GatewayBatchResultsResponse to integration-specific format.
// It takes a GatewayBatchResultsResponse and returns the format expected by the specific integration.
type BatchResultsResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayBatchResultsResponse) (interface{}, error)

// BatchDeleteResponseConverter is a function that converts GatewayBatchDeleteResponse to integration-specific format.
// It takes a GatewayBatchDeleteResponse and returns the format expected by the specific integration.
type BatchDeleteResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayBatchDeleteResponse) (interface{}, error)

// FileUploadResponseConverter is a function that converts GatewayFileUploadResponse to integration-specific format.
// It takes a GatewayFileUploadResponse and returns the format expected by the specific integration.
type FileUploadResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayFileUploadResponse) (interface{}, error)

// FileListResponseConverter is a function that converts GatewayFileListResponse to integration-specific format.
// It takes a GatewayFileListResponse and returns the format expected by the specific integration.
type FileListResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayFileListResponse) (interface{}, error)

// FileRetrieveResponseConverter is a function that converts GatewayFileRetrieveResponse to integration-specific format.
// It takes a GatewayFileRetrieveResponse and returns the format expected by the specific integration.
type FileRetrieveResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayFileRetrieveResponse) (interface{}, error)

// FileDeleteResponseConverter is a function that converts GatewayFileDeleteResponse to integration-specific format.
// It takes a GatewayFileDeleteResponse and returns the format expected by the specific integration.
type FileDeleteResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayFileDeleteResponse) (interface{}, error)

// FileContentResponseConverter is a function that converts GatewayFileContentResponse to integration-specific format.
// It takes a GatewayFileContentResponse and returns the format expected by the specific integration.
// Note: This may return binary data or a wrapper object depending on the integration.
type FileContentResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayFileContentResponse) (interface{}, error)

// ContainerCreateResponseConverter is a function that converts GatewayContainerCreateResponse to integration-specific format.
// It takes a GatewayContainerCreateResponse and returns the format expected by the specific integration.
type ContainerCreateResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayContainerCreateResponse) (interface{}, error)

// ContainerListResponseConverter is a function that converts GatewayContainerListResponse to integration-specific format.
// It takes a GatewayContainerListResponse and returns the format expected by the specific integration.
type ContainerListResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayContainerListResponse) (interface{}, error)

// ContainerRetrieveResponseConverter is a function that converts GatewayContainerRetrieveResponse to integration-specific format.
// It takes a GatewayContainerRetrieveResponse and returns the format expected by the specific integration.
type ContainerRetrieveResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayContainerRetrieveResponse) (interface{}, error)

// ContainerDeleteResponseConverter is a function that converts GatewayContainerDeleteResponse to integration-specific format.
// It takes a GatewayContainerDeleteResponse and returns the format expected by the specific integration.
type ContainerDeleteResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayContainerDeleteResponse) (interface{}, error)

// ContainerFileCreateResponseConverter is a function that converts GatewayContainerFileCreateResponse to integration-specific format.
// It takes a GatewayContainerFileCreateResponse and returns the format expected by the specific integration.
type ContainerFileCreateResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayContainerFileCreateResponse) (interface{}, error)

// ContainerFileListResponseConverter is a function that converts GatewayContainerFileListResponse to integration-specific format.
// It takes a GatewayContainerFileListResponse and returns the format expected by the specific integration.
type ContainerFileListResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayContainerFileListResponse) (interface{}, error)

// ContainerFileRetrieveResponseConverter is a function that converts GatewayContainerFileRetrieveResponse to integration-specific format.
// It takes a GatewayContainerFileRetrieveResponse and returns the format expected by the specific integration.
type ContainerFileRetrieveResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayContainerFileRetrieveResponse) (interface{}, error)

// ContainerFileContentResponseConverter is a function that converts GatewayContainerFileContentResponse to integration-specific format.
// It takes a GatewayContainerFileContentResponse and returns the format expected by the specific integration.
type ContainerFileContentResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayContainerFileContentResponse) (interface{}, error)

// ContainerFileDeleteResponseConverter is a function that converts GatewayContainerFileDeleteResponse to integration-specific format.
// It takes a GatewayContainerFileDeleteResponse and returns the format expected by the specific integration.
type ContainerFileDeleteResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayContainerFileDeleteResponse) (interface{}, error)

// CountTokensResponseConverter is a function that converts GatewayCountTokensResponse to integration-specific format.
// It takes a GatewayCountTokensResponse and returns the format expected by the specific integration.
type CountTokensResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayCountTokensResponse) (interface{}, error)

// CompactionResponseConverter is a function that converts GatewayCompactionResponse to integration-specific format.
type CompactionResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayCompactionResponse) (interface{}, error)

// TextStreamResponseConverter is a function that converts GatewayTextCompletionResponse to integration-specific streaming format.
// It takes a GatewayTextCompletionResponse and returns the event type and the streaming format expected by the specific integration.
type TextStreamResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayTextCompletionResponse) (string, interface{}, error)

// ChatStreamResponseConverter is a function that converts GatewayChatResponse to integration-specific streaming format.
// It takes a GatewayChatResponse and returns the event type and the streaming format expected by the specific integration.
type ChatStreamResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayChatResponse) (string, interface{}, error)

// ResponsesStreamResponseConverter is a function that converts GatewayResponsesStreamResponse to integration-specific streaming format.
// It takes a GatewayResponsesStreamResponse and returns a single event type and payload, which can itself encode one or more SSE events if needed by the integration.
type ResponsesStreamResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayResponsesStreamResponse) (string, interface{}, error)

// SpeechStreamResponseConverter is a function that converts GatewaySpeechStreamResponse to integration-specific streaming format.
// It takes a GatewaySpeechStreamResponse and returns the event type and the streaming format expected by the specific integration.
type SpeechStreamResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewaySpeechStreamResponse) (string, interface{}, error)

// TranscriptionStreamResponseConverter is a function that converts GatewayTranscriptionStreamResponse to integration-specific streaming format.
// It takes a GatewayTranscriptionStreamResponse and returns the event type and the streaming format expected by the specific integration.
type TranscriptionStreamResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayTranscriptionStreamResponse) (string, interface{}, error)

// ImageGenerationResponseConverter is a function that converts GatewayImageGenerationResponse to integration-specific format.
// It takes a GatewayImageGenerationResponse and returns the format expected by the specific integration.
type ImageGenerationResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayImageGenerationResponse) (interface{}, error)

// ImageGenerationStreamResponseConverter is a function that converts GatewayImageGenerationStreamResponse to integration-specific streaming format.
// It takes a GatewayImageGenerationStreamResponse and returns the event type and the streaming format expected by the specific integration.
type ImageGenerationStreamResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayImageGenerationStreamResponse) (string, interface{}, error)

// ImageEditResponseConverter is a function that converts GatewayImageGenerationResponse to integration-specific format.
// It takes a GatewayImageGenerationResponse and returns the format expected by the specific integration.
type ImageEditResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayImageGenerationResponse) (interface{}, error)

// VideoGenerationResponseConverter is a function that converts GatewayVideoGenerationResponse to integration-specific format.
// It takes a GatewayVideoGenerationResponse and returns the format expected by the specific integration.
type VideoGenerationResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayVideoGenerationResponse) (interface{}, error)

// VideoDownloadResponseConverter is a function that converts GatewayVideoDownloadResponse to integration-specific format.
// It takes a GatewayVideoDownloadResponse and returns the format expected by the specific integration.
type VideoDownloadResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayVideoDownloadResponse) (interface{}, error)

// VideoRetrieveAsDownloadConverter is a function that converts GatewayVideoGenerationResponse to integration-specific format.
// It takes a GatewayVideoGenerationResponse and returns the format expected by the specific integration.
type VideoRetrieveAsDownloadConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayVideoGenerationResponse) (interface{}, error)

// VideoDeleteResponseConverter is a function that converts GatewayVideoDeleteResponse to integration-specific format.
// It takes a GatewayVideoDeleteResponse and returns the format expected by the specific integration.
type VideoDeleteResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayVideoDeleteResponse) (interface{}, error)

// VideoListResponseConverter is a function that converts GatewayVideoListResponse to integration-specific format.
// It takes a GatewayVideoListResponse and returns the format expected by the specific integration.
type VideoListResponseConverter func(ctx *schemas.GatewayContext, resp *schemas.GatewayVideoListResponse) (interface{}, error)

// ErrorConverter is a function that converts GatewayError to integration-specific format.
// It takes a GatewayError and returns the format expected by the specific integration.
type ErrorConverter func(ctx *schemas.GatewayContext, err *schemas.GatewayError) interface{}

// StreamErrorConverter is a function that converts GatewayError to integration-specific streaming error format.
// It takes a GatewayError and returns the streaming error format expected by the specific integration.
type StreamErrorConverter func(ctx *schemas.GatewayContext, err *schemas.GatewayError) interface{}

// RequestParser is a function that handles custom request body parsing.
// It replaces the default JSON parsing when configured (e.g., for multipart/form-data).
// The parser should populate the provided request object from the fasthttp context.
// If it returns an error, the request processing stops.
type RequestParser func(ctx *fasthttp.RequestCtx, req interface{}) error

func parseJSONRequestBody(rawBody []byte, req interface{}) error {
	if len(rawBody) == 0 {
		return nil
	}
	if err := sonic.Unmarshal(rawBody, req); err != nil {
		return fmt.Errorf("invalid JSON request body (length %d): %w", len(rawBody), err)
	}
	return nil
}

// PreRequestCallback is called after parsing the request but before processing through Gateway.
// It can be used to modify the request object (e.g., extract model from URL parameters)
// or perform validation. If it returns an error, the request processing stops.
// It can also modify the gateway context based on the request context before it is given to Gateway.
type PreRequestCallback func(ctx *fasthttp.RequestCtx, gatewayCtx *schemas.GatewayContext, req interface{}) error

// PostRequestCallback is called after processing the request but before sending the response.
// It can be used to modify the response or perform additional logging/metrics.
// If it returns an error, an error response is sent instead of the success response.
type PostRequestCallback func(ctx *fasthttp.RequestCtx, req interface{}, resp interface{}) error

// HTTPRequestTypeGetter is a function type that accepts only a *fasthttp.RequestCtx and
// returns a schemas.RequestType indicating the HTTP request type derived from the context.
type HTTPRequestTypeGetter func(ctx *fasthttp.RequestCtx) schemas.RequestType

// ShortCircuit is a function that determines if the request should be short-circuited.
type ShortCircuit func(ctx *fasthttp.RequestCtx, gatewayCtx *schemas.GatewayContext, req interface{}) (bool, error)

// StreamConfig defines streaming-specific configuration for an integration
//
// SSE FORMAT BEHAVIOR:
//
// The ResponseConverter and ErrorConverter functions in StreamConfig can return either:
//
// 1. OBJECTS (interface{} that's not a string):
//   - Will be JSON marshaled and sent as standard SSE: data: {json}\n\n
//   - Use this for most providers (OpenAI, Google, etc.)
//   - Example: return map[string]interface{}{"delta": {"content": "hello"}}
//   - Result: data: {"delta":{"content":"hello"}}\n\n
//
// 2. STRINGS:
//   - Will be sent directly as-is without any modification
//   - Use this for providers requiring custom SSE event types (Anthropic, etc.)
//   - Example: return "event: content_block_delta\ndata: {\"type\":\"text\"}\n\n"
//   - Result: event: content_block_delta
//     data: {"type":"text"}
//
// Choose the appropriate return type based on your provider's SSE specification.
type StreamConfig struct {
	TextStreamResponseConverter            TextStreamResponseConverter            // Function to convert GatewayTextCompletionResponse to streaming format
	ChatStreamResponseConverter            ChatStreamResponseConverter            // Function to convert GatewayChatResponse to streaming format
	ResponsesStreamResponseConverter       ResponsesStreamResponseConverter       // Function to convert GatewayResponsesResponse to streaming format
	SpeechStreamResponseConverter          SpeechStreamResponseConverter          // Function to convert GatewaySpeechResponse to streaming format
	TranscriptionStreamResponseConverter   TranscriptionStreamResponseConverter   // Function to convert GatewayTranscriptionResponse to streaming format
	ImageGenerationStreamResponseConverter ImageGenerationStreamResponseConverter // Function to convert GatewayImageGenerationStreamResponse to streaming format
	ErrorConverter                         StreamErrorConverter                   // Function to convert GatewayError to streaming error format
}

type RouteConfigType string

const (
	RouteConfigTypeOpenAI    RouteConfigType = "openai"
	RouteConfigTypeAnthropic RouteConfigType = "anthropic"
	RouteConfigTypeGenAI     RouteConfigType = "genai"
	RouteConfigTypeBedrock   RouteConfigType = "bedrock"
	RouteConfigTypeCohere    RouteConfigType = "cohere"
)

// RouteConfig defines the configuration for a single route in an integration.
// It specifies the path, method, and handlers for request/response conversion.
type RouteConfig struct {
	Type                                   RouteConfigType                        // Type of the route
	Path                                   string                                 // HTTP path pattern (e.g., "/openai/v1/chat/completions")
	Method                                 string                                 // HTTP method (POST, GET, PUT, DELETE)
	GetHTTPRequestType                     HTTPRequestTypeGetter                  // Function to get the HTTP request type from the context (SHOULD NOT BE NIL)
	GetRequestTypeInstance                 func(ctx context.Context) interface{}  // Factory function to create request instance (SHOULD NOT BE NIL)
	RequestParser                          RequestParser                          // Optional: custom request parsing (e.g., multipart/form-data)
	RequestConverter                       RequestConverter                       // Function to convert request to GatewayRequest (for inference requests)
	BatchRequestConverter                  BatchRequestConverter                  // Function to convert request to BatchRequest (for batch operations)
	FileRequestConverter                   FileRequestConverter                   // Function to convert request to FileRequest (for file operations)
	ContainerRequestConverter              ContainerRequestConverter              // Function to convert request to ContainerRequest (for container operations)
	ContainerFileRequestConverter          ContainerFileRequestConverter          // Function to convert request to ContainerFileRequest (for container file operations)
	CachedContentRequestConverter          CachedContentRequestConverter          // Function to convert request to CachedContentRequest (for cached content lifecycle)
	CachedContentCreateResponseConverter   CachedContentCreateResponseConverter   // Optional response converter for cached content create
	CachedContentListResponseConverter     CachedContentListResponseConverter     // Optional response converter for cached content list
	CachedContentRetrieveResponseConverter CachedContentRetrieveResponseConverter // Optional response converter for cached content retrieve
	CachedContentUpdateResponseConverter   CachedContentUpdateResponseConverter   // Optional response converter for cached content update
	CachedContentDeleteResponseConverter   CachedContentDeleteResponseConverter   // Optional response converter for cached content delete
	ListModelsResponseConverter            ListModelsResponseConverter            // Function to convert GatewayListModelsResponse to integration format (SHOULD NOT BE NIL)
	TextResponseConverter                  TextResponseConverter                  // Function to convert GatewayTextCompletionResponse to integration format (SHOULD NOT BE NIL)
	ChatResponseConverter                  ChatResponseConverter                  // Function to convert GatewayChatResponse to integration format (SHOULD NOT BE NIL)
	AsyncChatResponseConverter             AsyncChatResponseConverter             // Function to convert AsyncJobResponse to integration format (SHOULD NOT BE NIL)
	ResponsesResponseConverter             ResponsesResponseConverter             // Function to convert GatewayResponsesResponse to integration format (SHOULD NOT BE NIL)
	AsyncResponsesResponseConverter        AsyncResponsesResponseConverter        // Function to convert AsyncJobResponse to integration format (SHOULD NOT BE NIL)
	EmbeddingResponseConverter             EmbeddingResponseConverter             // Function to convert GatewayEmbeddingResponse to integration format (SHOULD NOT BE NIL)
	RerankResponseConverter                RerankResponseConverter                // Function to convert GatewayRerankResponse to integration format
	OCRResponseConverter                   OCRResponseConverter                   // Function to convert GatewayOCRResponse to integration format
	SpeechResponseConverter                SpeechResponseConverter                // Function to convert GatewaySpeechResponse to integration format (SHOULD NOT BE NIL)
	TranscriptionResponseConverter         TranscriptionResponseConverter         // Function to convert GatewayTranscriptionResponse to integration format (SHOULD NOT BE NIL)
	ImageGenerationResponseConverter       ImageGenerationResponseConverter       // Function to convert GatewayImageGenerationResponse to integration format (SHOULD NOT BE NIL)
	VideoGenerationResponseConverter       VideoGenerationResponseConverter       // Function to convert GatewayVideoGenerationResponse to integration format (SHOULD NOT BE NIL)
	VideoDownloadResponseConverter         VideoDownloadResponseConverter         // Function to convert GatewayVideoDownloadResponse to integration format (SHOULD NOT BE NIL)
	VideoDeleteResponseConverter           VideoDeleteResponseConverter           // Function to convert GatewayVideoDeleteResponse to integration format (SHOULD NOT BE NIL)
	VideoListResponseConverter             VideoListResponseConverter             // Function to convert GatewayVideoListResponse to integration format (SHOULD NOT BE NIL)
	BatchCreateResponseConverter           BatchCreateResponseConverter           // Function to convert GatewayBatchCreateResponse to integration format
	BatchListResponseConverter             BatchListResponseConverter             // Function to convert GatewayBatchListResponse to integration format
	BatchRetrieveResponseConverter         BatchRetrieveResponseConverter         // Function to convert GatewayBatchRetrieveResponse to integration format
	BatchCancelResponseConverter           BatchCancelResponseConverter           // Function to convert GatewayBatchCancelResponse to integration format
	BatchDeleteResponseConverter           BatchDeleteResponseConverter           // Function to convert GatewayBatchDeleteResponse to integration format
	BatchResultsResponseConverter          BatchResultsResponseConverter          // Function to convert GatewayBatchResultsResponse to integration format
	FileUploadResponseConverter            FileUploadResponseConverter            // Function to convert GatewayFileUploadResponse to integration format
	FileListResponseConverter              FileListResponseConverter              // Function to convert GatewayFileListResponse to integration format
	FileRetrieveResponseConverter          FileRetrieveResponseConverter          // Function to convert GatewayFileRetrieveResponse to integration format
	FileDeleteResponseConverter            FileDeleteResponseConverter            // Function to convert GatewayFileDeleteResponse to integration format
	FileContentResponseConverter           FileContentResponseConverter           // Function to convert GatewayFileContentResponse to integration format
	ContainerCreateResponseConverter       ContainerCreateResponseConverter       // Function to convert GatewayContainerCreateResponse to integration format
	ContainerListResponseConverter         ContainerListResponseConverter         // Function to convert GatewayContainerListResponse to integration format
	ContainerRetrieveResponseConverter     ContainerRetrieveResponseConverter     // Function to convert GatewayContainerRetrieveResponse to integration format
	ContainerDeleteResponseConverter       ContainerDeleteResponseConverter       // Function to convert GatewayContainerDeleteResponse to integration format
	ContainerFileCreateResponseConverter   ContainerFileCreateResponseConverter   // Function to convert GatewayContainerFileCreateResponse to integration format
	ContainerFileListResponseConverter     ContainerFileListResponseConverter     // Function to convert GatewayContainerFileListResponse to integration format
	ContainerFileRetrieveResponseConverter ContainerFileRetrieveResponseConverter // Function to convert GatewayContainerFileRetrieveResponse to integration format
	ContainerFileContentResponseConverter  ContainerFileContentResponseConverter  // Function to convert GatewayContainerFileContentResponse to integration format
	ContainerFileDeleteResponseConverter   ContainerFileDeleteResponseConverter   // Function to convert GatewayContainerFileDeleteResponse to integration format
	CountTokensResponseConverter           CountTokensResponseConverter           // Function to convert GatewayCountTokensResponse to integration format
	CompactionResponseConverter            CompactionResponseConverter            // Function to convert GatewayCompactionResponse to integration format
	ErrorConverter                         ErrorConverter                         // Function to convert GatewayError to integration format (SHOULD NOT BE NIL)
	StreamConfig                           *StreamConfig                          // Optional: Streaming configuration (if nil, streaming not supported)
	PreCallback                            PreRequestCallback                     // Optional: called after parsing but before Gateway processing
	PostCallback                           PostRequestCallback                    // Optional: called after request processing
	ShortCircuit                           ShortCircuit
}

type PassthroughConfig struct {
	Provider         schemas.ModelProvider                                              // which provider's key pool to draw from
	ProviderDetector func(ctx *fasthttp.RequestCtx, model string) schemas.ModelProvider // optional: dynamic provider detection
	StripPrefix      []string                                                           // e.g. "/openai" — stripped before forwarding
}

// LargePayloadHook is called before body parsing to detect and set up large payload streaming.
// If it returns skipBodyParse=true, the router skips JSON parsing of the request body.
// The hook is responsible for setting all relevant context keys (GatewayContextKeyLargePayloadMode,
// GatewayContextKeyLargePayloadReader, GatewayContextKeyLargePayloadContentLength,
// GatewayContextKeyLargePayloadMetadata) when activating large payload mode.
type LargePayloadHook func(
	ctx *fasthttp.RequestCtx,
	gatewayCtx *schemas.GatewayContext,
	routeType RouteConfigType,
) (skipBodyParse bool, err error)

// LargeResponseHook is called before streaming a large response body to the client.
// Enterprise uses this to wrap the response reader with Phase B scanning (e.g., usage extraction
// from the full response stream when usage is beyond the Phase A prefetch window).
// The hook receives the gateway context with GatewayContextKeyLargeResponseReader already set
// and may replace the reader on context with a wrapped version.
type LargeResponseHook func(
	ctx *fasthttp.RequestCtx,
	gatewayCtx *schemas.GatewayContext,
)

// GenericRouter provides a reusable router implementation for all integrations.
// It handles the common flow of: parse request → convert to Gateway → execute → convert response.
// Integration-specific logic is handled through the RouteConfig callbacks and converters.
type GenericRouter struct {
	client            *gateway.Gateway // Gateway client for executing requests
	handlerStore      lib.HandlerStore // Config provider for the router
	routes            []RouteConfig    // List of route configurations
	passthroughCfg    *PassthroughConfig
	logger            schemas.Logger    // Logger for the router
	largePayloadHook  LargePayloadHook  // Optional: enterprise hook for large payload detection
	largeResponseHook LargeResponseHook // Optional: enterprise hook for large response scanning
}

type modelCatalogProvider interface {
	GetModelCatalog() *modelcatalog.ModelCatalog
}

// SetLargePayloadHook sets the hook for large payload detection and streaming.
// This is used by enterprise to inject large payload optimization without
// embedding the logic in the OSS router.
func (g *GenericRouter) SetLargePayloadHook(hook LargePayloadHook) {
	g.largePayloadHook = hook
}

// SetLargeResponseHook sets the hook for large response scanning.
// Enterprise uses this to inject Phase B usage extraction into the response stream
// without embedding scanning logic in the OSS router.
func (g *GenericRouter) SetLargeResponseHook(hook LargeResponseHook) {
	g.largeResponseHook = hook
}

// NewGenericRouter creates a new generic router with the given gateway client and route configurations.
// Each integration should create their own routes and pass them to this constructor.
func NewGenericRouter(client *gateway.Gateway, handlerStore lib.HandlerStore, routes []RouteConfig, passthroughCfg *PassthroughConfig, logger schemas.Logger) *GenericRouter {
	return &GenericRouter{
		client:         client,
		handlerStore:   handlerStore,
		routes:         routes,
		passthroughCfg: passthroughCfg,
		logger:         logger,
	}
}

// RegisterRoutes registers all configured routes on the given fasthttp router.
// This method implements the ExtensionRouter interface.
func (g *GenericRouter) RegisterRoutes(r *router.Router, middlewares ...schemas.GatewayHTTPMiddleware) {
	for _, route := range g.routes {
		// Validate route configuration at startup to fail fast
		method := strings.ToUpper(route.Method)

		if route.GetRequestTypeInstance == nil {
			g.logger.Warn("route configuration is invalid: GetRequestTypeInstance cannot be nil for route " + route.Path)
			continue
		}

		// Test that GetRequestTypeInstance returns a valid instance
		if testInstance := route.GetRequestTypeInstance(context.Background()); testInstance == nil {
			g.logger.Warn("route configuration is invalid: GetRequestTypeInstance returned nil for route " + route.Path)
			continue
		}

		// Determine route type: inference, batch, file, container, container file, or cached content
		isBatchRoute := route.BatchRequestConverter != nil
		isFileRoute := route.FileRequestConverter != nil
		isContainerRoute := route.ContainerRequestConverter != nil
		isContainerFileRoute := route.ContainerFileRequestConverter != nil
		isCachedContentRoute := route.CachedContentRequestConverter != nil
		isInferenceRoute := !isBatchRoute && !isFileRoute && !isContainerRoute && !isContainerFileRoute && !isCachedContentRoute

		// For inference routes, require RequestConverter
		if isInferenceRoute && route.RequestConverter == nil {
			g.logger.Warn("route configuration is invalid: RequestConverter cannot be nil for inference route " + route.Path)
			continue
		}

		if route.ErrorConverter == nil {
			g.logger.Warn("route configuration is invalid: ErrorConverter cannot be nil for route " + route.Path)
			continue
		}

		registerRequestTypeMiddleware := func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
			return func(ctx *fasthttp.RequestCtx) {
				if route.GetHTTPRequestType != nil {
					ctx.SetUserValue(schemas.GatewayContextKeyHTTPRequestType, route.GetHTTPRequestType(ctx))
				}
				next(ctx)
			}
		}

		// Create a fresh middlewares list for this route (don't mutate the original)
		// This ensures each route only has its own middleware plus the originally passed middlewares
		routeMiddlewares := append([]schemas.GatewayHTTPMiddleware{registerRequestTypeMiddleware}, middlewares...)

		handler := g.createHandler(route)
		switch method {
		case fasthttp.MethodPost:
			r.POST(route.Path, lib.ChainMiddlewares(handler, routeMiddlewares...))
		case fasthttp.MethodGet:
			r.GET(route.Path, lib.ChainMiddlewares(handler, routeMiddlewares...))
		case fasthttp.MethodPut:
			r.PUT(route.Path, lib.ChainMiddlewares(handler, routeMiddlewares...))
		case fasthttp.MethodDelete:
			r.DELETE(route.Path, lib.ChainMiddlewares(handler, routeMiddlewares...))
		case fasthttp.MethodPatch:
			r.PATCH(route.Path, lib.ChainMiddlewares(handler, routeMiddlewares...))
		case fasthttp.MethodHead:
			r.HEAD(route.Path, lib.ChainMiddlewares(handler, routeMiddlewares...))
		default:
			r.POST(route.Path, lib.ChainMiddlewares(handler, routeMiddlewares...)) // Default to POST
		}
	}

	if g.passthroughCfg != nil {
		catchAll := lib.ChainMiddlewares(g.handlePassthrough, middlewares...)
		// Register for all methods that need forwarding
		for _, method := range []string{fasthttp.MethodGet, fasthttp.MethodPost, fasthttp.MethodPut, fasthttp.MethodDelete, fasthttp.MethodPatch, fasthttp.MethodHead} {
			for _, prefix := range g.passthroughCfg.StripPrefix {
				r.Handle(method, prefix+"/{path:*}", catchAll)
			}
		}
	}
}

// createHandler creates a fasthttp handler for the given route configuration.
// The handler follows this flow:
// 1. Parse JSON request body into the configured request type (for methods that expect bodies)
// 2. Execute pre-callback (if configured) for request modification/validation
// 3. Convert request to GatewayRequest using the configured converter
// 4. Execute the request through Gateway (streaming or non-streaming)
// 5. Execute post-callback (if configured) for response modification
// 6. Convert and send the response using the configured response converter
func (g *GenericRouter) createHandler(config RouteConfig) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		method := string(ctx.Method())

		// Parse request body into the integration-specific request type
		// Note: config validation is performed at startup in RegisterRoutes
		req := config.GetRequestTypeInstance(ctx)
		var rawBody []byte

		// Execute the request through Gateway
		gatewayCtx, cancel := lib.ConvertToGatewayContext(ctx, g.handlerStore)
		// Centralized cleanup. The streaming branch below transfers ownership via
		// streamingOwnsCancel because its producer goroutine outlives this lambda.
		streamingOwnsCancel := false
		defer func() {
			if !streamingOwnsCancel {
				cancel()
			}
		}()

		// Set integration type to context. Used by the ModelCatalogResolver built-in
		// PreRequestHook (last routing layer) to prefer this integration's canonical
		// provider when the model is unprefixed and the catalog returns multiple options.
		gatewayCtx.SetValue(schemas.GatewayContextKeyIntegrationType, string(config.Type))

		// Async retrieve: check x-uf-async-id header early (before body parsing)
		if asyncID := string(ctx.Request.Header.Peek(schemas.AsyncHeaderGetID)); asyncID != "" {
			g.handleAsyncRetrieve(ctx, config, gatewayCtx)
			return
		}

		// Parse request body based on configuration
		if method != fasthttp.MethodGet && method != fasthttp.MethodHead {
			// Hook executes before JSON parsing so large requests can remain streaming.
			isLargePayload := false
			if g.largePayloadHook != nil {
				var err error
				isLargePayload, err = g.largePayloadHook(ctx, gatewayCtx, config.Type)
				if err != nil {
					g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "large payload detection failed"))
					return
				}
			}

			if isLargePayload {
				// Large payload mode: body streams directly to provider via
				// GatewayContextKeyLargePayloadReader. Skip all body parsing
				// (JSON and multipart) — metadata was already extracted by the hook.
			} else if config.RequestParser != nil {
				// Use custom parser (e.g., for multipart/form-data)
				if err := config.RequestParser(ctx, req); err != nil {
					ctx.SetConnectionClose()
					g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayErrorWithCode(err, "failed to parse request", fasthttp.StatusBadRequest))
					return
				}
			} else {
				// Use default JSON parsing
				rawBody = ctx.Request.Body()
				if len(rawBody) > 0 {
					if err := parseJSONRequestBody(rawBody, req); err != nil {
						ctx.SetConnectionClose()
						g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayErrorWithCode(err, "Invalid JSON", fasthttp.StatusBadRequest))
						return
					}
				}
			}

			// Extract the "extra_params" JSON key when passthrough is
			// explicitly enabled via x-uf-passthrough-extra-params: true.
			// Provider-specific fields (e.g. Bedrock guardrailConfig)
			// must be nested under "extra_params" in the request body.
			// Runs after both RequestParser and default JSON paths.
			if !isLargePayload && gatewayCtx.Value(schemas.GatewayContextKeyPassthroughExtraParams) == true {
				if rws, ok := req.(RequestWithSettableExtraParams); ok {
					if rawBody == nil {
						rawBody = ctx.Request.Body()
					}
					if len(rawBody) > 0 {
						var wrapper struct {
							ExtraParams map[string]interface{} `json:"extra_params"`
						}
						if err := sonic.Unmarshal(rawBody, &wrapper); err == nil && len(wrapper.ExtraParams) > 0 {
							rws.SetExtraParams(wrapper.ExtraParams)
						}
					}
				}
			}
		}

		// Execute pre-request callback if configured
		// This is typically used for extracting data from URL parameters
		// or performing request validation after parsing
		if config.PreCallback != nil {
			if err := config.PreCallback(ctx, gatewayCtx, req); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute pre-request callback: "+err.Error()))
				return
			}
		}

		// Execute short-circuit handler if configured.
		// If it returns handled=true the callback has already written a response
		// to ctx and we return immediately, bypassing the Gateway flow entirely.
		if config.ShortCircuit != nil {
			handled, err := config.ShortCircuit(ctx, gatewayCtx, req)
			if err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "short-circuit handler error: "+err.Error()))
				return
			}
			if handled {
				return
			}
		}

		// Handle batch requests if BatchRequestConverter is set
		// GenAI has two cases: (1) Dedicated batch routes (list/retrieve) have only BatchRequestConverter — always use batch path.
		// (2) The models path has both BatchRequestConverter and RequestConverter — use batch path only for batch create.
		isGenAIBatchCreate := config.Type == RouteConfigTypeGenAI && gatewayCtx.Value(isGeminiBatchCreateRequestContextKey) != nil
		useBatchPath := config.BatchRequestConverter != nil && (config.RequestConverter == nil || config.Type != RouteConfigTypeGenAI || isGenAIBatchCreate)
		if useBatchPath {
			batchReq, err := config.BatchRequestConverter(gatewayCtx, req)
			if err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert batch request"))
				return
			}
			if batchReq == nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid batch request"))
				return
			}
			g.handleBatchRequest(ctx, config, req, batchReq, gatewayCtx)
			return
		}
		// Handle file requests if FileRequestConverter is set
		if config.FileRequestConverter != nil {
			fileReq, err := config.FileRequestConverter(gatewayCtx, req)
			if err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert file request"))
				return
			}
			if fileReq == nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid file request"))
				return
			}
			g.handleFileRequest(ctx, config, req, fileReq, gatewayCtx)
			return
		}

		// Handle container requests if ContainerRequestConverter is set
		if config.ContainerRequestConverter != nil {
			containerReq, err := config.ContainerRequestConverter(gatewayCtx, req)
			if err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert container request"))
				return
			}
			if containerReq == nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid container request"))
				return
			}
			g.handleContainerRequest(ctx, config, req, containerReq, gatewayCtx)
			return
		}

		// Handle container file requests if ContainerFileRequestConverter is set
		if config.ContainerFileRequestConverter != nil {
			containerFileReq, err := config.ContainerFileRequestConverter(gatewayCtx, req)
			if err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert container file request"))
				return
			}
			if containerFileReq == nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid container file request"))
				return
			}
			g.handleContainerFileRequest(ctx, config, req, containerFileReq, gatewayCtx)
			return
		}

		// Handle cached content requests if CachedContentRequestConverter is set
		if config.CachedContentRequestConverter != nil {
			cachedContentReq, err := config.CachedContentRequestConverter(gatewayCtx, req)
			if err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert cached content request"))
				return
			}
			if cachedContentReq == nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid cached content request"))
				return
			}
			g.handleCachedContentRequest(ctx, config, req, cachedContentReq, gatewayCtx)
			return
		}

		// Convert the integration-specific request to Gateway format (inference requests)
		gatewayReq, err := config.RequestConverter(gatewayCtx, req)
		if err != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert request to Gateway format"))
			return
		}
		if gatewayReq == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid request"))
			return
		}
		if sendRawRequestBody, ok := (*gatewayCtx).Value(schemas.GatewayContextKeyUseRawRequestBody).(bool); ok && sendRawRequestBody {
			gatewayReq.SetRawRequestBody(rawBody)
		}

		// Extract and parse fallbacks from the request if present
		if err := g.extractAndParseFallbacks(gatewayCtx, req, gatewayReq); err != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to parse fallbacks: "+err.Error()))
			return
		}

		// Async create: check x-uf-async header (needs parsed gatewayReq)
		if string(ctx.Request.Header.Peek(schemas.AsyncHeaderCreate)) != "" {
			g.handleAsyncCreate(ctx, config, req, gatewayReq, gatewayCtx)
			return
		}

		// Check if streaming is requested
		isStreaming := false
		if streamingReq, ok := req.(StreamingRequest); ok {
			isStreaming = streamingReq.IsStreamingRequested()
		}

		if isStreaming {
			// Hand cancel ownership to the streaming path; its producer goroutine
			// fires cancel on client-disconnect (handleStreaming) and on pre-stream
			// errors (handleStreamingRequest).
			streamingOwnsCancel = true
			g.handleStreamingRequest(ctx, config, gatewayReq, gatewayCtx, cancel)
		} else {
			g.handleNonStreamingRequest(ctx, config, req, gatewayReq, gatewayCtx)
		}
	}
}

// handleNonStreamingRequest handles regular (non-streaming) requests
func (g *GenericRouter) handleNonStreamingRequest(ctx *fasthttp.RequestCtx, config RouteConfig, req interface{}, gatewayReq *schemas.GatewayRequest, gatewayCtx *schemas.GatewayContext) {
	// Use the cancellable context from ConvertToGatewayContext
	// While we can't detect client disconnects until we try to write, having a cancellable context
	// allows providers that check ctx.Done() to cancel early if needed. This is less critical than
	// streaming requests (where we actively detect write errors), but still provides a mechanism
	// for providers to respect cancellation.
	var response interface{}
	var err error
	// gatewayExtraFields snapshots the routed identity (provider, original/resolved
	// model) plus the upstream provider's response headers from whichever Gateway
	// response variant the case below populates. The common footer below surfaces
	// the routed identity as `x-gateway-*` response headers and forwards the
	// upstream provider headers verbatim.
	var gatewayExtraFields schemas.GatewayResponseExtraFields

	switch {
	case gatewayReq.ListModelsRequest != nil:
		// Determine provider: explicit header overrides request field; otherwise
		// fall back to the request field and finally to list-all behavior.
		listModelsProvider := strings.ToLower(string(ctx.Request.Header.Peek("x-uf-model-provider")))
		switch listModelsProvider {
		case "":
			// keep any provider already set on the request
		case "all":
			gatewayReq.ListModelsRequest.Provider = ""
		default:
			gatewayReq.ListModelsRequest.Provider = schemas.ModelProvider(listModelsProvider)
		}

		var listModelsResponse *schemas.GatewayListModelsResponse
		var gatewayErr *schemas.GatewayError

		if gatewayReq.ListModelsRequest.Provider != "" {
			listModelsResponse, gatewayErr = g.client.ListModelsRequest(gatewayCtx, gatewayReq.ListModelsRequest)
		} else {
			listModelsResponse, gatewayErr = g.client.ListAllModels(gatewayCtx, gatewayReq.ListModelsRequest)
		}

		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, listModelsResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if listModelsResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}
		g.filterDeprecatedListModelsResponse(listModelsResponse)

		response, err = config.ListModelsResponseConverter(gatewayCtx, listModelsResponse)
		gatewayExtraFields = listModelsResponse.ExtraFields
	case gatewayReq.TextCompletionRequest != nil:
		textCompletionResponse, gatewayErr := g.client.TextCompletionRequest(gatewayCtx, gatewayReq.TextCompletionRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		// Execute post-request callback if configured
		// This is typically used for response modification or additional processing
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, textCompletionResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if textCompletionResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		// Convert Gateway response to integration-specific format and send
		response, err = config.TextResponseConverter(gatewayCtx, textCompletionResponse)
		gatewayExtraFields = textCompletionResponse.ExtraFields
	case gatewayReq.ChatRequest != nil:
		chatResponse, gatewayErr := g.client.ChatCompletionRequest(gatewayCtx, gatewayReq.ChatRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		// Execute post-request callback if configured
		// This is typically used for response modification or additional processing
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, chatResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if chatResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		// Convert Gateway response to integration-specific format and send
		response, err = config.ChatResponseConverter(gatewayCtx, chatResponse)
		gatewayExtraFields = chatResponse.ExtraFields
	case gatewayReq.ResponsesRequest != nil:
		responsesResponse, gatewayErr := g.client.ResponsesRequest(gatewayCtx, gatewayReq.ResponsesRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		// Execute post-request callback if configured
		// This is typically used for response modification or additional processing
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, responsesResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if responsesResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		// Convert Gateway response to integration-specific format and send
		response, err = config.ResponsesResponseConverter(gatewayCtx, responsesResponse)
		gatewayExtraFields = responsesResponse.ExtraFields
	case gatewayReq.EmbeddingRequest != nil:
		embeddingResponse, gatewayErr := g.client.EmbeddingRequest(gatewayCtx, gatewayReq.EmbeddingRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		// Execute post-request callback if configured
		// This is typically used for response modification or additional processing
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, embeddingResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if embeddingResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}
		gatewayExtraFields = embeddingResponse.ExtraFields
		// Convert Gateway response to integration-specific format and send
		response, err = config.EmbeddingResponseConverter(gatewayCtx, embeddingResponse)
	case gatewayReq.RerankRequest != nil:
		rerankResponse, gatewayErr := g.client.RerankRequest(gatewayCtx, gatewayReq.RerankRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, rerankResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if rerankResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}
		gatewayExtraFields = rerankResponse.ExtraFields
		if config.RerankResponseConverter != nil {
			response, err = config.RerankResponseConverter(gatewayCtx, rerankResponse)
		} else {
			response = rerankResponse
		}

	case gatewayReq.OCRRequest != nil:
		ocrResponse, gatewayErr := g.client.OCRRequest(gatewayCtx, gatewayReq.OCRRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, ocrResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if ocrResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "gateway response is nil after post-request callback"))
			return
		}
		gatewayExtraFields = ocrResponse.ExtraFields
		if config.OCRResponseConverter != nil {
			response, err = config.OCRResponseConverter(gatewayCtx, ocrResponse)
		} else {
			response = ocrResponse
		}

	case gatewayReq.SpeechRequest != nil:
		speechResponse, gatewayErr := g.client.SpeechRequest(gatewayCtx, gatewayReq.SpeechRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, speechResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if speechResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		gatewayExtraFields = speechResponse.ExtraFields

		if g.tryStreamLargeResponse(ctx, gatewayCtx) {
			return
		}

		if config.SpeechResponseConverter != nil {
			response, err = config.SpeechResponseConverter(gatewayCtx, speechResponse)
			if err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert speech response"))
				return
			}
			g.sendSuccess(ctx, gatewayCtx, config.ErrorConverter, response, nil)
			return
		} else {
			ctx.Response.Header.Set("Content-Type", "audio/mpeg")
			ctx.Response.Header.Set("Content-Disposition", "attachment; filename=speech.mp3")
			ctx.Response.Header.Set("Content-Length", strconv.Itoa(len(speechResponse.Audio)))
			ctx.Response.SetBody(speechResponse.Audio)
			return
		}
	case gatewayReq.TranscriptionRequest != nil:
		transcriptionResponse, gatewayErr := g.client.TranscriptionRequest(gatewayCtx, gatewayReq.TranscriptionRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		// Execute post-request callback if configured
		// This is typically used for response modification or additional processing
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, transcriptionResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if transcriptionResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		if g.tryStreamLargeResponse(ctx, gatewayCtx) {
			return
		}

		// Convert Gateway response to integration-specific format and send
		response, err = config.TranscriptionResponseConverter(gatewayCtx, transcriptionResponse)
		gatewayExtraFields = transcriptionResponse.ExtraFields

		// If converter returns raw bytes, write directly with provider headers.
		// Used for plain-text transcription formats (text, srt, vtt).
		if err == nil {
			if rawBytes, ok := response.([]byte); ok {
				applyGatewayResponseHeaders(ctx, gatewayCtx, gatewayExtraFields)
				ctx.SetStatusCode(fasthttp.StatusOK)
				ctx.SetBody(rawBytes)
				return
			}
		}
	case gatewayReq.ImageGenerationRequest != nil:
		imageGenerationResponse, gatewayErr := g.client.ImageGenerationRequest(gatewayCtx, gatewayReq.ImageGenerationRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		// Execute post-request callback if configured
		// This is typically used for response modification or additional processing
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, imageGenerationResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if imageGenerationResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		if config.ImageGenerationResponseConverter == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "missing ImageGenerationResponseConverter for integration"))
			return
		}

		if g.tryStreamLargeResponse(ctx, gatewayCtx) {
			return
		}

		// Convert Gateway response to integration-specific format and send
		response, err = config.ImageGenerationResponseConverter(gatewayCtx, imageGenerationResponse)
		gatewayExtraFields = imageGenerationResponse.ExtraFields
	case gatewayReq.ImageEditRequest != nil:
		imageEditResponse, gatewayErr := g.client.ImageEditRequest(gatewayCtx, gatewayReq.ImageEditRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		// Execute post-request callback if configured
		// This is typically used for response modification or additional processing
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, imageEditResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if imageEditResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		if config.ImageGenerationResponseConverter == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "missing ImageGenerationResponseConverter for integration"))
			return
		}

		if g.tryStreamLargeResponse(ctx, gatewayCtx) {
			return
		}

		// Convert Gateway response to integration-specific format and send
		response, err = config.ImageGenerationResponseConverter(gatewayCtx, imageEditResponse)
		gatewayExtraFields = imageEditResponse.ExtraFields
	case gatewayReq.ImageVariationRequest != nil:
		imageVariationResponse, gatewayErr := g.client.ImageVariationRequest(gatewayCtx, gatewayReq.ImageVariationRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		// Execute post-request callback if configured
		// This is typically used for response modification or additional processing
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, imageVariationResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if imageVariationResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		if config.ImageGenerationResponseConverter == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "missing ImageGenerationResponseConverter for integration"))
			return
		}

		if g.tryStreamLargeResponse(ctx, gatewayCtx) {
			return
		}

		// Convert Gateway response to integration-specific format and send
		response, err = config.ImageGenerationResponseConverter(gatewayCtx, imageVariationResponse)
		gatewayExtraFields = imageVariationResponse.ExtraFields
	case gatewayReq.VideoGenerationRequest != nil:
		videoGenerationResponse, gatewayErr := g.client.VideoGenerationRequest(gatewayCtx, gatewayReq.VideoGenerationRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, videoGenerationResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if videoGenerationResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		if config.VideoGenerationResponseConverter == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "missing VideoGenerationResponseConverter for integration"))
			return
		}

		response, err = config.VideoGenerationResponseConverter(gatewayCtx, videoGenerationResponse)
		gatewayExtraFields = videoGenerationResponse.ExtraFields
	case gatewayReq.VideoRetrieveRequest != nil:
		videoRetrieveResponse, gatewayErr := g.client.VideoRetrieveRequest(gatewayCtx, gatewayReq.VideoRetrieveRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, videoRetrieveResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if videoRetrieveResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		if config.VideoGenerationResponseConverter == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "missing VideoGenerationResponseConverter for integration"))
			return
		}
		response, err = config.VideoGenerationResponseConverter(gatewayCtx, videoRetrieveResponse)
		gatewayExtraFields = videoRetrieveResponse.ExtraFields
	case gatewayReq.VideoDownloadRequest != nil:
		videoDownloadResponse, gatewayErr := g.client.VideoDownloadRequest(gatewayCtx, gatewayReq.VideoDownloadRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, videoDownloadResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if videoDownloadResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		if config.VideoDownloadResponseConverter == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "missing VideoDownloadResponseConverter for integration"))
			return
		}

		response, err = config.VideoDownloadResponseConverter(gatewayCtx, videoDownloadResponse)
		gatewayExtraFields = videoDownloadResponse.ExtraFields

		// If converter returns binary content, write directly with content-type.
		if err == nil {
			if rawBytes, ok := response.([]byte); ok {
				contentType := videoDownloadResponse.ContentType
				if contentType == "" {
					contentType = "application/octet-stream"
				}
				ctx.Response.Header.Set("Content-Type", contentType)
				ctx.Response.Header.Set("Content-Length", strconv.Itoa(len(rawBytes)))
				ctx.Response.SetBody(rawBytes)
				return
			}
		}
	case gatewayReq.VideoDeleteRequest != nil:
		videoDeleteResponse, gatewayErr := g.client.VideoDeleteRequest(gatewayCtx, gatewayReq.VideoDeleteRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, videoDeleteResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if videoDeleteResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		if config.VideoDeleteResponseConverter == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "missing VideoDeleteResponseConverter for integration"))
			return
		}

		response, err = config.VideoDeleteResponseConverter(gatewayCtx, videoDeleteResponse)
		gatewayExtraFields = videoDeleteResponse.ExtraFields
	case gatewayReq.VideoRemixRequest != nil:
		videoRemixResponse, gatewayErr := g.client.VideoRemixRequest(gatewayCtx, gatewayReq.VideoRemixRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, videoRemixResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if videoRemixResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		if config.VideoGenerationResponseConverter == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "missing VideoGenerationResponseConverter for integration"))
			return
		}

		response, err = config.VideoGenerationResponseConverter(gatewayCtx, videoRemixResponse)
		gatewayExtraFields = videoRemixResponse.ExtraFields
	case gatewayReq.VideoListRequest != nil:

		// extract provider from header
		providerHeader := strings.ToLower(string(ctx.Request.Header.Peek("x-uf-video-list-provider")))
		if providerHeader != "" {
			gatewayReq.VideoListRequest.Provider = schemas.ModelProvider(providerHeader)
		} else if gatewayReq.VideoListRequest.Provider == "" {
			gatewayReq.VideoListRequest.Provider = schemas.OpenAI
		}
		videoListResponse, gatewayErr := g.client.VideoListRequest(gatewayCtx, gatewayReq.VideoListRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, videoListResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if videoListResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		if config.VideoListResponseConverter == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "missing VideoListResponseConverter for integration"))
			return
		}

		response, err = config.VideoListResponseConverter(gatewayCtx, videoListResponse)
		gatewayExtraFields = videoListResponse.ExtraFields

	case gatewayReq.ResponsesRetrieveRequest != nil:
		responsesRetrieveResponse, gatewayErr := g.client.ResponsesRetrieveRequest(gatewayCtx, gatewayReq.ResponsesRetrieveRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, responsesRetrieveResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if responsesRetrieveResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}
		if config.ResponsesResponseConverter == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "missing ResponsesResponseConverter for integration"))
			return
		}
		response, err = config.ResponsesResponseConverter(gatewayCtx, responsesRetrieveResponse)
		gatewayExtraFields = responsesRetrieveResponse.ExtraFields

	case gatewayReq.ResponsesDeleteRequest != nil:
		responsesDeleteResponse, gatewayErr := g.client.ResponsesDeleteRequest(gatewayCtx, gatewayReq.ResponsesDeleteRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, responsesDeleteResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if responsesDeleteResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}
		response = responsesDeleteResponse
		gatewayExtraFields = responsesDeleteResponse.ExtraFields

	case gatewayReq.ResponsesCancelRequest != nil:
		responsesCancelResponse, gatewayErr := g.client.ResponsesCancelRequest(gatewayCtx, gatewayReq.ResponsesCancelRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, responsesCancelResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if responsesCancelResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}
		if config.ResponsesResponseConverter == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "missing ResponsesResponseConverter for integration"))
			return
		}
		response, err = config.ResponsesResponseConverter(gatewayCtx, responsesCancelResponse)
		gatewayExtraFields = responsesCancelResponse.ExtraFields

	case gatewayReq.ResponsesInputItemsRequest != nil:
		inputItemsResponse, gatewayErr := g.client.ResponsesInputItemsRequest(gatewayCtx, gatewayReq.ResponsesInputItemsRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, inputItemsResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if inputItemsResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}
		response = inputItemsResponse
		gatewayExtraFields = inputItemsResponse.ExtraFields

	case gatewayReq.CountTokensRequest != nil:
		countTokensResponse, gatewayErr := g.client.CountTokensRequest(gatewayCtx, gatewayReq.CountTokensRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		// Execute post-request callback if configured
		// This is typically used for response modification or additional processing
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, countTokensResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if countTokensResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		// Convert Gateway response to integration-specific format and send
		if config.CountTokensResponseConverter == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "CountTokensResponseConverter not configured"))
			return
		}
		response, err = config.CountTokensResponseConverter(gatewayCtx, countTokensResponse)
		gatewayExtraFields = countTokensResponse.ExtraFields

	case gatewayReq.CompactionRequest != nil:
		compactionResponse, gatewayErr := g.client.CompactionRequest(gatewayCtx, gatewayReq.CompactionRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}

		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, compactionResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}

		if compactionResponse == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Gateway response is nil after post-request callback"))
			return
		}

		if config.CompactionResponseConverter == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "CompactionResponseConverter not configured"))
			return
		}
		response, err = config.CompactionResponseConverter(gatewayCtx, compactionResponse)
		gatewayExtraFields = compactionResponse.ExtraFields

	default:
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Invalid request type"))
		return
	}

	if err != nil {
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to encode response"))
		return
	}

	// Forward upstream provider response headers (filtered) plus the gateway-level
	// `x-gateway-*` routing identity headers, only after conversion succeeds.
	applyGatewayResponseHeaders(ctx, gatewayCtx, gatewayExtraFields)

	if g.tryStreamLargeResponse(ctx, gatewayCtx) {
		return
	}

	g.sendSuccess(ctx, gatewayCtx, config.ErrorConverter, response, nil)
}

// --- Async integration handlers ---

// handleAsyncCreate submits an async job for the current inference request.
// It stores the raw Gateway response in the DB; the response converter is applied at retrieval time.
func (g *GenericRouter) handleAsyncCreate(
	ctx *fasthttp.RequestCtx,
	config RouteConfig,
	req interface{},
	gatewayReq *schemas.GatewayRequest,
	gatewayCtx *schemas.GatewayContext,
) {
	executor := g.handlerStore.GetAsyncJobExecutor()
	if executor == nil {
		g.sendError(ctx, gatewayCtx, config.ErrorConverter,
			newGatewayError(nil, "async operations not available: logs store not configured"))
		return
	}

	// Reject streaming + async
	if streamingReq, ok := req.(StreamingRequest); ok && streamingReq.IsStreamingRequested() {
		g.sendError(ctx, gatewayCtx, config.ErrorConverter,
			newGatewayErrorWithCode(nil, "streaming is not supported for async requests", fasthttp.StatusBadRequest))
		return
	}

	// Reject non-inference routes (batch, file, container)
	if config.BatchRequestConverter != nil || config.FileRequestConverter != nil ||
		config.ContainerRequestConverter != nil || config.ContainerFileRequestConverter != nil {
		g.sendError(ctx, gatewayCtx, config.ErrorConverter,
			newGatewayError(nil, "async is not supported for batch, file, or container operations"))
		return
	}

	switch config.GetHTTPRequestType(ctx) {
	case schemas.ChatCompletionRequest:
		if config.AsyncChatResponseConverter == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "async operation is not supported on this route"))
			return
		}
	case schemas.ResponsesRequest:
		if config.AsyncResponsesResponseConverter == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "async operation is not supported on this route"))
			return
		}
	default:
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "async operation is not supported on this route"))
		return
	}

	operationType := config.GetHTTPRequestType(ctx)
	resultTTL := getResultTTLFromHeaderWithDefault(ctx, g.handlerStore.GetAsyncJobResultTTL())

	// The operation closure runs the Gateway client call in the background.
	// It returns the raw typed Gateway response (NOT provider-converted).
	// The response converter is applied at retrieval time via handleAsyncRetrieve.
	operation := func(bgCtx *schemas.GatewayContext) (interface{}, *schemas.GatewayError) {
		switch {
		case gatewayReq.ChatRequest != nil:
			return g.client.ChatCompletionRequest(bgCtx, gatewayReq.ChatRequest)
		case gatewayReq.ResponsesRequest != nil:
			return g.client.ResponsesRequest(bgCtx, gatewayReq.ResponsesRequest)
		default:
			return nil, newGatewayError(nil, "unsupported request type for async execution")
		}
	}

	job, err := executor.SubmitJob(gatewayCtx, resultTTL, operation, operationType)
	if err != nil {
		g.sendError(ctx, gatewayCtx, config.ErrorConverter,
			newGatewayError(err, "failed to create async job"))
		return
	}

	g.handleAsyncJobResponse(ctx, gatewayCtx, config, job)
}

// handleAsyncRetrieve retrieves an async job by ID and returns the response
// using the route's response converter for completed jobs.
func (g *GenericRouter) handleAsyncRetrieve(
	ctx *fasthttp.RequestCtx,
	config RouteConfig,
	gatewayCtx *schemas.GatewayContext,
) {
	executor := g.handlerStore.GetAsyncJobExecutor()
	if executor == nil {
		g.sendError(ctx, gatewayCtx, config.ErrorConverter,
			newGatewayError(nil, "async operations not available: logs store not configured"))
		return
	}

	jobID := string(ctx.Request.Header.Peek(schemas.AsyncHeaderGetID))
	if jobID == "" {
		g.sendError(ctx, gatewayCtx, config.ErrorConverter,
			newGatewayError(nil, "x-uf-async-id header value is empty"))
		return
	}

	vkValue := getVirtualKeyFromGatewayContext(gatewayCtx)

	job, err := executor.RetrieveJob(gatewayCtx, jobID, vkValue, config.GetHTTPRequestType(ctx))
	if err != nil {
		if errors.Is(err, logstore.ErrJobInternal) {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter,
				newGatewayErrorWithCode(err, "failed to retrieve async job", fasthttp.StatusInternalServerError))
		} else {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter,
				newGatewayErrorWithCode(err, "job not found or expired", fasthttp.StatusNotFound))
		}
		return
	}

	g.handleAsyncJobResponse(ctx, gatewayCtx, config, job)
}

func (g *GenericRouter) handleAsyncJobResponse(ctx *fasthttp.RequestCtx, gatewayCtx *schemas.GatewayContext, config RouteConfig, job *logstore.AsyncJob) {
	ctx.SetContentType("application/json")

	resp := job.ToResponse()

	switch job.Status {
	case schemas.AsyncJobStatusPending, schemas.AsyncJobStatusProcessing, schemas.AsyncJobStatusCompleted:
		switch job.RequestType {
		case schemas.ChatCompletionRequest:
			if config.AsyncChatResponseConverter == nil || config.ChatResponseConverter == nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "async operation is not supported on this route"))
				return
			}
			response, extraHeaders, err := config.AsyncChatResponseConverter(gatewayCtx, resp, config.ChatResponseConverter)
			if err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert async chat response"))
				return
			}
			g.sendSuccess(ctx, gatewayCtx, config.ErrorConverter, response, extraHeaders)
			return
		case schemas.ResponsesRequest:
			if config.AsyncResponsesResponseConverter == nil || config.ResponsesResponseConverter == nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "either async responses response converter or responses response converter not configured"))
				return
			}
			response, extraHeaders, err := config.AsyncResponsesResponseConverter(gatewayCtx, resp, config.ResponsesResponseConverter)
			if err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert async responses response"))
				return
			}
			g.sendSuccess(ctx, gatewayCtx, config.ErrorConverter, response, extraHeaders)
			return
		default:
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "unknown request type"))
			return
		}

	case schemas.AsyncJobStatusFailed:
		var err schemas.GatewayError
		// Deserialize the stored GatewayError and send through provider error converter
		if job.Error != "" {
			if unmarshalErr := sonic.Unmarshal([]byte(job.Error), &err); unmarshalErr != nil {
				// If unmarshal fails, create a basic error with the raw error string
				err = schemas.GatewayError{
					Error: &schemas.ErrorField{
						Message: job.Error,
					},
				}
			}
		}
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, &err)
	}
}

func (g *GenericRouter) filterDeprecatedListModelsResponse(resp *schemas.GatewayListModelsResponse) {
	if resp == nil || len(resp.Data) == 0 {
		return
	}
	catalogProvider, ok := g.handlerStore.(modelCatalogProvider)
	if !ok || catalogProvider.GetModelCatalog() == nil {
		resp.FilterDeprecatedModels()
		return
	}
	catalog := catalogProvider.GetModelCatalog()
	models := resp.Data[:0]
	for _, model := range resp.Data {
		provider, modelName := schemas.ParseModelString(model.ID, "")
		pricingEntry := catalog.GetPricingEntryForModel(modelName, provider)
		if pricingEntry == nil && model.Alias != nil {
			pricingEntry = catalog.GetPricingEntryForModel(*model.Alias, provider)
		}
		if model.IsDeprecated || (pricingEntry != nil && pricingEntry.IsDeprecated) {
			continue
		}
		model.IsDeprecated = false
		models = append(models, model)
	}
	resp.Data = models
}

// handleBatchRequest handles batch API requests (create, list, retrieve, cancel, results)
func (g *GenericRouter) handleBatchRequest(ctx *fasthttp.RequestCtx, config RouteConfig, req interface{}, batchReq *BatchRequest, gatewayCtx *schemas.GatewayContext) {
	var response interface{}
	var err error

	switch batchReq.Type {
	case schemas.BatchCreateRequest:
		if batchReq.CreateRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid batch create request"))
			return
		}
		batchResponse, gatewayErr := g.client.BatchCreateRequest(gatewayCtx, batchReq.CreateRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, batchResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.BatchCreateResponseConverter != nil {
			response, err = config.BatchCreateResponseConverter(gatewayCtx, batchResponse)
		} else {
			response = batchResponse
		}

	case schemas.BatchListRequest:
		if batchReq.ListRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid batch list request"))
			return
		}
		batchResponse, gatewayErr := g.client.BatchListRequest(gatewayCtx, batchReq.ListRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, batchResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.BatchListResponseConverter != nil {
			response, err = config.BatchListResponseConverter(gatewayCtx, batchResponse)
		} else {
			response = batchResponse
		}

	case schemas.BatchRetrieveRequest:
		if batchReq.RetrieveRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid batch retrieve request"))
			return
		}
		batchResponse, gatewayErr := g.client.BatchRetrieveRequest(gatewayCtx, batchReq.RetrieveRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, batchResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.BatchRetrieveResponseConverter != nil {
			response, err = config.BatchRetrieveResponseConverter(gatewayCtx, batchResponse)
		} else {
			response = batchResponse
		}

	case schemas.BatchCancelRequest:
		if batchReq.CancelRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid batch cancel request"))
			return
		}
		batchResponse, gatewayErr := g.client.BatchCancelRequest(gatewayCtx, batchReq.CancelRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, batchResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.BatchCancelResponseConverter != nil {
			response, err = config.BatchCancelResponseConverter(gatewayCtx, batchResponse)
		} else {
			response = batchResponse
		}
	case schemas.BatchDeleteRequest:
		if batchReq.DeleteRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid batch delete request"))
			return
		}
		batchResponse, gatewayErr := g.client.BatchDeleteRequest(gatewayCtx, batchReq.DeleteRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, batchResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.BatchDeleteResponseConverter != nil {
			response, err = config.BatchDeleteResponseConverter(gatewayCtx, batchResponse)
		} else {
			response = batchResponse
		}

	case schemas.BatchResultsRequest:
		if batchReq.ResultsRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid batch results request"))
			return
		}
		batchResponse, gatewayErr := g.client.BatchResultsRequest(gatewayCtx, batchReq.ResultsRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, batchResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.BatchResultsResponseConverter != nil {
			response, err = config.BatchResultsResponseConverter(gatewayCtx, batchResponse)
		} else {
			response = batchResponse
		}

	default:
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Unknown batch request type"))
		return
	}

	if err != nil {
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert batch response"))
		return
	}

	g.sendSuccess(ctx, gatewayCtx, config.ErrorConverter, response, nil)
}

// handleFileRequest handles file API requests (upload, list, retrieve, delete, content)
func (g *GenericRouter) handleFileRequest(ctx *fasthttp.RequestCtx, config RouteConfig, req interface{}, fileReq *FileRequest, gatewayCtx *schemas.GatewayContext) {
	var response interface{}
	var err error

	switch fileReq.Type {
	case schemas.FileUploadRequest:
		if fileReq.UploadRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid file upload request"))
			return
		}
		fileResponse, gatewayErr := g.client.FileUploadRequest(gatewayCtx, fileReq.UploadRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, fileResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.FileUploadResponseConverter != nil {
			response, err = config.FileUploadResponseConverter(gatewayCtx, fileResponse)
		} else {
			response = fileResponse
		}

	case schemas.FileListRequest:
		if fileReq.ListRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid file list request"))
			return
		}
		fileResponse, gatewayErr := g.client.FileListRequest(gatewayCtx, fileReq.ListRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, fileResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.FileListResponseConverter != nil {
			response, err = config.FileListResponseConverter(gatewayCtx, fileResponse)
			if err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert file list response"))
				return
			}
			// Handle raw byte responses (e.g., XML for S3 APIs)
			if rawBytes, ok := response.([]byte); ok {
				ctx.SetBody(rawBytes)
				return
			}
		} else {
			response = fileResponse
		}

	case schemas.FileRetrieveRequest:
		if fileReq.RetrieveRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid file retrieve request"))
			return
		}
		fileResponse, gatewayErr := g.client.FileRetrieveRequest(gatewayCtx, fileReq.RetrieveRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, fileResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.FileRetrieveResponseConverter != nil {
			response, err = config.FileRetrieveResponseConverter(gatewayCtx, fileResponse)
		} else {
			response = fileResponse
		}

	case schemas.FileDeleteRequest:
		if fileReq.DeleteRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid file delete request"))
			return
		}
		fileResponse, gatewayErr := g.client.FileDeleteRequest(gatewayCtx, fileReq.DeleteRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, fileResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.FileDeleteResponseConverter != nil {
			response, err = config.FileDeleteResponseConverter(gatewayCtx, fileResponse)
		} else {
			response = fileResponse
		}

	case schemas.FileContentRequest:
		if fileReq.ContentRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid file content request"))
			return
		}
		fileResponse, gatewayErr := g.client.FileContentRequest(gatewayCtx, fileReq.ContentRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, fileResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		// For file content, handle binary response specially if no converter is set
		if config.FileContentResponseConverter != nil {
			response, err = config.FileContentResponseConverter(gatewayCtx, fileResponse)
			if err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert file content response"))
				return
			}
			// Check if response is raw bytes - write directly without JSON encoding
			if rawBytes, ok := response.([]byte); ok {
				ctx.Response.Header.Set("Content-Type", fileResponse.ContentType)
				ctx.Response.Header.Set("Content-Length", strconv.Itoa(len(rawBytes)))
				ctx.Response.SetBody(rawBytes)
			} else {
				g.sendSuccess(ctx, gatewayCtx, config.ErrorConverter, response, nil)
			}
		} else {
			// Return raw file content
			ctx.Response.Header.Set("Content-Type", fileResponse.ContentType)
			ctx.Response.Header.Set("Content-Length", strconv.Itoa(len(fileResponse.Content)))
			ctx.Response.SetBody(fileResponse.Content)
		}
		return

	default:
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Unknown file request type"))
		return
	}

	if err != nil {
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert file response"))
		return
	}

	// If response is nil, PostCallback has set headers/status - return without body
	if response == nil {
		return
	}

	g.sendSuccess(ctx, gatewayCtx, config.ErrorConverter, response, nil)
}

// handleContainerRequest handles container API requests (create, list, retrieve, delete)
func (g *GenericRouter) handleContainerRequest(ctx *fasthttp.RequestCtx, config RouteConfig, req interface{}, containerReq *ContainerRequest, gatewayCtx *schemas.GatewayContext) {
	var response interface{}
	var err error

	switch containerReq.Type {
	case schemas.ContainerCreateRequest:
		if containerReq.CreateRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid container create request"))
			return
		}
		containerResponse, gatewayErr := g.client.ContainerCreateRequest(gatewayCtx, containerReq.CreateRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, containerResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.ContainerCreateResponseConverter != nil {
			response, err = config.ContainerCreateResponseConverter(gatewayCtx, containerResponse)
		} else {
			response = containerResponse
		}

	case schemas.ContainerListRequest:
		if containerReq.ListRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid container list request"))
			return
		}
		containerResponse, gatewayErr := g.client.ContainerListRequest(gatewayCtx, containerReq.ListRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, containerResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.ContainerListResponseConverter != nil {
			response, err = config.ContainerListResponseConverter(gatewayCtx, containerResponse)
		} else {
			response = containerResponse
		}

	case schemas.ContainerRetrieveRequest:
		if containerReq.RetrieveRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid container retrieve request"))
			return
		}
		containerResponse, gatewayErr := g.client.ContainerRetrieveRequest(gatewayCtx, containerReq.RetrieveRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, containerResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.ContainerRetrieveResponseConverter != nil {
			response, err = config.ContainerRetrieveResponseConverter(gatewayCtx, containerResponse)
		} else {
			response = containerResponse
		}

	case schemas.ContainerDeleteRequest:
		if containerReq.DeleteRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid container delete request"))
			return
		}
		containerResponse, gatewayErr := g.client.ContainerDeleteRequest(gatewayCtx, containerReq.DeleteRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, containerResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.ContainerDeleteResponseConverter != nil {
			response, err = config.ContainerDeleteResponseConverter(gatewayCtx, containerResponse)
		} else {
			response = containerResponse
		}

	default:
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Unknown container request type"))
		return
	}

	if err != nil {
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert container response"))
		return
	}

	g.sendSuccess(ctx, gatewayCtx, config.ErrorConverter, response, nil)
}

// handleContainerFileRequest handles container file API requests (create, list, retrieve, content, delete)
func (g *GenericRouter) handleContainerFileRequest(ctx *fasthttp.RequestCtx, config RouteConfig, req interface{}, containerFileReq *ContainerFileRequest, gatewayCtx *schemas.GatewayContext) {
	var response interface{}
	var err error

	switch containerFileReq.Type {
	case schemas.ContainerFileCreateRequest:
		if containerFileReq.CreateRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid container file create request"))
			return
		}
		containerFileResponse, gatewayErr := g.client.ContainerFileCreateRequest(gatewayCtx, containerFileReq.CreateRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, containerFileResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.ContainerFileCreateResponseConverter != nil {
			response, err = config.ContainerFileCreateResponseConverter(gatewayCtx, containerFileResponse)
		} else {
			response = containerFileResponse
		}

	case schemas.ContainerFileListRequest:
		if containerFileReq.ListRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid container file list request"))
			return
		}
		containerFileResponse, gatewayErr := g.client.ContainerFileListRequest(gatewayCtx, containerFileReq.ListRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, containerFileResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.ContainerFileListResponseConverter != nil {
			response, err = config.ContainerFileListResponseConverter(gatewayCtx, containerFileResponse)
		} else {
			response = containerFileResponse
		}

	case schemas.ContainerFileRetrieveRequest:
		if containerFileReq.RetrieveRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid container file retrieve request"))
			return
		}
		containerFileResponse, gatewayErr := g.client.ContainerFileRetrieveRequest(gatewayCtx, containerFileReq.RetrieveRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, containerFileResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.ContainerFileRetrieveResponseConverter != nil {
			response, err = config.ContainerFileRetrieveResponseConverter(gatewayCtx, containerFileResponse)
		} else {
			response = containerFileResponse
		}

	case schemas.ContainerFileContentRequest:
		if containerFileReq.ContentRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid container file content request"))
			return
		}
		containerFileResponse, gatewayErr := g.client.ContainerFileContentRequest(gatewayCtx, containerFileReq.ContentRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, containerFileResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		// For content requests, handle binary response specially if converter is set
		if config.ContainerFileContentResponseConverter != nil {
			response, err = config.ContainerFileContentResponseConverter(gatewayCtx, containerFileResponse)
			if err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert container file content response"))
				return
			}
			// Check if response is raw bytes - write directly without JSON encoding
			if rawBytes, ok := response.([]byte); ok {
				ctx.Response.Header.Set("Content-Type", containerFileResponse.ContentType)
				ctx.Response.Header.Set("Content-Length", strconv.Itoa(len(rawBytes)))
				ctx.Response.SetBody(rawBytes)
			} else {
				g.sendSuccess(ctx, gatewayCtx, config.ErrorConverter, response, nil)
			}
		} else {
			// Return raw binary content
			ctx.Response.Header.Set("Content-Type", containerFileResponse.ContentType)
			ctx.Response.Header.Set("Content-Length", strconv.Itoa(len(containerFileResponse.Content)))
			ctx.Response.SetBody(containerFileResponse.Content)
		}
		return

	case schemas.ContainerFileDeleteRequest:
		if containerFileReq.DeleteRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid container file delete request"))
			return
		}
		containerFileResponse, gatewayErr := g.client.ContainerFileDeleteRequest(gatewayCtx, containerFileReq.DeleteRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if err := config.PostCallback(ctx, req, containerFileResponse); err != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to execute post-request callback"))
				return
			}
		}
		if config.ContainerFileDeleteResponseConverter != nil {
			response, err = config.ContainerFileDeleteResponseConverter(gatewayCtx, containerFileResponse)
		} else {
			response = containerFileResponse
		}

	default:
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "Unknown container file request type"))
		return
	}

	if err != nil {
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert container file response"))
		return
	}

	g.sendSuccess(ctx, gatewayCtx, config.ErrorConverter, response, nil)
}

// handleCachedContentRequest handles cached content API requests
// (create, list, retrieve, update, delete) for Gemini and Vertex AI.
func (g *GenericRouter) handleCachedContentRequest(ctx *fasthttp.RequestCtx, config RouteConfig, req interface{}, cachedReq *CachedContentRequest, gatewayCtx *schemas.GatewayContext) {
	var response interface{}
	var err error

	switch cachedReq.Type {
	case schemas.CachedContentCreateRequest:
		if cachedReq.CreateRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid cached content create request"))
			return
		}
		gatewayResp, gatewayErr := g.client.CachedContentCreateRequest(gatewayCtx, cachedReq.CreateRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if perr := config.PostCallback(ctx, req, gatewayResp); perr != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(perr, "failed to execute post-request callback"))
				return
			}
		}
		if config.CachedContentCreateResponseConverter != nil {
			response, err = config.CachedContentCreateResponseConverter(gatewayCtx, gatewayResp)
		} else {
			response = gatewayResp
		}

	case schemas.CachedContentListRequest:
		if cachedReq.ListRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid cached content list request"))
			return
		}
		gatewayResp, gatewayErr := g.client.CachedContentListRequest(gatewayCtx, cachedReq.ListRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if perr := config.PostCallback(ctx, req, gatewayResp); perr != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(perr, "failed to execute post-request callback"))
				return
			}
		}
		if config.CachedContentListResponseConverter != nil {
			response, err = config.CachedContentListResponseConverter(gatewayCtx, gatewayResp)
		} else {
			response = gatewayResp
		}

	case schemas.CachedContentRetrieveRequest:
		if cachedReq.RetrieveRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid cached content retrieve request"))
			return
		}
		gatewayResp, gatewayErr := g.client.CachedContentRetrieveRequest(gatewayCtx, cachedReq.RetrieveRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if perr := config.PostCallback(ctx, req, gatewayResp); perr != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(perr, "failed to execute post-request callback"))
				return
			}
		}
		if config.CachedContentRetrieveResponseConverter != nil {
			response, err = config.CachedContentRetrieveResponseConverter(gatewayCtx, gatewayResp)
		} else {
			response = gatewayResp
		}

	case schemas.CachedContentUpdateRequest:
		if cachedReq.UpdateRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid cached content update request"))
			return
		}
		gatewayResp, gatewayErr := g.client.CachedContentUpdateRequest(gatewayCtx, cachedReq.UpdateRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if perr := config.PostCallback(ctx, req, gatewayResp); perr != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(perr, "failed to execute post-request callback"))
				return
			}
		}
		if config.CachedContentUpdateResponseConverter != nil {
			response, err = config.CachedContentUpdateResponseConverter(gatewayCtx, gatewayResp)
		} else {
			response = gatewayResp
		}

	case schemas.CachedContentDeleteRequest:
		if cachedReq.DeleteRequest == nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "invalid cached content delete request"))
			return
		}
		gatewayResp, gatewayErr := g.client.CachedContentDeleteRequest(gatewayCtx, cachedReq.DeleteRequest)
		if gatewayErr != nil {
			g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
			return
		}
		if config.PostCallback != nil {
			if perr := config.PostCallback(ctx, req, gatewayResp); perr != nil {
				g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(perr, "failed to execute post-request callback"))
				return
			}
		}
		if config.CachedContentDeleteResponseConverter != nil {
			response, err = config.CachedContentDeleteResponseConverter(gatewayCtx, gatewayResp)
		} else {
			response = gatewayResp
		}

	default:
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "unsupported cached content request type"))
		return
	}

	if err != nil {
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(err, "failed to convert cached content response"))
		return
	}

	g.sendSuccess(ctx, gatewayCtx, config.ErrorConverter, response, nil)
}

// handleStreamingRequest handles streaming requests using Server-Sent Events (SSE)
func (g *GenericRouter) handleStreamingRequest(ctx *fasthttp.RequestCtx, config RouteConfig, gatewayReq *schemas.GatewayRequest, gatewayCtx *schemas.GatewayContext, cancel context.CancelFunc) {
	// Use the cancellable context from ConvertToGatewayContext
	// ctx.Done() never fires here in practice: fasthttp.RequestCtx.Done only closes when the whole server shuts down, not when an individual connection drops.
	// As a result we'll leave the provider stream running until it naturally completes, even if the client went away (write error, network drop, etc.).
	// That keeps goroutines and upstream tokens alive long after the SSE writer has exited.
	//
	// We now get a cancellable context from ConvertToGatewayContext so we can cancel the upstream stream immediately when the client disconnects.
	var stream chan *schemas.GatewayStreamChunk
	var gatewayErr *schemas.GatewayError

	// Handle different request types
	if gatewayReq.TextCompletionRequest != nil {
		stream, gatewayErr = g.client.TextCompletionStreamRequest(gatewayCtx, gatewayReq.TextCompletionRequest)
	} else if gatewayReq.ChatRequest != nil {
		stream, gatewayErr = g.client.ChatCompletionStreamRequest(gatewayCtx, gatewayReq.ChatRequest)
	} else if gatewayReq.ResponsesRequest != nil {
		stream, gatewayErr = g.client.ResponsesStreamRequest(gatewayCtx, gatewayReq.ResponsesRequest)
	} else if gatewayReq.SpeechRequest != nil {
		stream, gatewayErr = g.client.SpeechStreamRequest(gatewayCtx, gatewayReq.SpeechRequest)
	} else if gatewayReq.TranscriptionRequest != nil {
		stream, gatewayErr = g.client.TranscriptionStreamRequest(gatewayCtx, gatewayReq.TranscriptionRequest)
	} else if gatewayReq.ImageGenerationRequest != nil {
		stream, gatewayErr = g.client.ImageGenerationStreamRequest(gatewayCtx, gatewayReq.ImageGenerationRequest)
	} else if gatewayReq.ImageEditRequest != nil {
		stream, gatewayErr = g.client.ImageEditStreamRequest(gatewayCtx, gatewayReq.ImageEditRequest)
	}

	// Provider error before streaming started — return proper HTTP error status
	// (SSE headers not yet committed, so we can still set status code + JSON body)
	if gatewayErr != nil {
		cancel()
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, gatewayErr)
		return
	}

	// No request type matched — stream is nil. Return error without spawning
	// a drain goroutine (for-range on nil channel blocks forever).
	if stream == nil {
		cancel()
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "streaming is not supported for this request type"))
		return
	}

	// Forward provider response headers stored in context by streaming handlers
	if headers, ok := gatewayCtx.Value(schemas.GatewayContextKeyProviderResponseHeaders).(map[string]string); ok {
		for key, value := range headers {
			ctx.Response.Header.Set(key, value)
		}
	}

	// Large payload streaming passthrough — bypass SSE event processing, pipe raw upstream
	if g.tryStreamLargeResponse(ctx, gatewayCtx) {
		ctx.Response.Header.Set("Cache-Control", "no-cache")
		ctx.Response.Header.Set("Connection", "keep-alive")
		ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
		cancel()
		go func() {
			for range stream {
			}
		}()
		return
	}

	// Check if streaming is configured for this route
	if config.StreamConfig == nil {
		cancel()
		// Drain the stream channel to prevent goroutine leaks
		go func() {
			for range stream {
			}
		}()
		g.sendError(ctx, gatewayCtx, config.ErrorConverter, newGatewayError(nil, "streaming is not supported for this integration"))
		return
	}

	// SSE headers set only after successful stream setup — errors above get proper HTTP status codes
	if config.Type == RouteConfigTypeBedrock {
		ctx.SetContentType("application/vnd.amazon.eventstream")
		ctx.Response.Header.Set("x-amzn-bedrock-content-type", "application/json")
	} else {
		ctx.SetContentType("text/event-stream")
	}

	ctx.Response.Header.Set("Cache-Control", "no-cache")
	ctx.Response.Header.Set("Connection", "keep-alive")
	ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")

	// Handle streaming using the centralized approach
	// Pass cancel function so it can be called when the writer exits (errors, completion, etc.)
	g.handleStreaming(ctx, gatewayCtx, config, stream, cancel)
}

// handleStreaming processes a stream of GatewayResponse objects and sends them as Server-Sent Events (SSE).
// It handles both successful responses and errors in the streaming format.
//
// SSE FORMAT HANDLING:
//
// By default, all responses and errors are sent in the standard SSE format:
//
//	data: {"response": "content"}\n\n
//
// However, some providers (like Anthropic) require custom SSE event formats with explicit event types:
//
//	event: content_block_delta
//	data: {"type": "content_block_delta", "delta": {...}}
//
//	event: message_stop
//	data: {"type": "message_stop"}
//
// STREAMCONFIG CONVERTER BEHAVIOR:
//
// The StreamConfig.ResponseConverter and StreamConfig.ErrorConverter functions can return:
//
// 1. OBJECTS (default behavior):
//   - Return any Go struct/map/interface{}
//   - Will be JSON marshaled and wrapped as: data: {json}\n\n
//   - Example: return map[string]interface{}{"content": "hello"}
//   - Result: data: {"content":"hello"}\n\n
//
// 2. STRINGS (custom SSE format):
//   - Return a complete SSE string with custom event types and formatting
//   - Will be sent directly without any wrapping or modification
//   - Example: return "event: content_block_delta\ndata: {\"type\":\"text\"}\n\n"
//   - Result: event: content_block_delta
//     data: {"type":"text"}
//
// IMPLEMENTATION GUIDELINES:
//
// For standard providers (OpenAI, etc.): Return objects from converters
// For custom SSE providers (Anthropic, etc.): Return pre-formatted SSE strings
//
// When returning strings, ensure they:
// - Include proper event: lines (if needed)
// - Include data: lines with JSON content
// - End with \n\n for proper SSE formatting
// - Follow the provider's specific SSE event specification
//
// CONTEXT CANCELLATION:
//
// The cancel function is called ONLY when client disconnects are detected via write errors.
// Gateway handles cleanup internally for normal completion and errors, so we only cancel
// upstream streams when write errors indicate the client has disconnected.
func (g *GenericRouter) handleStreaming(ctx *fasthttp.RequestCtx, gatewayCtx *schemas.GatewayContext, config RouteConfig, streamChan chan *schemas.GatewayStreamChunk, cancel context.CancelFunc) {
	// Signal to tracing middleware that trace completion should be deferred
	// The streaming callback will complete the trace after the stream ends
	ctx.SetUserValue(schemas.GatewayContextKeyDeferTraceCompletion, true)

	// Get the trace completer function for use in the streaming callback.
	// Signature is func([]schemas.PluginLogEntry) so the callback never reads from
	// ctx.UserValue (ctx may be recycled by fasthttp by the time this fires).
	// Router path has no transport post-hook phase, so we always pass nil.
	traceCompleter, _ := ctx.UserValue(schemas.GatewayContextKeyTraceCompleter).(func([]schemas.PluginLogEntry))

	// Get stream chunk interceptor for plugin hooks
	interceptor := g.handlerStore.GetStreamChunkInterceptor()
	var httpReq *schemas.HTTPRequest
	if interceptor != nil {
		httpReq = lib.BuildHTTPRequestFromFastHTTP(ctx)
	}

	// Use SSEStreamReader to bypass fasthttp's internal pipe (fasthttputil.PipeConns)
	// which batches multiple SSE events into single TCP segments.
	reader := lib.NewSSEStreamReader()
	ctx.Response.SetBodyStream(reader, -1)

	// Producer goroutine: processes the stream channel, formats events, sends to reader
	go func() {
		// Separate defers ensure each cleanup runs even if an earlier one panics (LIFO order)
		defer reader.Done()
		defer schemas.ReleaseHTTPRequest(httpReq)
		defer func() {
			// Complete the trace after streaming finishes
			// This ensures all spans (including llm.call) are properly ended before the trace is sent to OTEL
			if traceCompleter != nil {
				traceCompleter(nil)
			}
		}()

		// Create encoder for AWS Event Stream if needed
		var eventStreamEncoder *eventstream.Encoder
		if config.Type == RouteConfigTypeBedrock {
			eventStreamEncoder = eventstream.NewEncoder()
		}

		shouldSendDoneMarker := true
		if config.Type == RouteConfigTypeAnthropic || strings.Contains(config.Path, "/responses") || strings.Contains(config.Path, "/images/generations") {
			shouldSendDoneMarker = false
		}

		// Process streaming responses
		for chunk := range streamChan {
			if chunk == nil {
				continue
			}

			// Note: We no longer check ctx.Done() here because fasthttp.RequestCtx.Done()
			// only closes when the whole server shuts down, not when an individual client disconnects.
			// Client disconnects are detected via write errors on reader.Send(), which returns false.

			// Handle errors
			if chunk.GatewayError != nil {
				var errorResponse interface{}
				gatewayErr := lib.SanitizeGatewayErrorForClient(chunk.GatewayError)
				if gatewayErr == nil {
					gatewayErr = newGatewayErrorWithCode(nil, lib.ClientSafeInternalErrorMessage, fasthttp.StatusInternalServerError)
				}

				// Use stream error converter if available, otherwise fallback to regular error converter
				if config.StreamConfig != nil && config.StreamConfig.ErrorConverter != nil {
					errorResponse = config.StreamConfig.ErrorConverter(gatewayCtx, gatewayErr)
				} else if config.ErrorConverter != nil {
					errorResponse = config.ErrorConverter(gatewayCtx, gatewayErr)
				} else {
					// Default error response
					errorResponse = map[string]interface{}{
						"error": map[string]interface{}{
							"type":    "internal_error",
							"message": "An error occurred while processing your request",
						},
					}
				}

				// Check if the error converter returned a raw SSE string or JSON object
				if sseErrorString, ok := errorResponse.(string); ok {
					// CUSTOM SSE FORMAT: The converter returned a complete SSE string
					// This is used by providers like Anthropic that need custom event types
					reader.Send([]byte(sseErrorString))
				} else if config.Type == RouteConfigTypeBedrock && eventStreamEncoder != nil {
					if bedrockException, ok := toBedrockEventStreamException(errorResponse); ok {
						if !sendBedrockEventStreamException(reader, eventStreamEncoder, bedrockException, g.logger) {
							cancel()
						}
						return
					}
					if bedrockEvent, ok := errorResponse.(*bedrock.BedrockStreamEvent); ok {
						if !sendBedrockEventStream(reader, eventStreamEncoder, bedrockEvent, g.logger) {
							cancel()
						}
						return
					}
					if !sendBedrockEventStreamException(reader, eventStreamEncoder, newBedrockEventStreamException("", ""), g.logger) {
						cancel()
					}
				} else {
					// STANDARD SSE FORMAT: The converter returned an object
					errorJSON, err := sonic.Marshal(errorResponse)
					if err != nil {
						// Fallback to basic error if marshaling fails
						basicError := map[string]interface{}{
							"error": map[string]interface{}{
								"type":    "internal_error",
								"message": "An error occurred while processing your request",
							},
						}
						if errorJSON, err = sonic.Marshal(basicError); err != nil {
							cancel()
							return
						}
					}

					// Send error as SSE data
					reader.SendEvent("", errorJSON)
				}

				return // End stream on error, Gateway handles cleanup internally
			} else {
				// Allow plugins to modify/filter the chunk via StreamChunkInterceptor
				if interceptor != nil {
					var err error
					chunk, err = interceptor.InterceptChunk(gatewayCtx, httpReq, chunk)
					if err != nil {
						if chunk == nil {
							errorJSON, marshalErr := sonic.Marshal(map[string]string{"error": err.Error()})
							if marshalErr != nil {
								cancel()
								for range streamChan {
								}
								return
							}
							// Return error event and stop streaming
							reader.SendError(errorJSON)
							cancel()
							for range streamChan {
							}
							return
						}
						// Else add warn log and continue
						g.logger.Warn("%v", err)
					}
					if chunk == nil {
						// Skip chunk if plugin wants to skip it
						continue
					}
				}
				// Handle successful responses
				// Convert response to integration-specific streaming format
				var eventType string
				var convertedResponse interface{}
				var err error

				switch {
				case chunk.GatewayTextCompletionResponse != nil:
					eventType, convertedResponse, err = config.StreamConfig.TextStreamResponseConverter(gatewayCtx, chunk.GatewayTextCompletionResponse)
				case chunk.GatewayChatResponse != nil:
					eventType, convertedResponse, err = config.StreamConfig.ChatStreamResponseConverter(gatewayCtx, chunk.GatewayChatResponse)
				case chunk.GatewayResponsesStreamResponse != nil:
					eventType, convertedResponse, err = config.StreamConfig.ResponsesStreamResponseConverter(gatewayCtx, chunk.GatewayResponsesStreamResponse)
				case chunk.GatewaySpeechStreamResponse != nil:
					eventType, convertedResponse, err = config.StreamConfig.SpeechStreamResponseConverter(gatewayCtx, chunk.GatewaySpeechStreamResponse)
				case chunk.GatewayTranscriptionStreamResponse != nil:
					eventType, convertedResponse, err = config.StreamConfig.TranscriptionStreamResponseConverter(gatewayCtx, chunk.GatewayTranscriptionStreamResponse)
				case chunk.GatewayImageGenerationStreamResponse != nil:
					eventType, convertedResponse, err = config.StreamConfig.ImageGenerationStreamResponseConverter(gatewayCtx, chunk.GatewayImageGenerationStreamResponse)
				default:
					requestType := safeGetRequestType(chunk)
					convertedResponse, err = nil, fmt.Errorf("no response converter found for request type: %s", requestType)
				}

				if convertedResponse == nil && err == nil {
					// Skip streaming chunk if no response is available and no error is returned
					continue
				}

				if err != nil {
					// Log conversion error but continue processing
					g.logger.Warn("Failed to convert streaming response: %v", err)
					continue
				}

				// Handle Bedrock Event Stream format
				if config.Type == RouteConfigTypeBedrock && eventStreamEncoder != nil {
					// We need to cast to BedrockStreamEvent to determine event type and structure
					if bedrockEvent, ok := convertedResponse.(*bedrock.BedrockStreamEvent); ok {
						if !sendBedrockEventStream(reader, eventStreamEncoder, bedrockEvent, g.logger) {
							cancel()
							return
						}
					}
					// Continue to next chunk (we handled sending internally)
					continue
				}

				// Build and send SSE event
				var buf []byte
				var sent bool
				if sseString, ok := convertedResponse.(string); ok {
					if strings.HasPrefix(sseString, "data: ") || strings.HasPrefix(sseString, "event: ") {
						// Pre-formatted SSE string (e.g. Anthropic custom event types)
						if eventType != "" {
							// Prepend event type line to pre-formatted data
							buf = make([]byte, 0, 7+len(eventType)+1+len(sseString))
							buf = append(buf, "event: "...)
							buf = append(buf, eventType...)
							buf = append(buf, '\n')
							buf = append(buf, sseString...)
							sent = reader.Send(buf)
						} else {
							sent = reader.Send([]byte(sseString))
						}
					} else {
						sent = reader.SendEvent(eventType, []byte(sseString))
					}
				} else {
					responseJSON, err := sonic.Marshal(convertedResponse)
					if err != nil {
						g.logger.Warn("Failed to marshal streaming response: %v", err)
						continue
					}
					sent = reader.SendEvent(eventType, responseJSON)
				}

				if !sent {
					cancel() // Client disconnected, cancel upstream stream
					// Drain remaining chunks so the provider goroutine's defer
					// (HandleStreamCancellation -> PostLLMHook -> storeOrEnqueueEntry) finishes
					// before our own defer fires traceCompleter. Without this, Inject runs
					// against an empty pendingLogsToInject and the cancellation log is orphaned.
					for range streamChan {
					}
					return
				}
			}
		}

		// Only send the [DONE] marker for plain SSE APIs that expect it.
		// Do NOT send [DONE] for the following cases:
		//   - OpenAI "responses" API and Anthropic messages API: they signal completion by simply closing the stream, not sending [DONE].
		//   - Bedrock: uses AWS Event Stream format rather than SSE with [DONE].
		// Gateway handles any additional cleanup internally on normal stream completion.
		if shouldSendDoneMarker && config.Type != RouteConfigTypeGenAI && config.Type != RouteConfigTypeBedrock {
			if !reader.SendDone() {
				g.logger.Warn("Failed to write SSE done marker: client disconnected")
				cancel()
				return
			}
		}
	}()
}

type bedrockEventStreamException struct {
	exceptionType string
	payload       []byte
}

func toBedrockEventStreamException(errorResponse interface{}) (*bedrockEventStreamException, bool) {
	switch response := errorResponse.(type) {
	case *bedrockEventStreamException:
		return response, true
	case *bedrock.BedrockError:
		return newBedrockEventStreamException(response.Type, response.Message), true
	default:
		return nil, false
	}
}

func newBedrockEventStreamException(exceptionType, message string) *bedrockEventStreamException {
	if message == "" {
		message = "An error occurred while processing your request"
	}
	if exceptionType == "" {
		exceptionType = "InternalServerException"
	}

	payloadJSON, err := sonic.Marshal(map[string]string{
		"__type":  exceptionType,
		"message": message,
	})
	if err != nil {
		payloadJSON = []byte(fmt.Sprintf(`{"__type":%q,"message":"An error occurred while processing your request"}`, exceptionType))
	}

	return &bedrockEventStreamException{
		exceptionType: exceptionType,
		payload:       payloadJSON,
	}
}

func sendBedrockEventStream(reader *lib.SSEStreamReader, encoder *eventstream.Encoder, bedrockEvent *bedrock.BedrockStreamEvent, logger schemas.Logger) bool {
	// Convert to sequence of specific Bedrock events
	events := bedrockEvent.ToEncodedEvents()

	// Send all collected events
	for _, evt := range events {
		jsonData, err := sonic.Marshal(evt.Payload)
		if err != nil {
			logger.Warn("Failed to marshal bedrock payload: %v", err)
			continue
		}

		headers := eventstream.Headers{
			{
				Name:  ":content-type",
				Value: eventstream.StringValue("application/json"),
			},
			{
				Name:  ":event-type",
				Value: eventstream.StringValue(evt.EventType),
			},
			{
				Name:  ":message-type",
				Value: eventstream.StringValue("event"),
			},
		}

		message := eventstream.Message{
			Headers: headers,
			Payload: jsonData,
		}

		var msgBuf bytes.Buffer
		if err := encoder.Encode(&msgBuf, message); err != nil {
			logger.Warn("[Bedrock Stream] Failed to encode message: %v", err)
			return false
		}

		if !reader.Send(msgBuf.Bytes()) {
			logger.Warn("[Bedrock Stream] Client disconnected")
			return false
		}
	}

	return true
}

func sendBedrockEventStreamException(reader *lib.SSEStreamReader, encoder *eventstream.Encoder, exception *bedrockEventStreamException, logger schemas.Logger) bool {
	headers := eventstream.Headers{
		{
			Name:  ":content-type",
			Value: eventstream.StringValue("application/json"),
		},
		{
			Name:  ":exception-type",
			Value: eventstream.StringValue(exception.exceptionType),
		},
		{
			Name:  ":message-type",
			Value: eventstream.StringValue("exception"),
		},
	}

	message := eventstream.Message{
		Headers: headers,
		Payload: exception.payload,
	}

	var msgBuf bytes.Buffer
	if err := encoder.Encode(&msgBuf, message); err != nil {
		logger.Warn("[Bedrock Stream] Failed to encode exception: %v", err)
		return false
	}

	if !reader.Send(msgBuf.Bytes()) {
		logger.Warn("[Bedrock Stream] Client disconnected")
		return false
	}

	return true
}

// extractPassthroughModel extracts the model from the passthrough request path and/or body.
// Path patterns: models/{model}, models/{model}:suffix (GenAI), .../models/{model} (Vertex), tunedModels/{model}.
// Body is pre-parsed by parsePassthroughBody to avoid redundant unmarshaling.
func extractPassthroughModel(path string, bodyModel string) string {
	if model := extractModelFromPath(path); model != "" {
		return model
	}
	return bodyModel
}

func extractModelFromPath(path string) string {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	for i, p := range parts {
		// GenAI uses models/{model} and tunedModels/{model}; Azure OpenAI uses
		// deployments/{deployment}, where the deployment name is the model identifier
		// (deployment-based Azure routes usually omit "model" from the request body).
		if p == "models" || p == "tunedModels" || p == "deployments" {
			if i+1 < len(parts) {
				model := parts[i+1]
				// Strip :suffix for GenAI (e.g. :generateContent, :streamGenerateContent)
				if idx := strings.Index(model, ":"); idx > 0 {
					model = model[:idx]
				}
				return strings.TrimSpace(model)
			}
			break
		}
	}
	return ""
}

// parsePassthroughBody extracts model and streaming flag from the request body in a
// single unmarshal pass. Pass the raw Content-Type header value so multipart boundaries
// are resolved from the header rather than scraped from the body bytes.
func parsePassthroughBody(contentType string, body []byte) (model string, isStream bool) {
	if len(body) == 0 {
		return
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err == nil && strings.HasPrefix(mediaType, "multipart/") {
		if boundary := params["boundary"]; boundary != "" {
			return parseMultipartPassthroughBody(body, boundary)
		}
	}
	// JSON (or unknown) body — one unmarshal for both fields.
	var parsed struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := sonic.Unmarshal(body, &parsed); err == nil {
		model = strings.TrimSpace(parsed.Model)
		isStream = parsed.Stream
	}
	return
}

// parseMultipartPassthroughBody scans multipart parts and extracts model and stream.
//   - Form fields (Content-Disposition name="model"/"stream"): read as plain text.
func parseMultipartPassthroughBody(body []byte, boundary string) (model string, isStream bool) {
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		// Plain form field — check by name.
		switch part.FormName() {
		case "model":
			val, _ := io.ReadAll(part)
			part.Close()
			model = strings.TrimSpace(string(val))
		case "stream":
			val, _ := io.ReadAll(part)
			part.Close()
			s := strings.TrimSpace(strings.ToLower(string(val)))
			isStream = s == "true" || s == "1"
		default:
			part.Close()
		}

		if model != "" && isStream {
			break
		}
	}
	return
}

func (g *GenericRouter) handlePassthrough(ctx *fasthttp.RequestCtx) {
	cfg := g.passthroughCfg

	safeHeaders := make(map[string]string)
	ctx.Request.Header.All()(func(key, value []byte) bool {
		keyStr := strings.ToLower(string(key))
		switch keyStr {
		case "authorization", "api-key", "x-api-key", "x-goog-api-key",
			"host", "connection", "transfer-encoding", "cookie", "set-cookie", "proxy-authorization", "accept-encoding":
		default:
			if strings.HasPrefix(keyStr, "x-uf-") {
				return true // drop internal gateway headers
			}
			safeHeaders[keyStr] = string(value)
		}
		return true
	})

	gatewayCtx, cancel := lib.ConvertToGatewayContext(ctx, g.handlerStore)

	path := string(ctx.Path())
	for _, prefix := range g.passthroughCfg.StripPrefix {
		if strings.HasPrefix(path, prefix) {
			path = path[len(prefix):]
			break
		}
	}

	body := ctx.Request.Body()
	// Parse body once to get both model and stream flag.
	contentType := string(ctx.Request.Header.ContentType())
	bodyModel, bodyStream := parsePassthroughBody(contentType, body)
	resolvedModel := extractPassthroughModel(path, bodyModel)
	provider := cfg.Provider
	if cfg.ProviderDetector != nil {
		provider = cfg.ProviderDetector(ctx, resolvedModel)
	}
	provider = getProviderFromHeader(ctx, provider)
	isStreaming := strings.Contains(strings.ToLower(path), "stream") || bodyStream

	passthroughReq := &schemas.GatewayPassthroughRequest{
		Method:      string(ctx.Method()),
		Path:        path,
		RawQuery:    string(ctx.URI().QueryString()),
		Body:        body,
		SafeHeaders: safeHeaders,
		Provider:    provider,
		Model:       resolvedModel,
	}

	if isStreaming {
		g.handlePassthroughStream(ctx, gatewayCtx, cancel, provider, passthroughReq)
	} else {
		g.handlePassthroughNonStream(ctx, gatewayCtx, cancel, provider, passthroughReq)
	}
}

func (g *GenericRouter) handlePassthroughNonStream(
	ctx *fasthttp.RequestCtx,
	gatewayCtx *schemas.GatewayContext,
	cancel context.CancelFunc,
	provider schemas.ModelProvider,
	req *schemas.GatewayPassthroughRequest,
) {
	defer cancel()

	resp, gatewayErr := g.client.Passthrough(gatewayCtx, provider, req)
	if gatewayErr != nil {
		g.sendError(ctx, gatewayCtx, func(_ *schemas.GatewayContext, err *schemas.GatewayError) interface{} {
			return err
		}, gatewayErr)
		return
	}

	ctx.SetStatusCode(resp.StatusCode)
	for k, v := range resp.Headers {
		switch strings.ToLower(k) {
		case "connection", "transfer-encoding", "set-cookie", "proxy-authenticate", "www-authenticate":
			// drop
		default:
			ctx.Response.Header.Set(k, v)
		}
	}
	ctx.Response.SetBody(resp.Body)
}

func (g *GenericRouter) handlePassthroughStream(
	ctx *fasthttp.RequestCtx,
	gatewayCtx *schemas.GatewayContext,
	cancel context.CancelFunc,
	provider schemas.ModelProvider,
	req *schemas.GatewayPassthroughRequest,
) {
	// Deferred trace completion must be explicitly finalized after streaming ends.
	// Without this, observability injectors (including logging) never flush final state.
	traceCompleter, _ := ctx.UserValue(schemas.GatewayContextKeyTraceCompleter).(func([]schemas.PluginLogEntry))

	stream, gatewayErr := g.client.PassthroughStream(gatewayCtx, provider, req)
	if gatewayErr != nil {
		cancel()
		g.sendError(ctx, gatewayCtx, func(_ *schemas.GatewayContext, err *schemas.GatewayError) interface{} {
			return err
		}, gatewayErr)
		return
	}

	// Read the first chunk to extract status code and headers before streaming begins.
	firstChunk, ok := <-stream
	if !ok {
		cancel()
		g.sendError(ctx, gatewayCtx, func(_ *schemas.GatewayContext, err *schemas.GatewayError) interface{} {
			return err
		}, newGatewayError(nil, "passthrough stream ended before headers were received"))
		return
	}
	if firstChunk == nil {
		cancel()
		g.sendError(ctx, gatewayCtx, func(_ *schemas.GatewayContext, err *schemas.GatewayError) interface{} {
			return err
		}, newGatewayError(nil, "passthrough stream returned nil first chunk"))
		return
	}
	if firstChunk.GatewayError != nil {
		cancel()
		g.sendError(ctx, gatewayCtx, func(_ *schemas.GatewayContext, err *schemas.GatewayError) interface{} {
			return err
		}, firstChunk.GatewayError)
		return
	}

	passthroughResp := firstChunk.GatewayPassthroughResponse
	if passthroughResp == nil {
		cancel()
		g.sendError(ctx, gatewayCtx, func(_ *schemas.GatewayContext, err *schemas.GatewayError) interface{} {
			return err
		}, newGatewayError(nil, "passthrough stream returned empty first chunk"))
		return
	}

	// Skip post-hook body materialization — ctx.Response.Body() would buffer the entire stream.
	ctx.SetUserValue(schemas.GatewayContextKeyDeferTraceCompletion, true)

	ctx.SetStatusCode(passthroughResp.StatusCode)
	// Preserve the upstream Content-Type. Passthrough streams aren't always SSE — e.g.
	// Vertex/Gemini :streamGenerateContent without ?alt=sse returns an incrementally-delivered
	// JSON array with Content-Type: application/json. Forcing text/event-stream mislabels that
	// stream, so clients that dispatch on content-type run an SSE parser over a non-SSE body and
	// hang. Fall back to text/event-stream only when the upstream didn't provide a Content-Type.
	contentType := ""
	for k, v := range passthroughResp.Headers {
		if strings.EqualFold(k, "content-type") {
			contentType = v
			break
		}
	}
	if contentType == "" {
		contentType = "text/event-stream"
	}
	ctx.SetContentType(contentType)
	ctx.Response.Header.Set("Cache-Control", "no-cache")
	ctx.Response.Header.Set("Connection", "keep-alive")
	ctx.Response.Header.Set("X-Accel-Buffering", "no")
	for k, v := range passthroughResp.Headers {
		switch strings.ToLower(k) {
		case "connection", "transfer-encoding", "content-length", "content-type",
			"cache-control", "x-accel-buffering",
			"set-cookie", "proxy-authenticate", "www-authenticate":
			// drop — streaming invariants are set explicitly above (Content-Type is set from the
			// upstream value before this loop); upstream must not override them here
		default:
			ctx.Response.Header.Set(k, v)
		}
	}

	// Use SSEStreamReader to bypass fasthttp's internal pipe batching
	reader := lib.NewSSEStreamReader()
	ctx.Response.SetBodyStream(reader, -1)

	go func() {
		defer func() {
			if traceCompleter != nil {
				traceCompleter(nil)
			}
			reader.Done()
			cancel()
		}()

		// Write the first chunk's data.
		if len(passthroughResp.Body) > 0 {
			if !reader.Send(passthroughResp.Body) {
				cancel()
				return
			}
		}

		for chunk := range stream {
			if chunk == nil {
				continue
			}
			if chunk.GatewayError != nil {
				break
			}
			if chunk.GatewayPassthroughResponse != nil && len(chunk.GatewayPassthroughResponse.Body) > 0 {
				if !reader.Send(chunk.GatewayPassthroughResponse.Body) {
					cancel()
					return
				}
			}
		}
	}()
}
