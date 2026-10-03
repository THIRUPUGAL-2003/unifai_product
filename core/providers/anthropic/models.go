package anthropic

import (
	"strings"
	"time"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
)

func (response *AnthropicListModelsResponse) ToRakshaListModelsResponse(providerKey schemas.ModelProvider, allowedModels schemas.WhiteList, blacklistedModels schemas.BlackList, aliases schemas.KeyAliases, unfiltered bool) *schemas.RakshaListModelsResponse {
	if response == nil {
		return nil
	}

	rakshaResponse := &schemas.RakshaListModelsResponse{
		Data:    make([]schemas.Model, 0, len(response.Data)),
		FirstID: response.FirstID,
		LastID:  response.LastID,
		HasMore: schemas.Ptr(response.HasMore),
	}

	// Map Anthropic's cursor-based pagination to Raksha's token-based pagination.
	// If there are more results, set next_page_token to last_id for the next request.
	if response.HasMore && response.LastID != nil {
		rakshaResponse.NextPageToken = *response.LastID
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
		return rakshaResponse
	}

	included := make(map[string]bool)

	for _, model := range response.Data {
		for _, result := range pipeline.FilterModel(model.ID) {
			resolvedKey := strings.ToLower(result.ResolvedID)
			if included[resolvedKey] {
				continue
			}
			entry := schemas.Model{
				ID:              string(providerKey) + "/" + result.ResolvedID,
				Name:            schemas.Ptr(model.DisplayName),
				Created:         schemas.Ptr(model.CreatedAt.Unix()),
				MaxInputTokens:  model.MaxInputTokens,
				MaxOutputTokens: model.MaxTokens,
				ProviderExtra:   model.Capabilities,
			}
			if result.AliasValue != "" {
				entry.Alias = schemas.Ptr(result.AliasValue)
			}
			rakshaResponse.Data = append(rakshaResponse.Data, entry)
			included[resolvedKey] = true
		}
	}

	rakshaResponse.Data = append(rakshaResponse.Data,
		pipeline.BackfillModels(included)...)

	return rakshaResponse
}

func ToAnthropicListModelsResponse(response *schemas.RakshaListModelsResponse) *AnthropicListModelsResponse {
	if response == nil {
		return nil
	}

	anthropicResponse := &AnthropicListModelsResponse{
		Data: make([]AnthropicModel, 0, len(response.Data)),
	}
	if response.FirstID != nil {
		anthropicResponse.FirstID = response.FirstID
	}
	if response.LastID != nil {
		anthropicResponse.LastID = response.LastID
	}
	if response.HasMore != nil {
		anthropicResponse.HasMore = *response.HasMore
	}

	for _, model := range response.Data {
		_, modelID := schemas.ParseModelString(model.ID, schemas.Anthropic)
		anthropicModel := AnthropicModel{
			ID:             modelID,
			Type:           "model",
			MaxInputTokens: model.MaxInputTokens,
			MaxTokens:      model.MaxOutputTokens,
			Capabilities:   model.ProviderExtra,
		}
		if model.Name != nil {
			anthropicModel.DisplayName = *model.Name
		}
		if model.Created != nil {
			anthropicModel.CreatedAt = time.Unix(*model.Created, 0)
		}
		anthropicResponse.Data = append(anthropicResponse.Data, anthropicModel)
	}

	return anthropicResponse
}
