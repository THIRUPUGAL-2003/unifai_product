package gemini

import (
	"strconv"
	"strings"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

// ToGeminiError derives a GeminiGenerationError from a GatewayError
func ToGeminiError(gatewayErr *schemas.GatewayError) *GeminiGenerationError {
	if gatewayErr == nil {
		return nil
	}
	code := 500
	status := ""
	if gatewayErr.Error != nil && gatewayErr.Error.Type != nil {
		status = *gatewayErr.Error.Type
	}
	message := ""
	if gatewayErr.Error != nil && gatewayErr.Error.Message != "" {
		message = gatewayErr.Error.Message
	}
	if gatewayErr.StatusCode != nil {
		code = *gatewayErr.StatusCode
	}
	return &GeminiGenerationError{
		Error: &GeminiGenerationErrorStruct{
			Code:    code,
			Message: message,
			Status:  status,
		},
	}
}

// parseGeminiError parses Gemini error responses
func parseGeminiError(resp *fasthttp.Response) *schemas.GatewayError {
	// Try to parse as []GeminiGenerationError
	var errorResps []GeminiGenerationError
	gatewayErr := providerUtils.HandleProviderAPIError(resp, &errorResps)
	if len(errorResps) > 0 {
		var message string
		var firstError *GeminiGenerationErrorStruct
		for _, errorResp := range errorResps {
			if errorResp.Error != nil {
				if firstError == nil {
					firstError = errorResp.Error
				}
				message = message + errorResp.Error.Message + "\n"
			}
		}
		// Trim trailing newline
		message = strings.TrimSuffix(message, "\n")
		if gatewayErr.Error == nil {
			gatewayErr.Error = &schemas.ErrorField{}
		}
		// Set Code from first error if available
		if firstError != nil {
			gatewayErr.Error.Code = schemas.Ptr(strconv.Itoa(firstError.Code))
		}
		// Set Message to trimmed concatenated message
		gatewayErr.Error.Message = message
		return gatewayErr
	}

	// Try to parse as GeminiGenerationError
	var errorResp GeminiGenerationError
	gatewayErr = providerUtils.HandleProviderAPIError(resp, &errorResp)
	if errorResp.Error != nil {
		if gatewayErr.Error == nil {
			gatewayErr.Error = &schemas.ErrorField{}
		}
		gatewayErr.Error.Code = schemas.Ptr(strconv.Itoa(errorResp.Error.Code))
		gatewayErr.Error.Message = errorResp.Error.Message
	}
	return gatewayErr
}
