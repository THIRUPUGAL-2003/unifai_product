package bedrock

import (
	"net/http"
	"strings"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

func parseBedrockHTTPError(statusCode int, headers http.Header, body []byte) *schemas.RakshaError {
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
	rakshaErr := providerUtils.HandleProviderAPIError(fastResp, &errorResp)
	if errorResp.Message != "" {
		if rakshaErr.Error == nil {
			rakshaErr.Error = &schemas.ErrorField{}
		}
		rakshaErr.Error.Message = errorResp.Message
		rakshaErr.Error.Code = errorResp.Code
	}

	if rakshaErr.Type == nil {
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
			rakshaErr.Type = &exceptionType
		}
	}

	return rakshaErr
}
