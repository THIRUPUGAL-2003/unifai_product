package openai

import (
	"strings"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
)

// ToGatewayListModelsResponse converts an OpenAI list models response to a Gateway list models response
func (response *OpenAIListModelsResponse) ToGatewayListModelsResponse(providerKey schemas.ModelProvider, allowedModels schemas.WhiteList, blacklistedModels schemas.BlackList, aliases schemas.KeyAliases, unfiltered bool) *schemas.GatewayListModelsResponse {
	if response == nil {
		return nil
	}

	gatewayResponse := &schemas.GatewayListModelsResponse{
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
		return gatewayResponse
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
			gatewayResponse.Data = append(gatewayResponse.Data, entry)
			included[strings.ToLower(result.ResolvedID)] = true
		}
	}

	gatewayResponse.Data = append(gatewayResponse.Data,
		pipeline.BackfillModels(included)...)

	return gatewayResponse
}

// ToOpenAIListModelsResponse converts a Gateway list models response to an OpenAI list models response
func ToOpenAIListModelsResponse(response *schemas.GatewayListModelsResponse) *OpenAIListModelsResponse {
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
