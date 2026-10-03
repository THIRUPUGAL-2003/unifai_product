package gemini

import (
	"strings"

	"github.com/raksha/raksha/core/schemas"
)

// ToRakshaCountTokensResponse converts a Gemini count tokens response to Raksha format.
func (resp *GeminiCountTokensResponse) ToRakshaCountTokensResponse(model string) *schemas.RakshaCountTokensResponse {
	if resp == nil {
		return nil
	}

	// Sum prompt tokens and map modality-specific counts
	inputTokens := 0
	inputDetails := &schemas.ResponsesResponseInputTokens{}

	for _, m := range resp.PromptTokensDetails {
		if m == nil {
			continue
		}
		inputTokens += int(m.TokenCount)
		mod := strings.ToLower(string(m.Modality))
		// handle audio modality
		if strings.Contains(mod, "audio") {
			inputDetails.AudioTokens += int(m.TokenCount)
		}
	}

	// Set cached tokens from top-level field if present
	if resp.CachedContentTokenCount != 0 {
		inputDetails.CachedReadTokens = int(resp.CachedContentTokenCount)
	} else if resp.CacheTokensDetails != nil {
		// If cache tokens details present, sum them
		cachedSum := 0
		for _, m := range resp.CacheTokensDetails {
			if m == nil {
				continue
			}
			cachedSum += int(m.TokenCount)
			if strings.Contains(strings.ToLower(string(m.Modality)), "audio") {
				// also populate audio tokens from cache into AudioTokens (additive)
				inputDetails.AudioTokens += int(m.TokenCount)
			}
		}
		inputDetails.CachedReadTokens = cachedSum
	}

	total := int(resp.TotalTokens)

	return &schemas.RakshaCountTokensResponse{
		Model:              model,
		Object:             "response.input_tokens",
		InputTokens:        inputTokens,
		InputTokensDetails: inputDetails,
		TotalTokens:        &total,
		ExtraFields:        schemas.RakshaResponseExtraFields{},
	}
}

// ToGeminiCountTokensResponse converts a Raksha count tokens response to Gemini format.
func ToGeminiCountTokensResponse(rakshaResp *schemas.RakshaCountTokensResponse) *GeminiCountTokensResponse {
	if rakshaResp == nil {
		return nil
	}

	response := &GeminiCountTokensResponse{
		TotalTokens: int32(rakshaResp.InputTokens),
	}

	// Map cached content token count if available
	if rakshaResp.InputTokensDetails != nil && rakshaResp.InputTokensDetails.CachedReadTokens > 0 {
		response.CachedContentTokenCount = int32(rakshaResp.InputTokensDetails.CachedReadTokens)
	} else {
		response.CachedContentTokenCount = 0
	}

	return response
}
