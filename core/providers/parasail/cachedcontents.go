package parasail

import (
	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
)

// CachedContentCreate is unsupported on ParasailProvider. Only Gemini and Vertex AI
// implement the cached-content lifecycle (Google AI Studio + Vertex AI named
// caches). Other providers either lack named cache management entirely or
// handle caching implicitly via per-message cache_control markers.
func (provider *ParasailProvider) CachedContentCreate(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaCachedContentCreateRequest) (*schemas.RakshaCachedContentCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentCreateRequest, provider.GetProviderKey())
}

// CachedContentList is unsupported on ParasailProvider (see CachedContentCreate).
func (provider *ParasailProvider) CachedContentList(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaCachedContentListRequest) (*schemas.RakshaCachedContentListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentListRequest, provider.GetProviderKey())
}

// CachedContentRetrieve is unsupported on ParasailProvider (see CachedContentCreate).
func (provider *ParasailProvider) CachedContentRetrieve(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaCachedContentRetrieveRequest) (*schemas.RakshaCachedContentRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentRetrieveRequest, provider.GetProviderKey())
}

// CachedContentUpdate is unsupported on ParasailProvider (see CachedContentCreate).
func (provider *ParasailProvider) CachedContentUpdate(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaCachedContentUpdateRequest) (*schemas.RakshaCachedContentUpdateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentUpdateRequest, provider.GetProviderKey())
}

// CachedContentDelete is unsupported on ParasailProvider (see CachedContentCreate).
func (provider *ParasailProvider) CachedContentDelete(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaCachedContentDeleteRequest) (*schemas.RakshaCachedContentDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentDeleteRequest, provider.GetProviderKey())
}
