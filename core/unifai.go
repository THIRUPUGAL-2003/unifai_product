// Package unifai provides the core implementation of the UnifAI system.
// UnifAI is a unified interface for interacting with various AI model providers,
// managing concurrent requests, and handling provider-specific configurations.
package unifai

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"

	"github.com/unifai/unifai/core/keyselectors"
	"github.com/unifai/unifai/core/mcp"
	"github.com/unifai/unifai/core/mcp/codemode/starlark"
	"github.com/unifai/unifai/core/mcp/credstore"
	"github.com/unifai/unifai/core/providers/anthropic"
	"github.com/unifai/unifai/core/providers/azure"
	"github.com/unifai/unifai/core/providers/bedrock"
	"github.com/unifai/unifai/core/providers/bedrockmantle"
	"github.com/unifai/unifai/core/providers/cerebras"
	"github.com/unifai/unifai/core/providers/openaicompat"
	"github.com/unifai/unifai/core/providers/cohere"
	"github.com/unifai/unifai/core/providers/deepseek"
	"github.com/unifai/unifai/core/providers/elevenlabs"
	"github.com/unifai/unifai/core/providers/fireworks"
	"github.com/unifai/unifai/core/providers/gemini"
	"github.com/unifai/unifai/core/providers/groq"
	"github.com/unifai/unifai/core/providers/huggingface"
	"github.com/unifai/unifai/core/providers/mistral"
	"github.com/unifai/unifai/core/providers/nebius"
	"github.com/unifai/unifai/core/providers/ollama"
	"github.com/unifai/unifai/core/providers/openai"
	"github.com/unifai/unifai/core/providers/opencode"
	"github.com/unifai/unifai/core/providers/openrouter"
	"github.com/unifai/unifai/core/providers/parasail"
	"github.com/unifai/unifai/core/providers/perplexity"
	"github.com/unifai/unifai/core/providers/replicate"
	"github.com/unifai/unifai/core/providers/runware"
	"github.com/unifai/unifai/core/providers/runway"
	"github.com/unifai/unifai/core/providers/sgl"
	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/providers/vertex"
	"github.com/unifai/unifai/core/providers/vllm"
	"github.com/unifai/unifai/core/providers/xai"
	schemas "github.com/unifai/unifai/core/schemas"
	"github.com/valyala/fasthttp"
)

// ChannelMessage represents a message passed through the request channel.
// It contains the request, response and error channels, and the request type.
type ChannelMessage struct {
	schemas.UnifAIRequest
	Context        *schemas.UnifAIContext
	Response       chan *schemas.UnifAIResponse
	ResponseStream chan chan *schemas.UnifAIStreamChunk
	Err            chan schemas.UnifAIError
}

// UnifAI manages providers and maintains specified open channels for concurrent processing.
// It handles request routing, provider management, and response processing.
type UnifAI struct {
	ctx                 *schemas.UnifAIContext
	cancel              context.CancelFunc
	account             schemas.Account                     // account interface
	llmPlugins          atomic.Pointer[[]schemas.LLMPlugin] // list of llm plugins
	mcpPlugins          atomic.Pointer[[]schemas.MCPPlugin] // list of mcp plugins
	providers           atomic.Pointer[[]schemas.Provider]  // list of providers
	requestQueues       sync.Map                            // provider request queues (thread-safe), stores *ProviderQueue
	waitGroups          sync.Map                            // wait groups for each provider (thread-safe)
	oldWorkerCleanups   sync.WaitGroup                      // tracks async cleanup of old workers after provider updates
	retiredWorkerWaits  sync.Map                            // provider old-worker cleanup wait groups (thread-safe), stores *sync.WaitGroup
	providerLifecycleMu sync.RWMutex                        // prevents provider updates from racing with shutdown cleanup waits
	providerMutexes     sync.Map                            // mutexes for each provider to prevent concurrent updates (thread-safe)
	channelMessagePool  sync.Pool                           // Pool for ChannelMessage objects, initial pool size is set in Init
	responseChannelPool sync.Pool                           // Pool for response channels, initial pool size is set in Init
	errorChannelPool    sync.Pool                           // Pool for error channels, initial pool size is set in Init
	responseStreamPool  sync.Pool                           // Pool for response stream channels, initial pool size is set in Init
	pluginPipelinePool  sync.Pool                           // Pool for PluginPipeline objects
	unifaiRequestPool  sync.Pool                           // Pool for UnifAIRequest objects
	logger              schemas.Logger                      // logger instance, default logger is used if not provided
	tracer              atomic.Value                        // tracer for distributed tracing (stores schemas.Tracer, NoOpTracer if not configured)
	MCPManager          mcp.MCPManagerInterface             // MCP integration manager (nil if MCP not configured)
	mcpCredStore        schemas.MCPCredentialStore          // Per-call credential resolver for MCP tool execution (wraps oauth2Provider for OAuth-flavored auth types)
	mcpInitOnce         sync.Once                           // Ensures MCP manager is initialized only once
	dropExcessRequests  atomic.Bool                         // If true, in cases where the queue is full, requests will not wait for the queue to be empty and will be dropped instead.
	keySelector         schemas.KeySelector                 // Custom key selector function
	keyPoolFilter       schemas.KeyPoolFilter               // optional hook to veto keys before selection (nil = all eligible)
	kvStore             schemas.KVStore                     // optional KV store for session stickiness (nil = disabled)
}

// ProviderQueue wraps a provider's request channel with lifecycle management
// to prevent "send on closed channel" panics during provider removal/update.
// Producers must check the closing flag or select on the done channel before sending.
//
// Why pq.queue is NEVER closed:
//
// Closing a channel in Go causes any concurrent send to that channel to panic
// ("send on closed channel"). There is always a TOCTOU window between a
// producer's isClosing() check and its select { case pq.queue <- msg: ... }:
// the producer could pass isClosing() while the queue is open, get preempted,
// and resume only after the queue is closed. Go's selectgo evaluates select
// cases in a random order, so even having case <-pq.done: in the same select
// does not protect against this — if selectgo evaluates the send case first on
// a closed channel it panics immediately via goto sclose, before reaching done.
//
// To close pq.queue safely you would need a sender-side WaitGroup so that
// signalClosing could wait for every in-flight producer to finish. That adds
// non-trivial overhead on the hot request path.
//
// Instead, pq.done is the sole shutdown signal. Receiving from a closed channel
// is always safe (returns the zero value immediately), so:
//   - Workers exit via case <-pq.done: — safe
//   - Producers bail via case <-pq.done: — safe
//   - drainQueueWithErrors handles any messages that slip through the TOCTOU window
//
// pq.queue is garbage collected automatically:
//   - RemoveProvider calls requestQueues.Delete, dropping the map's reference.
//   - UpdateProvider calls requestQueues.Store with a new queue, dropping the
//     map's reference to oldPq. Shutdown does not Delete at all — the whole
//     UnifAI instance is torn down.
//     In all cases, once no producer goroutine holds a reference to the
//     ProviderQueue, both the struct and pq.queue are eligible for GC.
//     No explicit close is needed.
type ProviderQueue struct {
	queue      chan *ChannelMessage // the actual request queue channel — never closed, see above
	done       chan struct{}        // closed by signalClosing() to signal shutdown; never written to otherwise
	closing    uint32               // atomic: 0 = open, 1 = closing
	signalOnce sync.Once
}

func isLargePayloadPassthrough(ctx *schemas.UnifAIContext) bool {
	if ctx == nil {
		return false
	}
	// Large payload mode intentionally skips JSON->UnifAI input materialization.
	// Example: a 400MB multipart/audio upload sets Input=nil by design; strict
	// non-nil validation here would reject valid passthrough requests.
	isLargePayload, _ := ctx.Value(schemas.UnifAIContextKeyLargePayloadMode).(bool)
	if !isLargePayload {
		return false
	}
	// Verify reader is present (flag and reader are always set together by middleware)
	reader := ctx.Value(schemas.UnifAIContextKeyLargePayloadReader)
	return reader != nil
}

// signalClosing signals the closing of the provider queue.
// This is lock-free: uses atomic store and sync.Once to safely signal shutdown.
func (pq *ProviderQueue) signalClosing() {
	pq.signalOnce.Do(func() {
		atomic.StoreUint32(&pq.closing, 1)
		close(pq.done)
	})
}

// isClosing returns true if the provider queue is closing.
// Uses atomic load for lock-free checking.
func (pq *ProviderQueue) isClosing() bool {
	return atomic.LoadUint32(&pq.closing) == 1
}

// PluginPipeline encapsulates the execution of plugin PreHooks and PostHooks, tracks how many plugins ran, and manages short-circuiting and error aggregation.
type PluginPipeline struct {
	llmPlugins []schemas.LLMPlugin
	mcpPlugins []schemas.MCPPlugin
	logger     schemas.Logger
	tracer     schemas.Tracer

	// Number of PreHooks that were executed (used to determine which PostHooks to run in reverse order)
	executedPreHooks int
	// Errors from PreHooks and PostHooks
	preHookErrors  []error
	postHookErrors []error

	// streamingMu guards the streaming post-hook accumulators below. Per-chunk
	// writes (accumulatePluginTiming) run in the provider goroutine while the
	// end-of-stream finalizer (FinalizeStreamingPostHookSpans) and
	// resetPluginPipeline can run in a different goroutine, so unsynchronised
	// access triggers "concurrent map read and map write" panics.
	streamingMu         sync.Mutex
	postHookTimings     map[string]*pluginTimingAccumulator // keyed by plugin name
	postHookPluginOrder []string                            // order in which post-hooks ran (for nested span creation)
	chunkCount          int

	// Plugin logging: cached scoped contexts for streaming post-hooks (reused across chunks)
	streamScopedCtxs map[string]*schemas.UnifAIContext
}

// pluginTimingAccumulator accumulates timing information for a plugin across streaming chunks
type pluginTimingAccumulator struct {
	totalDuration time.Duration
	invocations   int
	errors        int
}

// tracerWrapper wraps a Tracer to ensure atomic.Value stores consistent types.
// This is necessary because atomic.Value.Store() panics if called with values
// of different concrete types, even if they implement the same interface.
type tracerWrapper struct {
	tracer schemas.Tracer
}

// INITIALIZATION

// Init initializes a new UnifAI instance with the given configuration.
// It sets up the account, plugins, object pools, and initializes providers.
// Returns an error if initialization fails.
// Initial Memory Allocations happens here as per the initial pool size.
func Init(ctx context.Context, config schemas.UnifAIConfig) (*UnifAI, error) {
	if config.Account == nil {
		return nil, fmt.Errorf("account is required to initialize UnifAI")
	}

	if config.Logger == nil {
		config.Logger = NewDefaultLogger(schemas.LogLevelInfo)
	}
	providerUtils.SetLogger(config.Logger)

	// Initialize tracer (use NoOpTracer if not provided)
	tracer := config.Tracer
	if tracer == nil {
		tracer = schemas.DefaultTracer()
	}

	unifaiCtx, cancel := schemas.NewUnifAIContextWithCancel(ctx)
	unifai := &UnifAI{
		ctx:           unifaiCtx,
		cancel:        cancel,
		account:       config.Account,
		llmPlugins:    atomic.Pointer[[]schemas.LLMPlugin]{},
		mcpPlugins:    atomic.Pointer[[]schemas.MCPPlugin]{},
		requestQueues: sync.Map{},
		waitGroups:    sync.Map{},
		keySelector:   config.KeySelector,
		keyPoolFilter: config.KeyPoolFilter,
		mcpCredStore:  credstore.NewCredStore(config.OAuth2Provider, config.MCPHeadersProvider, config.Logger),
		logger:        config.Logger,
		kvStore:       config.KVStore,
	}
	unifai.tracer.Store(&tracerWrapper{tracer: tracer})
	if config.LLMPlugins == nil {
		config.LLMPlugins = make([]schemas.LLMPlugin, 0)
	}
	if config.MCPPlugins == nil {
		config.MCPPlugins = make([]schemas.MCPPlugin, 0)
	}
	unifai.llmPlugins.Store(&config.LLMPlugins)
	unifai.mcpPlugins.Store(&config.MCPPlugins)

	// Initialize providers slice
	unifai.providers.Store(&[]schemas.Provider{})

	unifai.dropExcessRequests.Store(config.DropExcessRequests)

	if unifai.keySelector == nil {
		unifai.keySelector = keyselectors.WeightedRandom
	}

	// Initialize object pools
	unifai.channelMessagePool = sync.Pool{
		New: func() interface{} {
			return &ChannelMessage{}
		},
	}
	unifai.responseChannelPool = sync.Pool{
		New: func() interface{} {
			return make(chan *schemas.UnifAIResponse, 1)
		},
	}
	unifai.errorChannelPool = sync.Pool{
		New: func() interface{} {
			return make(chan schemas.UnifAIError, 1)
		},
	}
	unifai.responseStreamPool = sync.Pool{
		New: func() interface{} {
			return make(chan chan *schemas.UnifAIStreamChunk, 1)
		},
	}
	unifai.pluginPipelinePool = sync.Pool{
		New: func() interface{} {
			return &PluginPipeline{
				preHookErrors:  make([]error, 0),
				postHookErrors: make([]error, 0),
			}
		},
	}
	unifai.unifaiRequestPool = sync.Pool{
		New: func() interface{} {
			return &schemas.UnifAIRequest{}
		},
	}
	// Prewarm pools. The MCP request pool is owned by the mcp package now —
	// see core/mcp/exec.go.
	for range config.InitialPoolSize {
		// Create and put new objects directly into pools
		unifai.channelMessagePool.Put(&ChannelMessage{})
		unifai.responseChannelPool.Put(make(chan *schemas.UnifAIResponse, 1))
		unifai.errorChannelPool.Put(make(chan schemas.UnifAIError, 1))
		unifai.responseStreamPool.Put(make(chan chan *schemas.UnifAIStreamChunk, 1))
		unifai.pluginPipelinePool.Put(&PluginPipeline{
			preHookErrors:  make([]error, 0),
			postHookErrors: make([]error, 0),
		})
		unifai.unifaiRequestPool.Put(&schemas.UnifAIRequest{})
	}

	providerKeys, err := unifai.account.GetConfiguredProviders()
	if err != nil {
		return nil, err
	}

	// Initialize MCP manager if configured
	if config.MCPConfig != nil {
		unifai.mcpInitOnce.Do(func() {
			// Set up plugin pipeline provider functions for executeCode tool hooks
			mcpConfig := *config.MCPConfig
			mcpConfig.PluginPipelineProvider = func() interface{} {
				return unifai.getPluginPipeline()
			}
			mcpConfig.ReleasePluginPipeline = func(pipeline interface{}) {
				if pp, ok := pipeline.(*PluginPipeline); ok {
					unifai.releasePluginPipeline(pp)
				}
			}
			// Create Starlark CodeMode for code execution
			var codeModeConfig *mcp.CodeModeConfig
			if mcpConfig.ToolManagerConfig != nil {
				codeModeConfig = &mcp.CodeModeConfig{
					BindingLevel:         mcpConfig.ToolManagerConfig.CodeModeBindingLevel,
					ToolExecutionTimeout: time.Duration(mcpConfig.ToolManagerConfig.ToolExecutionTimeout),
				}
			}
			codeMode := starlark.NewStarlarkCodeMode(codeModeConfig, unifai.logger)
			unifai.MCPManager = mcp.NewMCPManager(unifaiCtx, mcpConfig, unifai.mcpCredStore, unifai.logger, codeMode)
			unifai.logger.Info("MCP integration initialized successfully")
		})
	}

	// Create buffered channels for each provider and start workers
	for _, providerKey := range providerKeys {
		if strings.TrimSpace(string(providerKey)) == "" {
			unifai.logger.Warn("provider key is empty, skipping init")
			continue
		}

		config, err := unifai.account.GetConfigForProvider(providerKey)
		if err != nil {
			unifai.logger.Warn("failed to get config for provider %s, skipping init: %v", providerKey, err)
			continue
		}
		if config == nil {
			unifai.logger.Warn("config is nil for provider %s, skipping init", providerKey)
			continue
		}

		// Lock the provider mutex during initialization
		providerMutex := unifai.getProviderMutex(providerKey)
		providerMutex.Lock()
		err = unifai.prepareProvider(providerKey, config)
		providerMutex.Unlock()

		if err != nil {
			unifai.logger.Warn("failed to prepare provider %s: %v", providerKey, err)
		}
	}
	return unifai, nil
}

// SetTracer sets the tracer for the UnifAI instance.
func (unifai *UnifAI) SetTracer(tracer schemas.Tracer) {
	if tracer == nil {
		// Fall back to no-op tracer if not provided
		tracer = schemas.DefaultTracer()
	}
	unifai.tracer.Store(&tracerWrapper{tracer: tracer})
}

// getTracer returns the tracer from atomic storage with type assertion.
func (unifai *UnifAI) getTracer() schemas.Tracer {
	return unifai.tracer.Load().(*tracerWrapper).tracer
}

// ReloadConfig reloads the config from DB
// Currently we update account, drop excess requests, and plugin lists
// We will keep on adding other aspects as required
func (unifai *UnifAI) ReloadConfig(config schemas.UnifAIConfig) error {
	unifai.dropExcessRequests.Store(config.DropExcessRequests)
	return nil
}

// PUBLIC API METHODS

// ListModelsRequest sends a list models request to the specified provider.
func (unifai *UnifAI) ListModelsRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIListModelsRequest) (*schemas.UnifAIListModelsResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "list models request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ListModelsRequest,
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for list models request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ListModelsRequest,
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	reqCtx := schemas.NewUnifAIContext(ctx, schemas.NoDeadline)
	reqCtx.SetValue(schemas.UnifAIContextKeySkipBudgetAndRateLimits, true)

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ListModelsRequest
	unifaiReq.ListModelsRequest = req

	resp, err := unifai.handleRequest(reqCtx, unifaiReq)
	if err != nil {
		return nil, err
	}

	return resp.ListModelsResponse, nil
}

// ListAllModels lists all models from all configured providers.
// It accumulates responses from all providers with a limit of 1000 per provider to get all results.
func (unifai *UnifAI) ListAllModels(ctx *schemas.UnifAIContext, req *schemas.UnifAIListModelsRequest) (*schemas.UnifAIListModelsResponse, *schemas.UnifAIError) {
	if req == nil {
		req = &schemas.UnifAIListModelsRequest{}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	providerKeys, err := unifai.GetConfiguredProviders()
	if err != nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: err.Error(),
				Error:   err,
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ListModelsRequest,
			},
		}
	}
	providerKeys = filterProvidersByContext(ctx, providerKeys)

	startTime := time.Now()

	// Result structure for collecting provider responses
	type providerResult struct {
		provider    schemas.ModelProvider
		models      []schemas.Model
		keyStatuses []schemas.KeyStatus
		err         *schemas.UnifAIError
	}

	results := make(chan providerResult, len(providerKeys))
	var wg sync.WaitGroup

	// Launch concurrent requests for all providers
	for _, providerKey := range providerKeys {
		if strings.TrimSpace(string(providerKey)) == "" {
			continue
		}

		wg.Add(1)
		go func(providerKey schemas.ModelProvider) {
			defer wg.Done()

			providerCtx := schemas.NewUnifAIContext(ctx, schemas.NoDeadline)
			providerCtx.SetValue(schemas.UnifAIContextKeyRequestID, uuid.New().String())

			providerModels := make([]schemas.Model, 0)
			var providerKeyStatuses []schemas.KeyStatus
			var providerErr *schemas.UnifAIError

			// Create request for this provider with limit of 1000
			providerRequest := &schemas.UnifAIListModelsRequest{
				Provider:   providerKey,
				PageSize:   schemas.DefaultPageSize,
				Unfiltered: req.Unfiltered,
			}

			iterations := 0
			for {
				// check for context cancellation
				select {
				case <-ctx.Done():
					unifai.logger.Warn("context cancelled for provider %s", providerKey)
					return
				default:
				}

				iterations++
				if iterations > schemas.MaxPaginationRequests {
					unifai.logger.Warn("reached maximum pagination requests (%d) for provider %s, please increase the page size", schemas.MaxPaginationRequests, providerKey)
					break
				}

				response, unifaiErr := unifai.ListModelsRequest(providerCtx, providerRequest)
				if unifaiErr != nil {
					// Some per-provider failures are expected when fanning out across all
					// configured providers and must not be surfaced as a top-level error
					errType := ""
					if unifaiErr.Type != nil {
						errType = *unifaiErr.Type
					}
					errMsg := ""
					if unifaiErr.Error != nil {
						errMsg = unifaiErr.Error.Message
					}
					isExpected := strings.Contains(errMsg, "no keys found") ||
						strings.Contains(errMsg, "not supported") ||
						errType == "provider_blocked"
					if !isExpected {
						providerErr = unifaiErr
						unifai.logger.Warn("failed to list models for provider %s: %s", providerKey, unifaiErr.GetErrorString())
					}
					// Collect key statuses from error (failure case)
					if len(unifaiErr.ExtraFields.KeyStatuses) > 0 {
						providerKeyStatuses = append(providerKeyStatuses, unifaiErr.ExtraFields.KeyStatuses...)
					}
					break
				}

				if response == nil || len(response.Data) == 0 {
					break
				}

				providerModels = append(providerModels, response.Data...)

				if len(response.KeyStatuses) > 0 {
					providerKeyStatuses = append(providerKeyStatuses, response.KeyStatuses...)
				}

				// Check if there are more pages
				if response.NextPageToken == "" {
					break
				}

				// Set the page token for the next request
				providerRequest.PageToken = response.NextPageToken
			}

			results <- providerResult{
				provider:    providerKey,
				models:      providerModels,
				keyStatuses: providerKeyStatuses,
				err:         providerErr,
			}
		}(providerKey)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(results)

	// Accumulate all models and key statuses from all providers
	allModels := make([]schemas.Model, 0)
	allKeyStatuses := make([]schemas.KeyStatus, 0)
	var firstError *schemas.UnifAIError

	for result := range results {
		if len(result.models) > 0 {
			allModels = append(allModels, result.models...)
		}
		if len(result.keyStatuses) > 0 {
			allKeyStatuses = append(allKeyStatuses, result.keyStatuses...)
		}
		if result.err != nil && firstError == nil {
			firstError = result.err
		}
	}

	// If we couldn't get any models from any provider, return the first error
	if len(allModels) == 0 && firstError != nil {
		// Attach all key statuses to the error
		firstError.ExtraFields.KeyStatuses = allKeyStatuses
		return nil, firstError
	}

	// Sort models alphabetically by ID
	sort.Slice(allModels, func(i, j int) bool {
		return allModels[i].ID < allModels[j].ID
	})

	// Return aggregated response with accumulated latency and key statuses
	response := &schemas.UnifAIListModelsResponse{
		Data:        allModels,
		KeyStatuses: allKeyStatuses,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			RequestType: schemas.ListModelsRequest,
			Latency:     time.Since(startTime).Milliseconds(),
		},
	}

	response = response.ApplyPagination(req.PageSize, req.PageToken)

	return response, nil
}

func filterProvidersByContext(ctx *schemas.UnifAIContext, providerKeys []schemas.ModelProvider) []schemas.ModelProvider {
	if ctx == nil {
		return providerKeys
	}

	rawAvailableProviders := ctx.Value(schemas.UnifAIContextKeyAvailableProviders)
	if rawAvailableProviders == nil {
		return providerKeys
	}

	availableProviders, ok := rawAvailableProviders.([]schemas.ModelProvider)
	if !ok {
		return []schemas.ModelProvider{}
	}

	if len(availableProviders) == 0 || len(providerKeys) == 0 {
		return []schemas.ModelProvider{}
	}

	filteredProviders := make([]schemas.ModelProvider, 0, len(providerKeys))
	for _, providerKey := range providerKeys {
		if slices.Contains(availableProviders, providerKey) {
			filteredProviders = append(filteredProviders, providerKey)
		}
	}

	return filteredProviders
}

// TextCompletionRequest sends a text completion request to the specified provider.
func (unifai *UnifAI) TextCompletionRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAITextCompletionRequest) (*schemas.UnifAITextCompletionResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "text completion request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.TextCompletionRequest,
			},
		}
	}
	if (req.Input == nil || (req.Input.PromptStr == nil && req.Input.PromptArray == nil)) && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "prompt not provided for text completion request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.TextCompletionRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}
	// Preparing request
	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.TextCompletionRequest
	unifaiReq.TextCompletionRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	// TODO: Release the response
	return response.TextCompletionResponse, nil
}

// TextCompletionStreamRequest sends a streaming text completion request to the specified provider.
func (unifai *UnifAI) TextCompletionStreamRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAITextCompletionRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "text completion stream request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.TextCompletionStreamRequest,
			},
		}
	}
	if (req.Input == nil || (req.Input.PromptStr == nil && req.Input.PromptArray == nil)) && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "text not provided for text completion stream request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.TextCompletionStreamRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}
	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.TextCompletionStreamRequest
	unifaiReq.TextCompletionRequest = req
	return unifai.handleStreamRequest(ctx, unifaiReq)
}

func (unifai *UnifAI) makeChatCompletionRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIChatRequest) (*schemas.UnifAIChatResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "chat completion request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ChatCompletionRequest,
			},
		}
	}
	if req.Input == nil && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "chats not provided for chat completion request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.ChatCompletionRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ChatCompletionRequest
	unifaiReq.ChatRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}

	return response.ChatResponse, nil
}

// ChatCompletionRequest sends a chat completion request to the specified provider.
func (unifai *UnifAI) ChatCompletionRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIChatRequest) (*schemas.UnifAIChatResponse, *schemas.UnifAIError) {
	// If ctx is nil, use the unifai context (defensive check for mcp agent mode)
	if ctx == nil {
		ctx = unifai.ctx
	}

	response, err := unifai.makeChatCompletionRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	// Check if we should enter agent mode.
	if unifai.MCPManager != nil {
		return unifai.MCPManager.CheckAndExecuteAgentForChatRequest(
			ctx,
			req,
			response,
			unifai.makeChatCompletionRequest,
		)
	}

	return response, nil
}

// ChatCompletionStreamRequest sends a chat completion stream request to the specified provider.
func (unifai *UnifAI) ChatCompletionStreamRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIChatRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "chat completion stream request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ChatCompletionStreamRequest,
			},
		}
	}
	if req.Input == nil && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "chats not provided for chat completion request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.ChatCompletionStreamRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ChatCompletionStreamRequest
	unifaiReq.ChatRequest = req

	return unifai.handleStreamRequest(ctx, unifaiReq)
}

func (unifai *UnifAI) makeResponsesRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIResponsesRequest) (*schemas.UnifAIResponsesResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "responses request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ResponsesRequest,
			},
		}
	}
	// In large payload mode, Input is intentionally nil — body streams directly to upstream
	if req.Input == nil {
		isLargePayload, _ := ctx.Value(schemas.UnifAIContextKeyLargePayloadMode).(bool)
		if !isLargePayload {
			return nil, &schemas.UnifAIError{
				IsUnifAIError: false,
				Error: &schemas.ErrorField{
					Message: "responses not provided for responses request",
				},
				ExtraFields: schemas.UnifAIErrorExtraFields{
					RequestType:            schemas.ResponsesRequest,
					Provider:               req.Provider,
					OriginalModelRequested: req.Model,
					ResolvedModelUsed:      req.Model,
				},
			}
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ResponsesRequest
	unifaiReq.ResponsesRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.ResponsesResponse, nil
}

// ResponsesRequest sends a responses request to the specified provider.
func (unifai *UnifAI) ResponsesRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIResponsesRequest) (*schemas.UnifAIResponsesResponse, *schemas.UnifAIError) {
	// If ctx is nil, use the unifai context (defensive check for mcp agent mode)
	if ctx == nil {
		ctx = unifai.ctx
	}

	response, err := unifai.makeResponsesRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	// Check if we should enter agent mode.
	if unifai.MCPManager != nil {
		return unifai.MCPManager.CheckAndExecuteAgentForResponsesRequest(
			ctx,
			req,
			response,
			unifai.makeResponsesRequest,
		)
	}

	return response, nil
}

// ResponsesStreamRequest sends a responses stream request to the specified provider.
func (unifai *UnifAI) ResponsesStreamRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIResponsesRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "responses stream request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ResponsesStreamRequest,
			},
		}
	}
	// In large payload mode, Input is intentionally nil — body streams directly to upstream
	if req.Input == nil {
		isLargePayload, _ := ctx.Value(schemas.UnifAIContextKeyLargePayloadMode).(bool)
		if !isLargePayload {
			return nil, &schemas.UnifAIError{
				IsUnifAIError: false,
				Error: &schemas.ErrorField{
					Message: "responses not provided for responses stream request",
				},
				ExtraFields: schemas.UnifAIErrorExtraFields{
					RequestType:            schemas.ResponsesStreamRequest,
					Provider:               req.Provider,
					OriginalModelRequested: req.Model,
					ResolvedModelUsed:      req.Model,
				},
			}
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ResponsesStreamRequest
	unifaiReq.ResponsesRequest = req

	return unifai.handleStreamRequest(ctx, unifaiReq)
}

// CountTokensRequest sends a count tokens request to the specified provider.
func (unifai *UnifAI) CountTokensRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIResponsesRequest) (*schemas.UnifAICountTokensResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "count tokens request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.CountTokensRequest,
			},
		}
	}
	if req.Input == nil && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "input not provided for count tokens request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.CountTokensRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.CountTokensRequest
	unifaiReq.CountTokensRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}

	return response.CountTokensResponse, nil
}

// CompactionRequest compacts a conversation context window via providers that implement
// the OpenAI-compatible /v1/responses/compact flow (OpenAI, Azure OpenAI, xAI).
// Providers without compaction support return an unsupported-operation error.
func (unifai *UnifAI) CompactionRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAICompactionRequest) (*schemas.UnifAICompactionResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "compaction request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.CompactionRequest,
			},
		}
	}

	if len(req.Input) == 0 && req.PreviousResponseID == nil && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "input not provided for compaction request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.CompactionRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.CompactionRequest
	unifaiReq.CompactionRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}

	return response.CompactionResponse, nil
}

// ResponsesRetrieveRequest retrieves a stored response by ID (OpenAI GET /v1/responses/{id}).
func (unifai *UnifAI) ResponsesRetrieveRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIResponsesRetrieveRequest) (*schemas.UnifAIResponsesResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "responses retrieve request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ResponsesRetrieveRequest,
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for responses retrieve request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ResponsesRetrieveRequest,
				Provider:    req.Provider,
			},
		}
	}
	if req.ResponseID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "response_id is required for responses retrieve request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ResponsesRetrieveRequest,
				Provider:    req.Provider,
			},
		}
	}
	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ResponsesRetrieveRequest
	unifaiReq.ResponsesRetrieveRequest = req
	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.ResponsesResponse, nil
}

// ResponsesDeleteRequest deletes a stored response (OpenAI DELETE /v1/responses/{id}).
func (unifai *UnifAI) ResponsesDeleteRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIResponsesDeleteRequest) (*schemas.UnifAIResponsesDeleteResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "responses delete request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ResponsesDeleteRequest,
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for responses delete request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ResponsesDeleteRequest,
			},
		}
	}
	if req.ResponseID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "response_id is required for responses delete request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ResponsesDeleteRequest,
				Provider:    req.Provider,
			},
		}
	}
	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ResponsesDeleteRequest
	unifaiReq.ResponsesDeleteRequest = req
	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.ResponsesDeleteResponse, nil
}

// ResponsesCancelRequest cancels an in-flight stored response (OpenAI POST /v1/responses/{id}/cancel).
func (unifai *UnifAI) ResponsesCancelRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIResponsesCancelRequest) (*schemas.UnifAIResponsesResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "responses cancel request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ResponsesCancelRequest,
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for responses cancel request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ResponsesCancelRequest,
			},
		}
	}
	if req.ResponseID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "response_id is required for responses cancel request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ResponsesCancelRequest,
				Provider:    req.Provider,
			},
		}
	}
	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ResponsesCancelRequest
	unifaiReq.ResponsesCancelRequest = req
	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.ResponsesResponse, nil
}

// ResponsesInputItemsRequest lists input items for a stored response (OpenAI GET /v1/responses/{id}/input_items).
func (unifai *UnifAI) ResponsesInputItemsRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIResponsesInputItemsRequest) (*schemas.UnifAIResponsesInputItemsResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "responses input items request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ResponsesInputItemsRequest,
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for responses input items request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ResponsesInputItemsRequest,
			},
		}
	}
	if req.ResponseID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "response_id is required for responses input items request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ResponsesInputItemsRequest,
				Provider:    req.Provider,
			},
		}
	}
	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ResponsesInputItemsRequest
	unifaiReq.ResponsesInputItemsRequest = req
	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.ResponsesInputItemsResponse, nil
}

// EmbeddingRequest sends an embedding request to the specified provider.
func (unifai *UnifAI) EmbeddingRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIEmbeddingRequest) (*schemas.UnifAIEmbeddingResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "embedding request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.EmbeddingRequest,
			},
		}
	}
	hasExtraInputs := req.Params != nil && req.Params.ExtraParams != nil &&
		(req.Params.ExtraParams["inputs"] != nil || req.Params.ExtraParams["images"] != nil)
	if (req.Input == nil || (req.Input.Text == nil && req.Input.Texts == nil && req.Input.Embedding == nil && req.Input.Embeddings == nil)) && !hasExtraInputs && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "embedding input not provided for embedding request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.EmbeddingRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.EmbeddingRequest
	unifaiReq.EmbeddingRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	// TODO: Release the response
	return response.EmbeddingResponse, nil
}

// RerankRequest sends a rerank request to the specified provider.
func (unifai *UnifAI) RerankRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIRerankRequest) (*schemas.UnifAIRerankResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "rerank request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.RerankRequest,
			},
		}
	}
	if strings.TrimSpace(req.Query) == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "query not provided for rerank request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.RerankRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}
	if len(req.Documents) == 0 {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "documents not provided for rerank request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.RerankRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}
	for i, doc := range req.Documents {
		if strings.TrimSpace(doc.Text) == "" {
			return nil, &schemas.UnifAIError{
				IsUnifAIError: false,
				Error: &schemas.ErrorField{
					Message: fmt.Sprintf("document text is empty at index %d", i),
				},
				ExtraFields: schemas.UnifAIErrorExtraFields{
					RequestType:            schemas.RerankRequest,
					Provider:               req.Provider,
					OriginalModelRequested: req.Model,
					ResolvedModelUsed:      req.Model,
				},
			}
		}
	}
	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.RerankRequest
	unifaiReq.RerankRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.RerankResponse, nil
}

// OCRRequest sends an OCR request to the specified provider.
func (unifai *UnifAI) OCRRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIOCRRequest) (*schemas.UnifAIOCRResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "ocr request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.OCRRequest,
			},
		}
	}
	if strings.TrimSpace(string(req.Document.Type)) == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "document type not provided for ocr request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.OCRRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}
	if req.Document.Type == schemas.OCRDocumentTypeDocumentURL && (req.Document.DocumentURL == nil || strings.TrimSpace(*req.Document.DocumentURL) == "") {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "document_url not provided for document_url type ocr request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.OCRRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}
	if req.Document.Type == schemas.OCRDocumentTypeImageURL && (req.Document.ImageURL == nil || strings.TrimSpace(*req.Document.ImageURL) == "") {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "image_url not provided for image_url type ocr request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.OCRRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}
	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.OCRRequest
	unifaiReq.OCRRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.OCRResponse, nil
}

// SpeechRequest sends a speech request to the specified provider.
func (unifai *UnifAI) SpeechRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAISpeechRequest) (*schemas.UnifAISpeechResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "speech request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.SpeechRequest,
			},
		}
	}
	if (req.Input == nil || req.Input.Input == "") && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "speech input not provided for speech request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.SpeechRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.SpeechRequest
	unifaiReq.SpeechRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	// TODO: Release the response
	return response.SpeechResponse, nil
}

// SpeechStreamRequest sends a speech stream request to the specified provider.
func (unifai *UnifAI) SpeechStreamRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAISpeechRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "speech stream request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.SpeechStreamRequest,
			},
		}
	}
	if (req.Input == nil || req.Input.Input == "") && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "speech input not provided for speech stream request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.SpeechStreamRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.SpeechStreamRequest
	unifaiReq.SpeechRequest = req

	return unifai.handleStreamRequest(ctx, unifaiReq)
}

// TranscriptionRequest sends a transcription request to the specified provider.
func (unifai *UnifAI) TranscriptionRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAITranscriptionRequest) (*schemas.UnifAITranscriptionResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "transcription request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.TranscriptionRequest,
			},
		}
	}
	if (req.Input == nil || req.Input.File == nil) && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "transcription input not provided for transcription request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.TranscriptionRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.TranscriptionRequest
	unifaiReq.TranscriptionRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	// TODO: Release the response
	return response.TranscriptionResponse, nil
}

