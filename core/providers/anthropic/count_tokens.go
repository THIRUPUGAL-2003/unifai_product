package anthropic

import (
	"github.com/unifai/unifai/core/schemas"
)

// ToUnifAICountTokensResponse converts an Anthropic count tokens response to UnifAI format
func (resp *AnthropicCountTokensResponse) ToUnifAICountTokensResponse(model string) *schemas.UnifAICountTokensResponse {
	if resp == nil {
		return nil
	}

	totalTokens := resp.InputTokens

	unifaiResp := &schemas.UnifAICountTokensResponse{
		Model:       model,
		InputTokens: resp.InputTokens,
		TotalTokens: &totalTokens,
		Object:      "response.input_tokens",
	}

	return unifaiResp
}

// ToAnthropicCountTokensResponse converts a UnifAI count tokens response to Anthropic format.
func ToAnthropicCountTokensResponse(unifaiResp *schemas.UnifAICountTokensResponse) *AnthropicCountTokensResponse {
	if unifaiResp == nil {
		return nil
	}

	return &AnthropicCountTokensResponse{
		InputTokens: unifaiResp.InputTokens,
	}
}
