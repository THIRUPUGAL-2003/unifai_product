package runware

import (
	"fmt"

	"github.com/google/uuid"
	providerUtils "github.com/unifai/unifai/core/providers/utils"
	schemas "github.com/unifai/unifai/core/schemas"
)

// ToRunwareImageGenerationRequest converts a UnifAI image generation request to a Runware
// imageInference task. A "seedImage" supplied via extra params (a Runware image UUID, a public
// URL, or a base64/data-URI string) turns the request into an image-to-image generation.
func ToRunwareImageGenerationRequest(unifaiReq *schemas.UnifAIImageGenerationRequest) (*RunwareInferenceRequest, error) {
	if unifaiReq.Input == nil {
		return nil, fmt.Errorf("input is required")
	}

	width, height := defaultRunwareWidth, defaultRunwareHeight
	request := &RunwareInferenceRequest{
		TaskType:       taskTypeImageInference,
		TaskUUID:       uuid.New().String(),
		Model:          unifaiReq.Model,
		PositivePrompt: &unifaiReq.Input.Prompt,
		Width:          &width,
		Height:         &height,
	}

	if unifaiReq.Params != nil {
		params := unifaiReq.Params

		if params.Size != nil && *params.Size != "" {
			*request.Width, *request.Height = parseRunwareSize(*params.Size)
		}
		request.NegativePrompt = params.NegativePrompt
		request.Steps = params.NumInferenceSteps
		request.Seed = params.Seed
		request.NumberResults = params.N
		request.OutputType = runwareOutputType(params.ResponseFormat)
		request.OutputFormat = runwareOutputFormat(params.OutputFormat)

		request.ExtraParams = params.ExtraParams

		if v := request.ExtraParams["seedImage"]; v != nil {
			delete(request.ExtraParams, "seedImage")
			if s, ok := v.(string); ok && s != "" {
				request.SeedImage = &s
			}
		}
	}

	return request, nil
}

// ToRunwareImageEditRequest converts a UnifAI image edit request to a Runware imageInference task.
// The first input image is the seed image; an optional mask enables inpainting. Outpainting,
// strength, maskMargin and other operation-specific fields flow through via extra params.
func ToRunwareImageEditRequest(unifaiReq *schemas.UnifAIImageEditRequest) (*RunwareInferenceRequest, error) {
	if unifaiReq.Input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if len(unifaiReq.Input.Images) == 0 || len(unifaiReq.Input.Images[0].Image) == 0 {
		return nil, fmt.Errorf("at least one input image is required")
	}

	width, height := defaultRunwareWidth, defaultRunwareHeight
	request := &RunwareInferenceRequest{
		TaskType:       taskTypeImageInference,
		TaskUUID:       uuid.New().String(),
		Model:          unifaiReq.Model,
		PositivePrompt: &unifaiReq.Input.Prompt,
		Width:          &width,
		Height:         &height,
	}

	// Seed image: the base image being edited (raw bytes -> base64 data URI).
	seedImage := providerUtils.FileBytesToBase64DataURL(unifaiReq.Input.Images[0].Image)
	request.SeedImage = &seedImage

	if unifaiReq.Params != nil {
		params := unifaiReq.Params

		if params.Size != nil && *params.Size != "" {
			*request.Width, *request.Height = parseRunwareSize(*params.Size)
		}
		request.NegativePrompt = params.NegativePrompt
		request.Steps = params.NumInferenceSteps
		request.Seed = params.Seed
		request.NumberResults = params.N
		request.OutputType = runwareOutputType(params.ResponseFormat)
		request.OutputFormat = runwareOutputFormat(params.OutputFormat)

		// Mask image enables inpainting (raw bytes -> base64 data URI).
		if len(params.Mask) > 0 {
			maskImage := providerUtils.FileBytesToBase64DataURL(params.Mask)
			request.MaskImage = &maskImage
		}

		request.ExtraParams = params.ExtraParams
	}

	return request, nil
}

// ToUnifAIImageGenerationResponse converts a Runware response envelope to a UnifAI image response.
func ToUnifAIImageGenerationResponse(resp *RunwareResponse) (*schemas.UnifAIImageGenerationResponse, *schemas.UnifAIError) {
	if resp == nil {
		return nil, providerUtils.NewUnifAIOperationError("runware response is nil", nil)
	}

	// Surface task-level failures returned alongside (or instead of) data.
	if len(resp.Data) == 0 {
		if msg := firstRunwareErrorMessage(resp.Errors); msg != "" {
			return nil, providerUtils.NewUnifAIOperationError(msg, nil)
		}
		return nil, providerUtils.NewUnifAIOperationError("runware returned no images", nil)
	}

	unifaiResp := &schemas.UnifAIImageGenerationResponse{
		ID:   resp.Data[0].TaskUUID,
		Data: []schemas.ImageData{},
	}

	var seeds []int
	for i, img := range resp.Data {
		data := schemas.ImageData{Index: i}
		switch {
		case img.ImageURL != "":
			data.URL = img.ImageURL
		case img.ImageBase64Data != "":
			data.B64JSON = img.ImageBase64Data
		case img.ImageDataURI != "":
			data.URL = img.ImageDataURI
		}
		unifaiResp.Data = append(unifaiResp.Data, data)
		if img.Seed != nil {
			seeds = append(seeds, *img.Seed)
		}
	}

	if len(seeds) > 0 {
		unifaiResp.ImageGenerationResponseParameters = &schemas.ImageGenerationResponseParameters{Seeds: seeds}
	}

	return unifaiResp, nil
}
