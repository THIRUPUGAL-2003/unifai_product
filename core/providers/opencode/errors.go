package opencode

import (
	"fmt"
	"strings"

	"github.com/bytedance/sonic"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
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
func parseOpencodeError(resp *fasthttp.Response) *schemas.GatewayError {
	var gatewayErr schemas.GatewayError

	// First, let the generic handler parse HTTP status and set base fields.
	_ = providerUtils.HandleProviderAPIError(resp, &gatewayErr)

	// Ensure Error is non-nil before accessing its fields.
	if gatewayErr.Error == nil {
		gatewayErr.Error = &schemas.ErrorField{}
	}

	// Then overlay Opencode-specific error details from the body.
	if body := resp.Body(); len(body) > 0 {
		var parsed opencodeErrorBody
		if err := sonic.Unmarshal(body, &parsed); err == nil && parsed.Type == "error" {
			if parsed.Error.Message != "" {
				gatewayErr.Error.Message = parsed.Error.Message
			}
			if parsed.Error.Type != "" {
				gatewayErr.Error.Type = &parsed.Error.Type
			}
		}
	}

	// Ensure we always have a non-empty error message.
	if strings.TrimSpace(gatewayErr.Error.Message) == "" {
		if gatewayErr.StatusCode != nil {
			gatewayErr.Error.Message = fmt.Sprintf("provider API error (status %d)", *gatewayErr.StatusCode)
		} else {
			gatewayErr.Error.Message = "provider API error"
		}
	}

	return &gatewayErr
}