// TranscriptionStreamRequest sends a transcription stream request to the specified provider.
func (unifai *UnifAI) TranscriptionStreamRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAITranscriptionRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "transcription stream request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.TranscriptionStreamRequest,
			},
		}
	}
	if (req.Input == nil || req.Input.File == nil) && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "transcription input not provided for transcription stream request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.TranscriptionStreamRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.TranscriptionStreamRequest
	unifaiReq.TranscriptionRequest = req

	return unifai.handleStreamRequest(ctx, unifaiReq)
}

// ImageGenerationRequest sends an image generation request to the specified provider.
func (unifai *UnifAI) ImageGenerationRequest(ctx *schemas.UnifAIContext,
	req *schemas.UnifAIImageGenerationRequest,
) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "image generation request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ImageGenerationRequest,
			},
		}
	}
	if (req.Input == nil || req.Input.Prompt == "") && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "prompt not provided for image generation request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.ImageGenerationRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ImageGenerationRequest
	unifaiReq.ImageGenerationRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	if response == nil || response.ImageGenerationResponse == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "received nil response from provider",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.ImageGenerationRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	return response.ImageGenerationResponse, nil
}

// ImageGenerationStreamRequest sends an image generation stream request to the specified provider.
func (unifai *UnifAI) ImageGenerationStreamRequest(ctx *schemas.UnifAIContext,
	req *schemas.UnifAIImageGenerationRequest,
) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "image generation stream request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ImageGenerationStreamRequest,
			},
		}
	}
	if (req.Input == nil || req.Input.Prompt == "") && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "prompt not provided for image generation stream request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.ImageGenerationStreamRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ImageGenerationStreamRequest
	unifaiReq.ImageGenerationRequest = req

	return unifai.handleStreamRequest(ctx, unifaiReq)
}

// ImageEditRequest sends an image edit request to the specified provider.
func (unifai *UnifAI) ImageEditRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIImageEditRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "image edit request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ImageEditRequest,
			},
		}
	}
	if (req.Input == nil || req.Input.Images == nil || len(req.Input.Images) == 0) && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "images not provided for image edit request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.ImageEditRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}
	// Prompt is not required for certain operation types that work without a text prompt
	var imageEditParamsType *string
	if req.Params != nil {
		imageEditParamsType = req.Params.Type
	}
	if !isPromptOptionalImageEditType(imageEditParamsType) &&
		(req.Input == nil || req.Input.Prompt == "") && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "prompt not provided for image edit request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.ImageEditRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ImageEditRequest
	unifaiReq.ImageEditRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}

	if response == nil || response.ImageGenerationResponse == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "received nil response from provider",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.ImageEditRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	return response.ImageGenerationResponse, nil
}

// ImageEditStreamRequest sends an image edit stream request to the specified provider.
func (unifai *UnifAI) ImageEditStreamRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIImageEditRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "image edit stream request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ImageEditStreamRequest,
			},
		}
	}
	if (req.Input == nil || req.Input.Images == nil || len(req.Input.Images) == 0) && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "images not provided for image edit stream request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.ImageEditStreamRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}
	// Prompt is not required for certain operation types that work without a text prompt
	var imageEditStreamParamsType *string
	if req.Params != nil {
		imageEditStreamParamsType = req.Params.Type
	}
	if !isPromptOptionalImageEditType(imageEditStreamParamsType) &&
		(req.Input == nil || req.Input.Prompt == "") && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "prompt not provided for image edit stream request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.ImageEditStreamRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ImageEditStreamRequest
	unifaiReq.ImageEditRequest = req

	return unifai.handleStreamRequest(ctx, unifaiReq)
}

// ImageVariationRequest sends an image variation request to the specified provider.
func (unifai *UnifAI) ImageVariationRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIImageVariationRequest) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "image variation request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.ImageVariationRequest,
			},
		}
	}
	if (req.Input == nil || req.Input.Image.Image == nil || len(req.Input.Image.Image) == 0) && !isLargePayloadPassthrough(ctx) {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "image not provided for image variation request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.ImageVariationRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ImageVariationRequest
	unifaiReq.ImageVariationRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}

	if response == nil || response.ImageGenerationResponse == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "received nil response from provider",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.ImageVariationRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	return response.ImageGenerationResponse, nil
}

// VideoGenerationRequest sends a video generation request to the specified provider.
func (unifai *UnifAI) VideoGenerationRequest(ctx *schemas.UnifAIContext,
	req *schemas.UnifAIVideoGenerationRequest,
) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "video generation request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoGenerationRequest,
			},
		}
	}
	if req.Input == nil || req.Input.Prompt == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "prompt not provided for video generation request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.VideoGenerationRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.VideoGenerationRequest
	unifaiReq.VideoGenerationRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	if response == nil || response.VideoGenerationResponse == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "received nil response from provider",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            schemas.VideoGenerationRequest,
				Provider:               req.Provider,
				OriginalModelRequested: req.Model,
				ResolvedModelUsed:      req.Model,
			},
		}
	}

	return response.VideoGenerationResponse, nil
}

func (unifai *UnifAI) VideoRetrieveRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIVideoRetrieveRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "video retrieve request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoRetrieveRequest,
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for video retrieve request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoRetrieveRequest,
			},
		}
	}
	if req.ID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "video_id is required for video retrieve request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoRetrieveRequest,
				Provider:    req.Provider,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.VideoRetrieveRequest
	unifaiReq.VideoRetrieveRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	if response == nil || response.VideoGenerationResponse == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "received nil response from provider",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoRetrieveRequest,
				Provider:    req.Provider,
			},
		}
	}
	return response.VideoGenerationResponse, nil
}

// VideoDownloadRequest downloads video content from the provider.
func (unifai *UnifAI) VideoDownloadRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIVideoDownloadRequest) (*schemas.UnifAIVideoDownloadResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "video download request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoDownloadRequest,
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for video download request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoDownloadRequest,
			},
		}
	}
	if req.ID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "video_id is required for video download request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoDownloadRequest,
				Provider:    req.Provider,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.VideoDownloadRequest
	unifaiReq.VideoDownloadRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.VideoDownloadResponse, nil
}

func (unifai *UnifAI) VideoRemixRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIVideoRemixRequest) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "video remix request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoRemixRequest,
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for video remix request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoRemixRequest,
			},
		}
	}
	if req.ID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "video_id is required for video remix request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoRemixRequest,
				Provider:    req.Provider,
			},
		}
	}
	if req.Input == nil || req.Input.Prompt == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "prompt is required for video remix request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoRemixRequest,
				Provider:    req.Provider,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.VideoRemixRequest
	unifaiReq.VideoRemixRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	if response == nil || response.VideoGenerationResponse == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "received nil response from provider",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoRemixRequest,
				Provider:    req.Provider,
			},
		}
	}
	return response.VideoGenerationResponse, nil
}

func (unifai *UnifAI) VideoListRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIVideoListRequest) (*schemas.UnifAIVideoListResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "video list request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoListRequest,
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for video list request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoListRequest,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.VideoListRequest
	unifaiReq.VideoListRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.VideoListResponse, nil
}

func (unifai *UnifAI) VideoDeleteRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIVideoDeleteRequest) (*schemas.UnifAIVideoDeleteResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "video delete request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoDeleteRequest,
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for video delete request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoDeleteRequest,
			},
		}
	}
	if req.ID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "video_id is required for video delete request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.VideoDeleteRequest,
				Provider:    req.Provider,
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.VideoDeleteRequest
	unifaiReq.VideoDeleteRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.VideoDeleteResponse, nil
}

// BatchCreateRequest creates a new batch job for asynchronous processing.
func (unifai *UnifAI) BatchCreateRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIBatchCreateRequest) (*schemas.UnifAIBatchCreateResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "batch create request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for batch create request",
			},
		}
	}
	hasInputBlob := req.InputBlob != nil && strings.TrimSpace(*req.InputBlob) != ""
	if req.InputFileID == "" && len(req.Requests) == 0 && !hasInputBlob {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "either input_file_id, input_blob, or requests is required for batch create request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	provider := unifai.getProviderByKey(req.Provider)
	if provider == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider not found for batch create request",
			},
		}
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.BatchCreateRequest
	unifaiReq.BatchCreateRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.BatchCreateResponse, nil
}

// BatchListRequest lists batch jobs for the specified provider.
func (unifai *UnifAI) BatchListRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIBatchListRequest) (*schemas.UnifAIBatchListResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "batch list request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for batch list request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.BatchListRequest
	unifaiReq.BatchListRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.BatchListResponse, nil
}

// BatchRetrieveRequest retrieves a specific batch job.
func (unifai *UnifAI) BatchRetrieveRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIBatchRetrieveRequest) (*schemas.UnifAIBatchRetrieveResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "batch retrieve request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for batch retrieve request",
			},
		}
	}
	if req.BatchID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "batch_id is required for batch retrieve request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.BatchRetrieveRequest
	unifaiReq.BatchRetrieveRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.BatchRetrieveResponse, nil
}

// BatchCancelRequest cancels a batch job.
func (unifai *UnifAI) BatchCancelRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIBatchCancelRequest) (*schemas.UnifAIBatchCancelResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "batch cancel request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for batch cancel request",
			},
		}
	}
	if req.BatchID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "batch_id is required for batch cancel request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.BatchCancelRequest
	unifaiReq.BatchCancelRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.BatchCancelResponse, nil
}

// BatchDeleteRequest deletes a batch job.
func (unifai *UnifAI) BatchDeleteRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIBatchDeleteRequest) (*schemas.UnifAIBatchDeleteResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "batch delete request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for batch delete request",
			},
		}
	}
	if req.BatchID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "batch_id is required for batch delete request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.BatchDeleteRequest
	unifaiReq.BatchDeleteRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.BatchDeleteResponse, nil
}

// BatchResultsRequest retrieves results from a completed batch job.
func (unifai *UnifAI) BatchResultsRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIBatchResultsRequest) (*schemas.UnifAIBatchResultsResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "batch results request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.BatchResultsRequest,
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for batch results request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.BatchResultsRequest,
			},
		}
	}
	if req.BatchID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "batch_id is required for batch results request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.BatchResultsRequest,
				Provider:    req.Provider,
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.BatchResultsRequest
	unifaiReq.BatchResultsRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.BatchResultsResponse, nil
}

// FileUploadRequest uploads a file to the specified provider.
func (unifai *UnifAI) FileUploadRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIFileUploadRequest) (*schemas.UnifAIFileUploadResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "file upload request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.FileUploadRequest,
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for file upload request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.FileUploadRequest,
			},
		}
	}

	if len(req.File) == 0 && req.Provider != schemas.Vertex {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "file content is required for file upload request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.FileUploadRequest,
				Provider:    req.Provider,
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.FileUploadRequest
	unifaiReq.FileUploadRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.FileUploadResponse, nil
}

// FileListRequest lists files from the specified provider.
func (unifai *UnifAI) FileListRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIFileListRequest) (*schemas.UnifAIFileListResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "file list request is nil",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.FileListRequest,
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for file list request",
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType: schemas.FileListRequest,
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.FileListRequest
	unifaiReq.FileListRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.FileListResponse, nil
}

// FileRetrieveRequest retrieves file metadata from the specified provider.
func (unifai *UnifAI) FileRetrieveRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIFileRetrieveRequest) (*schemas.UnifAIFileRetrieveResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "file retrieve request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for file retrieve request",
			},
		}
	}
	if req.FileID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "file_id is required for file retrieve request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.FileRetrieveRequest
	unifaiReq.FileRetrieveRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.FileRetrieveResponse, nil
}

// FileDeleteRequest deletes a file from the specified provider.
func (unifai *UnifAI) FileDeleteRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIFileDeleteRequest) (*schemas.UnifAIFileDeleteResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "file delete request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for file delete request",
			},
		}
	}
	if req.FileID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "file_id is required for file delete request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.FileDeleteRequest
	unifaiReq.FileDeleteRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.FileDeleteResponse, nil
}

// FileContentRequest downloads file content from the specified provider.
func (unifai *UnifAI) FileContentRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIFileContentRequest) (*schemas.UnifAIFileContentResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "file content request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for file content request",
			},
		}
	}
	if req.FileID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "file_id is required for file content request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.FileContentRequest
	unifaiReq.FileContentRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.FileContentResponse, nil
}

// CachedContentCreateRequest creates a new cached content (Gemini / Vertex AI named cache lifecycle).
func (unifai *UnifAI) CachedContentCreateRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAICachedContentCreateRequest) (*schemas.UnifAICachedContentCreateResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{IsUnifAIError: false, Error: &schemas.ErrorField{Message: "cached content create request is nil"}}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{IsUnifAIError: false, Error: &schemas.ErrorField{Message: "provider is required for cached content create request"}}
	}
	if req.Model == "" {
		return nil, &schemas.UnifAIError{IsUnifAIError: false, Error: &schemas.ErrorField{Message: "model is required for cached content create request"}}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}
	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.CachedContentCreateRequest
	unifaiReq.CachedContentCreateRequest = req
	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.CachedContentCreateResponse, nil
}

// CachedContentListRequest lists cached contents.
func (unifai *UnifAI) CachedContentListRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAICachedContentListRequest) (*schemas.UnifAICachedContentListResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{IsUnifAIError: false, Error: &schemas.ErrorField{Message: "cached content list request is nil"}}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{IsUnifAIError: false, Error: &schemas.ErrorField{Message: "provider is required for cached content list request"}}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}
	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.CachedContentListRequest
	unifaiReq.CachedContentListRequest = req
	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.CachedContentListResponse, nil
}

// CachedContentRetrieveRequest retrieves a single cached content by name.
func (unifai *UnifAI) CachedContentRetrieveRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAICachedContentRetrieveRequest) (*schemas.UnifAICachedContentRetrieveResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{IsUnifAIError: false, Error: &schemas.ErrorField{Message: "cached content retrieve request is nil"}}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{IsUnifAIError: false, Error: &schemas.ErrorField{Message: "provider is required for cached content retrieve request"}}
	}
	if req.Name == "" {
		return nil, &schemas.UnifAIError{IsUnifAIError: false, Error: &schemas.ErrorField{Message: "name is required for cached content retrieve request"}}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}
	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.CachedContentRetrieveRequest
	unifaiReq.CachedContentRetrieveRequest = req
	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.CachedContentRetrieveResponse, nil
}

// CachedContentUpdateRequest updates expiration on a cached content.
func (unifai *UnifAI) CachedContentUpdateRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAICachedContentUpdateRequest) (*schemas.UnifAICachedContentUpdateResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{IsUnifAIError: false, Error: &schemas.ErrorField{Message: "cached content update request is nil"}}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{IsUnifAIError: false, Error: &schemas.ErrorField{Message: "provider is required for cached content update request"}}
	}
	if req.Name == "" {
		return nil, &schemas.UnifAIError{IsUnifAIError: false, Error: &schemas.ErrorField{Message: "name is required for cached content update request"}}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}
	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.CachedContentUpdateRequest
	unifaiReq.CachedContentUpdateRequest = req
	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.CachedContentUpdateResponse, nil
}

// CachedContentDeleteRequest deletes a cached content by name.
func (unifai *UnifAI) CachedContentDeleteRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAICachedContentDeleteRequest) (*schemas.UnifAICachedContentDeleteResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{IsUnifAIError: false, Error: &schemas.ErrorField{Message: "cached content delete request is nil"}}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{IsUnifAIError: false, Error: &schemas.ErrorField{Message: "provider is required for cached content delete request"}}
	}
	if req.Name == "" {
		return nil, &schemas.UnifAIError{IsUnifAIError: false, Error: &schemas.ErrorField{Message: "name is required for cached content delete request"}}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}
	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.CachedContentDeleteRequest
	unifaiReq.CachedContentDeleteRequest = req
	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.CachedContentDeleteResponse, nil
}

func (unifai *UnifAI) Passthrough(
	ctx *schemas.UnifAIContext,
	provider schemas.ModelProvider,
	req *schemas.UnifAIPassthroughRequest,
) (*schemas.UnifAIPassthroughResponse, *schemas.UnifAIError) {
	if req == nil {
		sc := fasthttp.StatusBadRequest
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			StatusCode:     &sc,
			Error:          &schemas.ErrorField{Message: "passthrough request is nil"},
		}
	}

	req.Provider = provider

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.PassthroughRequest
	unifaiReq.PassthroughRequest = req

	resp, unifaiErr := unifai.handleRequest(ctx, unifaiReq)
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	if resp == nil || resp.PassthroughResponse == nil {
		sc := fasthttp.StatusBadGateway
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			StatusCode:     &sc,
			Error:          &schemas.ErrorField{Message: "provider returned nil passthrough response"},
		}
	}
	return resp.PassthroughResponse, nil
}

func (unifai *UnifAI) PassthroughStream(
	ctx *schemas.UnifAIContext,
	provider schemas.ModelProvider,
	req *schemas.UnifAIPassthroughRequest,
) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	if req == nil {
		sc := fasthttp.StatusBadRequest
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			StatusCode:     &sc,
			Error:          &schemas.ErrorField{Message: "passthrough request is nil"},
		}
	}

	req.Provider = provider

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.PassthroughStreamRequest
	unifaiReq.PassthroughRequest = req

	return unifai.handleStreamRequest(ctx, unifaiReq)
}

// ensureMCPRawStorageContext sets UnifAIContextKeyShouldStoreRawInLogs for standalone MCP
// tool executions so PostMCPHook consumers (e.g. the logging plugin) see an explicit value.
// In-pipeline tool calls already carry the key from the LLM request path (see the effective
// raw-storage computation in requestWorker), so an existing value is never overwritten. There is
// no provider config on the standalone path, so the default is false and only the per-request
// override is honored, mirroring the per-request half of the LLM pipeline's logic.
//
// Callers must only pass request-scoped contexts, never the shared instance context
// (unifai.ctx): SetValue mutates the receiver's value map, so writing here would stamp the
// flag onto state shared by unrelated calls. Nil-ctx standalone executions therefore skip this
// entirely — consumers treat a missing key as false, and the per-request override keys can
// never be present on the instance context, so the outcome is identical.
func ensureMCPRawStorageContext(ctx *schemas.UnifAIContext) {
	if ctx == nil {
		return
	}
	if _, ok := ctx.Value(schemas.UnifAIContextKeyShouldStoreRawInLogs).(bool); ok {
		return
	}
	effectiveStore := false
	if allowStorageOverride, _ := ctx.Value(schemas.UnifAIContextKeyAllowPerRequestStorageOverride).(bool); allowStorageOverride {
		if override, ok := ctx.Value(schemas.UnifAIContextKeyStoreRawRequestResponse).(bool); ok {
			effectiveStore = override
		}
	}
	ctx.SetValue(schemas.UnifAIContextKeyShouldStoreRawInLogs, effectiveStore)
}

// ExecuteChatMCPTool executes an MCP tool call and returns the result as a chat message.
// This is the main public API for manual MCP tool execution in Chat format. All the
// real work — request pooling, plugin gate (PreMCPHook / PostMCPHook), short-circuit
// handling, error enrichment — lives on MCPManager.ExecuteChatTool.
func (unifai *UnifAI) ExecuteChatMCPTool(ctx *schemas.UnifAIContext, toolCall *schemas.ChatAssistantMessageToolCall) (*schemas.ChatMessage, *schemas.UnifAIError) {
	if ctx == nil {
		ctx = unifai.ctx
	} else {
		ensureMCPRawStorageContext(ctx)
	}
	if unifai.MCPManager == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error:          &schemas.ErrorField{Message: "mcp is not configured in this unifai instance"},
			ExtraFields:    schemas.UnifAIErrorExtraFields{RequestType: schemas.ChatCompletionRequest},
		}
	}
	return unifai.MCPManager.ExecuteChatTool(ctx, toolCall)
}

// ExecuteResponsesMCPTool executes an MCP tool call and returns the result as a responses
// message. Thin delegator — see ExecuteChatMCPTool for the rationale.
func (unifai *UnifAI) ExecuteResponsesMCPTool(ctx *schemas.UnifAIContext, toolCall *schemas.ResponsesToolMessage) (*schemas.ResponsesMessage, *schemas.UnifAIError) {
	if ctx == nil {
		ctx = unifai.ctx
	} else {
		ensureMCPRawStorageContext(ctx)
	}
	if unifai.MCPManager == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error:          &schemas.ErrorField{Message: "mcp is not configured in this unifai instance"},
			ExtraFields:    schemas.UnifAIErrorExtraFields{RequestType: schemas.ResponsesRequest},
		}
	}
	return unifai.MCPManager.ExecuteResponsesTool(ctx, toolCall)
}

// ContainerCreateRequest creates a new container.
func (unifai *UnifAI) ContainerCreateRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIContainerCreateRequest) (*schemas.UnifAIContainerCreateResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container create request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for container create request",
			},
		}
	}
	if req.Name == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "name is required for container create request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ContainerCreateRequest
	unifaiReq.ContainerCreateRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.ContainerCreateResponse, nil
}

// ContainerListRequest lists containers.
func (unifai *UnifAI) ContainerListRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIContainerListRequest) (*schemas.UnifAIContainerListResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container list request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for container list request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ContainerListRequest
	unifaiReq.ContainerListRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.ContainerListResponse, nil
}

// ContainerRetrieveRequest retrieves a specific container.
func (unifai *UnifAI) ContainerRetrieveRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIContainerRetrieveRequest) (*schemas.UnifAIContainerRetrieveResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container retrieve request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for container retrieve request",
			},
		}
	}
	if req.ContainerID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container_id is required for container retrieve request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ContainerRetrieveRequest
	unifaiReq.ContainerRetrieveRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.ContainerRetrieveResponse, nil
}

// ContainerDeleteRequest deletes a container.
func (unifai *UnifAI) ContainerDeleteRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIContainerDeleteRequest) (*schemas.UnifAIContainerDeleteResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container delete request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for container delete request",
			},
		}
	}
	if req.ContainerID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container_id is required for container delete request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ContainerDeleteRequest
	unifaiReq.ContainerDeleteRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.ContainerDeleteResponse, nil
}

// ContainerFileCreateRequest creates a file in a container.
func (unifai *UnifAI) ContainerFileCreateRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIContainerFileCreateRequest) (*schemas.UnifAIContainerFileCreateResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container file create request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for container file create request",
			},
		}
	}
	if req.ContainerID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container_id is required for container file create request",
			},
		}
	}
	if len(req.File) == 0 && (req.FileID == nil || strings.TrimSpace(*req.FileID) == "") && (req.Path == nil || strings.TrimSpace(*req.Path) == "") {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "one of file, file_id, or path is required for container file create request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ContainerFileCreateRequest
	unifaiReq.ContainerFileCreateRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.ContainerFileCreateResponse, nil
}

// ContainerFileListRequest lists files in a container.
func (unifai *UnifAI) ContainerFileListRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIContainerFileListRequest) (*schemas.UnifAIContainerFileListResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container file list request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for container file list request",
			},
		}
	}
	if req.ContainerID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container_id is required for container file list request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ContainerFileListRequest
	unifaiReq.ContainerFileListRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.ContainerFileListResponse, nil
}

// ContainerFileRetrieveRequest retrieves a file from a container.
func (unifai *UnifAI) ContainerFileRetrieveRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIContainerFileRetrieveRequest) (*schemas.UnifAIContainerFileRetrieveResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container file retrieve request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for container file retrieve request",
			},
		}
	}
	if req.ContainerID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container_id is required for container file retrieve request",
			},
		}
	}
	if req.FileID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "file_id is required for container file retrieve request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ContainerFileRetrieveRequest
	unifaiReq.ContainerFileRetrieveRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.ContainerFileRetrieveResponse, nil
}

// ContainerFileContentRequest retrieves the content of a file from a container.
func (unifai *UnifAI) ContainerFileContentRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIContainerFileContentRequest) (*schemas.UnifAIContainerFileContentResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container file content request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for container file content request",
			},
		}
	}
	if req.ContainerID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container_id is required for container file content request",
			},
		}
	}
	if req.FileID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "file_id is required for container file content request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ContainerFileContentRequest
	unifaiReq.ContainerFileContentRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.ContainerFileContentResponse, nil
}

// ContainerFileDeleteRequest deletes a file from a container.
func (unifai *UnifAI) ContainerFileDeleteRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIContainerFileDeleteRequest) (*schemas.UnifAIContainerFileDeleteResponse, *schemas.UnifAIError) {
	if req == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container file delete request is nil",
			},
		}
	}
	if req.Provider == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "provider is required for container file delete request",
			},
		}
	}
	if req.ContainerID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "container_id is required for container file delete request",
			},
		}
	}
	if req.FileID == "" {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: "file_id is required for container file delete request",
			},
		}
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	unifaiReq := unifai.getUnifAIRequest()
	unifaiReq.RequestType = schemas.ContainerFileDeleteRequest
	unifaiReq.ContainerFileDeleteRequest = req

	response, err := unifai.handleRequest(ctx, unifaiReq)
	if err != nil {
		return nil, err
	}
	return response.ContainerFileDeleteResponse, nil
}

