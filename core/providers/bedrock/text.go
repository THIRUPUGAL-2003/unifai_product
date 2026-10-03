package bedrock

import (
	"strings"

	"github.com/raksha/raksha/core/providers/anthropic"
	"github.com/raksha/raksha/core/schemas"
)

// ToBedrockTextCompletionRequest converts a Raksha text completion request to Bedrock format
func ToBedrockTextCompletionRequest(rakshaReq *schemas.RakshaTextCompletionRequest) *BedrockTextCompletionRequest {
	if rakshaReq == nil || (rakshaReq.Input.PromptStr == nil && len(rakshaReq.Input.PromptArray) == 0) {
		return nil
	}

	// Extract the raw prompt from rakshaReq
	prompt := ""
	if rakshaReq.Input != nil {
		if rakshaReq.Input.PromptStr != nil {
			prompt = *rakshaReq.Input.PromptStr
		} else if len(rakshaReq.Input.PromptArray) > 0 && rakshaReq.Input.PromptArray != nil {
			prompt = strings.Join(rakshaReq.Input.PromptArray, "\n\n")
		}
	}

	bedrockReq := &BedrockTextCompletionRequest{
		Prompt: prompt,
	}

	// Apply parameters
	if rakshaReq.Params != nil {
		bedrockReq.Temperature = rakshaReq.Params.Temperature
		bedrockReq.TopP = rakshaReq.Params.TopP

		if rakshaReq.Params.ExtraParams != nil {
			bedrockReq.ExtraParams = rakshaReq.Params.ExtraParams
			if topK, ok := schemas.SafeExtractIntPointer(rakshaReq.Params.ExtraParams["top_k"]); ok {
				delete(bedrockReq.ExtraParams, "top_k")
				bedrockReq.TopK = topK
			}
		}
	}

	// Apply model-specific formatting and field naming
	if strings.Contains(rakshaReq.Model, "anthropic.") || strings.Contains(rakshaReq.Model, "claude") {
		// For Claude models, wrap the prompt in Anthropic format and use Anthropic field names
		anthropicReq := anthropic.ToAnthropicTextCompletionRequest(rakshaReq)
		bedrockReq.Prompt = anthropicReq.Prompt
		bedrockReq.MaxTokensToSample = &anthropicReq.MaxTokensToSample
		bedrockReq.StopSequences = anthropicReq.StopSequences
	} else {
		// For other models, use standard field names with raw prompt
		if rakshaReq.Params != nil {
			bedrockReq.MaxTokens = rakshaReq.Params.MaxTokens
			bedrockReq.Stop = rakshaReq.Params.Stop
		}
	}

	return bedrockReq
}

// ToRakshaTextCompletionRequest converts a Bedrock text completion request to Raksha format
func (request *BedrockTextCompletionRequest) ToRakshaTextCompletionRequest(ctx *schemas.RakshaContext) *schemas.RakshaTextCompletionRequest {
	if request == nil {
		return nil
	}

	prompt := request.Prompt
	// Fallback for Claude 3 Messages API
	if prompt == "" && len(request.Messages) > 0 {
		var parts []string
		for _, msg := range request.Messages {
			for _, content := range msg.Content {
				if content.Text != nil {
					parts = append(parts, *content.Text)
				}
			}
		}
		prompt = strings.Join(parts, "\n\n")
	}

	provider, model := schemas.ParseModelString(request.ModelID, "")

	rakshaReq := &schemas.RakshaTextCompletionRequest{
		Provider: provider,
		Model:    model,
		Input: &schemas.TextCompletionInput{
			PromptStr: &prompt,
		},
		Params: &schemas.TextCompletionParameters{
			Temperature: request.Temperature,
			TopP:        request.TopP,
		},
	}

	if request.MaxTokens != nil {
		rakshaReq.Params.MaxTokens = request.MaxTokens
	} else if request.MaxTokensToSample != nil {
		rakshaReq.Params.MaxTokens = request.MaxTokensToSample
	}

	if len(request.Stop) > 0 {
		rakshaReq.Params.Stop = request.Stop
	} else if len(request.StopSequences) > 0 {
		rakshaReq.Params.Stop = request.StopSequences
	}

	return rakshaReq
}

