// Package governance provides utility functions for the governance plugin
package governance

import (
	"context"
	"fmt"
	"strings"

	unifai "github.com/unifai/unifai/core"
	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/configstore"
	configstoreTables "github.com/unifai/unifai/framework/configstore/tables"
	"github.com/valyala/fasthttp"
)

// ParseVirtualKeyFromFastHTTPRequest parses the virtual key from FastHTTP request headers.
// Parameters:
//   - req: The FastHTTP request containing headers to parse
//
// Returns:
//   - *string: The virtual key if found, nil otherwise
func ParseVirtualKeyFromFastHTTPRequest(req *fasthttp.RequestCtx) *string {
	vkHeader := string(req.Request.Header.Peek("x-uf-vk"))
	if vkHeader != "" && strings.HasPrefix(strings.ToLower(vkHeader), VirtualKeyPrefix) {
		return unifai.Ptr(vkHeader)
	}
	authHeader := string(req.Request.Header.Peek("Authorization"))
	if authHeader != "" {
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			authHeaderValue := strings.TrimSpace(authHeader[7:]) // Remove "Bearer " prefix
			if authHeaderValue != "" && strings.HasPrefix(strings.ToLower(authHeaderValue), VirtualKeyPrefix) {
				return unifai.Ptr(authHeaderValue)
			}
		}
	}
	xAPIKey := string(req.Request.Header.Peek("x-api-key"))
	if xAPIKey != "" && strings.HasPrefix(strings.ToLower(xAPIKey), VirtualKeyPrefix) {
		return unifai.Ptr(xAPIKey)
	}
	xGoogleAPIKey := string(req.Request.Header.Peek("x-goog-api-key"))
	if xGoogleAPIKey != "" && strings.HasPrefix(strings.ToLower(xGoogleAPIKey), VirtualKeyPrefix) {
		return unifai.Ptr(xGoogleAPIKey)
	}
	return nil
}

// IsModelRequiredForRequest checks if the requested model is required for this request
func IsModelRequiredForRequest(requestType schemas.RequestType) bool {
	// Here we will have to check for some requests which do not need model
	// For example, batches, container, files, videos, passthrough requests
	// For these requests, we will only check for provider filtering
	// Cached content list/retrieve/update/delete target a resource name (cachedContents/{id}),
	// not a model, so they carry no model to filter on; only create binds a cache to a model.
	if requestType == schemas.ListModelsRequest || requestType == schemas.MCPToolExecutionRequest || requestType == schemas.BatchCreateRequest || requestType == schemas.BatchListRequest || requestType == schemas.BatchRetrieveRequest || requestType == schemas.BatchCancelRequest || requestType == schemas.BatchResultsRequest || requestType == schemas.FileUploadRequest || requestType == schemas.FileListRequest || requestType == schemas.FileRetrieveRequest || requestType == schemas.FileDeleteRequest || requestType == schemas.FileContentRequest || requestType == schemas.ContainerCreateRequest || requestType == schemas.ContainerListRequest || requestType == schemas.ContainerRetrieveRequest || requestType == schemas.ContainerDeleteRequest || requestType == schemas.ContainerFileCreateRequest || requestType == schemas.ContainerFileListRequest || requestType == schemas.ContainerFileRetrieveRequest || requestType == schemas.ContainerFileContentRequest || requestType == schemas.ContainerFileDeleteRequest || requestType == schemas.CachedContentListRequest || requestType == schemas.CachedContentRetrieveRequest || requestType == schemas.CachedContentUpdateRequest || requestType == schemas.CachedContentDeleteRequest || requestType == schemas.VideoRetrieveRequest || requestType == schemas.VideoDownloadRequest || requestType == schemas.VideoListRequest || requestType == schemas.VideoDeleteRequest || requestType == schemas.VideoRemixRequest || requestType == schemas.PassthroughRequest || requestType == schemas.PassthroughStreamRequest {
		return false
	}
	return true
}

// getWeight safely dereferences a *float64 weight pointer, returning 1.0 as default if nil.
// This allows distinguishing between "not set" (nil -> 1.0) and "explicitly set to 0" (0.0).
func getWeight(w *float64) float64 {
	if w == nil {
		return 1.0
	}
	return *w
}

