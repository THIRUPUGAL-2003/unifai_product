package gemini

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/tidwall/sjson"
	"github.com/valyala/fasthttp"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
)

// geminiCachedContent mirrors the Gemini API CachedContent resource shape
// for both request bodies (create) and response parsing.
//
// API ref: https://ai.google.dev/api/caching#CachedContent
type geminiCachedContent struct {
	Name              string         `json:"name,omitempty"`
	DisplayName       string         `json:"displayName,omitempty"`
	Model             string         `json:"model,omitempty"`
	SystemInstruction any            `json:"systemInstruction,omitempty"`
	Contents          []any          `json:"contents,omitempty"`
	Tools             []any          `json:"tools,omitempty"`
	ToolConfig        any            `json:"toolConfig,omitempty"`
	CreateTime        string         `json:"createTime,omitempty"`
	UpdateTime        string         `json:"updateTime,omitempty"`
	ExpireTime        string         `json:"expireTime,omitempty"`
	TTL               string         `json:"ttl,omitempty"`
	UsageMetadata     map[string]any `json:"usageMetadata,omitempty"`
}

type geminiCachedContentList struct {
	CachedContents []geminiCachedContent `json:"cachedContents"`
	NextPageToken  string                `json:"nextPageToken,omitempty"`
}

func (g *geminiCachedContent) toUnifAIObject() schemas.CachedContentObject {
	return schemas.CachedContentObject{
		Name:              g.Name,
		DisplayName:       g.DisplayName,
		Model:             g.Model,
		SystemInstruction: g.SystemInstruction,
		Contents:          g.Contents,
		Tools:             g.Tools,
		ToolConfig:        g.ToolConfig,
		CreateTime:        g.CreateTime,
		UpdateTime:        g.UpdateTime,
		ExpireTime:        g.ExpireTime,
		UsageMetadata:     g.UsageMetadata,
	}
}

// cachedContentObjectToWire builds the Gemini camelCase wire shape from a
// shared CachedContentObject. Used by the response converters below to render
// upstream-compatible JSON for native Gemini SDK clients.
func cachedContentObjectToWire(obj schemas.CachedContentObject) geminiCachedContent {
	return geminiCachedContent{
		Name:              obj.Name,
		DisplayName:       obj.DisplayName,
		Model:             obj.Model,
		SystemInstruction: obj.SystemInstruction,
		Contents:          obj.Contents,
		Tools:             obj.Tools,
		ToolConfig:        obj.ToolConfig,
		CreateTime:        obj.CreateTime,
		UpdateTime:        obj.UpdateTime,
		ExpireTime:        obj.ExpireTime,
		UsageMetadata:     obj.UsageMetadata,
	}
}

// ToGeminiCachedContentCreateResponse renders a UnifAI create response as the
// Gemini camelCase wire shape (https://ai.google.dev/api/caching#CachedContent).
func ToGeminiCachedContentCreateResponse(resp *schemas.UnifAICachedContentCreateResponse) interface{} {
	if resp == nil {
		return nil
	}
	return geminiCachedContent{
		Name:              resp.Name,
		DisplayName:       resp.DisplayName,
		Model:             resp.Model,
		SystemInstruction: resp.SystemInstruction,
		Contents:          resp.Contents,
		Tools:             resp.Tools,
		ToolConfig:        resp.ToolConfig,
		CreateTime:        resp.CreateTime,
		UpdateTime:        resp.UpdateTime,
		ExpireTime:        resp.ExpireTime,
		UsageMetadata:     resp.UsageMetadata,
	}
}

// ToGeminiCachedContentListResponse renders a UnifAI list response as the
// Gemini wire shape (cachedContents/nextPageToken).
func ToGeminiCachedContentListResponse(resp *schemas.UnifAICachedContentListResponse) interface{} {
	if resp == nil {
		return nil
	}
	wire := geminiCachedContentList{
		CachedContents: []geminiCachedContent{},
		NextPageToken:  resp.NextPageToken,
	}
	if len(resp.CachedContents) > 0 {
		wire.CachedContents = make([]geminiCachedContent, len(resp.CachedContents))
		for i, obj := range resp.CachedContents {
			wire.CachedContents[i] = cachedContentObjectToWire(obj)
		}
	}
	return wire
}

