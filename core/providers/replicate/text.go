package replicate

import (
	"fmt"
	"strings"

	schemas "github.com/gateway/gateway/core/schemas"
)

func ToReplicateTextRequest(gatewayReq *schemas.GatewayTextCompletionRequest) (*ReplicatePredictionRequest, error) {
	if gatewayReq == nil || gatewayReq.Input == nil {
		return nil, fmt.Errorf("gateway request is nil or prompt is nil")
	}

	input := &ReplicatePredictionRequestInput{}
	if gatewayReq.Input.PromptStr != nil {
		input.Prompt = gatewayReq.Input.PromptStr
	} else if len(gatewayReq.Input.PromptArray) > 0 {
		prompt := strings.Join(gatewayReq.Input.PromptArray, "\n")
		input.Prompt = &prompt
	}

	// Map parameters if present
	if gatewayReq.Params != nil {
		params := gatewayReq.Params

		// Temperature
		if params.Temperature != nil {
			input.Temperature = params.Temperature
		}

		// Top P
		if params.TopP != nil {
			input.TopP = params.TopP
		}

		// Max tokens
		if params.MaxTokens != nil {
			input.MaxTokens = params.MaxTokens
		}

		// Presence penalty
		if params.PresencePenalty != nil {
			input.PresencePenalty = params.PresencePenalty
		}

		// Frequency penalty
		if params.FrequencyPenalty != nil {
			input.FrequencyPenalty = params.FrequencyPenalty
		}

		// Top K (from ExtraParams)
		if topK, ok := schemas.SafeExtractIntPointer(params.ExtraParams["top_k"]); ok {
			input.TopK = topK
		}

		// Seed
		if params.Seed != nil {
			input.Seed = params.Seed
		}

		if params.ExtraParams != nil {
			input.ExtraParams = params.ExtraParams
		}
	}

	// Check if model is a version ID and set version field accordingly
	req := &ReplicatePredictionRequest{
		Input: input,
	}

	if isVersionID(gatewayReq.Model) {
		req.Version = &gatewayReq.Model
	}

	if gatewayReq.Params != nil && gatewayReq.Params.ExtraParams != nil {
		if webhook, ok := schemas.SafeExtractStringPointer(gatewayReq.Params.ExtraParams["webhook"]); ok {
			req.Webhook = webhook
		}
		if webhookEventsFilter, ok := schemas.SafeExtractStringSlice(gatewayReq.Params.ExtraParams["webhook_events_filter"]); ok {
			req.WebhookEventsFilter = webhookEventsFilter
		}
	}

	return req, nil
}

// ToGatewayTextCompletionResponse converts a Replicate prediction response to Gateway format
func (response *ReplicatePredictionResponse) ToGatewayTextCompletionResponse() *schemas.GatewayTextCompletionResponse {
	if response == nil {
		return nil
	}

	// Initialize Gateway response
	gatewayResponse := &schemas.GatewayTextCompletionResponse{
		ID:     response.ID,
		Model:  response.Model,
		Object: "text_completion",
	}

	// Convert output to text
	var textOutput *string
	if response.Output != nil {
		if response.Output.OutputStr != nil {
			textOutput = response.Output.OutputStr
		} else if response.Output.OutputArray != nil {
			// Join array of strings into a single string
			joined := strings.Join(response.Output.OutputArray, "")
			textOutput = &joined
		}
	}

	// Determine finish reason based on status
	var finishReason *string
	switch response.Status {
	case ReplicatePredictionStatusSucceeded:
		finishReason = schemas.Ptr("stop")
	case ReplicatePredictionStatusFailed:
		finishReason = schemas.Ptr("error")
	case ReplicatePredictionStatusCanceled:
		finishReason = schemas.Ptr("stop")
	}

	// Create choice with text completion response choice
	choice := schemas.GatewayResponseChoice{
		Index: 0,
		TextCompletionResponseChoice: &schemas.TextCompletionResponseChoice{
			Text: textOutput,
		},
		FinishReason: finishReason,
	}

	gatewayResponse.Choices = []schemas.GatewayResponseChoice{choice}

	// Extract usage information from logs
	if response.Logs != nil {
		inputTokens, outputTokens, totalTokens, found := parseTokenUsageFromLogs(response.Logs, schemas.TextCompletionRequest)
		if found {
			gatewayResponse.Usage = &schemas.GatewayLLMUsage{
				PromptTokens:     inputTokens,
				CompletionTokens: outputTokens,
				TotalTokens:      totalTokens,
			}
		}
	}

	return gatewayResponse
}