// stampGovernanceCtxFromVK copies team/customer identifiers from the VK onto ctx so
// downstream plugins (logging, observability) see the governance scope.
func stampGovernanceCtxFromVK(ctx *schemas.UnifAIContext, vk *configstoreTables.TableVirtualKey) {
	if vk == nil {
		return
	}
	if vk.TeamID != nil {
		ctx.SetValue(schemas.UnifAIContextKeyGovernanceTeamID, *vk.TeamID)
	}
	if vk.Team != nil {
		ctx.SetValue(schemas.UnifAIContextKeyGovernanceTeamName, vk.Team.Name)
		if vk.Team.CustomerID != nil {
			ctx.SetValue(schemas.UnifAIContextKeyGovernanceCustomerID, *vk.Team.CustomerID)
			if vk.Team.Customer != nil {
				ctx.SetValue(schemas.UnifAIContextKeyGovernanceCustomerName, vk.Team.Customer.Name)
			}
		}
	} else {
		if vk.CustomerID != nil {
			ctx.SetValue(schemas.UnifAIContextKeyGovernanceCustomerID, *vk.CustomerID)
		}
		if vk.Customer != nil {
			ctx.SetValue(schemas.UnifAIContextKeyGovernanceCustomerName, vk.Customer.Name)
		}
	}
}

func teamIDFromVK(vk *configstoreTables.TableVirtualKey) string {
	if vk == nil {
		return ""
	}
	if vk.TeamID != nil && *vk.TeamID != "" {
		return *vk.TeamID
	}
	if vk.Team != nil {
		return vk.Team.ID
	}
	return ""
}

// stampGovernanceCtx stamps team/customer/BU identity from a VK onto the request context,
// then resolves the assigned user (if missing) and merges team membership for rankings.
func (p *GovernancePlugin) stampGovernanceCtx(ctx *schemas.UnifAIContext, vk *configstoreTables.TableVirtualKey) {
	stampGovernanceCtxFromVK(ctx, vk)
	if local, ok := p.store.(*LocalGovernanceStore); ok {
		local.stampBusinessUnitsForTeam(ctx, teamIDFromVK(vk))
	}
	p.stampUserFromVKAssignment(ctx, vk)
	p.stampUserOrgMembership(ctx)
}

// stampUserFromVKAssignment sets user_id/user_name from governance_virtual_key_users when
// the request has a VK but no session/header user — so User rankings are never empty for assigned VKs.
func (p *GovernancePlugin) stampUserFromVKAssignment(ctx *schemas.UnifAIContext, vk *configstoreTables.TableVirtualKey) {
	if ctx == nil || vk == nil || p.configStore == nil {
		return
	}
	if unifai.GetStringFromContext(ctx, schemas.UnifAIContextKeyUserID) != "" {
		return
	}
	ws, ok := configstore.AsWorkspaceStore(p.configStore)
	if !ok || ws == nil {
		return
	}
	links, err := ws.ListVirtualKeyUsers(ctx, vk.ID)
	if err != nil || len(links) == 0 {
		return
	}
	uid := links[0].UserID
	if uid == "" {
		return
	}
	ctx.SetValue(schemas.UnifAIContextKeyUserID, uid)
	if user, gerr := p.configStore.GetUserByID(ctx, uid); gerr == nil && user != nil && user.Username != "" {
		ctx.SetValue(schemas.UnifAIContextKeyUserName, user.Username)
	}
}

// stampUserOrgMembership loads governance_team_members for the request user and stamps
// multi-team / multi-BU context keys used by LLM rankings and observability filters.
func (p *GovernancePlugin) stampUserOrgMembership(ctx *schemas.UnifAIContext) {
	if ctx == nil || p.configStore == nil {
		return
	}
	userID := unifai.GetStringFromContext(ctx, schemas.UnifAIContextKeyUserID)
	if userID == "" {
		return
	}
	ws, ok := configstore.AsWorkspaceStore(p.configStore)
	if !ok || ws == nil {
		return
	}
	links, err := ws.ListTeamsForUser(ctx, userID)
	if err != nil || len(links) == 0 {
		return
	}

	teamIDs := make([]string, 0, len(links)+1)
	teamNames := make([]string, 0, len(links)+1)
	seen := map[string]bool{}

	// Keep VK primary team first when already stamped.
	if primary := unifai.GetStringFromContext(ctx, schemas.UnifAIContextKeyGovernanceTeamID); primary != "" {
		teamIDs = append(teamIDs, primary)
		seen[primary] = true
		if name := unifai.GetStringFromContext(ctx, schemas.UnifAIContextKeyGovernanceTeamName); name != "" {
			teamNames = append(teamNames, name)
		} else {
			teamNames = append(teamNames, primary)
		}
	}

	local, _ := p.store.(*LocalGovernanceStore)
	for _, link := range links {
		if link.TeamID == "" || seen[link.TeamID] {
			continue
		}
		seen[link.TeamID] = true
		name := link.TeamID
		if local != nil {
			if v, ok := local.teams.Load(link.TeamID); ok {
				if t, ok := v.(*configstoreTables.TableTeam); ok && t != nil && t.Name != "" {
					name = t.Name
				}
			}
		}
		if name == link.TeamID {
			if team, gerr := p.configStore.GetTeam(ctx, link.TeamID); gerr == nil && team != nil && team.Name != "" {
				name = team.Name
			}
		}
		teamIDs = append(teamIDs, link.TeamID)
		teamNames = append(teamNames, name)
	}
	if len(teamIDs) == 0 {
		return
	}

	ctx.SetValue(schemas.UnifAIContextKeyGovernanceTeamIDs, teamIDs)
	ctx.SetValue(schemas.UnifAIContextKeyGovernanceTeamNames, teamNames)

	// If no VK team was stamped, use the user's first membership team as primary.
	if unifai.GetStringFromContext(ctx, schemas.UnifAIContextKeyGovernanceTeamID) == "" {
		ctx.SetValue(schemas.UnifAIContextKeyGovernanceTeamID, teamIDs[0])
		ctx.SetValue(schemas.UnifAIContextKeyGovernanceTeamName, teamNames[0])
	}

	if local != nil {
		local.stampBusinessUnitsForTeams(ctx, teamIDs)
	}
}

