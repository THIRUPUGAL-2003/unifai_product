package jsonparser

import (
	"strings"
	"sync"
	"time"

	raksha "github.com/raksha/raksha/core"
	"github.com/raksha/raksha/core/schemas"
)

const (
	PluginName = "streaming-json-parser"
)

type Usage string

const (
	AllRequests Usage = "all_requests"
	PerRequest  Usage = "per_request"
)

// AccumulatedContent holds both the content and timestamp for a request
type AccumulatedContent struct {
	Content   *strings.Builder
	Timestamp time.Time
}

// JsonParserPlugin provides JSON parsing capabilities for streaming responses
// It handles partial JSON chunks by accumulating them and making the accumulated content valid JSON
type JsonParserPlugin struct {
	usage Usage
	// State management for accumulating chunks
	accumulatedContent map[string]*AccumulatedContent // requestID -> accumulated content with timestamp
	mutex              sync.RWMutex
	// Cleanup configuration
	cleanupInterval time.Duration
	maxAge          time.Duration
	stopCleanup     chan struct{}
	stopOnce        sync.Once
}

// PluginConfig holds configuration options for the JSON parser plugin
type PluginConfig struct {
	Usage           Usage
	CleanupInterval time.Duration
	MaxAge          time.Duration
}

const (
	EnableStreamingJSONParser schemas.RakshaContextKey = "enable-streaming-json-parser"
)

// Init creates a new JSON parser plugin instance with custom configuration
func Init(config PluginConfig) (*JsonParserPlugin, error) {
	// Set defaults if not provided
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 5 * time.Minute
	}
	if config.MaxAge <= 0 {
		config.MaxAge = 30 * time.Minute
	}
	if config.Usage == "" {
		config.Usage = PerRequest
	}

	plugin := &JsonParserPlugin{
		usage:              config.Usage,
		accumulatedContent: make(map[string]*AccumulatedContent),
		cleanupInterval:    config.CleanupInterval,
		maxAge:             config.MaxAge,
		stopCleanup:        make(chan struct{}),
	}

	// Start the cleanup goroutine
	go plugin.startCleanupGoroutine()

	return plugin, nil
}

// GetName returns the plugin name
func (p *JsonParserPlugin) GetName() string {
	return PluginName
}

// HTTPTransportPreHook is not used for this plugin
func (p *JsonParserPlugin) HTTPTransportPreHook(ctx *schemas.RakshaContext, req *schemas.HTTPRequest) (*schemas.HTTPResponse, error) {
	return nil, nil
}

// HTTPTransportPostHook is not used for this plugin
func (p *JsonParserPlugin) HTTPTransportPostHook(ctx *schemas.RakshaContext, req *schemas.HTTPRequest, resp *schemas.HTTPResponse) error {
	return nil
}

// HTTPTransportStreamChunkHook passes through streaming chunks unchanged
func (p *JsonParserPlugin) HTTPTransportStreamChunkHook(ctx *schemas.RakshaContext, req *schemas.HTTPRequest, chunk *schemas.RakshaStreamChunk) (*schemas.RakshaStreamChunk, error) {
	return chunk, nil
}

// PreRequestHook implements schemas.LLMPlugin (no-op — required for plugin indexing).
func (p *JsonParserPlugin) PreRequestHook(_ *schemas.RakshaContext, _ *schemas.RakshaRequest) error {
	return nil
}

// PreLLMHook is not used for this plugin as we only process responses
// Parameters:
//   - ctx: The Raksha context
//   - req: The Raksha request
//
// Returns:
//   - *schemas.RakshaRequest: The processed request
//   - *schemas.LLMPluginShortCircuit: The plugin short circuit if the request is not allowed
//   - error: Any error that occurred during processing
func (p *JsonParserPlugin) PreLLMHook(ctx *schemas.RakshaContext, req *schemas.RakshaRequest) (*schemas.RakshaRequest, *schemas.LLMPluginShortCircuit, error) {
	return req, nil, nil
}

