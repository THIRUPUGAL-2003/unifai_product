package vertex

import (
	"github.com/raksha/raksha/core/schemas"
)

// ToVertexEmbeddingRequest converts a Raksha embedding request to Vertex AI format
func ToVertexEmbeddingRequest(rakshaReq *schemas.RakshaEmbeddingRequest) *VertexEmbeddingRequest {
	if rakshaReq == nil || rakshaReq.Input == nil || (rakshaReq.Input.Text == nil && rakshaReq.Input.Texts == nil) {
		return nil
	}
	// Create the request
	vertexReq := &VertexEmbeddingRequest{}
	if rakshaReq.Params != nil {
		vertexReq.ExtraParams = rakshaReq.Params.ExtraParams
	}
	var texts []string
	if rakshaReq.Input.Text != nil {
		texts = []string{*rakshaReq.Input.Text}
	} else {
		texts = rakshaReq.Input.Texts
	}

	// Create instances for each text
	instances := make([]VertexEmbeddingInstance, 0, len(texts))
	for _, text := range texts {
		instance := VertexEmbeddingInstance{
			Content: text,
		}

		// Add optional task_type and title from params
		if rakshaReq.Params != nil {
			if taskTypeStr, ok := schemas.SafeExtractStringPointer(rakshaReq.Params.ExtraParams["task_type"]); ok {
				delete(vertexReq.ExtraParams, "task_type")
				instance.TaskType = taskTypeStr
			}
			if title, ok := schemas.SafeExtractStringPointer(rakshaReq.Params.ExtraParams["title"]); ok {
				delete(vertexReq.ExtraParams, "title")
				instance.Title = title
			}
		}

		instances = append(instances, instance)
	}
	vertexReq.Instances = instances
	// Add parameters if present
	if rakshaReq.Params != nil {
		parameters := &VertexEmbeddingParameters{}

		// Set autoTruncate (defaults to true)
		autoTruncate := true
		if rakshaReq.Params.ExtraParams != nil {
			if autoTruncateVal, ok := schemas.SafeExtractBool(rakshaReq.Params.ExtraParams["autoTruncate"]); ok {
				delete(vertexReq.ExtraParams, "autoTruncate")
				autoTruncate = autoTruncateVal
			}
		}
		parameters.AutoTruncate = &autoTruncate

		// Add outputDimensionality if specified
		if rakshaReq.Params.Dimensions != nil {
			delete(vertexReq.ExtraParams, "dimensions")
			parameters.OutputDimensionality = rakshaReq.Params.Dimensions
		}

		vertexReq.Parameters = parameters
	}

	return vertexReq
}

// ToRakshaEmbeddingResponse converts a Vertex AI embedding response to Raksha format
func (response *VertexEmbeddingResponse) ToRakshaEmbeddingResponse() *schemas.RakshaEmbeddingResponse {
	if response == nil || len(response.Predictions) == 0 {
		return nil
	}

	// Convert predictions to Raksha embeddings
	embeddings := make([]schemas.EmbeddingData, 0, len(response.Predictions))
	var usage *schemas.RakshaLLMUsage

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
				usage = &schemas.RakshaLLMUsage{}
			}
			usage.TotalTokens += prediction.Embeddings.Statistics.TokenCount
			usage.PromptTokens += prediction.Embeddings.Statistics.TokenCount
		}

		embeddings = append(embeddings, embedding)
	}

	return &schemas.RakshaEmbeddingResponse{
		Object: "list",
		Data:   embeddings,
		Usage:  usage,
		ExtraFields: schemas.RakshaResponseExtraFields{
		},
	}
}
