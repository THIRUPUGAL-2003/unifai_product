package openai

import (
	"maps"

	"github.com/raksha/raksha/core/schemas"
)

// ToOpenAITextCompletionRequest converts a Raksha text completion request to OpenAI format
func ToOpenAITextCompletionRequest(rakshaReq *schemas.RakshaTextCompletionRequest) *OpenAITextCompletionRequest {
	if rakshaReq == nil {
		return nil
	}
	params := rakshaReq.Params
	openaiReq := &OpenAITextCompletionRequest{
		Model:  rakshaReq.Model,
		Prompt: rakshaReq.Input,
	}
	if params != nil {
		openaiReq.TextCompletionParameters = *params
		// Drop user field if it exceeds OpenAI's 64 character limit
		openaiReq.TextCompletionParameters.User = SanitizeUserField(openaiReq.TextCompletionParameters.User)
		if rakshaReq.Params.ExtraParams != nil {
			openaiReq.ExtraParams = maps.Clone(rakshaReq.Params.ExtraParams)
			openaiReq.TextCompletionParameters.ExtraParams = openaiReq.ExtraParams
		}
	}
	if rakshaReq.Provider == schemas.Fireworks {
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

// ToRakshaTextCompletionRequest converts an OpenAI text completion request to Raksha format
func (req *OpenAITextCompletionRequest) ToRakshaTextCompletionRequest(ctx *schemas.RakshaContext) *schemas.RakshaTextCompletionRequest {
	if req == nil {
		return nil
	}

	provider, model := schemas.ParseModelString(req.Model, "")

	return &schemas.RakshaTextCompletionRequest{
		Provider:  provider,
		Model:     model,
		Input:     req.Prompt,
		Params:    &req.TextCompletionParameters,
		Fallbacks: schemas.ParseFallbacks(req.Fallbacks),
	}
}
