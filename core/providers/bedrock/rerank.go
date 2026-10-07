package bedrock

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gateway/gateway/core/schemas"
)

// ToBedrockRerankRequest converts a Gateway rerank request into Bedrock Agent Runtime format.
func ToBedrockRerankRequest(gatewayReq *schemas.GatewayRerankRequest, modelARN string) (*BedrockRerankRequest, error) {
	if gatewayReq == nil {
		return nil, fmt.Errorf("gateway rerank request is nil")
	}
	if strings.TrimSpace(modelARN) == "" {
		return nil, fmt.Errorf("bedrock rerank model ARN is empty")
	}
	if len(gatewayReq.Documents) == 0 {
		return nil, fmt.Errorf("documents are required for rerank request")
	}

	bedrockReq := &BedrockRerankRequest{
		Queries: []BedrockRerankQuery{
			{
				Type: bedrockRerankQueryTypeText,
				TextQuery: BedrockRerankTextRef{
					Text: gatewayReq.Query,
				},
			},
		},
		Sources: make([]BedrockRerankSource, len(gatewayReq.Documents)),
		RerankingConfiguration: BedrockRerankingConfiguration{
			Type: bedrockRerankConfigurationTypeBedrock,
			BedrockRerankingConfiguration: BedrockRerankingModelConfiguration{
				ModelConfiguration: BedrockRerankModelConfiguration{
					ModelARN: modelARN,
				},
			},
		},
	}

	for i, doc := range gatewayReq.Documents {
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

	if gatewayReq.Params == nil {
		return bedrockReq, nil
	}

	if gatewayReq.Params.TopN != nil {
		topN := *gatewayReq.Params.TopN
		if topN < 1 {
			return nil, fmt.Errorf("top_n must be at least 1")
		}
		if topN > len(gatewayReq.Documents) {
			topN = len(gatewayReq.Documents)
		}
		bedrockReq.RerankingConfiguration.BedrockRerankingConfiguration.NumberOfResults = schemas.Ptr(topN)
	}

	additionalFields := make(map[string]interface{})
	if gatewayReq.Params.MaxTokensPerDoc != nil {
		additionalFields["max_tokens_per_doc"] = *gatewayReq.Params.MaxTokensPerDoc
	}
	if gatewayReq.Params.Priority != nil {
		additionalFields["priority"] = *gatewayReq.Params.Priority
	}
	for k, v := range gatewayReq.Params.ExtraParams {
		additionalFields[k] = v
	}
	if len(additionalFields) > 0 {
		bedrockReq.RerankingConfiguration.BedrockRerankingConfiguration.ModelConfiguration.AdditionalModelRequestFields = additionalFields
	}

	return bedrockReq, nil
}

// ToGatewayRerankResponse converts a Bedrock rerank response into Gateway format.
func (response *BedrockRerankResponse) ToGatewayRerankResponse(documents []schemas.RerankDocument, returnDocuments bool) *schemas.GatewayRerankResponse {
	if response == nil {
		return nil
	}

	gatewayResponse := &schemas.GatewayRerankResponse{
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

	return gatewayResponse
}

// ToGatewayRerankRequest converts a Bedrock Agent Runtime rerank request to Gateway format.
func (req *BedrockRerankRequest) ToGatewayRerankRequest(ctx *schemas.GatewayContext) *schemas.GatewayRerankRequest {
	if req == nil {
		return nil
	}

	modelARN := req.RerankingConfiguration.BedrockRerankingConfiguration.ModelConfiguration.ModelARN
	provider, model := schemas.ParseModelString(modelARN, "")

	gatewayReq := &schemas.GatewayRerankRequest{
		Provider: provider,
		Model:    model,
		Params:   &schemas.RerankParameters{},
	}

	// Extract query from the first query entry
	if len(req.Queries) > 0 {
		gatewayReq.Query = req.Queries[0].TextQuery.Text
	}

	// Convert sources to documents
	for _, source := range req.Sources {
		gatewayReq.Documents = append(gatewayReq.Documents, schemas.RerankDocument{
			Text: source.InlineDocumentSource.TextDocument.Text,
		})
	}

	// Extract TopN from NumberOfResults
	if req.RerankingConfiguration.BedrockRerankingConfiguration.NumberOfResults != nil {
		gatewayReq.Params.TopN = req.RerankingConfiguration.BedrockRerankingConfiguration.NumberOfResults
	}

	// Pass AdditionalModelRequestFields as ExtraParams
	if fields := req.RerankingConfiguration.BedrockRerankingConfiguration.ModelConfiguration.AdditionalModelRequestFields; len(fields) > 0 {
		gatewayReq.Params.ExtraParams = fields
	}

	return gatewayReq
}