// ToGeminiCachedContentRetrieveResponse renders a UnifAI retrieve response as
// the Gemini camelCase wire shape.
func ToGeminiCachedContentRetrieveResponse(resp *schemas.UnifAICachedContentRetrieveResponse) interface{} {
	if resp == nil {
		return nil
	}
	return geminiCachedContent{
		Name:              resp.Name,
		DisplayName:       resp.DisplayName,
		Model:             resp.Model,
		SystemInstruction: resp.SystemInstruction,
		Contents:          resp.Contents,
		Tools:             resp.Tools,
		ToolConfig:        resp.ToolConfig,
		CreateTime:        resp.CreateTime,
		UpdateTime:        resp.UpdateTime,
		ExpireTime:        resp.ExpireTime,
		UsageMetadata:     resp.UsageMetadata,
	}
}

// ToGeminiCachedContentUpdateResponse renders a UnifAI update response as the
// Gemini camelCase wire shape.
func ToGeminiCachedContentUpdateResponse(resp *schemas.UnifAICachedContentUpdateResponse) interface{} {
	if resp == nil {
		return nil
	}
	return geminiCachedContent{
		Name:              resp.Name,
		DisplayName:       resp.DisplayName,
		Model:             resp.Model,
		SystemInstruction: resp.SystemInstruction,
		Contents:          resp.Contents,
		Tools:             resp.Tools,
		ToolConfig:        resp.ToolConfig,
		CreateTime:        resp.CreateTime,
		UpdateTime:        resp.UpdateTime,
		ExpireTime:        resp.ExpireTime,
		UsageMetadata:     resp.UsageMetadata,
	}
}

// ToGeminiCachedContentDeleteResponse renders a UnifAI delete response. Gemini
// returns an empty body on success; mirror that with an empty struct so the
// payload is serialized as `{}` rather than the unifai-internal shape.
func ToGeminiCachedContentDeleteResponse(_ *schemas.UnifAICachedContentDeleteResponse) interface{} {
	return struct{}{}
}

func validateTTLExpireMutex(ttl, expireTime *string) *schemas.UnifAIError {
	if ttl != nil && *ttl != "" && expireTime != nil && *expireTime != "" {
		return providerUtils.NewUnifAIOperationError("ttl and expire_time are mutually exclusive", nil)
	}
	return nil
}

func normalizeCachedContentName(name string) string {
	if strings.HasPrefix(name, "cachedContents/") {
		return name
	}
	return "cachedContents/" + name
}

// CachedContentCreate creates a new cached content via Google AI Studio's
// /v1beta/cachedContents endpoint.
func (provider *GeminiProvider) CachedContentCreate(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAICachedContentCreateRequest) (*schemas.UnifAICachedContentCreateResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.CachedContentCreateRequest); err != nil {
		return nil, err
	}
	if err := validateTTLExpireMutex(request.TTL, request.ExpireTime); err != nil {
		return nil, err
	}
	if request.Model == "" {
		return nil, providerUtils.NewUnifAIOperationError("model is required for cached content create", nil)
	}

	model := request.Model
	if !strings.HasPrefix(model, "models/") {
		model = "models/" + model
	}

	jsonBody, useRaw := providerUtils.CheckAndGetRawRequestBody(ctx, request)
	if useRaw && len(jsonBody) > 0 {
		var err error
		jsonBody, err = sjson.SetBytes(jsonBody, "model", model)
		if err != nil {
			return nil, providerUtils.NewUnifAIOperationError("failed to set cached content model", err)
		}
	} else {
		body := geminiCachedContent{
			Model:             model,
			SystemInstruction: request.SystemInstruction,
			Contents:          request.Contents,
			Tools:             request.Tools,
			ToolConfig:        request.ToolConfig,
		}
		if request.DisplayName != nil {
			body.DisplayName = *request.DisplayName
		}
		if request.TTL != nil {
			body.TTL = *request.TTL
		}
		if request.ExpireTime != nil {
			body.ExpireTime = *request.ExpireTime
		}

		var err error
		jsonBody, err = sonic.Marshal(body)
		if err != nil {
			return nil, providerUtils.NewUnifAIOperationError("failed to marshal cached content create body", err)
		}
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	requestURL := fmt.Sprintf("%s/cachedContents", provider.networkConfig.BaseURL)
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}
	req.SetBody(jsonBody)

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, unifaiErr
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	respBody, decErr := providerUtils.CheckAndDecodeBody(resp)
	if decErr != nil {
		return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, decErr)
	}

	var geminiResp geminiCachedContent
	if err := sonic.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	return &schemas.UnifAICachedContentCreateResponse{
		Name:              geminiResp.Name,
		DisplayName:       geminiResp.DisplayName,
		Model:             geminiResp.Model,
		SystemInstruction: geminiResp.SystemInstruction,
		Contents:          geminiResp.Contents,
		Tools:             geminiResp.Tools,
		ToolConfig:        geminiResp.ToolConfig,
		CreateTime:        geminiResp.CreateTime,
		UpdateTime:        geminiResp.UpdateTime,
		ExpireTime:        geminiResp.ExpireTime,
		UsageMetadata:     geminiResp.UsageMetadata,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			Latency: latency.Milliseconds(),
		},
	}, nil
}

