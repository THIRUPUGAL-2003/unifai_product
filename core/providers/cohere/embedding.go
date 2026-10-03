package cohere

import (
	"github.com/raksha/raksha/core/schemas"
)

// ToCohereEmbeddingRequest converts a Raksha embedding request to Cohere format
func ToCohereEmbeddingRequest(rakshaReq *schemas.RakshaEmbeddingRequest) *CohereEmbeddingRequest {
	if rakshaReq == nil || rakshaReq.Input == nil || (rakshaReq.Input.Text == nil && rakshaReq.Input.Texts == nil) {
		return nil
	}

	embeddingInput := rakshaReq.Input
	cohereReq := &CohereEmbeddingRequest{
		Model: rakshaReq.Model,
	}

	texts := []string{}
	if embeddingInput.Text != nil {
		texts = append(texts, *embeddingInput.Text)
	} else {
		texts = embeddingInput.Texts
	}

	// Convert texts from Raksha format
	if len(texts) > 0 {
		cohereReq.Texts = texts
	}

	// Set default input type if not specified in extra params
	cohereReq.InputType = "search_document" // Default value

	if rakshaReq.Params != nil {
		cohereReq.OutputDimension = rakshaReq.Params.Dimensions
		cohereReq.ExtraParams = rakshaReq.Params.ExtraParams
		if rakshaReq.Params.ExtraParams != nil {
			if maxTokens, ok := schemas.SafeExtractIntPointer(rakshaReq.Params.ExtraParams["max_tokens"]); ok {
				delete(cohereReq.ExtraParams, "max_tokens")
				cohereReq.MaxTokens = maxTokens
			}
		}
	}

	// Handle extra params
	if rakshaReq.Params != nil && rakshaReq.Params.ExtraParams != nil {
		// Input type
		if inputType, ok := schemas.SafeExtractString(rakshaReq.Params.ExtraParams["input_type"]); ok {
			delete(cohereReq.ExtraParams, "input_type")
			cohereReq.InputType = inputType
		}

		// Embedding types
		if embeddingTypes, ok := schemas.SafeExtractStringSlice(rakshaReq.Params.ExtraParams["embedding_types"]); ok {
			if len(embeddingTypes) > 0 {
				delete(cohereReq.ExtraParams, "embedding_types")
				cohereReq.EmbeddingTypes = embeddingTypes
			}
		}

		// Truncate
		if truncate, ok := schemas.SafeExtractStringPointer(rakshaReq.Params.ExtraParams["truncate"]); ok {
			delete(cohereReq.ExtraParams, "truncate")
			cohereReq.Truncate = truncate
		}
	}

	return cohereReq
}

// ToRakshaEmbeddingRequest converts a Cohere embedding request to Raksha format
func (req *CohereEmbeddingRequest) ToRakshaEmbeddingRequest(ctx *schemas.RakshaContext) *schemas.RakshaEmbeddingRequest {
	if req == nil {
		return nil
	}

	provider, model := schemas.ParseModelString(req.Model, "")

	rakshaReq := &schemas.RakshaEmbeddingRequest{
		Provider: provider,
		Model:    model,
		Input:    &schemas.EmbeddingInput{},
		Params:   &schemas.EmbeddingParameters{},
	}

	// Convert texts
	if len(req.Texts) > 0 {
		if len(req.Texts) == 1 {
			rakshaReq.Input.Text = &req.Texts[0]
		} else {
			rakshaReq.Input.Texts = req.Texts
		}
	}

	// Convert parameters
	if req.OutputDimension != nil {
		rakshaReq.Params.Dimensions = req.OutputDimension
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
		rakshaReq.Params.ExtraParams = extraParams
	}

	return rakshaReq
}

// ToRakshaEmbeddingResponse converts a Cohere embedding response to Raksha format
func (response *CohereEmbeddingResponse) ToRakshaEmbeddingResponse() *schemas.RakshaEmbeddingResponse {
	if response == nil {
		return nil
	}

	rakshaResponse := &schemas.RakshaEmbeddingResponse{
		Object: "list",
	}

	// Convert embeddings data
	if response.Embeddings != nil {
		var rakshaEmbeddings []schemas.EmbeddingData

		// Handle different embedding types - prioritize float embeddings
		if response.Embeddings.Float != nil {
			for i, embedding := range response.Embeddings.Float {
				rakshaEmbedding := schemas.EmbeddingData{
					Object: "embedding",
					Index:  i,
					Embedding: schemas.EmbeddingStruct{
						EmbeddingArray: embedding,
					},
				}
				rakshaEmbeddings = append(rakshaEmbeddings, rakshaEmbedding)
			}
		} else if response.Embeddings.Base64 != nil {
			// Handle base64 embeddings as strings
			for i, embedding := range response.Embeddings.Base64 {
				rakshaEmbedding := schemas.EmbeddingData{
					Object: "embedding",
					Index:  i,
					Embedding: schemas.EmbeddingStruct{
						EmbeddingStr: &embedding,
					},
				}
				rakshaEmbeddings = append(rakshaEmbeddings, rakshaEmbedding)
			}
		}
		// Note: Int8, Uint8, Binary, Ubinary types would need special handling
		// depending on how Raksha wants to represent them

		rakshaResponse.Data = rakshaEmbeddings
	}

	// Convert usage information
	if response.Meta != nil {
		if response.Meta.Tokens != nil {
			rakshaResponse.Usage = &schemas.RakshaLLMUsage{}
			if response.Meta.Tokens.InputTokens != nil {
				rakshaResponse.Usage.PromptTokens = int(*response.Meta.Tokens.InputTokens)
			}
			if response.Meta.Tokens.OutputTokens != nil {
				rakshaResponse.Usage.CompletionTokens = int(*response.Meta.Tokens.OutputTokens)
			}
			rakshaResponse.Usage.TotalTokens = rakshaResponse.Usage.PromptTokens + rakshaResponse.Usage.CompletionTokens
		} else if response.Meta.BilledUnits != nil {
			rakshaResponse.Usage = &schemas.RakshaLLMUsage{}
			if response.Meta.BilledUnits.InputTokens != nil {
				rakshaResponse.Usage.PromptTokens = int(*response.Meta.BilledUnits.InputTokens)
			}
			if response.Meta.BilledUnits.OutputTokens != nil {
				rakshaResponse.Usage.CompletionTokens = int(*response.Meta.BilledUnits.OutputTokens)
			}
			rakshaResponse.Usage.TotalTokens = rakshaResponse.Usage.PromptTokens + rakshaResponse.Usage.CompletionTokens
		}
	}

	return rakshaResponse
}
