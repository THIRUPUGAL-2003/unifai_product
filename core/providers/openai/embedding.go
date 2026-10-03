package openai

import (
	"github.com/raksha/raksha/core/schemas"
)

// ToRakshaEmbeddingRequest converts an OpenAI embedding request to Raksha format
func (request *OpenAIEmbeddingRequest) ToRakshaEmbeddingRequest(ctx *schemas.RakshaContext) *schemas.RakshaEmbeddingRequest {
	provider, model := schemas.ParseModelString(request.Model, "")

	return &schemas.RakshaEmbeddingRequest{
		Provider:  provider,
		Model:     model,
		Input:     request.Input,
		Params:    &request.EmbeddingParameters,
		Fallbacks: schemas.ParseFallbacks(request.Fallbacks),
	}
}

// ToOpenAIEmbeddingRequest converts a Raksha embedding request to OpenAI format
func ToOpenAIEmbeddingRequest(rakshaReq *schemas.RakshaEmbeddingRequest) *OpenAIEmbeddingRequest {
	if rakshaReq == nil {
		return nil
	}

	params := rakshaReq.Params

	openaiReq := &OpenAIEmbeddingRequest{
		Model: rakshaReq.Model,
		Input: rakshaReq.Input,
	}

	// Map parameters
	if params != nil {
		openaiReq.EmbeddingParameters = *params
		openaiReq.ExtraParams = params.ExtraParams
	}
	return openaiReq
}
