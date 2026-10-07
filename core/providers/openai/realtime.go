package openai

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

// SupportsRealtimeAPI returns true since OpenAI natively supports the Realtime API.
func (provider *OpenAIProvider) SupportsRealtimeAPI() bool {
	return true
}

// RealtimeWebSocketURL returns the WSS URL for the OpenAI Realtime API.
// Format: wss://api.openai.com/v1/realtime?model=<model>
func (provider *OpenAIProvider) RealtimeWebSocketURL(key schemas.Key, model string) string {
	base := provider.networkConfig.BaseURL
	base = strings.Replace(base, "https://", "wss://", 1)
	base = strings.Replace(base, "http://", "ws://", 1)
	return base + "/v1/realtime?model=" + url.QueryEscape(model)
}

// RealtimeHeaders returns the headers required for the OpenAI Realtime WebSocket connection.
func (provider *OpenAIProvider) RealtimeHeaders(_ *schemas.GatewayContext, key schemas.Key) (map[string]string, *schemas.GatewayError) {
	headers := map[string]string{
		"Authorization": "Bearer " + key.Value.GetValue(),
	}
	for k, v := range provider.networkConfig.ExtraHeaders {
		headers[k] = v
	}
	return headers, nil
}

// SupportsRealtimeWebRTC reports that OpenAI supports WebRTC SDP exchange.
func (provider *OpenAIProvider) SupportsRealtimeWebRTC() bool {
	return true
}

// ExchangeRealtimeWebRTCSDP performs the GA SDP exchange via multipart POST to /v1/realtime/calls.
func (provider *OpenAIProvider) ExchangeRealtimeWebRTCSDP(
	ctx *schemas.GatewayContext,
	key schemas.Key,
	model string,
	sdp string,
	session json.RawMessage,
) (string, *schemas.GatewayError) {
	path := "/v1/realtime/calls"
	if session == nil && strings.TrimSpace(model) != "" {
		path += "?model=" + url.QueryEscape(model)
	}
	return provider.exchangeWebRTCSDP(ctx, key, path, sdp, session)
}

// ExchangeLegacyRealtimeWebRTCSDP performs the beta SDP exchange via multipart POST to /v1/realtime.
// Same multipart format but targets the legacy endpoint with model in the URL.
func (provider *OpenAIProvider) ExchangeLegacyRealtimeWebRTCSDP(
	ctx *schemas.GatewayContext,
	key schemas.Key,
	sdp string,
	session json.RawMessage,
	model string,
) (string, *schemas.GatewayError) {
	return provider.exchangeWebRTCSDP(ctx, key, "/v1/realtime?model="+url.QueryEscape(model), sdp, session)
}

// exchangeWebRTCSDP is the shared multipart SDP exchange implementation.
// Builds a multipart body with sdp + optional session, POSTs to the given path.
func (provider *OpenAIProvider) exchangeWebRTCSDP(
	ctx *schemas.GatewayContext,
	key schemas.Key,
	path string,
	sdp string,
	session json.RawMessage,
) (string, *schemas.GatewayError) {
	bodyBuf := &bytes.Buffer{}
	writer := multipart.NewWriter(bodyBuf)
	if err := writer.WriteField("sdp", sdp); err != nil {
		return "", newRealtimeWebRTCSDPError(fasthttp.StatusInternalServerError, "server_error", "failed to encode upstream SDP body", err)
	}
	if session != nil {
		if err := writer.WriteField("session", string(session)); err != nil {
			return "", newRealtimeWebRTCSDPError(fasthttp.StatusInternalServerError, "server_error", "failed to encode upstream session body", err)
		}
	}
	if err := writer.Close(); err != nil {
		return "", newRealtimeWebRTCSDPError(fasthttp.StatusInternalServerError, "server_error", "failed to finalize upstream SDP body", err)
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(provider.buildRequestURL(ctx, path, schemas.RealtimeRequest))
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType(writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+key.Value.GetValue())
	for k, v := range provider.networkConfig.ExtraHeaders {
		req.Header.Set(k, v)
	}
	if headers, _ := ctx.Value(schemas.GatewayContextKeyRequestHeaders).(map[string]string); headers != nil {
		if agentsSDK := headers["x-openai-agents-sdk"]; agentsSDK != "" {
			req.Header.Set("X-OpenAI-Agents-SDK", agentsSDK)
		}
	}
	req.SetBody(bodyBuf.Bytes())

	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return "", gatewayErr
	}

	answerBody := resp.Body()
	if resp.StatusCode() < fasthttp.StatusOK || resp.StatusCode() >= fasthttp.StatusMultipleChoices {
		return "", providerUtils.SetErrorLatency(provider.realtimeWebRTCUpstreamError(ctx, resp.StatusCode(), answerBody), latency)
	}

	return string(answerBody), nil
}

