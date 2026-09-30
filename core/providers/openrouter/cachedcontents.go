package openrouter

import (
	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
)

// CachedContentCreate is unsupported on OpenRouterProvider. Only Gemini and Vertex AI
// implement the cached-content lifecycle (Google AI Studio + Vertex AI named
// caches). Other providers either lack named cache management entirely or
// handle caching implicitly via per-message cache_control markers.
func (provider *OpenRouterProvider) CachedContentCreate(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAICachedContentCreateRequest) (*schemas.UnifAICachedContentCreateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentCreateRequest, provider.GetProviderKey())
}

// CachedContentList is unsupported on OpenRouterProvider (see CachedContentCreate).
func (provider *OpenRouterProvider) CachedContentList(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAICachedContentListRequest) (*schemas.UnifAICachedContentListResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentListRequest, provider.GetProviderKey())
}

// CachedContentRetrieve is unsupported on OpenRouterProvider (see CachedContentCreate).
func (provider *OpenRouterProvider) CachedContentRetrieve(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAICachedContentRetrieveRequest) (*schemas.UnifAICachedContentRetrieveResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentRetrieveRequest, provider.GetProviderKey())
}

// CachedContentUpdate is unsupported on OpenRouterProvider (see CachedContentCreate).
func (provider *OpenRouterProvider) CachedContentUpdate(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAICachedContentUpdateRequest) (*schemas.UnifAICachedContentUpdateResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentUpdateRequest, provider.GetProviderKey())
}

// CachedContentDelete is unsupported on OpenRouterProvider (see CachedContentCreate).
func (provider *OpenRouterProvider) CachedContentDelete(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAICachedContentDeleteRequest) (*schemas.UnifAICachedContentDeleteResponse, *schemas.UnifAIError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentDeleteRequest, provider.GetProviderKey())
}
