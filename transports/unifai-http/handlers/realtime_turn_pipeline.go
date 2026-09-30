package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	unifai "github.com/unifai/unifai/core"
	openaiProvider "github.com/unifai/unifai/core/providers/openai"
	"github.com/unifai/unifai/core/schemas"
	ufws "github.com/unifai/unifai/transports/unifai-http/websocket"
)

func newRealtimeTurnContext(
	baseCtx *schemas.UnifAIContext,
	requestID string,
	sessionID string,
	providerSessionID string,
	source realtimeTurnSource,
	eventType schemas.RealtimeEventType,
	key *schemas.Key,
) *schemas.UnifAIContext {
	ctx := schemas.NewUnifAIContext(context.Background(), schemas.NoDeadline)
	if baseCtx != nil {
		// Realtime post-hook contexts must preserve plugin-private values written in
		// pre-hooks (for example telemetry start timestamps), not just public keys.
		for ctxKey, value := range baseCtx.GetUserValues() {
			if value == nil {
				continue
			}
			// Never inherit a session/transport-level trace ID. Each realtime turn
			// must mint its own trace in RunRealtimeTurnPreHooks so its log entry is
			// delivered when the turn's trace is completed and flushed. Inheriting a
			// trace whose lifecycle is owned elsewhere strands the entry forever.
			if ctxKey == schemas.UnifAIContextKeyTraceID {
				continue
			}
			ctx.SetValue(ctxKey, value)
		}
	}

	ctx.SetValue(schemas.UnifAIContextKeyHTTPRequestType, schemas.RealtimeRequest)
	if requestID == "" {
		requestID = uuid.NewString()
	}
	ctx.SetValue(schemas.UnifAIContextKeyRequestID, requestID)
	resolvedSessionID := strings.TrimSpace(providerSessionID)
	if resolvedSessionID == "" {
		resolvedSessionID = strings.TrimSpace(sessionID)
	}
	if baseCtx != nil {
		if externalSessionID, ok := baseCtx.Value(schemas.UnifAIContextKeyParentRequestID).(string); ok && strings.TrimSpace(externalSessionID) != "" {
			resolvedSessionID = strings.TrimSpace(externalSessionID)
		}
	}
	if resolvedSessionID != "" {
		ctx.SetValue(schemas.UnifAIContextKeyParentRequestID, resolvedSessionID)
	}
	if strings.TrimSpace(providerSessionID) != "" {
		ctx.SetValue(schemas.UnifAIContextKeyRealtimeSessionID, providerSessionID)
		ctx.SetValue(schemas.UnifAIContextKeyRealtimeProviderSessionID, providerSessionID)
	}
	if source != "" {
		ctx.SetValue(schemas.UnifAIContextKeyRealtimeSource, string(source))
	}
	if eventType != "" {
		ctx.SetValue(schemas.UnifAIContextKeyRealtimeEventType, string(eventType))
	}
	if key != nil {
		if strings.TrimSpace(key.ID) != "" {
			ctx.SetValue(schemas.UnifAIContextKeySelectedKeyID, key.ID)
		}
		if strings.TrimSpace(key.Name) != "" {
			ctx.SetValue(schemas.UnifAIContextKeySelectedKeyName, key.Name)
		}
	}
	return ctx
}

func applyRealtimeRawStorageContext(ctx *schemas.UnifAIContext, storeRaw bool) {
	if ctx == nil {
		return
	}
	// Realtime turn logging captures raw payloads only for log storage. There is
	// no client-facing raw send-back path for synthetic realtime turn responses.
	sendBackRawRequest := false
	sendBackRawResponse := false
	ctx.SetValue(schemas.UnifAIContextKeyShouldStoreRawInLogs, storeRaw)
	ctx.SetValue(schemas.UnifAIContextKeyCaptureRawRequest, storeRaw || sendBackRawRequest)
	ctx.SetValue(schemas.UnifAIContextKeyCaptureRawResponse, storeRaw || sendBackRawResponse)
	ctx.SetValue(schemas.UnifAIContextKeyDropRawRequestFromClient, storeRaw && !sendBackRawRequest)
	ctx.SetValue(schemas.UnifAIContextKeyDropRawResponseFromClient, storeRaw && !sendBackRawResponse)
}

func shouldStoreRealtimeRawPayloads(ctx *schemas.UnifAIContext) bool {
	if ctx == nil {
		return false
	}
	storeRaw, _ := ctx.Value(schemas.UnifAIContextKeyShouldStoreRawInLogs).(bool)
	return storeRaw
}

