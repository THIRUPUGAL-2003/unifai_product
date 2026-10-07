package opencode

import (
	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
)

// CachedContentCreate is not supported by Opencode.
func (p *opencodeProvider) CachedContentCreate(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayCachedContentCreateRequest) (*schemas.GatewayCachedContentCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentCreateRequest, p.GetProviderKey())
}

// CachedContentList is not supported by Opencode.
func (p *opencodeProvider) CachedContentList(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayCachedContentListRequest) (*schemas.GatewayCachedContentListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentListRequest, p.GetProviderKey())
}

// CachedContentRetrieve is not supported by Opencode.
func (p *opencodeProvider) CachedContentRetrieve(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayCachedContentRetrieveRequest) (*schemas.GatewayCachedContentRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentRetrieveRequest, p.GetProviderKey())
}

// CachedContentUpdate is not supported by Opencode.
func (p *opencodeProvider) CachedContentUpdate(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayCachedContentUpdateRequest) (*schemas.GatewayCachedContentUpdateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentUpdateRequest, p.GetProviderKey())
}

// CachedContentDelete is not supported by Opencode.
func (p *opencodeProvider) CachedContentDelete(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayCachedContentDeleteRequest) (*schemas.GatewayCachedContentDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentDeleteRequest, p.GetProviderKey())
}
