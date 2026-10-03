package cohere

import (
	"sort"

	"github.com/bytedance/sonic"
	"github.com/raksha/raksha/core/schemas"
	"gopkg.in/yaml.v3"
)

// ToCohereRerankRequest converts a Raksha rerank request to Cohere format
func ToCohereRerankRequest(rakshaReq *schemas.RakshaRerankRequest) *CohereRerankRequest {
	if rakshaReq == nil {
		return nil
	}

	cohereReq := &CohereRerankRequest{
		Model: rakshaReq.Model,
		Query: rakshaReq.Query,
	}

	// Cohere v2 expects documents as a list of strings.
	documents := make([]string, len(rakshaReq.Documents))
	for i, doc := range rakshaReq.Documents {
		documents[i] = formatCohereRerankDocument(doc)
	}
	cohereReq.Documents = documents

	if rakshaReq.Params != nil {
		cohereReq.TopN = rakshaReq.Params.TopN
		cohereReq.MaxTokensPerDoc = rakshaReq.Params.MaxTokensPerDoc
		cohereReq.Priority = rakshaReq.Params.Priority
		cohereReq.ExtraParams = rakshaReq.Params.ExtraParams
	}

	return cohereReq
}

// ToRakshaRerankRequest converts a Cohere rerank request to Raksha format
func (req *CohereRerankRequest) ToRakshaRerankRequest(ctx *schemas.RakshaContext) *schemas.RakshaRerankRequest {
	if req == nil {
		return nil
	}

	provider, model := schemas.ParseModelString(req.Model, "")

	rakshaReq := &schemas.RakshaRerankRequest{
		Provider: provider,
		Model:    model,
		Query:    req.Query,
		Params:   &schemas.RerankParameters{},
	}

	// Convert documents
	for _, doc := range req.Documents {
		rakshaReq.Documents = append(rakshaReq.Documents, schemas.RerankDocument{
			Text: doc,
		})
	}

	if req.TopN != nil {
		rakshaReq.Params.TopN = req.TopN
	}
	if req.MaxTokensPerDoc != nil {
		rakshaReq.Params.MaxTokensPerDoc = req.MaxTokensPerDoc
	}
	if req.Priority != nil {
		rakshaReq.Params.Priority = req.Priority
	}
	if req.ExtraParams != nil {
		rakshaReq.Params.ExtraParams = req.ExtraParams
	}

	return rakshaReq
}

// ToRakshaRerankResponse converts a Cohere rerank response to Raksha format.
func (response *CohereRerankResponse) ToRakshaRerankResponse(documents []schemas.RerankDocument, returnDocuments bool) *schemas.RakshaRerankResponse {
	if response == nil {
		return nil
	}

	rakshaResponse := &schemas.RakshaRerankResponse{
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

		rakshaResponse.Results = append(rakshaResponse.Results, rerankResult)
	}
	sort.SliceStable(rakshaResponse.Results, func(i, j int) bool {
		if rakshaResponse.Results[i].RelevanceScore == rakshaResponse.Results[j].RelevanceScore {
			return rakshaResponse.Results[i].Index < rakshaResponse.Results[j].Index
		}
		return rakshaResponse.Results[i].RelevanceScore > rakshaResponse.Results[j].RelevanceScore
	})
	if returnDocuments {
		for i := range rakshaResponse.Results {
			resultIndex := rakshaResponse.Results[i].Index
			if resultIndex >= 0 && resultIndex < len(documents) {
				rakshaResponse.Results[i].Document = schemas.Ptr(documents[resultIndex])
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
			rakshaResponse.Usage = &schemas.RakshaLLMUsage{
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				TotalTokens:      promptTokens + completionTokens,
			}
		}
	}

	return rakshaResponse
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
