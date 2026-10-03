package mistral

import (
	"fmt"
	"strings"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// MistralErrorResponse captures both Mistral's top-level error shape and nested OpenAI-style errors.
type MistralErrorResponse struct {
	Object  string              `json:"object,omitempty"`
	Message string              `json:"message,omitempty"`
	Type    string              `json:"type,omitempty"`
	Code    string              `json:"code,omitempty"`
	Error   *schemas.ErrorField `json:"error,omitempty"`
}

// ParseMistralError parses Mistral-specific error responses.
func ParseMistralError(resp *fasthttp.Response) *schemas.RakshaError {
	var errorResp MistralErrorResponse
	rakshaErr := providerUtils.HandleProviderAPIError(resp, &errorResp)
	if rakshaErr == nil {
		return nil
	}

	if rakshaErr.Error == nil {
		rakshaErr.Error = &schemas.ErrorField{}
	}

	if errorResp.Error != nil {
		if strings.TrimSpace(errorResp.Error.Message) != "" {
			rakshaErr.Error.Message = errorResp.Error.Message
		}
		if errorResp.Error.Type != nil && strings.TrimSpace(*errorResp.Error.Type) != "" {
			rakshaErr.Error.Type = errorResp.Error.Type
			rakshaErr.Type = errorResp.Error.Type
		}
		if errorResp.Error.Code != nil && strings.TrimSpace(*errorResp.Error.Code) != "" {
			rakshaErr.Error.Code = errorResp.Error.Code
		}
		rakshaErr.Error.Param = errorResp.Error.Param
		if errorResp.Error.EventID != nil {
			rakshaErr.Error.EventID = errorResp.Error.EventID
		}
	}

	if strings.TrimSpace(errorResp.Message) != "" {
		rakshaErr.Error.Message = errorResp.Message
	}
	if strings.TrimSpace(errorResp.Type) != "" {
		errorType := schemas.Ptr(errorResp.Type)
		rakshaErr.Error.Type = errorType
		rakshaErr.Type = errorType
	}
	if strings.TrimSpace(errorResp.Code) != "" {
		rakshaErr.Error.Code = schemas.Ptr(errorResp.Code)
	}

	if strings.TrimSpace(rakshaErr.Error.Message) == "" {
		if rakshaErr.StatusCode != nil {
			rakshaErr.Error.Message = fmt.Sprintf("provider API error (status %d)", *rakshaErr.StatusCode)
		} else {
			rakshaErr.Error.Message = "provider API error"
		}
	}

	return rakshaErr
}