// RemovePlugin removes a plugin from the server.
func (unifai *UnifAI) RemovePlugin(name string, pluginTypes []schemas.PluginType) error {
	for _, pluginType := range pluginTypes {
		switch pluginType {
		case schemas.PluginTypeLLM:
			err := unifai.removeLLMPlugin(name)
			if err != nil {
				return err
			}
		case schemas.PluginTypeMCP:
			err := unifai.removeMCPPlugin(name)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// removeLLMPlugin removes an LLM plugin from the server.
func (unifai *UnifAI) removeLLMPlugin(name string) error {
	for {
		oldPlugins := unifai.llmPlugins.Load()
		if oldPlugins == nil {
			return nil
		}
		var pluginToCleanup schemas.LLMPlugin
		found := false
		// Create new slice without the plugin to remove
		newPlugins := make([]schemas.LLMPlugin, 0, len(*oldPlugins))
		for _, p := range *oldPlugins {
			if p.GetName() == name {
				pluginToCleanup = p
				unifai.logger.Debug("removing LLM plugin %s", name)
				found = true
			} else {
				newPlugins = append(newPlugins, p)
			}
		}
		if !found {
			return nil
		}
		// Atomic compare-and-swap
		if unifai.llmPlugins.CompareAndSwap(oldPlugins, &newPlugins) {
			// Cleanup the old plugin
			err := pluginToCleanup.Cleanup()
			if err != nil {
				unifai.logger.Warn("failed to cleanup old LLM plugin %s: %v", pluginToCleanup.GetName(), err)
			}
			return nil
		}
		// Retrying as swapping did not work
	}
}

// removeMCPPlugin removes an MCP plugin from the server.
func (unifai *UnifAI) removeMCPPlugin(name string) error {
	for {
		oldPlugins := unifai.mcpPlugins.Load()
		if oldPlugins == nil {
			return nil
		}
		var pluginToCleanup schemas.MCPPlugin
		found := false
		// Create new slice without the plugin to remove
		newPlugins := make([]schemas.MCPPlugin, 0, len(*oldPlugins))
		for _, p := range *oldPlugins {
			if p.GetName() == name {
				pluginToCleanup = p
				unifai.logger.Debug("removing MCP plugin %s", name)
				found = true
			} else {
				newPlugins = append(newPlugins, p)
			}
		}
		if !found {
			return nil
		}
		// Atomic compare-and-swap
		if unifai.mcpPlugins.CompareAndSwap(oldPlugins, &newPlugins) {
			// Cleanup the old plugin
			err := pluginToCleanup.Cleanup()
			if err != nil {
				unifai.logger.Warn("failed to cleanup old MCP plugin %s: %v", pluginToCleanup.GetName(), err)
			}
			return nil
		}
		// Retrying as swapping did not work
	}
}

// ReloadPlugin reloads a plugin with new instance
// During the reload - it's stop the world phase where we take a global lock on the plugin mutex
func (unifai *UnifAI) ReloadPlugin(plugin schemas.BasePlugin, pluginTypes []schemas.PluginType) error {
	for _, pluginType := range pluginTypes {
		switch pluginType {
		case schemas.PluginTypeLLM:
			llmPlugin, ok := plugin.(schemas.LLMPlugin)
			if !ok {
				return fmt.Errorf("plugin %s is not an LLMPlugin", plugin.GetName())
			}
			err := unifai.reloadLLMPlugin(llmPlugin)
			if err != nil {
				return err
			}
		case schemas.PluginTypeMCP:
			mcpPlugin, ok := plugin.(schemas.MCPPlugin)
			if !ok {
				return fmt.Errorf("plugin %s is not an MCPPlugin", plugin.GetName())
			}
			err := unifai.reloadMCPPlugin(mcpPlugin)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// reloadLLMPlugin reloads an LLM plugin with new instance
func (unifai *UnifAI) reloadLLMPlugin(plugin schemas.LLMPlugin) error {
	for {
		var pluginToCleanup schemas.LLMPlugin
		found := false
		oldPlugins := unifai.llmPlugins.Load()

		// Create new slice with replaced plugin or initialize empty slice
		var newPlugins []schemas.LLMPlugin
		if oldPlugins == nil {
			// Initialize new empty slice for the first plugin
			newPlugins = make([]schemas.LLMPlugin, 0)
		} else {
			newPlugins = make([]schemas.LLMPlugin, len(*oldPlugins))
			copy(newPlugins, *oldPlugins)
		}

		for i, p := range newPlugins {
			if p.GetName() == plugin.GetName() {
				// Cleaning up old plugin before replacing it
				pluginToCleanup = p
				unifai.logger.Debug("replacing LLM plugin %s with new instance", plugin.GetName())
				newPlugins[i] = plugin
				found = true
				break
			}
		}
		if !found {
			// This means that user is adding a new plugin
			unifai.logger.Debug("adding new LLM plugin %s", plugin.GetName())
			newPlugins = append(newPlugins, plugin)
		}
		// Atomic compare-and-swap
		if unifai.llmPlugins.CompareAndSwap(oldPlugins, &newPlugins) {
			// Cleanup the old plugin
			if found && pluginToCleanup != nil {
				err := pluginToCleanup.Cleanup()
				if err != nil {
					unifai.logger.Warn("failed to cleanup old LLM plugin %s: %v", pluginToCleanup.GetName(), err)
				}
			}
			return nil
		}
		// Retrying as swapping did not work
	}
}

// reloadMCPPlugin reloads an MCP plugin with new instance
func (unifai *UnifAI) reloadMCPPlugin(plugin schemas.MCPPlugin) error {
	for {
		var pluginToCleanup schemas.MCPPlugin
		found := false
		oldPlugins := unifai.mcpPlugins.Load()
		if oldPlugins == nil {
			return nil
		}
		// Create new slice with replaced plugin
		newPlugins := make([]schemas.MCPPlugin, len(*oldPlugins))
		copy(newPlugins, *oldPlugins)
		for i, p := range newPlugins {
			if p.GetName() == plugin.GetName() {
				// Cleaning up old plugin before replacing it
				pluginToCleanup = p
				unifai.logger.Debug("replacing MCP plugin %s with new instance", plugin.GetName())
				newPlugins[i] = plugin
				found = true
				break
			}
		}
		if !found {
			// This means that user is adding a new plugin
			unifai.logger.Debug("adding new MCP plugin %s", plugin.GetName())
			newPlugins = append(newPlugins, plugin)
		}
		// Atomic compare-and-swap
		if unifai.mcpPlugins.CompareAndSwap(oldPlugins, &newPlugins) {
			// Cleanup the old plugin
			if found && pluginToCleanup != nil {
				err := pluginToCleanup.Cleanup()
				if err != nil {
					unifai.logger.Warn("failed to cleanup old MCP plugin %s: %v", pluginToCleanup.GetName(), err)
				}
			}
			return nil
		}
		// Retrying as swapping did not work
	}
}

// ReorderPlugins reorders all plugin slices (LLM, MCP) to match the given
// base plugin name ordering. This should be called after SortAndRebuildPlugins
// on the config layer to sync the core's execution order.
// Plugins not in the ordering are appended at the end (defensive).
func (unifai *UnifAI) ReorderPlugins(orderedNames []string) {
	pos := make(map[string]int, len(orderedNames))
	for i, name := range orderedNames {
		pos[name] = i
	}
	reorderAtomicSlice(&unifai.llmPlugins, pos)
	reorderAtomicSlice(&unifai.mcpPlugins, pos)
}

// pluginWithName is satisfied by both LLMPlugin and MCPPlugin.
type pluginWithName interface {
	GetName() string
}

// reorderAtomicSlice atomically reorders the plugin slice stored behind ptr
// so that plugins appear in the order given by pos (name → position).
// Uses CAS retry for lock-free safety.
func reorderAtomicSlice[T pluginWithName](ptr *atomic.Pointer[[]T], pos map[string]int) {
	for {
		old := ptr.Load()
		if old == nil || len(*old) == 0 {
			return
		}
		reordered := make([]T, len(*old))
		copy(reordered, *old)
		sort.SliceStable(reordered, func(i, j int) bool {
			iPos, iOk := pos[reordered[i].GetName()]
			jPos, jOk := pos[reordered[j].GetName()]
			if !iOk && !jOk {
				return false
			}
			if !iOk {
				return false
			}
			if !jOk {
				return true
			}
			return iPos < jPos
		})
		if ptr.CompareAndSwap(old, &reordered) {
			return
		}
	}
}

// GetConfiguredProviders returns the configured providers.
//
// Returns:
//   - []schemas.ModelProvider: List of configured providers
//   - error: Any error that occurred during the retrieval process
//
// Example:
//
//	providers, err := unifai.GetConfiguredProviders()
//	if err != nil {
//		return nil, err
//	}
//	fmt.Println(providers)
func (unifai *UnifAI) GetConfiguredProviders() ([]schemas.ModelProvider, error) {
	providers := unifai.providers.Load()
	if providers == nil {
		return nil, fmt.Errorf("no providers configured")
	}
	modelProviders := make([]schemas.ModelProvider, len(*providers))
	for i, provider := range *providers {
		modelProviders[i] = provider.GetProviderKey()
	}
	return modelProviders, nil
}

// RemoveProvider removes a provider from the server.
// This method gracefully stops all workers for the provider,
// closes the request queue, and removes the provider from the providers slice.
//
// Parameters:
//   - providerKey: The provider to remove
//
// Returns:
//   - error: Any error that occurred during the removal process
func (unifai *UnifAI) RemoveProvider(providerKey schemas.ModelProvider) error {
	unifai.logger.Info("Removing provider %s", providerKey)
	providerMutex := unifai.getProviderMutex(providerKey)
	providerMutex.Lock()
	defer providerMutex.Unlock()

	// Step 1: Load the ProviderQueue and verify provider exists
	pqValue, exists := unifai.requestQueues.Load(providerKey)
	if !exists {
		return fmt.Errorf("provider %s not found in request queues", providerKey)
	}
	pq := pqValue.(*ProviderQueue)

	// Step 2: Signal closing. Blocks new producers (isClosing() returns true) and
	// causes idle workers to drain remaining buffered requests with errors then exit.
	pq.signalClosing()
	unifai.logger.Debug("signaled closing for provider %s", providerKey)

	// Step 3: Wait for all workers to finish in-flight requests and exit.
	waitGroup, exists := unifai.waitGroups.Load(providerKey)
	if exists {
		waitGroup.(*sync.WaitGroup).Wait()
		unifai.logger.Debug("all workers for provider %s have stopped", providerKey)
	}

	// Step 3b: Final drain sweep — see drainQueueWithErrors for full explanation.
	unifai.drainQueueWithErrors(pq)

	// Step 3c: Wait for retired worker generations from earlier provider updates.
	// They are no longer tracked in unifai.waitGroups after a new generation is
	// published, but removing the provider must still wait for their in-flight work.
	if retiredWaitValue, exists := unifai.retiredWorkerWaits.Load(providerKey); exists {
		retiredWaitValue.(*sync.WaitGroup).Wait()
		unifai.retiredWorkerWaits.Delete(providerKey)
		unifai.logger.Debug("all retired workers for provider %s have stopped", providerKey)
	}

	// Step 4: Remove the provider from the request queues.
	unifai.requestQueues.Delete(providerKey)

	// Step 5: Remove the provider from the wait groups.
	unifai.waitGroups.Delete(providerKey)

	// Step 6: Remove the provider from the providers slice.
	if err := unifai.removeProviderFromSlice(providerKey); err != nil {
		unifai.logger.Error(
			"provider %s was removed from queues but could not be removed from the providers slice — "+
				"unifai.providers is now inconsistent. "+
				"To recover: retry RemoveProvider(%s), or restart UnifAI if that fails.",
			providerKey, providerKey,
		)
		return err
	}

	unifai.logger.Info("successfully removed provider %s", providerKey)
	schemas.UnregisterKnownProvider(providerKey)
	return nil
}

// UpdateProvider dynamically updates a provider with new configuration.
// This method gracefully recreates the provider instance with updated settings,
// stops existing workers, creates a new queue with updated settings,
// and starts new workers with the updated provider and concurrency configuration.
//
// Parameters:
//   - providerKey: The provider to update
//
// Returns:
//   - error: Any error that occurred during the update process
//
// Note: This operation will temporarily pause request processing for the specified provider
// while the transition occurs. In-flight requests will complete before workers are stopped.
// Buffered requests in the old queue will be transferred to the new queue to prevent loss.
//
// Concurrency safety — update handoff:
// UpdateProvider holds a per-provider write lock only while publishing the new
// provider instance, queue, and workers. It starts the new workers before the
// old queue is signalled closed, then releases the lock before old workers are
// waited on. This avoids high-load updates blocking new requests behind the
// provider read lock while slow in-flight old-worker requests finish.
func (unifai *UnifAI) UpdateProvider(providerKey schemas.ModelProvider) error {
	unifai.providerLifecycleMu.RLock()
	defer unifai.providerLifecycleMu.RUnlock()
	if unifai.ctx.Err() != nil {
		return fmt.Errorf("unifai is shutting down")
	}

	unifai.logger.Info(fmt.Sprintf("Updating provider configuration for provider %s", providerKey))
	// Get the updated configuration from the account
	providerConfig, err := unifai.account.GetConfigForProvider(providerKey)
	if err != nil {
		return fmt.Errorf("failed to get updated config for provider %s: %v", providerKey, err)
	}
	if providerConfig == nil {
		return fmt.Errorf("config is nil for provider %s", providerKey)
	}
	// Lock the provider while publishing the new runtime state. The slow cleanup
	// of old workers happens after unlock so new requests can route to newPq.
	providerMutex := unifai.getProviderMutex(providerKey)
	providerMutex.Lock()
	defer providerMutex.Unlock()

	// Check if provider currently exists
	oldPqValue, exists := unifai.requestQueues.Load(providerKey)
	if !exists {
		unifai.logger.Debug("provider %s not currently active, initializing with new configuration", providerKey)
		// If provider doesn't exist, just prepare it with new configuration
		return unifai.prepareProvider(providerKey, providerConfig)
	}

	oldPq := oldPqValue.(*ProviderQueue)
	var oldWaitGroup *sync.WaitGroup
	if waitGroupValue, exists := unifai.waitGroups.Load(providerKey); exists {
		oldWaitGroup = waitGroupValue.(*sync.WaitGroup)
	}

	unifai.logger.Debug("gracefully replacing existing workers for provider %s", providerKey)

	// Step 1: Create provider instance before touching live routing state. If
	// provider construction fails, the old provider/queue continues serving.
	provider, err := unifai.createBaseProvider(providerKey, providerConfig)
	if err != nil {
		return fmt.Errorf("provider update for %s failed during initialization; old provider is still active: %v", providerKey, err)
	}

	// Step 2: Create new ProviderQueue and wait group with updated settings.
	newPq := &ProviderQueue{
		queue:      make(chan *ChannelMessage, providerConfig.ConcurrencyAndBufferSize.BufferSize),
		done:       make(chan struct{}),
		signalOnce: sync.Once{},
	}
	newWaitGroup := &sync.WaitGroup{}

	// Step 3: Atomically replace the provider in the providers slice before new
	// workers start so all fresh worker executions use the updated provider.
	unifai.logger.Debug("atomically replacing provider instance in providers slice for %s", providerKey)

	replacementAttempts := 0
	maxReplacementAttempts := 100 // Prevent infinite loops in high-contention scenarios

	for {
		replacementAttempts++
		if replacementAttempts > maxReplacementAttempts {
			return fmt.Errorf("failed to replace provider %s in providers slice after %d attempts; old provider is still active", providerKey, maxReplacementAttempts)
		}

		oldPtr := unifai.providers.Load()
		var oldSlice []schemas.Provider
		if oldPtr != nil {
			oldSlice = *oldPtr
		}

		newSlice := make([]schemas.Provider, 0, len(oldSlice))
		oldProviderFound := false

		for _, existingProvider := range oldSlice {
			if existingProvider.GetProviderKey() != providerKey {
				newSlice = append(newSlice, existingProvider)
			} else {
				oldProviderFound = true
			}
		}

		newSlice = append(newSlice, provider)

		if unifai.providers.CompareAndSwap(oldPtr, &newSlice) {
			if oldProviderFound {
				unifai.logger.Debug("successfully replaced existing provider instance for %s in providers slice", providerKey)
			} else {
				unifai.logger.Debug("successfully added new provider instance for %s to providers slice", providerKey)
			}
			break
		}
		// Retrying as swapping did not work (likely due to concurrent modification)
	}

	// Step 4: Publish the new queue/wait group and start new workers before old
	// workers are stopped. This avoids the update-induced no-worker window while
	// still preventing new producers from seeing partial state until unlock.
	unifai.requestQueues.Store(providerKey, newPq)
	unifai.waitGroups.Store(providerKey, newWaitGroup)
	unifai.logger.Debug("stored new queue for provider %s, new producers will use it", providerKey)

	unifai.logger.Debug("starting %d new workers for provider %s with buffer size %d",
		providerConfig.ConcurrencyAndBufferSize.Concurrency,
		providerKey,
		providerConfig.ConcurrencyAndBufferSize.BufferSize)
	for range providerConfig.ConcurrencyAndBufferSize.Concurrency {
		newWaitGroup.Add(1)
		go unifai.requestWorker(provider, providerConfig, newPq, newWaitGroup)
	}

	// Step 5: Transfer buffered requests from the old queue to the new queue BEFORE
	// signalling workers to stop. Old workers are still running and may consume some
	// items concurrently — that is fine, they process them normally. Since new
	// workers are already running, successful transfers can be processed immediately.
	transferredCount := 0
	cancelledCount := 0
	for {
		select {
		case msg := <-oldPq.queue:
			select {
			case newPq.queue <- msg:
				transferredCount++
			default:
				// newPq is full — cancel this message and all remaining in oldPq.
				cancelMsg := func(r *ChannelMessage) {
					prov, mod, _ := r.UnifAIRequest.GetRequestFields()
					select {
					case r.Err <- schemas.UnifAIError{
						IsUnifAIError: false,
						Error:          &schemas.ErrorField{Message: "request failed during provider concurrency update: queue full"},
						ExtraFields: schemas.UnifAIErrorExtraFields{
							RequestType:            r.RequestType,
							Provider:               prov,
							OriginalModelRequested: mod,
						},
					}:
					case <-r.Context.Done():
					}
				}
				cancelMsg(msg)
				cancelledCount++
				for {
					select {
					case r := <-oldPq.queue:
						cancelMsg(r)
						cancelledCount++
					default:
						goto transferComplete
					}
				}
			}
		default:
			// No more buffered messages
			goto transferComplete
		}
	}

transferComplete:
	if transferredCount > 0 {
		unifai.logger.Info("transferred %d buffered requests to new queue for provider %s", transferredCount, providerKey)
	}
	if cancelledCount > 0 {
		unifai.logger.Warn("cancelled %d buffered requests during transfer for provider %s: new queue was full", cancelledCount, providerKey)
	}

	// Step 6: Register cleanup before signalling the old queue. Otherwise
	// Shutdown/RemoveProvider can observe no pending old-worker cleanup after the
	// old wait group is replaced, then return while old in-flight requests run.
	var retiredWaitGroup *sync.WaitGroup
	if oldWaitGroup != nil {
		retiredWaitGroup = unifai.getRetiredWorkerWaitGroup(providerKey)
		retiredWaitGroup.Add(1)
		unifai.oldWorkerCleanups.Add(1)
	}

	// Step 7: Signal the old queue is closing. Stale producers that still hold a
	// reference to oldPq will detect this via isClosing() and re-route to newPq.
	oldPq.signalClosing()
	unifai.logger.Debug("signaled closing for old queue of provider %s", providerKey)

	// Step 8: Cleanup old workers asynchronously. Waiting here under the provider
	// lock caused high-load provider updates to block new requests until every slow
	// in-flight old-worker request completed.
	if oldWaitGroup != nil {
		go func(pq *ProviderQueue, wg *sync.WaitGroup, cleanupWg *sync.WaitGroup, key schemas.ModelProvider) {
			defer unifai.oldWorkerCleanups.Done()
			defer cleanupWg.Done()
			wg.Wait()
			unifai.logger.Debug("all old workers for provider %s have stopped", key)
			unifai.drainQueueWithErrors(pq)
		}(oldPq, oldWaitGroup, retiredWaitGroup, providerKey)
	} else {
		unifai.drainQueueWithErrors(oldPq)
	}

	unifai.logger.Info("successfully updated provider configuration for provider %s", providerKey)
	return nil
}

// GetDropExcessRequests returns the current value of DropExcessRequests
func (unifai *UnifAI) GetDropExcessRequests() bool {
	return unifai.dropExcessRequests.Load()
}

// UpdateDropExcessRequests updates the DropExcessRequests setting at runtime.
// This allows for hot-reloading of this configuration value.
func (unifai *UnifAI) UpdateDropExcessRequests(value bool) {
	unifai.dropExcessRequests.Store(value)
	unifai.logger.Info("drop_excess_requests updated to: %v", value)
}

// getProviderMutex gets or creates a mutex for the given provider
func (unifai *UnifAI) getProviderMutex(providerKey schemas.ModelProvider) *sync.RWMutex {
	mutexValue, _ := unifai.providerMutexes.LoadOrStore(providerKey, &sync.RWMutex{})
	return mutexValue.(*sync.RWMutex)
}

// getRetiredWorkerWaitGroup gets or creates a wait group for retired worker
// generations for the given provider.
func (unifai *UnifAI) getRetiredWorkerWaitGroup(providerKey schemas.ModelProvider) *sync.WaitGroup {
	waitGroupValue, _ := unifai.retiredWorkerWaits.LoadOrStore(providerKey, &sync.WaitGroup{})
	return waitGroupValue.(*sync.WaitGroup)
}

// removeProviderFromSlice atomically removes the provider with the given key
// from unifai.providers using a CAS retry loop. Callers hold the per-provider
// write mutex so no concurrent goroutine can re-add this key — contention is
// only from other providers' CAS operations, so the loop converges in at most
// a few iterations under any concurrency level.
// Returns an error if the limit is hit (state will be inconsistent).
func (unifai *UnifAI) removeProviderFromSlice(providerKey schemas.ModelProvider) error {
	const maxAttempts = 100
	for range maxAttempts {
		oldPtr := unifai.providers.Load()
		if oldPtr == nil {
			return nil
		}
		oldSlice := *oldPtr
		newSlice := make([]schemas.Provider, 0, len(oldSlice))
		for _, p := range oldSlice {
			if p.GetProviderKey() != providerKey {
				newSlice = append(newSlice, p)
			}
		}
		if unifai.providers.CompareAndSwap(oldPtr, &newSlice) {
			return nil
		}
	}
	return fmt.Errorf("failed to remove provider %s from providers slice after %d attempts", providerKey, maxAttempts)
}

// MCP PUBLIC API

// RegisterMCPTool registers a typed tool handler with the MCP integration.
// This allows developers to easily add custom tools that will be available
// to all LLM requests processed by this UnifAI instance.
//
// Parameters:
//   - name: Unique tool name
//   - description: Human-readable tool description
//   - handler: Function that handles tool execution
//   - toolSchema: UnifAI tool schema for function calling
//
// Returns:
//   - error: Any registration error
//
// Example:
//
//	type EchoArgs struct {
//	    Message string `json:"message"`
//	}
//
//	err := unifai.RegisterMCPTool("echo", "Echo a message",
//	    func(args EchoArgs) (string, error) {
//	        return args.Message, nil
//	    }, toolSchema)
func (unifai *UnifAI) RegisterMCPTool(name, description string, handler func(args any) (string, error), toolSchema schemas.ChatTool) error {
	if unifai.MCPManager == nil {
		return fmt.Errorf("mcp is not configured in this unifai instance")
	}

	return unifai.MCPManager.RegisterTool(name, description, handler, toolSchema)
}

// IMPORTANT: Running the MCP client management operations (GetMCPClients, AddMCPClient, RemoveMCPClient, EditMCPClientTools)
// may temporarily increase latency for incoming requests while the operations are being processed.
// These operations involve network I/O and connection management that require mutex locks
// which can block briefly during execution.

// GetMCPClients returns all MCP clients managed by the UnifAI instance.
//
// Returns:
//   - []schemas.MCPClient: List of all MCP clients
//   - error: Any retrieval error
func (unifai *UnifAI) GetMCPClients() ([]schemas.MCPClient, error) {
	if unifai.MCPManager == nil {
		return nil, fmt.Errorf("mcp is not configured in this unifai instance")
	}

	clients := unifai.MCPManager.GetClients()
	clientsInConfig := make([]schemas.MCPClient, 0, len(clients))

	for _, client := range clients {
		tools := make([]schemas.ChatToolFunction, 0, len(client.ToolMap))
		for _, tool := range client.ToolMap {
			if tool.Function != nil {
				// Create a deep copy (for name) of the tool function to avoid modifying the original
				toolFunction := schemas.ChatToolFunction{}
				toolFunction.Name = tool.Function.Name
				toolFunction.Description = tool.Function.Description
				toolFunction.Parameters = tool.Function.Parameters
				toolFunction.Strict = tool.Function.Strict
				// Remove the client prefix from the tool name
				toolFunction.Name = strings.TrimPrefix(toolFunction.Name, client.ExecutionConfig.Name+"-")
				tools = append(tools, toolFunction)
			}
		}

		sort.Slice(tools, func(i, j int) bool {
			return tools[i].Name < tools[j].Name
		})

		clientsInConfig = append(clientsInConfig, schemas.MCPClient{
			Config: client.ExecutionConfig,
			Tools:  tools,
			State:  client.State,
		})
	}

	return clientsInConfig, nil
}

// GetAvailableTools returns the available tools for the given context.
//
// Returns:
//   - []schemas.ChatTool: List of available tools
func (unifai *UnifAI) GetAvailableMCPTools(ctx *schemas.UnifAIContext) []schemas.ChatTool {
	if unifai.MCPManager == nil {
		return nil
	}
	return unifai.MCPManager.GetAvailableTools(ctx)
}

// AddMCPClient adds a new MCP client to the UnifAI instance.
// This allows for dynamic MCP client management at runtime.
//
// Parameters:
//   - config: MCP client configuration
//
// Returns:
//   - error: Any registration error
//
// Example:
//
//	err := unifai.AddMCPClient(ctx, &schemas.MCPClientConfig{
//	    Name: "my-mcp-client",
//	    ConnectionType: schemas.MCPConnectionTypeHTTP,
//	    ConnectionString: &url,
//	})
//
// ConnectConfiguredMCPClients dials the MCP clients supplied to Init. Construction
// no longer auto-connects, so callers invoke this after all plugins are registered
// (so every PreMCPConnectionHook participates in the connection). No-op if MCP is
// not configured.
func (unifai *UnifAI) ConnectConfiguredMCPClients(ctx context.Context) {
	if unifai.MCPManager != nil {
		unifai.MCPManager.ConnectConfiguredClients(ctx)
	}
}

func (unifai *UnifAI) AddMCPClient(ctx context.Context, config *schemas.MCPClientConfig) error {
	if unifai.MCPManager == nil {
		// Use sync.Once to ensure thread-safe initialization
		unifai.mcpInitOnce.Do(func() {
			// Initialize with empty config - client will be added via AddClient below
			mcpConfig := schemas.MCPConfig{
				ClientConfigs: []*schemas.MCPClientConfig{},
			}
			// Set up plugin pipeline provider functions for executeCode tool hooks
			mcpConfig.PluginPipelineProvider = func() interface{} {
				return unifai.getPluginPipeline()
			}
			mcpConfig.ReleasePluginPipeline = func(pipeline interface{}) {
				if pp, ok := pipeline.(*PluginPipeline); ok {
					unifai.releasePluginPipeline(pp)
				}
			}
			// Create Starlark CodeMode for code execution (with default config)
			codeMode := starlark.NewStarlarkCodeMode(nil, unifai.logger)
			unifai.MCPManager = mcp.NewMCPManager(unifai.ctx, mcpConfig, unifai.mcpCredStore, unifai.logger, codeMode)
		})
	}

	// Handle case where initialization succeeded elsewhere but manager is still nil
	if unifai.MCPManager == nil {
		return fmt.Errorf("MCP manager is not initialized")
	}

	return unifai.MCPManager.AddClient(ctx, config)
}

// RemoveMCPClient removes an MCP client from the UnifAI instance.
// This allows for dynamic MCP client management at runtime.
//
// Parameters:
//   - id: ID of the client to remove
//
// Returns:
//   - error: Any removal error
//
// Example:
//
//	err := unifai.RemoveMCPClient("my-mcp-client-id")
//	if err != nil {
//	    log.Fatalf("Failed to remove MCP client: %v", err)
//	}
func (unifai *UnifAI) RemoveMCPClient(id string) error {
	if unifai.MCPManager == nil {
		return fmt.Errorf("mcp is not configured in this unifai instance")
	}

	return unifai.MCPManager.RemoveClient(id)
}

// SetMCPManager sets the MCP manager for this UnifAI instance.
// This allows injecting a custom MCP manager implementation.
// If the provided manager is a concrete *mcp.MCPManager, UnifAI's plugin pipeline is injected
// into the manager's CodeMode so that nested tool calls run through the plugin hooks.
//
// Parameters:
//   - manager: The MCP manager to set (must implement MCPManagerInterface)
func (unifai *UnifAI) SetMCPManager(manager mcp.MCPManagerInterface) {
	unifai.MCPManager = manager
	// Inject UnifAI's plugin pipeline into the manager's CodeMode so that
	// nested tool calls (e.g. via Starlark executeCode) run through plugin hooks.
	if m, ok := manager.(*mcp.MCPManager); ok {
		m.SetPluginPipeline(
			func() mcp.PluginPipeline {
				pipeline := unifai.getPluginPipeline()
				if pp, ok := any(pipeline).(mcp.PluginPipeline); ok {
					return pp
				}
				return nil
			},
			func(pipeline mcp.PluginPipeline) {
				if pp, ok := pipeline.(*PluginPipeline); ok {
					unifai.releasePluginPipeline(pp)
				}
			},
		)
	}
}

// UpdateMCPClient updates the MCP client.
// This allows for dynamic MCP client tool management at runtime.
//
// Parameters:
//   - id: ID of the client to edit
//   - updatedConfig: Updated MCP client configuration
//
// Returns:
//   - error: Any edit error
//
// Example:
//
//	err := unifai.UpdateMCPClient("my-mcp-client-id", schemas.MCPClientConfig{
//	    Name:           "my-mcp-client-name",
//	    ToolsToExecute: []string{"tool1", "tool2"},
//	})
func (unifai *UnifAI) UpdateMCPClient(id string, updatedConfig *schemas.MCPClientConfig) error {
	if unifai.MCPManager == nil {
		return fmt.Errorf("mcp is not configured in this unifai instance")
	}

	return unifai.MCPManager.UpdateClient(id, updatedConfig)
}

// UpdateMCPClientConnection reconnects an existing MCP client using updated headers
func (unifai *UnifAI) UpdateMCPClientConnection(id string, newConfig *schemas.MCPClientConfig) error {
	if unifai.MCPManager == nil {
		return fmt.Errorf("mcp is not configured in this unifai instance")
	}
	return unifai.MCPManager.UpdateClientConnection(id, newConfig)
}

// ReconnectMCPClient attempts to reconnect an MCP client if it is disconnected.
//
// Parameters:
//   - id: ID of the client to reconnect
//
// Returns:
//   - error: Any reconnection error
func (unifai *UnifAI) ReconnectMCPClient(id string) error {
	if unifai.MCPManager == nil {
		return fmt.Errorf("mcp is not configured in this unifai instance")
	}

	return unifai.MCPManager.ReconnectClient(id)
}

// DisableMCPClient shuts down an MCP client's connection, health monitor, and tool
// syncer without removing it. The client entry is kept in a "disabled" state so it
// can be re-enabled via EnableMCPClient.
func (unifai *UnifAI) DisableMCPClient(id string) error {
	if unifai.MCPManager == nil {
		return fmt.Errorf("mcp is not configured in this unifai instance")
	}
	return unifai.MCPManager.DisableClient(id)
}

// EnableMCPClient reconnects a previously disabled MCP client and restarts its
// health monitor and tool syncer.
func (unifai *UnifAI) EnableMCPClient(id string) error {
	if unifai.MCPManager == nil {
		return fmt.Errorf("mcp is not configured in this unifai instance")
	}
	return unifai.MCPManager.EnableClient(id)
}

// VerifyPerUserOAuthConnection delegates to the MCP manager to verify an MCP
// server using a temporary access token and discover available tools. The
// connection is closed after verification. If the MCP manager is not yet
// initialized, it is lazily created (same as AddMCPClient).
func (unifai *UnifAI) VerifyPerUserOAuthConnection(ctx context.Context, config *schemas.MCPClientConfig, accessToken string) (map[string]schemas.ChatTool, map[string]string, error) {
	// Ensure MCP manager is initialized (lazy init, same pattern as AddMCPClient)
	if unifai.MCPManager == nil {
		unifai.mcpInitOnce.Do(func() {
			mcpConfig := schemas.MCPConfig{
				ClientConfigs: []*schemas.MCPClientConfig{},
			}
			mcpConfig.PluginPipelineProvider = func() interface{} {
				return unifai.getPluginPipeline()
			}
			mcpConfig.ReleasePluginPipeline = func(pipeline interface{}) {
				if pp, ok := pipeline.(*PluginPipeline); ok {
					unifai.releasePluginPipeline(pp)
				}
			}
			codeMode := starlark.NewStarlarkCodeMode(nil, unifai.logger)
			unifai.MCPManager = mcp.NewMCPManager(unifai.ctx, mcpConfig, unifai.mcpCredStore, unifai.logger, codeMode)
		})
	}
	if unifai.MCPManager == nil {
		return nil, nil, fmt.Errorf("MCP manager is not initialized")
	}
	return unifai.MCPManager.VerifyPerUserOAuthConnection(ctx, config, accessToken)
}

// VerifyHeadersConnection delegates to the MCP manager to verify an MCP
// server using caller-supplied header values (admin sample or user-submitted)
// and discover available tools. Mirrors VerifyPerUserOAuthConnection's lazy
// MCP-manager init.
func (unifai *UnifAI) VerifyHeadersConnection(ctx context.Context, config *schemas.MCPClientConfig, userHeaders map[string]string) (map[string]schemas.ChatTool, map[string]string, error) {
	if unifai.MCPManager == nil {
		unifai.mcpInitOnce.Do(func() {
			mcpConfig := schemas.MCPConfig{
				ClientConfigs: []*schemas.MCPClientConfig{},
			}
			mcpConfig.PluginPipelineProvider = func() interface{} {
				return unifai.getPluginPipeline()
			}
			mcpConfig.ReleasePluginPipeline = func(pipeline interface{}) {
				if pp, ok := pipeline.(*PluginPipeline); ok {
					unifai.releasePluginPipeline(pp)
				}
			}
			codeMode := starlark.NewStarlarkCodeMode(nil, unifai.logger)
			unifai.MCPManager = mcp.NewMCPManager(unifai.ctx, mcpConfig, unifai.mcpCredStore, unifai.logger, codeMode)
		})
	}
	if unifai.MCPManager == nil {
		return nil, nil, fmt.Errorf("MCP manager is not initialized")
	}
	return unifai.MCPManager.VerifyHeadersConnection(ctx, config, userHeaders)
}

// SetClientTools delegates to the MCP manager to update the tool map for an
// existing MCP client.
func (unifai *UnifAI) SetClientTools(clientID string, tools map[string]schemas.ChatTool, toolNameMapping map[string]string) {
	if unifai.MCPManager != nil {
		unifai.MCPManager.SetClientTools(clientID, tools, toolNameMapping)
	}
}

// UpdateToolManagerConfig updates the tool manager config for the MCP manager.
// This allows for hot-reloading of the tool manager config at runtime.
// Pass the current value of disableAutoToolInject whenever only other fields
// change so the flag is never silently reset to its zero value.
func (unifai *UnifAI) UpdateToolManagerConfig(maxAgentDepth int, toolExecutionTimeoutInSeconds int, codeModeBindingLevel string, disableAutoToolInject bool) error {
	if unifai.MCPManager == nil {
		return fmt.Errorf("mcp is not configured in this unifai instance")
	}

	unifai.MCPManager.UpdateToolManagerConfig(&schemas.MCPToolManagerConfig{
		MaxAgentDepth:         maxAgentDepth,
		ToolExecutionTimeout:  schemas.Duration(time.Duration(toolExecutionTimeoutInSeconds) * time.Second),
		CodeModeBindingLevel:  schemas.CodeModeBindingLevel(codeModeBindingLevel),
		DisableAutoToolInject: disableAutoToolInject,
	})
	return nil
}

// PROVIDER MANAGEMENT

// createBaseProvider creates a provider based on the base provider type
func (unifai *UnifAI) createBaseProvider(providerKey schemas.ModelProvider, config *schemas.ProviderConfig) (schemas.Provider, error) {
	// Determine which provider type to create
	targetProviderKey := providerKey

	if config.CustomProviderConfig != nil {
		// Validate custom provider config
		if config.CustomProviderConfig.BaseProviderType == "" {
			return nil, fmt.Errorf("custom provider config missing base provider type")
		}

		// Validate that base provider type is supported
		if !IsSupportedBaseProvider(config.CustomProviderConfig.BaseProviderType) {
			return nil, fmt.Errorf("unsupported base provider type: %s", config.CustomProviderConfig.BaseProviderType)
		}

		// Automatically set the custom provider key to the provider name
		config.CustomProviderConfig.CustomProviderKey = string(providerKey)

		targetProviderKey = config.CustomProviderConfig.BaseProviderType
	}

	switch targetProviderKey {
	case schemas.OpenAI:
		return openai.NewOpenAIProvider(config, unifai.logger), nil
	case schemas.Anthropic:
		return anthropic.NewAnthropicProvider(config, unifai.logger), nil
	case schemas.Bedrock:
		return bedrock.NewBedrockProvider(config, unifai.logger)
	case schemas.BedrockMantle:
		return bedrockmantle.NewBedrockMantleProvider(config, unifai.logger)
	case schemas.Cohere:
		return cohere.NewCohereProvider(config, unifai.logger)
	case schemas.Azure:
		return azure.NewAzureProvider(config, unifai.logger)
	case schemas.Vertex:
		return vertex.NewVertexProvider(config, unifai.logger)
	case schemas.Mistral:
		return mistral.NewMistralProvider(config, unifai.logger), nil
	case schemas.Ollama:
		return ollama.NewOllamaProvider(config, unifai.logger)
	case schemas.Groq:
		return groq.NewGroqProvider(config, unifai.logger)
	case schemas.OpencodeGo:
		return opencode.NewOpencodeGoProvider(config, unifai.logger)
	case schemas.OpencodeZen:
		return opencode.NewOpencodeZenProvider(config, unifai.logger)
	case schemas.SGL:
		return sgl.NewSGLProvider(config, unifai.logger)
	case schemas.Parasail:
		return parasail.NewParasailProvider(config, unifai.logger)
	case schemas.Perplexity:
		return perplexity.NewPerplexityProvider(config, unifai.logger)
	case schemas.Cerebras:
		return cerebras.NewCerebrasProvider(config, unifai.logger)
	case schemas.DeepSeek:
		return deepseek.NewDeepSeekProvider(config, unifai.logger)
	case schemas.Gemini:
		return gemini.NewGeminiProvider(config, unifai.logger), nil
	case schemas.OpenRouter:
		return openrouter.NewOpenRouterProvider(config, unifai.logger), nil
	case schemas.Elevenlabs:
		return elevenlabs.NewElevenlabsProvider(config, unifai.logger), nil
	case schemas.Nebius:
		return nebius.NewNebiusProvider(config, unifai.logger)
	case schemas.HuggingFace:
		return huggingface.NewHuggingFaceProvider(config, unifai.logger), nil
	case schemas.XAI:
		return xai.NewXAIProvider(config, unifai.logger)
	case schemas.Replicate:
		return replicate.NewReplicateProvider(config, unifai.logger)
	case schemas.VLLM:
		return vllm.NewVLLMProvider(config, unifai.logger)
	case schemas.Runway:
		return runway.NewRunwayProvider(config, unifai.logger)
	case schemas.Runware:
		return runware.NewRunwareProvider(config, unifai.logger)
	case schemas.Fireworks:
		return fireworks.NewFireworksProvider(config, unifai.logger)
	default:
		if _, ok := openaicompat.Registry[targetProviderKey]; ok {
			return openaicompat.New(targetProviderKey, config, unifai.logger)
		}
		return nil, fmt.Errorf("unsupported provider: %s", targetProviderKey)
	}
}

// prepareProvider sets up a provider with its configuration, keys, and worker channels.
// It initializes the request queue and starts worker goroutines for processing requests.
// Note: This function assumes the caller has already acquired the appropriate mutex for the provider.
func (unifai *UnifAI) prepareProvider(providerKey schemas.ModelProvider, config *schemas.ProviderConfig) error {
	// Create ProviderQueue with lifecycle management
	pq := &ProviderQueue{
		queue:      make(chan *ChannelMessage, config.ConcurrencyAndBufferSize.BufferSize),
		done:       make(chan struct{}),
		signalOnce: sync.Once{},
	}

	unifai.requestQueues.Store(providerKey, pq)

	// Start specified number of workers
	unifai.waitGroups.Store(providerKey, &sync.WaitGroup{})

	provider, err := unifai.createBaseProvider(providerKey, config)
	if err != nil {
		return fmt.Errorf("failed to create provider for the given key: %v", err)
	}

	waitGroupValue, _ := unifai.waitGroups.Load(providerKey)
	currentWaitGroup := waitGroupValue.(*sync.WaitGroup)

	// Atomically append provider to the providers slice
	for {
		oldPtr := unifai.providers.Load()
		var oldSlice []schemas.Provider
		if oldPtr != nil {
			oldSlice = *oldPtr
		}
		newSlice := make([]schemas.Provider, len(oldSlice)+1)
		copy(newSlice, oldSlice)
		newSlice[len(oldSlice)] = provider
		if unifai.providers.CompareAndSwap(oldPtr, &newSlice) {
			break
		}
	}

	schemas.RegisterKnownProvider(providerKey)

	for range config.ConcurrencyAndBufferSize.Concurrency {
		currentWaitGroup.Add(1)
		go unifai.requestWorker(provider, config, pq, currentWaitGroup)
	}

	return nil
}

// getProviderQueue returns the ProviderQueue for a given provider key.
// If the queue doesn't exist, it creates one at runtime and initializes the provider,
// given the provider config is provided in the account interface implementation.
// This function uses read locks to prevent race conditions during provider updates.
// Callers must check the closing flag or select on the done channel before sending.
func (unifai *UnifAI) getProviderQueue(providerKey schemas.ModelProvider) (*ProviderQueue, error) {
	// Use read lock to allow concurrent reads but prevent concurrent updates
	providerMutex := unifai.getProviderMutex(providerKey)
	providerMutex.RLock()

	if pqValue, exists := unifai.requestQueues.Load(providerKey); exists {
		pq := pqValue.(*ProviderQueue)
		providerMutex.RUnlock()
		return pq, nil
	}

	// Provider doesn't exist, need to create it
	// Upgrade to write lock for creation
	providerMutex.RUnlock()
	providerMutex.Lock()
	defer providerMutex.Unlock()

	// Double-check after acquiring write lock (another goroutine might have created it)
	if pqValue, exists := unifai.requestQueues.Load(providerKey); exists {
		pq := pqValue.(*ProviderQueue)
		return pq, nil
	}
	unifai.logger.Debug(fmt.Sprintf("Creating new request queue for provider %s at runtime", providerKey))
	config, err := unifai.account.GetConfigForProvider(providerKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get config for provider %s: %v", providerKey, err)
	}
	if config == nil {
		return nil, fmt.Errorf("config is nil for provider %s", providerKey)
	}
	if err := unifai.prepareProvider(providerKey, config); err != nil {
		return nil, err
	}
	pqValue, ok := unifai.requestQueues.Load(providerKey)
	if !ok {
		return nil, fmt.Errorf("request queue not found for provider %s", providerKey)
	}
	pq := pqValue.(*ProviderQueue)
	return pq, nil
}

// GetProviderByKey returns the provider instance for the given provider key.
// Returns nil if no provider with the given key exists.
func (unifai *UnifAI) GetProviderByKey(providerKey schemas.ModelProvider) schemas.Provider {
	return unifai.getProviderByKey(providerKey)
}

// SelectKeyForProviderRequestType selects an API key for the given provider, request type, and model.
// Used by WebSocket handlers that need a key for upstream connections while honoring request-specific
// AllowedRequests gates such as realtime-only support.
func (unifai *UnifAI) SelectKeyForProviderRequestType(ctx *schemas.UnifAIContext, requestType schemas.RequestType, providerKey schemas.ModelProvider, model string) (schemas.Key, error) {
	if ctx == nil {
		ctx = unifai.ctx
	}
	baseProvider := providerKey
	if config, err := unifai.account.GetConfigForProvider(providerKey); err == nil && config != nil &&
		config.CustomProviderConfig != nil && config.CustomProviderConfig.BaseProviderType != "" {
		baseProvider = config.CustomProviderConfig.BaseProviderType
	}
	supportedKeys, _, err := unifai.selectKeyFromProviderForModelWithPool(ctx, requestType, providerKey, model, baseProvider)
	if err != nil {
		return schemas.Key{}, err
	}
	if len(supportedKeys) == 0 {
		return schemas.Key{}, nil
	}
	if len(supportedKeys) == 1 {
		return supportedKeys[0], nil
	}
	return unifai.keySelector(ctx, supportedKeys, providerKey, model)
}

// ComputeRawStorageForProvider determines whether raw request/response payloads should be
// captured and stored in log records for the given provider. This is the same computation
// performed inside executeRequest (lines 5675-5713), exported for callers that bypass
// the normal inference path (e.g. realtime WebSocket/WebRTC sessions).
func (unifai *UnifAI) ComputeRawStorageForProvider(ctx *schemas.UnifAIContext, providerKey schemas.ModelProvider) bool {
	if ctx == nil {
		ctx = unifai.ctx
	}
	if ctx == nil {
		return false
	}
	config, err := unifai.account.GetConfigForProvider(providerKey)
	if err != nil || config == nil {
		return false
	}
	effectiveStore := config.StoreRawRequestResponse
	allowStorageOverride, _ := ctx.Value(schemas.UnifAIContextKeyAllowPerRequestStorageOverride).(bool)
	if allowStorageOverride {
		if override, ok := ctx.Value(schemas.UnifAIContextKeyStoreRawRequestResponse).(bool); ok {
			effectiveStore = override
		}
	}
	return effectiveStore
}

// WSStreamHooks holds the post-hook runner and cleanup function returned by RunStreamPreHooks.
// Call PostHookRunner for each streaming chunk, setting StreamEndIndicator on the final chunk.
// Call Cleanup when done to release the pipeline back to the pool.
// If ShortCircuitResponse is non-nil, a plugin short-circuited with a cached response —
// the caller should write this response to the client and skip the upstream call.
type WSStreamHooks struct {
	PostHookRunner       schemas.PostHookRunner
	Cleanup              func()
	ShortCircuitResponse *schemas.UnifAIResponse
}

// RealtimeTurnHooks mirrors RunStreamPreHooks but is explicitly scoped to a
// single realtime turn rather than one long-lived transport connection.
type RealtimeTurnHooks struct {
	PostHookRunner schemas.PostHookRunner
	Cleanup        func()
}

// RunPreRequestHooks acquires a plugin pipeline and runs PreRequestHook on each LLM plugin
// for callers that do not flow through handleRequest/handleStreamRequest — primarily realtime
// WebSocket upgrades, where the upgrade itself is the routing decision (once per WS connection)
// but the per-turn pipeline handles PreLLMHook/PostLLMHook separately.
//
// Mutations to req.Provider/req.Model/req.Fallbacks made by PreRequestHook plugins are committed
// to the shared *UnifAIRequest. Plugin errors are non-blocking — they are logged as warnings
// and the pipeline continues to the next plugin (same semantics as RunLLMPreHooks). Callers
// should validate req.Provider after this returns if a provider is required.
func (unifai *UnifAI) RunPreRequestHooks(ctx *schemas.UnifAIContext, req *schemas.UnifAIRequest) {
	if ctx == nil {
		ctx = unifai.ctx
	}

	if _, ok := ctx.Value(schemas.UnifAIContextKeyRequestID).(string); !ok {
		ctx.SetValue(schemas.UnifAIContextKeyRequestID, uuid.New().String())
	}

	pipeline := unifai.getPluginPipeline()
	defer unifai.releasePluginPipeline(pipeline)
	pipeline.RunPreRequestHooks(ctx, req)
	// This path has no downstream post-hook cleanup, so drain any plugin logs
	// emitted by PreRequestHook here to avoid them bleeding into a later request
	// on a reused/long-lived context (e.g. realtime WS connections).
	flushPluginLogs(ctx)
}

// RunStreamPreHooks acquires a plugin pipeline, sets up tracing context, runs PreLLMHooks,
// and returns a PostHookRunner for per-chunk post-processing.
// Used by WebSocket handlers that bypass the normal inference path but still need plugin hooks.
func (unifai *UnifAI) RunStreamPreHooks(ctx *schemas.UnifAIContext, req *schemas.UnifAIRequest) (*WSStreamHooks, *schemas.UnifAIError) {
	if ctx == nil {
		ctx = unifai.ctx
	}

	if _, ok := ctx.Value(schemas.UnifAIContextKeyRequestID).(string); !ok {
		ctx.SetValue(schemas.UnifAIContextKeyRequestID, uuid.New().String())
	}

	tracer := unifai.getTracer()
	ctx.SetValue(schemas.UnifAIContextKeyTracer, tracer)

	// Create a trace so the logging plugin can accumulate streaming chunks.
	// The traceID is used as the accumulator key in ProcessStreamingChunk.
	if _, ok := ctx.Value(schemas.UnifAIContextKeyTraceID).(string); !ok {
		traceID := tracer.CreateTrace("")
		if traceID != "" {
			ctx.SetValue(schemas.UnifAIContextKeyTraceID, traceID)
		}
	}

	// Mark as streaming context so RunPostLLMHooks uses accumulated timing
	ctx.SetValue(schemas.UnifAIContextKeyStreamStartTime, time.Now())

	pipeline := unifai.getPluginPipeline()

	cleanup := func() {
		if traceID, ok := ctx.Value(schemas.UnifAIContextKeyTraceID).(string); ok && traceID != "" {
			tracer.CleanupStreamAccumulator(traceID)
		}
		unifai.releasePluginPipeline(pipeline)
	}

	// Capture provider/model from the original request for early-exit paths below.
	// RequestType, Provider, OriginalModelRequested, and ResolvedModelUsed are always
	// overwritten around RunPostLLMHooks — plugin modifications to these 4 fields are
	// no-ops by design; proper request metadata is preserved and tampering is discouraged.
	reqProvider, reqModel, _ := req.GetRequestFields()

	preReq, shortCircuit, preCount := pipeline.RunLLMPreHooks(ctx, req)
	if preReq == nil && shortCircuit == nil {
		unifaiErr := newUnifAIErrorFromMsg("unifai request after plugin hooks cannot be nil")
		unifaiErr.PopulateExtraFields(req.RequestType, reqProvider, reqModel, reqModel)
		_, unifaiErr = pipeline.RunPostLLMHooks(ctx, nil, unifaiErr, preCount)
		if unifaiErr != nil {
			unifaiErr.PopulateExtraFields(req.RequestType, reqProvider, reqModel, reqModel)
		}
		drainAndAttachPluginLogs(ctx)
		if traceID, ok := ctx.Value(schemas.UnifAIContextKeyTraceID).(string); ok && strings.TrimSpace(traceID) != "" {
			tracer.CompleteAndFlushTrace(strings.TrimSpace(traceID))
		}
		cleanup()
		return nil, unifaiErr
	}
	if shortCircuit != nil {
		if shortCircuit.Error != nil {
			shortCircuit.Error.PopulateExtraFields(req.RequestType, reqProvider, reqModel, reqModel)
			_, unifaiErr := pipeline.RunPostLLMHooks(ctx, nil, shortCircuit.Error, preCount)
			if unifaiErr != nil {
				unifaiErr.PopulateExtraFields(req.RequestType, reqProvider, reqModel, reqModel)
			}
			drainAndAttachPluginLogs(ctx)
			if traceID, ok := ctx.Value(schemas.UnifAIContextKeyTraceID).(string); ok && strings.TrimSpace(traceID) != "" {
				tracer.CompleteAndFlushTrace(strings.TrimSpace(traceID))
			}
			cleanup()
			if unifaiErr != nil {
				return nil, unifaiErr
			}
			return nil, shortCircuit.Error
		}
		if shortCircuit.Response != nil {
			shortCircuit.Response.PopulateExtraFields(req.RequestType, reqProvider, reqModel, reqModel)
			resp, unifaiErr := pipeline.RunPostLLMHooks(ctx, shortCircuit.Response, nil, preCount)
			if unifaiErr != nil {
				unifaiErr.PopulateExtraFields(req.RequestType, reqProvider, reqModel, reqModel)
			} else if resp != nil {
				resp.PopulateExtraFields(req.RequestType, reqProvider, reqModel, reqModel)
			}
			drainAndAttachPluginLogs(ctx)
			if traceID, ok := ctx.Value(schemas.UnifAIContextKeyTraceID).(string); ok && strings.TrimSpace(traceID) != "" {
				tracer.CompleteAndFlushTrace(strings.TrimSpace(traceID))
			}
			cleanup()
			if unifaiErr != nil {
				return nil, unifaiErr
			}
			return &WSStreamHooks{
				Cleanup:              func() {},
				ShortCircuitResponse: resp,
			}, nil
		}
	}

	wsProvider, wsModel, _ := preReq.GetRequestFields()
	postHookRunner := func(ctx *schemas.UnifAIContext, result *schemas.UnifAIResponse, err *schemas.UnifAIError) (*schemas.UnifAIResponse, *schemas.UnifAIError) {
		// Populate extra fields before RunPostLLMHooks so plugins (e.g. logging)
		// can read requestType/provider/model from the chunk or error.
		if result != nil {
			result.PopulateExtraFields(req.RequestType, wsProvider, wsModel, wsModel)
		}
		if err != nil {
			err.PopulateExtraFields(req.RequestType, wsProvider, wsModel, wsModel)
		}
		resp, unifaiErr := pipeline.RunPostLLMHooks(ctx, result, err, preCount)
		if IsFinalChunk(ctx) {
			drainAndAttachPluginLogs(ctx)
			if traceID, ok := ctx.Value(schemas.UnifAIContextKeyTraceID).(string); ok && strings.TrimSpace(traceID) != "" {
				tracer.CompleteAndFlushTrace(strings.TrimSpace(traceID))
			}
		}
		if unifaiErr != nil {
			unifaiErr.PopulateExtraFields(req.RequestType, wsProvider, wsModel, wsModel)
			return nil, unifaiErr
		} else if resp != nil {
			resp.PopulateExtraFields(req.RequestType, wsProvider, wsModel, wsModel)
		}
		return resp, nil
	}

	return &WSStreamHooks{
		PostHookRunner: postHookRunner,
		Cleanup:        cleanup,
	}, nil
}

// RunRealtimeTurnPreHooks acquires a plugin pipeline and runs LLM pre-hooks for
// a single realtime turn. Unlike generic stream hooks, realtime turns do not
// support short-circuit responses in v1 because the transports cannot yet emit a
// fully synthetic assistant turn without an upstream generation.
func (unifai *UnifAI) RunRealtimeTurnPreHooks(ctx *schemas.UnifAIContext, req *schemas.UnifAIRequest) (*RealtimeTurnHooks, *schemas.UnifAIError) {
	if req == nil {
		unifaiErr := newUnifAIErrorFromMsg("realtime turn request is nil")
		unifaiErr.ExtraFields.RequestType = schemas.RealtimeRequest
		return nil, unifaiErr
	}
	if ctx == nil {
		ctx = unifai.ctx
	}

	if _, ok := ctx.Value(schemas.UnifAIContextKeyRequestID).(string); !ok {
		ctx.SetValue(schemas.UnifAIContextKeyRequestID, uuid.New().String())
	}

	tracer := unifai.getTracer()
	ctx.SetValue(schemas.UnifAIContextKeyTracer, tracer)

	if _, ok := ctx.Value(schemas.UnifAIContextKeyTraceID).(string); !ok {
		traceID := tracer.CreateTrace("")
		if traceID != "" {
			ctx.SetValue(schemas.UnifAIContextKeyTraceID, traceID)
		}
	}

	pipeline := unifai.getPluginPipeline()
	cleanup := func() {
		if traceID, ok := ctx.Value(schemas.UnifAIContextKeyTraceID).(string); ok && traceID != "" {
			tracer.CleanupStreamAccumulator(traceID)
		}
		unifai.releasePluginPipeline(pipeline)
	}
	provider, model, _ := req.GetRequestFields()

	preReq, shortCircuit, preCount := pipeline.RunLLMPreHooks(ctx, req)
	if preReq == nil && shortCircuit == nil {
		unifaiErr := newUnifAIErrorFromMsg("unifai request after plugin hooks cannot be nil")
		unifaiErr.PopulateExtraFields(schemas.RealtimeRequest, provider, model, model)
		_, unifaiErr = pipeline.RunPostLLMHooks(ctx, nil, unifaiErr, preCount)
		drainAndAttachPluginLogs(ctx)
		if traceID, ok := ctx.Value(schemas.UnifAIContextKeyTraceID).(string); ok && strings.TrimSpace(traceID) != "" {
			tracer.CompleteAndFlushTrace(strings.TrimSpace(traceID))
		}
		cleanup()
		return nil, unifaiErr
	}
	if shortCircuit != nil {
		if shortCircuit.Error != nil {
			shortCircuit.Error.PopulateExtraFields(schemas.RealtimeRequest, provider, model, model)
			_, unifaiErr := pipeline.RunPostLLMHooks(ctx, nil, shortCircuit.Error, preCount)
			drainAndAttachPluginLogs(ctx)
			if traceID, ok := ctx.Value(schemas.UnifAIContextKeyTraceID).(string); ok && strings.TrimSpace(traceID) != "" {
				tracer.CompleteAndFlushTrace(strings.TrimSpace(traceID))
			}
			cleanup()
			if unifaiErr != nil {
				return nil, unifaiErr
			}
			return nil, shortCircuit.Error
		}
		if shortCircuit.Response != nil {
			// Short-circuit responses are not supported for realtime turns (v1).
			// Treat this like an error turn so plugins can close pending state cleanly.
			unifaiErr := newUnifAIErrorFromMsg("realtime turn short-circuit responses are not supported")
			unifaiErr.PopulateExtraFields(schemas.RealtimeRequest, provider, model, model)
			_, unifaiErr = pipeline.RunPostLLMHooks(ctx, nil, unifaiErr, preCount)
			drainAndAttachPluginLogs(ctx)
			if traceID, ok := ctx.Value(schemas.UnifAIContextKeyTraceID).(string); ok && strings.TrimSpace(traceID) != "" {
				tracer.CompleteAndFlushTrace(strings.TrimSpace(traceID))
			}
			cleanup()
			return nil, unifaiErr
		}
	}

	provider, model, _ = preReq.GetRequestFields()

	return &RealtimeTurnHooks{
		PostHookRunner: func(ctx *schemas.UnifAIContext, result *schemas.UnifAIResponse, err *schemas.UnifAIError) (*schemas.UnifAIResponse, *schemas.UnifAIError) {
			if result != nil {
				result.PopulateExtraFields(schemas.RealtimeRequest, provider, model, model)
			}
			if err != nil {
				err.PopulateExtraFields(schemas.RealtimeRequest, provider, model, model)
			}
			resp, unifaiErr := pipeline.RunPostLLMHooks(ctx, result, err, preCount)
			drainAndAttachPluginLogs(ctx)
			if unifaiErr != nil {
				unifaiErr.PopulateExtraFields(schemas.RealtimeRequest, provider, model, model)
				return nil, unifaiErr
			} else if resp != nil {
				resp.PopulateExtraFields(schemas.RealtimeRequest, provider, model, model)
			}
			return resp, nil
		},
		Cleanup: cleanup,
	}, nil
}

// getProviderByKey retrieves a provider instance from the providers array by its provider key.
// Returns the provider if found, or nil if no provider with the given key exists.
func (unifai *UnifAI) getProviderByKey(providerKey schemas.ModelProvider) schemas.Provider {
	providers := unifai.providers.Load()
	if providers == nil {
		return nil
	}
	// Checking if provider is in the memory
	for _, provider := range *providers {
		if provider.GetProviderKey() == providerKey {
			return provider
		}
	}
	// Could happen when provider is not initialized yet, check if provider config exists in account and if so, initialize it
	config, err := unifai.account.GetConfigForProvider(providerKey)
	if err != nil || config == nil {
		if slices.Contains(dynamicallyConfigurableProviders, providerKey) {
			unifai.logger.Info(fmt.Sprintf("initializing provider %s with default config", providerKey))
			// If no config found, use default config
			config = &schemas.ProviderConfig{
				NetworkConfig:            schemas.DefaultNetworkConfig,
				ConcurrencyAndBufferSize: schemas.DefaultConcurrencyAndBufferSize,
			}
		} else {
			return nil
		}
	}
	// Lock the provider mutex to avoid races
	providerMutex := unifai.getProviderMutex(providerKey)
	providerMutex.Lock()
	defer providerMutex.Unlock()
	// Double-check after acquiring the lock
	providers = unifai.providers.Load()
	if providers != nil {
		for _, p := range *providers {
			if p.GetProviderKey() == providerKey {
				return p
			}
		}
	}
	// Preparing provider
	if err := unifai.prepareProvider(providerKey, config); err != nil {
		return nil
	}
	// Return newly prepared provider without recursion
	providers = unifai.providers.Load()
	if providers != nil {
		for _, p := range *providers {
			if p.GetProviderKey() == providerKey {
				return p
			}
		}
	}
	return nil
}

// CORE INTERNAL LOGIC

// shouldTryFallbacks handles the primary error and returns true if we should proceed with fallbacks, false if we should return immediately
func (unifai *UnifAI) shouldTryFallbacks(req *schemas.UnifAIRequest, primaryErr *schemas.UnifAIError) bool {
	// If no primary error, we succeeded
	if primaryErr == nil {
		unifai.logger.Debug("no primary error, we should not try fallbacks")
		return false
	}

	// Handle request cancellation
	if primaryErr.Error != nil && primaryErr.Error.Type != nil && *primaryErr.Error.Type == schemas.RequestCancelled {
		unifai.logger.Debug("request cancelled, we should not try fallbacks")
		return false
	}

	// Check if this is a short-circuit error that doesn't allow fallbacks
	// Note: AllowFallbacks = nil is treated as true (allow fallbacks by default)
	if primaryErr.AllowFallbacks != nil && !*primaryErr.AllowFallbacks {
		unifai.logger.Debug("allowFallbacks is false, we should not try fallbacks")
		return false
	}

	// If no fallbacks configured, return primary error
	_, _, fallbacks := req.GetRequestFields()
	if len(fallbacks) == 0 {
		unifai.logger.Debug("no fallbacks configured, we should not try fallbacks")
		return false
	}

	// Should proceed with fallbacks
	return true
}

// prepareFallbackRequest creates a fallback request and validates the provider config
// Returns the fallback request or nil if this fallback should be skipped
func (unifai *UnifAI) prepareFallbackRequest(req *schemas.UnifAIRequest, fallback schemas.Fallback) *schemas.UnifAIRequest {
	// Check if we have config for this fallback provider
	_, err := unifai.account.GetConfigForProvider(fallback.Provider)
	if err != nil {
		unifai.logger.Warn("config not found for provider %s, skipping fallback: %v", fallback.Provider, err)
		return nil
	}

	// Create a new request with the fallback provider and model
	fallbackReq := *req

	if req.TextCompletionRequest != nil {
		tmp := *req.TextCompletionRequest
		tmp.Provider = fallback.Provider
		tmp.Model = fallback.Model
		fallbackReq.TextCompletionRequest = &tmp
	}

	if req.ChatRequest != nil {
		tmp := *req.ChatRequest
		tmp.Provider = fallback.Provider
		tmp.Model = fallback.Model
		fallbackReq.ChatRequest = &tmp
	}

	if req.ResponsesRequest != nil {
		tmp := *req.ResponsesRequest
		tmp.Provider = fallback.Provider
		tmp.Model = fallback.Model
		fallbackReq.ResponsesRequest = &tmp
	}

	if req.CountTokensRequest != nil {
		tmp := *req.CountTokensRequest
		tmp.Provider = fallback.Provider
		tmp.Model = fallback.Model
		fallbackReq.CountTokensRequest = &tmp
	}

	if req.CompactionRequest != nil {
		tmp := *req.CompactionRequest
		tmp.Provider = fallback.Provider
		tmp.Model = fallback.Model
		fallbackReq.CompactionRequest = &tmp
	}

	if req.EmbeddingRequest != nil {
		tmp := *req.EmbeddingRequest
		tmp.Provider = fallback.Provider
		tmp.Model = fallback.Model
		fallbackReq.EmbeddingRequest = &tmp
	}
	if req.RerankRequest != nil {
		tmp := *req.RerankRequest
		tmp.Provider = fallback.Provider
		tmp.Model = fallback.Model
		fallbackReq.RerankRequest = &tmp
	}
	if req.OCRRequest != nil {
		tmp := *req.OCRRequest
		tmp.Provider = fallback.Provider
		tmp.Model = fallback.Model
		fallbackReq.OCRRequest = &tmp
	}

	if req.SpeechRequest != nil {
		tmp := *req.SpeechRequest
		tmp.Provider = fallback.Provider
		tmp.Model = fallback.Model
		fallbackReq.SpeechRequest = &tmp
	}

	if req.TranscriptionRequest != nil {
		tmp := *req.TranscriptionRequest
		tmp.Provider = fallback.Provider
		tmp.Model = fallback.Model
		fallbackReq.TranscriptionRequest = &tmp
	}
	if req.ImageGenerationRequest != nil {
		tmp := *req.ImageGenerationRequest
		tmp.Provider = fallback.Provider
		tmp.Model = fallback.Model
		fallbackReq.ImageGenerationRequest = &tmp
	}
	if req.VideoGenerationRequest != nil {
		tmp := *req.VideoGenerationRequest
		tmp.Provider = fallback.Provider
		tmp.Model = fallback.Model
		fallbackReq.VideoGenerationRequest = &tmp
	}
	return &fallbackReq
}

// shouldContinueWithFallbacks processes errors from fallback attempts
// Returns true if we should continue with more fallbacks, false if we should stop
func (unifai *UnifAI) shouldContinueWithFallbacks(fallback schemas.Fallback, fallbackErr *schemas.UnifAIError) bool {
	if fallbackErr.Error.Type != nil && *fallbackErr.Error.Type == schemas.RequestCancelled {
		return false
	}

	// Check if it was a short-circuit error that doesn't allow fallbacks
	if fallbackErr.AllowFallbacks != nil && !*fallbackErr.AllowFallbacks {
		return false
	}

	unifai.logger.Debug(fmt.Sprintf("Fallback provider %s failed: %s", fallback.Provider, fallbackErr.Error.Message))
	return true
}

// handleRequest handles the request to the provider based on the request type
// It handles plugin hooks, request validation, response processing, and fallback providers.
// If the primary provider fails, it will try each fallback provider in order until one succeeds.
// It is the wrapper for all non-streaming public API methods.
func (unifai *UnifAI) handleRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIRequest) (*schemas.UnifAIResponse, *schemas.UnifAIError) {
	defer unifai.releaseUnifAIRequest(req)
	provider, model, fallbacks := req.GetRequestFields()

	// Handle nil context early to prevent blocking
	if ctx == nil {
		ctx = unifai.ctx
	}

	// Try the primary provider first
	ctx.SetValue(schemas.UnifAIContextKeyFallbackIndex, 0)
	// Ensure request ID is set in context before PreHooks
	if _, ok := ctx.Value(schemas.UnifAIContextKeyRequestID).(string); !ok {
		requestID := uuid.New().String()
		ctx.SetValue(schemas.UnifAIContextKeyRequestID, requestID)
	}

	// PreRequestHook: once-per-request phase where plugins decide provider/model/fallbacks
	// (and may mutate other request fields). Mutations commit to req and are observed by
	// all downstream phases and fallbacks. Plugin errors are non-blocking (logged + skipped).
	preReqPipeline := unifai.getPluginPipeline()
	preReqPipeline.RunPreRequestHooks(ctx, req)
	unifai.releasePluginPipeline(preReqPipeline)
	// Re-read after PreRequestHook — provider/model/fallbacks may have changed.
	provider, model, fallbacks = req.GetRequestFields()
	// Empty provider/model after PreRequestHook means no plugin
	// could pick a provider for this model — the caller's input is unresolvable.
	if err := validateRequestAfterPreRequestHooks(req); err != nil {
		// Returning before tryRequest skips the downstream log drain, so flush
		// any PreRequestHook-emitted plugin logs here.
		flushPluginLogs(ctx)
		err.PopulateExtraFields(req.RequestType, provider, model, model)
		return nil, err
	}

	unifai.logger.Debug(fmt.Sprintf("primary provider %s with model %s and %d fallbacks", provider, model, len(fallbacks)))

	primaryResult, primaryErr := unifai.tryRequest(ctx, req)
	if primaryErr != nil {
		if primaryErr.Error != nil {
			unifai.logger.Debug(fmt.Sprintf("primary provider %s with model %s returned error: %s", provider, model, primaryErr.Error.Message))
		} else {
			unifai.logger.Debug(fmt.Sprintf("primary provider %s with model %s returned error: %v", provider, model, primaryErr))
		}
		if len(fallbacks) > 0 {
			unifai.logger.Debug(fmt.Sprintf("check if we should try %d fallbacks", len(fallbacks)))
		}
	}

	// Check if we should proceed with fallbacks
	shouldTryFallbacks := unifai.shouldTryFallbacks(req, primaryErr)
	if !shouldTryFallbacks {
		return primaryResult, primaryErr
	}

	// Core is about to make routing decisions of its own (fallback transitions)
	// — record it on the request's used-engines list so the audit trail closes
	// the loop on whatever plugin-level engine selected the primary upstream.
	schemas.AppendToContextList(ctx, schemas.UnifAIContextKeyRoutingEnginesUsed, schemas.RoutingEngineCore)
	ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelInfo, fmt.Sprintf("Primary %s/%s failed (%s); evaluating %d configured fallback(s)", provider, model, routingErrorSummary(primaryErr), len(fallbacks)))

	// Tracks the most recent failure so each fallback transition log carries
	// the error that triggered it (primary error for the first iteration, the
	// prior fallback's error for subsequent iterations).
	lastErr := primaryErr

	// Try fallbacks in order
	for i, fallback := range fallbacks {
		ctx.SetValue(schemas.UnifAIContextKeyFallbackIndex, i+1)
		unifai.logger.Debug(fmt.Sprintf("trying fallback provider %s with model %s", fallback.Provider, fallback.Model))
		ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelInfo, fmt.Sprintf("Trying fallback %d/%d: %s/%s (previous attempt failed: %s)", i+1, len(fallbacks), fallback.Provider, fallback.Model, routingErrorSummary(lastErr)))
		ctx.SetValue(schemas.UnifAIContextKeyFallbackRequestID, uuid.New().String())
		clearCtxForFallback(ctx)

		// Start span for fallback attempt
		tracer := unifai.getTracer()
		spanCtx, handle := tracer.StartSpan(ctx, fmt.Sprintf("fallback.%s.%s", fallback.Provider, fallback.Model), schemas.SpanKindFallback)
		tracer.SetAttribute(handle, schemas.AttrProviderName, schemas.OTelProviderName(fallback.Provider))
		tracer.SetAttribute(handle, schemas.AttrUnifAIProviderName, string(fallback.Provider)) // raw UnifAI short name, mirrors canonical gen_ai.provider.name
		tracer.SetAttribute(handle, schemas.AttrRequestModel, fallback.Model)
		tracer.SetAttribute(handle, "fallback.index", i+1)
		ctx.SetValue(schemas.UnifAIContextKeySpanID, spanCtx.Value(schemas.UnifAIContextKeySpanID))

		fallbackReq := unifai.prepareFallbackRequest(req, fallback)
		if fallbackReq == nil {
			unifai.logger.Debug(fmt.Sprintf("fallback provider %s with model %s is nil", fallback.Provider, fallback.Model))
			ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelWarn, fmt.Sprintf("Fallback %s/%s skipped: missing provider config", fallback.Provider, fallback.Model))
			tracer.SetAttribute(handle, "error", "fallback request preparation failed")
			tracer.EndSpan(handle, schemas.SpanStatusError, "fallback request preparation failed")
			continue
		}

		// Try the fallback provider
		result, fallbackErr := unifai.tryRequest(ctx, fallbackReq)
		// Layer on Primary/IsFallback — the per-attempt code populates only
		// attempt-level RoutingInfo (Provider/Model/Key/ResolvedKeyAlias);
		// fallback-relative signals belong to the orchestrator scope.
		result.SetFallbackRoutingInfo(provider, model)
		fallbackErr.SetFallbackRoutingInfo(provider, model)
		if fallbackErr == nil {
			unifai.logger.Debug(fmt.Sprintf("successfully used fallback provider %s with model %s", fallback.Provider, fallback.Model))
			ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelInfo, fmt.Sprintf("Request served by fallback %s/%s (attempt %d/%d)", fallback.Provider, fallback.Model, i+1, len(fallbacks)))
			tracer.EndSpan(handle, schemas.SpanStatusOk, "")
			return result, nil
		}

		// End span with error status
		if fallbackErr.Error != nil {
			tracer.SetAttribute(handle, "error", fallbackErr.Error.Message)
		}
		tracer.EndSpan(handle, schemas.SpanStatusError, "fallback failed")

		// Check if we should continue with more fallbacks
		if !unifai.shouldContinueWithFallbacks(fallback, fallbackErr) {
			ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelError, fmt.Sprintf("Fallback %s/%s failed (%s); halting further fallbacks", fallback.Provider, fallback.Model, routingErrorSummary(fallbackErr)))
			return nil, fallbackErr
		}

		lastErr = fallbackErr
	}

	ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelError, fmt.Sprintf("All %d fallback(s) exhausted; returning primary error (%s)", len(fallbacks), routingErrorSummary(primaryErr)))
	// All providers failed, return the original error
	return nil, primaryErr
}

