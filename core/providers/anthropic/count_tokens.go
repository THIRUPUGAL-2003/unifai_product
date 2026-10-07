package anthropic

import (
	"github.com/gateway/gateway/core/schemas"
)

// ToGatewayCountTokensResponse converts an Anthropic count tokens response to Gateway format
func (resp *AnthropicCountTokensResponse) ToGatewayCountTokensResponse(model string) *schemas.GatewayCountTokensResponse {
	if resp == nil {
		return nil
	}

	totalTokens := resp.InputTokens

	gatewayResp := &schemas.GatewayCountTokensResponse{
		Model:       model,
		InputTokens: resp.InputTokens,
		TotalTokens: &totalTokens,
		Object:      "response.input_tokens",
	}

	return gatewayResp
}

// ToAnthropicCountTokensResponse converts a Gateway count tokens response to Anthropic format.
func ToAnthropicCountTokensResponse(gatewayResp *schemas.GatewayCountTokensResponse) *AnthropicCountTokensResponse {
	if gatewayResp == nil {
		return nil
	}

	return &AnthropicCountTokensResponse{
		InputTokens: gatewayResp.InputTokens,
	}
}
