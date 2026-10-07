package nebius

import (
	"fmt"
	"strconv"
	"strings"

	schemas "github.com/gateway/gateway/core/schemas"
)

// ToNebiusImageGenerationRequest converts a gateway image generation request to nebius format.
func (provider *NebiusProvider) ToNebiusImageGenerationRequest(gatewayReq *schemas.GatewayImageGenerationRequest) (*NebiusImageGenerationRequest, error) {
	if gatewayReq == nil || gatewayReq.Input == nil {
		return nil, fmt.Errorf("gateway request is nil or input is nil")
	}

	req := &NebiusImageGenerationRequest{
		Model:  &gatewayReq.Model,
		Prompt: &gatewayReq.Input.Prompt,
	}

	if gatewayReq.Params != nil {

		if gatewayReq.Params.ResponseFormat != nil {
			req.ResponseFormat = gatewayReq.Params.ResponseFormat
		}

		if gatewayReq.Params.Size != nil && strings.TrimSpace(strings.ToLower(*gatewayReq.Params.Size)) != "auto" {
			size := strings.Split(strings.TrimSpace(strings.ToLower(*gatewayReq.Params.Size)), "x")
			if len(size) != 2 {
				return nil, fmt.Errorf("invalid size format: expected 'WIDTHxHEIGHT', got %q", *gatewayReq.Params.Size)
			}

			width, err := strconv.Atoi(size[0])
			if err != nil {
				return nil, fmt.Errorf("invalid width in size %q: %w", *gatewayReq.Params.Size, err)
			}

			height, err := strconv.Atoi(size[1])
			if err != nil {
				return nil, fmt.Errorf("invalid height in size %q: %w", *gatewayReq.Params.Size, err)
			}

			req.Width = &width
			req.Height = &height
		}
		if gatewayReq.Params.OutputFormat != nil {
			req.ResponseExtension = gatewayReq.Params.OutputFormat
		}
		if req.ResponseExtension != nil && strings.ToLower(*req.ResponseExtension) == "jpeg" {
			req.ResponseExtension = schemas.Ptr("jpg")
		}
		if gatewayReq.Params.Seed != nil {
			req.Seed = gatewayReq.Params.Seed
		}
		if gatewayReq.Params.NegativePrompt != nil {
			req.NegativePrompt = gatewayReq.Params.NegativePrompt
		}
		if gatewayReq.Params.NumInferenceSteps != nil {
			req.NumInferenceSteps = gatewayReq.Params.NumInferenceSteps
		}
		// Handle extra params
		if gatewayReq.Params.ExtraParams != nil {
			req.ExtraParams = gatewayReq.Params.ExtraParams
			// Map guidance_scale
			if v, ok := schemas.SafeExtractIntPointer(gatewayReq.Params.ExtraParams["guidance_scale"]); ok {
				delete(req.ExtraParams, "guidance_scale")
				req.GuidanceScale = v
			}

			// Map loras in array format [{"url": "...", "scale": ...}]
			if lorasValue, exists := gatewayReq.Params.ExtraParams["loras"]; exists && lorasValue != nil {
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

// ToGatewayImageResponse converts a nebius image generation response to gateway format.
func ToGatewayImageResponse(nebiusResponse *NebiusImageGenerationResponse) *schemas.GatewayImageGenerationResponse {
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
	return &schemas.GatewayImageGenerationResponse{
		ID:   nebiusResponse.Id,
		Data: data,
	}
}