// handleStreamRequest handles the stream request to the provider based on the request type
// It handles plugin hooks, request validation, response processing, and fallback providers.
// If the primary provider fails, it will try each fallback provider in order until one succeeds.
// It is the wrapper for all streaming public API methods.
func (unifai *UnifAI) handleStreamRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	defer unifai.releaseUnifAIRequest(req)
	provider, model, fallbacks := req.GetRequestFields()

	// Handle nil context early to prevent blocking
	if ctx == nil {
		ctx = unifai.ctx
	}

	// Try the primary provider first
	ctx.SetValue(schemas.UnifAIContextKeyFallbackIndex, 0)
	// Ensure request ID is set in context before PreHooks
	if _, ok := ctx.Value(schemas.UnifAIContextKeyRequestID).(string); !ok {
		requestID := uuid.New().String()
		ctx.SetValue(schemas.UnifAIContextKeyRequestID, requestID)
	}

	// PreRequestHook: once-per-request phase. See handleRequest for semantics.
	preReqPipeline := unifai.getPluginPipeline()
	preReqPipeline.RunPreRequestHooks(ctx, req)
	unifai.releasePluginPipeline(preReqPipeline)
	// Re-read after PreRequestHook — provider/model/fallbacks may have changed.
	provider, model, fallbacks = req.GetRequestFields()
	// Empty provider after PreRequestHook means no plugin
	// could pick a provider for this model — the caller's input is unresolvable.
	if err := validateRequestAfterPreRequestHooks(req); err != nil {
		// Returning before tryStreamRequest skips the downstream log drain, so
		// flush any PreRequestHook-emitted plugin logs here.
		flushPluginLogs(ctx)
		err.PopulateExtraFields(req.RequestType, provider, model, model)
		return nil, err
	}

	unifai.logger.Debug(fmt.Sprintf("primary provider %s with model %s and %d fallbacks", provider, model, len(fallbacks)))

	primaryResult, primaryErr := unifai.tryStreamRequest(ctx, req)
	if primaryErr != nil {
		if primaryErr.Error != nil {
			unifai.logger.Debug(fmt.Sprintf("primary provider %s with model %s returned error: %s", provider, model, primaryErr.Error.Message))
		} else {
			unifai.logger.Debug(fmt.Sprintf("primary provider %s with model %s returned error: %v", provider, model, primaryErr))
		}
		if len(fallbacks) > 0 {
			unifai.logger.Debug(fmt.Sprintf("check if we should try %d fallbacks", len(fallbacks)))
		}
	}

	// Check if we should proceed with fallbacks
	shouldTryFallbacks := unifai.shouldTryFallbacks(req, primaryErr)
	if !shouldTryFallbacks {
		return primaryResult, primaryErr
	}

	// Mirror handleRequest: register core on the engines-used list and post
	// the primary-failure entry to the routing engine log trail before
	// iterating fallbacks. See handleRequest for the rationale.
	schemas.AppendToContextList(ctx, schemas.UnifAIContextKeyRoutingEnginesUsed, schemas.RoutingEngineCore)
	ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelInfo, fmt.Sprintf("Primary %s/%s failed (%s); evaluating %d configured fallback(s)", provider, model, routingErrorSummary(primaryErr), len(fallbacks)))

	lastErr := primaryErr

	// Try fallbacks in order
	for i, fallback := range fallbacks {
		ctx.SetValue(schemas.UnifAIContextKeyFallbackIndex, i+1)
		ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelInfo, fmt.Sprintf("Trying fallback %d/%d: %s/%s (previous attempt failed: %s)", i+1, len(fallbacks), fallback.Provider, fallback.Model, routingErrorSummary(lastErr)))
		ctx.SetValue(schemas.UnifAIContextKeyFallbackRequestID, uuid.New().String())
		clearCtxForFallback(ctx)

		// Start span for fallback attempt
		tracer := unifai.getTracer()
		spanCtx, handle := tracer.StartSpan(ctx, fmt.Sprintf("fallback.%s.%s", fallback.Provider, fallback.Model), schemas.SpanKindFallback)
		tracer.SetAttribute(handle, schemas.AttrProviderName, schemas.OTelProviderName(fallback.Provider))
		tracer.SetAttribute(handle, schemas.AttrUnifAIProviderName, string(fallback.Provider)) // raw UnifAI short name, mirrors canonical gen_ai.provider.name
		tracer.SetAttribute(handle, schemas.AttrRequestModel, fallback.Model)
		tracer.SetAttribute(handle, "fallback.index", i+1)
		ctx.SetValue(schemas.UnifAIContextKeySpanID, spanCtx.Value(schemas.UnifAIContextKeySpanID))

		fallbackReq := unifai.prepareFallbackRequest(req, fallback)
		if fallbackReq == nil {
			ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelWarn, fmt.Sprintf("Fallback %s/%s skipped: missing provider config", fallback.Provider, fallback.Model))
			tracer.SetAttribute(handle, "error", "fallback request preparation failed")
			tracer.EndSpan(handle, schemas.SpanStatusError, "fallback request preparation failed")
			continue
		}

		// Try the fallback provider
		result, fallbackErr := unifai.tryStreamRequest(ctx, fallbackReq)
		// Layer on Primary/IsFallback on errors. For the success case the
		// result is a chan of stream chunks emitted asynchronously — those
		// chunks already carry per-attempt RoutingInfo populated upstream,
		// but Primary/IsFallback aren't reachable from here without wrapping
		// the channel. See SetFallbackRoutingInfo doc.
		fallbackErr.SetFallbackRoutingInfo(provider, model)
		if fallbackErr == nil {
			unifai.logger.Debug(fmt.Sprintf("successfully used fallback provider %s with model %s", fallback.Provider, fallback.Model))
			ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelInfo, fmt.Sprintf("Request served by fallback %s/%s (attempt %d/%d)", fallback.Provider, fallback.Model, i+1, len(fallbacks)))
			tracer.EndSpan(handle, schemas.SpanStatusOk, "")
			return result, nil
		}

		// End span with error status
		if fallbackErr.Error != nil {
			tracer.SetAttribute(handle, "error", fallbackErr.Error.Message)
		}
		tracer.EndSpan(handle, schemas.SpanStatusError, "fallback failed")

		// Check if we should continue with more fallbacks
		if !unifai.shouldContinueWithFallbacks(fallback, fallbackErr) {
			ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelError, fmt.Sprintf("Fallback %s/%s failed (%s); halting further fallbacks", fallback.Provider, fallback.Model, routingErrorSummary(fallbackErr)))
			return nil, fallbackErr
		}

		lastErr = fallbackErr
	}

	ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelError, fmt.Sprintf("All %d fallback(s) exhausted; returning primary error (%s)", len(fallbacks), routingErrorSummary(primaryErr)))
	// All providers failed, return the original error
	return nil, primaryErr
}

