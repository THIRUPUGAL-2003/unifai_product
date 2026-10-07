package replicate

import (
	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
)

// CachedContentCreate is unsupported on ReplicateProvider. Only Gemini and Vertex AI
// implement the cached-content lifecycle (Google AI Studio + Vertex AI named
// caches). Other providers either lack named cache management entirely or
// handle caching implicitly via per-message cache_control markers.
func (provider *ReplicateProvider) CachedContentCreate(ctx *schemas.GatewayContext, key schemas.Key, request *schemas.GatewayCachedContentCreateRequest) (*schemas.GatewayCachedContentCreateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentCreateRequest, provider.GetProviderKey())
}

// CachedContentList is unsupported on ReplicateProvider (see CachedContentCreate).
func (provider *ReplicateProvider) CachedContentList(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayCachedContentListRequest) (*schemas.GatewayCachedContentListResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentListRequest, provider.GetProviderKey())
}

// CachedContentRetrieve is unsupported on ReplicateProvider (see CachedContentCreate).
func (provider *ReplicateProvider) CachedContentRetrieve(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayCachedContentRetrieveRequest) (*schemas.GatewayCachedContentRetrieveResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentRetrieveRequest, provider.GetProviderKey())
}

// CachedContentUpdate is unsupported on ReplicateProvider (see CachedContentCreate).
func (provider *ReplicateProvider) CachedContentUpdate(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayCachedContentUpdateRequest) (*schemas.GatewayCachedContentUpdateResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentUpdateRequest, provider.GetProviderKey())
}

// CachedContentDelete is unsupported on ReplicateProvider (see CachedContentCreate).
func (provider *ReplicateProvider) CachedContentDelete(ctx *schemas.GatewayContext, keys []schemas.Key, request *schemas.GatewayCachedContentDeleteRequest) (*schemas.GatewayCachedContentDeleteResponse, *schemas.GatewayError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentDeleteRequest, provider.GetProviderKey())
}
