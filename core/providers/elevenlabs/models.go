package elevenlabs

import (
	"strings"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
)

func (response *ElevenlabsListModelsResponse) ToUnifAIListModelsResponse(providerKey schemas.ModelProvider, allowedModels schemas.WhiteList, blacklistedModels schemas.BlackList, aliases schemas.KeyAliases, unfiltered bool) *schemas.UnifAIListModelsResponse {
	if response == nil {
		return nil
	}

	unifaiResponse := &schemas.UnifAIListModelsResponse{
		Data: make([]schemas.Model, 0, len(*response)),
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

	for _, model := range *response {
		for _, result := range pipeline.FilterModel(model.ModelID) {
			entry := schemas.Model{
				ID:   string(providerKey) + "/" + result.ResolvedID,
				Name: schemas.Ptr(model.Name),
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
