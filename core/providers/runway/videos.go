package runway

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	schemas "github.com/gateway/gateway/core/schemas"
)

func ToRunwayVideoGenerationRequest(gatewayReq *schemas.GatewayVideoGenerationRequest) (*RunwayVideoGenerationRequest, error) {
	// three types of video generation requests in runway api
	// 1. image to video
	// 2. text to video
	// 3. video to video
	if gatewayReq.Input == nil {
		return nil, fmt.Errorf("input is required")
	}

	request := &RunwayVideoGenerationRequest{
		Model: gatewayReq.Model,
		Ratio: schemas.Ptr("1280:720"),
	}

	if isRunwayVeoModel(gatewayReq.Model) {
		request.Duration = schemas.Ptr(4)
	} else if isRunwayGenModel(gatewayReq.Model) {
		request.Duration = schemas.Ptr(2)
	}

	if gatewayReq.Input.Prompt != "" {
		request.PromptText = &gatewayReq.Input.Prompt
	}
	if gatewayReq.Input.InputReference != nil {
		sanitizedURL, err := schemas.SanitizeImageURL(*gatewayReq.Input.InputReference)
		if err != nil {
			return nil, fmt.Errorf("invalid input reference: %w", err)
		}
		request.PromptImage = &PromptImage{
			PromptImageStr: schemas.Ptr(sanitizedURL),
		}
	}

	if gatewayReq.Params != nil {
		if gatewayReq.Params.Seconds != nil {
			seconds, err := strconv.Atoi(*gatewayReq.Params.Seconds)
			if err != nil {
				return nil, fmt.Errorf("invalid seconds value: %w", err)
			}
			request.Duration = &seconds
		}

		if gatewayReq.Params.Size != "" {
			// convert 1280x720 to 1280:720
			request.Ratio = schemas.Ptr(strings.Replace(gatewayReq.Params.Size, "x", ":", 1))
		}

		if isRunwayVeoModel(gatewayReq.Model) {
			if gatewayReq.Params.Audio != nil {
				request.Audio = gatewayReq.Params.Audio
			}
		}

		if isRunwayGenModel(gatewayReq.Model) {
			if gatewayReq.Params.Seed != nil {
				request.Seed = gatewayReq.Params.Seed
			}
		}

		if gatewayReq.Params.VideoURI != nil {
			if !supportsVideoToVideo(gatewayReq.Model) {
				return nil, fmt.Errorf("video_uri is not supported for model %s", gatewayReq.Model)
			}
			request.VideoURI = gatewayReq.Params.VideoURI
		}

		if gatewayReq.Params.ExtraParams != nil {
			request.ExtraParams = gatewayReq.Params.ExtraParams
			// Handle references for video-to-video generation
			if refsVal := gatewayReq.Params.ExtraParams["references"]; refsVal != nil {
				if refs, ok := refsVal.([]Reference); ok && refs != nil {
					request.References = refs
					delete(request.ExtraParams, "references")
				} else if refs, err := schemas.ConvertViaJSON[[]Reference](refsVal); err == nil {
					request.References = refs
					delete(request.ExtraParams, "references")
				}
			}

			// Handle reference images for video generation
			if refImagesVal := gatewayReq.Params.ExtraParams["reference_images"]; refImagesVal != nil {
				if refImages, ok := refImagesVal.([]ReferenceImage); ok && refImages != nil {
					delete(request.ExtraParams, "reference_images")
					request.ReferenceImages = refImages
				} else if refImages, err := schemas.ConvertViaJSON[[]ReferenceImage](refImagesVal); err == nil {
					delete(request.ExtraParams, "reference_images")
					request.ReferenceImages = refImages
				}
			}

			// add content moderation
			if isRunwayVeoModel(gatewayReq.Model) {
				if cmVal := gatewayReq.Params.ExtraParams["content_moderation"]; cmVal != nil {
					if cm, ok := cmVal.(*ContentModeration); ok && cm != nil {
						delete(request.ExtraParams, "content_moderation")
						request.ContentModeration = cm
					} else if cm, err := schemas.ConvertViaJSON[ContentModeration](cmVal); err == nil {
						delete(request.ExtraParams, "content_moderation")
						request.ContentModeration = &cm
					}
				}
			}
		}
	}

	return request, nil
}

// ToGatewayVideoGenerationResponse converts Runway task details to Gateway video generation response format.
func ToGatewayVideoGenerationResponse(taskDetails *RunwayTaskDetailsResponse) (*schemas.GatewayVideoGenerationResponse, *schemas.GatewayError) {
	if taskDetails == nil {
		return nil, providerUtils.NewGatewayOperationError("task details is nil", nil)
	}

	response := &schemas.GatewayVideoGenerationResponse{
		ID:        taskDetails.ID,
		Object:    "video",
		CreatedAt: time.Now().Unix(),
	}

	// Map Runway task status to Gateway video status
	switch taskDetails.Status {
	case RunwayTaskStatusPending, RunwayTaskStatusThrottled:
		response.Status = schemas.VideoStatusQueued
	case RunwayTaskStatusRunning:
		response.Status = schemas.VideoStatusInProgress
	case RunwayTaskStatusSucceeded:
		response.Status = schemas.VideoStatusCompleted
	case RunwayTaskStatusFailed, RunwayTaskStatusCancelled:
		response.Status = schemas.VideoStatusFailed
		// Set error message for failed tasks
		errorMsg := fmt.Sprintf("Task %s", taskDetails.Status)
		response.Error = &schemas.VideoCreateError{
			Code:    string(taskDetails.Status),
			Message: errorMsg,
		}
	default:
		response.Status = schemas.VideoStatusQueued
	}

	if len(taskDetails.Output) > 0 {
		response.Videos = make([]schemas.VideoOutput, len(taskDetails.Output))
		for i, url := range taskDetails.Output {
			response.Videos[i] = schemas.VideoOutput{
				Type:        schemas.VideoOutputTypeURL,
				URL:         schemas.Ptr(url),
				ContentType: "video/mp4",
			}
		}
	}

	// Parse created_at timestamp if available
	if taskDetails.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, taskDetails.CreatedAt); err == nil {
			response.CreatedAt = t.Unix()
		}
	}

	return response, nil
}
