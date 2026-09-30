package cohere

import (
	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
	"github.com/valyala/fasthttp"
)

func parseCohereError(resp *fasthttp.Response) *schemas.UnifAIError {
	var errorResp CohereError
	unifaiErr := providerUtils.HandleProviderAPIError(resp, &errorResp)
	unifaiErr.Type = &errorResp.Type
	if unifaiErr.Error == nil {
		unifaiErr.Error = &schemas.ErrorField{}
	}
	unifaiErr.Error.Message = errorResp.Message
	if errorResp.Code != nil {
		unifaiErr.Error.Code = errorResp.Code
	}
	return unifaiErr
}
