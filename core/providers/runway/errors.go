package runway

import (
	"strings"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	schemas "github.com/unifai/unifai/core/schemas"
	"github.com/valyala/fasthttp"
)

// parseRunwayError parses Runway API error responses and converts them to UnifAIError.
func parseRunwayError(resp *fasthttp.Response) *schemas.UnifAIError {
	// Parse as RunwayAPIError
	var errorResp RunwayAPIError
	unifaiErr := providerUtils.HandleProviderAPIError(resp, &errorResp)

	// Set error message if available
	if errorResp.Error != "" {
		if unifaiErr.Error == nil {
			unifaiErr.Error = &schemas.ErrorField{}
		}
		unifaiErr.Error.Message = errorResp.Error
	} else if unifaiErr.Error != nil && unifaiErr.Error.Message == "" {
		// If no error message was extracted, use a generic one
		unifaiErr.Error.Message = "Runway API request failed"
	} else if unifaiErr.Error == nil {
		unifaiErr.Error = &schemas.ErrorField{
			Message: "Runway API request failed",
		}
	}

	// Remove trailing newlines
	if unifaiErr.Error != nil && unifaiErr.Error.Message != "" {
		unifaiErr.Error.Message = strings.TrimRight(unifaiErr.Error.Message, "\n")
	}

	return unifaiErr
}
