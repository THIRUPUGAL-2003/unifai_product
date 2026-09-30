package runware

import (
	"strings"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	schemas "github.com/unifai/unifai/core/schemas"
	"github.com/valyala/fasthttp"
)

// parseRunwareError parses a Runware error HTTP response into a UnifAIError.
// Runware reports failures in a top-level "errors" array.
func parseRunwareError(resp *fasthttp.Response) *schemas.UnifAIError {
	var errorResp RunwareResponse
	unifaiErr := providerUtils.HandleProviderAPIError(resp, &errorResp)

	if msg := firstRunwareErrorMessage(errorResp.Errors); msg != "" {
		if unifaiErr.Error == nil {
			unifaiErr.Error = &schemas.ErrorField{}
		}
		unifaiErr.Error.Message = msg
	} else if unifaiErr.Error == nil || unifaiErr.Error.Message == "" {
		if unifaiErr.Error == nil {
			unifaiErr.Error = &schemas.ErrorField{}
		}
		unifaiErr.Error.Message = "Runware API request failed"
	}

	if unifaiErr.Error != nil {
		unifaiErr.Error.Message = strings.TrimRight(unifaiErr.Error.Message, "\n")
	}

	return unifaiErr
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
