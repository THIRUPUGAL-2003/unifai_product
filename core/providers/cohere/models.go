package cohere

import (
	"encoding/json"
	"strings"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
)

// CohereRerankRequest represents a Cohere rerank API request.
type CohereRerankRequest struct {
	Model           string                 `json:"model"`
	Query           string                 `json:"query"`
	Documents       []string               `json:"documents"`
	TopN            *int                   `json:"top_n,omitempty"`
	MaxTokensPerDoc *int                   `json:"max_tokens_per_doc,omitempty"`
	Priority        *int                   `json:"priority,omitempty"`
	ExtraParams     map[string]interface{} `json:"-"`
}

// GetExtraParams returns extra parameters for the rerank request.
func (r *CohereRerankRequest) GetExtraParams() map[string]interface{} {
	return r.ExtraParams
}

// CohereRerankResult represents a single result from Cohere rerank.
type CohereRerankResult struct {
	Index          int             `json:"index"`
	RelevanceScore float64         `json:"relevance_score"`
	Document       json.RawMessage `json:"document,omitempty"`
}

// CohereRerankResponse represents a Cohere rerank API response.
type CohereRerankResponse struct {
	ID      string               `json:"id"`
	Results []CohereRerankResult `json:"results"`
	Meta    *CohereRerankMeta    `json:"meta,omitempty"`
}

// CohereRerankMeta represents metadata in Cohere rerank response.
type CohereRerankMeta struct {
	APIVersion  *CohereEmbeddingAPIVersion `json:"api_version,omitempty"`
	BilledUnits *CohereBilledUnits         `json:"billed_units,omitempty"`
	Tokens      *CohereTokenUsage          `json:"tokens,omitempty"`
}

func (response *CohereListModelsResponse) ToUnifAIListModelsResponse(providerKey schemas.ModelProvider, allowedModels schemas.WhiteList, blacklistedModels schemas.BlackList, aliases schemas.KeyAliases, unfiltered bool) *schemas.UnifAIListModelsResponse {
	if response == nil {
		return nil
	}

	unifaiResponse := &schemas.UnifAIListModelsResponse{
		Data: make([]schemas.Model, 0, len(response.Models)),
	}

	pipeline := &providerUtils.ListModelsPipeline{
		AllowedModels:     allowedModels,
		BlacklistedModels: blacklistedModels,
		Aliases:           aliases,
		Unfiltered:        unfiltered,
		ProviderKey:       providerKey,
		MatchFns:          providerUtils.DefaultMatchFns(),
	}
	if pipeline.ShouldEarlyExit() {
		return unifaiResponse
	}

	included := make(map[string]bool)

	for _, model := range response.Models {
		// Cohere uses model.Name as the model identifier
		for _, result := range pipeline.FilterModel(model.Name) {
			entry := schemas.Model{
				ID:               string(providerKey) + "/" + result.ResolvedID,
				Name:             schemas.Ptr(model.Name),
				ContextLength:    schemas.Ptr(int(model.ContextLength)),
				SupportedMethods: model.Endpoints,
			}
			if result.AliasValue != "" {
				entry.Alias = schemas.Ptr(result.AliasValue)
			}
			unifaiResponse.Data = append(unifaiResponse.Data, entry)
			included[strings.ToLower(result.ResolvedID)] = true
		}
	}

	unifaiResponse.Data = append(unifaiResponse.Data,
		pipeline.BackfillModels(included)...)

	return unifaiResponse
}
