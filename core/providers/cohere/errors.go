package cohere

import (
	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

func parseCohereError(resp *fasthttp.Response) *schemas.GatewayError {
	var errorResp CohereError
	gatewayErr := providerUtils.HandleProviderAPIError(resp, &errorResp)
	gatewayErr.Type = &errorResp.Type
	if gatewayErr.Error == nil {
		gatewayErr.Error = &schemas.ErrorField{}
	}
	gatewayErr.Error.Message = errorResp.Message
	if errorResp.Code != nil {
		gatewayErr.Error.Code = errorResp.Code
	}
	return gatewayErr
}
