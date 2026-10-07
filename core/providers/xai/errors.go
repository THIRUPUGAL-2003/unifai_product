package xai

import (
	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

// XAIErrorResponse represents xAI's error response format
type XAIErrorResponse struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

// ParseXAIError parses xAI-specific error responses.
// xAI returns errors in format: {"code": "...", "error": "..."}
// Unlike OpenAI which uses: {"error": {"message": "...", "type": "...", "code": "..."}}
func ParseXAIError(resp *fasthttp.Response) *schemas.GatewayError {
	// Try to parse xAI error format
	var xaiErr XAIErrorResponse
	gatewayErr := providerUtils.HandleProviderAPIError(resp, &xaiErr)

	if gatewayErr == nil {
		return nil
	}

	// If we successfully parsed xAI format, extract the fields
	if xaiErr.Error != "" {
		if gatewayErr.Error == nil {
			gatewayErr.Error = &schemas.ErrorField{}
		}
		gatewayErr.Error.Message = xaiErr.Error
		if xaiErr.Code != "" {
			gatewayErr.Error.Code = schemas.Ptr(xaiErr.Code)
		}
	}

	return gatewayErr
}
