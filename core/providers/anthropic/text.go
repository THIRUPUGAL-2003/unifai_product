package anthropic

import (
	"fmt"
	"strings"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
)

// ToAnthropicTextCompletionRequest converts a Raksha text completion request to Anthropic format
func ToAnthropicTextCompletionRequest(rakshaReq *schemas.RakshaTextCompletionRequest) *AnthropicTextRequest {
	if rakshaReq == nil {
		return nil
	}

	prompt := ""
	if rakshaReq.Input.PromptStr != nil {
		prompt = *rakshaReq.Input.PromptStr
	} else if len(rakshaReq.Input.PromptArray) > 0 {
		prompt = strings.Join(rakshaReq.Input.PromptArray, "\n\n")
	}

	anthropicReq := &AnthropicTextRequest{
		Model:             rakshaReq.Model,
		Prompt:            fmt.Sprintf("\n\nHuman: %s\n\nAssistant:", prompt),
		MaxTokensToSample: providerUtils.GetMaxOutputTokensOrDefault(rakshaReq.Model, AnthropicDefaultMaxTokens),
	}

	// Convert parameters
	if rakshaReq.Params != nil {
		if rakshaReq.Params.MaxTokens != nil {
			anthropicReq.MaxTokensToSample = *rakshaReq.Params.MaxTokens
		}
		anthropicReq.Temperature = rakshaReq.Params.Temperature
		anthropicReq.TopP = rakshaReq.Params.TopP
		anthropicReq.StopSequences = rakshaReq.Params.Stop

		if rakshaReq.Params.ExtraParams != nil {
			anthropicReq.ExtraParams = rakshaReq.Params.ExtraParams
			if topK, ok := schemas.SafeExtractIntPointer(rakshaReq.Params.ExtraParams["top_k"]); ok {
				delete(anthropicReq.ExtraParams, "top_k")
				anthropicReq.TopK = topK
			}
		}
	}

	return anthropicReq
}

// ToRakshaTextCompletionRequest converts an Anthropic text request back to Raksha format
func (req *AnthropicTextRequest) ToRakshaTextCompletionRequest(ctx *schemas.RakshaContext) *schemas.RakshaTextCompletionRequest {
	if req == nil {
		return nil
	}

	provider, model := schemas.ParseModelString(req.Model, "")

	rakshaReq := &schemas.RakshaTextCompletionRequest{
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
		rakshaReq.Params.ExtraParams = map[string]interface{}{
			"top_k": *req.TopK,
		}
	}

	return rakshaReq
}

// ToRakshaTextCompletionResponse converts an Anthropic text response back to Raksha format
func (response *AnthropicTextResponse) ToRakshaTextCompletionResponse() *schemas.RakshaTextCompletionResponse {
	if response == nil {
		return nil
	}
	return &schemas.RakshaTextCompletionResponse{
		ID:     response.ID,
		Object: "text_completion",
		Choices: []schemas.RakshaResponseChoice{
			{
				Index: 0,
				TextCompletionResponseChoice: &schemas.TextCompletionResponseChoice{
					Text: &response.Completion,
				},
			},
		},
		Usage: &schemas.RakshaLLMUsage{
			PromptTokens:     response.Usage.InputTokens,
			CompletionTokens: response.Usage.OutputTokens,
			TotalTokens:      response.Usage.InputTokens + response.Usage.OutputTokens,
		},
		Model: response.Model,
	}
}

// ToAnthropicTextCompletionResponse converts a RakshaResponse back to Anthropic text completion format
func ToAnthropicTextCompletionResponse(rakshaResp *schemas.RakshaTextCompletionResponse) *AnthropicTextResponse {
	if rakshaResp == nil {
		return nil
	}

	anthropicResp := &AnthropicTextResponse{
		ID:    rakshaResp.ID,
		Type:  "completion",
		Model: rakshaResp.Model,
	}

	// Convert choices to completion text
	if len(rakshaResp.Choices) > 0 {
		choice := rakshaResp.Choices[0] // Anthropic text API typically returns one choice

		if choice.TextCompletionResponseChoice != nil && choice.TextCompletionResponseChoice.Text != nil {
			anthropicResp.Completion = *choice.TextCompletionResponseChoice.Text
		}
	}

	// Convert usage information
	if rakshaResp.Usage != nil {
		anthropicResp.Usage.InputTokens = rakshaResp.Usage.PromptTokens
		anthropicResp.Usage.OutputTokens = rakshaResp.Usage.CompletionTokens
	}

	return anthropicResp
}
