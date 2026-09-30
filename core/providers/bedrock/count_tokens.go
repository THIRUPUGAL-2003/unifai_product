package bedrock

import (
	"strings"

	"github.com/unifai/unifai/core/schemas"
)

const estimatedBytesPerToken = 4

// ToUnifAICountTokensResponse converts a Bedrock count tokens response to UnifAI format
func (resp *BedrockCountTokensResponse) ToUnifAICountTokensResponse(model string) *schemas.UnifAICountTokensResponse {
	if resp == nil {
		return nil
	}

	totalTokens := resp.InputTokens

	return &schemas.UnifAICountTokensResponse{
		Model:       model,
		InputTokens: resp.InputTokens,
		TotalTokens: &totalTokens,
		Object:      "response.input_tokens",
	}
}

// ToBedrockCountTokensResponse converts a UnifAI count tokens response to Bedrock native format
func ToBedrockCountTokensResponse(resp *schemas.UnifAICountTokensResponse) *BedrockCountTokensResponse {
	if resp == nil {
		return nil
	}

	return &BedrockCountTokensResponse{
		InputTokens: resp.InputTokens,
	}
}

// isCountTokensUnsupported checks whether a UnifAIError indicates that the
// Bedrock model does not support the count-tokens operation.
func isCountTokensUnsupported(err *schemas.UnifAIError) bool {
	if err == nil || err.Error == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error.Message), "doesn't support counting tokens")
}

// estimateTokenCount returns a rough token count derived from the byte length
// of the serialized request body. Claude's tokenizer averages ~4 bytes per
// token on mixed content; this intentionally rounds up so that context-window
// management decisions stay on the conservative side.
func estimateTokenCount(requestBody []byte) int {
	n := len(requestBody)
	if n == 0 {
		return 0
	}
	return (n + estimatedBytesPerToken - 1) / estimatedBytesPerToken
}
