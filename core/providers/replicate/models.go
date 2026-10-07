package replicate

import (
	"strings"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
)

// ToGatewayListModelsResponse converts Replicate deployments to a Gateway list models response.
// Replicate model IDs are composite: "{owner}/{name}" (e.g. "stability-ai/stable-diffusion").
func ToGatewayListModelsResponse(
	deploymentsResponse *ReplicateDeploymentListResponse,
	providerKey schemas.ModelProvider,
	allowedModels schemas.WhiteList,
	blacklistedModels schemas.BlackList,
	aliases schemas.KeyAliases,
	unfiltered bool,
) *schemas.GatewayListModelsResponse {
	gatewayResponse := &schemas.GatewayListModelsResponse{
		Data: make([]schemas.Model, 0),
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

	if deploymentsResponse != nil {
		for _, deployment := range deploymentsResponse.Results {
			// Replicate model IDs are composite owner/name
			deploymentID := deployment.Owner + "/" + deployment.Name

			var created *int64
			if deployment.CurrentRelease != nil && deployment.CurrentRelease.CreatedAt != "" {
				createdTimestamp := ParseReplicateTimestamp(deployment.CurrentRelease.CreatedAt)
				if createdTimestamp > 0 {
					created = schemas.Ptr(createdTimestamp)
				}
			}

			for _, result := range pipeline.FilterModel(deploymentID) {
				gatewayModel := schemas.Model{
					ID:      string(providerKey) + "/" + result.ResolvedID,
					Name:    schemas.Ptr(deployment.Name),
					OwnedBy: schemas.Ptr(deployment.Owner),
					Created: created,
				}
				if result.AliasValue != "" {
					gatewayModel.Alias = schemas.Ptr(result.AliasValue)
				}
				gatewayResponse.Data = append(gatewayResponse.Data, gatewayModel)
				included[strings.ToLower(result.ResolvedID)] = true
			}
		}

		if deploymentsResponse.Next != nil {
			gatewayResponse.NextPageToken = *deploymentsResponse.Next
		}
	}

	gatewayResponse.Data = append(gatewayResponse.Data,
		pipeline.BackfillModels(included)...)

	return gatewayResponse
}