// cachedContentListByKey lists cached contents for a single key.
func (provider *GeminiProvider) cachedContentListByKey(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAICachedContentListRequest) (*schemas.UnifAICachedContentListResponse, time.Duration, *schemas.UnifAIError) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	requestURL := fmt.Sprintf("%s/cachedContents", provider.networkConfig.BaseURL)
	queryArgs := url.Values{}
	if request.PageSize > 0 {
		queryArgs.Set("pageSize", strconv.Itoa(request.PageSize))
	}
	if request.PageToken != nil && *request.PageToken != "" {
		queryArgs.Set("pageToken", *request.PageToken)
	}
	if len(queryArgs) > 0 {
		requestURL += "?" + queryArgs.Encode()
	}

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodGet)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, latency, unifaiErr
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, latency, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	respBody, decErr := providerUtils.CheckAndDecodeBody(resp)
	if decErr != nil {
		return nil, latency, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, decErr)
	}

	var geminiList geminiCachedContentList
	if err := sonic.Unmarshal(respBody, &geminiList); err != nil {
		return nil, latency, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	unifaiObjects := make([]schemas.CachedContentObject, 0, len(geminiList.CachedContents))
	for i := range geminiList.CachedContents {
		unifaiObjects = append(unifaiObjects, geminiList.CachedContents[i].toUnifAIObject())
	}

	return &schemas.UnifAICachedContentListResponse{
		CachedContents: unifaiObjects,
		NextPageToken:  geminiList.NextPageToken,
	}, latency, nil
}

// CachedContentList lists cached contents, trying each key until successful.
func (provider *GeminiProvider) CachedContentList(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAICachedContentListRequest) (*schemas.UnifAICachedContentListResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.CachedContentListRequest); err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, providerUtils.NewUnifAIOperationError("no keys provided for cached content list", nil)
	}

	var lastErr *schemas.UnifAIError
	for _, key := range keys {
		resp, latency, unifaiErr := provider.cachedContentListByKey(ctx, key, request)
		if unifaiErr == nil {
			resp.ExtraFields = schemas.UnifAIResponseExtraFields{Latency: latency.Milliseconds()}
			return resp, nil
		}
		lastErr = unifaiErr
	}
	return nil, lastErr
}

// cachedContentRetrieveByKey retrieves a single cached content for one key.
func (provider *GeminiProvider) cachedContentRetrieveByKey(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAICachedContentRetrieveRequest) (*schemas.UnifAICachedContentRetrieveResponse, time.Duration, *schemas.UnifAIError) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	name := normalizeCachedContentName(request.Name)
	requestURL := fmt.Sprintf("%s/%s", provider.networkConfig.BaseURL, name)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodGet)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, latency, unifaiErr
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, latency, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	respBody, decErr := providerUtils.CheckAndDecodeBody(resp)
	if decErr != nil {
		return nil, latency, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, decErr)
	}

	var geminiResp geminiCachedContent
	if err := sonic.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, latency, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	return &schemas.UnifAICachedContentRetrieveResponse{
		Name:              geminiResp.Name,
		DisplayName:       geminiResp.DisplayName,
		Model:             geminiResp.Model,
		SystemInstruction: geminiResp.SystemInstruction,
		Contents:          geminiResp.Contents,
		Tools:             geminiResp.Tools,
		ToolConfig:        geminiResp.ToolConfig,
		CreateTime:        geminiResp.CreateTime,
		UpdateTime:        geminiResp.UpdateTime,
		ExpireTime:        geminiResp.ExpireTime,
		UsageMetadata:     geminiResp.UsageMetadata,
	}, latency, nil
}

