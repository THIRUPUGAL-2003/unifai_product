package cohere

import (
	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

func parseCohereError(resp *fasthttp.Response) *schemas.RakshaError {
	var errorResp CohereError
	rakshaErr := providerUtils.HandleProviderAPIError(resp, &errorResp)
	rakshaErr.Type = &errorResp.Type
	if rakshaErr.Error == nil {
		rakshaErr.Error = &schemas.ErrorField{}
	}
	rakshaErr.Error.Message = errorResp.Message
	if errorResp.Code != nil {
		rakshaErr.Error.Code = errorResp.Code
	}
	return rakshaErr
}
