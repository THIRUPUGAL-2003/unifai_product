package runway

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"
)

func ToRunwayVideoGenerationRequest(rakshaReq *schemas.RakshaVideoGenerationRequest) (*RunwayVideoGenerationRequest, error) {
	// three types of video generation requests in runway api
	// 1. image to video
	// 2. text to video
	// 3. video to video
	if rakshaReq.Input == nil {
		return nil, fmt.Errorf("input is required")
	}

	request := &RunwayVideoGenerationRequest{
		Model: rakshaReq.Model,
		Ratio: schemas.Ptr("1280:720"),
	}

	if isRunwayVeoModel(rakshaReq.Model) {
		request.Duration = schemas.Ptr(4)
	} else if isRunwayGenModel(rakshaReq.Model) {
		request.Duration = schemas.Ptr(2)
	}

	if rakshaReq.Input.Prompt != "" {
		request.PromptText = &rakshaReq.Input.Prompt
	}
	if rakshaReq.Input.InputReference != nil {
		sanitizedURL, err := schemas.SanitizeImageURL(*rakshaReq.Input.InputReference)
		if err != nil {
			return nil, fmt.Errorf("invalid input reference: %w", err)
		}
		request.PromptImage = &PromptImage{
			PromptImageStr: schemas.Ptr(sanitizedURL),
		}
	}

	if rakshaReq.Params != nil {
		if rakshaReq.Params.Seconds != nil {
			seconds, err := strconv.Atoi(*rakshaReq.Params.Seconds)
			if err != nil {
				return nil, fmt.Errorf("invalid seconds value: %w", err)
			}
			request.Duration = &seconds
		}

		if rakshaReq.Params.Size != "" {
			// convert 1280x720 to 1280:720
			request.Ratio = schemas.Ptr(strings.Replace(rakshaReq.Params.Size, "x", ":", 1))
		}

		if isRunwayVeoModel(rakshaReq.Model) {
			if rakshaReq.Params.Audio != nil {
				request.Audio = rakshaReq.Params.Audio
			}
		}

		if isRunwayGenModel(rakshaReq.Model) {
			if rakshaReq.Params.Seed != nil {
				request.Seed = rakshaReq.Params.Seed
			}
		}

		if rakshaReq.Params.VideoURI != nil {
			if !supportsVideoToVideo(rakshaReq.Model) {
				return nil, fmt.Errorf("video_uri is not supported for model %s", rakshaReq.Model)
			}
			request.VideoURI = rakshaReq.Params.VideoURI
		}

		if rakshaReq.Params.ExtraParams != nil {
			request.ExtraParams = rakshaReq.Params.ExtraParams
			// Handle references for video-to-video generation
			if refsVal := rakshaReq.Params.ExtraParams["references"]; refsVal != nil {
				if refs, ok := refsVal.([]Reference); ok && refs != nil {
					request.References = refs
					delete(request.ExtraParams, "references")
				} else if refs, err := schemas.ConvertViaJSON[[]Reference](refsVal); err == nil {
					request.References = refs
					delete(request.ExtraParams, "references")
				}
			}

			// Handle reference images for video generation
			if refImagesVal := rakshaReq.Params.ExtraParams["reference_images"]; refImagesVal != nil {
				if refImages, ok := refImagesVal.([]ReferenceImage); ok && refImages != nil {
					delete(request.ExtraParams, "reference_images")
					request.ReferenceImages = refImages
				} else if refImages, err := schemas.ConvertViaJSON[[]ReferenceImage](refImagesVal); err == nil {
					delete(request.ExtraParams, "reference_images")
					request.ReferenceImages = refImages
				}
			}

			// add content moderation
			if isRunwayVeoModel(rakshaReq.Model) {
				if cmVal := rakshaReq.Params.ExtraParams["content_moderation"]; cmVal != nil {
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

// ToRakshaVideoGenerationResponse converts Runway task details to Raksha video generation response format.
func ToRakshaVideoGenerationResponse(taskDetails *RunwayTaskDetailsResponse) (*schemas.RakshaVideoGenerationResponse, *schemas.RakshaError) {
	if taskDetails == nil {
		return nil, providerUtils.NewRakshaOperationError("task details is nil", nil)
	}

	response := &schemas.RakshaVideoGenerationResponse{
		ID:        taskDetails.ID,
		Object:    "video",
		CreatedAt: time.Now().Unix(),
	}

	// Map Runway task status to Raksha video status
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
