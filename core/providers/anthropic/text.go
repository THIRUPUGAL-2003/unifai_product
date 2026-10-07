package anthropic

import (
	"fmt"
	"strings"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
)

// ToAnthropicTextCompletionRequest converts a Gateway text completion request to Anthropic format
func ToAnthropicTextCompletionRequest(gatewayReq *schemas.GatewayTextCompletionRequest) *AnthropicTextRequest {
	if gatewayReq == nil {
		return nil
	}

	prompt := ""
	if gatewayReq.Input.PromptStr != nil {
		prompt = *gatewayReq.Input.PromptStr
	} else if len(gatewayReq.Input.PromptArray) > 0 {
		prompt = strings.Join(gatewayReq.Input.PromptArray, "\n\n")
	}

	anthropicReq := &AnthropicTextRequest{
		Model:             gatewayReq.Model,
		Prompt:            fmt.Sprintf("\n\nHuman: %s\n\nAssistant:", prompt),
		MaxTokensToSample: providerUtils.GetMaxOutputTokensOrDefault(gatewayReq.Model, AnthropicDefaultMaxTokens),
	}

	// Convert parameters
	if gatewayReq.Params != nil {
		if gatewayReq.Params.MaxTokens != nil {
			anthropicReq.MaxTokensToSample = *gatewayReq.Params.MaxTokens
		}
		anthropicReq.Temperature = gatewayReq.Params.Temperature
		anthropicReq.TopP = gatewayReq.Params.TopP
		anthropicReq.StopSequences = gatewayReq.Params.Stop

		if gatewayReq.Params.ExtraParams != nil {
			anthropicReq.ExtraParams = gatewayReq.Params.ExtraParams
			if topK, ok := schemas.SafeExtractIntPointer(gatewayReq.Params.ExtraParams["top_k"]); ok {
				delete(anthropicReq.ExtraParams, "top_k")
				anthropicReq.TopK = topK
			}
		}
	}

	return anthropicReq
}

// ToGatewayTextCompletionRequest converts an Anthropic text request back to Gateway format
func (req *AnthropicTextRequest) ToGatewayTextCompletionRequest(ctx *schemas.GatewayContext) *schemas.GatewayTextCompletionRequest {
	if req == nil {
		return nil
	}

	provider, model := schemas.ParseModelString(req.Model, "")

	gatewayReq := &schemas.GatewayTextCompletionRequest{
		Provider: provider,
		Model:    model,
		Input: &schemas.TextCompletionInput{
			PromptStr: &req.Prompt,
		},
		Params: &schemas.TextCompletionParameters{
			MaxTokens:   &req.MaxTokensToSample,
			Temperature: req.Temperature,
			TopP:        req.TopP,
			Stop:        req.StopSequences,
		},
		Fallbacks: schemas.ParseFallbacks(req.Fallbacks),
	}

	// Add extra params if present
	if req.TopK != nil {
		gatewayReq.Params.ExtraParams = map[string]interface{}{
			"top_k": *req.TopK,
		}
	}

	return gatewayReq
}

// ToGatewayTextCompletionResponse converts an Anthropic text response back to Gateway format
func (response *AnthropicTextResponse) ToGatewayTextCompletionResponse() *schemas.GatewayTextCompletionResponse {
	if response == nil {
		return nil
	}
	return &schemas.GatewayTextCompletionResponse{
		ID:     response.ID,
		Object: "text_completion",
		Choices: []schemas.GatewayResponseChoice{
			{
				Index: 0,
				TextCompletionResponseChoice: &schemas.TextCompletionResponseChoice{
					Text: &response.Completion,
				},
			},
		},
		Usage: &schemas.GatewayLLMUsage{
			PromptTokens:     response.Usage.InputTokens,
			CompletionTokens: response.Usage.OutputTokens,
			TotalTokens:      response.Usage.InputTokens + response.Usage.OutputTokens,
		},
		Model: response.Model,
	}
}

// ToAnthropicTextCompletionResponse converts a GatewayResponse back to Anthropic text completion format
func ToAnthropicTextCompletionResponse(gatewayResp *schemas.GatewayTextCompletionResponse) *AnthropicTextResponse {
	if gatewayResp == nil {
		return nil
	}

	anthropicResp := &AnthropicTextResponse{
		ID:    gatewayResp.ID,
		Type:  "completion",
		Model: gatewayResp.Model,
	}

	// Convert choices to completion text
	if len(gatewayResp.Choices) > 0 {
		choice := gatewayResp.Choices[0] // Anthropic text API typically returns one choice

		if choice.TextCompletionResponseChoice != nil && choice.TextCompletionResponseChoice.Text != nil {
			anthropicResp.Completion = *choice.TextCompletionResponseChoice.Text
		}
	}

	// Convert usage information
	if gatewayResp.Usage != nil {
		anthropicResp.Usage.InputTokens = gatewayResp.Usage.PromptTokens
		anthropicResp.Usage.OutputTokens = gatewayResp.Usage.CompletionTokens
	}

	return anthropicResp
}
