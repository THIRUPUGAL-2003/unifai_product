package cohere

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gateway/gateway/core/schemas"
)

// ToGatewayResponsesRequest converts a Cohere count tokens request to Gateway format.
func (req *CohereCountTokensRequest) ToGatewayResponsesRequest(ctx *schemas.GatewayContext) *schemas.GatewayResponsesRequest {
	if req == nil {
		return nil
	}

	provider, model := schemas.ParseModelString(req.Model, "")

	userRole := schemas.ResponsesInputMessageRoleUser
	return &schemas.GatewayResponsesRequest{
		Provider: provider,
		Model:    model,
		Input: []schemas.ResponsesMessage{
			{
				Role: &userRole,
				Content: &schemas.ResponsesMessageContent{
					ContentStr: &req.Text,
				},
			},
		},
	}
}

// ToCohereCountTokensRequest converts a Gateway count tokens request to Cohere's tokenize payload.
func ToCohereCountTokensRequest(gatewayReq *schemas.GatewayResponsesRequest) (*CohereCountTokensRequest, error) {
	if gatewayReq == nil {
		return nil, nil
	}

	if gatewayReq.Input == nil {
		return nil, fmt.Errorf("count tokens input is not provided")
	}

	text := buildCohereCountTokensText(gatewayReq.Input)
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, fmt.Errorf("count tokens text is empty after conversion")
	}
	runeCount := utf8.RuneCountInString(trimmed)
	if runeCount < cohereTokenizeMinTextLength || runeCount > cohereTokenizeMaxTextLength {
		return nil, fmt.Errorf("count tokens text length must be between %d and %d characters", cohereTokenizeMinTextLength, cohereTokenizeMaxTextLength)
	}

	cohereReq := &CohereCountTokensRequest{
		Model: gatewayReq.Model,
		Text:  trimmed,
	}
	if gatewayReq.Params != nil {
		cohereReq.ExtraParams = gatewayReq.Params.ExtraParams
	}

	return cohereReq, nil
}

// ToGatewayCountTokensResponse converts a Cohere tokenize response to Gateway format.
func (resp *CohereCountTokensResponse) ToGatewayCountTokensResponse(model string) *schemas.GatewayCountTokensResponse {
	if resp == nil {
		return nil
	}

	inputTokens := len(resp.Tokens)
	if inputTokens == 0 && len(resp.TokenStrings) > 0 {
		inputTokens = len(resp.TokenStrings)
	}
	totalTokens := inputTokens

	return &schemas.GatewayCountTokensResponse{
		Model:        model,
		InputTokens:  inputTokens,
		TotalTokens:  &totalTokens,
		TokenStrings: resp.TokenStrings,
		Tokens:       resp.Tokens,
		Object:       "response.input_tokens",
	}
}

// buildCohereCountTokensText flattens Responses messages into a plain text payload for tokenization.
func buildCohereCountTokensText(messages []schemas.ResponsesMessage) string {
	var parts []string

	for _, msg := range messages {
		var contentParts []string

		if msg.Content != nil {
			if msg.Content.ContentStr != nil {
				contentParts = append(contentParts, *msg.Content.ContentStr)
			}
			for _, block := range msg.Content.ContentBlocks {
				if block.Text != nil {
					contentParts = append(contentParts, *block.Text)
				}
				if block.ResponsesOutputMessageContentRefusal != nil && block.ResponsesOutputMessageContentRefusal.Refusal != "" {
					contentParts = append(contentParts, block.ResponsesOutputMessageContentRefusal.Refusal)
				}
			}
		}

		if msg.ResponsesReasoning != nil {
			for _, summary := range msg.ResponsesReasoning.Summary {
				if summary.Text != "" {
					contentParts = append(contentParts, summary.Text)
				}
			}
		}

		if len(contentParts) == 0 {
			continue
		}

		parts = append(parts, strings.Join(contentParts, "\n"))
	}

	return strings.TrimSpace(strings.Join(parts, "\n"))
}
