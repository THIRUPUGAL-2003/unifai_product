package openai

import (
	"maps"

	"github.com/gateway/gateway/core/schemas"
)

// ToOpenAITextCompletionRequest converts a Gateway text completion request to OpenAI format
func ToOpenAITextCompletionRequest(gatewayReq *schemas.GatewayTextCompletionRequest) *OpenAITextCompletionRequest {
	if gatewayReq == nil {
		return nil
	}
	params := gatewayReq.Params
	openaiReq := &OpenAITextCompletionRequest{
		Model:  gatewayReq.Model,
		Prompt: gatewayReq.Input,
	}
	if params != nil {
		openaiReq.TextCompletionParameters = *params
		// Drop user field if it exceeds OpenAI's 64 character limit
		openaiReq.TextCompletionParameters.User = SanitizeUserField(openaiReq.TextCompletionParameters.User)
		if gatewayReq.Params.ExtraParams != nil {
			openaiReq.ExtraParams = maps.Clone(gatewayReq.Params.ExtraParams)
			openaiReq.TextCompletionParameters.ExtraParams = openaiReq.ExtraParams
		}
	}
	if gatewayReq.Provider == schemas.Fireworks {
		openaiReq.applyFireworksTextCompletionCompatibility()
	}
	return openaiReq
}

// applyFireworksTextCompletionCompatibility maps Fireworks-specific text fields.
func (req *OpenAITextCompletionRequest) applyFireworksTextCompletionCompatibility() {
	if req == nil || req.ExtraParams == nil {
		return
	}

	// Fireworks uses prompt_cache_isolation_key for text-completion cache isolation.
	if req.PromptCacheIsolationKey == nil {
		if value, ok := req.ExtraParams["prompt_cache_key"]; ok {
			switch typed := value.(type) {
			case string:
				if typed != "" {
					req.PromptCacheIsolationKey = &typed
				}
			case *string:
				if typed != nil && *typed != "" {
					req.PromptCacheIsolationKey = typed
				}
			}
		}
	}
	delete(req.ExtraParams, "prompt_cache_key")
	req.TextCompletionParameters.ExtraParams = req.ExtraParams
}

// ToGatewayTextCompletionRequest converts an OpenAI text completion request to Gateway format
func (req *OpenAITextCompletionRequest) ToGatewayTextCompletionRequest(ctx *schemas.GatewayContext) *schemas.GatewayTextCompletionRequest {
	if req == nil {
		return nil
	}

	provider, model := schemas.ParseModelString(req.Model, "")

	return &schemas.GatewayTextCompletionRequest{
		Provider:  provider,
		Model:     model,
		Input:     req.Prompt,
		Params:    &req.TextCompletionParameters,
		Fallbacks: schemas.ParseFallbacks(req.Fallbacks),
	}
}