func (provider *OpenAIProvider) realtimeWebRTCUpstreamError(ctx *schemas.GatewayContext, statusCode int, body []byte) *schemas.GatewayError {
	gatewayErr := &schemas.GatewayError{
		IsGatewayError: false,
		StatusCode:     schemas.Ptr(fasthttp.StatusBadGateway),
		Error: &schemas.ErrorField{
			Type:    schemas.Ptr("upstream_connection_error"),
			Message: fmt.Sprintf("upstream realtime WebRTC handshake failed for %s", provider.GetProviderKey()),
		},
		ExtraFields: schemas.GatewayErrorExtraFields{
			RequestType: schemas.RealtimeRequest,
			Provider:    provider.GetProviderKey(),
		},
	}
	if providerUtils.ShouldSendBackRawResponse(ctx, provider.sendBackRawResponse) {
		gatewayErr.ExtraFields.RawResponse = map[string]any{
			"status": statusCode,
			"body":   string(body),
		}
	}
	return gatewayErr
}

func newRealtimeWebRTCSDPError(status int, errorType, message string, err error) *schemas.GatewayError {
	gatewayErr := &schemas.GatewayError{
		IsGatewayError: true,
		StatusCode:     schemas.Ptr(status),
		Error: &schemas.ErrorField{
			Type:    schemas.Ptr(errorType),
			Message: message,
		},
	}
	if err != nil {
		gatewayErr.Error.Error = err
	}
	return gatewayErr
}

func (provider *OpenAIProvider) ShouldStartRealtimeTurn(event *schemas.GatewayRealtimeEvent) bool {
	if event == nil {
		return false
	}
	switch event.Type {
	case schemas.RTEventResponseCreate, schemas.RTEventInputAudioBufferCommitted:
		return true
	default:
		return false
	}
}

func (provider *OpenAIProvider) RealtimeTurnFinalEvent() schemas.RealtimeEventType {
	return schemas.RTEventResponseDone
}

func (provider *OpenAIProvider) RealtimeWebRTCDataChannelLabel() string {
	return "oai-events"
}

func (provider *OpenAIProvider) RealtimeWebSocketSubprotocol() string {
	return "realtime"
}

func (provider *OpenAIProvider) ShouldForwardRealtimeEvent(event *schemas.GatewayRealtimeEvent) bool {
	return true
}

func (provider *OpenAIProvider) ShouldAccumulateRealtimeOutput(eventType schemas.RealtimeEventType) bool {
	switch eventType {
	case schemas.RTEventResponseTextDelta,
		schemas.RTEventResponseAudioTransDelta,
		schemas.RealtimeEventType("response.output_text.delta"),
		schemas.RealtimeEventType("response.output_audio_transcript.delta"):
		return true
	default:
		return false
	}
}

// CreateRealtimeClientSecret mints an OpenAI Realtime client secret and returns
// the native OpenAI response body unchanged.
func (provider *OpenAIProvider) CreateRealtimeClientSecret(
	ctx *schemas.GatewayContext,
	key schemas.Key,
	endpointType schemas.RealtimeSessionEndpointType,
	rawRequest json.RawMessage,
) (*schemas.GatewayPassthroughResponse, *schemas.GatewayError) {
	if err := providerUtils.CheckOperationAllowed(schemas.OpenAI, provider.customProviderConfig, schemas.RealtimeRequest); err != nil {
		return nil, err
	}

	normalizedBody, _, gatewayErr := NormalizeRealtimeClientSecretRequest(rawRequest, provider.GetProviderKey(), endpointType)
	if gatewayErr != nil {
		return nil, gatewayErr
	}
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	upstreamURL := provider.buildRequestURL(ctx, realtimeSessionUpstreamPath(endpointType), schemas.RealtimeRequest)
	req.SetRequestURI(upstreamURL)
	req.Header.SetMethod(http.MethodPost)
	req.Header.SetContentType("application/json")
	for k, v := range provider.realtimeSessionHeaders(key, endpointType) {
		req.Header.Set(k, v)
	}
	req.SetBody(normalizedBody)

	latency, gatewayErr, wait := providerUtils.MakeRequestWithContext(ctx, provider.client, req, resp)
	defer wait()
	if gatewayErr != nil {
		return nil, gatewayErr
	}

	headers := providerUtils.ExtractProviderResponseHeaders(resp)
	ctx.SetValue(schemas.GatewayContextKeyProviderResponseHeaders, headers)

	if resp.StatusCode() < fasthttp.StatusOK || resp.StatusCode() >= fasthttp.StatusMultipleChoices {
		return nil, providerUtils.SetErrorLatency(ParseOpenAIError(resp), latency)
	}

	body, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		return nil, providerUtils.NewGatewayOperationError("failed to decode response body", err)
	}

	out := &schemas.GatewayPassthroughResponse{
		StatusCode: resp.StatusCode(),
		Headers:    headers,
		Body:       body,
		ExtraFields: schemas.GatewayResponseExtraFields{
			Latency:                 latency.Milliseconds(),
			ProviderResponseHeaders: headers,
		},
	}
	if providerUtils.ShouldSendBackRawRequest(ctx, provider.sendBackRawRequest) {
		providerUtils.ParseAndSetRawRequestIfJSON(req, &out.ExtraFields)
	}

	return out, nil
}

