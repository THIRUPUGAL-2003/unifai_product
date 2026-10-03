package replicate

import (
	"fmt"
	"strings"

	schemas "github.com/raksha/raksha/core/schemas"
)

func ToReplicateTextRequest(rakshaReq *schemas.RakshaTextCompletionRequest) (*ReplicatePredictionRequest, error) {
	if rakshaReq == nil || rakshaReq.Input == nil {
		return nil, fmt.Errorf("raksha request is nil or prompt is nil")
	}

	input := &ReplicatePredictionRequestInput{}
	if rakshaReq.Input.PromptStr != nil {
		input.Prompt = rakshaReq.Input.PromptStr
	} else if len(rakshaReq.Input.PromptArray) > 0 {
		prompt := strings.Join(rakshaReq.Input.PromptArray, "\n")
		input.Prompt = &prompt
	}

	// Map parameters if present
	if rakshaReq.Params != nil {
		params := rakshaReq.Params

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

	if isVersionID(rakshaReq.Model) {
		req.Version = &rakshaReq.Model
	}

	if rakshaReq.Params != nil && rakshaReq.Params.ExtraParams != nil {
		if webhook, ok := schemas.SafeExtractStringPointer(rakshaReq.Params.ExtraParams["webhook"]); ok {
			req.Webhook = webhook
		}
		if webhookEventsFilter, ok := schemas.SafeExtractStringSlice(rakshaReq.Params.ExtraParams["webhook_events_filter"]); ok {
			req.WebhookEventsFilter = webhookEventsFilter
		}
	}

	return req, nil
}

// ToRakshaTextCompletionResponse converts a Replicate prediction response to Raksha format
func (response *ReplicatePredictionResponse) ToRakshaTextCompletionResponse() *schemas.RakshaTextCompletionResponse {
	if response == nil {
		return nil
	}

	// Initialize Raksha response
	rakshaResponse := &schemas.RakshaTextCompletionResponse{
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
	choice := schemas.RakshaResponseChoice{
		Index: 0,
		TextCompletionResponseChoice: &schemas.TextCompletionResponseChoice{
			Text: textOutput,
		},
		FinishReason: finishReason,
	}

	rakshaResponse.Choices = []schemas.RakshaResponseChoice{choice}

	// Extract usage information from logs
	if response.Logs != nil {
		inputTokens, outputTokens, totalTokens, found := parseTokenUsageFromLogs(response.Logs, schemas.TextCompletionRequest)
		if found {
			rakshaResponse.Usage = &schemas.RakshaLLMUsage{
				PromptTokens:     inputTokens,
				CompletionTokens: outputTokens,
				TotalTokens:      totalTokens,
			}
		}
	}

	return rakshaResponse
}
