package openai

import (
	"strings"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
)

// ToRakshaListModelsResponse converts an OpenAI list models response to a Raksha list models response
func (response *OpenAIListModelsResponse) ToRakshaListModelsResponse(providerKey schemas.ModelProvider, allowedModels schemas.WhiteList, blacklistedModels schemas.BlackList, aliases schemas.KeyAliases, unfiltered bool) *schemas.RakshaListModelsResponse {
	if response == nil {
		return nil
	}

	rakshaResponse := &schemas.RakshaListModelsResponse{
		Data: make([]schemas.Model, 0, len(response.Data)),
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
			ownedBy := model.OwnedBy
			if ownedBy == "" {
				ownedBy = model.Organization
			}
			contextLength := model.ContextWindow
			if contextLength == nil {
				contextLength = model.ContextLength
			}
			entry := schemas.Model{
				ID:            string(providerKey) + "/" + result.ResolvedID,
				Created:       model.Created,
				OwnedBy:       schemas.Ptr(ownedBy),
				ContextLength: contextLength,
			}
			if result.AliasValue != "" {
				entry.Alias = schemas.Ptr(result.AliasValue)
			}
			rakshaResponse.Data = append(rakshaResponse.Data, entry)
			included[strings.ToLower(result.ResolvedID)] = true
		}
	}

	rakshaResponse.Data = append(rakshaResponse.Data,
		pipeline.BackfillModels(included)...)

	return rakshaResponse
}

// ToOpenAIListModelsResponse converts a Raksha list models response to an OpenAI list models response
func ToOpenAIListModelsResponse(response *schemas.RakshaListModelsResponse) *OpenAIListModelsResponse {
	if response == nil {
		return nil
	}
	openaiResponse := &OpenAIListModelsResponse{
		Data: make([]OpenAIModel, 0, len(response.Data)),
	}
	for _, model := range response.Data {
		openaiModel := OpenAIModel{
			ID:     model.ID,
			Object: "model",
		}
		if model.Created != nil {
			openaiModel.Created = model.Created
		}
		if model.OwnedBy != nil {
			openaiModel.OwnedBy = *model.OwnedBy
		}
		if model.ContextLength != nil {
			openaiModel.ContextWindow = model.ContextLength
		} else if model.MaxInputTokens != nil {
			openaiModel.ContextWindow = model.MaxInputTokens // Fallback to MaxInputTokens if ContextLength is not set
		}

		openaiResponse.Data = append(openaiResponse.Data, openaiModel)

	}
	return openaiResponse
}
