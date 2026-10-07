package bedrock

import (
	"net/http"
	"strings"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

func parseBedrockHTTPError(statusCode int, headers http.Header, body []byte) *schemas.GatewayError {
	fastResp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(fastResp)

	fastResp.SetStatusCode(statusCode)
	for k, values := range headers {
		for _, value := range values {
			fastResp.Header.Add(k, value)
		}
	}
	fastResp.SetBody(body)

	var errorResp BedrockError
	gatewayErr := providerUtils.HandleProviderAPIError(fastResp, &errorResp)
	if errorResp.Message != "" {
		if gatewayErr.Error == nil {
			gatewayErr.Error = &schemas.ErrorField{}
		}
		gatewayErr.Error.Message = errorResp.Message
		gatewayErr.Error.Code = errorResp.Code
	}

	if gatewayErr.Type == nil {
		exceptionType := errorResp.Type
		if exceptionType == "" {
			if hv := headers.Get("X-Amzn-Errortype"); hv != "" {
				if i := strings.IndexAny(hv, ":#"); i >= 0 {
					hv = hv[:i]
				}
				exceptionType = strings.TrimSpace(hv)
			}
		}
		if exceptionType != "" {
			gatewayErr.Type = &exceptionType
		}
	}

	return gatewayErr
}
