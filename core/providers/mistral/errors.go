package mistral

import (
	"fmt"
	"strings"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
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
func ParseMistralError(resp *fasthttp.Response) *schemas.UnifAIError {
	var errorResp MistralErrorResponse
	unifaiErr := providerUtils.HandleProviderAPIError(resp, &errorResp)
	if unifaiErr == nil {
		return nil
	}

	if unifaiErr.Error == nil {
		unifaiErr.Error = &schemas.ErrorField{}
	}

	if errorResp.Error != nil {
		if strings.TrimSpace(errorResp.Error.Message) != "" {
			unifaiErr.Error.Message = errorResp.Error.Message
		}
		if errorResp.Error.Type != nil && strings.TrimSpace(*errorResp.Error.Type) != "" {
			unifaiErr.Error.Type = errorResp.Error.Type
			unifaiErr.Type = errorResp.Error.Type
		}
		if errorResp.Error.Code != nil && strings.TrimSpace(*errorResp.Error.Code) != "" {
			unifaiErr.Error.Code = errorResp.Error.Code
		}
		unifaiErr.Error.Param = errorResp.Error.Param
		if errorResp.Error.EventID != nil {
			unifaiErr.Error.EventID = errorResp.Error.EventID
		}
	}

	if strings.TrimSpace(errorResp.Message) != "" {
		unifaiErr.Error.Message = errorResp.Message
	}
	if strings.TrimSpace(errorResp.Type) != "" {
		errorType := schemas.Ptr(errorResp.Type)
		unifaiErr.Error.Type = errorType
		unifaiErr.Type = errorType
	}
	if strings.TrimSpace(errorResp.Code) != "" {
		unifaiErr.Error.Code = schemas.Ptr(errorResp.Code)
	}

	if strings.TrimSpace(unifaiErr.Error.Message) == "" {
		if unifaiErr.StatusCode != nil {
			unifaiErr.Error.Message = fmt.Sprintf("provider API error (status %d)", *unifaiErr.StatusCode)
		} else {
			unifaiErr.Error.Message = "provider API error"
		}
	}

	return unifaiErr
}
