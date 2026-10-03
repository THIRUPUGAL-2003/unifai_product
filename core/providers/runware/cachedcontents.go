package runware

import (
	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"
)

// CachedContentCreate is unsupported on RunwareProvider. Only Gemini and Vertex AI
// support the named cache lifecycle.
func (provider *RunwareProvider) CachedContentCreate(ctx *schemas.RakshaContext, key schemas.Key, request *schemas.RakshaCachedContentCreateRequest) (*schemas.RakshaCachedContentCreateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentCreateRequest, provider.GetProviderKey())
}

// CachedContentList is unsupported on RunwareProvider (see CachedContentCreate).
func (provider *RunwareProvider) CachedContentList(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaCachedContentListRequest) (*schemas.RakshaCachedContentListResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentListRequest, provider.GetProviderKey())
}

// CachedContentRetrieve is unsupported on RunwareProvider (see CachedContentCreate).
func (provider *RunwareProvider) CachedContentRetrieve(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaCachedContentRetrieveRequest) (*schemas.RakshaCachedContentRetrieveResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentRetrieveRequest, provider.GetProviderKey())
}

// CachedContentUpdate is unsupported on RunwareProvider (see CachedContentCreate).
func (provider *RunwareProvider) CachedContentUpdate(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaCachedContentUpdateRequest) (*schemas.RakshaCachedContentUpdateResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentUpdateRequest, provider.GetProviderKey())
}

// CachedContentDelete is unsupported on RunwareProvider (see CachedContentCreate).
func (provider *RunwareProvider) CachedContentDelete(ctx *schemas.RakshaContext, keys []schemas.Key, request *schemas.RakshaCachedContentDeleteRequest) (*schemas.RakshaCachedContentDeleteResponse, *schemas.RakshaError) {
	return nil, providerUtils.NewUnsupportedOperationError(schemas.CachedContentDeleteRequest, provider.GetProviderKey())
}
