package openaicompat

import (
	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
)

func (p *Provider) CachedContentCreate(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayCachedContentCreateRequest) (*schemas.GatewayCachedContentCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentCreateRequest, p.GetProviderKey())
}
func (p *Provider) CachedContentList(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayCachedContentListRequest) (*schemas.GatewayCachedContentListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentListRequest, p.GetProviderKey())
}
func (p *Provider) CachedContentRetrieve(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayCachedContentRetrieveRequest) (*schemas.GatewayCachedContentRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentRetrieveRequest, p.GetProviderKey())
}
func (p *Provider) CachedContentUpdate(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayCachedContentUpdateRequest) (*schemas.GatewayCachedContentUpdateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentUpdateRequest, p.GetProviderKey())
}
func (p *Provider) CachedContentDelete(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayCachedContentDeleteRequest) (*schemas.GatewayCachedContentDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentDeleteRequest, p.GetProviderKey())
}