func applyRealtimeTurnContextValues(ctx *schemas.UnifAIContext, values map[any]any) {
	if ctx == nil || len(values) == 0 {
		return
	}
	for ctxKey, value := range values {
		switch ctxKey {
		case schemas.UnifAIContextKeyRequestID,
			schemas.UnifAIContextKeyParentRequestID,
			schemas.UnifAIContextKeyRealtimeSessionID,
			schemas.UnifAIContextKeyRealtimeProviderSessionID,
			schemas.UnifAIContextKeyRealtimeSource,
			schemas.UnifAIContextKeyRealtimeEventType,
			schemas.UnifAIContextKeyStreamStartTime,
			schemas.UnifAIContextKeyStreamEndIndicator:
			continue
		}
		if value != nil {
			ctx.SetValue(ctxKey, value)
		}
	}
}

func restoreRealtimeTurnTraceContext(ctx *schemas.UnifAIContext, traceID string, values map[any]any) {
	if ctx == nil {
		return
	}
	if strings.TrimSpace(traceID) != "" {
		ctx.SetValue(schemas.UnifAIContextKeyTraceID, strings.TrimSpace(traceID))
	}
	if tracer, ok := values[schemas.UnifAIContextKeyTracer].(schemas.Tracer); ok && tracer != nil {
		ctx.SetValue(schemas.UnifAIContextKeyTracer, tracer)
	}
}

func setRealtimeTurnStreamContext(ctx *schemas.UnifAIContext, startedAt time.Time, isFinal bool) {
	if ctx == nil {
		return
	}
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	ctx.SetValue(schemas.UnifAIContextKeyStreamStartTime, startedAt)
	if isFinal {
		ctx.SetValue(schemas.UnifAIContextKeyStreamEndIndicator, true)
	}
}

// sanitizeRealtimeSessionEventForProvider mutates outbound session events before provider
// serialization. It must not persist session state; rejected session.update events should
// not affect later turn logs.
func sanitizeRealtimeSessionEventForProvider(event *schemas.UnifAIRealtimeEvent) {
	if event == nil || event.Session == nil {
		return
	}
	switch event.Type {
	case schemas.RTEventSessionUpdate,
		schemas.RTEventSessionCreated,
		schemas.RTEventSessionUpdated:
		if event.Session.ExtraParams != nil {
			openaiProvider.StripNestedModelPrefixes(event.Session.ExtraParams)
		}
	}
}

// updateRealtimeSessionFromEvent updates the session's tracked tool
// definitions and voice whenever a session.update, session.created, or
// session.updated event carries them.
func updateRealtimeSessionFromEvent(session *ufws.Session, event *schemas.UnifAIRealtimeEvent) {
	if event == nil || event.Session == nil {
		return
	}
	switch event.Type {
	case schemas.RTEventSessionUpdate,
		schemas.RTEventSessionCreated,
		schemas.RTEventSessionUpdated:
		// Only update if the event explicitly carries tools (even an empty array
		// means "clear tools"). A nil/absent tools field means "not changed".
		if event.Session.Tools != nil {
			session.SetRealtimeSessionTools(event.Session.Tools)
		}
		if event.Session.Voice != "" {
			session.SetRealtimeVoice(event.Session.Voice)
		} else if audioRaw, ok := event.Session.ExtraParams["audio"]; ok {
			// New API format nests voice under session.audio.output.voice
			// instead of the legacy top-level session.voice.
			if voice := openaiProvider.ExtractNestedVoice(audioRaw); voice != "" {
				session.SetRealtimeVoice(voice)
			}
		}
	}
}

func buildRealtimeTurnPreRequest(provider schemas.ModelProvider, model string, turnInputs []ufws.RealtimeTurnInput, sessionTools json.RawMessage) *schemas.UnifAIRequest {
	input := make([]schemas.ResponsesMessage, 0, len(turnInputs))
	for _, turnInput := range turnInputs {
		summary := strings.TrimSpace(turnInput.Summary)
		if summary == "" {
			continue
		}
		switch turnInput.Role {
		case string(schemas.ChatMessageRoleTool):
			itemType := schemas.ResponsesMessageTypeFunctionCallOutput
			output := &schemas.ResponsesToolMessageOutputStruct{
				ResponsesToolCallOutputStr: schemas.Ptr(summary),
			}
			input = append(input, schemas.ResponsesMessage{
				Type:                 &itemType,
				ResponsesToolMessage: &schemas.ResponsesToolMessage{Output: output},
			})
		case string(schemas.ChatMessageRoleUser):
			itemType := schemas.ResponsesMessageTypeMessage
			role := schemas.ResponsesInputMessageRoleUser
			input = append(input, schemas.ResponsesMessage{
				Type:    &itemType,
				Role:    &role,
				Content: &schemas.ResponsesMessageContent{ContentStr: schemas.Ptr(summary)},
			})
		}
	}

	var params *schemas.ResponsesParameters
	if len(sessionTools) > 0 {
		var tools []schemas.ResponsesTool
		if json.Unmarshal(sessionTools, &tools) == nil && len(tools) > 0 {
			params = &schemas.ResponsesParameters{Tools: tools}
		}
	}

	return &schemas.UnifAIRequest{
		RequestType: schemas.RealtimeRequest,
		ResponsesRequest: &schemas.UnifAIResponsesRequest{
			Provider: provider,
			Model:    model,
			Input:    input,
			Params:   params,
		},
	}
}

