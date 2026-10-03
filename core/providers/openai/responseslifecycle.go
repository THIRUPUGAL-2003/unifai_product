package openai

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/valyala/fasthttp"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
)

func buildResponsesRetrieveQuery(req *schemas.RakshaResponsesRetrieveRequest) string {
	if req == nil {
		return ""
	}
	v := url.Values{}
	for _, inc := range req.Include {
		if inc != "" {
			v.Add("include", inc)
		}
	}
	if req.StartingAfter != nil {
		v.Set("starting_after", strconv.Itoa(*req.StartingAfter))
	}
	if req.IncludeObfuscation != nil {
		v.Set("include_obfuscation", strconv.FormatBool(*req.IncludeObfuscation))
	}
	return v.Encode()
}

func buildResponsesInputItemsQuery(req *schemas.RakshaResponsesInputItemsRequest) string {
	if req == nil {
		return ""
	}
	v := url.Values{}
	if req.After != "" {
		v.Set("after", req.After)
	}
	for _, inc := range req.Include {
		if inc != "" {
			v.Add("include", inc)
		}
	}
	if req.Limit != nil {
		v.Set("limit", strconv.Itoa(*req.Limit))
	}
	if req.Order != "" {
		v.Set("order", req.Order)
	}
	return v.Encode()
}

// executeResponsesLifecycleUnary performs a unary HTTP call for Responses lifecycle endpoints.
func (provider *OpenAIProvider) executeResponsesLifecycleUnary(
	ctx *schemas.RakshaContext,
	method string,
	relativePath string,
	requestType schemas.RequestType,
	rawQuery string,
	key schemas.Key,
	body []byte,
) ([]byte, int64, map[string]string, *schemas.RakshaError) {
	effectiveBody := body
	fullURL := provider.buildRequestURL(ctx, relativePath, requestType)
	if rawQuery != "" {
		fullURL = fullURL + "?" + rawQuery
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	respOwned := true
	defer func() {
		if respOwned {
			fasthttp.ReleaseResponse(resp)
		}
	}()

	// Lifecycle JSON is always consumed in-process (no transport streaming). Skip
	// PrepareResponseStreaming so large-response threshold mode never leaves the body
	// on a stream-only path that finalizeOpenAIResponse would treat as unsupported here.
	activeClient := provider.client
	providerUtils.SetExtraHeaders(ctx, req, provider.networkConfig.ExtraHeaders, nil)

	req.SetRequestURI(fullURL)
	req.Header.SetMethod(method)
	if method == http.MethodPost || len(body) > 0 {
		req.Header.SetContentType("application/json")
		if len(body) > 0 {
			req.SetBody(body)
		} else if method == http.MethodPost {
			effectiveBody = []byte("{}")
			req.SetBody(effectiveBody)
		}
	}

	if key.Value.GetValue() != "" {
		req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	}

	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, activeClient, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, 0, nil, providerUtils.EnrichError(ctx, rakshaErr, effectiveBody, nil, sendBackRawRequest, sendBackRawResponse)
	}

	providerResponseHeaders := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, providerResponseHeaders)

	if resp.StatusCode() != fasthttp.StatusOK {
		providerUtils.MaterializeStreamErrorBody(ctx, resp)
		return nil, 0, providerResponseHeaders, providerUtils.EnrichError(ctx, ParseOpenAIError(resp), effectiveBody, resp.Body(), sendBackRawRequest, sendBackRawResponse)
	}

	bodyBytes, lpResult, finalErr := finalizeOpenAIResponse(ctx, resp, latency, provider.GetProviderKey(), provider.logger)
	respOwned = false
	if finalErr != nil {
		return nil, 0, providerResponseHeaders, providerUtils.EnrichError(ctx, finalErr, effectiveBody, nil, sendBackRawRequest, sendBackRawResponse)
	}
	if lpResult != nil {
		return nil, lpResult.Latency, providerResponseHeaders, providerUtils.NewRakshaOperationError(
			schemas.ErrProviderResponseEmpty,
			fmt.Errorf("responses lifecycle does not support large-response streaming mode"),
		)
	}

	return bodyBytes, latency.Milliseconds(), providerResponseHeaders, nil
}

// ResponsesRetrieve implements schemas.ResponsesLifecycleProvider.
func (provider *OpenAIProvider) ResponsesRetrieve(ctx *schemas.RakshaContext, key schemas.Key, req *schemas.RakshaResponsesRetrieveRequest) (*schemas.RakshaResponsesResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ResponsesRetrieveRequest); err != nil {
		return nil, err
	}
	if req == nil || req.ResponseID == "" {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrRequestBodyConversion, fmt.Errorf("response_id is required"))
	}

	path := "/v1/responses/" + url.PathEscape(req.ResponseID)
	bodyBytes, latencyMs, headers, rakshaErr := provider.executeResponsesLifecycleUnary(
		ctx, http.MethodGet, path, schemas.ResponsesRetrieveRequest, buildResponsesRetrieveQuery(req), key, nil)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	response := &schemas.RakshaResponsesResponse{}
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)
	_, rawResponse, err := providerUtils.HandleProviderResponse(bodyBytes, response, nil, sendBackRawRequest, sendBackRawResponse)
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, err, nil, bodyBytes, sendBackRawRequest, sendBackRawResponse)
	}
	response.ExtraFields.Latency = latencyMs
	response.ExtraFields.ProviderResponseHeaders = headers
	response.ExtraFields.Provider = provider.GetProviderKey()
	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}
	return response, nil
}

