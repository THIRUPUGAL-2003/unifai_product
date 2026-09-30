package bedrock

import (
	"strings"

	"github.com/unifai/unifai/core/providers/anthropic"
	"github.com/unifai/unifai/core/schemas"
)

// ToBedrockTextCompletionRequest converts a UnifAI text completion request to Bedrock format
func ToBedrockTextCompletionRequest(unifaiReq *schemas.UnifAITextCompletionRequest) *BedrockTextCompletionRequest {
	if unifaiReq == nil || (unifaiReq.Input.PromptStr == nil && len(unifaiReq.Input.PromptArray) == 0) {
		return nil
	}

	// Extract the raw prompt from unifaiReq
	prompt := ""
	if unifaiReq.Input != nil {
		if unifaiReq.Input.PromptStr != nil {
			prompt = *unifaiReq.Input.PromptStr
		} else if len(unifaiReq.Input.PromptArray) > 0 && unifaiReq.Input.PromptArray != nil {
			prompt = strings.Join(unifaiReq.Input.PromptArray, "\n\n")
		}
	}

	bedrockReq := &BedrockTextCompletionRequest{
		Prompt: prompt,
	}

	// Apply parameters
	if unifaiReq.Params != nil {
		bedrockReq.Temperature = unifaiReq.Params.Temperature
		bedrockReq.TopP = unifaiReq.Params.TopP

		if unifaiReq.Params.ExtraParams != nil {
			bedrockReq.ExtraParams = unifaiReq.Params.ExtraParams
			if topK, ok := schemas.SafeExtractIntPointer(unifaiReq.Params.ExtraParams["top_k"]); ok {
				delete(bedrockReq.ExtraParams, "top_k")
				bedrockReq.TopK = topK
			}
		}
	}

	// Apply model-specific formatting and field naming
	if strings.Contains(unifaiReq.Model, "anthropic.") || strings.Contains(unifaiReq.Model, "claude") {
		// For Claude models, wrap the prompt in Anthropic format and use Anthropic field names
		anthropicReq := anthropic.ToAnthropicTextCompletionRequest(unifaiReq)
		bedrockReq.Prompt = anthropicReq.Prompt
		bedrockReq.MaxTokensToSample = &anthropicReq.MaxTokensToSample
		bedrockReq.StopSequences = anthropicReq.StopSequences
	} else {
		// For other models, use standard field names with raw prompt
		if unifaiReq.Params != nil {
			bedrockReq.MaxTokens = unifaiReq.Params.MaxTokens
			bedrockReq.Stop = unifaiReq.Params.Stop
		}
	}

	return bedrockReq
}

// ToUnifAITextCompletionRequest converts a Bedrock text completion request to UnifAI format
func (request *BedrockTextCompletionRequest) ToUnifAITextCompletionRequest(ctx *schemas.UnifAIContext) *schemas.UnifAITextCompletionRequest {
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

	unifaiReq := &schemas.UnifAITextCompletionRequest{
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
		unifaiReq.Params.MaxTokens = request.MaxTokens
	} else if request.MaxTokensToSample != nil {
		unifaiReq.Params.MaxTokens = request.MaxTokensToSample
	}

	if len(request.Stop) > 0 {
		unifaiReq.Params.Stop = request.Stop
	} else if len(request.StopSequences) > 0 {
		unifaiReq.Params.Stop = request.StopSequences
	}

	return unifaiReq
}

// ToUnifAITextCompletionResponse converts a Bedrock Anthropic text response to UnifAI format
func (response *BedrockAnthropicTextResponse) ToUnifAITextCompletionResponse() *schemas.UnifAITextCompletionResponse {
	if response == nil {
		return nil
	}

	return &schemas.UnifAITextCompletionResponse{
		Object: "text_completion",
		Choices: []schemas.UnifAIResponseChoice{
			{
				Index: 0,
				TextCompletionResponseChoice: &schemas.TextCompletionResponseChoice{
					Text: &response.Completion,
				},
				FinishReason: &response.StopReason,
			},
		},
		ExtraFields: schemas.UnifAIResponseExtraFields{},
	}
}

// ToUnifAITextCompletionResponse converts a Bedrock Mistral text response to UnifAI format
func (response *BedrockMistralTextResponse) ToUnifAITextCompletionResponse() *schemas.UnifAITextCompletionResponse {
	if response == nil {
		return nil
	}

	var choices []schemas.UnifAIResponseChoice
	for i, output := range response.Outputs {
		choices = append(choices, schemas.UnifAIResponseChoice{
			Index: i,
			TextCompletionResponseChoice: &schemas.TextCompletionResponseChoice{
				Text: &output.Text,
			},
			FinishReason: &output.StopReason,
		})
	}

	return &schemas.UnifAITextCompletionResponse{
		Object:      "text_completion",
		Choices:     choices,
		ExtraFields: schemas.UnifAIResponseExtraFields{},
	}
}

// ToBedrockTextCompletionResponse converts a UnifAITextCompletionResponse back to Bedrock text completion format
// Returns either *BedrockAnthropicTextResponse or *BedrockMistralTextResponse based on the model
func ToBedrockTextCompletionResponse(unifaiResp *schemas.UnifAITextCompletionResponse) interface{} {
	if unifaiResp == nil {
		return nil
	}

	// Determine response format based on resolved model identity.
	// Use ResolvedModelUsed (actual provider ID) for accurate family detection,
	// falling back to unifaiResp.Model, then OriginalModelRequested as a last resort.
	model := unifaiResp.Model
	if unifaiResp.ExtraFields.ResolvedModelUsed != "" {
		model = unifaiResp.ExtraFields.ResolvedModelUsed
	} else if model == "" && unifaiResp.ExtraFields.OriginalModelRequested != "" {
		model = unifaiResp.ExtraFields.OriginalModelRequested
	}

	if strings.Contains(model, "anthropic.") || strings.Contains(model, "claude") {
		// Convert to Anthropic format
		bedrockResp := &BedrockAnthropicTextResponse{}

		// Convert choices to completion text
		if len(unifaiResp.Choices) > 0 {
			choice := unifaiResp.Choices[0] // Anthropic text API typically returns one choice
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
		for _, choice := range unifaiResp.Choices {
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
	if len(unifaiResp.Choices) > 0 {
		choice := unifaiResp.Choices[0]
		if choice.TextCompletionResponseChoice != nil && choice.TextCompletionResponseChoice.Text != nil {
			bedrockResp.Completion = *choice.TextCompletionResponseChoice.Text
		}
		if choice.FinishReason != nil {
			bedrockResp.StopReason = *choice.FinishReason
		}
	}

	return bedrockResp
}
