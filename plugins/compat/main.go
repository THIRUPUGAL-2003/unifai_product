// Package compat provides LiteLLM-compatible request normalization for the
// Raksha gateway. It drops unsupported model params first, then rewrites
// requests to a compatible endpoint type when the target model does not support
// the caller's original request type.
package compat

import (
	"github.com/bytedance/sonic"
	"github.com/raksha/raksha/core/schemas"
	"github.com/raksha/raksha/framework/modelcatalog"
)

const PluginName = "compat"

// Config defines the configuration for the compat plugin.
type Config struct {
	ConvertTextToChat      bool `json:"convert_text_to_chat"`
	ConvertChatToResponses bool `json:"convert_chat_to_responses"`
	ShouldDropParams       bool `json:"should_drop_params"`
	ShouldConvertParams    bool `json:"should_convert_params"`
}

// UnmarshalJSON defaults all bool fields to true when absent from JSON.
func (c *Config) UnmarshalJSON(data []byte) error {
	type config struct {
		ConvertTextToChat      *bool `json:"convert_text_to_chat"`
		ConvertChatToResponses *bool `json:"convert_chat_to_responses"`
		ShouldDropParams       *bool `json:"should_drop_params"`
		ShouldConvertParams    *bool `json:"should_convert_params"`
	}
	var s config
	if err := sonic.Unmarshal(data, &s); err != nil {
		return err
	}
	c.ConvertTextToChat = s.ConvertTextToChat == nil || *s.ConvertTextToChat
	c.ConvertChatToResponses = s.ConvertChatToResponses == nil || *s.ConvertChatToResponses
	c.ShouldDropParams = s.ShouldDropParams == nil || *s.ShouldDropParams
	c.ShouldConvertParams = s.ShouldConvertParams == nil || *s.ShouldConvertParams
	return nil
}

// IsEnabled returns true if any compat feature is enabled
func (c Config) IsEnabled() bool {
	return c.ConvertTextToChat || c.ConvertChatToResponses || c.ShouldDropParams || c.ShouldConvertParams
}

// CompatPlugin provides LiteLLM-compatible request/response transformations.
// When enabled, it automatically converts text completion requests to chat
// completion requests for models that only support chat completions, matching
// LiteLLM's behavior. It also converts chat completion requests to responses
// for models that only support the responses endpoint.
type CompatPlugin struct {
	config        Config
	logger        schemas.Logger
	modelCatalog  *modelcatalog.ModelCatalog
	droppedParams []string
}

// Init creates a new compat plugin instance with model catalog support.
// The model catalog is used to determine if a model supports text completion or
// chat completion natively. If the model catalog is nil, the plugin will
// convert all text completion requests to chat completion and all chat
// completion requests to responses.
func Init(config Config, logger schemas.Logger, mc *modelcatalog.ModelCatalog) (*CompatPlugin, error) {
	return &CompatPlugin{
		config:       config,
		logger:       logger,
		modelCatalog: mc,
	}, nil
}

// GetName returns the plugin name
func (p *CompatPlugin) GetName() string {
	return PluginName
}

// HTTPTransportPreHook is not used for this plugin
func (p *CompatPlugin) HTTPTransportPreHook(ctx *schemas.RakshaContext, req *schemas.HTTPRequest) (*schemas.HTTPResponse, error) {
	return nil, nil
}

// HTTPTransportPostHook is not used for this plugin
func (p *CompatPlugin) HTTPTransportPostHook(ctx *schemas.RakshaContext, req *schemas.HTTPRequest, resp *schemas.HTTPResponse) error {
	return nil
}

// HTTPTransportStreamChunkHook passes through streaming chunks unchanged.
func (p *CompatPlugin) HTTPTransportStreamChunkHook(ctx *schemas.RakshaContext, req *schemas.HTTPRequest, chunk *schemas.RakshaStreamChunk) (*schemas.RakshaStreamChunk, error) {
	return chunk, nil
}

// PreRequestHook implements schemas.LLMPlugin (no-op — required for plugin indexing).
func (p *CompatPlugin) PreRequestHook(_ *schemas.RakshaContext, _ *schemas.RakshaRequest) error {
	return nil
}

