package huggingface

import (
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/raksha/raksha/core/schemas"
)

// ToHuggingFaceEmbeddingRequest converts a Raksha embedding request to HuggingFace format
func ToHuggingFaceEmbeddingRequest(rakshaReq *schemas.RakshaEmbeddingRequest) (*HuggingFaceEmbeddingRequest, error) {
	if rakshaReq == nil {
		return nil, nil
	}

	inferenceProvider, modelName, nameErr := splitIntoModelProvider(rakshaReq.Model)
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
	if rakshaReq.Input != nil {
		var input InputsCustomType
		if rakshaReq.Input.Text != nil {
			input = InputsCustomType{Text: rakshaReq.Input.Text}

		} else if rakshaReq.Input.Texts != nil {
			input = InputsCustomType{Texts: rakshaReq.Input.Texts}
		}
		if inferenceProvider == hfInference {
			hfReq.Inputs = &input
		} else {
			hfReq.Input = &input
		}
	}

	// Map parameters
	if rakshaReq.Params != nil {
		params := rakshaReq.Params

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

// UnmarshalHuggingFaceEmbeddingResponse unmarshals HuggingFace API response directly into RakshaEmbeddingResponse
// Handles multiple formats: standard object, 2D array, or 1D array
func UnmarshalHuggingFaceEmbeddingResponse(data []byte, model string) (*schemas.RakshaEmbeddingResponse, error) {
	if data == nil {
		return nil, fmt.Errorf("response data is nil")
	}

	// Try standard object format first
	type tempResponse struct {
		Data  []schemas.EmbeddingData  `json:"data,omitempty"`
		Model *string                  `json:"model,omitempty"`
		Usage *schemas.RakshaLLMUsage `json:"usage,omitempty"`
	}
	var obj tempResponse
	if err := sonic.Unmarshal(data, &obj); err == nil {
		if obj.Data != nil || obj.Model != nil || obj.Usage != nil {
			rakshaResponse := &schemas.RakshaEmbeddingResponse{
				Data:   obj.Data,
				Model:  model,
				Object: "list",
			}
			if obj.Model != nil {
				rakshaResponse.Model = *obj.Model
			}
			if obj.Usage != nil {
				rakshaResponse.Usage = obj.Usage
			} else {
				rakshaResponse.Usage = &schemas.RakshaLLMUsage{
					PromptTokens:     0,
					CompletionTokens: 0,
					TotalTokens:      0,
				}
			}
			return rakshaResponse, nil
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
		return &schemas.RakshaEmbeddingResponse{
			Data:   embeddings,
			Model:  model,
			Object: "list",
			Usage: &schemas.RakshaLLMUsage{
				PromptTokens:     0,
				CompletionTokens: 0,
				TotalTokens:      0,
			},
		}, nil
	}

	// Try 1D array: [num, ...]
	var arr1D []float64
	if err := sonic.Unmarshal(data, &arr1D); err == nil {
		return &schemas.RakshaEmbeddingResponse{
			Data: []schemas.EmbeddingData{{
				Embedding: schemas.EmbeddingStruct{EmbeddingArray: append([]float64(nil), arr1D...)},
				Index:     0,
				Object:    "embedding",
			}},
			Model:  model,
			Object: "list",
			Usage: &schemas.RakshaLLMUsage{
				PromptTokens:     0,
				CompletionTokens: 0,
				TotalTokens:      0,
			},
		}, nil
	}

	return nil, fmt.Errorf("failed to unmarshal HuggingFace embedding response: unexpected structure")
}
