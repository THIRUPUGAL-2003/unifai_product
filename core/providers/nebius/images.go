package nebius

import (
	"fmt"
	"strconv"
	"strings"

	schemas "github.com/raksha/raksha/core/schemas"
)

// ToNebiusImageGenerationRequest converts a raksha image generation request to nebius format.
func (provider *NebiusProvider) ToNebiusImageGenerationRequest(rakshaReq *schemas.RakshaImageGenerationRequest) (*NebiusImageGenerationRequest, error) {
	if rakshaReq == nil || rakshaReq.Input == nil {
		return nil, fmt.Errorf("raksha request is nil or input is nil")
	}

	req := &NebiusImageGenerationRequest{
		Model:  &rakshaReq.Model,
		Prompt: &rakshaReq.Input.Prompt,
	}

	if rakshaReq.Params != nil {

		if rakshaReq.Params.ResponseFormat != nil {
			req.ResponseFormat = rakshaReq.Params.ResponseFormat
		}

		if rakshaReq.Params.Size != nil && strings.TrimSpace(strings.ToLower(*rakshaReq.Params.Size)) != "auto" {
			size := strings.Split(strings.TrimSpace(strings.ToLower(*rakshaReq.Params.Size)), "x")
			if len(size) != 2 {
				return nil, fmt.Errorf("invalid size format: expected 'WIDTHxHEIGHT', got %q", *rakshaReq.Params.Size)
			}

			width, err := strconv.Atoi(size[0])
			if err != nil {
				return nil, fmt.Errorf("invalid width in size %q: %w", *rakshaReq.Params.Size, err)
			}

			height, err := strconv.Atoi(size[1])
			if err != nil {
				return nil, fmt.Errorf("invalid height in size %q: %w", *rakshaReq.Params.Size, err)
			}

			req.Width = &width
			req.Height = &height
		}
		if rakshaReq.Params.OutputFormat != nil {
			req.ResponseExtension = rakshaReq.Params.OutputFormat
		}
		if req.ResponseExtension != nil && strings.ToLower(*req.ResponseExtension) == "jpeg" {
			req.ResponseExtension = schemas.Ptr("jpg")
		}
		if rakshaReq.Params.Seed != nil {
			req.Seed = rakshaReq.Params.Seed
		}
		if rakshaReq.Params.NegativePrompt != nil {
			req.NegativePrompt = rakshaReq.Params.NegativePrompt
		}
		if rakshaReq.Params.NumInferenceSteps != nil {
			req.NumInferenceSteps = rakshaReq.Params.NumInferenceSteps
		}
		// Handle extra params
		if rakshaReq.Params.ExtraParams != nil {
			req.ExtraParams = rakshaReq.Params.ExtraParams
			// Map guidance_scale
			if v, ok := schemas.SafeExtractIntPointer(rakshaReq.Params.ExtraParams["guidance_scale"]); ok {
				delete(req.ExtraParams, "guidance_scale")
				req.GuidanceScale = v
			}

			// Map loras in array format [{"url": "...", "scale": ...}]
			if lorasValue, exists := rakshaReq.Params.ExtraParams["loras"]; exists && lorasValue != nil {
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

// ToRakshaImageResponse converts a nebius image generation response to raksha format.
func ToRakshaImageResponse(nebiusResponse *NebiusImageGenerationResponse) *schemas.RakshaImageGenerationResponse {
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
	return &schemas.RakshaImageGenerationResponse{
		ID:   nebiusResponse.Id,
		Data: data,
	}
}
