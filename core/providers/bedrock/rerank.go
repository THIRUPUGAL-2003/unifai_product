package bedrock

import (
	"fmt"
	"sort"
	"strings"

	"github.com/raksha/raksha/core/schemas"
)

// ToBedrockRerankRequest converts a Raksha rerank request into Bedrock Agent Runtime format.
func ToBedrockRerankRequest(rakshaReq *schemas.RakshaRerankRequest, modelARN string) (*BedrockRerankRequest, error) {
	if rakshaReq == nil {
		return nil, fmt.Errorf("raksha rerank request is nil")
	}
	if strings.TrimSpace(modelARN) == "" {
		return nil, fmt.Errorf("bedrock rerank model ARN is empty")
	}
	if len(rakshaReq.Documents) == 0 {
		return nil, fmt.Errorf("documents are required for rerank request")
	}

	bedrockReq := &BedrockRerankRequest{
		Queries: []BedrockRerankQuery{
			{
				Type: bedrockRerankQueryTypeText,
				TextQuery: BedrockRerankTextRef{
					Text: rakshaReq.Query,
				},
			},
		},
		Sources: make([]BedrockRerankSource, len(rakshaReq.Documents)),
		RerankingConfiguration: BedrockRerankingConfiguration{
			Type: bedrockRerankConfigurationTypeBedrock,
			BedrockRerankingConfiguration: BedrockRerankingModelConfiguration{
				ModelConfiguration: BedrockRerankModelConfiguration{
					ModelARN: modelARN,
				},
			},
		},
	}

	for i, doc := range rakshaReq.Documents {
		bedrockReq.Sources[i] = BedrockRerankSource{
			Type: bedrockRerankSourceTypeInline,
			InlineDocumentSource: BedrockRerankInlineSource{
				Type: bedrockRerankInlineDocumentTypeText,
				TextDocument: BedrockRerankTextValue{
					Text: doc.Text,
				},
			},
		}
	}

	if rakshaReq.Params == nil {
		return bedrockReq, nil
	}

	if rakshaReq.Params.TopN != nil {
		topN := *rakshaReq.Params.TopN
		if topN < 1 {
			return nil, fmt.Errorf("top_n must be at least 1")
		}
		if topN > len(rakshaReq.Documents) {
			topN = len(rakshaReq.Documents)
		}
		bedrockReq.RerankingConfiguration.BedrockRerankingConfiguration.NumberOfResults = schemas.Ptr(topN)
	}

	additionalFields := make(map[string]interface{})
	if rakshaReq.Params.MaxTokensPerDoc != nil {
		additionalFields["max_tokens_per_doc"] = *rakshaReq.Params.MaxTokensPerDoc
	}
	if rakshaReq.Params.Priority != nil {
		additionalFields["priority"] = *rakshaReq.Params.Priority
	}
	for k, v := range rakshaReq.Params.ExtraParams {
		additionalFields[k] = v
	}
	if len(additionalFields) > 0 {
		bedrockReq.RerankingConfiguration.BedrockRerankingConfiguration.ModelConfiguration.AdditionalModelRequestFields = additionalFields
	}

	return bedrockReq, nil
}

// ToRakshaRerankResponse converts a Bedrock rerank response into Raksha format.
func (response *BedrockRerankResponse) ToRakshaRerankResponse(documents []schemas.RerankDocument, returnDocuments bool) *schemas.RakshaRerankResponse {
	if response == nil {
		return nil
	}

	rakshaResponse := &schemas.RakshaRerankResponse{
		Results: make([]schemas.RerankResult, 0, len(response.Results)),
	}

	for _, result := range response.Results {
		rerankResult := schemas.RerankResult{
			Index:          result.Index,
			RelevanceScore: result.RelevanceScore,
		}
		if result.Document != nil && result.Document.TextDocument != nil {
			rerankResult.Document = &schemas.RerankDocument{
				Text: result.Document.TextDocument.Text,
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

	return rakshaResponse
}

// ToRakshaRerankRequest converts a Bedrock Agent Runtime rerank request to Raksha format.
func (req *BedrockRerankRequest) ToRakshaRerankRequest(ctx *schemas.RakshaContext) *schemas.RakshaRerankRequest {
	if req == nil {
		return nil
	}

	modelARN := req.RerankingConfiguration.BedrockRerankingConfiguration.ModelConfiguration.ModelARN
	provider, model := schemas.ParseModelString(modelARN, "")

	rakshaReq := &schemas.RakshaRerankRequest{
		Provider: provider,
		Model:    model,
		Params:   &schemas.RerankParameters{},
	}

	// Extract query from the first query entry
	if len(req.Queries) > 0 {
		rakshaReq.Query = req.Queries[0].TextQuery.Text
	}

	// Convert sources to documents
	for _, source := range req.Sources {
		rakshaReq.Documents = append(rakshaReq.Documents, schemas.RerankDocument{
			Text: source.InlineDocumentSource.TextDocument.Text,
		})
	}

	// Extract TopN from NumberOfResults
	if req.RerankingConfiguration.BedrockRerankingConfiguration.NumberOfResults != nil {
		rakshaReq.Params.TopN = req.RerankingConfiguration.BedrockRerankingConfiguration.NumberOfResults
	}

	// Pass AdditionalModelRequestFields as ExtraParams
	if fields := req.RerankingConfiguration.BedrockRerankingConfiguration.ModelConfiguration.AdditionalModelRequestFields; len(fields) > 0 {
		rakshaReq.Params.ExtraParams = fields
	}

	return rakshaReq
}
