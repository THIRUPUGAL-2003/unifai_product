package huggingface

import (
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/unifai/unifai/core/schemas"
)

// ToHuggingFaceEmbeddingRequest converts a UnifAI embedding request to HuggingFace format
func ToHuggingFaceEmbeddingRequest(unifaiReq *schemas.UnifAIEmbeddingRequest) (*HuggingFaceEmbeddingRequest, error) {
	if unifaiReq == nil {
		return nil, nil
	}

	inferenceProvider, modelName, nameErr := splitIntoModelProvider(unifaiReq.Model)
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
	if unifaiReq.Input != nil {
		var input InputsCustomType
		if unifaiReq.Input.Text != nil {
			input = InputsCustomType{Text: unifaiReq.Input.Text}

		} else if unifaiReq.Input.Texts != nil {
			input = InputsCustomType{Texts: unifaiReq.Input.Texts}
		}
		if inferenceProvider == hfInference {
			hfReq.Inputs = &input
		} else {
			hfReq.Input = &input
		}
	}

	// Map parameters
	if unifaiReq.Params != nil {
		params := unifaiReq.Params

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

// UnmarshalHuggingFaceEmbeddingResponse unmarshals HuggingFace API response directly into UnifAIEmbeddingResponse
// Handles multiple formats: standard object, 2D array, or 1D array
func UnmarshalHuggingFaceEmbeddingResponse(data []byte, model string) (*schemas.UnifAIEmbeddingResponse, error) {
	if data == nil {
		return nil, fmt.Errorf("response data is nil")
	}

	// Try standard object format first
	type tempResponse struct {
		Data  []schemas.EmbeddingData  `json:"data,omitempty"`
		Model *string                  `json:"model,omitempty"`
		Usage *schemas.UnifAILLMUsage `json:"usage,omitempty"`
	}
	var obj tempResponse
	if err := sonic.Unmarshal(data, &obj); err == nil {
		if obj.Data != nil || obj.Model != nil || obj.Usage != nil {
			unifaiResponse := &schemas.UnifAIEmbeddingResponse{
				Data:   obj.Data,
				Model:  model,
				Object: "list",
			}
			if obj.Model != nil {
				unifaiResponse.Model = *obj.Model
			}
			if obj.Usage != nil {
				unifaiResponse.Usage = obj.Usage
			} else {
				unifaiResponse.Usage = &schemas.UnifAILLMUsage{
					PromptTokens:     0,
					CompletionTokens: 0,
					TotalTokens:      0,
				}
			}
			return unifaiResponse, nil
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
		return &schemas.UnifAIEmbeddingResponse{
			Data:   embeddings,
			Model:  model,
			Object: "list",
			Usage: &schemas.UnifAILLMUsage{
				PromptTokens:     0,
				CompletionTokens: 0,
				TotalTokens:      0,
			},
		}, nil
	}

	// Try 1D array: [num, ...]
	var arr1D []float64
	if err := sonic.Unmarshal(data, &arr1D); err == nil {
		return &schemas.UnifAIEmbeddingResponse{
			Data: []schemas.EmbeddingData{{
				Embedding: schemas.EmbeddingStruct{EmbeddingArray: append([]float64(nil), arr1D...)},
				Index:     0,
				Object:    "embedding",
			}},
			Model:  model,
			Object: "list",
			Usage: &schemas.UnifAILLMUsage{
				PromptTokens:     0,
				CompletionTokens: 0,
				TotalTokens:      0,
			},
		}, nil
	}

	return nil, fmt.Errorf("failed to unmarshal HuggingFace embedding response: unexpected structure")
}