// CachedContentRetrieve retrieves a cached content by name, trying each key.
func (provider *GeminiProvider) CachedContentRetrieve(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAICachedContentRetrieveRequest) (*schemas.UnifAICachedContentRetrieveResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.CachedContentRetrieveRequest); err != nil {
		return nil, err
	}
	if request.Name == "" {
		return nil, providerUtils.NewUnifAIOperationError("name is required for cached content retrieve", nil)
	}
	if len(keys) == 0 {
		return nil, providerUtils.NewUnifAIOperationError("no keys provided for cached content retrieve", nil)
	}

	var lastErr *schemas.UnifAIError
	for _, key := range keys {
		resp, latency, unifaiErr := provider.cachedContentRetrieveByKey(ctx, key, request)
		if unifaiErr == nil {
			resp.ExtraFields = schemas.UnifAIResponseExtraFields{Latency: latency.Milliseconds()}
			return resp, nil
		}
		lastErr = unifaiErr
	}
	return nil, lastErr
}

// cachedContentUpdateByKey updates expiration on a cached content for one key.
func (provider *GeminiProvider) cachedContentUpdateByKey(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAICachedContentUpdateRequest) (*schemas.UnifAICachedContentUpdateResponse, time.Duration, *schemas.UnifAIError) {
	body := geminiCachedContent{}
	updateMaskFields := []string{}
	if request.TTL != nil && *request.TTL != "" {
		body.TTL = *request.TTL
		updateMaskFields = append(updateMaskFields, "ttl")
	}
	if request.ExpireTime != nil && *request.ExpireTime != "" {
		body.ExpireTime = *request.ExpireTime
		updateMaskFields = append(updateMaskFields, "expireTime")
	}

	jsonBody, useRaw := providerUtils.CheckAndGetRawRequestBody(ctx, request)
	if !useRaw || len(jsonBody) == 0 {
		var marshalErr error
		jsonBody, marshalErr = sonic.Marshal(body)
		if marshalErr != nil {
			return nil, 0, providerUtils.NewUnifAIOperationError("failed to marshal cached content update body", marshalErr)
		}
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	name := normalizeCachedContentName(request.Name)
	requestURL := fmt.Sprintf("%s/%s", provider.networkConfig.BaseURL, name)
	if len(updateMaskFields) > 0 {
		requestURL += "?updateMask=" + strings.Join(updateMaskFields, ",")
	}

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodPatch)
	req.Header.SetContentType("application/json")
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}
	req.SetBody(jsonBody)

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, latency, unifaiErr
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, latency, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	respBody, decErr := providerUtils.CheckAndDecodeBody(resp)
	if decErr != nil {
		return nil, latency, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, decErr)
	}

	var geminiResp geminiCachedContent
	if err := sonic.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, latency, providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal, err)
	}

	return &schemas.UnifAICachedContentUpdateResponse{
		Name:              geminiResp.Name,
		DisplayName:       geminiResp.DisplayName,
		Model:             geminiResp.Model,
		SystemInstruction: geminiResp.SystemInstruction,
		Contents:          geminiResp.Contents,
		Tools:             geminiResp.Tools,
		ToolConfig:        geminiResp.ToolConfig,
		CreateTime:        geminiResp.CreateTime,
		UpdateTime:        geminiResp.UpdateTime,
		ExpireTime:        geminiResp.ExpireTime,
		UsageMetadata:     geminiResp.UsageMetadata,
	}, latency, nil
}

