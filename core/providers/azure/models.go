package azure

import (
	"strings"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
)

func (response *AzureListModelsResponse) ToGatewayListModelsResponse(allowedModels schemas.WhiteList, blacklistedModels schemas.BlackList, aliases schemas.KeyAliases, unfiltered bool) *schemas.GatewayListModelsResponse {
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
		ProviderKey:       schemas.Azure,
		MatchFns:          providerUtils.DefaultMatchFns(),
	}
	if pipeline.ShouldEarlyExit() {
		return gatewayResponse
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
			gatewayResponse.Data = append(gatewayResponse.Data, entry)
			included[strings.ToLower(result.ResolvedID)] = true
		}
	}

	gatewayResponse.Data = append(gatewayResponse.Data,
		pipeline.BackfillModels(included)...)

	return gatewayResponse
}