func buildRealtimeTurnPostResponse(
	rtProvider schemas.RealtimeProvider,
	provider schemas.ModelProvider,
	model string,
	rawRequest string,
	rawResponse []byte,
	contentOverride string,
	latency int64,
) *schemas.UnifAIResponse {
	output := buildRealtimeTurnOutputMessages(rtProvider, rawResponse, contentOverride)
	resp := &schemas.UnifAIResponsesResponse{
		Object: "response",
		Model:  model,
		Output: output,
		ExtraFields: schemas.UnifAIResponseExtraFields{
			RequestType:            schemas.RealtimeRequest,
			Provider:               provider,
			OriginalModelRequested: model,
			Latency:                latency,
		},
	}
	if usage := extractRealtimeTurnUsage(rtProvider, rawResponse); usage != nil {
		resp.Usage = buildRealtimeResponsesUsage(usage)
	}
	if strings.TrimSpace(rawRequest) != "" {
		resp.ExtraFields.RawRequest = rawRequest
	}
	if len(rawResponse) > 0 {
		resp.ExtraFields.RawResponse = string(rawResponse)
	}

	return &schemas.UnifAIResponse{ResponsesResponse: resp}
}

func buildRealtimeTurnOutputMessages(rtProvider schemas.RealtimeProvider, rawResponse []byte, contentOverride string) []schemas.ResponsesMessage {
	outputs := make([]schemas.ResponsesMessage, 0)
	seenFunctionCalls := make(map[string]struct{})
	if outputMessage := extractRealtimeTurnOutputMessage(rtProvider, rawResponse, contentOverride); outputMessage != nil {
		outputs = append(outputs, buildRealtimeResponsesMessagesFromChat(outputMessage, contentOverride)...)
		for _, output := range outputs {
			if output.Type == nil || *output.Type != schemas.ResponsesMessageTypeFunctionCall {
				continue
			}
			seenFunctionCalls[realtimeResponsesFunctionCallKey(output)] = struct{}{}
		}
	}

	var parsed realtimeResponseDoneEnvelope
	if len(rawResponse) > 0 && schemas.Unmarshal(rawResponse, &parsed) == nil {
		for _, item := range parsed.Response.Output {
			switch item.Type {
			case "message":
				if realtimeOutputsContainMessage(outputs) {
					continue
				}
				content := strings.TrimSpace(contentOverride)
				if content == "" {
					content = extractRealtimeResponseDoneContentText(item.Content)
				}
				itemType := schemas.ResponsesMessageTypeMessage
				role := schemas.ResponsesInputMessageRoleAssistant
				msg := schemas.ResponsesMessage{
					Type:   &itemType,
					Role:   &role,
					Status: schemas.Ptr("completed"),
				}
				if strings.TrimSpace(item.ID) != "" {
					msg.ID = schemas.Ptr(strings.TrimSpace(item.ID))
				}
				if content != "" {
					msg.Content = &schemas.ResponsesMessageContent{ContentStr: schemas.Ptr(content)}
				}
				outputs = append(outputs, msg)
			case "function_call":
				itemType := schemas.ResponsesMessageTypeFunctionCall
				msg := schemas.ResponsesMessage{
					Type:   &itemType,
					Status: schemas.Ptr("completed"),
					ResponsesToolMessage: &schemas.ResponsesToolMessage{
						Name:      schemas.Ptr(strings.TrimSpace(item.Name)),
						Arguments: schemas.Ptr(item.Arguments),
					},
				}
				if strings.TrimSpace(item.ID) != "" {
					msg.ID = schemas.Ptr(strings.TrimSpace(item.ID))
				}
				if strings.TrimSpace(item.CallID) != "" {
					msg.CallID = schemas.Ptr(strings.TrimSpace(item.CallID))
				}
				key := realtimeResponsesFunctionCallKey(msg)
				if _, exists := seenFunctionCalls[key]; exists {
					continue
				}
				seenFunctionCalls[key] = struct{}{}
				outputs = append(outputs, msg)
			}
		}
	}

	if len(outputs) == 0 && strings.TrimSpace(contentOverride) != "" {
		itemType := schemas.ResponsesMessageTypeMessage
		role := schemas.ResponsesInputMessageRoleAssistant
		outputs = append(outputs, schemas.ResponsesMessage{
			Type:    &itemType,
			Role:    &role,
			Status:  schemas.Ptr("completed"),
			Content: &schemas.ResponsesMessageContent{ContentStr: schemas.Ptr(strings.TrimSpace(contentOverride))},
		})
	}

	return outputs
}

