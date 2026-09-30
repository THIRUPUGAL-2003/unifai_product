package nebius

import (
	"fmt"
	"strconv"
	"strings"

	schemas "github.com/unifai/unifai/core/schemas"
)

// ToNebiusImageGenerationRequest converts a unifai image generation request to nebius format.
func (provider *NebiusProvider) ToNebiusImageGenerationRequest(unifaiReq *schemas.UnifAIImageGenerationRequest) (*NebiusImageGenerationRequest, error) {
	if unifaiReq == nil || unifaiReq.Input == nil {
		return nil, fmt.Errorf("unifai request is nil or input is nil")
	}

	req := &NebiusImageGenerationRequest{
		Model:  &unifaiReq.Model,
		Prompt: &unifaiReq.Input.Prompt,
	}

	if unifaiReq.Params != nil {

		if unifaiReq.Params.ResponseFormat != nil {
			req.ResponseFormat = unifaiReq.Params.ResponseFormat
		}

		if unifaiReq.Params.Size != nil && strings.TrimSpace(strings.ToLower(*unifaiReq.Params.Size)) != "auto" {
			size := strings.Split(strings.TrimSpace(strings.ToLower(*unifaiReq.Params.Size)), "x")
			if len(size) != 2 {
				return nil, fmt.Errorf("invalid size format: expected 'WIDTHxHEIGHT', got %q", *unifaiReq.Params.Size)
			}

			width, err := strconv.Atoi(size[0])
			if err != nil {
				return nil, fmt.Errorf("invalid width in size %q: %w", *unifaiReq.Params.Size, err)
			}

			height, err := strconv.Atoi(size[1])
			if err != nil {
				return nil, fmt.Errorf("invalid height in size %q: %w", *unifaiReq.Params.Size, err)
			}

			req.Width = &width
			req.Height = &height
		}
		if unifaiReq.Params.OutputFormat != nil {
			req.ResponseExtension = unifaiReq.Params.OutputFormat
		}
		if req.ResponseExtension != nil && strings.ToLower(*req.ResponseExtension) == "jpeg" {
			req.ResponseExtension = schemas.Ptr("jpg")
		}
		if unifaiReq.Params.Seed != nil {
			req.Seed = unifaiReq.Params.Seed
		}
		if unifaiReq.Params.NegativePrompt != nil {
			req.NegativePrompt = unifaiReq.Params.NegativePrompt
		}
		if unifaiReq.Params.NumInferenceSteps != nil {
			req.NumInferenceSteps = unifaiReq.Params.NumInferenceSteps
		}
		// Handle extra params
		if unifaiReq.Params.ExtraParams != nil {
			req.ExtraParams = unifaiReq.Params.ExtraParams
			// Map guidance_scale
			if v, ok := schemas.SafeExtractIntPointer(unifaiReq.Params.ExtraParams["guidance_scale"]); ok {
				delete(req.ExtraParams, "guidance_scale")
				req.GuidanceScale = v
			}

			// Map loras in array format [{"url": "...", "scale": ...}]
			if lorasValue, exists := unifaiReq.Params.ExtraParams["loras"]; exists && lorasValue != nil {
				delete(req.ExtraParams, "loras")
				// Check if lorasValue is an array of maps
				if lorasArray, ok := lorasValue.([]interface{}); ok {
					for _, item := range lorasArray {
						if loraMap, ok := item.(map[string]interface{}); ok {
							if url, ok := schemas.SafeExtractString(loraMap["url"]); ok {
								if scale, ok := schemas.SafeExtractInt(loraMap["scale"]); ok {
									req.Loras = append(req.Loras, NebiusLora{URL: url, Scale: scale})
								}
							}
						}
					}
				}
			}
		}
	}
	return req, nil
}

// ToUnifAIImageResponse converts a nebius image generation response to unifai format.
func ToUnifAIImageResponse(nebiusResponse *NebiusImageGenerationResponse) *schemas.UnifAIImageGenerationResponse {
	if nebiusResponse == nil {
		return nil
	}

	data := make([]schemas.ImageData, len(nebiusResponse.Data))
	for i, img := range nebiusResponse.Data {
		data[i] = schemas.ImageData{
			URL:           img.URL,
			B64JSON:       img.B64JSON,
			RevisedPrompt: img.RevisedPrompt,
			Index:         i,
		}
	}
	return &schemas.UnifAIImageGenerationResponse{
		ID:   nebiusResponse.Id,
		Data: data,
	}
}
