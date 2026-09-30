package openai

import (
	"fmt"
	"strings"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
	"github.com/valyala/fasthttp"
)

// ErrorConverter is a function that converts provider-specific error responses to UnifAIError.
type ErrorConverter func(resp *fasthttp.Response) *schemas.UnifAIError

// ParseOpenAIError parses OpenAI error responses.
func ParseOpenAIError(resp *fasthttp.Response) *schemas.UnifAIError {
	var errorResp schemas.UnifAIError

	unifaiErr := providerUtils.HandleProviderAPIError(resp, &errorResp)

	if errorResp.EventID != nil {
		unifaiErr.EventID = errorResp.EventID
	}

	if errorResp.Error != nil {
		if unifaiErr.Error == nil {
			unifaiErr.Error = &schemas.ErrorField{}
		}
		unifaiErr.Error.Type = errorResp.Error.Type
		unifaiErr.Error.Code = errorResp.Error.Code
		if errorResp.Error.Message != "" {
			unifaiErr.Error.Message = errorResp.Error.Message
		}
		unifaiErr.Error.Param = errorResp.Error.Param
		if errorResp.Error.EventID != nil {
			unifaiErr.Error.EventID = errorResp.Error.EventID
		}
	}

	if unifaiErr.Error == nil {
		unifaiErr.Error = &schemas.ErrorField{}
	}
	if strings.TrimSpace(unifaiErr.Error.Message) == "" {
		if unifaiErr.StatusCode != nil {
			unifaiErr.Error.Message = fmt.Sprintf("provider API error (status %d)", *unifaiErr.StatusCode)
		} else {
			unifaiErr.Error.Message = "provider API error"
		}
	}

	// Set ExtraFields unconditionally so provider/model/request metadata is always attached

	return unifaiErr
}
