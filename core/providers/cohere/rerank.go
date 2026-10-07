package cohere

import (
	"sort"

	"github.com/bytedance/sonic"
	"github.com/gateway/gateway/core/schemas"
	"gopkg.in/yaml.v3"
)

// ToCohereRerankRequest converts a Gateway rerank request to Cohere format
func ToCohereRerankRequest(gatewayReq *schemas.GatewayRerankRequest) *CohereRerankRequest {
	if gatewayReq == nil {
		return nil
	}

	cohereReq := &CohereRerankRequest{
		Model: gatewayReq.Model,
		Query: gatewayReq.Query,
	}

	// Cohere v2 expects documents as a list of strings.
	documents := make([]string, len(gatewayReq.Documents))
	for i, doc := range gatewayReq.Documents {
		documents[i] = formatCohereRerankDocument(doc)
	}
	cohereReq.Documents = documents

	if gatewayReq.Params != nil {
		cohereReq.TopN = gatewayReq.Params.TopN
		cohereReq.MaxTokensPerDoc = gatewayReq.Params.MaxTokensPerDoc
		cohereReq.Priority = gatewayReq.Params.Priority
		cohereReq.ExtraParams = gatewayReq.Params.ExtraParams
	}

	return cohereReq
}

// ToGatewayRerankRequest converts a Cohere rerank request to Gateway format
func (req *CohereRerankRequest) ToGatewayRerankRequest(ctx *schemas.GatewayContext) *schemas.GatewayRerankRequest {
	if req == nil {
		return nil
	}

	provider, model := schemas.ParseModelString(req.Model, "")

	gatewayReq := &schemas.GatewayRerankRequest{
		Provider: provider,
		Model:    model,
		Query:    req.Query,
		Params:   &schemas.RerankParameters{},
	}

	// Convert documents
	for _, doc := range req.Documents {
		gatewayReq.Documents = append(gatewayReq.Documents, schemas.RerankDocument{
			Text: doc,
		})
	}

	if req.TopN != nil {
		gatewayReq.Params.TopN = req.TopN
	}
	if req.MaxTokensPerDoc != nil {
		gatewayReq.Params.MaxTokensPerDoc = req.MaxTokensPerDoc
	}
	if req.Priority != nil {
		gatewayReq.Params.Priority = req.Priority
	}
	if req.ExtraParams != nil {
		gatewayReq.Params.ExtraParams = req.ExtraParams
	}

	return gatewayReq
}

// ToGatewayRerankResponse converts a Cohere rerank response to Gateway format.
func (response *CohereRerankResponse) ToGatewayRerankResponse(documents []schemas.RerankDocument, returnDocuments bool) *schemas.GatewayRerankResponse {
	if response == nil {
		return nil
	}

	gatewayResponse := &schemas.GatewayRerankResponse{
		ID: response.ID,
	}

	// Convert results
	for _, result := range response.Results {
		rerankResult := schemas.RerankResult{
			Index:          result.Index,
			RelevanceScore: result.RelevanceScore,
		}

		// Convert document if present
		if len(result.Document) > 0 {
			var docMap map[string]interface{}
			if err := sonic.Unmarshal(result.Document, &docMap); err == nil {
				doc := &schemas.RerankDocument{}
				populated := false
				if text, ok := docMap["text"].(string); ok {
					doc.Text = text
					populated = true
				}
				if id, ok := docMap["id"].(string); ok {
					doc.ID = &id
					populated = true
				}
				// Collect metadata: unwrap "metadata"/"meta" keys to avoid nesting
				meta := make(map[string]interface{})
				if rawMeta, ok := docMap["metadata"].(map[string]interface{}); ok {
					for k, v := range rawMeta {
						meta[k] = v
					}
				} else if rawMeta, ok := docMap["meta"].(map[string]interface{}); ok {
					for k, v := range rawMeta {
						meta[k] = v
					}
				}
				for k, v := range docMap {
					if k != "text" && k != "id" && k != "metadata" && k != "meta" {
						meta[k] = v
					}
				}
				if len(meta) > 0 {
					doc.Meta = meta
					populated = true
				}
				if populated {
					rerankResult.Document = doc
				}
			}
		}

		gatewayResponse.Results = append(gatewayResponse.Results, rerankResult)
	}
	sort.SliceStable(gatewayResponse.Results, func(i, j int) bool {
		if gatewayResponse.Results[i].RelevanceScore == gatewayResponse.Results[j].RelevanceScore {
			return gatewayResponse.Results[i].Index < gatewayResponse.Results[j].Index
		}
		return gatewayResponse.Results[i].RelevanceScore > gatewayResponse.Results[j].RelevanceScore
	})
	if returnDocuments {
		for i := range gatewayResponse.Results {
			resultIndex := gatewayResponse.Results[i].Index
			if resultIndex >= 0 && resultIndex < len(documents) {
				gatewayResponse.Results[i].Document = schemas.Ptr(documents[resultIndex])
			}
		}
	}

	// Convert usage information
	if response.Meta != nil {
		promptTokens := 0
		completionTokens := 0
		hasTokenUsage := false
		if response.Meta.Tokens != nil {
			if response.Meta.Tokens.InputTokens != nil {
				promptTokens = int(*response.Meta.Tokens.InputTokens)
				hasTokenUsage = true
			}
			if response.Meta.Tokens.OutputTokens != nil {
				completionTokens = int(*response.Meta.Tokens.OutputTokens)
				hasTokenUsage = true
			}
		} else if response.Meta.BilledUnits != nil {
			if response.Meta.BilledUnits.InputTokens != nil {
				promptTokens = int(*response.Meta.BilledUnits.InputTokens)
				hasTokenUsage = true
			}
			if response.Meta.BilledUnits.OutputTokens != nil {
				completionTokens = int(*response.Meta.BilledUnits.OutputTokens)
				hasTokenUsage = true
			}
		}
		if hasTokenUsage {
			gatewayResponse.Usage = &schemas.GatewayLLMUsage{
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				TotalTokens:      promptTokens + completionTokens,
			}
		}
	}

	return gatewayResponse
}

func formatCohereRerankDocument(doc schemas.RerankDocument) string {
	if doc.ID == nil && len(doc.Meta) == 0 {
		return doc.Text
	}

	// Keep metadata/id available by encoding a structured string document.
	documentPayload := map[string]interface{}{
		"text": doc.Text,
	}
	if doc.ID != nil {
		documentPayload["id"] = *doc.ID
	}
	if len(doc.Meta) > 0 {
		documentPayload["metadata"] = doc.Meta
	}

	encoded, err := yaml.Marshal(documentPayload)
	if err != nil {
		return doc.Text
	}
	return string(encoded)
}
