package bedrock

import (
	"net/http"
	"strings"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
	"github.com/valyala/fasthttp"
)

func parseBedrockHTTPError(statusCode int, headers http.Header, body []byte) *schemas.UnifAIError {
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
	unifaiErr := providerUtils.HandleProviderAPIError(fastResp, &errorResp)
	if errorResp.Message != "" {
		if unifaiErr.Error == nil {
			unifaiErr.Error = &schemas.ErrorField{}
		}
		unifaiErr.Error.Message = errorResp.Message
		unifaiErr.Error.Code = errorResp.Code
	}

	if unifaiErr.Type == nil {
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
			unifaiErr.Type = &exceptionType
		}
	}

	return unifaiErr
}