// PostLLMHook processes streaming responses by accumulating chunks and making accumulated content valid JSON
// Parameters:
//   - ctx: The Raksha context
//   - result: The Raksha response to be processed
//   - err: The Raksha error to be processed
//
// Returns:
//   - *schemas.RakshaResponse: The processed response
//   - *schemas.RakshaError: The processed error
//   - error: Any error that occurred during processing
func (p *JsonParserPlugin) PostLLMHook(ctx *schemas.RakshaContext, result *schemas.RakshaResponse, err *schemas.RakshaError) (*schemas.RakshaResponse, *schemas.RakshaError, error) {
	// If there's an error, don't process
	if err != nil {
		return result, err, nil
	}

	if result == nil {
		return result, err, nil
	}

	extraFields := result.GetExtraFields()

	// Check if plugin should run based on usage type
	if !p.shouldRun(ctx, extraFields.RequestType) {
		return result, err, nil
	}

	// If no supported response type, return as is
	if result.ChatResponse == nil && result.ResponsesStreamResponse == nil {
		return result, err, nil
	}

	// Get request ID for state management, if it's not set, return as is
	requestID := p.getRequestID(ctx, result)
	if requestID == "" {
		return result, err, nil
	}

	// Create a deep copy of the result to avoid modifying the original pointer
	// This ensures other plugins using the same pointer don't get corrupted data
	resultCopy := p.deepCopyRakshaResponse(result)
	if resultCopy == nil {
		return result, err, nil
	}

	if extraFields.RequestType == schemas.ChatCompletionStreamRequest {
		if resultCopy.ChatResponse == nil {
			return result, err, nil
		}

		// Process only streaming choices to accumulate and fix partial JSON
		if len(resultCopy.ChatResponse.Choices) > 0 {
			for i := range resultCopy.ChatResponse.Choices {
				choice := &resultCopy.ChatResponse.Choices[i]

				// Handle only streaming response
				if choice.ChatStreamResponseChoice != nil {
					if choice.ChatStreamResponseChoice.Delta != nil && choice.ChatStreamResponseChoice.Delta.Content != nil {
						content := *choice.ChatStreamResponseChoice.Delta.Content
						if content != "" {
							// Accumulate the content
							accumulated := p.accumulateContent(requestID, content)

							// Process the accumulated content to make it valid JSON
							fixedContent := p.parsePartialJSON(accumulated)

							if !p.isValidJSON(fixedContent) {
								err = &schemas.RakshaError{
									Error: &schemas.ErrorField{
										Message: "Invalid JSON in streaming response",
									},
									StreamControl: &schemas.StreamControl{
										SkipStream: raksha.Ptr(true),
									},
								}

								return nil, err, nil
							}

							// Replace the delta content with the complete valid JSON
							choice.ChatStreamResponseChoice.Delta.Content = &fixedContent
						}
					}
				}
			}
		}
	} else if extraFields.RequestType == schemas.ResponsesStreamRequest {
		if resultCopy.ResponsesStreamResponse == nil {
			return result, err, nil
		}

		resp := resultCopy.ResponsesStreamResponse

		// Only process output text delta events
		if resp.Type == schemas.ResponsesStreamResponseTypeOutputTextDelta && resp.Delta != nil && *resp.Delta != "" {
			accumulated := p.accumulateContent(requestID, *resp.Delta)

			fixedContent := p.parsePartialJSON(accumulated)

			if !p.isValidJSON(fixedContent) {
				err = &schemas.RakshaError{
					Error: &schemas.ErrorField{
						Message: "Invalid JSON in streaming response",
					},
					StreamControl: &schemas.StreamControl{
						SkipStream: raksha.Ptr(true),
					},
				}

				return nil, err, nil
			}

			resp.Delta = &fixedContent
		}
	}

	// If this is the final chunk, cleanup the accumulated content for this request
	if streamEndIndicatorValue := ctx.Value(schemas.RakshaContextKeyStreamEndIndicator); streamEndIndicatorValue != nil {
		isFinalChunk, ok := streamEndIndicatorValue.(bool)
		if ok && isFinalChunk {
			p.ClearRequestState(requestID)
		}
	}

	// Return the modified copy instead of the original
	return resultCopy, err, nil
}

// Cleanup performs plugin cleanup and clears accumulated content
func (p *JsonParserPlugin) Cleanup() error {
	// Stop the cleanup goroutine
	p.StopCleanup()

	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Clear accumulated content
	p.accumulatedContent = make(map[string]*AccumulatedContent)
	return nil
}

// ClearRequestState clears the accumulated content for a specific request
func (p *JsonParserPlugin) ClearRequestState(requestID string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	delete(p.accumulatedContent, requestID)
}

// CLEANUP METHODS

// startCleanupGoroutine starts a goroutine that periodically cleans up old accumulated content
func (p *JsonParserPlugin) startCleanupGoroutine() {
	ticker := time.NewTicker(p.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.cleanupOldEntries()
		case <-p.stopCleanup:
			return
		}
	}
}

// cleanupOldEntries removes accumulated content entries that are older than maxAge
func (p *JsonParserPlugin) cleanupOldEntries() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	now := time.Now()
	cutoff := now.Add(-p.maxAge)

	for requestID, content := range p.accumulatedContent {
		if content.Timestamp.Before(cutoff) {
			delete(p.accumulatedContent, requestID)
		}
	}
}

// StopCleanup stops the cleanup goroutine
func (p *JsonParserPlugin) StopCleanup() {
	p.stopOnce.Do(func() {
		close(p.stopCleanup)
	})
}
