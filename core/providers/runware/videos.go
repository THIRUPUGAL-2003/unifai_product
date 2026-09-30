package runware

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	schemas "github.com/unifai/unifai/core/schemas"
)

// ToRunwareVideoGenerationRequest converts a UnifAI video generation request to a Runware
// videoInference task. An input reference image turns it into image-to-video generation.
func ToRunwareVideoGenerationRequest(unifaiReq *schemas.UnifAIVideoGenerationRequest) (*RunwareInferenceRequest, error) {
	if unifaiReq.Input == nil {
		return nil, fmt.Errorf("input is required")
	}

	// Runware requires explicit width/height for video; default to 16:9 1080p when no size is given.
	request := &RunwareInferenceRequest{
		TaskType:       taskTypeVideoInference,
		TaskUUID:       uuid.New().String(),
		DeliveryMethod: new(deliveryMethodAsync),
		Model:          unifaiReq.Model,
		Width:          new(defaultRunwareVideoWidth),
		Height:         new(defaultRunwareVideoHeight),
	}

	if unifaiReq.Input.Prompt != "" {
		request.PositivePrompt = &unifaiReq.Input.Prompt
	}

	// Input reference image (image-to-video): anchored to the first frame.
	if unifaiReq.Input.InputReference != nil && *unifaiReq.Input.InputReference != "" {
		sanitizedURL, err := schemas.SanitizeImageURL(*unifaiReq.Input.InputReference)
		if err != nil {
			return nil, fmt.Errorf("invalid input reference: %w", err)
		}
		request.FrameImages = []RunwareFrameImage{{InputImage: sanitizedURL, Frame: new("first")}}
	}

	if unifaiReq.Params != nil {
		params := unifaiReq.Params

		request.NegativePrompt = params.NegativePrompt
		request.Seed = params.Seed

		if params.Size != "" {
			*request.Width, *request.Height = parseRunwareSize(params.Size)
		}

		if params.Seconds != nil && *params.Seconds != "" {
			seconds, err := strconv.ParseFloat(*params.Seconds, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid seconds value: %w", err)
			}
			request.Duration = &seconds
		}

		request.ExtraParams = params.ExtraParams
	}

	return request, nil
}

// ToUnifAIVideoGenerationResponse converts a Runware video task result to a UnifAI video response.
func ToUnifAIVideoGenerationResponse(result *RunwareResult) *schemas.UnifAIVideoGenerationResponse {
	response := &schemas.UnifAIVideoGenerationResponse{
		ID:        result.TaskUUID,
		Object:    "video",
		CreatedAt: time.Now().Unix(),
	}

	switch strings.ToLower(result.Status) {
	case "success":
		response.Status = schemas.VideoStatusCompleted
	case "processing":
		response.Status = schemas.VideoStatusInProgress
	case "error":
		response.Status = schemas.VideoStatusFailed
		response.Error = &schemas.VideoCreateError{Code: result.Status, Message: "runware video task failed"}
	default:
		response.Status = schemas.VideoStatusQueued
	}

	if result.VideoURL != "" {
		response.Videos = []schemas.VideoOutput{{
			Type:        schemas.VideoOutputTypeURL,
			URL:         new(result.VideoURL),
			ContentType: "video/mp4",
		}}
	}

	return response
}