// CachedContentUpdate updates expiration on a cached content, trying each key.
func (provider *GeminiProvider) CachedContentUpdate(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAICachedContentUpdateRequest) (*schemas.UnifAICachedContentUpdateResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.CachedContentUpdateRequest); err != nil {
		return nil, err
	}
	if request.Name == "" {
		return nil, providerUtils.NewUnifAIOperationError("name is required for cached content update", nil)
	}
	if err := validateTTLExpireMutex(request.TTL, request.ExpireTime); err != nil {
		return nil, err
	}
	if (request.TTL == nil || *request.TTL == "") && (request.ExpireTime == nil || *request.ExpireTime == "") {
		return nil, providerUtils.NewUnifAIOperationError("either ttl or expire_time must be set for cached content update", nil)
	}
	if len(keys) == 0 {
		return nil, providerUtils.NewUnifAIOperationError("no keys provided for cached content update", nil)
	}

	var lastErr *schemas.UnifAIError
	for _, key := range keys {
		resp, latency, unifaiErr := provider.cachedContentUpdateByKey(ctx, key, request)
		if unifaiErr == nil {
			resp.ExtraFields = schemas.UnifAIResponseExtraFields{Latency: latency.Milliseconds()}
			return resp, nil
		}
		lastErr = unifaiErr
	}
	return nil, lastErr
}

// cachedContentDeleteByKey deletes a cached content for one key.
func (provider *GeminiProvider) cachedContentDeleteByKey(ctx *schemas.UnifAIContext, key schemas.Key, request *schemas.UnifAICachedContentDeleteRequest) (*schemas.UnifAICachedContentDeleteResponse, time.Duration, *schemas.UnifAIError) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	name := normalizeCachedContentName(request.Name)
	requestURL := fmt.Sprintf("%s/%s", provider.networkConfig.BaseURL, name)

	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)
	req.SetRequestURI(requestURL)
	req.Header.SetMethod(http.MethodDelete)
	if key.Value.GetValue() != "" {
		req.Header.Set("x-goog-api-key", key.Value.GetValue())
	}

	latency, unifaiErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if unifaiErr != nil {
		return nil, latency, unifaiErr
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, latency, providerUtils.SetErrorLatency(parseGeminiError(resp), latency)
	}

	return &schemas.UnifAICachedContentDeleteResponse{
		Name:    name,
		Deleted: true,
	}, latency, nil
}

// CachedContentDelete deletes a cached content by name, trying each key.
func (provider *GeminiProvider) CachedContentDelete(ctx *schemas.UnifAIContext, keys []schemas.Key, request *schemas.UnifAICachedContentDeleteRequest) (*schemas.UnifAICachedContentDeleteResponse, *schemas.UnifAIError) {
	if err := providerUtils.CheckOperationAllowed(schemas.Gemini, provider.customProviderConfig, schemas.CachedContentDeleteRequest); err != nil {
		return nil, err
	}
	if request.Name == "" {
		return nil, providerUtils.NewUnifAIOperationError("name is required for cached content delete", nil)
	}
	if len(keys) == 0 {
		return nil, providerUtils.NewUnifAIOperationError("no keys provided for cached content delete", nil)
	}

	var lastErr *schemas.UnifAIError
	for _, key := range keys {
		resp, latency, unifaiErr := provider.cachedContentDeleteByKey(ctx, key, request)
		if unifaiErr == nil {
			resp.ExtraFields = schemas.UnifAIResponseExtraFields{Latency: latency.Milliseconds()}
			return resp, nil
		}
		lastErr = unifaiErr
	}
	return nil, lastErr
}

// fromUnifAIObject is the inverse of toUnifAIObject — projects a unifai-canonical
// CachedContentObject back into the Gemini wire shape (camelCase keys).
//
// API ref: https://ai.google.dev/api/caching#CachedContent
func fromUnifAIObject(o schemas.CachedContentObject) geminiCachedContent {
	return geminiCachedContent{
		Name:              o.Name,
		DisplayName:       o.DisplayName,
		Model:             o.Model,
		SystemInstruction: o.SystemInstruction,
		Contents:          o.Contents,
		Tools:             o.Tools,
		ToolConfig:        o.ToolConfig,
		CreateTime:        o.CreateTime,
		UpdateTime:        o.UpdateTime,
		ExpireTime:        o.ExpireTime,
		UsageMetadata:     o.UsageMetadata,
	}
}
