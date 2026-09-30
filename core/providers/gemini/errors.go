package gemini

import (
	"strconv"
	"strings"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
	"github.com/valyala/fasthttp"
)

// ToGeminiError derives a GeminiGenerationError from a UnifAIError
func ToGeminiError(unifaiErr *schemas.UnifAIError) *GeminiGenerationError {
	if unifaiErr == nil {
		return nil
	}
	code := 500
	status := ""
	if unifaiErr.Error != nil && unifaiErr.Error.Type != nil {
		status = *unifaiErr.Error.Type
	}
	message := ""
	if unifaiErr.Error != nil && unifaiErr.Error.Message != "" {
		message = unifaiErr.Error.Message
	}
	if unifaiErr.StatusCode != nil {
		code = *unifaiErr.StatusCode
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
func parseGeminiError(resp *fasthttp.Response) *schemas.UnifAIError {
	// Try to parse as []GeminiGenerationError
	var errorResps []GeminiGenerationError
	unifaiErr := providerUtils.HandleProviderAPIError(resp, &errorResps)
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
		if unifaiErr.Error == nil {
			unifaiErr.Error = &schemas.ErrorField{}
		}
		// Set Code from first error if available
		if firstError != nil {
			unifaiErr.Error.Code = schemas.Ptr(strconv.Itoa(firstError.Code))
		}
		// Set Message to trimmed concatenated message
		unifaiErr.Error.Message = message
		return unifaiErr
	}

	// Try to parse as GeminiGenerationError
	var errorResp GeminiGenerationError
	unifaiErr = providerUtils.HandleProviderAPIError(resp, &errorResp)
	if errorResp.Error != nil {
		if unifaiErr.Error == nil {
			unifaiErr.Error = &schemas.ErrorField{}
		}
		unifaiErr.Error.Code = schemas.Ptr(strconv.Itoa(errorResp.Error.Code))
		unifaiErr.Error.Message = errorResp.Error.Message
	}
	return unifaiErr
}