// tryRequest is a generic function that handles common request processing logic
// It consolidates queue setup, plugin pipeline execution, enqueue logic, and response handling
func (unifai *UnifAI) tryRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIRequest) (*schemas.UnifAIResponse, *schemas.UnifAIError) {
	provider, model, _ := req.GetRequestFields()
	pq, err := unifai.getProviderQueue(provider)
	if err != nil {
		unifaiErr := newUnifAIError(err)
		unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
		return nil, unifaiErr
	}

	// Add MCP tools to request if MCP is configured and requested
	if unifai.MCPManager != nil {
		req = unifai.MCPManager.AddToolsToRequest(ctx, req)
	}

	tracer := unifai.getTracer()
	if tracer == nil {
		unifaiErr := newUnifAIErrorFromMsg("tracer not found in context")
		unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
		return nil, unifaiErr
	}

	// Store tracer in context BEFORE calling requestHandler, so streaming goroutines
	// have access to it for completing deferred spans when the stream ends.
	// The streaming goroutine captures the context when it starts, so these values
	// must be set before requestHandler() is called.
	ctx.SetValue(schemas.UnifAIContextKeyTracer, tracer)

	pipeline := unifai.getPluginPipeline()
	defer unifai.releasePluginPipeline(pipeline)

	// RequestType, Provider, OriginalModelRequested, and ResolvedModelUsed are always
	// overwritten around RunPostLLMHooks — plugin modifications to these 4 fields are
	// no-ops by design; proper request metadata is preserved and tampering is discouraged.
	preReq, shortCircuit, preCount := pipeline.RunLLMPreHooks(ctx, req)
	if shortCircuit != nil {
		// Handle short-circuit with response (success case)
		if shortCircuit.Response != nil {
			shortCircuit.Response.PopulateExtraFields(req.RequestType, provider, model, model)
			resp, unifaiErr := pipeline.RunPostLLMHooks(ctx, shortCircuit.Response, nil, preCount)
			if unifaiErr != nil {
				unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
			} else if resp != nil {
				resp.PopulateExtraFields(req.RequestType, provider, model, model)
			}
			drainAndAttachPluginLogs(ctx)
			if unifaiErr != nil {
				return nil, unifaiErr
			}
			return resp, nil
		}
		// Handle short-circuit with error
		if shortCircuit.Error != nil {
			shortCircuit.Error.PopulateExtraFields(req.RequestType, provider, model, model)
			resp, unifaiErr := pipeline.RunPostLLMHooks(ctx, nil, shortCircuit.Error, preCount)
			if unifaiErr != nil {
				unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
			} else if resp != nil {
				resp.PopulateExtraFields(req.RequestType, provider, model, model)
			}
			drainAndAttachPluginLogs(ctx)
			if unifaiErr != nil {
				return nil, unifaiErr
			}
			return resp, nil
		}
	}
	if preReq == nil {
		unifaiErr := newUnifAIErrorFromMsg("unifai request after plugin hooks cannot be nil")
		unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
		return nil, unifaiErr
	}

	provider, model, _ = preReq.GetRequestFields()

	msg := unifai.getChannelMessage(*preReq)
	msg.Context = ctx

	// If the queue is closing, check whether the provider was updated (new queue
	// available) or removed. On update, transparently re-route to the new queue
	// so in-flight producers don't get spurious errors. On removal, error out.
	//
	// Use a direct sync.Map lookup instead of getProviderQueue to avoid the
	// lazy-creation path: getProviderQueue can resurrect a provider that was
	// just removed by RemoveProvider if the account config still exists.
	if pq.isClosing() {
		var reroutedPq *ProviderQueue
		if val, ok := unifai.requestQueues.Load(provider); ok {
			if candidate := val.(*ProviderQueue); candidate != pq && !candidate.isClosing() {
				reroutedPq = candidate
			}
		}
		if reroutedPq == nil {
			unifai.releaseChannelMessage(msg)
			unifaiErr := newUnifAIErrorFromMsg("provider is shutting down")
			unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
			return nil, unifaiErr
		}
		pq = reroutedPq
	}

	// Use select with done channel to detect shutdown during send
	select {
	case pq.queue <- msg:
		// Message was sent successfully
	case <-pq.done:
		unifai.releaseChannelMessage(msg)
		unifaiErr := newUnifAIErrorFromMsg("provider is shutting down")
		unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
		return nil, unifaiErr
	case <-ctx.Done():
		unifai.releaseChannelMessage(msg)
		unifaiErr := newUnifAICtxDoneError(ctx, "while waiting for queue space")
		unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
		return nil, unifaiErr
	default:
		if unifai.dropExcessRequests.Load() {
			unifai.releaseChannelMessage(msg)
			unifai.logger.Warn("request dropped: queue is full, please increase the queue size or set dropExcessRequests to false")
			unifaiErr := newUnifAIErrorFromMsg("request dropped: queue is full")
			unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
			return nil, unifaiErr
		}
		// Re-check closing flag before blocking send (lock-free atomic check)
		if pq.isClosing() {
			unifai.releaseChannelMessage(msg)
			unifaiErr := newUnifAIErrorFromMsg("provider is shutting down")
			unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
			return nil, unifaiErr
		}
		select {
		case pq.queue <- msg:
			// Message was sent successfully
		case <-pq.done:
			unifai.releaseChannelMessage(msg)
			unifaiErr := newUnifAIErrorFromMsg("provider is shutting down")
			unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
			return nil, unifaiErr
		case <-ctx.Done():
			unifai.releaseChannelMessage(msg)
			unifaiErr := newUnifAICtxDoneError(ctx, "while waiting for queue space")
			unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
			return nil, unifaiErr
		}
	}

	var result *schemas.UnifAIResponse
	var resp *schemas.UnifAIResponse
	pluginCount := len(*unifai.llmPlugins.Load())
	select {
	case result = <-msg.Response:
		resp, unifaiErr := pipeline.RunPostLLMHooks(msg.Context, result, nil, pluginCount)
		if unifaiErr != nil {
			unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
		} else if resp != nil {
			resp.PopulateExtraFields(req.RequestType, provider, model, model)
		}
		drainAndAttachPluginLogs(msg.Context)
		if unifaiErr != nil {
			unifai.releaseChannelMessage(msg)
			return nil, unifaiErr
		}
		unifai.releaseChannelMessage(msg)
		// Strip raw fields that were captured for logging but should not reach the client.
		if resp != nil {
			dropReq, _ := ctx.Value(schemas.UnifAIContextKeyDropRawRequestFromClient).(bool)
			dropResp, _ := ctx.Value(schemas.UnifAIContextKeyDropRawResponseFromClient).(bool)
			if dropReq || dropResp {
				extraField := resp.GetExtraFields()
				if dropReq {
					extraField.RawRequest = nil
				}
				if dropResp {
					extraField.RawResponse = nil
				}
			}
		}
		return resp, nil
	case unifaiErrVal := <-msg.Err:
		unifaiErrPtr := &unifaiErrVal
		resp, unifaiErrPtr = pipeline.RunPostLLMHooks(msg.Context, nil, unifaiErrPtr, pluginCount)
		if unifaiErrPtr != nil {
			unifaiErrPtr.PopulateExtraFields(req.RequestType, provider, model, model)
		} else if resp != nil {
			resp.PopulateExtraFields(req.RequestType, provider, model, model)
		}
		drainAndAttachPluginLogs(msg.Context)
		unifai.releaseChannelMessage(msg)
		// Strip raw fields on error path too.
		dropReq, _ := ctx.Value(schemas.UnifAIContextKeyDropRawRequestFromClient).(bool)
		dropResp, _ := ctx.Value(schemas.UnifAIContextKeyDropRawResponseFromClient).(bool)
		if dropReq || dropResp {
			if unifaiErrPtr != nil {
				if dropReq {
					unifaiErrPtr.ExtraFields.RawRequest = nil
				}
				if dropResp {
					unifaiErrPtr.ExtraFields.RawResponse = nil
				}
			}
			if resp != nil {
				extraField := resp.GetExtraFields()
				if dropReq {
					extraField.RawRequest = nil
				}
				if dropResp {
					extraField.RawResponse = nil
				}
			}
		}
		if unifaiErrPtr != nil {
			return nil, unifaiErrPtr
		}
		return resp, nil
	case <-ctx.Done():
		// Do NOT releaseChannelMessage here. The message is already enqueued and
		// the worker still holds a reference to msg.Response and msg.Err. Returning
		// those channels to the pool now would let the next request reuse them while
		// the worker is still writing to them — stale data corruption. The worker
		// never calls releaseChannelMessage itself, so this message leaks from the
		// pool and is GC'd. That is intentional: a small pool leak on cancellation
		// is far safer than corrupting another request's channels.
		provider, model, _ := req.GetRequestFields()
		unifaiErr := newUnifAICtxDoneError(ctx, "waiting for provider response")
		unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
		return nil, unifaiErr
	}
}

// tryStreamRequest is a generic function that handles common request processing logic
// It consolidates queue setup, plugin pipeline execution, enqueue logic, and response handling
func (unifai *UnifAI) tryStreamRequest(ctx *schemas.UnifAIContext, req *schemas.UnifAIRequest) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	provider, model, _ := req.GetRequestFields()
	pq, err := unifai.getProviderQueue(provider)
	if err != nil {
		unifaiErr := newUnifAIError(err)
		unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
		return nil, unifaiErr
	}

	// Add MCP tools to request if MCP is configured and requested
	if req.RequestType != schemas.SpeechStreamRequest && req.RequestType != schemas.TranscriptionStreamRequest && unifai.MCPManager != nil {
		req = unifai.MCPManager.AddToolsToRequest(ctx, req)
	}

	tracer := unifai.getTracer()
	if tracer == nil {
		unifaiErr := newUnifAIErrorFromMsg("tracer not found in context")
		unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
		return nil, unifaiErr
	}

	// Store tracer in context BEFORE calling RunLLMPreHooks, so plugins and streaming goroutines
	// have access to it for completing deferred spans when the stream ends.
	// The streaming goroutine captures the context when it starts, so these values
	// must be set before requestHandler() is called.
	ctx.SetValue(schemas.UnifAIContextKeyTracer, tracer)

	// Ensure traceID exists so the logging plugin can create a stream accumulator
	// in PreLLMHook and accumulate chunks in PostLLMHook. For HTTP handler requests the
	// tracing middleware already sets this; for WebSocket bridge and Go SDK callers it
	// may be absent.
	if _, ok := ctx.Value(schemas.UnifAIContextKeyTraceID).(string); !ok {
		traceID := tracer.CreateTrace("")
		if traceID != "" {
			ctx.SetValue(schemas.UnifAIContextKeyTraceID, traceID)
		}
	}

	pipeline := unifai.getPluginPipeline()
	releasePipeline := true
	defer func() {
		if releasePipeline {
			unifai.releasePluginPipeline(pipeline)
		}
	}()

	// RequestType, Provider, OriginalModelRequested, and ResolvedModelUsed are always
	// overwritten around RunPostLLMHooks — plugin modifications to these 4 fields are
	// no-ops by design; proper request metadata is preserved and tampering is discouraged.
	preReq, shortCircuit, preCount := pipeline.RunLLMPreHooks(ctx, req)
	if shortCircuit != nil {
		// Handle short-circuit with response (success case)
		if shortCircuit.Response != nil {
			shortCircuit.Response.PopulateExtraFields(req.RequestType, provider, model, model)
			resp, unifaiErr := pipeline.RunPostLLMHooks(ctx, shortCircuit.Response, nil, preCount)
			if unifaiErr != nil {
				unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
			} else if resp != nil {
				resp.PopulateExtraFields(req.RequestType, provider, model, model)
			}
			drainAndAttachPluginLogs(ctx)
			if unifaiErr != nil {
				return nil, unifaiErr
			}
			return newUnifAIMessageChan(resp), nil
		}
		// Handle short-circuit with stream
		if shortCircuit.Stream != nil {
			outputStream := make(chan *schemas.UnifAIStreamChunk)
			releasePipeline = false // pipeline is released inside the goroutine after stream drains

			// Snapshot RequestType before the closure. The *UnifAIRequest is released
			// back to unifaiRequestPool when handleStreamRequest returns (via defer);
			// a concurrent request can reuse it and overwrite RequestType.
			shortCircuitRequestType := req.RequestType
			// Create a post hook runner cause pipeline object is put back in the pool on defer
			pipelinePostHookRunner := func(ctx *schemas.UnifAIContext, result *schemas.UnifAIResponse, err *schemas.UnifAIError) (*schemas.UnifAIResponse, *schemas.UnifAIError) {
				if result != nil {
					result.PopulateExtraFields(shortCircuitRequestType, provider, model, model)
				}
				if err != nil {
					err.PopulateExtraFields(shortCircuitRequestType, provider, model, model)
				}
				resp, unifaiErr := pipeline.RunPostLLMHooks(ctx, result, err, preCount)
				if IsFinalChunk(ctx) {
					drainAndAttachPluginLogs(ctx)
				}
				if unifaiErr != nil {
					unifaiErr.PopulateExtraFields(shortCircuitRequestType, provider, model, model)
					return nil, unifaiErr
				} else if resp != nil {
					resp.PopulateExtraFields(shortCircuitRequestType, provider, model, model)
				}
				return resp, nil
			}

			go func() {
				defer func() {
					drainAndAttachPluginLogs(ctx) // ensure logs are drained even if stream closes without a final chunk
					pipeline.FinalizeStreamingPostHookSpans(ctx)
					unifai.releasePluginPipeline(pipeline)
				}()
				defer providerUtils.CloseStream(ctx, outputStream)

				for streamMsg := range shortCircuit.Stream {
					if streamMsg == nil {
						continue
					}

					unifaiResponse := &schemas.UnifAIResponse{}
					if streamMsg.UnifAITextCompletionResponse != nil {
						unifaiResponse.TextCompletionResponse = streamMsg.UnifAITextCompletionResponse
					}
					if streamMsg.UnifAIChatResponse != nil {
						unifaiResponse.ChatResponse = streamMsg.UnifAIChatResponse
					}
					if streamMsg.UnifAIResponsesStreamResponse != nil {
						unifaiResponse.ResponsesStreamResponse = streamMsg.UnifAIResponsesStreamResponse
					}
					if streamMsg.UnifAISpeechStreamResponse != nil {
						unifaiResponse.SpeechStreamResponse = streamMsg.UnifAISpeechStreamResponse
					}
					if streamMsg.UnifAITranscriptionStreamResponse != nil {
						unifaiResponse.TranscriptionStreamResponse = streamMsg.UnifAITranscriptionStreamResponse
					}
					if streamMsg.UnifAIImageGenerationStreamResponse != nil {
						unifaiResponse.ImageGenerationStreamResponse = streamMsg.UnifAIImageGenerationStreamResponse
					}

					// Run post hooks on the stream message
					processedResponse, processedError := pipelinePostHookRunner(ctx, unifaiResponse, streamMsg.UnifAIError)

					// Build the client-facing chunk via the shared helper, which strips raw
					// request/response fields when in logging-only mode without mutating the
					// shared processedResponse or processedError objects.
					streamResponse := providerUtils.BuildClientStreamChunk(ctx, processedResponse, processedError)

					// Guarded send: if the consumer abandons outputStream (client
					// disconnect, ctx cancel), drain the upstream shortCircuit.Stream
					// so its producer can exit cleanly instead of blocking on its send.
					// GateSendChunk routes through the pause/resume gate when a plugin
					// has engaged it; otherwise it's a bare ctx-guarded channel send.
					if !providerUtils.GateSendChunk(ctx, streamResponse, outputStream) {
						for range shortCircuit.Stream {
						}
						return
					}

					// TODO: Release the processed response immediately after use
				}
			}()

			return outputStream, nil
		}
		// Handle short-circuit with error
		if shortCircuit.Error != nil {
			shortCircuit.Error.PopulateExtraFields(req.RequestType, provider, model, model)
			resp, unifaiErr := pipeline.RunPostLLMHooks(ctx, nil, shortCircuit.Error, preCount)
			if unifaiErr != nil {
				unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
			} else if resp != nil {
				resp.PopulateExtraFields(req.RequestType, provider, model, model)
			}
			drainAndAttachPluginLogs(ctx)
			if unifaiErr != nil {
				return nil, unifaiErr
			}
			return newUnifAIMessageChan(resp), nil
		}
	}
	if preReq == nil {
		unifaiErr := newUnifAIErrorFromMsg("unifai request after plugin hooks cannot be nil")
		unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
		return nil, unifaiErr
	}

	provider, model, _ = preReq.GetRequestFields()

	msg := unifai.getChannelMessage(*preReq)
	msg.Context = ctx

	// If the queue is closing, check whether the provider was updated (new queue
	// available) or removed. On update, transparently re-route to the new queue
	// so in-flight producers don't get spurious errors. On removal, error out.
	//
	// Use a direct sync.Map lookup instead of getProviderQueue to avoid the
	// lazy-creation path: getProviderQueue can resurrect a provider that was
	// just removed by RemoveProvider if the account config still exists.
	if pq.isClosing() {
		var reroutedPq *ProviderQueue
		if val, ok := unifai.requestQueues.Load(provider); ok {
			if candidate := val.(*ProviderQueue); candidate != pq && !candidate.isClosing() {
				reroutedPq = candidate
			}
		}
		if reroutedPq == nil {
			unifai.releaseChannelMessage(msg)
			unifaiErr := newUnifAIErrorFromMsg("provider is shutting down")
			unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
			return nil, unifaiErr
		}
		pq = reroutedPq
	}

	// Use select with done channel to detect shutdown during send
	select {
	case pq.queue <- msg:
		// Message was sent successfully
	case <-pq.done:
		unifai.releaseChannelMessage(msg)
		unifaiErr := newUnifAIErrorFromMsg("provider is shutting down")
		unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
		return nil, unifaiErr
	case <-ctx.Done():
		unifai.releaseChannelMessage(msg)
		unifaiErr := newUnifAICtxDoneError(ctx, "while waiting for queue space")
		unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
		return nil, unifaiErr
	default:
		if unifai.dropExcessRequests.Load() {
			unifai.releaseChannelMessage(msg)
			unifai.logger.Warn("request dropped: queue is full, please increase the queue size or set dropExcessRequests to false")
			unifaiErr := newUnifAIErrorFromMsg("request dropped: queue is full")
			unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
			return nil, unifaiErr
		}
		// Re-check closing flag before blocking send (lock-free atomic check)
		if pq.isClosing() {
			unifai.releaseChannelMessage(msg)
			unifaiErr := newUnifAIErrorFromMsg("provider is shutting down")
			unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
			return nil, unifaiErr
		}
		select {
		case pq.queue <- msg:
			// Message was sent successfully
		case <-pq.done:
			unifai.releaseChannelMessage(msg)
			unifaiErr := newUnifAIErrorFromMsg("provider is shutting down")
			unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
			return nil, unifaiErr
		case <-ctx.Done():
			unifai.releaseChannelMessage(msg)
			unifaiErr := newUnifAICtxDoneError(ctx, "while waiting for queue space")
			unifaiErr.PopulateExtraFields(req.RequestType, provider, model, model)
			return nil, unifaiErr
		}
	}

	select {
	case stream := <-msg.ResponseStream:
		unifai.releaseChannelMessage(msg)
		return stream, nil
	case unifaiErrVal := <-msg.Err:
		if unifaiErrVal.Error != nil {
			unifai.logger.Debug("error while executing stream request: %s", unifaiErrVal.Error.Message)
		} else {
			unifai.logger.Debug("error while executing stream request: %+v", unifaiErrVal)
		}
		// Marking final chunk
		ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
		// On error we will complete post-hooks
		recoveredResp, recoveredErr := pipeline.RunPostLLMHooks(ctx, nil, &unifaiErrVal, len(*unifai.llmPlugins.Load()))
		if recoveredErr != nil {
			recoveredErr.PopulateExtraFields(req.RequestType, provider, model, model)
		} else if recoveredResp != nil {
			recoveredResp.PopulateExtraFields(req.RequestType, provider, model, model)
		}
		drainAndAttachPluginLogs(ctx)
		unifai.releaseChannelMessage(msg)
		if recoveredErr != nil {
			return nil, recoveredErr
		}
		if recoveredResp != nil {
			return newUnifAIMessageChan(recoveredResp), nil
		}
		return nil, &unifaiErrVal
	case <-ctx.Done():
		// Do NOT releaseChannelMessage here — see the identical note in tryRequest.
		// Worker still holds msg.ResponseStream/msg.Err; releasing now corrupts the
		// next request that reuses those pooled channels.
		return nil, newUnifAICtxDoneError(ctx, "while waiting for stream response")
	}
}

// errAllKeysDead is returned by a keyProvider closure when every key in the configured pool
// has been marked permanently dead via deadKeyIDs (or, for a fixed/sticky key, when that key
// is itself dead). executeRequestWithRetries detects this via errors.Is and surfaces it as a
// synthetic 502 upstream_credentials_exhausted, rather than bubbling the raw 401/403 which
// would falsely suggest the *caller's* UnifAI API key is bad. Any other error from the
// keyProvider (custom selector failure, etc.) is propagated unchanged.
var errAllKeysDead = errors.New("all configured keys returned permanent per-key errors (401/402/403)")

// errAllKeysFiltered is returned by a keyProvider closure when healthy (non-dead) keys exist but
// the KeyPoolFilter hook suppressed all of them. Unlike errAllKeysDead this is a transient
// condition (the filter/circuit breaker self-heals), so it surfaces as a 503 rather than a 502.
var errAllKeysFiltered = errors.New("all eligible keys are temporarily suppressed by the key pool filter")

// executeRequestWithRetries is a generic function that handles common request processing logic.
// It consolidates retry logic, backoff calculation, error handling, and key rotation.
// It is not a unifai method because interface methods in go cannot be generic.
//
// keyProvider, when non-nil, is called on the first attempt and again whenever a per-key error
// triggers a rotation. It receives two sets of key IDs to exclude:
//   - usedKeyIDs: keys that hit a transient per-key failure (429). When the pool is exhausted of
//     non-dead keys, the provider resets this set and starts a fresh weighted round — a previously
//     rate-limited key may have free quota by then.
//   - deadKeyIDs: keys that hit a permanent per-key failure (401/402/403). These are NEVER reset
//     within a single request — a bad credential will not become valid by waiting.
//
// Network/5xx errors reuse the same key since they are transient server issues, not per-key.
func executeRequestWithRetries[T any](
	ctx *schemas.UnifAIContext,
	config *schemas.ProviderConfig,
	requestHandler func(key schemas.Key) (T, *schemas.UnifAIError),
	keyProvider func(usedKeyIDs, deadKeyIDs map[string]bool) (schemas.Key, error),
	requestType schemas.RequestType,
	providerKey schemas.ModelProvider,
	model string,
	req *schemas.UnifAIRequest,
	logger schemas.Logger,
) (result T, unifaiError *schemas.UnifAIError) {
	var attempts int

	// Emit the terminal routing-engine entry on every return path — including
	// early returns from key-selection failures and tracer-missing — so the
	// audit trail isn't truncated when execution exits before reaching the
	// natural end of the function. Skipped when attempts == 0: the request
	// never crossed core's retry-orchestration boundary, so there's nothing
	// to record.
	defer func() {
		if attempts <= 0 {
			return
		}
		schemas.AppendToContextList(ctx, schemas.UnifAIContextKeyRoutingEnginesUsed, schemas.RoutingEngineCore)
		switch {
		case unifaiError == nil:
			ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelInfo, fmt.Sprintf("Request to %s/%s succeeded after %d retry attempt(s)", providerKey, model, attempts))
		case unifaiError.IsUnifAIError:
			ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelError, fmt.Sprintf("Retries halted for %s/%s after %d attempt(s): internal UnifAI error (%s)", providerKey, model, attempts, routingErrorSummary(unifaiError)))
		case unifaiError.Error != nil && unifaiError.Error.Type != nil && *unifaiError.Error.Type == schemas.RequestCancelled:
			ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelError, fmt.Sprintf("Request to %s/%s cancelled after %d attempt(s)", providerKey, model, attempts))
		default:
			ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelError, fmt.Sprintf("Retries exhausted for %s/%s after %d attempt(s); last error: %s", providerKey, model, attempts, routingErrorSummary(unifaiError)))
		}
	}()

	var currentKey schemas.Key
	var usedKeyIDs map[string]bool
	var deadKeyIDs map[string]bool
	lastWasPerKeyFailure := false
	// True iff the previous attempt failed with a *permanent* per-key error (401/402/403).
	// Used to suppress backoff on the next attempt only when we genuinely rotated to a
	// different credential — a dead key gains nothing from waiting. 429 rotations stay
	// subject to backoff because account-level rate limits share quota across keys.
	lastWasPermanentKeyFailure := false
	// ID of the key used on the previous attempt. Compared against the freshly selected
	// key to confirm an actual credential swap happened before suppressing backoff —
	// after a 429 pool reset, the selector can legitimately re-pick the same key.
	previousKeyID := ""
	// Index in UnifAIContextKeyAttemptTrail of an attempt that hit a rate limit and is waiting
	// to learn whether the *next* key selection actually picks a different key. -1 = no pending.
	pendingRotationAttemptIdx := -1

	for attempts = 0; attempts <= config.NetworkConfig.MaxRetries; attempts++ {
		ctx.SetValue(schemas.UnifAIContextKeyNumberOfRetries, attempts)

		// Reset the trail on the first attempt so a reused or shared context (unifai.ctx)
		// doesn't carry over records from a previous request.
		if keyProvider != nil && attempts == 0 {
			ctx.SetValue(schemas.UnifAIContextKeyAttemptTrail, []schemas.KeyAttemptRecord{})
		}

		// Select / rotate key: always on attempt 0, and again when the previous failure was
		// tied to the key itself (rate-limit, auth, billing, or permission error). Transient
		// server errors (5xx, network) keep the same key since they're not per-key problems.
		if keyProvider != nil && (attempts == 0 || lastWasPerKeyFailure) {
			if usedKeyIDs == nil {
				usedKeyIDs = make(map[string]bool)
			}

			// Wrap key selection in a dedicated span so traces show which key was chosen
			// (and when rotation happened). The span is opened before keyProvider is called
			// so selection errors are captured too.
			keyTracer, _ := ctx.Value(schemas.UnifAIContextKeyTracer).(schemas.Tracer)
			var keySpanCtx context.Context
			var keyHandle schemas.SpanHandle
			if keyTracer != nil {
				keySpanCtx, keyHandle = keyTracer.StartSpan(ctx, "key.selection", schemas.SpanKindInternal)
				keyTracer.SetAttribute(keyHandle, schemas.AttrProviderName, schemas.OTelProviderName(providerKey))
				keyTracer.SetAttribute(keyHandle, schemas.AttrUnifAIProviderName, string(providerKey)) // raw UnifAI short name, mirrors canonical gen_ai.provider.name
				keyTracer.SetAttribute(keyHandle, schemas.AttrRequestModel, model)
				if attempts > 0 {
					keyTracer.SetAttribute(keyHandle, schemas.AttrLegacyRetryCount, attempts)
				}
			}

			selectedKey, err := keyProvider(usedKeyIDs, deadKeyIDs)

			if keyTracer != nil {
				if err != nil {
					keyTracer.SetAttribute(keyHandle, "error", err.Error())
					keyTracer.EndSpan(keyHandle, schemas.SpanStatusError, err.Error())
				} else {
					keyTracer.SetAttribute(keyHandle, "key.id", selectedKey.ID)
					keyTracer.SetAttribute(keyHandle, "key.name", selectedKey.Name)
					keyTracer.EndSpan(keyHandle, schemas.SpanStatusOk, "")
					// Propagate the span context so subsequent spans (llm.call / retry.attempt.N)
					// are correctly linked in the trace hierarchy.
					ctx.SetValue(schemas.UnifAIContextKeySpanID, keySpanCtx.Value(schemas.UnifAIContextKeySpanID))
				}
			}

			if err != nil {
				var zero T
				// Clear any selected_key_* set by a *previous* attempt: this early return
				// skips the terminal cleanup at the end of the function, and the invariant
				// is that selected_key_id / selected_key_name are populated only on a
				// successful response. Use attempt_trail for failure attribution.
				ctx.SetValue(schemas.UnifAIContextKeySelectedKeyID, "")
				ctx.SetValue(schemas.UnifAIContextKeySelectedKeyName, "")
				// Only collapse into 502 upstream_credentials_exhausted when keyProvider
				// explicitly signals "every key is dead" via the errAllKeysDead sentinel.
				// Any other error (custom selector failure, etc.) propagates unchanged so
				// that a stray selector error doesn't get misreported as exhausted just
				// because *some* keys happened to be dead.
				if errors.Is(err, errAllKeysFiltered) {
					statusCode := 503
					errType := "no_eligible_keys"
					return zero, &schemas.UnifAIError{
						IsUnifAIError: false,
						StatusCode:     &statusCode,
						Type:           &errType,
						Error: &schemas.ErrorField{
							Type:    &errType,
							Message: err.Error(),
						},
					}
				}
				if errors.Is(err, errAllKeysDead) {
					statusCode := 502
					errType := "upstream_credentials_exhausted"
					return zero, &schemas.UnifAIError{
						IsUnifAIError: false,
						StatusCode:     &statusCode,
						Type:           &errType,
						Error: &schemas.ErrorField{
							Type:    &errType,
							Message: err.Error(),
						},
					}
				}
				return zero, newUnifAIErrorFromMsg(err.Error())
			}
			currentKey = selectedKey
			ctx.SetValue(schemas.UnifAIContextKeySelectedKeyID, currentKey.ID)
			ctx.SetValue(schemas.UnifAIContextKeySelectedKeyName, currentKey.Name)

			// Resolve any pending rotation marker from the previous failed attempt. Only mark
			// TriggeredRotation=true if the newly selected key differs from the failed one —
			// fixed-key paths return the same key, in which case no rotation actually happened.
			if pendingRotationAttemptIdx >= 0 {
				if trail, ok := ctx.Value(schemas.UnifAIContextKeyAttemptTrail).([]schemas.KeyAttemptRecord); ok &&
					pendingRotationAttemptIdx < len(trail) &&
					trail[pendingRotationAttemptIdx].KeyID != currentKey.ID {
					trail[pendingRotationAttemptIdx].TriggeredRotation = true
					ctx.SetValue(schemas.UnifAIContextKeyAttemptTrail, trail)
				}
				pendingRotationAttemptIdx = -1
			}
		}

		// Append a trail record for every attempt (key rotation and same-key retries alike).
		// Skipped when keyProvider is nil (keyless providers have no key to track).
		// FailReason is populated below once the attempt outcome is known.
		if keyProvider != nil {
			schemas.AppendToContextList(ctx, schemas.UnifAIContextKeyAttemptTrail, schemas.KeyAttemptRecord{
				Attempt: attempts,
				KeyID:   currentKey.ID,
				KeyName: currentKey.Name,
			})
		}

		if attempts > 0 {
			// Log retry attempt
			var retryMsg string
			if unifaiError != nil && unifaiError.Error != nil {
				retryMsg = unifaiError.Error.Message
			} else if unifaiError != nil && unifaiError.StatusCode != nil {
				retryMsg = fmt.Sprintf("status=%d", *unifaiError.StatusCode)
				if unifaiError.Type != nil {
					retryMsg += ", type=" + *unifaiError.Type
				}
			}
			logger.Debug("retrying request (attempt %d/%d) for model %s: %s", attempts, config.NetworkConfig.MaxRetries, model, retryMsg)

			// Skip backoff only when (a) we genuinely rotated to a different credential AND
			// (b) the previous failure was a *permanent* per-key error (401/402/403) where
			// waiting offers nothing against a dead key.
			//
			// Backoff is preserved in every other case:
			//   - 5xx / network retries (same key) — transient upstream issue, classic backoff.
			//   - 429 rotations — account-level rate limits share quota across keys, so the
			//     new key may not have fresh capacity; the backoff lets the quota window slide.
			//   - 429 pool reset that re-picks the same key — no rotation actually happened.
			//   - keyless providers — currentKey.ID stays empty, so keyChanged is false.
			keyChanged := keyProvider != nil && currentKey.ID != previousKeyID

			// Emit a routing-engine log entry for this retry transition so the
			// per-request audit trail records *why* core decided to retry and
			// whether it rotated the credential. routingErrorSummary() omits
			// the upstream message so keys/PII don't leak into the log row.
			// Key.Name is a user-set label (not the secret value) and is safe.
			schemas.AppendToContextList(ctx, schemas.UnifAIContextKeyRoutingEnginesUsed, schemas.RoutingEngineCore)
			// Omit the key=... segment for keyless providers, where currentKey is
			// the zero value and the trailing token would render as "same key=".
			keyNote := ""
			if keyProvider != nil {
				rotationNote := "same key"
				if keyChanged {
					rotationNote = "rotated key"
				}
				keyNote = fmt.Sprintf("; %s=%s", rotationNote, currentKey.Name)
			}
			ctx.AppendRoutingEngineLog(schemas.RoutingEngineCore, schemas.LogLevelInfo, fmt.Sprintf("Retry %d/%d for %s/%s (previous attempt failed: %s%s)", attempts, config.NetworkConfig.MaxRetries, providerKey, model, routingErrorSummary(unifaiError), keyNote))

			if !(lastWasPermanentKeyFailure && keyChanged) {
				backoff := calculateBackoff(attempts-1, config)
				logger.Debug("sleeping for %s before retry", backoff)
				time.Sleep(backoff)
			}
		}

		logger.Debug("attempting %s request for provider %s", requestType, providerKey)

		// Start span for LLM call (or retry attempt)
		tracer, ok := ctx.Value(schemas.UnifAIContextKeyTracer).(schemas.Tracer)
		if !ok || tracer == nil {
			logger.Error("tracer not found in context of executeRequestWithRetries")
			return result, newUnifAIErrorFromMsg("tracer not found in context")
		}
		var spanName string
		var spanKind schemas.SpanKind
		otelOp := schemas.OTelOperationName(requestType)
		if attempts > 0 {
			spanName = fmt.Sprintf("retry.attempt.%d", attempts)
			spanKind = schemas.SpanKindRetry
		} else {
			// Span name format per OTel GenAI semconv: "{operation} {model}".
			spanName = fmt.Sprintf("%s %s", otelOp, model)
			spanKind = schemas.SpanKindLLMCall
		}
		spanCtx, handle := tracer.StartSpan(ctx, spanName, spanKind)
		tracer.SetAttribute(handle, schemas.AttrProviderName, schemas.OTelProviderName(providerKey))
		tracer.SetAttribute(handle, schemas.AttrUnifAIProviderName, string(providerKey)) // raw UnifAI short name, mirrors canonical gen_ai.provider.name
		tracer.SetAttribute(handle, schemas.AttrRequestModel, model)
		tracer.SetAttribute(handle, schemas.AttrOperationName, otelOp)
		tracer.SetAttribute(handle, schemas.AttrLegacyRequestType, string(requestType)) // legacy: replaced by gen_ai.operation.name
		if attempts > 0 {
			tracer.SetAttribute(handle, schemas.AttrLegacyRetryCount, attempts) // legacy: bare key with no semconv prefix
		}

		// Add context-related attributes (selected key, virtual key, team, customer, etc.)
		// Each AttrXxx (gen_ai.*) emission below is LEGACY namespace pollution: the
		// UnifAI-internal concept does not belong under gen_ai.*. The unifai.* mirrors
		// are the canonical home going forward; once all dashboards migrate, drop the
		// gen_ai.* lines (grep for "// legacy:" in this block).
		if selectedKeyID, ok := ctx.Value(schemas.UnifAIContextKeySelectedKeyID).(string); ok && selectedKeyID != "" {
			tracer.SetAttribute(handle, schemas.AttrSelectedKeyID, selectedKeyID) // legacy: gen_ai.* placement of unifai-internal attr
			tracer.SetAttribute(handle, schemas.AttrUnifAISelectedKeyID, selectedKeyID)
		}
		if selectedKeyName, ok := ctx.Value(schemas.UnifAIContextKeySelectedKeyName).(string); ok && selectedKeyName != "" {
			tracer.SetAttribute(handle, schemas.AttrSelectedKeyName, selectedKeyName) // legacy: gen_ai.* placement of unifai-internal attr
			tracer.SetAttribute(handle, schemas.AttrUnifAISelectedKeyName, selectedKeyName)
		}
		if virtualKeyID, ok := ctx.Value(schemas.UnifAIContextKeyGovernanceVirtualKeyID).(string); ok && virtualKeyID != "" {
			tracer.SetAttribute(handle, schemas.AttrVirtualKeyID, virtualKeyID) // legacy: gen_ai.* placement of unifai-internal attr
			tracer.SetAttribute(handle, schemas.AttrUnifAIVirtualKeyID, virtualKeyID)
		}
		if virtualKeyName, ok := ctx.Value(schemas.UnifAIContextKeyGovernanceVirtualKeyName).(string); ok && virtualKeyName != "" {
			tracer.SetAttribute(handle, schemas.AttrVirtualKeyName, virtualKeyName) // legacy: gen_ai.* placement of unifai-internal attr
			tracer.SetAttribute(handle, schemas.AttrUnifAIVirtualKeyName, virtualKeyName)
		}
		if teamID, ok := ctx.Value(schemas.UnifAIContextKeyGovernanceTeamID).(string); ok && teamID != "" {
			tracer.SetAttribute(handle, schemas.AttrTeamID, teamID) // legacy: gen_ai.* placement of unifai-internal attr
			tracer.SetAttribute(handle, schemas.AttrUnifAITeamID, teamID)
		}
		if teamName, ok := ctx.Value(schemas.UnifAIContextKeyGovernanceTeamName).(string); ok && teamName != "" {
			tracer.SetAttribute(handle, schemas.AttrTeamName, teamName) // legacy: gen_ai.* placement of unifai-internal attr
			tracer.SetAttribute(handle, schemas.AttrUnifAITeamName, teamName)
		}
		if customerID, ok := ctx.Value(schemas.UnifAIContextKeyGovernanceCustomerID).(string); ok && customerID != "" {
			tracer.SetAttribute(handle, schemas.AttrCustomerID, customerID) // legacy: gen_ai.* placement of unifai-internal attr
			tracer.SetAttribute(handle, schemas.AttrUnifAICustomerID, customerID)
		}
		if customerName, ok := ctx.Value(schemas.UnifAIContextKeyGovernanceCustomerName).(string); ok && customerName != "" {
			tracer.SetAttribute(handle, schemas.AttrCustomerName, customerName) // legacy: gen_ai.* placement of unifai-internal attr
			tracer.SetAttribute(handle, schemas.AttrUnifAICustomerName, customerName)
		}
		if businessUnitID, ok := ctx.Value(schemas.UnifAIContextKeyGovernanceBusinessUnitID).(string); ok && businessUnitID != "" {
			tracer.SetAttribute(handle, schemas.AttrUnifAIBusinessUnitID, businessUnitID)
		}
		if businessUnitName, ok := ctx.Value(schemas.UnifAIContextKeyGovernanceBusinessUnitName).(string); ok && businessUnitName != "" {
			tracer.SetAttribute(handle, schemas.AttrUnifAIBusinessUnitName, businessUnitName)
		}
		if teamIDs, ok := ctx.Value(schemas.UnifAIContextKeyGovernanceTeamIDs).([]string); ok && len(teamIDs) > 0 {
			tracer.SetAttribute(handle, schemas.AttrUnifAITeamIDs, teamIDs)
		}
		if teamNames, ok := ctx.Value(schemas.UnifAIContextKeyGovernanceTeamNames).([]string); ok && len(teamNames) > 0 {
			tracer.SetAttribute(handle, schemas.AttrUnifAITeamNames, teamNames)
		}
		if customerIDs, ok := ctx.Value(schemas.UnifAIContextKeyGovernanceCustomerIDs).([]string); ok && len(customerIDs) > 0 {
			tracer.SetAttribute(handle, schemas.AttrUnifAICustomerIDs, customerIDs)
		}
		if customerNames, ok := ctx.Value(schemas.UnifAIContextKeyGovernanceCustomerNames).([]string); ok && len(customerNames) > 0 {
			tracer.SetAttribute(handle, schemas.AttrUnifAICustomerNames, customerNames)
		}
		if businessUnitIDs, ok := ctx.Value(schemas.UnifAIContextKeyGovernanceBusinessUnitIDs).([]string); ok && len(businessUnitIDs) > 0 {
			tracer.SetAttribute(handle, schemas.AttrUnifAIBusinessUnitIDs, businessUnitIDs)
		}
		if businessUnitNames, ok := ctx.Value(schemas.UnifAIContextKeyGovernanceBusinessUnitNames).([]string); ok && len(businessUnitNames) > 0 {
			tracer.SetAttribute(handle, schemas.AttrUnifAIBusinessUnitNames, businessUnitNames)
		}
		if userID, ok := ctx.Value(schemas.UnifAIContextKeyUserID).(string); ok && userID != "" {
			tracer.SetAttribute(handle, schemas.AttrUnifAIUserID, userID)
		}
		if userName, ok := ctx.Value(schemas.UnifAIContextKeyUserName).(string); ok && userName != "" {
			tracer.SetAttribute(handle, schemas.AttrUnifAIUserName, userName)
		}
		if fallbackIndex, ok := ctx.Value(schemas.UnifAIContextKeyFallbackIndex).(int); ok {
			tracer.SetAttribute(handle, schemas.AttrFallbackIndex, fallbackIndex) // legacy: gen_ai.* placement of unifai-internal attr
			tracer.SetAttribute(handle, schemas.AttrUnifAIFallbackIndex, fallbackIndex)
		}
		tracer.SetAttribute(handle, schemas.AttrNumberOfRetries, attempts) // legacy: gen_ai.* placement of unifai-internal attr
		tracer.SetAttribute(handle, schemas.AttrUnifAIRetries, attempts)

		// Surface caller-supplied extra headers (from x-uf-eh-* and direct-allowlist
		// header forwarding) as span attributes so observability backends see the
		// same set UnifAI forwards to the upstream provider.
		if extraHeaders, ok := ctx.Value(schemas.UnifAIContextKeyExtraHeaders).(map[string][]string); ok {
			for name, values := range extraHeaders {
				if name == "" || len(values) == 0 {
					continue
				}
				// Never export credential-bearing headers verbatim. The transport
				// layer denylists most sensitive headers, but plain authorization /
				// set-cookie can still reach here, and core SDK callers bypass that
				// guard entirely. Keep the key (presence is useful) but redact the value.
				if schemas.IsSensitiveHeader(name) {
					tracer.SetAttribute(handle, schemas.AttrExtraHeaderPrefix+name, schemas.RedactedAttrValue)
					continue
				}
				if len(values) == 1 {
					tracer.SetAttribute(handle, schemas.AttrExtraHeaderPrefix+name, values[0])
				} else {
					tracer.SetAttribute(handle, schemas.AttrExtraHeaderPrefix+name, values)
				}
			}
		}

		// Populate LLM request attributes (messages, parameters, etc.)
		if req != nil {
			tracer.PopulateLLMRequestAttributes(handle, req)
		}

		// Update context with span ID
		ctx.SetValue(schemas.UnifAIContextKeySpanID, spanCtx.Value(schemas.UnifAIContextKeySpanID))

		// Record stream start time for TTFT calculation (only for streaming requests)
		// This is also used by RunPostLLMHooks to detect streaming mode
		if IsStreamRequestType(requestType) {
			streamStartTime := time.Now()
			ctx.SetValue(schemas.UnifAIContextKeyStreamStartTime, streamStartTime)
		}

		// Attempt the request
		result, unifaiError = requestHandler(currentKey)

		// For streaming requests that returned success, check if the first chunk
		// is actually an error (e.g., rate limits sent as SSE events in HTTP 200).
		// This enables retries and fallbacks for providers that embed errors in
		// the SSE stream instead of returning proper HTTP error status codes.
		if unifaiError == nil {
			if streamChan, ok := any(result).(chan *schemas.UnifAIStreamChunk); ok {
				checkedStream, drainDone, firstChunkErr := providerUtils.CheckFirstStreamChunkForError(ctx, streamChan)
				if firstChunkErr != nil {
					<-drainDone
					// The dead stream's teardown (ReleaseStreamingResponse) claimed the
					// connection_closed flag on the shared context. That claim is scoped
					// to the response it released; clear it so the retry or fallback
					// attempt that follows doesn't see its own fresh stream as already
					// closed and fail every read with ErrStreamClosed.
					ctx.ClearValue(schemas.UnifAIContextKeyConnectionClosed)
					unifaiError = firstChunkErr
				} else {
					result = any(checkedStream).(T)
				}
			}
		}

		// Check if result is a streaming channel - if so, defer span completion
		// Only defer for successful stream setup; error paths must end the span synchronously
		isStreamChan := false
		if unifaiError == nil {
			if ch, ok := any(result).(chan *schemas.UnifAIStreamChunk); ok && ch != nil {
				isStreamChan = true
			}
		}
		if isStreamChan {
			// For streaming requests, store the span handle in TraceStore keyed by trace ID
			// This allows the provider's streaming goroutine to retrieve it later
			if traceID, ok := ctx.Value(schemas.UnifAIContextKeyTraceID).(string); ok && traceID != "" {
				tracer.StoreDeferredSpan(traceID, handle)
			}
			// Don't end the span here - it will be ended when streaming completes
		} else {
			// Populate LLM response attributes for non-streaming responses
			if resp, ok := any(result).(*schemas.UnifAIResponse); ok {
				// Populate ExtraFields with provider/model/requestType before cost
				// calculation, because the per-request worker only calls PopulateExtraFields
				// after executeRequestWithRetries returns (line ~5802).  Without this,
				// resp.GetExtraFields().Provider is empty and CalculateCost always returns 0.
				// This is currently done for showing correct OTEL cost calculations. Will check if there is a better way to get this done
				resolvedModelUsed := model
				if req != nil {
					if _, rm, _ := req.GetRequestFields(); rm != "" {
						resolvedModelUsed = rm
					}
				}
				resp.PopulateExtraFields(requestType, providerKey, model, resolvedModelUsed)
				tracer.PopulateLLMResponseAttributes(ctx, handle, resp, unifaiError)
			}

			// End span with appropriate status
			if unifaiError != nil {
				if unifaiError.Error != nil {
					tracer.SetAttribute(handle, "error", unifaiError.Error.Message)
				}
				if unifaiError.StatusCode != nil {
					tracer.SetAttribute(handle, "status_code", *unifaiError.StatusCode)
				}
				tracer.EndSpan(handle, schemas.SpanStatusError, "request failed")
			} else {
				tracer.EndSpan(handle, schemas.SpanStatusOk, "")
			}
		}

		logger.Debug("request %s for provider %s completed", requestType, providerKey)

		// Check if successful or if we should retry
		if unifaiError == nil ||
			unifaiError.IsUnifAIError ||
			(unifaiError.Error != nil && unifaiError.Error.Type != nil && *unifaiError.Error.Type == schemas.RequestCancelled) {
			break
		}

		// Classify the failure to decide whether to retry and whether to rotate the key.
		//
		// isPerKeyFailure: failure is bound to this specific key/account (401/402/403/429, or a
		//   rate-limit error surfaced via message text instead of a 429 status). The same key
		//   won't help — try a different one.
		// retryable 5xx / network errors: transient server issues — retry with the same key.
		shouldRetry := false
		isPerKeyFailure := (unifaiError.StatusCode != nil && perKeyFailureStatusCodes[*unifaiError.StatusCode]) ||
			(unifaiError.Error != nil &&
				(IsRateLimitErrorMessage(unifaiError.Error.Message) ||
					(unifaiError.Error.Type != nil && IsRateLimitErrorMessage(*unifaiError.Error.Type)) ||
					(unifaiError.Error.Code != nil && IsRateLimitErrorMessage(*unifaiError.Error.Code))))

		errMessage := unifaiError.GetErrorString()

		if unifaiError.Error != nil &&
			(unifaiError.Error.Message == schemas.ErrProviderDoRequest ||
				unifaiError.Error.Message == schemas.ErrProviderNetworkError) {
			shouldRetry = true
			logger.Debug("detected request HTTP/network error, will retry: %s", errMessage)
		} else if (unifaiError.StatusCode != nil && transientServerStatusCodes[*unifaiError.StatusCode]) || isPerKeyFailure {
			shouldRetry = true
			logger.Debug("encountered error that should be retried: %s", errMessage)
		}

		// Fill FailReason on any failed attempt (retryable or terminal). The trail field
		// answers "why was this key skipped?", so for rotation-triggering status codes the
		// status itself is the truthful answer — provider Type labels can be misleading
		// (e.g. OpenAI returns Type="invalid_request_error" for 401 invalid_api_key, which
		// describes the request, not the rotation reason). Fall back to provider Type for
		// non-rotation failures, then "unknown".
		if trail, ok := ctx.Value(schemas.UnifAIContextKeyAttemptTrail).([]schemas.KeyAttemptRecord); ok && len(trail) > 0 {
			reason := "unknown"
			switch {
			case unifaiError.StatusCode != nil && *unifaiError.StatusCode == 429:
				reason = "rate_limit_error"
			case unifaiError.StatusCode != nil && (*unifaiError.StatusCode == 401 || *unifaiError.StatusCode == 403):
				reason = "authentication_error"
			case unifaiError.StatusCode != nil && *unifaiError.StatusCode == 402:
				reason = "billing_error"
			case unifaiError.Error != nil && unifaiError.Error.Type != nil && *unifaiError.Error.Type != "":
				reason = *unifaiError.Error.Type
			}
			trail[len(trail)-1].FailReason = &reason
			ctx.SetValue(schemas.UnifAIContextKeyAttemptTrail, trail)
		}

		if !shouldRetry {
			break
		}

		// Track key state so the next keyProvider call excludes this key. Permanent
		// per-key failures (401/402/403) go into deadKeyIDs which is never reset within
		// this request — a bad credential won't become valid by waiting. Transient
		// per-key failures (429) go into usedKeyIDs which the keyProvider may reset
		// once all keys are exhausted, since a rate-limited key may have free quota by
		// the time we come back to it.
		isPermanentKeyFailure := false
		if isPerKeyFailure && keyProvider != nil {
			isPermanentKeyFailure = unifaiError.StatusCode != nil &&
				(*unifaiError.StatusCode == 401 || *unifaiError.StatusCode == 402 || *unifaiError.StatusCode == 403)
			if isPermanentKeyFailure {
				if deadKeyIDs == nil {
					deadKeyIDs = make(map[string]bool)
				}
				deadKeyIDs[currentKey.ID] = true
			} else {
				if usedKeyIDs == nil {
					usedKeyIDs = make(map[string]bool)
				}
				usedKeyIDs[currentKey.ID] = true
			}
		}
		lastWasPerKeyFailure = isPerKeyFailure
		lastWasPermanentKeyFailure = isPermanentKeyFailure
		// Remember the key used on this attempt so the next iteration's backoff check
		// can detect whether key selection genuinely picked a different credential.
		previousKeyID = currentKey.ID

		// Record the just-failed attempt as a *candidate* for rotation. The next iteration will
		// confirm it (and set TriggeredRotation=true) only if key selection actually picks a
		// different key — this avoids false positives for fixed-key providers whose keyProvider
		// is non-nil but returns the same key. Network-error retries reuse the same key, and
		// terminal attempts (attempts == MaxRetries) won't run another iteration.
		if lastWasPerKeyFailure && keyProvider != nil && attempts < config.NetworkConfig.MaxRetries {
			if trail, ok := ctx.Value(schemas.UnifAIContextKeyAttemptTrail).([]schemas.KeyAttemptRecord); ok && len(trail) > 0 {
				pendingRotationAttemptIdx = len(trail) - 1
			}
		}
	}

	// Add retry information to error
	if attempts > 0 {
		logger.Debug("request failed after %d %s", attempts, map[bool]string{true: "attempts", false: "attempt"}[attempts > 1])
	}

	// Terminal routing-engine log entry is emitted by the defer at the top of
	// the function so it runs on every return path, including the early
	// returns from key-selection or tracer-missing.

	// On final error, clear selected_key so it only reflects a key that actually served a successful response.
	// The attempt trail is the authoritative record of which keys were tried.
	if unifaiError != nil && keyProvider != nil {
		ctx.SetValue(schemas.UnifAIContextKeySelectedKeyID, "")
		ctx.SetValue(schemas.UnifAIContextKeySelectedKeyName, "")
	}

	return result, unifaiError
}

