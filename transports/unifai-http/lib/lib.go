package lib

import (
	"io"
	"strconv"

	"github.com/unifai/unifai/core/schemas"
	"github.com/valyala/fasthttp"
)

var logger schemas.Logger

// SetLogger sets the logger for the application.
func SetLogger(l schemas.Logger) {
	logger = l
}

// HasDuplicates reports whether the slice contains any repeated element.
// Comparison is exact; callers needing case-insensitive or whitespace-tolerant
// semantics should normalize the slice before calling (e.g. lower-case the
// entries for case-insensitive HTTP header names).
func HasDuplicates[T comparable](items []T) bool {
	if len(items) < 2 {
		return false
	}
	seen := make(map[T]struct{}, len(items))
	for _, it := range items {
		if _, dup := seen[it]; dup {
			return true
		}
		seen[it] = struct{}{}
	}
	return false
}

// StreamLargeResponseBody extracts the large response reader from context and streams
// it directly to the client. Sets status 200, content-type, and content-length headers.
// Returns false if the reader is not available (caller should send an error response).
func StreamLargeResponseBody(ctx *fasthttp.RequestCtx, unifaiCtx *schemas.UnifAIContext) bool {
	if unifaiCtx == nil {
		return false
	}
	reader, ok := unifaiCtx.Value(schemas.UnifAIContextKeyLargeResponseReader).(io.ReadCloser)
	if !ok || reader == nil {
		return false
	}

	contentLength, _ := unifaiCtx.Value(schemas.UnifAIContextKeyLargeResponseContentLength).(int)
	contentType, _ := unifaiCtx.Value(schemas.UnifAIContextKeyLargeResponseContentType).(string)
	contentDisposition, _ := unifaiCtx.Value(schemas.UnifAIContextKeyLargeResponseContentDisposition).(string)

	// Mirror large-response-mode to fasthttp UserValue so post-hook middleware
	// (which only sees ctx.UserValue, not unifaiCtx) can skip body materialization.
	ctx.SetUserValue(FastHTTPUserValueLargeResponseMode, true)

	ctx.SetStatusCode(fasthttp.StatusOK)
	if contentType != "" {
		ctx.SetContentType(contentType)
	} else {
		ctx.SetContentType("application/json")
	}
	if contentDisposition != "" {
		ctx.Response.Header.Set("Content-Disposition", contentDisposition)
	}
	// bodySize for SetBodyStream: positive = known size, -1 = unknown (read until EOF).
	// fasthttp treats 0 as "known empty", so default to -1 when CL is unavailable.
	bodySize := contentLength
	if bodySize > 0 {
		ctx.Response.Header.Set("Content-Length", strconv.Itoa(contentLength))
	} else {
		bodySize = -1
	}

	ctx.Response.SetBodyStream(reader, bodySize)
	return true
}
