package runway

import (
	"strings"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// parseRunwayError parses Runway API error responses and converts them to RakshaError.
func parseRunwayError(resp *fasthttp.Response) *schemas.RakshaError {
	// Parse as RunwayAPIError
	var errorResp RunwayAPIError
	rakshaErr := providerUtils.HandleProviderAPIError(resp, &errorResp)

	// Set error message if available
	if errorResp.Error != "" {
		if rakshaErr.Error == nil {
			rakshaErr.Error = &schemas.ErrorField{}
		}
		rakshaErr.Error.Message = errorResp.Error
	} else if rakshaErr.Error != nil && rakshaErr.Error.Message == "" {
		// If no error message was extracted, use a generic one
		rakshaErr.Error.Message = "Runway API request failed"
	} else if rakshaErr.Error == nil {
		rakshaErr.Error = &schemas.ErrorField{
			Message: "Runway API request failed",
		}
	}

	// Remove trailing newlines
	if rakshaErr.Error != nil && rakshaErr.Error.Message != "" {
		rakshaErr.Error.Message = strings.TrimRight(rakshaErr.Error.Message, "\n")
	}

	return rakshaErr
}
