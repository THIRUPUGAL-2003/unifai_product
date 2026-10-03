package azure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	openaiProvider "github.com/raksha/raksha/core/providers/openai"
	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// openAIEventHelper is a zero-value OpenAI provider used solely to delegate
// event conversion calls. Azure uses the exact same Realtime wire protocol as
// OpenAI, so all event parsing, serialisation, usage extraction, turn detection,
// and output extraction can be reused without modification.
var openAIEventHelper = &openaiProvider.OpenAIProvider{}

// ---------------------------------------------------------------------------
// RealtimeProvider interface
// ---------------------------------------------------------------------------

func (provider *AzureProvider) SupportsRealtimeAPI() bool {
	return true
}

func (provider *AzureProvider) RealtimeWebSocketURL(key schemas.Key, model string) string {
	endpoint := strings.TrimRight(key.AzureKeyConfig.Endpoint.GetValue(), "/")
	endpoint = strings.Replace(endpoint, "https://", "wss://", 1)
	endpoint = strings.Replace(endpoint, "http://", "ws://", 1)

	return fmt.Sprintf("%s/openai/v1/realtime?model=%s",
		endpoint, url.QueryEscape(model))
}

func (provider *AzureProvider) RealtimeHeaders(ctx *schemas.RakshaContext, key schemas.Key) (map[string]string, *schemas.RakshaError) {
	value := key.Value.GetValue()

	// Ephemeral tokens from /client_secrets use Bearer auth.
	if strings.HasPrefix(value, "ek_") {
		headers := map[string]string{
			"Authorization": "Bearer " + value,
		}
		for k, v := range provider.networkConfig.ExtraHeaders {
			headers[k] = v
		}
		return headers, nil
	}

	headers, authErr := provider.getAzureAuthHeaders(ctx, key, false)
	if authErr != nil {
		return nil, authErr
	}
	for k, v := range provider.networkConfig.ExtraHeaders {
		headers[k] = v
	}
	return headers, nil
}

func (provider *AzureProvider) SupportsRealtimeWebRTC() bool {
	return true
}

func (provider *AzureProvider) ExchangeRealtimeWebRTCSDP(
	ctx *schemas.RakshaContext,
	key schemas.Key,
	model string,
	sdp string,
	session json.RawMessage,
) (string, *schemas.RakshaError) {
	endpoint := strings.TrimRight(key.AzureKeyConfig.Endpoint.GetValue(), "/")

	upstreamURL := fmt.Sprintf("%s/openai/v1/realtime?model=%s",
		endpoint, url.QueryEscape(model))

	// Build multipart body: sdp + optional session
	bodyBuf := &bytes.Buffer{}
	writer := multipart.NewWriter(bodyBuf)
	if err := writer.WriteField("sdp", sdp); err != nil {
		return "", newAzureRealtimeError(fasthttp.StatusInternalServerError, "server_error", "failed to encode upstream SDP body", err)
	}
	if session != nil {
		if err := writer.WriteField("session", string(session)); err != nil {
			return "", newAzureRealtimeError(fasthttp.StatusInternalServerError, "server_error", "failed to encode upstream session body", err)
		}
	}
	if err := writer.Close(); err != nil {
		return "", newAzureRealtimeError(fasthttp.StatusInternalServerError, "server_error", "failed to finalize upstream SDP body", err)
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(upstreamURL)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType(writer.FormDataContentType())

	// Ephemeral tokens (ek_*) need Bearer auth; regular API keys use api-key header.
	value := key.Value.GetValue()
	if strings.HasPrefix(value, "ek_") {
		req.Header.Set("Authorization", "Bearer "+value)
	} else {
		authHeaders, authErr := provider.getAzureAuthHeaders(ctx, key, false)
		if authErr != nil {
			return "", authErr
		}
		for k, v := range authHeaders {
			req.Header.Set(k, v)
		}
	}

	for k, v := range provider.networkConfig.ExtraHeaders {
		req.Header.Set(k, v)
	}
	req.SetBody(bodyBuf.Bytes())

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return "", rakshaErr
	}

	answerBody := resp.Body()
	if resp.StatusCode() < fasthttp.StatusOK || resp.StatusCode() >= fasthttp.StatusMultipleChoices {
		return "", providerUtils.SetErrorLatency(provider.realtimeWebRTCUpstreamError(ctx, resp.StatusCode(), answerBody), latency)
	}

	return string(answerBody), nil
}