// ResponsesDelete implements schemas.ResponsesLifecycleProvider.
func (provider *OpenAIProvider) ResponsesDelete(ctx *schemas.RakshaContext, key schemas.Key, req *schemas.RakshaResponsesDeleteRequest) (*schemas.RakshaResponsesDeleteResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ResponsesDeleteRequest); err != nil {
		return nil, err
	}
	if req == nil || req.ResponseID == "" {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrRequestBodyConversion, fmt.Errorf("response_id is required"))
	}

	path := "/v1/responses/" + url.PathEscape(req.ResponseID)
	bodyBytes, latencyMs, headers, rakshaErr := provider.executeResponsesLifecycleUnary(
		ctx, http.MethodDelete, path, schemas.ResponsesDeleteRequest, "", key, nil)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	response := &schemas.RakshaResponsesDeleteResponse{}
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)
	_, rawResponse, err := providerUtils.HandleProviderResponse(bodyBytes, response, nil, sendBackRawRequest, sendBackRawResponse)
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, err, nil, bodyBytes, sendBackRawRequest, sendBackRawResponse)
	}
	response.ExtraFields.Latency = latencyMs
	response.ExtraFields.ProviderResponseHeaders = headers
	response.ExtraFields.Provider = provider.GetProviderKey()
	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}
	return response, nil
}

// ResponsesCancel implements schemas.ResponsesLifecycleProvider.
func (provider *OpenAIProvider) ResponsesCancel(ctx *schemas.RakshaContext, key schemas.Key, req *schemas.RakshaResponsesCancelRequest) (*schemas.RakshaResponsesResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ResponsesCancelRequest); err != nil {
		return nil, err
	}
	if req == nil || req.ResponseID == "" {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrRequestBodyConversion, fmt.Errorf("response_id is required"))
	}

	path := "/v1/responses/" + url.PathEscape(req.ResponseID) + "/cancel"
	bodyBytes, latencyMs, headers, rakshaErr := provider.executeResponsesLifecycleUnary(
		ctx, http.MethodPost, path, schemas.ResponsesCancelRequest, "", key, nil)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	response := &schemas.RakshaResponsesResponse{}
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)
	cancelBody := []byte("{}")
	rawRequest, rawResponse, err := providerUtils.HandleProviderResponse(bodyBytes, response, cancelBody, sendBackRawRequest, sendBackRawResponse)
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, err, cancelBody, bodyBytes, sendBackRawRequest, sendBackRawResponse)
	}
	response.ExtraFields.Latency = latencyMs
	response.ExtraFields.ProviderResponseHeaders = headers
	response.ExtraFields.Provider = provider.GetProviderKey()
	if sendBackRawRequest {
		response.ExtraFields.RawRequest = rawRequest
	}
	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}
	return response, nil
}

// ResponsesInputItems implements schemas.ResponsesLifecycleProvider.
func (provider *OpenAIProvider) ResponsesInputItems(ctx *schemas.RakshaContext, key schemas.Key, req *schemas.RakshaResponsesInputItemsRequest) (*schemas.RakshaResponsesInputItemsResponse, *schemas.RakshaError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.ResponsesInputItemsRequest); err != nil {
		return nil, err
	}
	if req == nil || req.ResponseID == "" {
		return nil, providerUtils.NewRakshaOperationError(schemas.ErrRequestBodyConversion, fmt.Errorf("response_id is required"))
	}

	path := "/v1/responses/" + url.PathEscape(req.ResponseID) + "/input_items"
	bodyBytes, latencyMs, headers, rakshaErr := provider.executeResponsesLifecycleUnary(
		ctx, http.MethodGet, path, schemas.ResponsesInputItemsRequest, buildResponsesInputItemsQuery(req), key, nil)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	response := &schemas.RakshaResponsesInputItemsResponse{}
	sendBackRawRequest := providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest)
	sendBackRawResponse := providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse)
	_, rawResponse, err := providerUtils.HandleProviderResponse(bodyBytes, response, nil, sendBackRawRequest, sendBackRawResponse)
	if err != nil {
		return nil, providerUtils.EnrichError(ctx, err, nil, bodyBytes, sendBackRawRequest, sendBackRawResponse)
	}
	response.ExtraFields.Latency = latencyMs
	response.ExtraFields.ProviderResponseHeaders = headers
	response.ExtraFields.Provider = provider.GetProviderKey()
	if sendBackRawResponse {
		response.ExtraFields.RawResponse = rawResponse
	}
	return response, nil
}
