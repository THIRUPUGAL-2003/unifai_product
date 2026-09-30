package gemini

import (
	"github.com/unifai/unifai/core/schemas"
)

// ToGeminiEmbeddingRequest converts a UnifAIRequest with embedding input to Gemini's batch embedding request format
// GeminiGenerationRequest contains requests array for batch embed content endpoint
func ToGeminiEmbeddingRequest(unifaiReq *schemas.UnifAIEmbeddingRequest) *GeminiBatchEmbeddingRequest {
	if unifaiReq == nil || unifaiReq.Input == nil || (unifaiReq.Input.Text == nil && unifaiReq.Input.Texts == nil) {
		return nil
	}

	embeddingInput := unifaiReq.Input

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
	if unifaiReq.Params != nil {
		batchRequest.ExtraParams = unifaiReq.Params.ExtraParams
	}

	// Create individual embedding requests for each text
	for i, text := range texts {
		embeddingReq := GeminiEmbeddingRequest{
			Model: "models/" + unifaiReq.Model,
			Content: &Content{
				Parts: []*Part{
					{
						Text: text,
					},
				},
			},
		}

		// Add parameters if available
		if unifaiReq.Params != nil {
			if unifaiReq.Params.Dimensions != nil {
				embeddingReq.OutputDimensionality = unifaiReq.Params.Dimensions
			}

			// Handle extra parameters
			if unifaiReq.Params.ExtraParams != nil {
				if taskType, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["taskType"]); ok {
					delete(batchRequest.ExtraParams, "taskType")
					embeddingReq.TaskType = taskType
				}
				if title, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["title"]); ok {
					delete(batchRequest.ExtraParams, "title")
					embeddingReq.Title = title
				}
			}
		}

		batchRequest.Requests[i] = embeddingReq
	}

	return batchRequest
}

// ToGeminiEmbedContentResponse converts a UnifAIEmbeddingResponse to the single :embedContent wire format.
func ToGeminiEmbedContentResponse(unifaiResp *schemas.UnifAIEmbeddingResponse) *GeminiEmbedContentResponse {
	if unifaiResp == nil || len(unifaiResp.Data) == 0 {
		return nil
	}
	values := unifaiResp.Data[0].Embedding.EmbeddingArray
	if values == nil && len(unifaiResp.Data[0].Embedding.Embedding2DArray) > 0 {
		values = unifaiResp.Data[0].Embedding.Embedding2DArray[0]
	}
	embedding := GeminiEmbedding{
		Values: append([]float64(nil), values...),
	}
	if unifaiResp.Usage != nil {
		embedding.Statistics = &ContentEmbeddingStatistics{
			TokenCount: int32(unifaiResp.Usage.PromptTokens),
		}
	}
	return &GeminiEmbedContentResponse{Embedding: embedding}
}

// ToGeminiEmbeddingResponse converts a UnifAIResponse with embedding data to Gemini's embedding response format
func ToGeminiEmbeddingResponse(unifaiResp *schemas.UnifAIEmbeddingResponse) *GeminiEmbeddingResponse {
	if unifaiResp == nil || len(unifaiResp.Data) == 0 {
		return nil
	}

	geminiResp := &GeminiEmbeddingResponse{
		Embeddings: make([]GeminiEmbedding, len(unifaiResp.Data)),
	}

	// Convert each embedding from UnifAI format to Gemini format
	for i, embedding := range unifaiResp.Data {
		var values []float64

		// Extract embedding values from UnifAIEmbeddingResponse
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
		if unifaiResp.Usage != nil {
			geminiEmbedding.Statistics = &ContentEmbeddingStatistics{
				TokenCount: int32(unifaiResp.Usage.PromptTokens),
			}
		}

		geminiResp.Embeddings[i] = geminiEmbedding
	}

	// Set metadata if available (for Vertex API compatibility)
	if unifaiResp.Usage != nil {
		geminiResp.Metadata = &EmbedContentMetadata{
			BillableCharacterCount: int32(unifaiResp.Usage.PromptTokens),
		}
	}

	return geminiResp
}