// PreLLMHook intercepts requests and applies LiteLLM-compatible request normalization.
func (p *CompatPlugin) PreLLMHook(ctx *schemas.RakshaContext, req *schemas.RakshaRequest) (*schemas.RakshaRequest, *schemas.LLMPluginShortCircuit, error) {
	if ctx == nil || req == nil {
		return req, nil, nil
	}

	convertTextToChatOverride, convertTextToChatOverrideEnabled := ctx.Value(schemas.RakshaContextKeyCompatConvertTextToChat).(bool)
	convertChatToResponsesOverride, convertChatToResponsesOverrideEnabled := ctx.Value(schemas.RakshaContextKeyCompatConvertChatToResponses).(bool)
	shouldDropParamsOverride, shouldDropParamsOverrideEnabled := ctx.Value(schemas.RakshaContextKeyCompatShouldDropParams).(bool)
	shouldConvertParamsOverride, shouldConvertParamsOverrideEnabled := ctx.Value(schemas.RakshaContextKeyCompatShouldConvertParams).(bool)

	modifiedReq := req
	if (shouldDropParamsOverrideEnabled && shouldDropParamsOverride) || (shouldConvertParamsOverrideEnabled && shouldConvertParamsOverride) || p.config.ShouldConvertParams || p.config.ShouldDropParams {
		modifiedReq = cloneRakshaReq(req)
	}
	p.droppedParams = nil

	// Text completion → chat conversion
	if (convertTextToChatOverrideEnabled && convertTextToChatOverride) || p.config.ConvertTextToChat {
		if (modifiedReq.RequestType == schemas.TextCompletionRequest || modifiedReq.RequestType == schemas.TextCompletionStreamRequest) && modifiedReq.TextCompletionRequest != nil {
			p.markForConversion(ctx, modifiedReq.TextCompletionRequest.Provider, modifiedReq.TextCompletionRequest.Model, schemas.TextCompletionRequest, schemas.ChatCompletionRequest)
		}
	}

	// Chat completion → responses conversion
	if (convertChatToResponsesOverrideEnabled && convertChatToResponsesOverride) || p.config.ConvertChatToResponses {
		if (modifiedReq.RequestType == schemas.ChatCompletionRequest || modifiedReq.RequestType == schemas.ChatCompletionStreamRequest) && modifiedReq.ChatRequest != nil {
			p.markForConversion(ctx, modifiedReq.ChatRequest.Provider, modifiedReq.ChatRequest.Model, schemas.ChatCompletionRequest, schemas.ResponsesRequest)
		}
	}

	// Compute unsupported parameters to drop based on model catalog allowlist
	if ((shouldDropParamsOverrideEnabled && shouldDropParamsOverride) || p.config.ShouldDropParams) && p.modelCatalog != nil {
		_, model, _ := modifiedReq.GetRequestFields()
		if model != "" {
			if supportedParams := p.modelCatalog.GetSupportedParameters(model); supportedParams != nil {
				droppedParams := dropUnsupportedParams(ctx, modifiedReq, supportedParams)
				if len(droppedParams) > 0 {
					p.droppedParams = droppedParams
				}
			}
		}
	}

	if (shouldConvertParamsOverride && shouldConvertParamsOverrideEnabled) || p.config.ShouldConvertParams {
		applyParameterConversion(modifiedReq)
	}

	return modifiedReq, nil, nil
}

// PostLLMHook converts provider responses back to the caller-facing shape
func (p *CompatPlugin) PostLLMHook(ctx *schemas.RakshaContext, result *schemas.RakshaResponse, rakshaErr *schemas.RakshaError) (*schemas.RakshaResponse, *schemas.RakshaError, error) {
	if ctx == nil {
		return result, rakshaErr, nil
	}

	if changeType, ok := ctx.Value(schemas.RakshaContextKeyChangeRequestType).(schemas.RequestType); ok {
		if result != nil {
			extraFields := result.GetExtraFields()
			if extraFields != nil {
				extraFields.ConvertedRequestType = changeType
			}
		}
		if rakshaErr != nil {
			rakshaErr.ExtraFields.ConvertedRequestType = changeType
		}
	}

	if result != nil {
		if extraFields := result.GetExtraFields(); extraFields != nil {
			extraFields.DroppedCompatPluginParams = p.droppedParams
		}
	}

	return result, rakshaErr, nil
}

// Cleanup performs plugin cleanup.
func (p *CompatPlugin) Cleanup() error {
	return nil
}

// markForConversion checks if the model supports the current request type; if not, mark for conversion
func (p *CompatPlugin) markForConversion(ctx *schemas.RakshaContext, provider schemas.ModelProvider, model string, currentType schemas.RequestType, targetType schemas.RequestType) {
	shouldConvert := false
	if p.modelCatalog != nil {
		if !p.modelCatalog.IsRequestTypeSupported(model, provider, currentType) && p.modelCatalog.IsRequestTypeSupported(model, provider, targetType) {
			shouldConvert = true
		}
	} else {
		p.logger.Debug("compat: model calalog is nil")
	}

	if shouldConvert {
		ctx.SetValue(schemas.RakshaContextKeyChangeRequestType, targetType)
	}
}
