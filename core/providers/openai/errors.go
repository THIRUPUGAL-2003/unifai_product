package openai

import (
	"fmt"
	"strings"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

// ErrorConverter is a function that converts provider-specific error responses to GatewayError.
type ErrorConverter func(resp *fasthttp.Response) *schemas.GatewayError

// ParseOpenAIError parses OpenAI error responses.
func ParseOpenAIError(resp *fasthttp.Response) *schemas.GatewayError {
	var errorResp schemas.GatewayError

	gatewayErr := providerUtils.HandleProviderAPIError(resp, &errorResp)

	if errorResp.EventID != nil {
		gatewayErr.EventID = errorResp.EventID
	}

	if errorResp.Error != nil {
		if gatewayErr.Error == nil {
			gatewayErr.Error = &schemas.ErrorField{}
		}
		gatewayErr.Error.Type = errorResp.Error.Type
		gatewayErr.Error.Code = errorResp.Error.Code
		if errorResp.Error.Message != "" {
			gatewayErr.Error.Message = errorResp.Error.Message
		}
		gatewayErr.Error.Param = errorResp.Error.Param
		if errorResp.Error.EventID != nil {
			gatewayErr.Error.EventID = errorResp.Error.EventID
		}
	}

	if gatewayErr.Error == nil {
		gatewayErr.Error = &schemas.ErrorField{}
	}
	if strings.TrimSpace(gatewayErr.Error.Message) == "" {
		if gatewayErr.StatusCode != nil {
			gatewayErr.Error.Message = fmt.Sprintf("provider API error (status %d)", *gatewayErr.StatusCode)
		} else {
			gatewayErr.Error.Message = "provider API error"
		}
	}

	// Set ExtraFields unconditionally so provider/model/request metadata is always attached

	return gatewayErr
}