func realtimeOutputsContainMessage(outputs []schemas.ResponsesMessage) bool {
	for _, output := range outputs {
		if output.Type != nil && *output.Type == schemas.ResponsesMessageTypeMessage {
			return true
		}
	}
	return false
}

func realtimeResponsesFunctionCallKey(message schemas.ResponsesMessage) string {
	if message.CallID != nil && strings.TrimSpace(*message.CallID) != "" {
		return "call_id:" + strings.TrimSpace(*message.CallID)
	}
	if message.ID != nil && strings.TrimSpace(*message.ID) != "" {
		return "id:" + strings.TrimSpace(*message.ID)
	}

	var parts []string
	if message.ResponsesToolMessage != nil {
		if message.ResponsesToolMessage.Name != nil {
			parts = append(parts, strings.TrimSpace(*message.ResponsesToolMessage.Name))
		}
		if message.ResponsesToolMessage.Arguments != nil {
			parts = append(parts, strings.TrimSpace(*message.ResponsesToolMessage.Arguments))
		}
	}
	return strings.Join(parts, "\x00")
}

func buildRealtimeResponsesMessagesFromChat(message *schemas.ChatMessage, contentOverride string) []schemas.ResponsesMessage {
	if message == nil {
		return nil
	}

	outputs := make([]schemas.ResponsesMessage, 0, 1)
	content := strings.TrimSpace(contentOverride)
	if content == "" && message.Content != nil && message.Content.ContentStr != nil {
		content = strings.TrimSpace(*message.Content.ContentStr)
	}
	if content != "" {
		itemType := schemas.ResponsesMessageTypeMessage
		role := schemas.ResponsesInputMessageRoleAssistant
		outputs = append(outputs, schemas.ResponsesMessage{
			Type:    &itemType,
			Role:    &role,
			Status:  schemas.Ptr("completed"),
			Content: &schemas.ResponsesMessageContent{ContentStr: schemas.Ptr(content)},
		})
	}

	if message.ChatAssistantMessage == nil {
		return outputs
	}

	for _, toolCall := range message.ChatAssistantMessage.ToolCalls {
		itemType := schemas.ResponsesMessageTypeFunctionCall
		msg := schemas.ResponsesMessage{
			Type:   &itemType,
			Status: schemas.Ptr("completed"),
			ResponsesToolMessage: &schemas.ResponsesToolMessage{
				Arguments: schemas.Ptr(toolCall.Function.Arguments),
			},
		}
		if toolCall.Function.Name != nil {
			msg.ResponsesToolMessage.Name = schemas.Ptr(strings.TrimSpace(*toolCall.Function.Name))
		}
		if toolCall.ID != nil {
			msg.CallID = schemas.Ptr(strings.TrimSpace(*toolCall.ID))
			msg.ID = schemas.Ptr(strings.TrimSpace(*toolCall.ID))
		}
		outputs = append(outputs, msg)
	}

	return outputs
}

func extractRealtimeResponseDoneContentText(content []realtimeResponseDoneContent) string {
	for _, block := range content {
		switch {
		case strings.TrimSpace(block.Text) != "":
			return strings.TrimSpace(block.Text)
		case strings.TrimSpace(block.Transcript) != "":
			return strings.TrimSpace(block.Transcript)
		case strings.TrimSpace(block.Refusal) != "":
			return strings.TrimSpace(block.Refusal)
		}
	}
	return ""
}

func buildRealtimeResponsesUsage(usage *schemas.UnifAILLMUsage) *schemas.ResponsesResponseUsage {
	if usage == nil {
		return nil
	}
	result := &schemas.ResponsesResponseUsage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
		TotalTokens:  usage.TotalTokens,
	}
	if usage.PromptTokensDetails != nil {
		result.InputTokensDetails = &schemas.ResponsesResponseInputTokens{
			TextTokens:        usage.PromptTokensDetails.TextTokens,
			AudioTokens:       usage.PromptTokensDetails.AudioTokens,
			ImageTokens:       usage.PromptTokensDetails.ImageTokens,
			CachedReadTokens:  usage.PromptTokensDetails.CachedReadTokens,
			CachedWriteTokens: usage.PromptTokensDetails.CachedWriteTokens,
		}
	}
	if usage.CompletionTokensDetails != nil {
		result.OutputTokensDetails = &schemas.ResponsesResponseOutputTokens{
			TextTokens:               usage.CompletionTokensDetails.TextTokens,
			AcceptedPredictionTokens: usage.CompletionTokensDetails.AcceptedPredictionTokens,
			AudioTokens:              usage.CompletionTokensDetails.AudioTokens,
			ImageTokens:              usage.CompletionTokensDetails.ImageTokens,
			ReasoningTokens:          usage.CompletionTokensDetails.ReasoningTokens,
			RejectedPredictionTokens: usage.CompletionTokensDetails.RejectedPredictionTokens,
			CitationTokens:           usage.CompletionTokensDetails.CitationTokens,
			NumSearchQueries:         usage.CompletionTokensDetails.NumSearchQueries,
		}
	}
	return result
}

