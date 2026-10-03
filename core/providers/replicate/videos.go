package replicate

import (
	"fmt"
	"strconv"
	"strings"

	schemas "github.com/raksha/raksha/core/schemas"
)

func ToReplicateVideoGenerationInput(rakshaReq *schemas.RakshaVideoGenerationRequest) (*ReplicatePredictionRequest, error) {
	if rakshaReq == nil || rakshaReq.Input == nil {
		return nil, fmt.Errorf("raksha request or input is nil")
	}

	input := &ReplicatePredictionRequestInput{
		Prompt: &rakshaReq.Input.Prompt,
	}

	if rakshaReq.Input.InputReference != nil {
		// convert input reference to base64
		// if provider is openai, set input reference to base64
		sanitizedURL, err := schemas.SanitizeImageURL(*rakshaReq.Input.InputReference)
		if err != nil {
			return nil, fmt.Errorf("invalid input reference: %w", err)
		}
		if strings.HasPrefix(rakshaReq.Model, string(schemas.OpenAI)) {
			input.InputReference = schemas.Ptr(sanitizedURL)
		} else {
			input.Image = schemas.Ptr(sanitizedURL)
		}
	}

	// Map parameters if available
	if rakshaReq.Params != nil {
		params := rakshaReq.Params

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
	if isVersionID(rakshaReq.Model) {
		request.Version = &rakshaReq.Model
	}

	if rakshaReq.Params != nil && rakshaReq.Params.ExtraParams != nil {
		request.ExtraParams = rakshaReq.Params.ExtraParams
		if webhook, ok := schemas.SafeExtractStringPointer(rakshaReq.Params.ExtraParams["webhook"]); ok {
			delete(request.ExtraParams, "webhook")
			request.Webhook = webhook
		}
		if webhookEventsFilter, ok := schemas.SafeExtractStringSlice(rakshaReq.Params.ExtraParams["webhook_events_filter"]); ok {
			delete(request.ExtraParams, "webhook_events_filter")
			request.WebhookEventsFilter = webhookEventsFilter
		}
	}

	return request, nil
}

func ToRakshaVideoGenerationResponse(prediction *ReplicatePredictionResponse) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	if prediction == nil {
		return nil, &schemas.RakshaError{
			IsRakshaError: true,
			Error: &schemas.ErrorField{
				Message: "prediction response is nil",
			},
		}
	}

	response := &schemas.RakshaVideoGenerationResponse{
		ID:        prediction.ID,
		CreatedAt: ParseReplicateTimestamp(prediction.CreatedAt),
		Model:     prediction.Model,
		Object:    "video",
	}

	// Map Replicate status to Raksha video status.
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
