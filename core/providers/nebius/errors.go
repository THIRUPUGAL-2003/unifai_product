package nebius

import (
	"strings"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
	"github.com/valyala/fasthttp"
)

// parseNebiusImageError parses Nebius error responses
func parseNebiusImageError(resp *fasthttp.Response) *schemas.UnifAIError {
	var nebiusErr NebiusError
	unifaiErr := providerUtils.HandleProviderAPIError(resp, &nebiusErr)

	if unifaiErr.Error == nil {
		unifaiErr.Error = &schemas.ErrorField{}
	}

	// Extract error message
	var message string
	if nebiusErr.Detail != nil {
		if nebiusErr.Detail.Message != nil {
			message = *nebiusErr.Detail.Message
		}

		if len(nebiusErr.Detail.ValidationErrors) > 0 {
			var messages []string
			var locations []string

			if message != "" {
				messages = append(messages, message)
			}

			for _, detail := range nebiusErr.Detail.ValidationErrors {
				if detail.Msg != "" {
					messages = append(messages, detail.Msg)
				}
				if len(detail.Loc) > 0 {
					locations = append(locations, strings.Join(detail.Loc, "."))
				}
			}

			if len(messages) > 0 {
				message = strings.Join(messages, "; ")
			}
			if len(locations) > 0 {
				locationStr := strings.Join(locations, ", ")
				if message == "" {
					message = "[" + locationStr + "]"
				} else {
					message = message + " [" + locationStr + "]"
				}
			}
		}
	}

	// Use the extracted message if available
	if message != "" {
		unifaiErr.Error.Message = message
	}

	return unifaiErr
}
