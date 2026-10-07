package mistral

import (
	"fmt"
	"strings"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
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
func ParseMistralError(resp *fasthttp.Response) *schemas.GatewayError {
	var errorResp MistralErrorResponse
	gatewayErr := providerUtils.HandleProviderAPIError(resp, &errorResp)
	if gatewayErr == nil {
		return nil
	}

	if gatewayErr.Error == nil {
		gatewayErr.Error = &schemas.ErrorField{}
	}

	if errorResp.Error != nil {
		if strings.TrimSpace(errorResp.Error.Message) != "" {
			gatewayErr.Error.Message = errorResp.Error.Message
		}
		if errorResp.Error.Type != nil && strings.TrimSpace(*errorResp.Error.Type) != "" {
			gatewayErr.Error.Type = errorResp.Error.Type
			gatewayErr.Type = errorResp.Error.Type
		}
		if errorResp.Error.Code != nil && strings.TrimSpace(*errorResp.Error.Code) != "" {
			gatewayErr.Error.Code = errorResp.Error.Code
		}
		gatewayErr.Error.Param = errorResp.Error.Param
		if errorResp.Error.EventID != nil {
			gatewayErr.Error.EventID = errorResp.Error.EventID
		}
	}

	if strings.TrimSpace(errorResp.Message) != "" {
		gatewayErr.Error.Message = errorResp.Message
	}
	if strings.TrimSpace(errorResp.Type) != "" {
		errorType := schemas.Ptr(errorResp.Type)
		gatewayErr.Error.Type = errorType
		gatewayErr.Type = errorType
	}
	if strings.TrimSpace(errorResp.Code) != "" {
		gatewayErr.Error.Code = schemas.Ptr(errorResp.Code)
	}

	if strings.TrimSpace(gatewayErr.Error.Message) == "" {
		if gatewayErr.StatusCode != nil {
			gatewayErr.Error.Message = fmt.Sprintf("provider API error (status %d)", *gatewayErr.StatusCode)
		} else {
			gatewayErr.Error.Message = "provider API error"
		}
	}

	return gatewayErr
}
