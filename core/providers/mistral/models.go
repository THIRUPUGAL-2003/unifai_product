package mistral

import (
	"strings"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
)

func (response *MistralListModelsResponse) ToRakshaListModelsResponse(allowedModels schemas.WhiteList, blacklistedModels schemas.BlackList, aliases schemas.KeyAliases, unfiltered bool) *schemas.RakshaListModelsResponse {
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
		ProviderKey:       schemas.Mistral,
		MatchFns:          providerUtils.DefaultMatchFns(),
	}
	if pipeline.ShouldEarlyExit() {
		return rakshaResponse
	}

	included := make(map[string]bool)

	for _, model := range response.Data {
		for _, result := range pipeline.FilterModel(model.ID) {
			entry := schemas.Model{
				ID:            string(schemas.Mistral) + "/" + result.ResolvedID,
				Name:          schemas.Ptr(model.Name),
				Description:   schemas.Ptr(model.Description),
				Created:       schemas.Ptr(model.Created),
				ContextLength: schemas.Ptr(int(model.MaxContextLength)),
				OwnedBy:       schemas.Ptr(model.OwnedBy),
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
