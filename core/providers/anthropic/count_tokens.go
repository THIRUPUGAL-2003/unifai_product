package anthropic

import (
	"github.com/raksha/raksha/core/schemas"
)

// ToRakshaCountTokensResponse converts an Anthropic count tokens response to Raksha format
func (resp *AnthropicCountTokensResponse) ToRakshaCountTokensResponse(model string) *schemas.RakshaCountTokensResponse {
	if resp == nil {
		return nil
	}

	totalTokens := resp.InputTokens

	rakshaResp := &schemas.RakshaCountTokensResponse{
		Model:       model,
		InputTokens: resp.InputTokens,
		TotalTokens: &totalTokens,
		Object:      "response.input_tokens",
	}

	return rakshaResp
}

// ToAnthropicCountTokensResponse converts a Raksha count tokens response to Anthropic format.
func ToAnthropicCountTokensResponse(rakshaResp *schemas.RakshaCountTokensResponse) *AnthropicCountTokensResponse {
	if rakshaResp == nil {
		return nil
	}

	return &AnthropicCountTokensResponse{
		InputTokens: rakshaResp.InputTokens,
	}
}
