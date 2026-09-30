package openai

import (
	"github.com/unifai/unifai/core/schemas"
)

// ToUnifAISpeechRequest converts an OpenAI speech request to UnifAI format
func (request *OpenAISpeechRequest) ToUnifAISpeechRequest(ctx *schemas.UnifAIContext) *schemas.UnifAISpeechRequest {
	provider, model := schemas.ParseModelString(request.Model, "")

	return &schemas.UnifAISpeechRequest{
		Provider:  provider,
		Model:     model,
		Input:     &schemas.SpeechInput{Input: request.Input},
		Params:    &request.SpeechParameters,
		Fallbacks: schemas.ParseFallbacks(request.Fallbacks),
	}
}

// ToOpenAISpeechRequest converts a UnifAI speech request to OpenAI format
func ToOpenAISpeechRequest(unifaiReq *schemas.UnifAISpeechRequest) *OpenAISpeechRequest {
	if unifaiReq == nil || unifaiReq.Input.Input == "" {
		return nil
	}

	speechInput := unifaiReq.Input
	params := unifaiReq.Params

	openaiReq := &OpenAISpeechRequest{
		Model: unifaiReq.Model,
		Input: speechInput.Input,
	}

	if params != nil {
		openaiReq.SpeechParameters = *params
	}

	if unifaiReq.Params != nil {
		openaiReq.ExtraParams = unifaiReq.Params.ExtraParams
	}
	return openaiReq
}