// clearAnthropicPassthroughForNonNativeProvider disables Anthropic raw-body passthrough when a
// request from the Anthropic-format integration resolves to a provider that doesn't speak the
// Anthropic Messages API natively (e.g. Bedrock), forcing that provider to convert the request
// itself. Gated on the final resolved provider so it fires regardless of how the provider was
// picked (prefix, catalog, key alias, governance) and re-runs per attempt for fallbacks. No-op
// for Anthropic/Vertex/Azure providers and for non-Anthropic integrations.
func clearAnthropicPassthroughForNonNativeProvider(ctx *schemas.UnifAIContext, baseProvider schemas.ModelProvider) {
	if integrationType, _ := ctx.Value(schemas.UnifAIContextKeyIntegrationType).(string); integrationType != "anthropic" {
		return
	}
	if baseProvider == schemas.Anthropic ||
		baseProvider == schemas.Vertex ||
		baseProvider == schemas.Azure ||
		baseProvider == schemas.BedrockMantle {
		return
	}
	ctx.SetValue(schemas.UnifAIContextKeyUseRawRequestBody, false)
	ctx.SetValue(schemas.UnifAIContextKeySendBackRawResponse, false)
	ctx.SetValue(schemas.UnifAIContextKeyPassthroughOverridesPresent, false)
}

// requestWorker handles incoming requests from the queue for a specific provider.
// It manages retries, error handling, and response processing.
func (unifai *UnifAI) requestWorker(provider schemas.Provider, config *schemas.ProviderConfig, pq *ProviderQueue, waitGroup *sync.WaitGroup) {
	defer waitGroup.Done()

	for {
		var req *ChannelMessage
		select {
		case r := <-pq.queue:
			req = r
		case <-pq.done:
			// Provider is shutting down. Drain any buffered requests and send
			// back errors so callers are not left blocked on their response channel.
			for {
				select {
				case r := <-pq.queue:
					provKey, mod, _ := r.GetRequestFields()
					select {
					case r.Err <- schemas.UnifAIError{
						IsUnifAIError: false,
						Error: &schemas.ErrorField{
							Message: "provider is shutting down",
						},
						ExtraFields: schemas.UnifAIErrorExtraFields{
							RequestType:            r.RequestType,
							Provider:               provKey,
							OriginalModelRequested: mod,
						},
					}:
					case <-r.Context.Done():
					}
				default:
					return
				}
			}
		}

		_, model, _ := req.UnifAIRequest.GetRequestFields()

		var result *schemas.UnifAIResponse
		var stream chan *schemas.UnifAIStreamChunk
		var unifaiError *schemas.UnifAIError
		var err error

		// Determine the base provider type for key requirement checks
		baseProvider := provider.GetProviderKey()
		if cfg := config.CustomProviderConfig; cfg != nil && cfg.BaseProviderType != "" {
			baseProvider = cfg.BaseProviderType
		}
		req.Context.SetValue(schemas.UnifAIContextKeyIsCustomProvider, !IsStandardProvider(baseProvider))

		// Disable Anthropic raw-body passthrough when this attempt's provider isn't Anthropic-native (e.g. Bedrock).
		clearAnthropicPassthroughForNonNativeProvider(req.Context, baseProvider)

		// Determine whether this provider attempt should capture raw payloads.
		//
		// Effective values are computed by merging provider config with any per-request
		// context overrides (UnifAIContextKeySendBackRawRequest/Response and
		// UnifAIContextKeyStoreRawRequestResponse). A context value set to either true
		// or false fully overrides the provider config for that flag.
		//
		// Each flag is independent:
		//   send_back_raw_request  — include raw request bytes in the client response.
		//   send_back_raw_response — include raw response bytes in the client response.
		//   store_raw_request_response — persist raw bytes in log records (logging plugin only).
		//
		// Capture is enabled per-side whenever send-back OR store is requested for that side.
		// Strip flags tell the response path to remove that side's bytes before the payload
		// reaches the caller (used when store=true but send-back=false for that side).
		//
		// All internal signals are always written explicitly on every attempt so stale values
		// from a previous provider attempt (e.g. different fallback provider config) cannot
		// leak into the new attempt on a reused context. The user override keys
		// (UnifAIContextKeySendBackRaw*, UnifAIContextKeyStoreRawRequestResponse) are
		// never overwritten — they are read-only from unifai.go's perspective.

		// Step 1: compute effective value for each flag (provider config ← per-request override).
		effectiveSendBackReq := config.SendBackRawRequest
		allowRawOverride, _ := req.Context.Value(schemas.UnifAIContextKeyAllowPerRequestRawOverride).(bool)
		passthroughOverridePresent, _ := req.Context.Value(schemas.UnifAIContextKeyPassthroughOverridesPresent).(bool)

		if allowRawOverride || passthroughOverridePresent {
			if override, ok := req.Context.Value(schemas.UnifAIContextKeySendBackRawRequest).(bool); ok {
				effectiveSendBackReq = override
			}
		}
		effectiveSendBackResp := config.SendBackRawResponse
		if allowRawOverride || passthroughOverridePresent {
			if override, ok := req.Context.Value(schemas.UnifAIContextKeySendBackRawResponse).(bool); ok {
				effectiveSendBackResp = override
			}
		}
		effectiveStore := config.StoreRawRequestResponse
		allowStorageOverride, _ := req.Context.Value(schemas.UnifAIContextKeyAllowPerRequestStorageOverride).(bool)
		if allowStorageOverride {
			if override, ok := req.Context.Value(schemas.UnifAIContextKeyStoreRawRequestResponse).(bool); ok {
				effectiveStore = override
			}
		}

		// Step 2: derive per-side capture and strip flags.
		// Capture if we need to send the data back OR store it — independent per side.
		captureReq := effectiveSendBackReq || effectiveStore
		captureResp := effectiveSendBackResp || effectiveStore
		// Strip from client response if we captured for storage but not for send-back.
		dropReq := effectiveStore && !effectiveSendBackReq
		dropResp := effectiveStore && !effectiveSendBackResp

		// Step 3: write all internal signals explicitly (never touch the user override keys).
		req.Context.SetValue(schemas.UnifAIContextKeyCaptureRawRequest, captureReq)
		req.Context.SetValue(schemas.UnifAIContextKeyCaptureRawResponse, captureResp)
		req.Context.SetValue(schemas.UnifAIContextKeyDropRawRequestFromClient, dropReq)
		req.Context.SetValue(schemas.UnifAIContextKeyDropRawResponseFromClient, dropResp)
		// Tells the logging plugin whether to persist raw bytes in log records.
		req.Context.SetValue(schemas.UnifAIContextKeyShouldStoreRawInLogs, effectiveStore)

		var keys []schemas.Key
		// keyProvider is passed to executeRequestWithRetries to manage key selection and rotation.
		// It is nil when no key is required (e.g. providerRequiresKey=false) or for multi-key
		// batch/file/container operations that manage their own key lists.
		var keyProvider func(usedKeyIDs, deadKeyIDs map[string]bool) (schemas.Key, error)

		if providerRequiresKey(config.CustomProviderConfig) {
			// ListModels needs all enabled/supported keys so providers can aggregate
			// and report per-key statuses (KeyStatuses).
			if req.RequestType == schemas.ListModelsRequest {
				keys, err = unifai.getAllSupportedKeys(req.Context, provider.GetProviderKey(), baseProvider)
				if err != nil {
					unifai.logger.Debug("error getting supported keys for list models: %v", err)
					req.Err <- schemas.UnifAIError{
						IsUnifAIError: false,
						Error: &schemas.ErrorField{
							Message: err.Error(),
							Error:   err,
						},
						ExtraFields: schemas.UnifAIErrorExtraFields{
							Provider:               provider.GetProviderKey(),
							RequestType:            req.RequestType,
							OriginalModelRequested: model,
							ResolvedModelUsed:      model,
						},
					}
					continue
				}
				// Scope to a single key when ListModelsRequest.KeyID is set, so
				// callers (the catalog composer) can cache per-key without an
				// extra round-trip and without the provider aggregating across
				// every configured key.
				if lmr := req.UnifAIRequest.ListModelsRequest; lmr != nil && lmr.KeyID != nil {
					target := *lmr.KeyID
					keys = filterKeysByID(keys, target)
					if len(keys) == 0 {
						req.Err <- schemas.UnifAIError{
							IsUnifAIError: false,
							Error: &schemas.ErrorField{
								Message: fmt.Sprintf("no key found with id %q for provider %s", target, provider.GetProviderKey()),
							},
							ExtraFields: schemas.UnifAIErrorExtraFields{
								Provider:               provider.GetProviderKey(),
								RequestType:            req.RequestType,
								OriginalModelRequested: model,
								ResolvedModelUsed:      model,
							},
						}
						continue
					}
				}
			} else {
				// Determine if this is a multi-key batch/file/container operation
				// BatchCreate, FileUpload, ContainerCreate, ContainerFileCreate use single key; other batch/file/container ops use multiple keys
				isMultiKeyBatchOp := isBatchRequestType(req.RequestType) && req.RequestType != schemas.BatchCreateRequest
				isMultiKeyFileOp := isFileRequestType(req.RequestType) && req.RequestType != schemas.FileUploadRequest
				isMultiKeyContainerOp := isContainerRequestType(req.RequestType) && req.RequestType != schemas.ContainerCreateRequest && req.RequestType != schemas.ContainerFileCreateRequest
				isMultiKeyCachedContentOp := isCachedContentRequestType(req.RequestType) && req.RequestType != schemas.CachedContentCreateRequest

				if isMultiKeyBatchOp || isMultiKeyFileOp || isMultiKeyContainerOp || isMultiKeyCachedContentOp {
					var modelPtr *string
					if model != "" {
						modelPtr = &model
					}
					keys, err = unifai.getKeysForBatchAndFileOps(req.Context, provider.GetProviderKey(), baseProvider, modelPtr, isMultiKeyBatchOp)
					if err != nil {
						unifai.logger.Debug("error getting keys for batch/file operation: %v", err)
						req.Err <- schemas.UnifAIError{
							IsUnifAIError: false,
							Error: &schemas.ErrorField{
								Message: err.Error(),
								Error:   err,
							},
							ExtraFields: schemas.UnifAIErrorExtraFields{
								Provider:               provider.GetProviderKey(),
								RequestType:            req.RequestType,
								OriginalModelRequested: model,
								ResolvedModelUsed:      model,
							},
						}
						continue
					}
				} else {
					// Build the key pool for this request. Selection and rotation are deferred to
					// executeRequestWithRetries via keyProvider so that each retry attempt can use
					// a different key (on rate-limit errors) without re-running the full filtering.
					supportedKeys, canRotate, keyPoolErr := unifai.selectKeyFromProviderForModelWithPool(req.Context, req.RequestType, provider.GetProviderKey(), model, baseProvider)
					if keyPoolErr != nil {
						unifai.logger.Debug("error building key pool for model %s: %v", model, keyPoolErr)
						req.Err <- schemas.UnifAIError{
							IsUnifAIError: false,
							Error: &schemas.ErrorField{
								Message: keyPoolErr.Error(),
								Error:   keyPoolErr,
							},
							ExtraFields: schemas.UnifAIErrorExtraFields{
								Provider:               provider.GetProviderKey(),
								RequestType:            req.RequestType,
								OriginalModelRequested: model,
								ResolvedModelUsed:      model,
							},
						}
						continue
					}

					if len(supportedKeys) == 0 {
						// SkipKeySelection path — keyProvider stays nil, zero Key is used.
					} else if !canRotate {
						// Fixed key (explicit ID/name, session stickiness): always
						// return the same key — *unless* it has been marked permanently
						// dead this request, in which case surface errAllKeysDead so the
						// caller emits 502 upstream_credentials_exhausted instead of
						// burning the remaining retries on the same bad credential.
						fixedKey := supportedKeys[0]
						keyProvider = func(_, deadKeyIDs map[string]bool) (schemas.Key, error) {
							if deadKeyIDs[fixedKey.ID] {
								return schemas.Key{}, errAllKeysDead
							}
							return fixedKey, nil
						}
					} else {
						// Rotating pool: weighted selection with per-cycle exclusion.
						// Captures supportedKeys, unifai.keySelector, provider/model by value.
						pool := supportedKeys
						provKey := provider.GetProviderKey()
						mdl := model
						keyProvider = func(usedKeyIDs, deadKeyIDs map[string]bool) (schemas.Key, error) {
							available := make([]schemas.Key, 0, len(pool))
							for _, k := range pool {
								if deadKeyIDs[k.ID] || usedKeyIDs[k.ID] {
									continue
								}
								available = append(available, k)
							}
							if unifai.keyPoolFilter != nil {
								if filtered, err := unifai.keyPoolFilter(req.Context, provKey, mdl, available); err != nil {
									unifai.logger.Warn("key pool filter failed for provider %s, using unfiltered keys: %v", provKey, err)
								} else {
									available = filtered
								}
							}
							if len(available) == 0 {
								// No non-dead keys remain in this cycle. If every key has been
								// marked permanently dead, give up — retrying won't help.
								// Otherwise reset usedKeyIDs and start a fresh weighted round
								// across the still-live (non-dead) keys; a previously
								// rate-limited key may have free quota by now.
								for _, k := range pool {
									if !deadKeyIDs[k.ID] {
										available = append(available, k)
									}
								}
								liveCount := len(available) // non-dead keys before the filter runs
								if unifai.keyPoolFilter != nil {
									if filtered, err := unifai.keyPoolFilter(req.Context, provKey, mdl, available); err != nil {
										unifai.logger.Warn("key pool filter failed for provider %s, using unfiltered keys: %v", provKey, err)
									} else {
										available = filtered
									}
								}
								if len(available) == 0 {
									if liveCount > 0 {
										return schemas.Key{}, fmt.Errorf("%w: provider %s", errAllKeysFiltered, provKey)
									}
									return schemas.Key{}, fmt.Errorf("%w: provider %s", errAllKeysDead, provKey)
								}
								for id := range usedKeyIDs {
									delete(usedKeyIDs, id)
								}
							}
							return unifai.keySelector(req.Context, available, provKey, mdl)
						}
					}
				}
			}
		}

		originalModelRequested := model
		// resolvedModel is set inside the handler closures below on every attempt so that each
		// key's own alias mapping is applied. The outer var holds the LAST attempt's value and is
		// read single-threaded by the worker after retries finish (e.g. the error-fallback at
		// line 5653). Streaming postHookRunner must NOT capture this var by reference — it
		// snapshots its own attemptResolvedModel inside the per-attempt closure.
		var resolvedModel string
		// attemptRoutingInfo holds the LAST attempt's RoutingInfo. Same single-writer/
		// single-reader contract as resolvedModel — assigned inside the per-attempt
		// closure, read after retries finish by the post-retry populate below.
		// Streaming postHookRunner must NOT capture by reference — it snapshots its
		// own copy inside the per-attempt closure.
		// Pre-seeded with the provider/model the orchestrator already knows so that
		// retries that fail before the per-attempt closure ever runs (e.g. key
		// selection error) still produce a populated RoutingInfo on the error —
		// otherwise the post-retry populate at line ~6221 would clobber RoutingInfo
		// to a zero value, leaving new consumers without provider/model context.
		attemptRoutingInfo := schemas.RoutingInfo{
			Provider: provider.GetProviderKey(),
			Model:    originalModelRequested,
		}
		// lastAttemptFinalizer captures the LAST attempt's postHookSpanFinalizer for the
		// worker-level error fallback below. Single-threaded write (assigned by the retry
		// loop's per-attempt closure) and single-threaded read (after retries finish), so
		// no synchronization needed. Earlier attempts' finalizers fire via their provider
		// goroutines' defers — passed via the postHookSpanFinalizer parameter directly to
		// handleProviderStreamRequest, never via the shared req.Context.
		var lastAttemptFinalizer func(context.Context)

		// Execute request with retries. For streaming, the plugin pipeline,
		// postHookRunner, and finalizer are allocated per-attempt inside the
		// request handler closure. If they were request-scoped, a retry
		// triggered by CheckFirstStreamChunkForError could run against a
		// pipeline the previous attempt's provider goroutine has already
		// returned to the pool via its deferred finalizer.
		if IsStreamRequestType(req.RequestType) {
			stream, unifaiError = executeRequestWithRetries(req.Context, config, func(k schemas.Key) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
				if aliasConfig := k.Aliases.ResolveConfig(originalModelRequested); aliasConfig != nil {
					resolvedModel = aliasConfig.ModelID
					req.Context.SetValue(schemas.UnifAIContextKeyResolvedAlias, &schemas.ResolvedAlias{Key: originalModelRequested, Config: aliasConfig})
				} else {
					resolvedModel = originalModelRequested
					req.Context.SetValue(schemas.UnifAIContextKeyResolvedAlias, nil)
				}
				req.SetModel(resolvedModel)
				// Snapshot per-attempt so postHookRunner doesn't observe a later retry's
				// alias while this attempt's provider goroutine is still emitting chunks.
				attemptResolvedModel := resolvedModel
				attemptRoutingInfo = schemas.BuildRoutingInfo(req.Context, provider.GetProviderKey(), originalModelRequested, k)
				// Per-attempt snapshot for the async postHookRunner closure (it must
				// not capture the outer var by reference — a later retry would race).
				perAttemptRoutingInfo := attemptRoutingInfo
				// Snapshot RequestType before the closure. After tryStreamRequest receives
				// the stream channel it releases the *ChannelMessage back to the pool;
				// a concurrent request can then reuse it and overwrite RequestType.
				// Reading req.RequestType inside the closure would observe the new request's type.
				attemptRequestType := req.RequestType
				pipeline := unifai.getPluginPipeline()
				postHookRunner := func(ctx *schemas.UnifAIContext, result *schemas.UnifAIResponse, err *schemas.UnifAIError) (*schemas.UnifAIResponse, *schemas.UnifAIError) {
					// Populate extra fields before RunPostLLMHooks so plugins (e.g. logging)
					// can read requestType/provider/model from the chunk or error.
					// Uses the per-attempt snapshot — capturing the outer resolvedModel by
					// reference would let a later retry's alias bleed into this attempt's chunks.
					if result != nil {
						result.PopulateExtraFields(attemptRequestType, provider.GetProviderKey(), originalModelRequested, attemptResolvedModel)
						result.PopulateRoutingInfo(perAttemptRoutingInfo)
					}
					if err != nil {
						err.PopulateExtraFields(attemptRequestType, provider.GetProviderKey(), originalModelRequested, attemptResolvedModel)
						err.PopulateRoutingInfo(perAttemptRoutingInfo)
					}
					resp, unifaiErr := pipeline.RunPostLLMHooks(ctx, result, err, len(*unifai.llmPlugins.Load()))
					if IsFinalChunk(ctx) {
						drainAndAttachPluginLogs(ctx)
					}
					if unifaiErr != nil {
						unifaiErr.PopulateExtraFields(attemptRequestType, provider.GetProviderKey(), originalModelRequested, attemptResolvedModel)
						unifaiErr.PopulateRoutingInfo(perAttemptRoutingInfo)
						return nil, unifaiErr
					} else if resp != nil {
						resp.PopulateExtraFields(attemptRequestType, provider.GetProviderKey(), originalModelRequested, attemptResolvedModel)
						resp.PopulateRoutingInfo(perAttemptRoutingInfo)
					}
					return resp, nil
				}
				// Store a finalizer callback to create aggregated post-hook spans at stream end.
				// Wrapped in sync.Once so the normal end-of-stream invocation and a deferred
				// safety-net invocation (e.g. from a provider goroutine's panic path) cannot
				// double-release the pipeline.
				var finalizerOnce sync.Once
				postHookSpanFinalizer := func(ctx context.Context) {
					finalizerOnce.Do(func() {
						pipeline.FinalizeStreamingPostHookSpans(ctx)
						unifai.releasePluginPipeline(pipeline)
					})
				}
				lastAttemptFinalizer = postHookSpanFinalizer
				streamCh, streamErr := unifai.handleProviderStreamRequest(provider, req, k, postHookRunner, postHookSpanFinalizer)
				// If stream setup failed before any provider goroutine started,
				// no deferred finalizer will run — release the pipeline directly
				// so a retry doesn't inherit a leaked pool entry.
				if streamErr != nil && streamCh == nil {
					finalizerOnce.Do(func() {
						unifai.releasePluginPipeline(pipeline)
					})
				}
				return streamCh, streamErr
			}, keyProvider, req.RequestType, provider.GetProviderKey(), model, &req.UnifAIRequest, unifai.logger)
		} else {
			result, unifaiError = executeRequestWithRetries(req.Context, config, func(k schemas.Key) (*schemas.UnifAIResponse, *schemas.UnifAIError) {
				if aliasConfig := k.Aliases.ResolveConfig(originalModelRequested); aliasConfig != nil {
					resolvedModel = aliasConfig.ModelID
					req.Context.SetValue(schemas.UnifAIContextKeyResolvedAlias, &schemas.ResolvedAlias{Key: originalModelRequested, Config: aliasConfig})
				} else {
					resolvedModel = originalModelRequested
					req.Context.SetValue(schemas.UnifAIContextKeyResolvedAlias, nil)
				}
				req.SetModel(resolvedModel)
				attemptRoutingInfo = schemas.BuildRoutingInfo(req.Context, provider.GetProviderKey(), originalModelRequested, k)
				return unifai.handleProviderRequest(provider, config, req, k, keys)
			}, keyProvider, req.RequestType, provider.GetProviderKey(), model, &req.UnifAIRequest, unifai.logger)
		}

		// For streaming with an error, route release through the LAST attempt's
		// finalizer (wrapped in sync.Once) so we don't double-Put into the pool
		// or race the provider goroutine's deferred FinalizeStreamingPostHookSpans
		// call. lastAttemptFinalizer is set inside the per-attempt closure on every
		// iteration; after retries finish, it holds the LAST attempt's finalizer.
		// Earlier attempts' finalizers have already fired via their provider
		// goroutines' defers (passed via the postHookSpanFinalizer parameter
		// directly to handleProviderStreamRequest). For streaming without error,
		// the finalizer is invoked by completeDeferredSpan / the provider
		// goroutine's defer.
		if IsStreamRequestType(req.RequestType) && unifaiError != nil {
			if lastAttemptFinalizer != nil {
				lastAttemptFinalizer(req.Context)
			}
		}

		if unifaiError != nil {
			unifaiError.PopulateExtraFields(req.RequestType, provider.GetProviderKey(), originalModelRequested, resolvedModel)
			unifaiError.PopulateRoutingInfo(attemptRoutingInfo)

			// Send error with context awareness to prevent deadlock
			select {
			case req.Err <- *unifaiError:
				// Error sent successfully
			case <-req.Context.Done():
				// Client no longer listening, log and continue
				unifai.logger.Debug("Client context cancelled while sending error response")
				// The provider already produced this error (possibly after
				// processing input tokens). tryRequest returned on ctx.Done and will
				// never receive it, so bill/log it here. Non-streaming only.
				unifai.billAbandonedTerminal(req, nil, unifaiError)
			case <-time.After(5 * time.Second):
				// Timeout to prevent indefinite blocking
				unifai.logger.Warn("Timeout while sending error response, client may have disconnected")
			}
		} else {
			if result != nil {
				result.PopulateExtraFields(req.RequestType, provider.GetProviderKey(), originalModelRequested, resolvedModel)
				result.PopulateRoutingInfo(attemptRoutingInfo)
			}
			if IsStreamRequestType(req.RequestType) {
				// Send stream with context awareness to prevent deadlock
				select {
				case req.ResponseStream <- stream:
					// Stream sent successfully
				case <-req.Context.Done():
					// Client no longer listening, log and continue
					unifai.logger.Debug("Client context cancelled while sending stream response")
				case <-time.After(5 * time.Second):
					// Timeout to prevent indefinite blocking
					unifai.logger.Warn("Timeout while sending stream response, client may have disconnected")
				}
			} else {
				// Send response with context awareness to prevent deadlock
				select {
				case req.Response <- result:
					// Response sent successfully
				case <-req.Context.Done():
					// Client no longer listening, log and continue
					unifai.logger.Debug("Client context cancelled while sending response")
					// The provider already produced this non-streaming result
					// (consuming tokens). tryRequest returned on ctx.Done and will never
					// receive it, so bill/log it here.
					unifai.billAbandonedTerminal(req, result, nil)
				case <-time.After(5 * time.Second):
					// Timeout to prevent indefinite blocking
					unifai.logger.Warn("Timeout while sending response, client may have disconnected")
				}
			}
		}
	}

	// unifai.logger.Debug("worker for provider %s exiting...", provider.GetProviderKey())
}

// billAbandonedTerminal runs terminal post-LLM hooks for a NON-STREAMING request
// whose client stopped waiting (tryRequest returned on ctx.Done) after the
// provider had already produced a result or error. Without this, tokens the
// provider consumed are never billed or logged (non-streaming
// cancellation). Safety:
//   - The channel rendezvous guarantees tryRequest did NOT also receive this
//     value (a value cannot be both delivered and land in the no-receiver
//     ctx.Done branch), so post-hooks never run twice for one call.
//   - The governance tracker additionally dedupes on RequestID+attempt.
//   - Streaming requests are excluded: their provider goroutine already runs
//     terminal post-hooks via HandleStreamCancellation.
func (unifai *UnifAI) billAbandonedTerminal(req *ChannelMessage, result *schemas.UnifAIResponse, unifaiErr *schemas.UnifAIError) {
	if req == nil || req.Context == nil || IsStreamRequestType(req.RequestType) {
		return
	}
	pipeline := unifai.getPluginPipeline()
	defer unifai.releasePluginPipeline(pipeline)
	pluginCount := len(*unifai.llmPlugins.Load())
	if unifaiErr != nil {
		_, _ = pipeline.RunPostLLMHooks(req.Context, nil, unifaiErr, pluginCount)
	} else if result != nil {
		_, _ = pipeline.RunPostLLMHooks(req.Context, result, nil, pluginCount)
	}
	drainAndAttachPluginLogs(req.Context)
}

