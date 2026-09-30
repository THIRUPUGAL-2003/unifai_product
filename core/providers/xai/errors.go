package xai

import (
	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
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
func ParseXAIError(resp *fasthttp.Response) *schemas.UnifAIError {
	// Try to parse xAI error format
	var xaiErr XAIErrorResponse
	unifaiErr := providerUtils.HandleProviderAPIError(resp, &xaiErr)

	if unifaiErr == nil {
		return nil
	}

	// If we successfully parsed xAI format, extract the fields
	if xaiErr.Error != "" {
		if unifaiErr.Error == nil {
			unifaiErr.Error = &schemas.ErrorField{}
		}
		unifaiErr.Error.Message = xaiErr.Error
		if xaiErr.Code != "" {
			unifaiErr.Error.Code = schemas.Ptr(xaiErr.Code)
		}
	}

	return unifaiErr
}