// NormalizeRealtimeClientSecretRequest normalizes a realtime client secret request body
// by parsing the model string, resolving the provider, and restructuring the body
// to match the upstream provider's expected format. Exported for reuse by providers
// that share the same OpenAI-compatible Realtime protocol (e.g. Azure).
func NormalizeRealtimeClientSecretRequest(
	rawRequest json.RawMessage,
	defaultProvider schemas.ModelProvider,
	endpointType schemas.RealtimeSessionEndpointType,
) ([]byte, string, *schemas.GatewayError) {
	root, gatewayErr := schemas.ParseRealtimeClientSecretBody(rawRequest)
	if gatewayErr != nil {
		return nil, "", gatewayErr
	}

	modelValue, gatewayErr := schemas.ExtractRealtimeClientSecretModel(root)
	if gatewayErr != nil {
		return nil, "", gatewayErr
	}
	providerKey, normalizedModel := schemas.ParseModelString(modelValue, defaultProvider)
	if normalizedModel == "" {
		return nil, "", newRealtimeClientSecretError(fasthttp.StatusBadRequest, "invalid_request_error", "session.model is required", nil)
	}
	if providerKey == "" {
		providerKey = defaultProvider
	}
	if providerKey == "" {
		return nil, "", newRealtimeClientSecretError(fasthttp.StatusBadRequest, "invalid_request_error", "unable to determine provider from model", nil)
	}

	if endpointType == schemas.RealtimeSessionEndpointSessions {
		return normalizeRealtimeSessionsRequest(root, normalizedModel)
	}

	return normalizeRealtimeClientSecretsRequest(root, normalizedModel)
}

func normalizeRealtimeClientSecretsRequest(
	root map[string]json.RawMessage,
	normalizedModel string,
) ([]byte, string, *schemas.GatewayError) {
	session := map[string]json.RawMessage{}
	if existingSession, ok := root["session"]; ok && len(existingSession) > 0 && !bytes.Equal(existingSession, []byte("null")) {
		if err := json.Unmarshal(existingSession, &session); err != nil {
			return nil, "", newRealtimeClientSecretError(fasthttp.StatusBadRequest, "invalid_request_error", "session must be an object", err)
		}
	}

	modelJSON, marshalErr := json.Marshal(normalizedModel)
	if marshalErr != nil {
		return nil, "", newRealtimeClientSecretError(fasthttp.StatusInternalServerError, "server_error", "failed to encode normalized model", marshalErr)
	}
	session["model"] = modelJSON
	StripNestedModelPrefixes(session)
	if _, ok := session["type"]; !ok {
		typeJSON, marshalErr := json.Marshal("realtime")
		if marshalErr != nil {
			return nil, "", newRealtimeClientSecretError(fasthttp.StatusInternalServerError, "server_error", "failed to encode realtime session type", marshalErr)
		}
		session["type"] = typeJSON
	}
	delete(root, "model")

	sessionJSON, marshalErr := json.Marshal(session)
	if marshalErr != nil {
		return nil, "", newRealtimeClientSecretError(fasthttp.StatusInternalServerError, "server_error", "failed to encode realtime session", marshalErr)
	}
	root["session"] = sessionJSON

	normalizedBody, marshalErr := json.Marshal(root)
	if marshalErr != nil {
		return nil, "", newRealtimeClientSecretError(fasthttp.StatusInternalServerError, "server_error", "failed to encode realtime request", marshalErr)
	}

	return normalizedBody, normalizedModel, nil
}

func normalizeRealtimeSessionsRequest(
	root map[string]json.RawMessage,
	normalizedModel string,
) ([]byte, string, *schemas.GatewayError) {
	if existingSession, ok := root["session"]; ok && len(existingSession) > 0 && !bytes.Equal(existingSession, []byte("null")) {
		session := map[string]json.RawMessage{}
		if err := json.Unmarshal(existingSession, &session); err != nil {
			return nil, "", newRealtimeClientSecretError(fasthttp.StatusBadRequest, "invalid_request_error", "session must be an object", err)
		}
		for key, value := range session {
			if _, exists := root[key]; !exists {
				root[key] = value
			}
		}
	}

	modelJSON, marshalErr := json.Marshal(normalizedModel)
	if marshalErr != nil {
		return nil, "", newRealtimeClientSecretError(fasthttp.StatusInternalServerError, "server_error", "failed to encode normalized model", marshalErr)
	}
	root["model"] = modelJSON
	delete(root, "session")
	StripNestedModelPrefixes(root)

	normalizedBody, marshalErr := json.Marshal(root)
	if marshalErr != nil {
		return nil, "", newRealtimeClientSecretError(fasthttp.StatusInternalServerError, "server_error", "failed to encode realtime request", marshalErr)
	}

	return normalizedBody, normalizedModel, nil
}