func newRealtimeTurnErrorEventPayload(unifaiErr *schemas.UnifAIError) []byte {
	if unifaiErr == nil {
		return []byte(`{"type":"error","error":{"type":"server_error","message":"internal server error"}}`)
	}

	errorType, errorCode, errorMessage, errorParam := mapRealtimeWireErrorFields(unifaiErr)
	payload := schemas.UnifAIRealtimeEvent{
		Type: schemas.RTEventError,
		Error: &schemas.RealtimeError{
			Type:    errorType,
			Code:    errorCode,
			Message: errorMessage,
			Param:   errorParam,
		},
	}
	if data, err := schemas.Marshal(payload); err == nil {
		return data
	}
	return []byte(`{"type":"error","error":{"type":"server_error","message":"internal server error"}}`)
}

// isBudgetOrBillingError returns true if the lowercased value indicates a budget or billing exhaustion error.
// Quota/rate-limit patterns (quota_exceeded, quota exceeded, etc.) are already covered by unifai.IsRateLimitErrorMessage.
func isBudgetOrBillingError(lower string) bool {
	return strings.Contains(lower, "budget_exceeded") ||
		strings.Contains(lower, "budget exceeded") ||
		strings.Contains(lower, "insufficient_quota") ||
		strings.Contains(lower, "hard limit reached") ||
		strings.Contains(lower, "billing hard limit")
}

func mapRealtimeWireErrorFields(unifaiErr *schemas.UnifAIError) (string, string, string, string) {
	errorType := "server_error"
	errorCode := "server_error"
	errorMessage := "internal server error"
	errorParam := ""

	if unifaiErr == nil {
		return errorType, errorCode, errorMessage, errorParam
	}

	var values []string
	if unifaiErr.Type != nil {
		values = append(values, strings.TrimSpace(*unifaiErr.Type))
	}
	if unifaiErr.Error != nil {
		if unifaiErr.Error.Type != nil {
			values = append(values, strings.TrimSpace(*unifaiErr.Error.Type))
		}
		if unifaiErr.Error.Code != nil {
			values = append(values, strings.TrimSpace(*unifaiErr.Error.Code))
		}
		if strings.TrimSpace(unifaiErr.Error.Message) != "" {
			errorMessage = strings.TrimSpace(unifaiErr.Error.Message)
			values = append(values, errorMessage)
		}
		if unifaiErr.Error.Param != nil {
			errorParam = strings.TrimSpace(fmt.Sprint(unifaiErr.Error.Param))
		}
	}

	for _, value := range values {
		lower := strings.ToLower(value)
		switch {
		case lower == "":
			continue
		case strings.Contains(lower, "invalid_request_error"):
			return "invalid_request_error", "invalid_request_error", errorMessage, errorParam
		case isBudgetOrBillingError(lower):
			return "insufficient_quota", "insufficient_quota", errorMessage, errorParam
		case unifai.IsRateLimitErrorMessage(lower):
			return "rate_limit_exceeded", "rate_limit_exceeded", errorMessage, errorParam
		}
	}

	return errorType, errorCode, errorMessage, errorParam
}

func shouldGracefullyDisconnectRealtime(unifaiErr *schemas.UnifAIError) bool {
	if unifaiErr == nil {
		return false
	}

	var values []string
	if unifaiErr.Type != nil {
		values = append(values, strings.TrimSpace(*unifaiErr.Type))
	}
	if unifaiErr.Error != nil {
		if unifaiErr.Error.Type != nil {
			values = append(values, strings.TrimSpace(*unifaiErr.Error.Type))
		}
		if unifaiErr.Error.Code != nil {
			values = append(values, strings.TrimSpace(*unifaiErr.Error.Code))
		}
		values = append(values, strings.TrimSpace(unifaiErr.Error.Message))
	}

	for _, value := range values {
		lower := strings.ToLower(value)
		if lower == "" {
			continue
		}
		if isBudgetOrBillingError(lower) || unifai.IsRateLimitErrorMessage(lower) {
			return true
		}
	}

	return false
}

