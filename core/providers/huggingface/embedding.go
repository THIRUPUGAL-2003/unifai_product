package huggingface

import (
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/gateway/gateway/core/schemas"
)

// ToHuggingFaceEmbeddingRequest converts a Gateway embedding request to HuggingFace format
func ToHuggingFaceEmbeddingRequest(gatewayReq *schemas.GatewayEmbeddingRequest) (*HuggingFaceEmbeddingRequest, error) {
	if gatewayReq == nil {
		return nil, nil
	}

	inferenceProvider, modelName, nameErr := splitIntoModelProvider(gatewayReq.Model)
	if nameErr != nil {
		return nil, nameErr
	}

	var hfReq *HuggingFaceEmbeddingRequest
	if inferenceProvider != hfInference {
		hfReq = &HuggingFaceEmbeddingRequest{
			Model:    schemas.Ptr(modelName),
			Provider: schemas.Ptr(string(inferenceProvider)),
		}
	} else {
		hfReq = &HuggingFaceEmbeddingRequest{}
	}

	// Convert input
	if gatewayReq.Input != nil {
		var input InputsCustomType
		if gatewayReq.Input.Text != nil {
			input = InputsCustomType{Text: gatewayReq.Input.Text}

		} else if gatewayReq.Input.Texts != nil {
			input = InputsCustomType{Texts: gatewayReq.Input.Texts}
		}
		if inferenceProvider == hfInference {
			hfReq.Inputs = &input
		} else {
			hfReq.Input = &input
		}
	}

	// Map parameters
	if gatewayReq.Params != nil {
		params := gatewayReq.Params

		// Map standard parameters
		if params.EncodingFormat != nil {
			encodingType := EncodingType(*params.EncodingFormat)
			hfReq.EncodingFormat = &encodingType
		}
		if params.Dimensions != nil {
			hfReq.Dimensions = params.Dimensions
		}

		// Check for HuggingFace-specific parameters in ExtraParams
		if params.ExtraParams != nil {
			if normalize, ok := params.ExtraParams["normalize"].(bool); ok {
				delete(params.ExtraParams, "normalize")
				hfReq.Normalize = &normalize
			}
			if promptName, ok := params.ExtraParams["prompt_name"].(string); ok {
				delete(params.ExtraParams, "prompt_name")
				hfReq.PromptName = &promptName
			}
			if truncate, ok := params.ExtraParams["truncate"].(bool); ok {
				delete(params.ExtraParams, "truncate")
				hfReq.Truncate = &truncate
			}
			if truncationDirection, ok := params.ExtraParams["truncation_direction"].(string); ok {
				delete(params.ExtraParams, "truncation_direction")
				hfReq.TruncationDirection = &truncationDirection
			}
		}
		hfReq.ExtraParams = params.ExtraParams
	}

	return hfReq, nil
}

// UnmarshalHuggingFaceEmbeddingResponse unmarshals HuggingFace API response directly into GatewayEmbeddingResponse
// Handles multiple formats: standard object, 2D array, or 1D array
func UnmarshalHuggingFaceEmbeddingResponse(data []byte, model string) (*schemas.GatewayEmbeddingResponse, error) {
	if data == nil {
		return nil, fmt.Errorf("response data is nil")
	}

	// Try standard object format first
	type tempResponse struct {
		Data  []schemas.EmbeddingData  `json:"data,omitempty"`
		Model *string                  `json:"model,omitempty"`
		Usage *schemas.GatewayLLMUsage `json:"usage,omitempty"`
	}
	var obj tempResponse
	if err := sonic.Unmarshal(data, &obj); err == nil {
		if obj.Data != nil || obj.Model != nil || obj.Usage != nil {
			gatewayResponse := &schemas.GatewayEmbeddingResponse{
				Data:   obj.Data,
				Model:  model,
				Object: "list",
			}
			if obj.Model != nil {
				gatewayResponse.Model = *obj.Model
			}
			if obj.Usage != nil {
				gatewayResponse.Usage = obj.Usage
			} else {
				gatewayResponse.Usage = &schemas.GatewayLLMUsage{
					PromptTokens:     0,
					CompletionTokens: 0,
					TotalTokens:      0,
				}
			}
			return gatewayResponse, nil
		}
	}

	// Try 2D array: [[num, ...], ...]
	var arr2D [][]float64
	if err := sonic.Unmarshal(data, &arr2D); err == nil {
		embeddings := make([]schemas.EmbeddingData, len(arr2D))
		for idx, embedding := range arr2D {
			embeddings[idx] = schemas.EmbeddingData{
				Embedding: schemas.EmbeddingStruct{EmbeddingArray: append([]float64(nil), embedding...)},
				Index:     idx,
				Object:    "embedding",
			}
		}
		return &schemas.GatewayEmbeddingResponse{
			Data:   embeddings,
			Model:  model,
			Object: "list",
			Usage: &schemas.GatewayLLMUsage{
				PromptTokens:     0,
				CompletionTokens: 0,
				TotalTokens:      0,
			},
		}, nil
	}

	// Try 1D array: [num, ...]
	var arr1D []float64
	if err := sonic.Unmarshal(data, &arr1D); err == nil {
		return &schemas.GatewayEmbeddingResponse{
			Data: []schemas.EmbeddingData{{
				Embedding: schemas.EmbeddingStruct{EmbeddingArray: append([]float64(nil), arr1D...)},
				Index:     0,
				Object:    "embedding",
			}},
			Model:  model,
			Object: "list",
			Usage: &schemas.GatewayLLMUsage{
				PromptTokens:     0,
				CompletionTokens: 0,
				TotalTokens:      0,
			},
		}, nil
	}

	return nil, fmt.Errorf("failed to unmarshal HuggingFace embedding response: unexpected structure")
}
