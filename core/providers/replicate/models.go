package replicate

import (
	"strings"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
)

// ToUnifAIListModelsResponse converts Replicate deployments to a UnifAI list models response.
// Replicate model IDs are composite: "{owner}/{name}" (e.g. "stability-ai/stable-diffusion").
func ToUnifAIListModelsResponse(
	deploymentsResponse *ReplicateDeploymentListResponse,
	providerKey schemas.ModelProvider,
	allowedModels schemas.WhiteList,
	blacklistedModels schemas.BlackList,
	aliases schemas.KeyAliases,
	unfiltered bool,
) *schemas.UnifAIListModelsResponse {
	unifaiResponse := &schemas.UnifAIListModelsResponse{
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
		return unifaiResponse
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
				unifaiModel := schemas.Model{
					ID:      string(providerKey) + "/" + result.ResolvedID,
					Name:    schemas.Ptr(deployment.Name),
					OwnedBy: schemas.Ptr(deployment.Owner),
					Created: created,
				}
				if result.AliasValue != "" {
					unifaiModel.Alias = schemas.Ptr(result.AliasValue)
				}
				unifaiResponse.Data = append(unifaiResponse.Data, unifaiModel)
				included[strings.ToLower(result.ResolvedID)] = true
			}
		}

		if deploymentsResponse.Next != nil {
			unifaiResponse.NextPageToken = *deploymentsResponse.Next
		}
	}

	unifaiResponse.Data = append(unifaiResponse.Data,
		pipeline.BackfillModels(included)...)

	return unifaiResponse
}
