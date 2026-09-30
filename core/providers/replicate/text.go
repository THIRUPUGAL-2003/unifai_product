package replicate

import (
	"fmt"
	"strings"

	schemas "github.com/unifai/unifai/core/schemas"
)

func ToReplicateTextRequest(unifaiReq *schemas.UnifAITextCompletionRequest) (*ReplicatePredictionRequest, error) {
	if unifaiReq == nil || unifaiReq.Input == nil {
		return nil, fmt.Errorf("unifai request is nil or prompt is nil")
	}

	input := &ReplicatePredictionRequestInput{}
	if unifaiReq.Input.PromptStr != nil {
		input.Prompt = unifaiReq.Input.PromptStr
	} else if len(unifaiReq.Input.PromptArray) > 0 {
		prompt := strings.Join(unifaiReq.Input.PromptArray, "\n")
		input.Prompt = &prompt
	}

	// Map parameters if present
	if unifaiReq.Params != nil {
		params := unifaiReq.Params

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

	if isVersionID(unifaiReq.Model) {
		req.Version = &unifaiReq.Model
	}

	if unifaiReq.Params != nil && unifaiReq.Params.ExtraParams != nil {
		if webhook, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["webhook"]); ok {
			req.Webhook = webhook
		}
		if webhookEventsFilter, ok := schemas.SafeExtractStringSlice(unifaiReq.Params.ExtraParams["webhook_events_filter"]); ok {
			req.WebhookEventsFilter = webhookEventsFilter
		}
	}

	return req, nil
}

// ToUnifAITextCompletionResponse converts a Replicate prediction response to UnifAI format
func (response *ReplicatePredictionResponse) ToUnifAITextCompletionResponse() *schemas.UnifAITextCompletionResponse {
	if response == nil {
		return nil
	}

	// Initialize UnifAI response
	unifaiResponse := &schemas.UnifAITextCompletionResponse{
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
	choice := schemas.UnifAIResponseChoice{
		Index: 0,
		TextCompletionResponseChoice: &schemas.TextCompletionResponseChoice{
			Text: textOutput,
		},
		FinishReason: finishReason,
	}

	unifaiResponse.Choices = []schemas.UnifAIResponseChoice{choice}

	// Extract usage information from logs
	if response.Logs != nil {
		inputTokens, outputTokens, totalTokens, found := parseTokenUsageFromLogs(response.Logs, schemas.TextCompletionRequest)
		if found {
			unifaiResponse.Usage = &schemas.UnifAILLMUsage{
				PromptTokens:     inputTokens,
				CompletionTokens: outputTokens,
				TotalTokens:      totalTokens,
			}
		}
	}

	return unifaiResponse
}