func startRealtimeTurnHooks(
	client *unifai.UnifAI,
	baseCtx *schemas.UnifAIContext,
	session *ufws.Session,
	rtProvider schemas.RealtimeProvider,
	provider schemas.ModelProvider,
	model string,
	key *schemas.Key,
	startEventType schemas.RealtimeEventType,
) *schemas.UnifAIError {
	if client == nil || session == nil {
		return &schemas.UnifAIError{
			Type:       schemas.Ptr("server_error"),
			StatusCode: schemas.Ptr(500),
			Error: &schemas.ErrorField{
				Type:    schemas.Ptr("server_error"),
				Message: "realtime turn pipeline is unavailable",
			},
		}
	}
	if !session.TryBeginRealtimeTurnHooks() {
		return &schemas.UnifAIError{
			Type:       schemas.Ptr("invalid_request_error"),
			StatusCode: schemas.Ptr(400),
			Error: &schemas.ErrorField{
				Type:    schemas.Ptr("invalid_request_error"),
				Message: "Conversation already has an active response in progress.",
			},
		}
	}
	committed := false
	defer func() {
		if !committed {
			session.AbortRealtimeTurnHooks()
		}
	}()

	startedAt := time.Now()
	storeRaw := shouldStoreRealtimeRawPayloads(baseCtx)
	turnCtx := newRealtimeTurnContext(baseCtx, "", session.ID(), session.ProviderSessionID(), realtimeTurnSourceEI, startEventType, key)
	applyRealtimeRawStorageContext(turnCtx, storeRaw)
	if voice := session.RealtimeVoice(); voice != "" {
		turnCtx.SetValue(schemas.UnifAIContextKeyRealtimeVoice, voice)
	}
	setRealtimeTurnStreamContext(turnCtx, startedAt, false)
	req := buildRealtimeTurnPreRequest(provider, model, session.PeekRealtimeTurnInputs(), session.RealtimeSessionTools())
	hooks, unifaiErr := client.RunRealtimeTurnPreHooks(turnCtx, req)
	if unifaiErr != nil {
		// RunRealtimeTurnPreHooks already executed post-hooks and flushed the trace
		// for this turn-start failure. Clear buffered turn state so transport-close
		// fallback finalization does not emit the same error a second time.
		session.ConsumeRealtimeTurnInputs()
		session.ConsumeRealtimeOutputText()
		return unifaiErr
	}

	requestID, _ := turnCtx.Value(schemas.UnifAIContextKeyRequestID).(string)
	traceID, _ := turnCtx.Value(schemas.UnifAIContextKeyTraceID).(string)
	session.SetRealtimeTurnHooks(&ufws.RealtimeTurnPluginState{
		PostHookRunner: hooks.PostHookRunner,
		Cleanup:        hooks.Cleanup,
		RequestID:      requestID,
		StartedAt:      startedAt,
		PreHookValues:  turnCtx.GetUserValues(),
		TraceID:        traceID,
		RawStore:       storeRaw,
	})
	committed = true
	return nil
}

func finalizeRealtimeTurnHooks(
	client *unifai.UnifAI,
	baseCtx *schemas.UnifAIContext,
	session *ufws.Session,
	rtProvider schemas.RealtimeProvider,
	provider schemas.ModelProvider,
	model string,
	key *schemas.Key,
	rawResponse []byte,
	contentOverride string,
) *schemas.UnifAIError {
	if client == nil || session == nil {
		return nil
	}

	turnInputs := session.ConsumeRealtimeTurnInputs()
	rawRequest := combineRealtimeInputRaw(turnInputs)

	if activeHooks := session.ConsumeRealtimeTurnHooks(); activeHooks != nil {
		defer func() {
			if activeHooks.Cleanup != nil {
				activeHooks.Cleanup()
			}
		}()
		postResponse := buildRealtimeTurnPostResponse(
			rtProvider,
			provider,
			model,
			rawRequest,
			rawResponse,
			contentOverride,
			time.Since(activeHooks.StartedAt).Milliseconds(),
		)
		postCtx := newRealtimeTurnContext(baseCtx, activeHooks.RequestID, session.ID(), session.ProviderSessionID(), realtimeTurnSourceLM, rtProvider.RealtimeTurnFinalEvent(), key)
		applyRealtimeTurnContextValues(postCtx, activeHooks.PreHookValues)
		restoreRealtimeTurnTraceContext(postCtx, activeHooks.TraceID, activeHooks.PreHookValues)
		applyRealtimeRawStorageContext(postCtx, activeHooks.RawStore)
		setRealtimeTurnStreamContext(postCtx, activeHooks.StartedAt, true)
		_, unifaiErr := activeHooks.PostHookRunner(postCtx, postResponse, nil)
		completeRealtimeTurnTrace(postCtx)
		return unifaiErr
	}

	startedAt := time.Now()
	storeRaw := shouldStoreRealtimeRawPayloads(baseCtx)
	preCtx := newRealtimeTurnContext(baseCtx, "", session.ID(), session.ProviderSessionID(), realtimeTurnSourceEI, "", key)
	applyRealtimeRawStorageContext(preCtx, storeRaw)
	setRealtimeTurnStreamContext(preCtx, startedAt, false)
	preReq := buildRealtimeTurnPreRequest(provider, model, turnInputs, session.RealtimeSessionTools())
	hooks, unifaiErr := client.RunRealtimeTurnPreHooks(preCtx, preReq)
	if unifaiErr != nil {
		return unifaiErr
	}
	preHookValues := preCtx.GetUserValues()
	if hooks.Cleanup != nil {
		defer hooks.Cleanup()
	}

	requestID, _ := preCtx.Value(schemas.UnifAIContextKeyRequestID).(string)
	traceID, _ := preCtx.Value(schemas.UnifAIContextKeyTraceID).(string)
	postResponse := buildRealtimeTurnPostResponse(
		rtProvider,
		provider,
		model,
		rawRequest,
		rawResponse,
		contentOverride,
		time.Since(startedAt).Milliseconds(),
	)
	postCtx := newRealtimeTurnContext(baseCtx, requestID, session.ID(), session.ProviderSessionID(), realtimeTurnSourceLM, rtProvider.RealtimeTurnFinalEvent(), key)
	applyRealtimeTurnContextValues(postCtx, preHookValues)
	restoreRealtimeTurnTraceContext(postCtx, traceID, preHookValues)
	applyRealtimeRawStorageContext(postCtx, storeRaw)
	setRealtimeTurnStreamContext(postCtx, startedAt, true)
	_, unifaiErr = hooks.PostHookRunner(postCtx, postResponse, nil)
	completeRealtimeTurnTrace(postCtx)
	return unifaiErr
}