// ToUnifAIEmbeddingResponse converts a Gemini embedding response to UnifAIEmbeddingResponse format
func ToUnifAIEmbeddingResponse(geminiResp *GeminiEmbeddingResponse, model string) *schemas.UnifAIEmbeddingResponse {
	if geminiResp == nil || len(geminiResp.Embeddings) == 0 {
		return nil
	}

	unifaiResp := &schemas.UnifAIEmbeddingResponse{
		Data:   make([]schemas.EmbeddingData, len(geminiResp.Embeddings)),
		Model:  model,
		Object: "list",
	}

	// Convert each embedding from Gemini format to UnifAI format
	for i, geminiEmbedding := range geminiResp.Embeddings {
		embeddingData := schemas.EmbeddingData{
			Index:  i,
			Object: "embedding",
			Embedding: schemas.EmbeddingStruct{
				EmbeddingArray: geminiEmbedding.Values,
			},
		}

		unifaiResp.Data[i] = embeddingData
	}

	// Convert usage metadata if available
	if geminiResp.Metadata != nil || (len(geminiResp.Embeddings) > 0 && geminiResp.Embeddings[0].Statistics != nil) {
		unifaiResp.Usage = &schemas.UnifAILLMUsage{}

		// Use statistics from the first embedding if available
		if geminiResp.Embeddings[0].Statistics != nil {
			unifaiResp.Usage.PromptTokens = int(geminiResp.Embeddings[0].Statistics.TokenCount)
		} else if geminiResp.Metadata != nil {
			// Fall back to metadata if statistics are not available
			unifaiResp.Usage.PromptTokens = int(geminiResp.Metadata.BillableCharacterCount)
		}

		// Set total tokens same as prompt tokens for embeddings
		unifaiResp.Usage.TotalTokens = unifaiResp.Usage.PromptTokens
	}

	return unifaiResp
}

// ToUnifAIEmbeddingRequest converts a GeminiGenerationRequest to UnifAIEmbeddingRequest format
func (request *GeminiGenerationRequest) ToUnifAIEmbeddingRequest(ctx *schemas.UnifAIContext) *schemas.UnifAIEmbeddingRequest {
	if request == nil {
		return nil
	}

	provider, model := schemas.ParseModelString(request.Model, "")

	// Create the embedding request
	unifaiReq := &schemas.UnifAIEmbeddingRequest{
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
			unifaiReq.Input = &schemas.EmbeddingInput{}
			if len(texts) == 1 {
				unifaiReq.Input.Text = &texts[0]
			} else {
				unifaiReq.Input.Texts = texts
			}
		}

		embeddingRequest := request.Requests[0]

		// Convert parameters
		if embeddingRequest.OutputDimensionality != nil || embeddingRequest.TaskType != nil || embeddingRequest.Title != nil {
			unifaiReq.Params = &schemas.EmbeddingParameters{}

			if embeddingRequest.OutputDimensionality != nil {
				unifaiReq.Params.Dimensions = embeddingRequest.OutputDimensionality
			}

			// Handle extra parameters
			if embeddingRequest.TaskType != nil || embeddingRequest.Title != nil {
				unifaiReq.Params.ExtraParams = make(map[string]interface{})
				if embeddingRequest.TaskType != nil {
					unifaiReq.Params.ExtraParams["taskType"] = embeddingRequest.TaskType
				}
				if embeddingRequest.Title != nil {
					unifaiReq.Params.ExtraParams["title"] = embeddingRequest.Title
				}
			}
		}
	}

	// Generation-style requests (e.g., non-Imagen :predict) carry text in contents[].parts[].
	// If no SDK requests[] were provided, derive embedding input from contents.
	if unifaiReq.Input == nil {
		var texts []string
		for _, content := range request.Contents {
			for _, part := range content.Parts {
				if part != nil && part.Text != "" {
					texts = append(texts, part.Text)
				}
			}
		}
		if len(texts) > 0 {
			unifaiReq.Input = &schemas.EmbeddingInput{}
			if len(texts) == 1 {
				unifaiReq.Input.Text = &texts[0]
			} else {
				unifaiReq.Input.Texts = texts
			}
		}
	}

	return unifaiReq
}
