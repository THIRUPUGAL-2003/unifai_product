package anthropic

import (
	"fmt"
	"strings"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
)

// ToAnthropicTextCompletionRequest converts a UnifAI text completion request to Anthropic format
func ToAnthropicTextCompletionRequest(unifaiReq *schemas.UnifAITextCompletionRequest) *AnthropicTextRequest {
	if unifaiReq == nil {
		return nil
	}

	prompt := ""
	if unifaiReq.Input.PromptStr != nil {
		prompt = *unifaiReq.Input.PromptStr
	} else if len(unifaiReq.Input.PromptArray) > 0 {
		prompt = strings.Join(unifaiReq.Input.PromptArray, "\n\n")
	}

	anthropicReq := &AnthropicTextRequest{
		Model:             unifaiReq.Model,
		Prompt:            fmt.Sprintf("\n\nHuman: %s\n\nAssistant:", prompt),
		MaxTokensToSample: providerUtils.GetMaxOutputTokensOrDefault(unifaiReq.Model, AnthropicDefaultMaxTokens),
	}

	// Convert parameters
	if unifaiReq.Params != nil {
		if unifaiReq.Params.MaxTokens != nil {
			anthropicReq.MaxTokensToSample = *unifaiReq.Params.MaxTokens
		}
		anthropicReq.Temperature = unifaiReq.Params.Temperature
		anthropicReq.TopP = unifaiReq.Params.TopP
		anthropicReq.StopSequences = unifaiReq.Params.Stop

		if unifaiReq.Params.ExtraParams != nil {
			anthropicReq.ExtraParams = unifaiReq.Params.ExtraParams
			if topK, ok := schemas.SafeExtractIntPointer(unifaiReq.Params.ExtraParams["top_k"]); ok {
				delete(anthropicReq.ExtraParams, "top_k")
				anthropicReq.TopK = topK
			}
		}
	}

	return anthropicReq
}

// ToUnifAITextCompletionRequest converts an Anthropic text request back to UnifAI format
func (req *AnthropicTextRequest) ToUnifAITextCompletionRequest(ctx *schemas.UnifAIContext) *schemas.UnifAITextCompletionRequest {
	if req == nil {
		return nil
	}

	provider, model := schemas.ParseModelString(req.Model, "")

	unifaiReq := &schemas.UnifAITextCompletionRequest{
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
		unifaiReq.Params.ExtraParams = map[string]interface{}{
			"top_k": *req.TopK,
		}
	}

	return unifaiReq
}

// ToUnifAITextCompletionResponse converts an Anthropic text response back to UnifAI format
func (response *AnthropicTextResponse) ToUnifAITextCompletionResponse() *schemas.UnifAITextCompletionResponse {
	if response == nil {
		return nil
	}
	return &schemas.UnifAITextCompletionResponse{
		ID:     response.ID,
		Object: "text_completion",
		Choices: []schemas.UnifAIResponseChoice{
			{
				Index: 0,
				TextCompletionResponseChoice: &schemas.TextCompletionResponseChoice{
					Text: &response.Completion,
				},
			},
		},
		Usage: &schemas.UnifAILLMUsage{
			PromptTokens:     response.Usage.InputTokens,
			CompletionTokens: response.Usage.OutputTokens,
			TotalTokens:      response.Usage.InputTokens + response.Usage.OutputTokens,
		},
		Model: response.Model,
	}
}

// ToAnthropicTextCompletionResponse converts a UnifAIResponse back to Anthropic text completion format
func ToAnthropicTextCompletionResponse(unifaiResp *schemas.UnifAITextCompletionResponse) *AnthropicTextResponse {
	if unifaiResp == nil {
		return nil
	}

	anthropicResp := &AnthropicTextResponse{
		ID:    unifaiResp.ID,
		Type:  "completion",
		Model: unifaiResp.Model,
	}

	// Convert choices to completion text
	if len(unifaiResp.Choices) > 0 {
		choice := unifaiResp.Choices[0] // Anthropic text API typically returns one choice

		if choice.TextCompletionResponseChoice != nil && choice.TextCompletionResponseChoice.Text != nil {
			anthropicResp.Completion = *choice.TextCompletionResponseChoice.Text
		}
	}

	// Convert usage information
	if unifaiResp.Usage != nil {
		anthropicResp.Usage.InputTokens = unifaiResp.Usage.PromptTokens
		anthropicResp.Usage.OutputTokens = unifaiResp.Usage.CompletionTokens
	}

	return anthropicResp
}
