package opencode

import (
	"fmt"
	"strings"

	"github.com/bytedance/sonic"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// opencodeErrorBody is the JSON envelope returned by Opencode Zen/Go API errors.
// Format: {"type": "error", "error": {"type": "...", "message": "..."}}
type opencodeErrorBody struct {
	Type  string            `json:"type"`
	Error opencodeErrorInner `json:"error"`
}

type opencodeErrorInner struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// parseOpencodeError parses Opencode-specific error responses.
// Opencode uses {"type":"error","error":{"type":"...","message":"..."}} instead
// of OpenAI's {"error":{"message":"...","type":"...","code":...}}.
func parseOpencodeError(resp *fasthttp.Response) *schemas.RakshaError {
	var rakshaErr schemas.RakshaError

	// First, let the generic handler parse HTTP status and set base fields.
	_ = providerUtils.HandleProviderAPIError(resp, &rakshaErr)

	// Ensure Error is non-nil before accessing its fields.
	if rakshaErr.Error == nil {
		rakshaErr.Error = &schemas.ErrorField{}
	}

	// Then overlay Opencode-specific error details from the body.
	if body := resp.Body(); len(body) > 0 {
		var parsed opencodeErrorBody
		if err := sonic.Unmarshal(body, &parsed); err == nil && parsed.Type == "error" {
			if parsed.Error.Message != "" {
				rakshaErr.Error.Message = parsed.Error.Message
			}
			if parsed.Error.Type != "" {
				rakshaErr.Error.Type = &parsed.Error.Type
			}
		}
	}

	// Ensure we always have a non-empty error message.
	if strings.TrimSpace(rakshaErr.Error.Message) == "" {
		if rakshaErr.StatusCode != nil {
			rakshaErr.Error.Message = fmt.Sprintf("provider API error (status %d)", *rakshaErr.StatusCode)
		} else {
			rakshaErr.Error.Message = "provider API error"
		}
	}

	return &rakshaErr
}