// ---------------------------------------------------------------------------
// Event conversion — delegates to OpenAI (same wire protocol)
// ---------------------------------------------------------------------------

func (provider *AzureProvider) ToRakshaRealtimeEvent(providerEvent json.RawMessage) (*schemas.RakshaRealtimeEvent, error) {
	return openAIEventHelper.ToRakshaRealtimeEvent(providerEvent)
}

func (provider *AzureProvider) ToProviderRealtimeEvent(rakshaEvent *schemas.RakshaRealtimeEvent) (json.RawMessage, error) {
	return openAIEventHelper.ToProviderRealtimeEvent(rakshaEvent)
}

// ---------------------------------------------------------------------------
// Turn lifecycle — delegates to OpenAI
// ---------------------------------------------------------------------------

func (provider *AzureProvider) ShouldStartRealtimeTurn(event *schemas.RakshaRealtimeEvent) bool {
	return openAIEventHelper.ShouldStartRealtimeTurn(event)
}

func (provider *AzureProvider) RealtimeTurnFinalEvent() schemas.RealtimeEventType {
	return openAIEventHelper.RealtimeTurnFinalEvent()
}

func (provider *AzureProvider) ShouldForwardRealtimeEvent(event *schemas.RakshaRealtimeEvent) bool {
	return true
}

func (provider *AzureProvider) ShouldAccumulateRealtimeOutput(eventType schemas.RealtimeEventType) bool {
	return openAIEventHelper.ShouldAccumulateRealtimeOutput(eventType)
}

func (provider *AzureProvider) RealtimeWebRTCDataChannelLabel() string {
	return "oai-events"
}

func (provider *AzureProvider) RealtimeWebSocketSubprotocol() string {
	return "realtime"
}

// ---------------------------------------------------------------------------
// RealtimeUsageExtractor — delegates to OpenAI
// ---------------------------------------------------------------------------

func (provider *AzureProvider) ExtractRealtimeTurnUsage(terminalEventRaw []byte) *schemas.RakshaLLMUsage {
	return openAIEventHelper.ExtractRealtimeTurnUsage(terminalEventRaw)
}

func (provider *AzureProvider) ExtractRealtimeTurnOutput(terminalEventRaw []byte) *schemas.ChatMessage {
	return openAIEventHelper.ExtractRealtimeTurnOutput(terminalEventRaw)
}

// ---------------------------------------------------------------------------
// RealtimeSessionProvider — client_secrets only (not legacy /sessions)
// ---------------------------------------------------------------------------

