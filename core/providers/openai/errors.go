package openai

import (
	"fmt"
	"strings"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// ErrorConverter is a function that converts provider-specific error responses to RakshaError.
type ErrorConverter func(resp *fasthttp.Response) *schemas.RakshaError

// ParseOpenAIError parses OpenAI error responses.
func ParseOpenAIError(resp *fasthttp.Response) *schemas.RakshaError {
	var errorResp schemas.RakshaError

	rakshaErr := providerUtils.HandleProviderAPIError(resp, &errorResp)

	if errorResp.EventID != nil {
		rakshaErr.EventID = errorResp.EventID
	}

	if errorResp.Error != nil {
		if rakshaErr.Error == nil {
			rakshaErr.Error = &schemas.ErrorField{}
		}
		rakshaErr.Error.Type = errorResp.Error.Type
		rakshaErr.Error.Code = errorResp.Error.Code
		if errorResp.Error.Message != "" {
			rakshaErr.Error.Message = errorResp.Error.Message
		}
		rakshaErr.Error.Param = errorResp.Error.Param
		if errorResp.Error.EventID != nil {
			rakshaErr.Error.EventID = errorResp.Error.EventID
		}
	}

	if rakshaErr.Error == nil {
		rakshaErr.Error = &schemas.ErrorField{}
	}
	if strings.TrimSpace(rakshaErr.Error.Message) == "" {
		if rakshaErr.StatusCode != nil {
			rakshaErr.Error.Message = fmt.Sprintf("provider API error (status %d)", *rakshaErr.StatusCode)
		} else {
			rakshaErr.Error.Message = "provider API error"
		}
	}

	// Set ExtraFields unconditionally so provider/model/request metadata is always attached

	return rakshaErr
}