// filterModelsForVirtualKey filters models based on virtual key's provider configs
// Returns only models that are allowed by the virtual key's ProviderConfigs
func (p *GovernancePlugin) filterModelsForVirtualKey(
	ctx context.Context,
	models []schemas.Model,
	virtualKeyValue string,
) []schemas.Model {
	// Get virtual key configuration
	vk, exists := p.store.GetVirtualKey(ctx, virtualKeyValue)
	if !exists {
		p.logger.Warn("[Governance] Virtual key not found for list models filtering: %s", virtualKeyValue)
		return []schemas.Model{} // VK not found, return empty list
	}

	// Empty ProviderConfigs means no models are allowed (deny-by-default)
	if len(vk.ProviderConfigs) == 0 {
		return []schemas.Model{}
	}

	// Filter models based on ProviderConfigs
	filteredModels := make([]schemas.Model, 0, len(models))
	for _, model := range models {
		provider, modelName := schemas.ParseModelString(model.ID, "")

		// Pre-pass: if any matching config blacklists the model, block it entirely.
		isBlocked := false
		for _, pc := range vk.ProviderConfigs {
			if pc.Provider == string(provider) && pc.BlacklistedModels.IsBlocked(modelName) {
				isBlocked = true
				break
			}
		}
		if isBlocked {
			continue
		}

		// Allowlist check — model is allowed if any matching config permits it.
		isAllowed := false
		for _, pc := range vk.ProviderConfigs {
			if pc.Provider == string(provider) {
				if p.modelCatalog != nil && p.inMemoryStore != nil {
					providerConfig, ok := p.inMemoryStore.GetConfiguredProviders()[provider]
					providerConfigPtr := &providerConfig
					if !ok {
						providerConfigPtr = nil
					}
					if p.modelCatalog.IsModelAllowedForProvider(provider, modelName, providerConfigPtr, pc.AllowedModels) {
						isAllowed = true
						break
					}
				} else {
					if pc.AllowedModels.IsAllowed(modelName) {
						isAllowed = true
						break
					}
				}
			}
		}

		if isAllowed {
			filteredModels = append(filteredModels, model)
		}
	}

	return filteredModels
}

// validateRequiredHeaders checks that all configured required headers are present in the request.
// Headers are compared case-insensitively (both sides lowercased).
// Returns a UnifAIError with status 400 if any required headers are missing, or nil if all present.
func (p *GovernancePlugin) validateRequiredHeaders(ctx *schemas.UnifAIContext) *schemas.UnifAIError {
	if p.requiredHeaders == nil || len(*p.requiredHeaders) == 0 {
		return nil
	}
	headers, _ := ctx.Value(schemas.UnifAIContextKeyRequestHeaders).(map[string]string)
	if headers == nil {
		headers = map[string]string{}
	}
	var missing []string
	for _, h := range *p.requiredHeaders {
		if _, ok := headers[strings.ToLower(h)]; !ok {
			missing = append(missing, h)
		}
	}
	if len(missing) > 0 {
		return &schemas.UnifAIError{
			Type:       unifai.Ptr("missing_required_headers"),
			StatusCode: unifai.Ptr(400),
			Error: &schemas.ErrorField{
				Message: fmt.Sprintf("missing required headers: %s", strings.Join(missing, ", ")),
			},
		}
	}
	return nil
}
