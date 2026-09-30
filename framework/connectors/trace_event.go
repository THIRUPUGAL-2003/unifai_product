package connectors

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/unifai/unifai/core/schemas"
)

func traceEvent(trace *schemas.Trace) map[string]any {
	if trace == nil {
		return map[string]any{}
	}
	event := map[string]any{
		"trace_id":   trace.TraceID,
		"request_id": trace.RequestID,
		"start_time": trace.StartTime.UTC().Format(time.RFC3339Nano),
		"end_time":   trace.EndTime.UTC().Format(time.RFC3339Nano),
		"duration_ms": trace.EndTime.Sub(trace.StartTime).Milliseconds(),
	}
	if trace.Attributes != nil {
		if v, ok := trace.Attributes[schemas.TraceAttrSessionID]; ok {
			event["session_id"] = v
		}
		if v, ok := trace.Attributes[schemas.TraceAttrDimensions]; ok {
			event["dimensions"] = v
		}
	}
	if trace.RootSpan != nil {
		mergeSpanAttrs(event, trace.RootSpan)
	}
	for _, span := range trace.Spans {
		if span == nil || span.Name != "llm.call" {
			continue
		}
		mergeSpanAttrs(event, span)
		break
	}
	return event
}

func mergeSpanAttrs(event map[string]any, span *schemas.Span) {
	if span == nil || span.Attributes == nil {
		return
	}
	for _, key := range []string{
		schemas.AttrProviderName,
		schemas.AttrRequestModel,
		schemas.AttrOperationName,
		schemas.AttrVirtualKeyID,
		schemas.AttrVirtualKeyName,
		schemas.AttrSelectedKeyID,
		schemas.AttrSelectedKeyName,
		schemas.AttrTeamID,
		schemas.AttrCustomerID,
		"http.status_code",
		"error.message",
	} {
		if v, ok := span.Attributes[key]; ok && v != nil {
			event[key] = v
		}
	}
}

func encodeEvent(event map[string]any) ([]byte, error) {
	return sonic.Marshal(event)
}
