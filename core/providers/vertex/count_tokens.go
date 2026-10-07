package vertex

import (
	"github.com/gateway/gateway/core/schemas"
)

func (resp *VertexCountTokensResponse) ToGatewayCountTokensResponse(model string) *schemas.GatewayCountTokensResponse {
	if resp == nil {
		return nil
	}

	inputDetails := &schemas.ResponsesResponseInputTokens{}
	inputTokens := int(resp.TotalTokens) // Vertex response typically represents prompt tokens for countTokens
	total := int(resp.TotalTokens)

	if resp.CachedContentTokenCount > 0 {
		inputDetails.CachedReadTokens = int(resp.CachedContentTokenCount)
	}

	return &schemas.GatewayCountTokensResponse{
		Model:              model,
		Object:             "response.input_tokens",
		InputTokens:        inputTokens,
		InputTokensDetails: inputDetails,
		TotalTokens:        &total,
	}
}
