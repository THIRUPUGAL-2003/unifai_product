package gemini

import (
	"strconv"
	"strings"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// ToGeminiError derives a GeminiGenerationError from a RakshaError
func ToGeminiError(rakshaErr *schemas.RakshaError) *GeminiGenerationError {
	if rakshaErr == nil {
		return nil
	}
	code := 500
	status := ""
	if rakshaErr.Error != nil && rakshaErr.Error.Type != nil {
		status = *rakshaErr.Error.Type
	}
	message := ""
	if rakshaErr.Error != nil && rakshaErr.Error.Message != "" {
		message = rakshaErr.Error.Message
	}
	if rakshaErr.StatusCode != nil {
		code = *rakshaErr.StatusCode
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
func parseGeminiError(resp *fasthttp.Response) *schemas.RakshaError {
	// Try to parse as []GeminiGenerationError
	var errorResps []GeminiGenerationError
	rakshaErr := providerUtils.HandleProviderAPIError(resp, &errorResps)
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
		if rakshaErr.Error == nil {
			rakshaErr.Error = &schemas.ErrorField{}
		}
		// Set Code from first error if available
		if firstError != nil {
			rakshaErr.Error.Code = schemas.Ptr(strconv.Itoa(firstError.Code))
		}
		// Set Message to trimmed concatenated message
		rakshaErr.Error.Message = message
		return rakshaErr
	}

	// Try to parse as GeminiGenerationError
	var errorResp GeminiGenerationError
	rakshaErr = providerUtils.HandleProviderAPIError(resp, &errorResp)
	if errorResp.Error != nil {
		if rakshaErr.Error == nil {
			rakshaErr.Error = &schemas.ErrorField{}
		}
		rakshaErr.Error.Code = schemas.Ptr(strconv.Itoa(errorResp.Error.Code))
		rakshaErr.Error.Message = errorResp.Error.Message
	}
	return rakshaErr
}