func finalizeRealtimeTurnHooksWithError(
	client *unifai.UnifAI,
	baseCtx *schemas.UnifAIContext,
	session *ufws.Session,
	provider schemas.ModelProvider,
	model string,
	key *schemas.Key,
	eventType schemas.RealtimeEventType,
	rawResponse []byte,
	unifaiErr *schemas.UnifAIError,
) *schemas.UnifAIError {
	if session == nil || unifaiErr == nil {
		return nil
	}

	turnInputs := session.ConsumeRealtimeTurnInputs()
	rawRequest := combineRealtimeInputRaw(turnInputs)
	session.ConsumeRealtimeOutputText()

	if activeHooks := session.ConsumeRealtimeTurnHooks(); activeHooks != nil {
		defer func() {
			if activeHooks.Cleanup != nil {
				activeHooks.Cleanup()
			}
		}()
		postErr := buildRealtimeTurnPostError(
			provider,
			model,
			rawRequest,
			rawResponse,
			unifaiErr,
		)
		postCtx := newRealtimeTurnContext(baseCtx, activeHooks.RequestID, session.ID(), session.ProviderSessionID(), realtimeTurnSourceLM, eventType, key)
		applyRealtimeTurnContextValues(postCtx, activeHooks.PreHookValues)
		restoreRealtimeTurnTraceContext(postCtx, activeHooks.TraceID, activeHooks.PreHookValues)
		applyRealtimeRawStorageContext(postCtx, activeHooks.RawStore)
		setRealtimeTurnStreamContext(postCtx, activeHooks.StartedAt, true)
		_, hookErr := activeHooks.PostHookRunner(postCtx, nil, postErr)
		completeRealtimeTurnTrace(postCtx)
		return hookErr
	}

	if len(turnInputs) == 0 {
		return nil
	}

	if client == nil {
		return nil
	}

	startedAt := time.Now()
	storeRaw := shouldStoreRealtimeRawPayloads(baseCtx)
	preCtx := newRealtimeTurnContext(baseCtx, "", session.ID(), session.ProviderSessionID(), realtimeTurnSourceEI, "", key)
	applyRealtimeRawStorageContext(preCtx, storeRaw)
	setRealtimeTurnStreamContext(preCtx, startedAt, false)
	preReq := buildRealtimeTurnPreRequest(provider, model, turnInputs, session.RealtimeSessionTools())
	hooks, hookPreErr := client.RunRealtimeTurnPreHooks(preCtx, preReq)
	if hookPreErr != nil {
		return hookPreErr
	}
	preHookValues := preCtx.GetUserValues()
	if hooks.Cleanup != nil {
		defer hooks.Cleanup()
	}

	requestID, _ := preCtx.Value(schemas.UnifAIContextKeyRequestID).(string)
	traceID, _ := preCtx.Value(schemas.UnifAIContextKeyTraceID).(string)
	postErr := buildRealtimeTurnPostError(
		provider,
		model,
		rawRequest,
		rawResponse,
		unifaiErr,
	)
	postCtx := newRealtimeTurnContext(baseCtx, requestID, session.ID(), session.ProviderSessionID(), realtimeTurnSourceLM, eventType, key)
	applyRealtimeTurnContextValues(postCtx, preHookValues)
	restoreRealtimeTurnTraceContext(postCtx, traceID, preHookValues)
	applyRealtimeRawStorageContext(postCtx, storeRaw)
	setRealtimeTurnStreamContext(postCtx, startedAt, true)
	_, hookErr := hooks.PostHookRunner(postCtx, nil, postErr)
	completeRealtimeTurnTrace(postCtx)
	return hookErr
}

