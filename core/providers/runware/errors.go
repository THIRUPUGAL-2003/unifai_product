package runware

import (
	"strings"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	schemas "github.com/gateway/gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

// parseRunwareError parses a Runware error HTTP response into a GatewayError.
// Runware reports failures in a top-level "errors" array.
func parseRunwareError(resp *fasthttp.Response) *schemas.GatewayError {
	var errorResp RunwareResponse
	gatewayErr := providerUtils.HandleProviderAPIError(resp, &errorResp)

	if msg := firstRunwareErrorMessage(errorResp.Errors); msg != "" {
		if gatewayErr.Error == nil {
			gatewayErr.Error = &schemas.ErrorField{}
		}
		gatewayErr.Error.Message = msg
	} else if gatewayErr.Error == nil || gatewayErr.Error.Message == "" {
		if gatewayErr.Error == nil {
			gatewayErr.Error = &schemas.ErrorField{}
		}
		gatewayErr.Error.Message = "Runware API request failed"
	}

	if gatewayErr.Error != nil {
		gatewayErr.Error.Message = strings.TrimRight(gatewayErr.Error.Message, "\n")
	}

	return gatewayErr
}

// firstRunwareErrorMessage returns a human-readable message from the first error, if any.
func firstRunwareErrorMessage(errs []RunwareError) string {
	for _, e := range errs {
		if e.Message != "" {
			if e.Parameter != "" {
				return e.Message + " (parameter: " + e.Parameter + ")"
			}
			return e.Message
		}
	}
	return ""
}
