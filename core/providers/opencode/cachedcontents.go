package opencode

import (
	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
)

// CachedContentCreate is not supported by Opencode.
func (p *opencodeProvider) CachedContentCreate(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAICachedContentCreateRequest) (*schemas.UnifAICachedContentCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentCreateRequest, p.GetProviderKey())
}

// CachedContentList is not supported by Opencode.
func (p *opencodeProvider) CachedContentList(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAICachedContentListRequest) (*schemas.UnifAICachedContentListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentListRequest, p.GetProviderKey())
}

// CachedContentRetrieve is not supported by Opencode.
func (p *opencodeProvider) CachedContentRetrieve(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAICachedContentRetrieveRequest) (*schemas.UnifAICachedContentRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentRetrieveRequest, p.GetProviderKey())
}

// CachedContentUpdate is not supported by Opencode.
func (p *opencodeProvider) CachedContentUpdate(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAICachedContentUpdateRequest) (*schemas.UnifAICachedContentUpdateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentUpdateRequest, p.GetProviderKey())
}

// CachedContentDelete is not supported by Opencode.
func (p *opencodeProvider) CachedContentDelete(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAICachedContentDeleteRequest) (*schemas.UnifAICachedContentDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentDeleteRequest, p.GetProviderKey())
}