// StripNestedModelPrefixes removes provider prefixes (e.g. "openai/whisper-1" → "whisper-1")
// from known nested model fields in the realtime session config. This prevents forwarding
// Gateway-style "provider/model" strings to upstream providers that expect bare model names.
func StripNestedModelPrefixes(session map[string]json.RawMessage) {
	// Old format: input_audio_transcription.model
	stripModelInNestedObject(session, "input_audio_transcription")

	// New format: audio.input.transcription.model
	if audioRaw, ok := session["audio"]; ok {
		var audio map[string]json.RawMessage
		if json.Unmarshal(audioRaw, &audio) == nil {
			if inputRaw, ok := audio["input"]; ok {
				var input map[string]json.RawMessage
				if json.Unmarshal(inputRaw, &input) == nil {
					if stripModelInNestedObject(input, "transcription") {
						if updated, err := json.Marshal(input); err == nil {
							audio["input"] = updated
							if updatedAudio, err := json.Marshal(audio); err == nil {
								session["audio"] = updatedAudio
							}
						}
					}
				}
			}
		}
	}
}

// stripModelInNestedObject strips the provider prefix from a "model" field inside a nested
// object at session[key]. Returns true if any change was made.
func stripModelInNestedObject(parent map[string]json.RawMessage, key string) bool {
	objRaw, ok := parent[key]
	if !ok || len(objRaw) == 0 || bytes.Equal(objRaw, []byte("null")) {
		return false
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(objRaw, &obj) != nil {
		return false
	}
	modelRaw, ok := obj["model"]
	if !ok {
		return false
	}
	var modelStr string
	if json.Unmarshal(modelRaw, &modelStr) != nil {
		return false
	}
	// Strip provider prefix if present (e.g. "openai/whisper-1" → "whisper-1")
	_, bareModel := schemas.ParseModelString(modelStr, "")
	if bareModel == modelStr {
		return false // no prefix to strip
	}
	if updated, err := json.Marshal(bareModel); err == nil {
		obj["model"] = updated
		if updatedObj, err := json.Marshal(obj); err == nil {
			parent[key] = updatedObj
			return true
		}
	}
	return false
}

func (provider *OpenAIProvider) realtimeSessionHeaders(
	key schemas.Key,
	endpointType schemas.RealtimeSessionEndpointType,
) map[string]string {
	headers := map[string]string{
		"Authorization": "Bearer " + key.Value.GetValue(),
	}
	if endpointType == schemas.RealtimeSessionEndpointSessions {
		headers["OpenAI-Beta"] = "realtime=v1"
	}
	for k, v := range provider.networkConfig.ExtraHeaders {
		headers[k] = v
	}
	return headers
}

func realtimeSessionUpstreamPath(endpointType schemas.RealtimeSessionEndpointType) string {
	if endpointType == schemas.RealtimeSessionEndpointSessions {
		return "/v1/realtime/sessions"
	}
	return "/v1/realtime/client_secrets"
}

func newRealtimeClientSecretError(status int, errorType, message string, err error) *schemas.GatewayError {
	return &schemas.GatewayError{
		IsGatewayError: false,
		StatusCode:     schemas.Ptr(status),
		Error: &schemas.ErrorField{
			Type:    schemas.Ptr(errorType),
			Message: message,
			Error:   err,
		},
		ExtraFields: schemas.GatewayErrorExtraFields{
			RequestType: schemas.RealtimeRequest,
			Provider:    schemas.OpenAI,
		},
	}
}

// openAIRealtimeEvent is the raw shape of an OpenAI Realtime protocol event.
type openAIRealtimeEvent struct {
	Type         string          `json:"type"`
	EventID      string          `json:"event_id,omitempty"`
	Session      json.RawMessage `json:"session,omitempty"`
	Conversation json.RawMessage `json:"conversation,omitempty"`
	Item         json.RawMessage `json:"item,omitempty"`
	Response     json.RawMessage `json:"response,omitempty"`
	Part         json.RawMessage `json:"part,omitempty"`
	Delta        string          `json:"delta,omitempty"`
	Audio        string          `json:"audio,omitempty"`
	Transcript   string          `json:"transcript,omitempty"`
	Text         string          `json:"text,omitempty"`
	Error        json.RawMessage `json:"error,omitempty"`
	ItemID       string          `json:"item_id,omitempty"`
	OutputIndex  *int            `json:"output_index,omitempty"`
	ContentIndex *int            `json:"content_index,omitempty"`
	ResponseID   string          `json:"response_id,omitempty"`
	AudioEndMS   *int            `json:"audio_end_ms,omitempty"`

	PreviousItemID string `json:"previous_item_id,omitempty"`
}

// openAIRealtimeSession is the session object within an OpenAI Realtime event.
type openAIRealtimeSession struct {
	ID               string          `json:"id,omitempty"`
	Model            string          `json:"model,omitempty"`
	Modalities       []string        `json:"modalities,omitempty"`
	Instructions     string          `json:"instructions,omitempty"`
	Voice            string          `json:"voice,omitempty"`
	Temperature      *float64        `json:"temperature,omitempty"`
	MaxOutputTokens  json.RawMessage `json:"max_output_tokens,omitempty"`
	TurnDetection    json.RawMessage `json:"turn_detection,omitempty"`
	InputAudioFormat string          `json:"input_audio_format,omitempty"`
	OutputAudioType  string          `json:"output_audio_type,omitempty"`
	Tools            json.RawMessage `json:"tools,omitempty"`
}

// openAIRealtimeItem is the item object within an OpenAI Realtime event.
type openAIRealtimeItem struct {
	ID        string          `json:"id,omitempty"`
	Type      string          `json:"type,omitempty"`
	Role      string          `json:"role,omitempty"`
	Status    string          `json:"status,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	Name      string          `json:"name,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
	Output    string          `json:"output,omitempty"`
}

// openAIRealtimeError is the error object within an OpenAI Realtime event.
type openAIRealtimeError struct {
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Param   string `json:"param,omitempty"`
}

// ToGatewayRealtimeEvent converts an OpenAI Realtime event (raw JSON) to the unified Gateway format.
func (provider *OpenAIProvider) ToGatewayRealtimeEvent(providerEvent json.RawMessage) (*schemas.GatewayRealtimeEvent, error) {
	var raw openAIRealtimeEvent
	if err := json.Unmarshal(providerEvent, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OpenAI realtime event: %w", err)
	}

	event := &schemas.GatewayRealtimeEvent{
		Type:    schemas.RealtimeEventType(raw.Type),
		EventID: raw.EventID,
		RawData: providerEvent,
	}
	setRealtimeExtraParam(event, "item_id", raw.ItemID)
	setRealtimeExtraParam(event, "previous_item_id", raw.PreviousItemID)
	setRealtimeExtraParam(event, "output_index", raw.OutputIndex)
	setRealtimeExtraParam(event, "content_index", raw.ContentIndex)
	setRealtimeExtraParam(event, "response_id", raw.ResponseID)
	setRealtimeExtraParam(event, "audio_end_ms", raw.AudioEndMS)
	setRealtimeExtraParam(event, "transcript", raw.Transcript)
	setRealtimeExtraParam(event, "text", raw.Text)
	setRealtimeExtraParam(event, "conversation", raw.Conversation)
	setRealtimeExtraParam(event, "response", raw.Response)
	setRealtimeExtraParam(event, "part", raw.Part)

	switch {
	case raw.Session != nil:
		var sess openAIRealtimeSession
		if err := json.Unmarshal(raw.Session, &sess); err == nil {
			event.Session = &schemas.RealtimeSession{
				ID:               sess.ID,
				Model:            sess.Model,
				Modalities:       sess.Modalities,
				Instructions:     sess.Instructions,
				Voice:            sess.Voice,
				Temperature:      sess.Temperature,
				MaxOutputTokens:  sess.MaxOutputTokens,
				TurnDetection:    sess.TurnDetection,
				InputAudioFormat: sess.InputAudioFormat,
				OutputAudioType:  sess.OutputAudioType,
				Tools:            sess.Tools,
			}
			if extra := extractRealtimeNestedParams(raw.Session, "id", "model", "modalities", "instructions", "voice", "temperature", "max_output_tokens", "turn_detection", "input_audio_format", "output_audio_type", "tools"); len(extra) > 0 {
				event.Session.ExtraParams = extra
			}
		}
	case raw.Item != nil:
		var item openAIRealtimeItem
		if err := json.Unmarshal(raw.Item, &item); err == nil {
			event.Item = &schemas.RealtimeItem{
				ID:        item.ID,
				Type:      item.Type,
				Role:      item.Role,
				Status:    item.Status,
				Content:   item.Content,
				Name:      item.Name,
				CallID:    item.CallID,
				Arguments: item.Arguments,
				Output:    item.Output,
			}
			if extra := extractRealtimeNestedParams(raw.Item, "id", "type", "role", "status", "content", "name", "call_id", "arguments", "output"); len(extra) > 0 {
				event.Item.ExtraParams = extra
			}
		}

	case raw.Error != nil:
		var rtErr openAIRealtimeError
		if err := json.Unmarshal(raw.Error, &rtErr); err == nil {
			event.Error = &schemas.RealtimeError{
				Type:    rtErr.Type,
				Code:    rtErr.Code,
				Message: rtErr.Message,
				Param:   rtErr.Param,
			}
			if extra := extractRealtimeNestedParams(raw.Error, "type", "code", "message", "param"); len(extra) > 0 {
				event.Error.ExtraParams = extra
			}
		}
	}

	if isRealtimeDeltaEvent(raw.Type) {
		event.Delta = &schemas.RealtimeDelta{
			Text:       raw.Text,
			Audio:      raw.Audio,
			Transcript: raw.Transcript,
			ItemID:     raw.ItemID,
			OutputIdx:  raw.OutputIndex,
			ContentIdx: raw.ContentIndex,
			ResponseID: raw.ResponseID,
		}
		if raw.Delta != "" {
			if event.Delta.Text == "" {
				event.Delta.Text = raw.Delta
			}
		}
	}

	return event, nil
}

// ToProviderRealtimeEvent converts a unified Gateway Realtime event back to OpenAI's native JSON.
func (provider *OpenAIProvider) ToProviderRealtimeEvent(gatewayEvent *schemas.GatewayRealtimeEvent) (json.RawMessage, error) {
	out := map[string]interface{}{
		"type": string(gatewayEvent.Type),
	}
	if gatewayEvent.EventID != "" {
		out["event_id"] = gatewayEvent.EventID
	}
	mergeRealtimeExtraParams(out, gatewayEvent.ExtraParams)

	if gatewayEvent.Session != nil {
		sess := map[string]interface{}{}
		if gatewayEvent.Session.ID != "" && gatewayEvent.Type != schemas.RTEventSessionUpdate {
			sess["id"] = gatewayEvent.Session.ID
		}
		if gatewayEvent.Session.Model != "" {
			sess["model"] = gatewayEvent.Session.Model
		}
		if len(gatewayEvent.Session.Modalities) > 0 {
			sess["modalities"] = gatewayEvent.Session.Modalities
		}
		if gatewayEvent.Session.Instructions != "" {
			sess["instructions"] = gatewayEvent.Session.Instructions
		}
		if gatewayEvent.Session.Voice != "" {
			sess["voice"] = gatewayEvent.Session.Voice
		}
		if gatewayEvent.Session.Temperature != nil {
			sess["temperature"] = *gatewayEvent.Session.Temperature
		}
		if gatewayEvent.Session.MaxOutputTokens != nil {
			sess["max_output_tokens"] = gatewayEvent.Session.MaxOutputTokens
		}
		if gatewayEvent.Session.TurnDetection != nil {
			sess["turn_detection"] = gatewayEvent.Session.TurnDetection
		}
		if gatewayEvent.Session.InputAudioFormat != "" {
			sess["input_audio_format"] = gatewayEvent.Session.InputAudioFormat
		}
		if gatewayEvent.Session.OutputAudioType != "" {
			sess["output_audio_type"] = gatewayEvent.Session.OutputAudioType
		}
		if gatewayEvent.Session.Tools != nil {
			sess["tools"] = gatewayEvent.Session.Tools
		}
		mergeRealtimeSessionExtraParams(sess, gatewayEvent.Session.ExtraParams, gatewayEvent.Type)
		out["session"] = sess
	}

	if gatewayEvent.Item != nil {
		item := map[string]interface{}{
			"type": gatewayEvent.Item.Type,
		}
		if gatewayEvent.Item.ID != "" {
			item["id"] = gatewayEvent.Item.ID
		}
		if gatewayEvent.Item.Role != "" {
			item["role"] = gatewayEvent.Item.Role
		}
		if gatewayEvent.Item.Status != "" {
			item["status"] = gatewayEvent.Item.Status
		}
		if gatewayEvent.Item.Content != nil {
			item["content"] = gatewayEvent.Item.Content
		}
		if gatewayEvent.Item.Name != "" {
			item["name"] = gatewayEvent.Item.Name
		}
		if gatewayEvent.Item.CallID != "" {
			item["call_id"] = gatewayEvent.Item.CallID
		}
		if gatewayEvent.Item.Arguments != "" {
			item["arguments"] = gatewayEvent.Item.Arguments
		}
		if gatewayEvent.Item.Output != "" {
			item["output"] = gatewayEvent.Item.Output
		}
		mergeRealtimeExtraParams(item, gatewayEvent.Item.ExtraParams)
		out["item"] = item
	}

	if gatewayEvent.Error != nil {
		rtErr := map[string]interface{}{}
		if gatewayEvent.Error.Type != "" {
			rtErr["type"] = gatewayEvent.Error.Type
		}
		if gatewayEvent.Error.Code != "" {
			rtErr["code"] = gatewayEvent.Error.Code
		}
		if gatewayEvent.Error.Message != "" {
			rtErr["message"] = gatewayEvent.Error.Message
		}
		if gatewayEvent.Error.Param != "" {
			rtErr["param"] = gatewayEvent.Error.Param
		}
		mergeRealtimeExtraParams(rtErr, gatewayEvent.Error.ExtraParams)
		out["error"] = rtErr
	}

	if gatewayEvent.Delta != nil {
		if gatewayEvent.Delta.Text != "" {
			out["delta"] = gatewayEvent.Delta.Text
		}
		if gatewayEvent.Delta.Audio != "" {
			out["audio"] = gatewayEvent.Delta.Audio
		}
		if gatewayEvent.Delta.Transcript != "" {
			out["transcript"] = gatewayEvent.Delta.Transcript
		}
		if gatewayEvent.Delta.ItemID != "" && !hasRealtimeExtraParam(gatewayEvent.ExtraParams, "item_id") {
			out["item_id"] = gatewayEvent.Delta.ItemID
		}
		if gatewayEvent.Delta.OutputIdx != nil && !hasRealtimeExtraParam(gatewayEvent.ExtraParams, "output_index") {
			out["output_index"] = *gatewayEvent.Delta.OutputIdx
		}
		if gatewayEvent.Delta.ContentIdx != nil && !hasRealtimeExtraParam(gatewayEvent.ExtraParams, "content_index") {
			out["content_index"] = *gatewayEvent.Delta.ContentIdx
		}
		if gatewayEvent.Delta.ResponseID != "" && !hasRealtimeExtraParam(gatewayEvent.ExtraParams, "response_id") {
			out["response_id"] = gatewayEvent.Delta.ResponseID
		}
	}

	if len(gatewayEvent.Audio) > 0 && (gatewayEvent.Delta == nil || gatewayEvent.Delta.Audio == "") {
		out["audio"] = base64.StdEncoding.EncodeToString(gatewayEvent.Audio)
	}

	return providerUtils.MarshalSorted(out)
}

func mergeRealtimeSessionExtraParams(out map[string]interface{}, params map[string]json.RawMessage, eventType schemas.RealtimeEventType) {
	filtered := params
	if eventType == schemas.RTEventSessionUpdate && len(params) > 0 {
		filtered = make(map[string]json.RawMessage, len(params))
		for key, value := range params {
			switch key {
			case "id", "object", "expires_at", "client_secret":
				continue
			default:
				filtered[key] = value
			}
		}
	}
	mergeRealtimeExtraParams(out, filtered)
}

func (provider *OpenAIProvider) ExtractRealtimeTurnUsage(terminalEventRaw []byte) *schemas.GatewayLLMUsage {
	if len(terminalEventRaw) == 0 {
		return nil
	}

	var parsed openAIRealtimeResponseDoneEnvelope
	if err := json.Unmarshal(terminalEventRaw, &parsed); err != nil || parsed.Response.Usage == nil {
		return nil
	}

	usage := &schemas.GatewayLLMUsage{
		PromptTokens:     parsed.Response.Usage.InputTokens,
		CompletionTokens: parsed.Response.Usage.OutputTokens,
		TotalTokens:      parsed.Response.Usage.TotalTokens,
	}

	if parsed.Response.Usage.InputTokenDetails != nil {
		usage.PromptTokensDetails = &schemas.ChatPromptTokensDetails{
			TextTokens:       parsed.Response.Usage.InputTokenDetails.TextTokens,
			AudioTokens:      parsed.Response.Usage.InputTokenDetails.AudioTokens,
			ImageTokens:      parsed.Response.Usage.InputTokenDetails.ImageTokens,
			CachedReadTokens: parsed.Response.Usage.InputTokenDetails.CachedTokens,
		}
	}

	if parsed.Response.Usage.OutputTokenDetails != nil {
		usage.CompletionTokensDetails = &schemas.ChatCompletionTokensDetails{
			TextTokens:               parsed.Response.Usage.OutputTokenDetails.TextTokens,
			AudioTokens:              parsed.Response.Usage.OutputTokenDetails.AudioTokens,
			ReasoningTokens:          parsed.Response.Usage.OutputTokenDetails.ReasoningTokens,
			ImageTokens:              parsed.Response.Usage.OutputTokenDetails.ImageTokens,
			CitationTokens:           parsed.Response.Usage.OutputTokenDetails.CitationTokens,
			NumSearchQueries:         parsed.Response.Usage.OutputTokenDetails.NumSearchQueries,
			AcceptedPredictionTokens: parsed.Response.Usage.OutputTokenDetails.AcceptedPredictionTokens,
			RejectedPredictionTokens: parsed.Response.Usage.OutputTokenDetails.RejectedPredictionTokens,
		}
	}

	return usage
}

func (provider *OpenAIProvider) ExtractRealtimeTurnOutput(terminalEventRaw []byte) *schemas.ChatMessage {
	if len(terminalEventRaw) == 0 {
		return nil
	}

	var parsed openAIRealtimeResponseDoneEnvelope
	if err := json.Unmarshal(terminalEventRaw, &parsed); err != nil {
		return nil
	}

	content := extractOpenAIRealtimeResponseDoneAssistantText(parsed.Response.Output)
	toolCalls := extractOpenAIRealtimeResponseDoneToolCalls(parsed.Response.Output)
	if content == "" && len(toolCalls) == 0 {
		return nil
	}

	message := &schemas.ChatMessage{Role: schemas.ChatMessageRoleAssistant}
	if content != "" {
		message.Content = &schemas.ChatMessageContent{ContentStr: schemas.Ptr(content)}
	}
	if len(toolCalls) > 0 {
		message.ChatAssistantMessage = &schemas.ChatAssistantMessage{ToolCalls: toolCalls}
	}

	return message
}

type openAIRealtimeResponseDoneEnvelope struct {
	Response struct {
		Output []openAIRealtimeResponseDoneOutput `json:"output"`
		Usage  *openAIRealtimeResponseDoneUsage   `json:"usage"`
	} `json:"response"`
}

type openAIRealtimeResponseDoneOutput struct {
	ID        string                            `json:"id"`
	Type      string                            `json:"type"`
	Name      string                            `json:"name"`
	CallID    string                            `json:"call_id"`
	Arguments string                            `json:"arguments"`
	Content   []openAIRealtimeResponseDoneBlock `json:"content"`
}

type openAIRealtimeResponseDoneBlock struct {
	Text       string `json:"text"`
	Transcript string `json:"transcript"`
	Refusal    string `json:"refusal"`
}

type openAIRealtimeResponseDoneUsage struct {
	TotalTokens        int                                         `json:"total_tokens"`
	InputTokens        int                                         `json:"input_tokens"`
	OutputTokens       int                                         `json:"output_tokens"`
	InputTokenDetails  *openAIRealtimeResponseDoneInputTokenUsage  `json:"input_token_details"`
	OutputTokenDetails *openAIRealtimeResponseDoneOutputTokenUsage `json:"output_token_details"`
}

type openAIRealtimeResponseDoneInputTokenUsage struct {
	TextTokens   int `json:"text_tokens"`
	AudioTokens  int `json:"audio_tokens"`
	ImageTokens  int `json:"image_tokens"`
	CachedTokens int `json:"cached_tokens"`
}

type openAIRealtimeResponseDoneOutputTokenUsage struct {
	TextTokens               int  `json:"text_tokens"`
	AudioTokens              int  `json:"audio_tokens"`
	ReasoningTokens          int  `json:"reasoning_tokens"`
	ImageTokens              *int `json:"image_tokens"`
	CitationTokens           *int `json:"citation_tokens"`
	NumSearchQueries         *int `json:"num_search_queries"`
	AcceptedPredictionTokens int  `json:"accepted_prediction_tokens"`
	RejectedPredictionTokens int  `json:"rejected_prediction_tokens"`
}

func extractOpenAIRealtimeResponseDoneAssistantText(outputs []openAIRealtimeResponseDoneOutput) string {
	var sb strings.Builder
	for _, output := range outputs {
		if output.Type != "message" {
			continue
		}
		for _, block := range output.Content {
			switch {
			case strings.TrimSpace(block.Text) != "":
				sb.WriteString(block.Text)
			case strings.TrimSpace(block.Transcript) != "":
				sb.WriteString(block.Transcript)
			case strings.TrimSpace(block.Refusal) != "":
				sb.WriteString(block.Refusal)
			}
		}
	}
	return strings.TrimSpace(sb.String())
}

func extractOpenAIRealtimeResponseDoneToolCalls(outputs []openAIRealtimeResponseDoneOutput) []schemas.ChatAssistantMessageToolCall {
	toolCalls := make([]schemas.ChatAssistantMessageToolCall, 0)
	for _, output := range outputs {
		if output.Type != "function_call" {
			continue
		}

		name := strings.TrimSpace(output.Name)
		if name == "" {
			continue
		}

		toolType := "function"
		id := strings.TrimSpace(output.CallID)
		if id == "" {
			id = strings.TrimSpace(output.ID)
		}

		toolCall := schemas.ChatAssistantMessageToolCall{
			Index: uint16(len(toolCalls)),
			Type:  &toolType,
			Function: schemas.ChatAssistantMessageToolCallFunction{
				Name:      schemas.Ptr(name),
				Arguments: output.Arguments,
			},
		}
		if id != "" {
			toolCall.ID = schemas.Ptr(id)
		}

		toolCalls = append(toolCalls, toolCall)
	}
	return toolCalls
}

func setRealtimeExtraParam(event *schemas.GatewayRealtimeEvent, key string, value any) {
	if event == nil || key == "" || value == nil {
		return
	}

	switch v := value.(type) {
	case string:
		if v == "" {
			return
		}
	case *int:
		if v == nil {
			return
		}
	case json.RawMessage:
		if len(v) == 0 || string(v) == "null" {
			return
		}
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	if event.ExtraParams == nil {
		event.ExtraParams = make(map[string]json.RawMessage)
	}
	event.ExtraParams[key] = raw
}

func mergeRealtimeExtraParams(out map[string]interface{}, params map[string]json.RawMessage) {
	for key, raw := range params {
		if len(raw) == 0 {
			continue
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			continue
		}
		out[key] = value
	}
}

func hasRealtimeExtraParam(params map[string]json.RawMessage, key string) bool {
	if params == nil {
		return false
	}
	raw, ok := params[key]
	return ok && len(raw) > 0
}

func extractRealtimeNestedParams(raw json.RawMessage, knownKeys ...string) map[string]json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	root := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil
	}
	for _, key := range knownKeys {
		delete(root, key)
	}
	if len(root) == 0 {
		return nil
	}
	return root
}

func isRealtimeDeltaEvent(eventType string) bool {
	switch eventType {
	case "response.text.delta",
		"response.output_text.delta",
		"response.audio.delta",
		"response.output_audio.delta",
		"response.audio_transcript.delta",
		"response.output_audio_transcript.delta",
		"conversation.item.input_audio_transcription.delta":
		return true
	}
	return false
}

// ExtractNestedVoice digs into the new session.audio.output.voice path.
func ExtractNestedVoice(audioRaw json.RawMessage) string {
	var audio struct {
		Output struct {
			Voice string `json:"voice"`
		} `json:"output"`
	}
	if err := json.Unmarshal(audioRaw, &audio); err == nil && audio.Output.Voice != "" {
		return audio.Output.Voice
	}
	return ""
}
