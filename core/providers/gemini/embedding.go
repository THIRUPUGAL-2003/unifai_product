package gemini

import (
	"github.com/gateway/gateway/core/schemas"
)

// ToGeminiEmbeddingRequest converts a GatewayRequest with embedding input to Gemini's batch embedding request format
// GeminiGenerationRequest contains requests array for batch embed content endpoint
func ToGeminiEmbeddingRequest(gatewayReq *schemas.GatewayEmbeddingRequest) *GeminiBatchEmbeddingRequest {
	if gatewayReq == nil || gatewayReq.Input == nil || (gatewayReq.Input.Text == nil && gatewayReq.Input.Texts == nil) {
		return nil
	}

	embeddingInput := gatewayReq.Input

	// Collect all texts to embed
	var texts []string
	if embeddingInput.Text != nil {
		texts = append(texts, *embeddingInput.Text)
	}
	if len(embeddingInput.Texts) > 0 {
		texts = append(texts, embeddingInput.Texts...)
	}

	if len(texts) == 0 {
		return nil
	}

	// Create batch embedding request with one request per text
	batchRequest := &GeminiBatchEmbeddingRequest{
		Requests: make([]GeminiEmbeddingRequest, len(texts)),
	}
	if gatewayReq.Params != nil {
		batchRequest.ExtraParams = gatewayReq.Params.ExtraParams
	}

	// Create individual embedding requests for each text
	for i, text := range texts {
		embeddingReq := GeminiEmbeddingRequest{
			Model: "models/" + gatewayReq.Model,
			Content: &Content{
				Parts: []*Part{
					{
						Text: text,
					},
				},
			},
		}

		// Add parameters if available
		if gatewayReq.Params != nil {
			if gatewayReq.Params.Dimensions != nil {
				embeddingReq.OutputDimensionality = gatewayReq.Params.Dimensions
			}

			// Handle extra parameters
			if gatewayReq.Params.ExtraParams != nil {
				if taskType, ok := schemas.SafeExtractStringPointer(gatewayReq.Params.ExtraParams["taskType"]); ok {
					delete(batchRequest.ExtraParams, "taskType")
					embeddingReq.TaskType = taskType
				}
				if title, ok := schemas.SafeExtractStringPointer(gatewayReq.Params.ExtraParams["title"]); ok {
					delete(batchRequest.ExtraParams, "title")
					embeddingReq.Title = title
				}
			}
		}

		batchRequest.Requests[i] = embeddingReq
	}

	return batchRequest
}

// ToGeminiEmbedContentResponse converts a GatewayEmbeddingResponse to the single :embedContent wire format.
func ToGeminiEmbedContentResponse(gatewayResp *schemas.GatewayEmbeddingResponse) *GeminiEmbedContentResponse {
	if gatewayResp == nil || len(gatewayResp.Data) == 0 {
		return nil
	}
	values := gatewayResp.Data[0].Embedding.EmbeddingArray
	if values == nil && len(gatewayResp.Data[0].Embedding.Embedding2DArray) > 0 {
		values = gatewayResp.Data[0].Embedding.Embedding2DArray[0]
	}
	embedding := GeminiEmbedding{
		Values: append([]float64(nil), values...),
	}
	if gatewayResp.Usage != nil {
		embedding.Statistics = &ContentEmbeddingStatistics{
			TokenCount: int32(gatewayResp.Usage.PromptTokens),
		}
	}
	return &GeminiEmbedContentResponse{Embedding: embedding}
}

// ToGeminiEmbeddingResponse converts a GatewayResponse with embedding data to Gemini's embedding response format
func ToGeminiEmbeddingResponse(gatewayResp *schemas.GatewayEmbeddingResponse) *GeminiEmbeddingResponse {
	if gatewayResp == nil || len(gatewayResp.Data) == 0 {
		return nil
	}

	geminiResp := &GeminiEmbeddingResponse{
		Embeddings: make([]GeminiEmbedding, len(gatewayResp.Data)),
	}

	// Convert each embedding from Gateway format to Gemini format
	for i, embedding := range gatewayResp.Data {
		var values []float64

		// Extract embedding values from GatewayEmbeddingResponse
		if embedding.Embedding.EmbeddingArray != nil {
			values = append([]float64(nil), embedding.Embedding.EmbeddingArray...)
		} else if len(embedding.Embedding.Embedding2DArray) > 0 {
			// If it's a 2D array, take the first array
			values = append([]float64(nil), embedding.Embedding.Embedding2DArray[0]...)
		}

		geminiEmbedding := GeminiEmbedding{
			Values: values,
		}

		// Add statistics if available (token count from usage metadata)
		if gatewayResp.Usage != nil {
			geminiEmbedding.Statistics = &ContentEmbeddingStatistics{
				TokenCount: int32(gatewayResp.Usage.PromptTokens),
			}
		}

		geminiResp.Embeddings[i] = geminiEmbedding
	}

	// Set metadata if available (for Vertex API compatibility)
	if gatewayResp.Usage != nil {
		geminiResp.Metadata = &EmbedContentMetadata{
			BillableCharacterCount: int32(gatewayResp.Usage.PromptTokens),
		}
	}

	return geminiResp
}