// ToRakshaTextCompletionResponse converts a Bedrock Anthropic text response to Raksha format
func (response *BedrockAnthropicTextResponse) ToRakshaTextCompletionResponse() *schemas.RakshaTextCompletionResponse {
	if response == nil {
		return nil
	}

	return &schemas.RakshaTextCompletionResponse{
		Object: "text_completion",
		Choices: []schemas.RakshaResponseChoice{
			{
				Index: 0,
				TextCompletionResponseChoice: &schemas.TextCompletionResponseChoice{
					Text: &response.Completion,
				},
				FinishReason: &response.StopReason,
			},
		},
		ExtraFields: schemas.RakshaResponseExtraFields{},
	}
}

// ToRakshaTextCompletionResponse converts a Bedrock Mistral text response to Raksha format
func (response *BedrockMistralTextResponse) ToRakshaTextCompletionResponse() *schemas.RakshaTextCompletionResponse {
	if response == nil {
		return nil
	}

	var choices []schemas.RakshaResponseChoice
	for i, output := range response.Outputs {
		choices = append(choices, schemas.RakshaResponseChoice{
			Index: i,
			TextCompletionResponseChoice: &schemas.TextCompletionResponseChoice{
				Text: &output.Text,
			},
			FinishReason: &output.StopReason,
		})
	}

	return &schemas.RakshaTextCompletionResponse{
		Object:      "text_completion",
		Choices:     choices,
		ExtraFields: schemas.RakshaResponseExtraFields{},
	}
}

// ToBedrockTextCompletionResponse converts a RakshaTextCompletionResponse back to Bedrock text completion format
// Returns either *BedrockAnthropicTextResponse or *BedrockMistralTextResponse based on the model
func ToBedrockTextCompletionResponse(rakshaResp *schemas.RakshaTextCompletionResponse) interface{} {
	if rakshaResp == nil {
		return nil
	}

	// Determine response format based on resolved model identity.
	// Use ResolvedModelUsed (actual provider ID) for accurate family detection,
	// falling back to rakshaResp.Model, then OriginalModelRequested as a last resort.
	model := rakshaResp.Model
	if rakshaResp.ExtraFields.ResolvedModelUsed != "" {
		model = rakshaResp.ExtraFields.ResolvedModelUsed
	} else if model == "" && rakshaResp.ExtraFields.OriginalModelRequested != "" {
		model = rakshaResp.ExtraFields.OriginalModelRequested
	}

	if strings.Contains(model, "anthropic.") || strings.Contains(model, "claude") {
		// Convert to Anthropic format
		bedrockResp := &BedrockAnthropicTextResponse{}

		// Convert choices to completion text
		if len(rakshaResp.Choices) > 0 {
			choice := rakshaResp.Choices[0] // Anthropic text API typically returns one choice
			if choice.TextCompletionResponseChoice != nil && choice.TextCompletionResponseChoice.Text != nil {
				bedrockResp.Completion = *choice.TextCompletionResponseChoice.Text
			}
			if choice.FinishReason != nil {
				bedrockResp.StopReason = *choice.FinishReason
			}
		}

		return bedrockResp
	} else if strings.Contains(model, "mistral.") {
		// Convert to Mistral format
		bedrockResp := &BedrockMistralTextResponse{}

		// Convert choices to outputs
		for _, choice := range rakshaResp.Choices {
			var output struct {
				Text       string `json:"text"`
				StopReason string `json:"stop_reason"`
			}

			if choice.TextCompletionResponseChoice != nil && choice.TextCompletionResponseChoice.Text != nil {
				output.Text = *choice.TextCompletionResponseChoice.Text
			}
			if choice.FinishReason != nil {
				output.StopReason = *choice.FinishReason
			}

			bedrockResp.Outputs = append(bedrockResp.Outputs, output)
		}

		return bedrockResp
	}

	// Default to Anthropic format if model type cannot be determined
	bedrockResp := &BedrockAnthropicTextResponse{}
	if len(rakshaResp.Choices) > 0 {
		choice := rakshaResp.Choices[0]
		if choice.TextCompletionResponseChoice != nil && choice.TextCompletionResponseChoice.Text != nil {
			bedrockResp.Completion = *choice.TextCompletionResponseChoice.Text
		}
		if choice.FinishReason != nil {
			bedrockResp.StopReason = *choice.FinishReason
		}
	}

	return bedrockResp
}
