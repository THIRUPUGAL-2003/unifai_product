package xai

import (
	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
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
func ParseXAIError(resp *fasthttp.Response) *schemas.RakshaError {
	// Try to parse xAI error format
	var xaiErr XAIErrorResponse
	rakshaErr := providerUtils.HandleProviderAPIError(resp, &xaiErr)

	if rakshaErr == nil {
		return nil
	}

	// If we successfully parsed xAI format, extract the fields
	if xaiErr.Error != "" {
		if rakshaErr.Error == nil {
			rakshaErr.Error = &schemas.ErrorField{}
		}
		rakshaErr.Error.Message = xaiErr.Error
		if xaiErr.Code != "" {
			rakshaErr.Error.Code = schemas.Ptr(xaiErr.Code)
		}
	}

	return rakshaErr
}
