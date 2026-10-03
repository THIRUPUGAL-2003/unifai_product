package runware

import (
	"strings"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// parseRunwareError parses a Runware error HTTP response into a RakshaError.
// Runware reports failures in a top-level "errors" array.
func parseRunwareError(resp *fasthttp.Response) *schemas.RakshaError {
	var errorResp RunwareResponse
	rakshaErr := providerUtils.HandleProviderAPIError(resp, &errorResp)

	if msg := firstRunwareErrorMessage(errorResp.Errors); msg != "" {
		if rakshaErr.Error == nil {
			rakshaErr.Error = &schemas.ErrorField{}
		}
		rakshaErr.Error.Message = msg
	} else if rakshaErr.Error == nil || rakshaErr.Error.Message == "" {
		if rakshaErr.Error == nil {
			rakshaErr.Error = &schemas.ErrorField{}
		}
		rakshaErr.Error.Message = "Runware API request failed"
	}

	if rakshaErr.Error != nil {
		rakshaErr.Error.Message = strings.TrimRight(rakshaErr.Error.Message, "\n")
	}

	return rakshaErr
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
