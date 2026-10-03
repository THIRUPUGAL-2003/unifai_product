package openai

import (
	"github.com/raksha/raksha/core/schemas"
)

// ToRakshaSpeechRequest converts an OpenAI speech request to Raksha format
func (request *OpenAISpeechRequest) ToRakshaSpeechRequest(ctx *schemas.RakshaContext) *schemas.RakshaSpeechRequest {
	provider, model := schemas.ParseModelString(request.Model, "")

	return &schemas.RakshaSpeechRequest{
		Provider:  provider,
		Model:     model,
		Input:     &schemas.SpeechInput{Input: request.Input},
		Params:    &request.SpeechParameters,
		Fallbacks: schemas.ParseFallbacks(request.Fallbacks),
	}
}

// ToOpenAISpeechRequest converts a Raksha speech request to OpenAI format
func ToOpenAISpeechRequest(rakshaReq *schemas.RakshaSpeechRequest) *OpenAISpeechRequest {
	if rakshaReq == nil || rakshaReq.Input.Input == "" {
		return nil
	}

	speechInput := rakshaReq.Input
	params := rakshaReq.Params

	openaiReq := &OpenAISpeechRequest{
		Model: rakshaReq.Model,
		Input: speechInput.Input,
	}

	if params != nil {
		openaiReq.SpeechParameters = *params
	}

	if rakshaReq.Params != nil {
		openaiReq.ExtraParams = rakshaReq.Params.ExtraParams
	}
	return openaiReq
}
