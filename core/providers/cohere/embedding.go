package cohere

import (
	"github.com/unifai/unifai/core/schemas"
)

// ToCohereEmbeddingRequest converts a UnifAI embedding request to Cohere format
func ToCohereEmbeddingRequest(unifaiReq *schemas.UnifAIEmbeddingRequest) *CohereEmbeddingRequest {
	if unifaiReq == nil || unifaiReq.Input == nil || (unifaiReq.Input.Text == nil && unifaiReq.Input.Texts == nil) {
		return nil
	}

	embeddingInput := unifaiReq.Input
	cohereReq := &CohereEmbeddingRequest{
		Model: unifaiReq.Model,
	}

	texts := []string{}
	if embeddingInput.Text != nil {
		texts = append(texts, *embeddingInput.Text)
	} else {
		texts = embeddingInput.Texts
	}

	// Convert texts from UnifAI format
	if len(texts) > 0 {
		cohereReq.Texts = texts
	}

	// Set default input type if not specified in extra params
	cohereReq.InputType = "search_document" // Default value

	if unifaiReq.Params != nil {
		cohereReq.OutputDimension = unifaiReq.Params.Dimensions
		cohereReq.ExtraParams = unifaiReq.Params.ExtraParams
		if unifaiReq.Params.ExtraParams != nil {
			if maxTokens, ok := schemas.SafeExtractIntPointer(unifaiReq.Params.ExtraParams["max_tokens"]); ok {
				delete(cohereReq.ExtraParams, "max_tokens")
				cohereReq.MaxTokens = maxTokens
			}
		}
	}

	// Handle extra params
	if unifaiReq.Params != nil && unifaiReq.Params.ExtraParams != nil {
		// Input type
		if inputType, ok := schemas.SafeExtractString(unifaiReq.Params.ExtraParams["input_type"]); ok {
			delete(cohereReq.ExtraParams, "input_type")
			cohereReq.InputType = inputType
		}

		// Embedding types
		if embeddingTypes, ok := schemas.SafeExtractStringSlice(unifaiReq.Params.ExtraParams["embedding_types"]); ok {
			if len(embeddingTypes) > 0 {
				delete(cohereReq.ExtraParams, "embedding_types")
				cohereReq.EmbeddingTypes = embeddingTypes
			}
		}

		// Truncate
		if truncate, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["truncate"]); ok {
			delete(cohereReq.ExtraParams, "truncate")
			cohereReq.Truncate = truncate
		}
	}

	return cohereReq
}

// ToUnifAIEmbeddingRequest converts a Cohere embedding request to UnifAI format
func (req *CohereEmbeddingRequest) ToUnifAIEmbeddingRequest(ctx *schemas.UnifAIContext) *schemas.UnifAIEmbeddingRequest {
	if req == nil {
		return nil
	}

	provider, model := schemas.ParseModelString(req.Model, "")

	unifaiReq := &schemas.UnifAIEmbeddingRequest{
		Provider: provider,
		Model:    model,
		Input:    &schemas.EmbeddingInput{},
		Params:   &schemas.EmbeddingParameters{},
	}

	// Convert texts
	if len(req.Texts) > 0 {
		if len(req.Texts) == 1 {
			unifaiReq.Input.Text = &req.Texts[0]
		} else {
			unifaiReq.Input.Texts = req.Texts
		}
	}

	// Convert parameters
	if req.OutputDimension != nil {
		unifaiReq.Params.Dimensions = req.OutputDimension
	}

	// Convert extra params
	extraParams := make(map[string]interface{})
	if req.InputType != "" {
		extraParams["input_type"] = req.InputType
	}
	if req.EmbeddingTypes != nil {
		extraParams["embedding_types"] = req.EmbeddingTypes
	}
	if req.Truncate != nil {
		extraParams["truncate"] = *req.Truncate
	}
	if req.MaxTokens != nil {
		extraParams["max_tokens"] = *req.MaxTokens
	}
	if len(extraParams) > 0 {
		unifaiReq.Params.ExtraParams = extraParams
	}

	return unifaiReq
}

// ToUnifAIEmbeddingResponse converts a Cohere embedding response to UnifAI format
func (response *CohereEmbeddingResponse) ToUnifAIEmbeddingResponse() *schemas.UnifAIEmbeddingResponse {
	if response == nil {
		return nil
	}

	unifaiResponse := &schemas.UnifAIEmbeddingResponse{
		Object: "list",
	}

	// Convert embeddings data
	if response.Embeddings != nil {
		var unifaiEmbeddings []schemas.EmbeddingData

		// Handle different embedding types - prioritize float embeddings
		if response.Embeddings.Float != nil {
			for i, embedding := range response.Embeddings.Float {
				unifaiEmbedding := schemas.EmbeddingData{
					Object: "embedding",
					Index:  i,
					Embedding: schemas.EmbeddingStruct{
						EmbeddingArray: embedding,
					},
				}
				unifaiEmbeddings = append(unifaiEmbeddings, unifaiEmbedding)
			}
		} else if response.Embeddings.Base64 != nil {
			// Handle base64 embeddings as strings
			for i, embedding := range response.Embeddings.Base64 {
				unifaiEmbedding := schemas.EmbeddingData{
					Object: "embedding",
					Index:  i,
					Embedding: schemas.EmbeddingStruct{
						EmbeddingStr: &embedding,
					},
				}
				unifaiEmbeddings = append(unifaiEmbeddings, unifaiEmbedding)
			}
		}
		// Note: Int8, Uint8, Binary, Ubinary types would need special handling
		// depending on how UnifAI wants to represent them

		unifaiResponse.Data = unifaiEmbeddings
	}

	// Convert usage information
	if response.Meta != nil {
		if response.Meta.Tokens != nil {
			unifaiResponse.Usage = &schemas.UnifAILLMUsage{}
			if response.Meta.Tokens.InputTokens != nil {
				unifaiResponse.Usage.PromptTokens = int(*response.Meta.Tokens.InputTokens)
			}
			if response.Meta.Tokens.OutputTokens != nil {
				unifaiResponse.Usage.CompletionTokens = int(*response.Meta.Tokens.OutputTokens)
			}
			unifaiResponse.Usage.TotalTokens = unifaiResponse.Usage.PromptTokens + unifaiResponse.Usage.CompletionTokens
		} else if response.Meta.BilledUnits != nil {
			unifaiResponse.Usage = &schemas.UnifAILLMUsage{}
			if response.Meta.BilledUnits.InputTokens != nil {
				unifaiResponse.Usage.PromptTokens = int(*response.Meta.BilledUnits.InputTokens)
			}
			if response.Meta.BilledUnits.OutputTokens != nil {
				unifaiResponse.Usage.CompletionTokens = int(*response.Meta.BilledUnits.OutputTokens)
			}
			unifaiResponse.Usage.TotalTokens = unifaiResponse.Usage.PromptTokens + unifaiResponse.Usage.CompletionTokens
		}
	}

	return unifaiResponse
}
