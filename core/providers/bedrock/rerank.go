package bedrock

import (
	"fmt"
	"sort"
	"strings"

	"github.com/unifai/unifai/core/schemas"
)

// ToBedrockRerankRequest converts a UnifAI rerank request into Bedrock Agent Runtime format.
func ToBedrockRerankRequest(unifaiReq *schemas.UnifAIRerankRequest, modelARN string) (*BedrockRerankRequest, error) {
	if unifaiReq == nil {
		return nil, fmt.Errorf("unifai rerank request is nil")
	}
	if strings.TrimSpace(modelARN) == "" {
		return nil, fmt.Errorf("bedrock rerank model ARN is empty")
	}
	if len(unifaiReq.Documents) == 0 {
		return nil, fmt.Errorf("documents are required for rerank request")
	}

	bedrockReq := &BedrockRerankRequest{
		Queries: []BedrockRerankQuery{
			{
				Type: bedrockRerankQueryTypeText,
				TextQuery: BedrockRerankTextRef{
					Text: unifaiReq.Query,
				},
			},
		},
		Sources: make([]BedrockRerankSource, len(unifaiReq.Documents)),
		RerankingConfiguration: BedrockRerankingConfiguration{
			Type: bedrockRerankConfigurationTypeBedrock,
			BedrockRerankingConfiguration: BedrockRerankingModelConfiguration{
				ModelConfiguration: BedrockRerankModelConfiguration{
					ModelARN: modelARN,
				},
			},
		},
	}

	for i, doc := range unifaiReq.Documents {
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

	if unifaiReq.Params == nil {
		return bedrockReq, nil
	}

	if unifaiReq.Params.TopN != nil {
		topN := *unifaiReq.Params.TopN
		if topN < 1 {
			return nil, fmt.Errorf("top_n must be at least 1")
		}
		if topN > len(unifaiReq.Documents) {
			topN = len(unifaiReq.Documents)
		}
		bedrockReq.RerankingConfiguration.BedrockRerankingConfiguration.NumberOfResults = schemas.Ptr(topN)
	}

	additionalFields := make(map[string]interface{})
	if unifaiReq.Params.MaxTokensPerDoc != nil {
		additionalFields["max_tokens_per_doc"] = *unifaiReq.Params.MaxTokensPerDoc
	}
	if unifaiReq.Params.Priority != nil {
		additionalFields["priority"] = *unifaiReq.Params.Priority
	}
	for k, v := range unifaiReq.Params.ExtraParams {
		additionalFields[k] = v
	}
	if len(additionalFields) > 0 {
		bedrockReq.RerankingConfiguration.BedrockRerankingConfiguration.ModelConfiguration.AdditionalModelRequestFields = additionalFields
	}

	return bedrockReq, nil
}

// ToUnifAIRerankResponse converts a Bedrock rerank response into UnifAI format.
func (response *BedrockRerankResponse) ToUnifAIRerankResponse(documents []schemas.RerankDocument, returnDocuments bool) *schemas.UnifAIRerankResponse {
	if response == nil {
		return nil
	}

	unifaiResponse := &schemas.UnifAIRerankResponse{
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

	return unifaiResponse
}

// ToUnifAIRerankRequest converts a Bedrock Agent Runtime rerank request to UnifAI format.
func (req *BedrockRerankRequest) ToUnifAIRerankRequest(ctx *schemas.UnifAIContext) *schemas.UnifAIRerankRequest {
	if req == nil {
		return nil
	}

	modelARN := req.RerankingConfiguration.BedrockRerankingConfiguration.ModelConfiguration.ModelARN
	provider, model := schemas.ParseModelString(modelARN, "")

	unifaiReq := &schemas.UnifAIRerankRequest{
		Provider: provider,
		Model:    model,
		Params:   &schemas.RerankParameters{},
	}

	// Extract query from the first query entry
	if len(req.Queries) > 0 {
		unifaiReq.Query = req.Queries[0].TextQuery.Text
	}

	// Convert sources to documents
	for _, source := range req.Sources {
		unifaiReq.Documents = append(unifaiReq.Documents, schemas.RerankDocument{
			Text: source.InlineDocumentSource.TextDocument.Text,
		})
	}

	// Extract TopN from NumberOfResults
	if req.RerankingConfiguration.BedrockRerankingConfiguration.NumberOfResults != nil {
		unifaiReq.Params.TopN = req.RerankingConfiguration.BedrockRerankingConfiguration.NumberOfResults
	}

	// Pass AdditionalModelRequestFields as ExtraParams
	if fields := req.RerankingConfiguration.BedrockRerankingConfiguration.ModelConfiguration.AdditionalModelRequestFields; len(fields) > 0 {
		unifaiReq.Params.ExtraParams = fields
	}

	return unifaiReq
}
