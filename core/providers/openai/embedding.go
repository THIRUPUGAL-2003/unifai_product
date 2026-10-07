package openai

import (
	"github.com/gateway/gateway/core/schemas"
)

// ToGatewayEmbeddingRequest converts an OpenAI embedding request to Gateway format
func (request *OpenAIEmbeddingRequest) ToGatewayEmbeddingRequest(ctx *schemas.GatewayContext) *schemas.GatewayEmbeddingRequest {
	provider, model := schemas.ParseModelString(request.Model, "")

	return &schemas.GatewayEmbeddingRequest{
		Provider:  provider,
		Model:     model,
		Input:     request.Input,
		Params:    &request.EmbeddingParameters,
		Fallbacks: schemas.ParseFallbacks(request.Fallbacks),
	}
}

// ToOpenAIEmbeddingRequest converts a Gateway embedding request to OpenAI format
func ToOpenAIEmbeddingRequest(gatewayReq *schemas.GatewayEmbeddingRequest) *OpenAIEmbeddingRequest {
	if gatewayReq == nil {
		return nil
	}

	params := gatewayReq.Params

	openaiReq := &OpenAIEmbeddingRequest{
		Model: gatewayReq.Model,
		Input: gatewayReq.Input,
	}

	// Map parameters
	if params != nil {
		openaiReq.EmbeddingParameters = *params
		openaiReq.ExtraParams = params.ExtraParams
	}
	return openaiReq
}
