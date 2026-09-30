package replicate

import (
	"fmt"
	"strconv"
	"strings"

	schemas "github.com/unifai/unifai/core/schemas"
)

func ToReplicateVideoGenerationInput(unifaiReq *schemas.UnifAIVideoGenerationRequest) (*ReplicatePredictionRequest, error) {
	if unifaiReq == nil || unifaiReq.Input == nil {
		return nil, fmt.Errorf("unifai request or input is nil")
	}

	input := &ReplicatePredictionRequestInput{
		Prompt: &unifaiReq.Input.Prompt,
	}

	if unifaiReq.Input.InputReference != nil {
		// convert input reference to base64
		// if provider is openai, set input reference to base64
		sanitizedURL, err := schemas.SanitizeImageURL(*unifaiReq.Input.InputReference)
		if err != nil {
			return nil, fmt.Errorf("invalid input reference: %w", err)
		}
		if strings.HasPrefix(unifaiReq.Model, string(schemas.OpenAI)) {
			input.InputReference = schemas.Ptr(sanitizedURL)
		} else {
			input.Image = schemas.Ptr(sanitizedURL)
		}
	}

	// Map parameters if available
	if unifaiReq.Params != nil {
		params := unifaiReq.Params

		if params.Seconds != nil {
			seconds, err := strconv.Atoi(*params.Seconds)
			if err != nil {
				return nil, fmt.Errorf("invalid seconds value: %w", err)
			}
			input.Duration = &seconds
		}

		if params.Seed != nil {
			input.Seed = params.Seed
		}

		if params.NegativePrompt != nil {
			input.NegativePrompt = params.NegativePrompt
		}

		if params.ExtraParams != nil {
			input.ExtraParams = params.ExtraParams
		}
	}

	request := &ReplicatePredictionRequest{
		Input: input,
	}

	// Check if model is a version ID and set version field accordingly
	if isVersionID(unifaiReq.Model) {
		request.Version = &unifaiReq.Model
	}

	if unifaiReq.Params != nil && unifaiReq.Params.ExtraParams != nil {
		request.ExtraParams = unifaiReq.Params.ExtraParams
		if webhook, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["webhook"]); ok {
			delete(request.ExtraParams, "webhook")
			request.Webhook = webhook
		}
		if webhookEventsFilter, ok := schemas.SafeExtractStringSlice(unifaiReq.Params.ExtraParams["webhook_events_filter"]); ok {
			delete(request.ExtraParams, "webhook_events_filter")
			request.WebhookEventsFilter = webhookEventsFilter
		}
	}

	return request, nil
}

func ToUnifAIVideoGenerationResponse(prediction *ReplicatePredictionResponse) (*schemas.UnifAIVideoGenerationResponse, *schemas.UnifAIError) {
	if prediction == nil {
		return nil, &schemas.UnifAIError{
			IsUnifAIError: true,
			Error: &schemas.ErrorField{
				Message: "prediction response is nil",
			},
		}
	}

	response := &schemas.UnifAIVideoGenerationResponse{
		ID:        prediction.ID,
		CreatedAt: ParseReplicateTimestamp(prediction.CreatedAt),
		Model:     prediction.Model,
		Object:    "video",
	}

	// Map Replicate status to UnifAI video status.
	switch prediction.Status {
	case ReplicatePredictionStatusStarting:
		response.Status = schemas.VideoStatusQueued
	case ReplicatePredictionStatusProcessing:
		response.Status = schemas.VideoStatusInProgress
	case ReplicatePredictionStatusSucceeded:
		response.Status = schemas.VideoStatusCompleted
	case ReplicatePredictionStatusFailed, ReplicatePredictionStatusCanceled:
		response.Status = schemas.VideoStatusFailed
	default:
		response.Status = schemas.VideoStatusQueued
	}

	// Surface provider error details on failed terminal states.
	if response.Status == schemas.VideoStatusFailed {
		errorMsg := "prediction failed"
		errorCode := string(prediction.Status)
		if prediction.Error != nil && *prediction.Error != "" {
			errorMsg = *prediction.Error
		}
		response.Error = &schemas.VideoCreateError{
			Code:    errorCode,
			Message: errorMsg,
		}
	}

	if prediction.CompletedAt != nil {
		response.CompletedAt = schemas.Ptr(ParseReplicateTimestamp(*prediction.CompletedAt))
	}

	// Convert output to ImageData
	// Replicate output can be either a string (single URL) or array of strings
	if prediction.Output != nil {
		if prediction.Output.OutputStr != nil && *prediction.Output.OutputStr != "" {
			response.Videos = append(response.Videos, schemas.VideoOutput{
				Type:        schemas.VideoOutputTypeURL,
				URL:         schemas.Ptr(*prediction.Output.OutputStr),
				ContentType: "video/mp4",
			})
		} else if len(prediction.Output.OutputArray) > 0 {
			for _, url := range prediction.Output.OutputArray {
				response.Videos = append(response.Videos, schemas.VideoOutput{
					Type:        schemas.VideoOutputTypeURL,
					URL:         schemas.Ptr(url),
					ContentType: "video/mp4",
				})
			}
		}
	}

	return response, nil
}
