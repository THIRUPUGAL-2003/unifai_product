package cohere

import (
	"github.com/gateway/gateway/core/schemas"
)

// ToCohereEmbeddingRequest converts a Gateway embedding request to Cohere format
func ToCohereEmbeddingRequest(gatewayReq *schemas.GatewayEmbeddingRequest) *CohereEmbeddingRequest {
	if gatewayReq == nil || gatewayReq.Input == nil || (gatewayReq.Input.Text == nil && gatewayReq.Input.Texts == nil) {
		return nil
	}

	embeddingInput := gatewayReq.Input
	cohereReq := &CohereEmbeddingRequest{
		Model: gatewayReq.Model,
	}

	texts := []string{}
	if embeddingInput.Text != nil {
		texts = append(texts, *embeddingInput.Text)
	} else {
		texts = embeddingInput.Texts
	}

	// Convert texts from Gateway format
	if len(texts) > 0 {
		cohereReq.Texts = texts
	}

	// Set default input type if not specified in extra params
	cohereReq.InputType = "search_document" // Default value

	if gatewayReq.Params != nil {
		cohereReq.OutputDimension = gatewayReq.Params.Dimensions
		cohereReq.ExtraParams = gatewayReq.Params.ExtraParams
		if gatewayReq.Params.ExtraParams != nil {
			if maxTokens, ok := schemas.SafeExtractIntPointer(gatewayReq.Params.ExtraParams["max_tokens"]); ok {
				delete(cohereReq.ExtraParams, "max_tokens")
				cohereReq.MaxTokens = maxTokens
			}
		}
	}

	// Handle extra params
	if gatewayReq.Params != nil && gatewayReq.Params.ExtraParams != nil {
		// Input type
		if inputType, ok := schemas.SafeExtractString(gatewayReq.Params.ExtraParams["input_type"]); ok {
			delete(cohereReq.ExtraParams, "input_type")
			cohereReq.InputType = inputType
		}

		// Embedding types
		if embeddingTypes, ok := schemas.SafeExtractStringSlice(gatewayReq.Params.ExtraParams["embedding_types"]); ok {
			if len(embeddingTypes) > 0 {
				delete(cohereReq.ExtraParams, "embedding_types")
				cohereReq.EmbeddingTypes = embeddingTypes
			}
		}

		// Truncate
		if truncate, ok := schemas.SafeExtractStringPointer(gatewayReq.Params.ExtraParams["truncate"]); ok {
			delete(cohereReq.ExtraParams, "truncate")
			cohereReq.Truncate = truncate
		}
	}

	return cohereReq
}

// ToGatewayEmbeddingRequest converts a Cohere embedding request to Gateway format
func (req *CohereEmbeddingRequest) ToGatewayEmbeddingRequest(ctx *schemas.GatewayContext) *schemas.GatewayEmbeddingRequest {
	if req == nil {
		return nil
	}

	provider, model := schemas.ParseModelString(req.Model, "")

	gatewayReq := &schemas.GatewayEmbeddingRequest{
		Provider: provider,
		Model:    model,
		Input:    &schemas.EmbeddingInput{},
		Params:   &schemas.EmbeddingParameters{},
	}

	// Convert texts
	if len(req.Texts) > 0 {
		if len(req.Texts) == 1 {
			gatewayReq.Input.Text = &req.Texts[0]
		} else {
			gatewayReq.Input.Texts = req.Texts
		}
	}

	// Convert parameters
	if req.OutputDimension != nil {
		gatewayReq.Params.Dimensions = req.OutputDimension
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
		gatewayReq.Params.ExtraParams = extraParams
	}

	return gatewayReq
}

// ToGatewayEmbeddingResponse converts a Cohere embedding response to Gateway format
func (response *CohereEmbeddingResponse) ToGatewayEmbeddingResponse() *schemas.GatewayEmbeddingResponse {
	if response == nil {
		return nil
	}

	gatewayResponse := &schemas.GatewayEmbeddingResponse{
		Object: "list",
	}

	// Convert embeddings data
	if response.Embeddings != nil {
		var gatewayEmbeddings []schemas.EmbeddingData

		// Handle different embedding types - prioritize float embeddings
		if response.Embeddings.Float != nil {
			for i, embedding := range response.Embeddings.Float {
				gatewayEmbedding := schemas.EmbeddingData{
					Object: "embedding",
					Index:  i,
					Embedding: schemas.EmbeddingStruct{
						EmbeddingArray: embedding,
					},
				}
				gatewayEmbeddings = append(gatewayEmbeddings, gatewayEmbedding)
			}
		} else if response.Embeddings.Base64 != nil {
			// Handle base64 embeddings as strings
			for i, embedding := range response.Embeddings.Base64 {
				gatewayEmbedding := schemas.EmbeddingData{
					Object: "embedding",
					Index:  i,
					Embedding: schemas.EmbeddingStruct{
						EmbeddingStr: &embedding,
					},
				}
				gatewayEmbeddings = append(gatewayEmbeddings, gatewayEmbedding)
			}
		}
		// Note: Int8, Uint8, Binary, Ubinary types would need special handling
		// depending on how Gateway wants to represent them

		gatewayResponse.Data = gatewayEmbeddings
	}

	// Convert usage information
	if response.Meta != nil {
		if response.Meta.Tokens != nil {
			gatewayResponse.Usage = &schemas.GatewayLLMUsage{}
			if response.Meta.Tokens.InputTokens != nil {
				gatewayResponse.Usage.PromptTokens = int(*response.Meta.Tokens.InputTokens)
			}
			if response.Meta.Tokens.OutputTokens != nil {
				gatewayResponse.Usage.CompletionTokens = int(*response.Meta.Tokens.OutputTokens)
			}
			gatewayResponse.Usage.TotalTokens = gatewayResponse.Usage.PromptTokens + gatewayResponse.Usage.CompletionTokens
		} else if response.Meta.BilledUnits != nil {
			gatewayResponse.Usage = &schemas.GatewayLLMUsage{}
			if response.Meta.BilledUnits.InputTokens != nil {
				gatewayResponse.Usage.PromptTokens = int(*response.Meta.BilledUnits.InputTokens)
			}
			if response.Meta.BilledUnits.OutputTokens != nil {
				gatewayResponse.Usage.CompletionTokens = int(*response.Meta.BilledUnits.OutputTokens)
			}
			gatewayResponse.Usage.TotalTokens = gatewayResponse.Usage.PromptTokens + gatewayResponse.Usage.CompletionTokens
		}
	}

	return gatewayResponse
}
