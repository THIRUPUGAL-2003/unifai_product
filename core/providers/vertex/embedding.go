package vertex

import (
	"github.com/gateway/gateway/core/schemas"
)

// ToVertexEmbeddingRequest converts a Gateway embedding request to Vertex AI format
func ToVertexEmbeddingRequest(gatewayReq *schemas.GatewayEmbeddingRequest) *VertexEmbeddingRequest {
	if gatewayReq == nil || gatewayReq.Input == nil || (gatewayReq.Input.Text == nil && gatewayReq.Input.Texts == nil) {
		return nil
	}
	// Create the request
	vertexReq := &VertexEmbeddingRequest{}
	if gatewayReq.Params != nil {
		vertexReq.ExtraParams = gatewayReq.Params.ExtraParams
	}
	var texts []string
	if gatewayReq.Input.Text != nil {
		texts = []string{*gatewayReq.Input.Text}
	} else {
		texts = gatewayReq.Input.Texts
	}

	// Create instances for each text
	instances := make([]VertexEmbeddingInstance, 0, len(texts))
	for _, text := range texts {
		instance := VertexEmbeddingInstance{
			Content: text,
		}

		// Add optional task_type and title from params
		if gatewayReq.Params != nil {
			if taskTypeStr, ok := schemas.SafeExtractStringPointer(gatewayReq.Params.ExtraParams["task_type"]); ok {
				delete(vertexReq.ExtraParams, "task_type")
				instance.TaskType = taskTypeStr
			}
			if title, ok := schemas.SafeExtractStringPointer(gatewayReq.Params.ExtraParams["title"]); ok {
				delete(vertexReq.ExtraParams, "title")
				instance.Title = title
			}
		}

		instances = append(instances, instance)
	}
	vertexReq.Instances = instances
	// Add parameters if present
	if gatewayReq.Params != nil {
		parameters := &VertexEmbeddingParameters{}

		// Set autoTruncate (defaults to true)
		autoTruncate := true
		if gatewayReq.Params.ExtraParams != nil {
			if autoTruncateVal, ok := schemas.SafeExtractBool(gatewayReq.Params.ExtraParams["autoTruncate"]); ok {
				delete(vertexReq.ExtraParams, "autoTruncate")
				autoTruncate = autoTruncateVal
			}
		}
		parameters.AutoTruncate = &autoTruncate

		// Add outputDimensionality if specified
		if gatewayReq.Params.Dimensions != nil {
			delete(vertexReq.ExtraParams, "dimensions")
			parameters.OutputDimensionality = gatewayReq.Params.Dimensions
		}

		vertexReq.Parameters = parameters
	}

	return vertexReq
}

// ToGatewayEmbeddingResponse converts a Vertex AI embedding response to Gateway format
func (response *VertexEmbeddingResponse) ToGatewayEmbeddingResponse() *schemas.GatewayEmbeddingResponse {
	if response == nil || len(response.Predictions) == 0 {
		return nil
	}

	// Convert predictions to Gateway embeddings
	embeddings := make([]schemas.EmbeddingData, 0, len(response.Predictions))
	var usage *schemas.GatewayLLMUsage

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
				usage = &schemas.GatewayLLMUsage{}
			}
			usage.TotalTokens += prediction.Embeddings.Statistics.TokenCount
			usage.PromptTokens += prediction.Embeddings.Statistics.TokenCount
		}

		embeddings = append(embeddings, embedding)
	}

	return &schemas.GatewayEmbeddingResponse{
		Object: "list",
		Data:   embeddings,
		Usage:  usage,
		ExtraFields: schemas.GatewayResponseExtraFields{
		},
	}
}
