package vertex

import (
	"github.com/unifai/unifai/core/schemas"
)

// ToVertexEmbeddingRequest converts a UnifAI embedding request to Vertex AI format
func ToVertexEmbeddingRequest(unifaiReq *schemas.UnifAIEmbeddingRequest) *VertexEmbeddingRequest {
	if unifaiReq == nil || unifaiReq.Input == nil || (unifaiReq.Input.Text == nil && unifaiReq.Input.Texts == nil) {
		return nil
	}
	// Create the request
	vertexReq := &VertexEmbeddingRequest{}
	if unifaiReq.Params != nil {
		vertexReq.ExtraParams = unifaiReq.Params.ExtraParams
	}
	var texts []string
	if unifaiReq.Input.Text != nil {
		texts = []string{*unifaiReq.Input.Text}
	} else {
		texts = unifaiReq.Input.Texts
	}

	// Create instances for each text
	instances := make([]VertexEmbeddingInstance, 0, len(texts))
	for _, text := range texts {
		instance := VertexEmbeddingInstance{
			Content: text,
		}

		// Add optional task_type and title from params
		if unifaiReq.Params != nil {
			if taskTypeStr, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["task_type"]); ok {
				delete(vertexReq.ExtraParams, "task_type")
				instance.TaskType = taskTypeStr
			}
			if title, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["title"]); ok {
				delete(vertexReq.ExtraParams, "title")
				instance.Title = title
			}
		}

		instances = append(instances, instance)
	}
	vertexReq.Instances = instances
	// Add parameters if present
	if unifaiReq.Params != nil {
		parameters := &VertexEmbeddingParameters{}

		// Set autoTruncate (defaults to true)
		autoTruncate := true
		if unifaiReq.Params.ExtraParams != nil {
			if autoTruncateVal, ok := schemas.SafeExtractBool(unifaiReq.Params.ExtraParams["autoTruncate"]); ok {
				delete(vertexReq.ExtraParams, "autoTruncate")
				autoTruncate = autoTruncateVal
			}
		}
		parameters.AutoTruncate = &autoTruncate

		// Add outputDimensionality if specified
		if unifaiReq.Params.Dimensions != nil {
			delete(vertexReq.ExtraParams, "dimensions")
			parameters.OutputDimensionality = unifaiReq.Params.Dimensions
		}

		vertexReq.Parameters = parameters
	}

	return vertexReq
}

// ToUnifAIEmbeddingResponse converts a Vertex AI embedding response to UnifAI format
func (response *VertexEmbeddingResponse) ToUnifAIEmbeddingResponse() *schemas.UnifAIEmbeddingResponse {
	if response == nil || len(response.Predictions) == 0 {
		return nil
	}

	// Convert predictions to UnifAI embeddings
	embeddings := make([]schemas.EmbeddingData, 0, len(response.Predictions))
	var usage *schemas.UnifAILLMUsage

	for i, prediction := range response.Predictions {
		if prediction.Embeddings == nil || len(prediction.Embeddings.Values) == 0 {
			continue
		}

		// Create embedding object
		embedding := schemas.EmbeddingData{
			Object: "embedding",
			Embedding: schemas.EmbeddingStruct{
				EmbeddingArray: append([]float64(nil), prediction.Embeddings.Values...),
			},
			Index: i,
		}

		// Extract statistics if available
		if prediction.Embeddings.Statistics != nil {
			if usage == nil {
				usage = &schemas.UnifAILLMUsage{}
			}
			usage.TotalTokens += prediction.Embeddings.Statistics.TokenCount
			usage.PromptTokens += prediction.Embeddings.Statistics.TokenCount
		}

		embeddings = append(embeddings, embedding)
	}

	return &schemas.UnifAIEmbeddingResponse{
		Object: "list",
		Data:   embeddings,
		Usage:  usage,
		ExtraFields: schemas.UnifAIResponseExtraFields{
		},
	}
}
