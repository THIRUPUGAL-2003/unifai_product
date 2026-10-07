package openai

import (
	"github.com/gateway/gateway/core/schemas"
)

// ToGatewaySpeechRequest converts an OpenAI speech request to Gateway format
func (request *OpenAISpeechRequest) ToGatewaySpeechRequest(ctx *schemas.GatewayContext) *schemas.GatewaySpeechRequest {
	provider, model := schemas.ParseModelString(request.Model, "")

	return &schemas.GatewaySpeechRequest{
		Provider:  provider,
		Model:     model,
		Input:     &schemas.SpeechInput{Input: request.Input},
		Params:    &request.SpeechParameters,
		Fallbacks: schemas.ParseFallbacks(request.Fallbacks),
	}
}

// ToOpenAISpeechRequest converts a Gateway speech request to OpenAI format
func ToOpenAISpeechRequest(gatewayReq *schemas.GatewaySpeechRequest) *OpenAISpeechRequest {
	if gatewayReq == nil || gatewayReq.Input.Input == "" {
		return nil
	}

	speechInput := gatewayReq.Input
	params := gatewayReq.Params

	openaiReq := &OpenAISpeechRequest{
		Model: gatewayReq.Model,
		Input: speechInput.Input,
	}

	if params != nil {
		openaiReq.SpeechParameters = *params
	}

	if gatewayReq.Params != nil {
		openaiReq.ExtraParams = gatewayReq.Params.ExtraParams
	}
	return openaiReq
}
