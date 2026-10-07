package bedrock

import (
	"strings"

	"github.com/gateway/gateway/core/providers/anthropic"
	"github.com/gateway/gateway/core/schemas"
)

// ToBedrockTextCompletionRequest converts a Gateway text completion request to Bedrock format
func ToBedrockTextCompletionRequest(gatewayReq *schemas.GatewayTextCompletionRequest) *BedrockTextCompletionRequest {
	if gatewayReq == nil || (gatewayReq.Input.PromptStr == nil && len(gatewayReq.Input.PromptArray) == 0) {
		return nil
	}

	// Extract the raw prompt from gatewayReq
	prompt := ""
	if gatewayReq.Input != nil {
		if gatewayReq.Input.PromptStr != nil {
			prompt = *gatewayReq.Input.PromptStr
		} else if len(gatewayReq.Input.PromptArray) > 0 && gatewayReq.Input.PromptArray != nil {
			prompt = strings.Join(gatewayReq.Input.PromptArray, "\n\n")
		}
	}

	bedrockReq := &BedrockTextCompletionRequest{
		Prompt: prompt,
	}

	// Apply parameters
	if gatewayReq.Params != nil {
		bedrockReq.Temperature = gatewayReq.Params.Temperature
		bedrockReq.TopP = gatewayReq.Params.TopP

		if gatewayReq.Params.ExtraParams != nil {
			bedrockReq.ExtraParams = gatewayReq.Params.ExtraParams
			if topK, ok := schemas.SafeExtractIntPointer(gatewayReq.Params.ExtraParams["top_k"]); ok {
				delete(bedrockReq.ExtraParams, "top_k")
				bedrockReq.TopK = topK
			}
		}
	}

	// Apply model-specific formatting and field naming
	if strings.Contains(gatewayReq.Model, "anthropic.") || strings.Contains(gatewayReq.Model, "claude") {
		// For Claude models, wrap the prompt in Anthropic format and use Anthropic field names
		anthropicReq := anthropic.ToAnthropicTextCompletionRequest(gatewayReq)
		bedrockReq.Prompt = anthropicReq.Prompt
		bedrockReq.MaxTokensToSample = &anthropicReq.MaxTokensToSample
		bedrockReq.StopSequences = anthropicReq.StopSequences
	} else {
		// For other models, use standard field names with raw prompt
		if gatewayReq.Params != nil {
			bedrockReq.MaxTokens = gatewayReq.Params.MaxTokens
			bedrockReq.Stop = gatewayReq.Params.Stop
		}
	}

	return bedrockReq
}

// ToGatewayTextCompletionRequest converts a Bedrock text completion request to Gateway format
func (request *BedrockTextCompletionRequest) ToGatewayTextCompletionRequest(ctx *schemas.GatewayContext) *schemas.GatewayTextCompletionRequest {
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

	gatewayReq := &schemas.GatewayTextCompletionRequest{
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
		gatewayReq.Params.MaxTokens = request.MaxTokens
	} else if request.MaxTokensToSample != nil {
		gatewayReq.Params.MaxTokens = request.MaxTokensToSample
	}

	if len(request.Stop) > 0 {
		gatewayReq.Params.Stop = request.Stop
	} else if len(request.StopSequences) > 0 {
		gatewayReq.Params.Stop = request.StopSequences
	}

	return gatewayReq
}

// ToGatewayTextCompletionResponse converts a Bedrock Anthropic text response to Gateway format
func (response *BedrockAnthropicTextResponse) ToGatewayTextCompletionResponse() *schemas.GatewayTextCompletionResponse {
	if response == nil {
		return nil
	}

	return &schemas.GatewayTextCompletionResponse{
		Object: "text_completion",
		Choices: []schemas.GatewayResponseChoice{
			{
				Index: 0,
				TextCompletionResponseChoice: &schemas.TextCompletionResponseChoice{
					Text: &response.Completion,
				},
				FinishReason: &response.StopReason,
			},
		},
		ExtraFields: schemas.GatewayResponseExtraFields{},
	}
}

// ToGatewayTextCompletionResponse converts a Bedrock Mistral text response to Gateway format
func (response *BedrockMistralTextResponse) ToGatewayTextCompletionResponse() *schemas.GatewayTextCompletionResponse {
	if response == nil {
		return nil
	}

	var choices []schemas.GatewayResponseChoice
	for i, output := range response.Outputs {
		choices = append(choices, schemas.GatewayResponseChoice{
			Index: i,
			TextCompletionResponseChoice: &schemas.TextCompletionResponseChoice{
				Text: &output.Text,
			},
			FinishReason: &output.StopReason,
		})
	}

	return &schemas.GatewayTextCompletionResponse{
		Object:      "text_completion",
		Choices:     choices,
		ExtraFields: schemas.GatewayResponseExtraFields{},
	}
}

// ToBedrockTextCompletionResponse converts a GatewayTextCompletionResponse back to Bedrock text completion format
// Returns either *BedrockAnthropicTextResponse or *BedrockMistralTextResponse based on the model
func ToBedrockTextCompletionResponse(gatewayResp *schemas.GatewayTextCompletionResponse) interface{} {
	if gatewayResp == nil {
		return nil
	}

	// Determine response format based on resolved model identity.
	// Use ResolvedModelUsed (actual provider ID) for accurate family detection,
	// falling back to gatewayResp.Model, then OriginalModelRequested as a last resort.
	model := gatewayResp.Model
	if gatewayResp.ExtraFields.ResolvedModelUsed != "" {
		model = gatewayResp.ExtraFields.ResolvedModelUsed
	} else if model == "" && gatewayResp.ExtraFields.OriginalModelRequested != "" {
		model = gatewayResp.ExtraFields.OriginalModelRequested
	}

	if strings.Contains(model, "anthropic.") || strings.Contains(model, "claude") {
		// Convert to Anthropic format
		bedrockResp := &BedrockAnthropicTextResponse{}

		// Convert choices to completion text
		if len(gatewayResp.Choices) > 0 {
			choice := gatewayResp.Choices[0] // Anthropic text API typically returns one choice
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
		for _, choice := range gatewayResp.Choices {
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
	if len(gatewayResp.Choices) > 0 {
		choice := gatewayResp.Choices[0]
		if choice.TextCompletionResponseChoice != nil && choice.TextCompletionResponseChoice.Text != nil {
			bedrockResp.Completion = *choice.TextCompletionResponseChoice.Text
		}
		if choice.FinishReason != nil {
			bedrockResp.StopReason = *choice.FinishReason
		}
	}

	return bedrockResp
}
