package azure

import (
	"strings"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
)

func (response *AzureListModelsResponse) ToUnifAIListModelsResponse(allowedModels schemas.WhiteList, blacklistedModels schemas.BlackList, aliases schemas.KeyAliases, unfiltered bool) *schemas.UnifAIListModelsResponse {
	if response == nil {
		return nil
	}

	unifaiResponse := &schemas.UnifAIListModelsResponse{
		Data: make([]schemas.Model, 0, len(response.Data)),
	}

	pipeline := &providerUtils.ListModelsPipeline{
		AllowedModels:     allowedModels,
		BlacklistedModels: blacklistedModels,
		Aliases:           aliases,
		Unfiltered:        unfiltered,
		ProviderKey:       schemas.Azure,
		MatchFns:          providerUtils.DefaultMatchFns(),
	}
	if pipeline.ShouldEarlyExit() {
		return unifaiResponse
	}

	included := make(map[string]bool)

	for _, model := range response.Data {
		for _, result := range pipeline.FilterModel(model.ID) {
			entry := schemas.Model{
				ID:      string(schemas.Azure) + "/" + result.ResolvedID,
				Created: schemas.Ptr(model.CreatedAt),
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
