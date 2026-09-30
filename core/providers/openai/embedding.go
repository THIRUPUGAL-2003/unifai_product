package openai

import (
	"github.com/unifai/unifai/core/schemas"
)

// ToUnifAIEmbeddingRequest converts an OpenAI embedding request to UnifAI format
func (request *OpenAIEmbeddingRequest) ToUnifAIEmbeddingRequest(ctx *schemas.UnifAIContext) *schemas.UnifAIEmbeddingRequest {
	provider, model := schemas.ParseModelString(request.Model, "")

	return &schemas.UnifAIEmbeddingRequest{
		Provider:  provider,
		Model:     model,
		Input:     request.Input,
		Params:    &request.EmbeddingParameters,
		Fallbacks: schemas.ParseFallbacks(request.Fallbacks),
	}
}

// ToOpenAIEmbeddingRequest converts a UnifAI embedding request to OpenAI format
func ToOpenAIEmbeddingRequest(unifaiReq *schemas.UnifAIEmbeddingRequest) *OpenAIEmbeddingRequest {
	if unifaiReq == nil {
		return nil
	}

	params := unifaiReq.Params

	openaiReq := &OpenAIEmbeddingRequest{
		Model: unifaiReq.Model,
		Input: unifaiReq.Input,
	}

	// Map parameters
	if params != nil {
		openaiReq.EmbeddingParameters = *params
		openaiReq.ExtraParams = params.ExtraParams
	}
	return openaiReq
}
