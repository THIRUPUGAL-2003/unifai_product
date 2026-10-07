package runway

import (
	"strings"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	schemas "github.com/gateway/gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

// parseRunwayError parses Runway API error responses and converts them to GatewayError.
func parseRunwayError(resp *fasthttp.Response) *schemas.GatewayError {
	// Parse as RunwayAPIError
	var errorResp RunwayAPIError
	gatewayErr := providerUtils.HandleProviderAPIError(resp, &errorResp)

	// Set error message if available
	if errorResp.Error != "" {
		if gatewayErr.Error == nil {
			gatewayErr.Error = &schemas.ErrorField{}
		}
		gatewayErr.Error.Message = errorResp.Error
	} else if gatewayErr.Error != nil && gatewayErr.Error.Message == "" {
		// If no error message was extracted, use a generic one
		gatewayErr.Error.Message = "Runway API request failed"
	} else if gatewayErr.Error == nil {
		gatewayErr.Error = &schemas.ErrorField{
			Message: "Runway API request failed",
		}
	}

	// Remove trailing newlines
	if gatewayErr.Error != nil && gatewayErr.Error.Message != "" {
		gatewayErr.Error.Message = strings.TrimRight(gatewayErr.Error.Message, "\n")
	}

	return gatewayErr
}