func buildRealtimeTurnPostError(
	provider schemas.ModelProvider,
	model string,
	rawRequest string,
	rawResponse []byte,
	unifaiErr *schemas.UnifAIError,
) *schemas.UnifAIError {
	if unifaiErr == nil {
		return nil
	}

	copied := *unifaiErr
	copied.ExtraFields = unifaiErr.ExtraFields
	if unifaiErr.Error != nil {
		errorCopy := *unifaiErr.Error
		copied.Error = &errorCopy
	}
	copied.ExtraFields.RequestType = schemas.RealtimeRequest
	if copied.ExtraFields.Provider == "" {
		copied.ExtraFields.Provider = provider
	}
	if strings.TrimSpace(copied.ExtraFields.OriginalModelRequested) == "" {
		copied.ExtraFields.OriginalModelRequested = model
	}
	if strings.TrimSpace(rawRequest) != "" && copied.ExtraFields.RawRequest == nil {
		copied.ExtraFields.RawRequest = rawRequest
	}
	if len(rawResponse) > 0 && copied.ExtraFields.RawResponse == nil {
		copied.ExtraFields.RawResponse = json.RawMessage(append([]byte(nil), rawResponse...))
	}
	return &copied
}

func newUnifAIErrorFromRealtimeError(
	provider schemas.ModelProvider,
	model string,
	rawResponse []byte,
	realtimeErr *schemas.RealtimeError,
) *schemas.UnifAIError {
	if realtimeErr == nil {
		return nil
	}

	statusCode := 500
	values := []string{
		strings.TrimSpace(realtimeErr.Type),
		strings.TrimSpace(realtimeErr.Code),
		strings.TrimSpace(realtimeErr.Message),
	}
	for _, value := range values {
		lower := strings.ToLower(value)
		switch {
		case lower == "":
			continue
		case strings.Contains(lower, "invalid_request_error"):
			statusCode = 400
		case isBudgetOrBillingError(lower), unifai.IsRateLimitErrorMessage(lower):
			statusCode = 429
		}
	}

	errType := strings.TrimSpace(realtimeErr.Type)
	if errType == "" {
		errType = "server_error"
	}
	errCode := strings.TrimSpace(realtimeErr.Code)
	if errCode == "" {
		errCode = errType
	}
	message := strings.TrimSpace(realtimeErr.Message)
	if message == "" {
		message = "realtime turn failed"
	}

	unifaiErr := &schemas.UnifAIError{
		IsUnifAIError: true,
		StatusCode:    schemas.Ptr(statusCode),
		Type:          schemas.Ptr(errType),
		Error: &schemas.ErrorField{
			Type:    schemas.Ptr(errType),
			Code:    schemas.Ptr(errCode),
			Message: message,
		},
		ExtraFields: schemas.UnifAIErrorExtraFields{
			Provider:               provider,
			OriginalModelRequested: model,
			RequestType:            schemas.RealtimeRequest,
		},
	}
	if strings.TrimSpace(realtimeErr.Param) != "" {
		unifaiErr.Error.Param = realtimeErr.Param
	}
	if len(rawResponse) > 0 {
		unifaiErr.ExtraFields.RawResponse = json.RawMessage(append([]byte(nil), rawResponse...))
	}
	return unifaiErr
}

func completeRealtimeTurnTrace(ctx *schemas.UnifAIContext) {
	if ctx == nil {
		return
	}
	traceID, _ := ctx.Value(schemas.UnifAIContextKeyTraceID).(string)
	if strings.TrimSpace(traceID) == "" {
		return
	}
	tracer, _ := ctx.Value(schemas.UnifAIContextKeyTracer).(schemas.Tracer)
	if tracer == nil {
		return
	}
	tracer.CompleteAndFlushTrace(strings.TrimSpace(traceID))
}

func finalizeRealtimeTurnHooksOnTransportError(
	client *unifai.UnifAI,
	baseCtx *schemas.UnifAIContext,
	session *ufws.Session,
	provider schemas.ModelProvider,
	model string,
	key *schemas.Key,
	status int,
	code string,
	message string,
) *schemas.UnifAIError {
	return finalizeRealtimeTurnHooksWithError(
		client,
		baseCtx,
		session,
		provider,
		model,
		key,
		schemas.RTEventError,
		nil,
		newRealtimeWireUnifAIError(status, code, message),
	)
}