func (provider *AzureProvider) CreateRealtimeClientSecret(
	ctx *schemas.RakshaContext,
	key schemas.Key,
	endpointType schemas.RealtimeSessionEndpointType,
	rawRequest json.RawMessage,
) (*schemas.RakshaPassthroughResponse, *schemas.RakshaError) {
	// Azure does not support the legacy /sessions endpoint.
	if endpointType == schemas.RealtimeSessionEndpointSessions {
		return nil, &schemas.RakshaError{
			IsRakshaError: true,
			StatusCode:     schemas.Ptr(fasthttp.StatusBadRequest),
			Error: &schemas.ErrorField{
				Type:    schemas.Ptr("invalid_request_error"),
				Message: "Azure does not support the legacy /sessions endpoint; use /v1/realtime/client_secrets instead",
			},
			ExtraFields: schemas.RakshaErrorExtraFields{
				RequestType: schemas.RealtimeRequest,
				Provider:    provider.GetProviderKey(),
			},
		}
	}

	normalizedBody, _, rakshaErr := openaiProvider.NormalizeRealtimeClientSecretRequest(rawRequest, schemas.Azure, endpointType)
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	endpoint := strings.TrimRight(key.AzureKeyConfig.Endpoint.GetValue(), "/")
	upstreamURL := fmt.Sprintf("%s/openai/v1/realtime/client_secrets", endpoint)

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(upstreamURL)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")

	authHeaders, authErr := provider.getAzureAuthHeaders(ctx, key, false)
	if authErr != nil {
		return nil, authErr
	}
	for k, v := range authHeaders {
		req.Header.Set(k, v)
	}
	for k, v := range provider.networkConfig.ExtraHeaders {
		req.Header.Set(k, v)
	}
	req.SetBody(normalizedBody)

	latency, rakshaErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if rakshaErr != nil {
		return nil, rakshaErr
	}

	headers := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.RakshaContextKeyProviderResponseHeaders, headers)

	if resp.StatusCode() < fasthttp.StatusOK || resp.StatusCode() >= fasthttp.StatusMultipleChoices {
		return nil, providerUtils.SetErrorLatency(provider.parseRealtimeClientSecretError(ctx, resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewRakshaOperationError("failed to decode response body", err)
	}

	out := &schemas.RakshaPassthroughResponse{
		StatusCode: resp.StatusCode(),
		Headers:    headers,
		Body:       body,
		ExtraFields: schemas.RakshaResponseExtraFields{
			Latency:                 latency.Milliseconds(),
			ProviderResponseHeaders: headers,
		},
	}
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		providerUtils.ParseAndSetRawRequestIfJSON(req, &out.ExtraFields)
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (provider *AzureProvider) realtimeWebRTCUpstreamError(ctx *schemas.RakshaContext, statusCode int, body []byte) *schemas.RakshaError {
	message := fmt.Sprintf("upstream realtime handshake failed for %s", provider.GetProviderKey())
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Error.Message != "" {
		message = parsed.Error.Message
	}

	rakshaErr := &schemas.RakshaError{
		IsRakshaError: false,
		StatusCode:     schemas.Ptr(statusCode),
		Error: &schemas.ErrorField{
			Type:    schemas.Ptr("upstream_error"),
			Message: message,
		},
		ExtraFields: schemas.RakshaErrorExtraFields{
			RequestType: schemas.RealtimeRequest,
			Provider:    provider.GetProviderKey(),
		},
	}
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		rakshaErr.ExtraFields.RawResponse = map[string]any{
			"status": statusCode,
			"body":   string(body),
		}
	}
	return rakshaErr
}

func newAzureRealtimeError(status int, errorType, message string, err error) *schemas.RakshaError {
	rakshaErr := &schemas.RakshaError{
		IsRakshaError: true,
		StatusCode:     schemas.Ptr(status),
		Error: &schemas.ErrorField{
			Type:    schemas.Ptr(errorType),
			Message: message,
		},
		ExtraFields: schemas.RakshaErrorExtraFields{
			RequestType: schemas.RealtimeRequest,
			Provider:    schemas.Azure,
		},
	}
	if err != nil {
		rakshaErr.Error.Error = err
	}
	return rakshaErr
}

func (provider *AzureProvider) parseRealtimeClientSecretError(ctx *schemas.RakshaContext, resp *fasthttp.Response) *schemas.RakshaError {
	body, _ := providerUtils.CheckAndDecodeBody(resp)
	var parsed struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	msg := string(body)
	if json.Unmarshal(body, &parsed) == nil && parsed.Error.Message != "" {
		msg = parsed.Error.Message
	}
	rakshaErr := &schemas.RakshaError{
		IsRakshaError: false,
		StatusCode:     schemas.Ptr(resp.StatusCode()),
		Error: &schemas.ErrorField{
			Type:    schemas.Ptr("upstream_error"),
			Message: msg,
		},
		ExtraFields: schemas.RakshaErrorExtraFields{
			RequestType: schemas.RealtimeRequest,
			Provider:    provider.GetProviderKey(),
		},
	}
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		rakshaErr.ExtraFields.RawResponse = map[string]any{
			"status": resp.StatusCode(),
			"body":   string(body),
		}
	}
	return rakshaErr
}