// handleProviderRequest handles the request to the provider based on the request type
// key is used for single-key operations, keys is used for batch/file operations that need multiple keys
func (unifai *UnifAI) handleProviderRequest(provider schemas.Provider, config *schemas.ProviderConfig, req *ChannelMessage, key schemas.Key, keys []schemas.Key) (*schemas.UnifAIResponse, *schemas.UnifAIError) {
	response := &schemas.UnifAIResponse{}
	switch req.RequestType {
	case schemas.ListModelsRequest:
		listModelsResponse, unifaiError := provider.ListModels(req.Context, keys, req.UnifAIRequest.ListModelsRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.ListModelsResponse = listModelsResponse
	case schemas.TextCompletionRequest:
		if changeType, ok := req.Context.Value(schemas.UnifAIContextKeyChangeRequestType).(schemas.RequestType); ok && changeType == schemas.ChatCompletionRequest {
			chatRequest := req.UnifAIRequest.TextCompletionRequest.ToUnifAIChatRequest()
			if chatRequest != nil {
				chatCompletionResponse, unifaiError := provider.ChatCompletion(req.Context, key, chatRequest)
				if unifaiError != nil {
					return nil, unifaiError
				}
				response.TextCompletionResponse = chatCompletionResponse.ToUnifAITextCompletionResponse()
				break
			}
		}
		textCompletionResponse, unifaiError := provider.TextCompletion(req.Context, key, req.UnifAIRequest.TextCompletionRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.TextCompletionResponse = textCompletionResponse
	case schemas.ChatCompletionRequest:
		if changeType, ok := req.Context.Value(schemas.UnifAIContextKeyChangeRequestType).(schemas.RequestType); ok && changeType == schemas.ResponsesRequest {
			responsesRequest := req.UnifAIRequest.ChatRequest.ToResponsesRequest()
			if responsesRequest != nil {
				responsesResponse, unifaiError := provider.Responses(req.Context, key, responsesRequest)
				if unifaiError != nil {
					return nil, unifaiError
				}
				response.ChatResponse = responsesResponse.ToUnifAIChatResponse()
				break
			}
		}
		chatCompletionResponse, unifaiError := provider.ChatCompletion(req.Context, key, req.UnifAIRequest.ChatRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		chatCompletionResponse.BackfillParams(req.UnifAIRequest.ChatRequest)
		response.ChatResponse = chatCompletionResponse
	case schemas.ResponsesRequest:
		responsesResponse, unifaiError := provider.Responses(req.Context, key, req.UnifAIRequest.ResponsesRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		responsesResponse.BackfillParams(req.UnifAIRequest.ResponsesRequest)
		response.ResponsesResponse = responsesResponse
	case schemas.CountTokensRequest:
		countTokensResponse, unifaiError := provider.CountTokens(req.Context, key, req.UnifAIRequest.CountTokensRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.CountTokensResponse = countTokensResponse
	case schemas.ResponsesRetrieveRequest:
		lifecycle, ok := provider.(schemas.ResponsesLifecycleProvider)
		if !ok {
			return nil, providerUtils.NewUnsupportedOperationError(schemas.ResponsesRetrieveRequest, provider.GetProviderKey())
		}
		retrieveResp, unifaiError := lifecycle.ResponsesRetrieve(req.Context, key, req.UnifAIRequest.ResponsesRetrieveRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.ResponsesResponse = retrieveResp
	case schemas.ResponsesDeleteRequest:
		lifecycle, ok := provider.(schemas.ResponsesLifecycleProvider)
		if !ok {
			return nil, providerUtils.NewUnsupportedOperationError(schemas.ResponsesDeleteRequest, provider.GetProviderKey())
		}
		deleteResp, unifaiError := lifecycle.ResponsesDelete(req.Context, key, req.UnifAIRequest.ResponsesDeleteRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.ResponsesDeleteResponse = deleteResp
	case schemas.ResponsesCancelRequest:
		lifecycle, ok := provider.(schemas.ResponsesLifecycleProvider)
		if !ok {
			return nil, providerUtils.NewUnsupportedOperationError(schemas.ResponsesCancelRequest, provider.GetProviderKey())
		}
		cancelResp, unifaiError := lifecycle.ResponsesCancel(req.Context, key, req.UnifAIRequest.ResponsesCancelRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.ResponsesResponse = cancelResp
	case schemas.ResponsesInputItemsRequest:
		lifecycle, ok := provider.(schemas.ResponsesLifecycleProvider)
		if !ok {
			return nil, providerUtils.NewUnsupportedOperationError(schemas.ResponsesInputItemsRequest, provider.GetProviderKey())
		}
		itemsResp, unifaiError := lifecycle.ResponsesInputItems(req.Context, key, req.UnifAIRequest.ResponsesInputItemsRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.ResponsesInputItemsResponse = itemsResp
	case schemas.CompactionRequest:
		compactionResponse, unifaiError := provider.Compaction(req.Context, key, req.UnifAIRequest.CompactionRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.CompactionResponse = compactionResponse
	case schemas.EmbeddingRequest:
		embeddingResponse, unifaiError := provider.Embedding(req.Context, key, req.UnifAIRequest.EmbeddingRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		embeddingResponse.BackfillParams(req.UnifAIRequest.EmbeddingRequest)
		response.EmbeddingResponse = embeddingResponse
	case schemas.RerankRequest:
		rerankResponse, unifaiError := provider.Rerank(req.Context, key, req.UnifAIRequest.RerankRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.RerankResponse = rerankResponse
	case schemas.OCRRequest:
		var customProviderConfig *schemas.CustomProviderConfig
		if config != nil {
			customProviderConfig = config.CustomProviderConfig
		}
		if unifaiError := providerUtils.CheckOperationAllowed(provider.GetProviderKey(), customProviderConfig, schemas.OCRRequest); unifaiError != nil {
			if req.UnifAIRequest.OCRRequest != nil {
				unifaiError.ExtraFields.OriginalModelRequested = req.UnifAIRequest.OCRRequest.Model
			}
			return nil, unifaiError
		}
		ocrResponse, unifaiError := provider.OCR(req.Context, key, req.UnifAIRequest.OCRRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.OCRResponse = ocrResponse
	case schemas.SpeechRequest:
		speechResponse, unifaiError := provider.Speech(req.Context, key, req.UnifAIRequest.SpeechRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		speechResponse.BackfillParams(req.UnifAIRequest.SpeechRequest)
		response.SpeechResponse = speechResponse
	case schemas.TranscriptionRequest:
		transcriptionResponse, unifaiError := provider.Transcription(req.Context, key, req.UnifAIRequest.TranscriptionRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		transcriptionResponse.BackfillParams(req.UnifAIRequest.TranscriptionRequest)
		response.TranscriptionResponse = transcriptionResponse
	case schemas.ImageGenerationRequest:
		imageResponse, unifaiError := provider.ImageGeneration(req.Context, key, req.UnifAIRequest.ImageGenerationRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		imageResponse.BackfillParams(&req.UnifAIRequest)
		response.ImageGenerationResponse = imageResponse
	case schemas.ImageEditRequest:
		imageEditResponse, unifaiError := provider.ImageEdit(req.Context, key, req.UnifAIRequest.ImageEditRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		imageEditResponse.BackfillParams(&req.UnifAIRequest)
		response.ImageGenerationResponse = imageEditResponse
	case schemas.ImageVariationRequest:
		imageVariationResponse, unifaiError := provider.ImageVariation(req.Context, key, req.UnifAIRequest.ImageVariationRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		imageVariationResponse.BackfillParams(&req.UnifAIRequest)
		response.ImageGenerationResponse = imageVariationResponse
	case schemas.VideoGenerationRequest:
		videoGenerationResponse, unifaiError := provider.VideoGeneration(req.Context, key, req.UnifAIRequest.VideoGenerationRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		videoGenerationResponse.BackfillParams(&req.UnifAIRequest)
		response.VideoGenerationResponse = videoGenerationResponse
	case schemas.VideoRetrieveRequest:
		videoRetrieveResponse, unifaiError := provider.VideoRetrieve(req.Context, key, req.UnifAIRequest.VideoRetrieveRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.VideoGenerationResponse = videoRetrieveResponse
	case schemas.VideoDownloadRequest:
		videoDownloadResponse, unifaiError := provider.VideoDownload(req.Context, key, req.UnifAIRequest.VideoDownloadRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.VideoDownloadResponse = videoDownloadResponse
	case schemas.VideoListRequest:
		videoListResponse, unifaiError := provider.VideoList(req.Context, key, req.UnifAIRequest.VideoListRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.VideoListResponse = videoListResponse
	case schemas.VideoDeleteRequest:
		videoDeleteResponse, unifaiError := provider.VideoDelete(req.Context, key, req.UnifAIRequest.VideoDeleteRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.VideoDeleteResponse = videoDeleteResponse
	case schemas.VideoRemixRequest:
		videoRemixResponse, unifaiError := provider.VideoRemix(req.Context, key, req.UnifAIRequest.VideoRemixRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.VideoGenerationResponse = videoRemixResponse
	case schemas.FileUploadRequest:
		fileUploadResponse, unifaiError := provider.FileUpload(req.Context, key, req.UnifAIRequest.FileUploadRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.FileUploadResponse = fileUploadResponse
	case schemas.FileListRequest:
		fileListResponse, unifaiError := provider.FileList(req.Context, keys, req.UnifAIRequest.FileListRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.FileListResponse = fileListResponse
	case schemas.FileRetrieveRequest:
		fileRetrieveResponse, unifaiError := provider.FileRetrieve(req.Context, keys, req.UnifAIRequest.FileRetrieveRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.FileRetrieveResponse = fileRetrieveResponse
	case schemas.FileDeleteRequest:
		fileDeleteResponse, unifaiError := provider.FileDelete(req.Context, keys, req.UnifAIRequest.FileDeleteRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.FileDeleteResponse = fileDeleteResponse
	case schemas.FileContentRequest:
		fileContentResponse, unifaiError := provider.FileContent(req.Context, keys, req.UnifAIRequest.FileContentRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.FileContentResponse = fileContentResponse
	case schemas.CachedContentCreateRequest:
		cachedContentCreateResponse, unifaiError := provider.CachedContentCreate(req.Context, key, req.UnifAIRequest.CachedContentCreateRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.CachedContentCreateResponse = cachedContentCreateResponse
	case schemas.CachedContentListRequest:
		cachedContentListResponse, unifaiError := provider.CachedContentList(req.Context, keys, req.UnifAIRequest.CachedContentListRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.CachedContentListResponse = cachedContentListResponse
	case schemas.CachedContentRetrieveRequest:
		cachedContentRetrieveResponse, unifaiError := provider.CachedContentRetrieve(req.Context, keys, req.UnifAIRequest.CachedContentRetrieveRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.CachedContentRetrieveResponse = cachedContentRetrieveResponse
	case schemas.CachedContentUpdateRequest:
		cachedContentUpdateResponse, unifaiError := provider.CachedContentUpdate(req.Context, keys, req.UnifAIRequest.CachedContentUpdateRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.CachedContentUpdateResponse = cachedContentUpdateResponse
	case schemas.CachedContentDeleteRequest:
		cachedContentDeleteResponse, unifaiError := provider.CachedContentDelete(req.Context, keys, req.UnifAIRequest.CachedContentDeleteRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.CachedContentDeleteResponse = cachedContentDeleteResponse
	case schemas.BatchCreateRequest:
		batchCreateResponse, unifaiError := provider.BatchCreate(req.Context, key, req.UnifAIRequest.BatchCreateRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.BatchCreateResponse = batchCreateResponse
	case schemas.BatchListRequest:
		batchListResponse, unifaiError := provider.BatchList(req.Context, keys, req.UnifAIRequest.BatchListRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.BatchListResponse = batchListResponse
	case schemas.BatchRetrieveRequest:
		batchRetrieveResponse, unifaiError := provider.BatchRetrieve(req.Context, keys, req.UnifAIRequest.BatchRetrieveRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.BatchRetrieveResponse = batchRetrieveResponse
	case schemas.BatchCancelRequest:
		batchCancelResponse, unifaiError := provider.BatchCancel(req.Context, keys, req.UnifAIRequest.BatchCancelRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.BatchCancelResponse = batchCancelResponse
	case schemas.BatchDeleteRequest:
		batchDeleteResponse, unifaiError := provider.BatchDelete(req.Context, keys, req.UnifAIRequest.BatchDeleteRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.BatchDeleteResponse = batchDeleteResponse
	case schemas.BatchResultsRequest:
		batchResultsResponse, unifaiError := provider.BatchResults(req.Context, keys, req.UnifAIRequest.BatchResultsRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.BatchResultsResponse = batchResultsResponse
	case schemas.ContainerCreateRequest:
		containerCreateResponse, unifaiError := provider.ContainerCreate(req.Context, key, req.UnifAIRequest.ContainerCreateRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.ContainerCreateResponse = containerCreateResponse
	case schemas.ContainerListRequest:
		containerListResponse, unifaiError := provider.ContainerList(req.Context, keys, req.UnifAIRequest.ContainerListRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.ContainerListResponse = containerListResponse
	case schemas.ContainerRetrieveRequest:
		containerRetrieveResponse, unifaiError := provider.ContainerRetrieve(req.Context, keys, req.UnifAIRequest.ContainerRetrieveRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.ContainerRetrieveResponse = containerRetrieveResponse
	case schemas.ContainerDeleteRequest:
		containerDeleteResponse, unifaiError := provider.ContainerDelete(req.Context, keys, req.UnifAIRequest.ContainerDeleteRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.ContainerDeleteResponse = containerDeleteResponse
	case schemas.ContainerFileCreateRequest:
		containerFileCreateResponse, unifaiError := provider.ContainerFileCreate(req.Context, key, req.UnifAIRequest.ContainerFileCreateRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.ContainerFileCreateResponse = containerFileCreateResponse
	case schemas.ContainerFileListRequest:
		containerFileListResponse, unifaiError := provider.ContainerFileList(req.Context, keys, req.UnifAIRequest.ContainerFileListRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.ContainerFileListResponse = containerFileListResponse
	case schemas.ContainerFileRetrieveRequest:
		containerFileRetrieveResponse, unifaiError := provider.ContainerFileRetrieve(req.Context, keys, req.UnifAIRequest.ContainerFileRetrieveRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.ContainerFileRetrieveResponse = containerFileRetrieveResponse
	case schemas.ContainerFileContentRequest:
		containerFileContentResponse, unifaiError := provider.ContainerFileContent(req.Context, keys, req.UnifAIRequest.ContainerFileContentRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.ContainerFileContentResponse = containerFileContentResponse
	case schemas.ContainerFileDeleteRequest:
		containerFileDeleteResponse, unifaiError := provider.ContainerFileDelete(req.Context, keys, req.UnifAIRequest.ContainerFileDeleteRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		response.ContainerFileDeleteResponse = containerFileDeleteResponse
	case schemas.PassthroughRequest:
		passthroughResponse, unifaiError := provider.Passthrough(req.Context, key, req.UnifAIRequest.PassthroughRequest)
		if unifaiError != nil {
			return nil, unifaiError
		}
		if passthroughResponse != nil {
			passthroughResponse.Path = req.UnifAIRequest.PassthroughRequest.Path
		}
		response.PassthroughResponse = passthroughResponse
	default:
		_, model, _ := req.UnifAIRequest.GetRequestFields()
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: fmt.Sprintf("unsupported request type: %s", req.RequestType),
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            req.RequestType,
				Provider:               provider.GetProviderKey(),
				OriginalModelRequested: model,
				ResolvedModelUsed:      model,
			},
		}
	}
	return response, nil
}

// handleProviderStreamRequest handles the stream request to the provider based on the request type
func (unifai *UnifAI) handleProviderStreamRequest(provider schemas.Provider, req *ChannelMessage, key schemas.Key, postHookRunner schemas.PostHookRunner, postHookSpanFinalizer func(context.Context)) (chan *schemas.UnifAIStreamChunk, *schemas.UnifAIError) {
	switch req.RequestType {
	case schemas.TextCompletionStreamRequest:
		if changeType, ok := req.Context.Value(schemas.UnifAIContextKeyChangeRequestType).(schemas.RequestType); ok && changeType == schemas.ChatCompletionRequest {
			chatRequest := req.UnifAIRequest.TextCompletionRequest.ToUnifAIChatRequest()
			if chatRequest != nil {
				return provider.ChatCompletionStream(req.Context, wrapConvertedStreamPostHookRunner(postHookRunner, schemas.ChatCompletionRequest), postHookSpanFinalizer, key, chatRequest)
			}
		}
		return provider.TextCompletionStream(req.Context, postHookRunner, postHookSpanFinalizer, key, req.UnifAIRequest.TextCompletionRequest)
	case schemas.ChatCompletionStreamRequest:
		if changeType, ok := req.Context.Value(schemas.UnifAIContextKeyChangeRequestType).(schemas.RequestType); ok && changeType == schemas.ResponsesRequest {
			responsesRequest := req.UnifAIRequest.ChatRequest.ToResponsesRequest()
			if responsesRequest != nil {
				return provider.ResponsesStream(req.Context, wrapConvertedStreamPostHookRunner(postHookRunner, schemas.ResponsesRequest), postHookSpanFinalizer, key, responsesRequest)
			}
		}
		return provider.ChatCompletionStream(req.Context, postHookRunner, postHookSpanFinalizer, key, req.UnifAIRequest.ChatRequest)
	case schemas.ResponsesStreamRequest:
		return provider.ResponsesStream(req.Context, postHookRunner, postHookSpanFinalizer, key, req.UnifAIRequest.ResponsesRequest)
	case schemas.SpeechStreamRequest:
		return provider.SpeechStream(req.Context, postHookRunner, postHookSpanFinalizer, key, req.UnifAIRequest.SpeechRequest)
	case schemas.TranscriptionStreamRequest:
		return provider.TranscriptionStream(req.Context, postHookRunner, postHookSpanFinalizer, key, req.UnifAIRequest.TranscriptionRequest)
	case schemas.ImageGenerationStreamRequest:
		return provider.ImageGenerationStream(req.Context, postHookRunner, postHookSpanFinalizer, key, req.UnifAIRequest.ImageGenerationRequest)
	case schemas.ImageEditStreamRequest:
		return provider.ImageEditStream(req.Context, postHookRunner, postHookSpanFinalizer, key, req.UnifAIRequest.ImageEditRequest)
	case schemas.PassthroughStreamRequest:
		return provider.PassthroughStream(req.Context, postHookRunner, postHookSpanFinalizer, key, req.UnifAIRequest.PassthroughRequest)
	default:
		_, model, _ := req.UnifAIRequest.GetRequestFields()
		return nil, &schemas.UnifAIError{
			IsUnifAIError: false,
			Error: &schemas.ErrorField{
				Message: fmt.Sprintf("unsupported request type: %s", req.RequestType),
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RequestType:            req.RequestType,
				Provider:               provider.GetProviderKey(),
				OriginalModelRequested: model,
				ResolvedModelUsed:      model,
			},
		}
	}
}

// PLUGIN MANAGEMENT

// RunLLMPreHooks executes PreHooks in order, tracks how many ran, and returns the final request, any short-circuit decision, and the count.
func (p *PluginPipeline) RunLLMPreHooks(ctx *schemas.UnifAIContext, req *schemas.UnifAIRequest) (*schemas.UnifAIRequest, *schemas.LLMPluginShortCircuit, int) {
	// If the skip plugin pipeline flag is set, skip the plugin pipeline
	if skipPluginPipeline, ok := ctx.Value(schemas.UnifAIContextKeySkipPluginPipeline).(bool); ok && skipPluginPipeline {
		return req, nil, 0
	}
	var shortCircuit *schemas.LLMPluginShortCircuit
	var err error
	ctx.BlockRestrictedWrites()
	defer ctx.UnblockRestrictedWrites()
	for i, plugin := range p.llmPlugins {
		pluginName := plugin.GetName()
		p.logger.Debug("running pre-hook for plugin %s", pluginName)
		// Start span for this plugin's PreLLMHook
		spanCtx, handle := p.tracer.StartSpan(ctx, fmt.Sprintf("plugin.%s.prehook", sanitizeSpanName(pluginName)), schemas.SpanKindPlugin)
		// Update pluginCtx with span context for nested operations
		if spanCtx != nil {
			if spanID, ok := spanCtx.Value(schemas.UnifAIContextKeySpanID).(string); ok {
				ctx.SetValue(schemas.UnifAIContextKeySpanID, spanID)
			}
		}

		pluginCtx := ctx.WithPluginScope(&pluginName)
		req, shortCircuit, err = plugin.PreLLMHook(pluginCtx, req)
		pluginCtx.ReleasePluginScope()

		// End span with appropriate status
		if err != nil {
			p.tracer.SetAttribute(handle, "error", err.Error())
			p.tracer.EndSpan(handle, schemas.SpanStatusError, err.Error())
			p.preHookErrors = append(p.preHookErrors, err)
			p.logger.Warn("error in PreLLMHook for plugin %s: %s", pluginName, err.Error())
		} else if shortCircuit != nil {
			p.tracer.SetAttribute(handle, "short_circuit", true)
			p.tracer.EndSpan(handle, schemas.SpanStatusOk, "short-circuit")
		} else {
			p.tracer.EndSpan(handle, schemas.SpanStatusOk, "")
		}

		p.executedPreHooks = i + 1
		if shortCircuit != nil {
			return req, shortCircuit, p.executedPreHooks // short-circuit: only plugins up to and including i ran
		}
	}
	return req, nil, p.executedPreHooks
}

// RunPreRequestHooks executes PreRequestHook on each LLM plugin in registration order, once per
// top-level request. Plugins mutate req.Provider, req.Model, req.Fallbacks (and any other field
// they choose); mutations are committed to the shared *UnifAIRequest and observed by every
// subsequent plugin, the provider call, and every fallback attempt. There is no short-circuit
// and errors are non-blocking — same semantics as RunLLMPreHooks: errors are logged as warnings
// and accumulated in p.preHookErrors, then the pipeline continues to the next plugin. The empty-
// provider validation in handleRequest/handleStreamRequest catches the case where no plugin
// successfully resolved a provider.
//
// Per-request semantics: unlike PreLLMHook (which runs again on every fallback), PreRequestHook
// runs exactly once at the top of handleRequest/handleStreamRequest, before any fan-out.
func (p *PluginPipeline) RunPreRequestHooks(ctx *schemas.UnifAIContext, req *schemas.UnifAIRequest) {
	// If the skip plugin pipeline flag is set, skip the plugin pipeline
	if skipPluginPipeline, ok := ctx.Value(schemas.UnifAIContextKeySkipPluginPipeline).(bool); ok && skipPluginPipeline {
		return
	}
	ctx.BlockRestrictedWrites()
	for _, plugin := range p.llmPlugins {
		pluginName := plugin.GetName()
		p.logger.Debug("running pre-request hook for plugin %s", pluginName)
		spanCtx, handle := p.tracer.StartSpan(ctx, fmt.Sprintf("plugin.%s.prerequesthook", sanitizeSpanName(pluginName)), schemas.SpanKindPlugin)
		if spanCtx != nil {
			if spanID, ok := spanCtx.Value(schemas.UnifAIContextKeySpanID).(string); ok {
				ctx.SetValue(schemas.UnifAIContextKeySpanID, spanID)
			}
		}

		pluginCtx := ctx.WithPluginScope(&pluginName)
		err := plugin.PreRequestHook(pluginCtx, req)
		pluginCtx.ReleasePluginScope()

		if err != nil {
			p.tracer.SetAttribute(handle, "error", err.Error())
			p.tracer.EndSpan(handle, schemas.SpanStatusError, err.Error())
			p.preHookErrors = append(p.preHookErrors, err)
			p.logger.Warn("error in PreRequestHook for plugin %s: %s", pluginName, err.Error())
			continue
		}
		p.tracer.EndSpan(handle, schemas.SpanStatusOk, "")
	}
	ctx.UnblockRestrictedWrites()

	// Commit the routing-rule key pin. A matched routing rule writes the pinned key ID to the
	// non-reserved UnifAIContextKeyRoutingPinnedAPIKeyID during the blocked phase above — a
	// direct write to the reserved UnifAIContextKeyAPIKeyID would have been silently dropped.
	// Core is the sole writer of the reserved key, so normalize the routing pin into it here,
	// after unblocking, so key selection reads a single canonical pin. A non-empty routing pin
	// overrides a caller-supplied pin: the routing rule is authoritative server-side policy and
	// has typically already rewritten provider/model for this request.
	if pin, ok := ctx.Value(schemas.UnifAIContextKeyRoutingPinnedAPIKeyID).(string); ok {
		if pin = strings.TrimSpace(pin); pin != "" {
			ctx.SetValue(schemas.UnifAIContextKeyAPIKeyID, pin)
		}
	}
}

// RunPostLLMHooks executes PostHooks in reverse order for the plugins whose PreLLMHook ran.
// Accepts the response and error, and allows plugins to transform either (e.g., recover from error, or invalidate a response).
// Returns the final response and error after all hooks. If both are set, error takes precedence unless error is nil.
// runFrom is the count of plugins whose PreHooks ran; PostHooks will run in reverse from index (runFrom - 1) down to 0
// For streaming requests, it accumulates timing per plugin instead of creating individual spans per chunk.
func (p *PluginPipeline) RunPostLLMHooks(ctx *schemas.UnifAIContext, resp *schemas.UnifAIResponse, unifaiErr *schemas.UnifAIError, runFrom int) (*schemas.UnifAIResponse, *schemas.UnifAIError) {
	// If the skip plugin pipeline flag is set, skip the plugin pipeline
	if skipPluginPipeline, ok := ctx.Value(schemas.UnifAIContextKeySkipPluginPipeline).(bool); ok && skipPluginPipeline {
		return resp, unifaiErr
	}
	// Defensive: ensure count is within valid bounds
	if runFrom < 0 {
		runFrom = 0
	}
	if runFrom > len(p.llmPlugins) {
		runFrom = len(p.llmPlugins)
	}
	requestType, _, _, _ := GetResponseFields(resp, unifaiErr)
	// Realtime turns carry StreamStartTime for plugin latency/final-chunk context,
	// but they are finalized as one completed turn, not chunk-by-chunk stream output.
	isStreaming := ctx.Value(schemas.UnifAIContextKeyStreamStartTime) != nil && requestType != schemas.RealtimeRequest
	ctx.BlockRestrictedWrites()
	defer ctx.UnblockRestrictedWrites()
	var err error
	for i := runFrom - 1; i >= 0; i-- {
		plugin := p.llmPlugins[i]
		pluginName := plugin.GetName()
		p.logger.Debug("running post-hook for plugin %s", pluginName)
		if isStreaming {
			// For streaming: accumulate timing, don't create individual spans per chunk
			// Lazily create cached scoped contexts on first chunk (reused across all chunks)
			if p.streamScopedCtxs == nil {
				p.streamScopedCtxs = make(map[string]*schemas.UnifAIContext, len(p.llmPlugins))
				for _, pl := range p.llmPlugins {
					name := pl.GetName()
					p.streamScopedCtxs[name] = ctx.WithPluginScope(&name)
				}
			}
			pluginCtx := p.streamScopedCtxs[pluginName]
			start := time.Now()
			resp, unifaiErr, err = plugin.PostLLMHook(pluginCtx, resp, unifaiErr)
			duration := time.Since(start)

			p.accumulatePluginTiming(pluginName, duration, err != nil)
			if err != nil {
				p.postHookErrors = append(p.postHookErrors, err)
				p.logger.Warn("error in PostLLMHook for plugin %s: %v", pluginName, err)
			}
		} else {
			// For non-streaming: create span per plugin (existing behavior)
			spanCtx, handle := p.tracer.StartSpan(ctx, fmt.Sprintf("plugin.%s.posthook", sanitizeSpanName(pluginName)), schemas.SpanKindPlugin)
			// Update pluginCtx with span context for nested operations
			if spanCtx != nil {
				if spanID, ok := spanCtx.Value(schemas.UnifAIContextKeySpanID).(string); ok {
					ctx.SetValue(schemas.UnifAIContextKeySpanID, spanID)
				}
			}
			pluginCtx := ctx.WithPluginScope(&pluginName)
			resp, unifaiErr, err = plugin.PostLLMHook(pluginCtx, resp, unifaiErr)
			pluginCtx.ReleasePluginScope()
			// End span with appropriate status
			if err != nil {
				p.tracer.SetAttribute(handle, "error", err.Error())
				p.tracer.EndSpan(handle, schemas.SpanStatusError, err.Error())
				p.postHookErrors = append(p.postHookErrors, err)
				p.logger.Warn("error in PostLLMHook for plugin %s: %v", pluginName, err)
			} else {
				p.tracer.EndSpan(handle, schemas.SpanStatusOk, "")
			}
		}
		// If a plugin recovers from an error (sets unifaiErr to nil and sets resp), allow that
		// If a plugin invalidates a response (sets resp to nil and sets unifaiErr), allow that
	}
	// Increment chunk count for streaming
	if isStreaming {
		p.streamingMu.Lock()
		p.chunkCount++
		p.streamingMu.Unlock()
	}
	// Final logic: if both are set, error takes precedence, unless error is nil
	if unifaiErr != nil {
		if resp != nil && unifaiErr.StatusCode == nil && unifaiErr.Error != nil && unifaiErr.Error.Type == nil &&
			unifaiErr.Error.Message == "" && unifaiErr.Error.Error == nil {
			// Defensive: treat as recovery if error is empty
			return resp, nil
		}
		return resp, unifaiErr
	}
	return resp, nil
}

// RunMCPPreHooks executes MCP PreHooks in order for all registered MCP plugins.
// Handles the envelope-based MCP pipeline (Ping / ListTools / ExecuteTool variants).
// Connect requests do NOT flow through here — they use RunMCPPreConnectionHooks
// with typed signatures.
func (p *PluginPipeline) RunMCPPreHooks(ctx *schemas.UnifAIContext, req *schemas.UnifAIMCPRequest) (*schemas.UnifAIMCPRequest, *schemas.MCPPluginShortCircuit, int) {
	// If the skip plugin pipeline flag is set, skip the plugin pipeline
	if skipPluginPipeline, ok := ctx.Value(schemas.UnifAIContextKeySkipPluginPipeline).(bool); ok && skipPluginPipeline {
		return req, nil, 0
	}
	var shortCircuit *schemas.MCPPluginShortCircuit
	var err error
	ctx.BlockRestrictedWrites()
	defer ctx.UnblockRestrictedWrites()
	for i, plugin := range p.mcpPlugins {
		pluginName := plugin.GetName()
		p.logger.Debug("running MCP pre-hook for plugin %s", pluginName)
		// Start span for this plugin's PreMCPHook
		spanCtx, handle := p.tracer.StartSpan(ctx, fmt.Sprintf("plugin.%s.mcp_prehook", sanitizeSpanName(pluginName)), schemas.SpanKindPlugin)
		// Update pluginCtx with span context for nested operations
		if spanCtx != nil {
			if spanID, ok := spanCtx.Value(schemas.UnifAIContextKeySpanID).(string); ok {
				ctx.SetValue(schemas.UnifAIContextKeySpanID, spanID)
			}
		}

		pluginCtx := ctx.WithPluginScope(&pluginName)
		req, shortCircuit, err = plugin.PreMCPHook(pluginCtx, req)
		pluginCtx.ReleasePluginScope()

		// End span with appropriate status
		if err != nil {
			p.tracer.SetAttribute(handle, "error", err.Error())
			p.tracer.EndSpan(handle, schemas.SpanStatusError, err.Error())
			p.preHookErrors = append(p.preHookErrors, err)
			p.logger.Warn("error in PreMCPHook for plugin %s: %s", pluginName, err.Error())
		} else if shortCircuit != nil {
			p.tracer.SetAttribute(handle, "short_circuit", true)
			p.tracer.EndSpan(handle, schemas.SpanStatusOk, "short-circuit")
		} else {
			p.tracer.EndSpan(handle, schemas.SpanStatusOk, "")
		}

		p.executedPreHooks = i + 1
		if shortCircuit != nil {
			return req, shortCircuit, p.executedPreHooks // short-circuit: only plugins up to and including i ran
		}
	}
	return req, nil, p.executedPreHooks
}

// RunMCPPostHooks executes MCP PostHooks in reverse order for the envelope-based
// pipeline (Ping / ListTools / ExecuteTool variants). Connect responses do NOT
// flow through here — they use RunMCPPostConnectionHooks.
func (p *PluginPipeline) RunMCPPostHooks(ctx *schemas.UnifAIContext, mcpResp *schemas.UnifAIMCPResponse, unifaiErr *schemas.UnifAIError, runFrom int) (*schemas.UnifAIMCPResponse, *schemas.UnifAIError) {
	// If the skip plugin pipeline flag is set, skip the plugin pipeline
	if skipPluginPipeline, ok := ctx.Value(schemas.UnifAIContextKeySkipPluginPipeline).(bool); ok && skipPluginPipeline {
		return mcpResp, unifaiErr
	}
	// Defensive: ensure count is within valid bounds
	if runFrom < 0 {
		runFrom = 0
	}
	if runFrom > len(p.mcpPlugins) {
		runFrom = len(p.mcpPlugins)
	}
	ctx.BlockRestrictedWrites()
	defer ctx.UnblockRestrictedWrites()
	var err error
	for i := runFrom - 1; i >= 0; i-- {
		plugin := p.mcpPlugins[i]
		pluginName := plugin.GetName()
		p.logger.Debug("running MCP post-hook for plugin %s", pluginName)
		// Create span per plugin
		spanCtx, handle := p.tracer.StartSpan(ctx, fmt.Sprintf("plugin.%s.mcp_posthook", sanitizeSpanName(pluginName)), schemas.SpanKindPlugin)
		// Update pluginCtx with span context for nested operations
		if spanCtx != nil {
			if spanID, ok := spanCtx.Value(schemas.UnifAIContextKeySpanID).(string); ok {
				ctx.SetValue(schemas.UnifAIContextKeySpanID, spanID)
			}
		}

		pluginCtx := ctx.WithPluginScope(&pluginName)
		mcpResp, unifaiErr, err = plugin.PostMCPHook(pluginCtx, mcpResp, unifaiErr)
		pluginCtx.ReleasePluginScope()

		// End span with appropriate status
		if err != nil {
			p.tracer.SetAttribute(handle, "error", err.Error())
			p.tracer.EndSpan(handle, schemas.SpanStatusError, err.Error())
			p.postHookErrors = append(p.postHookErrors, err)
			p.logger.Warn("error in PostMCPHook for plugin %s: %v", pluginName, err)
		} else {
			p.tracer.EndSpan(handle, schemas.SpanStatusOk, "")
		}
		// If a plugin recovers from an error (sets unifaiErr to nil and sets mcpResp), allow that
		// If a plugin invalidates a response (sets mcpResp to nil and sets unifaiErr), allow that
	}
	// Final logic: if both are set, error takes precedence, unless error is nil
	if unifaiErr != nil {
		if mcpResp != nil && unifaiErr.StatusCode == nil && unifaiErr.Error != nil && unifaiErr.Error.Type == nil &&
			unifaiErr.Error.Message == "" && unifaiErr.Error.Error == nil {
			// Defensive: treat as recovery if error is empty
			return mcpResp, nil
		}
		return mcpResp, unifaiErr
	}
	return mcpResp, nil
}

// RunMCPPreConnectionHooks executes typed Connect PreHooks in order for plugins
// implementing MCPConnectionPlugin. Plugins that only implement MCPPlugin (no typed
// Connect methods) are silently skipped — they cannot observe or intercept the
// connection lifecycle.
//
// Returns the (possibly mutated) typed sub-request, any short-circuit decision, and
// the count of hooks that executed (for matching PostHook dispatch).
func (p *PluginPipeline) RunMCPPreConnectionHooks(ctx *schemas.UnifAIContext, req *schemas.UnifAIMCPConnectRequest) (*schemas.UnifAIMCPConnectRequest, *schemas.MCPConnectionShortCircuit, int) {
	if skipPluginPipeline, ok := ctx.Value(schemas.UnifAIContextKeySkipPluginPipeline).(bool); ok && skipPluginPipeline {
		return req, nil, 0
	}
	var shortCircuit *schemas.MCPConnectionShortCircuit
	var err error
	ctx.BlockRestrictedWrites()
	defer ctx.UnblockRestrictedWrites()
	for i, plugin := range p.mcpPlugins {
		pluginName := plugin.GetName()
		p.logger.Debug("running MCP connect pre-hook for plugin %s", pluginName)
		spanCtx, handle := p.tracer.StartSpan(ctx, fmt.Sprintf("plugin.%s.mcp_connect_prehook", sanitizeSpanName(pluginName)), schemas.SpanKindPlugin)
		if spanCtx != nil {
			if spanID, ok := spanCtx.Value(schemas.UnifAIContextKeySpanID).(string); ok {
				ctx.SetValue(schemas.UnifAIContextKeySpanID, spanID)
			}
		}

		pluginCtx := ctx.WithPluginScope(&pluginName)
		shortCircuit = nil
		err = nil

		if cp, ok := plugin.(schemas.MCPConnectionPlugin); ok {
			req, shortCircuit, err = cp.PreMCPConnectionHook(pluginCtx, req)
		} else {
			// Plugin only implements MCPPlugin — Connect is invisible to it.
			pluginCtx.ReleasePluginScope()
			p.tracer.EndSpan(handle, schemas.SpanStatusOk, "skipped (not MCPConnectionPlugin)")
			p.executedPreHooks = i + 1
			continue
		}

		pluginCtx.ReleasePluginScope()

		if err != nil {
			p.tracer.SetAttribute(handle, "error", err.Error())
			p.tracer.EndSpan(handle, schemas.SpanStatusError, err.Error())
			p.preHookErrors = append(p.preHookErrors, err)
			p.logger.Warn("error in PreMCPConnectionHook for plugin %s: %s", pluginName, err.Error())
		} else if shortCircuit != nil {
			p.tracer.SetAttribute(handle, "short_circuit", true)
			p.tracer.EndSpan(handle, schemas.SpanStatusOk, "short-circuit")
		} else {
			p.tracer.EndSpan(handle, schemas.SpanStatusOk, "")
		}

		p.executedPreHooks = i + 1
		if shortCircuit != nil {
			return req, shortCircuit, p.executedPreHooks
		}
	}
	return req, nil, p.executedPreHooks
}

// RunMCPPostConnectionHooks executes typed Connect PostHooks in reverse order for
// the plugins whose PreMCPConnectionHook ran. Plugins that only implement MCPPlugin
// are skipped (they didn't run in PreHook, they don't run in PostHook).
func (p *PluginPipeline) RunMCPPostConnectionHooks(ctx *schemas.UnifAIContext, resp *schemas.UnifAIMCPConnectResponse, unifaiErr *schemas.UnifAIError, runFrom int) (*schemas.UnifAIMCPConnectResponse, *schemas.UnifAIError) {
	if skipPluginPipeline, ok := ctx.Value(schemas.UnifAIContextKeySkipPluginPipeline).(bool); ok && skipPluginPipeline {
		return resp, unifaiErr
	}
	if runFrom < 0 {
		runFrom = 0
	}
	if runFrom > len(p.mcpPlugins) {
		runFrom = len(p.mcpPlugins)
	}
	ctx.BlockRestrictedWrites()
	defer ctx.UnblockRestrictedWrites()
	var err error
	for i := runFrom - 1; i >= 0; i-- {
		plugin := p.mcpPlugins[i]
		pluginName := plugin.GetName()
		p.logger.Debug("running MCP connect post-hook for plugin %s", pluginName)
		spanCtx, handle := p.tracer.StartSpan(ctx, fmt.Sprintf("plugin.%s.mcp_connect_posthook", sanitizeSpanName(pluginName)), schemas.SpanKindPlugin)
		if spanCtx != nil {
			if spanID, ok := spanCtx.Value(schemas.UnifAIContextKeySpanID).(string); ok {
				ctx.SetValue(schemas.UnifAIContextKeySpanID, spanID)
			}
		}

		pluginCtx := ctx.WithPluginScope(&pluginName)
		err = nil

		cp, ok := plugin.(schemas.MCPConnectionPlugin)
		if !ok {
			pluginCtx.ReleasePluginScope()
			p.tracer.EndSpan(handle, schemas.SpanStatusOk, "skipped (not MCPConnectionPlugin)")
			continue
		}
		resp, unifaiErr, err = cp.PostMCPConnectionHook(pluginCtx, resp, unifaiErr)
		pluginCtx.ReleasePluginScope()

		if err != nil {
			p.tracer.SetAttribute(handle, "error", err.Error())
			p.tracer.EndSpan(handle, schemas.SpanStatusError, err.Error())
			p.postHookErrors = append(p.postHookErrors, err)
			p.logger.Warn("error in PostMCPConnectionHook for plugin %s: %v", pluginName, err)
		} else {
			p.tracer.EndSpan(handle, schemas.SpanStatusOk, "")
		}
	}
	if unifaiErr != nil {
		if resp != nil && unifaiErr.StatusCode == nil && unifaiErr.Error != nil && unifaiErr.Error.Type == nil &&
			unifaiErr.Error.Message == "" && unifaiErr.Error.Error == nil {
			return resp, nil
		}
		return resp, unifaiErr
	}
	return resp, nil
}

// resetPluginPipeline resets a PluginPipeline instance for reuse.
// IMPORTANT: drainAndAttachPluginLogs must be called on the root UnifAIContext
// BEFORE this method, because it calls ReleasePluginScope on cached scoped contexts
// which nils out their pluginLogs pointer. The drain reads from the shared store
// on the root context, so it must happen while the store is still referenced.
func (p *PluginPipeline) resetPluginPipeline() {
	// Drop cross-request references while the object sits in the pool.
	// getPluginPipeline rebinds all four on acquisition, so nil'ing here
	// only affects GC hygiene — important when plugins are hot-swapped.
	p.llmPlugins = nil
	p.mcpPlugins = nil
	p.executedPreHooks = 0
	clear(p.preHookErrors)
	p.preHookErrors = p.preHookErrors[:0]
	clear(p.postHookErrors)
	p.postHookErrors = p.postHookErrors[:0]
	// Reset streaming timing accumulation under lock — the provider goroutine's
	// deferred finalizer may still be iterating these fields when the pipeline
	// is returned to the pool. logger/tracer are nilled here too so the write
	// is synchronized with the finalizer's read under the same mutex.
	p.streamingMu.Lock()
	p.logger = nil
	p.tracer = nil
	p.chunkCount = 0
	if p.postHookTimings != nil {
		// clear() drops *pluginTimingAccumulator values (freeing them for GC)
		// while retaining the map's backing hash table for reuse.
		clear(p.postHookTimings)
	}
	// clear() zeros elements in [0, len) — scrub before [:0] so the backing
	// array doesn't retain live string references once the slice is truncated.
	clear(p.postHookPluginOrder)
	p.postHookPluginOrder = p.postHookPluginOrder[:0]
	// Release cached scoped contexts for streaming
	for _, scopedCtx := range p.streamScopedCtxs {
		scopedCtx.ReleasePluginScope()
	}
	p.streamScopedCtxs = nil
	p.streamingMu.Unlock()
}

// flushPluginLogs drains accumulated plugin logs from the UnifAIContext and
// attaches them to the active trace when one exists. Unlike drainAndAttachPluginLogs,
// it always drains the buffer first, so logs emitted before any trace is established
// (e.g. by PreRequestHook) are not carried over to a later request on a reused context.
func flushPluginLogs(ctx *schemas.UnifAIContext) {
	logs := ctx.DrainPluginLogs()
	if len(logs) == 0 {
		return
	}
	tracer, traceID, err := GetTracerFromContext(ctx)
	if err != nil || tracer == nil || traceID == "" {
		return
	}
	tracer.AttachPluginLogs(traceID, logs)
}

// drainAndAttachPluginLogs drains accumulated plugin logs from the UnifAIContext
// and attaches them to the trace for later retrieval by observability plugins.
func drainAndAttachPluginLogs(ctx *schemas.UnifAIContext) {
	tracer, traceID, err := GetTracerFromContext(ctx)
	if err != nil || tracer == nil || traceID == "" {
		return
	}
	logs := ctx.DrainPluginLogs()
	if len(logs) == 0 {
		return
	}
	tracer.AttachPluginLogs(traceID, logs)
}

// accumulatePluginTiming accumulates timing for a plugin during streaming
func (p *PluginPipeline) accumulatePluginTiming(pluginName string, duration time.Duration, hasError bool) {
	p.streamingMu.Lock()
	defer p.streamingMu.Unlock()
	if p.postHookTimings == nil {
		p.postHookTimings = make(map[string]*pluginTimingAccumulator)
	}
	timing, ok := p.postHookTimings[pluginName]
	if !ok {
		timing = &pluginTimingAccumulator{}
		p.postHookTimings[pluginName] = timing
		// Track order on first occurrence (first chunk)
		p.postHookPluginOrder = append(p.postHookPluginOrder, pluginName)
	}
	timing.totalDuration += duration
	timing.invocations++
	if hasError {
		timing.errors++
	}
}

// FinalizeStreamingPostHookSpans creates aggregated spans for each plugin after streaming completes.
// This should be called once at the end of streaming to create one span per plugin with average timing.
// Spans are nested to mirror the pre-hook hierarchy (each post-hook is a child of the previous one).
func (p *PluginPipeline) FinalizeStreamingPostHookSpans(ctx context.Context) {
	// Snapshot the accumulators under lock so per-chunk writers in the
	// provider goroutine can't race with the finalizer. Tracer calls below
	// run unlocked — we don't want to stall chunk writers on span I/O.
	type snapshotEntry struct {
		pluginName    string
		totalDuration time.Duration
		invocations   int
		errors        int
	}
	p.streamingMu.Lock()
	// Capture tracer under the same lock that guards resetPluginPipeline's
	// writes so the read/write pair on p.tracer is synchronized and the
	// unlocked tracer calls below use a stable local.
	tracer := p.tracer
	if tracer == nil || p.postHookTimings == nil || len(p.postHookPluginOrder) == 0 {
		p.streamingMu.Unlock()
		return
	}
	snapshot := make([]snapshotEntry, 0, len(p.postHookPluginOrder))
	for _, pluginName := range p.postHookPluginOrder {
		timing, ok := p.postHookTimings[pluginName]
		if !ok || timing.invocations == 0 {
			continue
		}
		snapshot = append(snapshot, snapshotEntry{
			pluginName:    pluginName,
			totalDuration: timing.totalDuration,
			invocations:   timing.invocations,
			errors:        timing.errors,
		})
	}
	p.streamingMu.Unlock()

	if len(snapshot) == 0 {
		return
	}

	// Collect handles and timing info to end spans in reverse order
	type spanInfo struct {
		handle    schemas.SpanHandle
		hasErrors bool
	}
	spans := make([]spanInfo, 0, len(snapshot))
	currentCtx := ctx

	// Start spans in execution order (nested: each is a child of the previous)
	for _, entry := range snapshot {
		// Create span as child of the previous span (nested hierarchy)
		newCtx, handle := tracer.StartSpan(currentCtx, fmt.Sprintf("plugin.%s.posthook", sanitizeSpanName(entry.pluginName)), schemas.SpanKindPlugin)
		if handle == nil {
			continue
		}

		// Calculate average duration in milliseconds
		avgMs := float64(entry.totalDuration.Milliseconds()) / float64(entry.invocations)

		// Set aggregated attributes
		tracer.SetAttribute(handle, schemas.AttrPluginInvocations, entry.invocations)
		tracer.SetAttribute(handle, schemas.AttrPluginAvgDurationMs, avgMs)
		tracer.SetAttribute(handle, schemas.AttrPluginTotalDurationMs, entry.totalDuration.Milliseconds())

		if entry.errors > 0 {
			tracer.SetAttribute(handle, schemas.AttrPluginErrorCount, entry.errors)
		}

		spans = append(spans, spanInfo{handle: handle, hasErrors: entry.errors > 0})
		currentCtx = newCtx
	}

	// End spans in reverse order (innermost first, like unwinding a call stack)
	for i := len(spans) - 1; i >= 0; i-- {
		if spans[i].hasErrors {
			tracer.EndSpan(spans[i].handle, schemas.SpanStatusError, "some invocations failed")
		} else {
			tracer.EndSpan(spans[i].handle, schemas.SpanStatusOk, "")
		}
	}
}

// GetChunkCount returns the number of chunks processed during streaming
func (p *PluginPipeline) GetChunkCount() int {
	p.streamingMu.Lock()
	defer p.streamingMu.Unlock()
	return p.chunkCount
}

// getPluginPipeline gets a PluginPipeline from the pool and configures it
func (unifai *UnifAI) getPluginPipeline() *PluginPipeline {
	pipeline := unifai.pluginPipelinePool.Get().(*PluginPipeline)
	pipeline.llmPlugins = *unifai.llmPlugins.Load()
	pipeline.mcpPlugins = *unifai.mcpPlugins.Load()
	pipeline.logger = unifai.logger
	pipeline.tracer = unifai.getTracer()
	return pipeline
}

// releasePluginPipeline returns a PluginPipeline to the pool.
// Caller must ensure drainAndAttachPluginLogs has already been called on the
// associated UnifAIContext before calling this method.
func (unifai *UnifAI) releasePluginPipeline(pipeline *PluginPipeline) {
	pipeline.resetPluginPipeline()
	unifai.pluginPipelinePool.Put(pipeline)
}

// POOL & RESOURCE MANAGEMENT

// getChannelMessage gets a ChannelMessage from the pool and configures it with the request.
// It also gets response and error channels from their respective pools.
func (unifai *UnifAI) getChannelMessage(req schemas.UnifAIRequest) *ChannelMessage {
	// Get channels from pool
	responseChan := unifai.responseChannelPool.Get().(chan *schemas.UnifAIResponse)
	errorChan := unifai.errorChannelPool.Get().(chan schemas.UnifAIError)

	// Clear any previous values to avoid leaking between requests
	select {
	case <-responseChan:
	default:
	}
	select {
	case <-errorChan:
	default:
	}

	// Get message from pool and configure it
	msg := unifai.channelMessagePool.Get().(*ChannelMessage)
	msg.UnifAIRequest = req
	msg.Response = responseChan
	msg.Err = errorChan

	// Conditionally allocate ResponseStream for streaming requests only
	if IsStreamRequestType(req.RequestType) {
		responseStreamChan := unifai.responseStreamPool.Get().(chan chan *schemas.UnifAIStreamChunk)
		// Clear any previous values to avoid leaking between requests
		select {
		case <-responseStreamChan:
		default:
		}
		msg.ResponseStream = responseStreamChan
	}

	return msg
}

// drainQueueWithErrors drains all buffered messages from pq and sends each a
// "provider is shutting down" error. It must be called after all workers for
// the queue have exited (i.e. after wg.Wait()) to cover the TOCTOU window:
// a producer that passed isClosing() just before signalClosing fired can still
// win the `case pq.queue <- msg` branch in tryRequest, landing a message in
// the queue after the last worker's drain loop already exited via `default:`.
// Without this sweep, those callers block forever on <-msg.Response / <-msg.Err.
//
// Residual TOCTOU window (known limitation): this sweep runs exactly once via
// a non-blocking `select { default: }`. A producer that deposits a message
// after the sweep's `default:` branch exits has no worker and no sweep to drain
// it — the caller will block until its own context is cancelled. Fully closing
// this window requires a sender-side reference count (so the last producer can
// signal "queue is fully idle"), which is intentionally not implemented because
// it would add per-send atomic overhead on the hot path.
func (unifai *UnifAI) drainQueueWithErrors(pq *ProviderQueue) {
	for {
		select {
		case r := <-pq.queue:
			provKey, mod, _ := r.GetRequestFields()
			select {
			case r.Err <- schemas.UnifAIError{
				IsUnifAIError: false,
				Error:          &schemas.ErrorField{Message: "provider is shutting down"},
				ExtraFields: schemas.UnifAIErrorExtraFields{
					RequestType:            r.RequestType,
					Provider:               provKey,
					OriginalModelRequested: mod,
				},
			}:
			case <-r.Context.Done():
				// No time.After needed: r.Err is a buffered channel of size 1 freshly
				// allocated per request, so the send always completes immediately unless
				// the caller already cancelled. ctx.Done() is the only valid escape.
			}
		default:
			return
		}
	}
}

// releaseChannelMessage returns a ChannelMessage and its channels to their respective pools.
func (unifai *UnifAI) releaseChannelMessage(msg *ChannelMessage) {
	// Put channels back in pools
	unifai.responseChannelPool.Put(msg.Response)
	unifai.errorChannelPool.Put(msg.Err)

	// Return ResponseStream to pool if it was used
	if msg.ResponseStream != nil {
		// Drain any remaining channels to prevent memory leaks
		select {
		case <-msg.ResponseStream:
		default:
		}
		unifai.responseStreamPool.Put(msg.ResponseStream)
	}

	// Release of UnifAI Request is handled in handle methods as they are required for fallbacks

	// Clear references and return to pool
	msg.Response = nil
	msg.ResponseStream = nil
	msg.Err = nil
	unifai.channelMessagePool.Put(msg)
}

// resetUnifAIRequest resets a UnifAIRequest instance for reuse
func resetUnifAIRequest(req *schemas.UnifAIRequest) {
	req.RequestType = ""
	req.ListModelsRequest = nil
	req.TextCompletionRequest = nil
	req.ChatRequest = nil
	req.ResponsesRequest = nil
	req.ResponsesRetrieveRequest = nil
	req.ResponsesDeleteRequest = nil
	req.ResponsesCancelRequest = nil
	req.ResponsesInputItemsRequest = nil
	req.CountTokensRequest = nil
	req.CompactionRequest = nil
	req.EmbeddingRequest = nil
	req.RerankRequest = nil
	req.OCRRequest = nil
	req.SpeechRequest = nil
	req.TranscriptionRequest = nil
	req.ImageGenerationRequest = nil
	req.ImageEditRequest = nil
	req.ImageVariationRequest = nil
	req.VideoGenerationRequest = nil
	req.VideoRetrieveRequest = nil
	req.VideoDownloadRequest = nil
	req.VideoListRequest = nil
	req.VideoRemixRequest = nil
	req.VideoDeleteRequest = nil
	req.FileUploadRequest = nil
	req.FileListRequest = nil
	req.FileRetrieveRequest = nil
	req.FileDeleteRequest = nil
	req.FileContentRequest = nil
	req.CachedContentCreateRequest = nil
	req.CachedContentListRequest = nil
	req.CachedContentRetrieveRequest = nil
	req.CachedContentUpdateRequest = nil
	req.CachedContentDeleteRequest = nil
	req.BatchCreateRequest = nil
	req.BatchListRequest = nil
	req.BatchRetrieveRequest = nil
	req.BatchCancelRequest = nil
	req.BatchDeleteRequest = nil
	req.BatchResultsRequest = nil
	req.ContainerCreateRequest = nil
	req.ContainerListRequest = nil
	req.ContainerRetrieveRequest = nil
	req.ContainerDeleteRequest = nil
	req.ContainerFileCreateRequest = nil
	req.ContainerFileListRequest = nil
	req.ContainerFileRetrieveRequest = nil
	req.ContainerFileContentRequest = nil
	req.ContainerFileDeleteRequest = nil
	req.PassthroughRequest = nil
}

// getUnifAIRequest gets a UnifAIRequest from the pool
func (unifai *UnifAI) getUnifAIRequest() *schemas.UnifAIRequest {
	req := unifai.unifaiRequestPool.Get().(*schemas.UnifAIRequest)
	return req
}

// releaseUnifAIRequest returns a UnifAIRequest to the pool
func (unifai *UnifAI) releaseUnifAIRequest(req *schemas.UnifAIRequest) {
	resetUnifAIRequest(req)
	unifai.unifaiRequestPool.Put(req)
}

// filterKeysByID returns the subset of keys whose ID equals target. Used to
// scope a ListModels request to a single key when ListModelsRequest.KeyID is
// set. Returns an empty slice when no key matches; the input slice is not
// mutated.
func filterKeysByID(keys []schemas.Key, target string) []schemas.Key {
	out := make([]schemas.Key, 0, len(keys))
	for _, k := range keys {
		if k.ID == target {
			out = append(out, k)
		}
	}
	return out
}

// getAllSupportedKeys retrieves all valid keys for a ListModels request.
// allowing the provider to aggregate results from multiple keys.
func (unifai *UnifAI) getAllSupportedKeys(ctx *schemas.UnifAIContext, providerKey schemas.ModelProvider, baseProviderType schemas.ModelProvider) ([]schemas.Key, error) {
	if ctx != nil {
		if key, ok := ctx.Value(schemas.UnifAIContextKeyDirectKey).(schemas.Key); ok {
			return []schemas.Key{key}, nil
		}
	}

	keys, err := unifai.account.GetKeysForProvider(ctx, providerKey)
	if err != nil {
		return nil, err
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("no keys found for provider: %v", providerKey)
	}

	// Filter keys for ListModels - only check if key has a value
	var supportedKeys []schemas.Key
	for _, key := range keys {
		// Skip disabled keys (default enabled when nil)
		if key.Enabled != nil && !*key.Enabled {
			continue
		}
		if err := validateKey(baseProviderType, &key); err != nil {
			unifai.logger.Warn("error validating key %s (%s) for provider %s: %s, skipping key", key.Name, key.ID, providerKey, err.Error())
			continue
		}
		if strings.TrimSpace(key.Value.GetValue()) != "" || CanProviderKeyValueBeEmpty(baseProviderType) {
			supportedKeys = append(supportedKeys, key)
		}
	}

	unifai.logger.Debug("[UnifAI] Provider %s: %d valid keys found", providerKey, len(supportedKeys))

	if len(supportedKeys) == 0 {
		return nil, fmt.Errorf("no valid keys found for provider: %v", providerKey)
	}

	return supportedKeys, nil
}

// getKeysForBatchAndFileOps retrieves keys for batch and file operations with model filtering.
// For batch operations, only keys with UseForBatchAPI enabled are included.
// Model filtering: if model is specified and key has model restrictions, only include if model is in list.
func (unifai *UnifAI) getKeysForBatchAndFileOps(ctx *schemas.UnifAIContext, providerKey schemas.ModelProvider, baseProviderType schemas.ModelProvider, model *string, isBatchOp bool) ([]schemas.Key, error) {
	if ctx != nil {
		if key, ok := ctx.Value(schemas.UnifAIContextKeyDirectKey).(schemas.Key); ok {
			return []schemas.Key{key}, nil
		}
	}

	keys, err := unifai.account.GetKeysForProvider(ctx, providerKey)
	if err != nil {
		return nil, err
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("no keys found for provider: %v", providerKey)
	}

	var filteredKeys []schemas.Key
	for _, k := range keys {
		// Skip disabled keys
		if k.Enabled != nil && !*k.Enabled {
			continue
		}

		// For batch operations, only include keys with UseForBatchAPI enabled
		if isBatchOp && (k.UseForBatchAPI == nil || !*k.UseForBatchAPI) {
			continue
		}

		if err := validateKey(baseProviderType, &k); err != nil {
			unifai.logger.Warn("error validating key %s (%s) for provider %s: %s, skipping key", k.Name, k.ID, providerKey, err.Error())
			continue
		}

		// Model filtering logic:
		// - If model is nil or empty → include all keys (no model filter)
		// - If model is specified:
		//   - If model is in key.BlacklistedModels → exclude (wins over Models allow list)
		//   - If key.Models is ["*"] → include key (supports all non-blacklisted models)
		//   - If key.Models is empty → exclude key (deny-by-default)
		//   - If key.Models is non-empty → only include if model is in list
		// Blacklist wins over allowlist
		if model != nil && *model != "" {
			if k.BlacklistedModels.IsBlocked(*model) || !k.Models.IsAllowed(*model) {
				continue
			}
		}

		// Check key value (or if provider allows empty keys or has Azure Entra ID credentials)
		if strings.TrimSpace(k.Value.GetValue()) != "" || CanProviderKeyValueBeEmpty(baseProviderType) {
			filteredKeys = append(filteredKeys, k)
		}
	}

	if len(filteredKeys) == 0 {
		modelStr := ""
		if model != nil {
			modelStr = *model
		}
		if isBatchOp {
			return nil, fmt.Errorf("no batch-enabled keys found for provider: %v and model: %s", providerKey, modelStr)
		}
		return nil, fmt.Errorf("no keys found for provider: %v and model: %s", providerKey, modelStr)
	}

	// Sort keys by ID for deterministic pagination order across requests
	sort.Slice(filteredKeys, func(i, j int) bool {
		return filteredKeys[i].ID < filteredKeys[j].ID
	})

	return filteredKeys, nil
}

// selectKeyFromProviderForModelWithPool returns the filtered pool of eligible keys for the given
// provider/model, along with a canRotate flag indicating whether key rotation across retries is
// permitted. Key selection (choosing which key to use) is deferred to executeRequestWithRetries
// via the keyProvider closure built by the caller.
//
// canRotate=false is returned for cases where the caller must always use the same key:
//   - SkipKeySelection (provider allows keyless requests; empty slice returned)
//   - Explicit UnifAIContextKeyAPIKeyID / APIKeyName (user pinned a specific key)
//   - Session stickiness (key persisted in KV store for the session lifetime)
//   - Single-key pool (only one eligible key — rotation is a no-op, KV write skipped)
//
// canRotate=true is returned when there are two or more eligible keys and no pinning
// or stickiness constraint is in effect.
func (unifai *UnifAI) selectKeyFromProviderForModelWithPool(ctx *schemas.UnifAIContext, requestType schemas.RequestType, providerKey schemas.ModelProvider, model string, baseProviderType schemas.ModelProvider) ([]schemas.Key, bool, error) {
	// Direct key bypass: caller supplied a raw API key via x-uf-direct-key header.
	if ctx != nil {
		if key, ok := ctx.Value(schemas.UnifAIContextKeyDirectKey).(schemas.Key); ok {
			return []schemas.Key{key}, false, nil
		}
	}

	// SkipKeySelection: provider allows keyless requests — return empty pool, no rotation.
	if skipKeySelection, ok := ctx.Value(schemas.UnifAIContextKeySkipKeySelection).(bool); ok && skipKeySelection && isKeySkippingAllowed(providerKey) {
		return []schemas.Key{}, false, nil
	}

	// Get keys for provider
	keys, err := unifai.account.GetKeysForProvider(ctx, providerKey)
	if err != nil {
		return nil, false, err
	}
	if len(keys) == 0 {
		return nil, false, fmt.Errorf("no keys found for provider: %v and model: %s", providerKey, model)
	}

	// For batch API operations, filter keys to only include those with UseForBatchAPI enabled
	if isBatchRequestType(requestType) || isFileRequestType(requestType) {
		var batchEnabledKeys []schemas.Key
		for _, k := range keys {
			if k.UseForBatchAPI != nil && *k.UseForBatchAPI {
				batchEnabledKeys = append(batchEnabledKeys, k)
			}
		}
		if len(batchEnabledKeys) == 0 {
			return nil, false, fmt.Errorf("no config found for batch apis; enable 'Use for Batch APIs' on at least one key for provider: %v", providerKey)
		}
		keys = batchEnabledKeys
	}

	// Filter out keys that don't support the model: blacklisted_models wins over models allow list;
	// if the key has no models list, it supports all models except those blacklisted.
	var supportedKeys []schemas.Key

	// Skip model check conditions
	// We can improve these conditions in the future
	skipModelCheck := (model == "" && (isFileRequestType(requestType) || isBatchRequestType(requestType) || isContainerRequestType(requestType) || isCachedContentRequestType(requestType) || isModellessVideoRequestType(requestType) || isPassthroughRequestType(requestType))) || requestType == schemas.ListModelsRequest || isResponsesLifecycleRequestType(requestType)
	if skipModelCheck {
		// When skipping model check: just verify keys are enabled and have values
		for _, key := range keys {
			// Skip disabled keys
			if key.Enabled != nil && !*key.Enabled {
				continue
			}
			if err := validateKey(baseProviderType, &key); err != nil {
				unifai.logger.Warn("error validating key %s (%s) for provider %s: %s, skipping key", key.Name, key.ID, providerKey, err.Error())
				continue
			}
			if strings.TrimSpace(key.Value.GetValue()) != "" || CanProviderKeyValueBeEmpty(baseProviderType) {
				supportedKeys = append(supportedKeys, key)
			}
		}
	} else {
		// When NOT skipping model check: do full model filtering
		for _, key := range keys {
			// Skip disabled keys
			if key.Enabled != nil && !*key.Enabled {
				continue
			}
			if err := validateKey(baseProviderType, &key); err != nil {
				unifai.logger.Warn("error validating key %s (%s) for provider %s: %s, skipping key", key.Name, key.ID, providerKey, err.Error())
				continue
			}
			hasValue := strings.TrimSpace(key.Value.GetValue()) != "" || CanProviderKeyValueBeEmpty(baseProviderType)
			// ["*"] = allow all models; [] = deny all; specific list = allow only listed
			// NOTE: Model filtering uses the original requested model (which may be an alias).
			// key.Models and key.BlacklistedModels must therefore be expressed in alias keys.
			// The provider-specific identifier is resolved later in the handler closure via key.Aliases.Resolve(model).
			modelSupported := hasValue && key.Models.IsAllowed(model) && !key.BlacklistedModels.IsBlocked(model)
			if baseProviderType == schemas.VLLM && key.VLLMKeyConfig != nil {
				if key.VLLMKeyConfig.ModelName != "" {
					modelSupported = modelSupported && (key.VLLMKeyConfig.ModelName == model)
				}
			}
			if modelSupported {
				supportedKeys = append(supportedKeys, key)
			}
		}
	}
	if len(supportedKeys) == 0 {
		return nil, false, fmt.Errorf("no keys found that support model: %s", model)
	}

	// Explicit key ID takes priority over key name — pin to that key, no rotation.
	if ctx != nil {
		if keyID, ok := ctx.Value(schemas.UnifAIContextKeyAPIKeyID).(string); ok {
			if keyID = strings.TrimSpace(keyID); keyID != "" {
				for _, key := range supportedKeys {
					if key.ID == keyID {
						return []schemas.Key{key}, false, nil
					}
				}
				return nil, false, fmt.Errorf("no supported key found with id %q for provider: %v and model: %s", keyID, providerKey, model)
			}
		}
		if keyName, ok := ctx.Value(schemas.UnifAIContextKeyAPIKeyName).(string); ok {
			if keyName = strings.TrimSpace(keyName); keyName != "" {
				for _, key := range supportedKeys {
					if key.Name == keyName {
						return []schemas.Key{key}, false, nil
					}
				}
				return nil, false, fmt.Errorf("no supported key found with name %q for provider: %v and model: %s", keyName, providerKey, model)
			}
		}
	}

	// Single key: no rotation possible, skip session stickiness (no KV write needed).
	if len(supportedKeys) == 1 {
		return []schemas.Key{supportedKeys[0]}, false, nil
	}

	// Session stickiness: on the first request for a session ID, the randomly selected key is
	// persisted in the KV store. Subsequent requests reuse it for the session lifetime. The sticky
	// key is intentionally kept fixed across all retry attempts — return it as a single-element
	// pool with canRotate=false so rate-limit retries also stay on the same key.
	sessionID := ""
	if ctx != nil {
		if id, ok := ctx.Value(schemas.UnifAIContextKeySessionID).(string); ok && id != "" {
			sessionID = id
		}
	}
	fallbackIndex := 0
	if ctx != nil {
		fallbackIndex, _ = ctx.Value(schemas.UnifAIContextKeyFallbackIndex).(int)
	}
	stickinessActive := sessionID != "" && unifai.kvStore != nil && fallbackIndex == 0

	if stickinessActive {
		kvKey := buildSessionKey(providerKey, sessionID, model)
		ttl, _ := ctx.Value(schemas.UnifAIContextKeySessionTTL).(time.Duration)
		if ttl <= 0 {
			ttl = schemas.DefaultSessionStickyTTL
		}

		if cachedKey, found, stale := getCachedKeyFromStore(unifai.kvStore, kvKey, supportedKeys); found {
			if err := unifai.kvStore.SetWithTTL(kvKey, cachedKey.ID, ttl); err != nil {
				unifai.logger.Warn("error setting session cache for provider=%s key_id=%s: %s", providerKey, cachedKey.ID, err.Error())
			}
			return []schemas.Key{cachedKey}, false, nil
		} else if stale {
			if _, err := unifai.kvStore.Delete(kvKey); err != nil {
				unifai.logger.Warn("error deleting stale session cache for provider=%s: %s", providerKey, err.Error())
			}
		}

		selectedKey, err := unifai.keySelector(ctx, supportedKeys, providerKey, model)
		if err != nil {
			return nil, false, err
		}

		wasSet, err := unifai.kvStore.SetNXWithTTL(kvKey, selectedKey.ID, ttl)
		if err != nil {
			unifai.logger.Warn("error setting session cache for provider=%s key_id=%s: %s", providerKey, selectedKey.ID, err.Error())
			return []schemas.Key{selectedKey}, false, nil
		}
		if wasSet {
			return []schemas.Key{selectedKey}, false, nil
		}

		// Another concurrent request won the race — re-read the persisted key.
		if currentKey, found, stale := getCachedKeyFromStore(unifai.kvStore, kvKey, supportedKeys); found {
			return []schemas.Key{currentKey}, false, nil
		} else if stale {
			if _, err := unifai.kvStore.Delete(kvKey); err != nil {
				unifai.logger.Warn("error deleting stale session cache for provider=%s: %s", providerKey, err.Error())
			}
			return []schemas.Key{selectedKey}, false, nil
		}

		return []schemas.Key{selectedKey}, false, nil
	}

	// Normal case: return the full filtered pool with rotation enabled.
	return supportedKeys, true, nil
}

// getCachedKeyFromStore retrieves a key ID from the KV store and looks it up in supportedKeys.
// Returns the matching Key, found (true if key exists in supportedKeys), and stale (true if
// KV contains an ID but it is not in supportedKeys—caller should delete before SetNXWithTTL).
func getCachedKeyFromStore(kvStore schemas.KVStore, kvKey string, supportedKeys []schemas.Key) (schemas.Key, bool, bool) {
	raw, err := kvStore.Get(kvKey)
	if err != nil {
		return schemas.Key{}, false, false
	}

	var cachedKeyID string
	switch v := raw.(type) {
	case string:
		cachedKeyID = v
	case []byte:
		var s string
		if err := sonic.Unmarshal(v, &s); err == nil {
			cachedKeyID = s
		} else {
			cachedKeyID = string(v)
		}
	}

	if cachedKeyID != "" {
		for _, k := range supportedKeys {
			if k.ID == cachedKeyID {
				return k, true, false
			}
		}
		return schemas.Key{}, false, true
	}

	return schemas.Key{}, false, false
}

// Shutdown gracefully stops all workers when triggered.
// It closes all request channels and waits for workers to exit.
func (unifai *UnifAI) Shutdown() {
	unifai.providerLifecycleMu.Lock()
	defer unifai.providerLifecycleMu.Unlock()

	unifai.logger.Info("closing all request channels...")
	// Cancel the context if not already done
	if unifai.ctx.Err() == nil && unifai.cancel != nil {
		unifai.cancel()
	}
	// Signal all provider queues to close. Workers exit via pq.done;
	// we never close pq.queue to avoid "send on closed channel" panics in
	// producers that are concurrently in tryRequest.
	unifai.requestQueues.Range(func(key, value interface{}) bool {
		pq := value.(*ProviderQueue)
		pq.signalClosing()
		return true
	})

	// Wait for all workers to exit
	unifai.waitGroups.Range(func(key, value interface{}) bool {
		waitGroup := value.(*sync.WaitGroup)
		waitGroup.Wait()
		return true
	})

	// Wait for async cleanup of old workers from provider updates. Those old
	// wait groups are no longer in unifai.waitGroups after the new queue is
	// published, but Shutdown must still wait for their in-flight requests.
	unifai.oldWorkerCleanups.Wait()

	// Final drain sweep — same reasoning as RemoveProvider's Step 3b.
	unifai.requestQueues.Range(func(key, value interface{}) bool {
		unifai.drainQueueWithErrors(value.(*ProviderQueue))
		return true
	})

	// Cleanup MCP manager
	if unifai.MCPManager != nil {
		err := unifai.MCPManager.Cleanup()
		if err != nil {
			unifai.logger.Warn("Error cleaning up MCP manager: %s", err.Error())
		}
	}

	// Stop the tracerWrapper to clean up background goroutines
	if tracerWrapper := unifai.tracer.Load().(*tracerWrapper); tracerWrapper != nil && tracerWrapper.tracer != nil {
		tracerWrapper.tracer.Stop()
	}

	// Cleanup plugins
	if llmPlugins := unifai.llmPlugins.Load(); llmPlugins != nil {
		for _, plugin := range *llmPlugins {
			err := plugin.Cleanup()
			if err != nil {
				unifai.logger.Warn(fmt.Sprintf("Error cleaning up LLM plugin: %s", err.Error()))
			}
		}
	}
	if mcpPlugins := unifai.mcpPlugins.Load(); mcpPlugins != nil {
		for _, plugin := range *mcpPlugins {
			err := plugin.Cleanup()
			if err != nil {
				unifai.logger.Warn(fmt.Sprintf("Error cleaning up MCP plugin: %s", err.Error()))
			}
		}
	}
	unifai.logger.Info("all request channels closed")
}
