package cohere

import (
	"sort"

	"github.com/bytedance/sonic"
	"github.com/unifai/unifai/core/schemas"
	"gopkg.in/yaml.v3"
)

// ToCohereRerankRequest converts a UnifAI rerank request to Cohere format
func ToCohereRerankRequest(unifaiReq *schemas.UnifAIRerankRequest) *CohereRerankRequest {
	if unifaiReq == nil {
		return nil
	}

	cohereReq := &CohereRerankRequest{
		Model: unifaiReq.Model,
		Query: unifaiReq.Query,
	}

	// Cohere v2 expects documents as a list of strings.
	documents := make([]string, len(unifaiReq.Documents))
	for i, doc := range unifaiReq.Documents {
		documents[i] = formatCohereRerankDocument(doc)
	}
	cohereReq.Documents = documents

	if unifaiReq.Params != nil {
		cohereReq.TopN = unifaiReq.Params.TopN
		cohereReq.MaxTokensPerDoc = unifaiReq.Params.MaxTokensPerDoc
		cohereReq.Priority = unifaiReq.Params.Priority
		cohereReq.ExtraParams = unifaiReq.Params.ExtraParams
	}

	return cohereReq
}

// ToUnifAIRerankRequest converts a Cohere rerank request to UnifAI format
func (req *CohereRerankRequest) ToUnifAIRerankRequest(ctx *schemas.UnifAIContext) *schemas.UnifAIRerankRequest {
	if req == nil {
		return nil
	}

	provider, model := schemas.ParseModelString(req.Model, "")

	unifaiReq := &schemas.UnifAIRerankRequest{
		Provider: provider,
		Model:    model,
		Query:    req.Query,
		Params:   &schemas.RerankParameters{},
	}

	// Convert documents
	for _, doc := range req.Documents {
		unifaiReq.Documents = append(unifaiReq.Documents, schemas.RerankDocument{
			Text: doc,
		})
	}

	if req.TopN != nil {
		unifaiReq.Params.TopN = req.TopN
	}
	if req.MaxTokensPerDoc != nil {
		unifaiReq.Params.MaxTokensPerDoc = req.MaxTokensPerDoc
	}
	if req.Priority != nil {
		unifaiReq.Params.Priority = req.Priority
	}
	if req.ExtraParams != nil {
		unifaiReq.Params.ExtraParams = req.ExtraParams
	}

	return unifaiReq
}

// ToUnifAIRerankResponse converts a Cohere rerank response to UnifAI format.
func (response *CohereRerankResponse) ToUnifAIRerankResponse(documents []schemas.RerankDocument, returnDocuments bool) *schemas.UnifAIRerankResponse {
	if response == nil {
		return nil
	}

	unifaiResponse := &schemas.UnifAIRerankResponse{
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

		unifaiResponse.Results = append(unifaiResponse.Results, rerankResult)
	}
	sort.SliceStable(unifaiResponse.Results, func(i, j int) bool {
		if unifaiResponse.Results[i].RelevanceScore == unifaiResponse.Results[j].RelevanceScore {
			return unifaiResponse.Results[i].Index < unifaiResponse.Results[j].Index
		}
		return unifaiResponse.Results[i].RelevanceScore > unifaiResponse.Results[j].RelevanceScore
	})
	if returnDocuments {
		for i := range unifaiResponse.Results {
			resultIndex := unifaiResponse.Results[i].Index
			if resultIndex >= 0 && resultIndex < len(documents) {
				unifaiResponse.Results[i].Document = schemas.Ptr(documents[resultIndex])
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
			unifaiResponse.Usage = &schemas.UnifAILLMUsage{
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				TotalTokens:      promptTokens + completionTokens,
			}
		}
	}

	return unifaiResponse
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