// ToGatewayEmbeddingResponse converts a Gemini embedding response to GatewayEmbeddingResponse format
func ToGatewayEmbeddingResponse(geminiResp *GeminiEmbeddingResponse, model string) *schemas.GatewayEmbeddingResponse {
	if geminiResp == nil || len(geminiResp.Embeddings) == 0 {
		return nil
	}

	gatewayResp := &schemas.GatewayEmbeddingResponse{
		Data:   make([]schemas.EmbeddingData, len(geminiResp.Embeddings)),
		Model:  model,
		Object: "list",
	}

	// Convert each embedding from Gemini format to Gateway format
	for i, geminiEmbedding := range geminiResp.Embeddings {
		embeddingData := schemas.EmbeddingData{
			Index:  i,
			Object: "embedding",
			Embedding: schemas.EmbeddingStruct{
				EmbeddingArray: geminiEmbedding.Values,
			},
		}

		gatewayResp.Data[i] = embeddingData
	}

	// Convert usage metadata if available
	if geminiResp.Metadata != nil || (len(geminiResp.Embeddings) > 0 && geminiResp.Embeddings[0].Statistics != nil) {
		gatewayResp.Usage = &schemas.GatewayLLMUsage{}

		// Use statistics from the first embedding if available
		if geminiResp.Embeddings[0].Statistics != nil {
			gatewayResp.Usage.PromptTokens = int(geminiResp.Embeddings[0].Statistics.TokenCount)
		} else if geminiResp.Metadata != nil {
			// Fall back to metadata if statistics are not available
			gatewayResp.Usage.PromptTokens = int(geminiResp.Metadata.BillableCharacterCount)
		}

		// Set total tokens same as prompt tokens for embeddings
		gatewayResp.Usage.TotalTokens = gatewayResp.Usage.PromptTokens
	}

	return gatewayResp
}

// ToGatewayEmbeddingRequest converts a GeminiGenerationRequest to GatewayEmbeddingRequest format
func (request *GeminiGenerationRequest) ToGatewayEmbeddingRequest(ctx *schemas.GatewayContext) *schemas.GatewayEmbeddingRequest {
	if request == nil {
		return nil
	}

	provider, model := schemas.ParseModelString(request.Model, "")

	// Create the embedding request
	gatewayReq := &schemas.GatewayEmbeddingRequest{
		Provider:  provider,
		Model:     model,
		Fallbacks: schemas.ParseFallbacks(request.Fallbacks),
	}

	// SDK batch embedding request contains multiple embedding requests with same parameters but different text fields.
	if len(request.Requests) > 0 {
		var texts []string
		for _, req := range request.Requests {
			if req.Content != nil && len(req.Content.Parts) > 0 {
				for _, part := range req.Content.Parts {
					if part != nil && part.Text != "" {
						texts = append(texts, part.Text)
					}
				}
			}
		}
		if len(texts) > 0 {
			gatewayReq.Input = &schemas.EmbeddingInput{}
			if len(texts) == 1 {
				gatewayReq.Input.Text = &texts[0]
			} else {
				gatewayReq.Input.Texts = texts
			}
		}

		embeddingRequest := request.Requests[0]

		// Convert parameters
		if embeddingRequest.OutputDimensionality != nil || embeddingRequest.TaskType != nil || embeddingRequest.Title != nil {
			gatewayReq.Params = &schemas.EmbeddingParameters{}

			if embeddingRequest.OutputDimensionality != nil {
				gatewayReq.Params.Dimensions = embeddingRequest.OutputDimensionality
			}

			// Handle extra parameters
			if embeddingRequest.TaskType != nil || embeddingRequest.Title != nil {
				gatewayReq.Params.ExtraParams = make(map[string]interface{})
				if embeddingRequest.TaskType != nil {
					gatewayReq.Params.ExtraParams["taskType"] = embeddingRequest.TaskType
				}
				if embeddingRequest.Title != nil {
					gatewayReq.Params.ExtraParams["title"] = embeddingRequest.Title
				}
			}
		}
	}

	// Generation-style requests (e.g., non-Imagen :predict) carry text in contents[].parts[].
	// If no SDK requests[] were provided, derive embedding input from contents.
	if gatewayReq.Input == nil {
		var texts []string
		for _, content := range request.Contents {
			for _, part := range content.Parts {
				if part != nil && part.Text != "" {
					texts = append(texts, part.Text)
				}
			}
		}
		if len(texts) > 0 {
			gatewayReq.Input = &schemas.EmbeddingInput{}
			if len(texts) == 1 {
				gatewayReq.Input.Text = &texts[0]
			} else {
				gatewayReq.Input.Texts = texts
			}
		}
	}

	return gatewayReq
}
