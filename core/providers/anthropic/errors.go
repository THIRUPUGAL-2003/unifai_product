package anthropic

import (
	"fmt"

	providerUtils "github.com/raksha/raksha/core/providers/utils"
	schemas "github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

// ToAnthropicChatCompletionError converts a RakshaError to AnthropicMessageError
func ToAnthropicChatCompletionError(rakshaErr *schemas.RakshaError) *AnthropicMessageError {
	if rakshaErr == nil {
		return nil
	}

	// Safely extract type and message from nested error
	errorType := "api_error"
	message := ""
	if rakshaErr.Error != nil {
		if rakshaErr.Error.Type != nil && *rakshaErr.Error.Type != "" {
			errorType = *rakshaErr.Error.Type
		}
		message = rakshaErr.Error.Message
	}

	// Handle nested error fields with nil checks
	errorStruct := AnthropicMessageErrorStruct{
		Type:    errorType,
		Message: message,
	}

	return &AnthropicMessageError{
		Type:  "error", // always "error" for Anthropic
		Error: errorStruct,
	}
}

// ToAnthropicResponsesStreamError converts a RakshaError to Anthropic responses streaming error in SSE format
func ToAnthropicResponsesStreamError(rakshaErr *schemas.RakshaError) string {
	if rakshaErr == nil {
		return ""
	}

	anthropicErr := ToAnthropicChatCompletionError(rakshaErr)

	// Marshal to JSON
	jsonData, err := providerUtils.MarshalSorted(anthropicErr)
	if err != nil {
		return ""
	}

	// Format as Anthropic SSE error event
	return fmt.Sprintf("event: error\ndata: %s\n\n", jsonData)
}

func parseAnthropicError(resp *fasthttp.Response) *schemas.RakshaError {
	var errorResp AnthropicError
	rakshaErr := providerUtils.HandleProviderAPIError(resp, &errorResp)
	if errorResp.Error != nil {
		if rakshaErr.Error == nil {
			rakshaErr.Error = &schemas.ErrorField{}
		}
		rakshaErr.Error.Type = &errorResp.Error.Type
		rakshaErr.Error.Message = errorResp.Error.Message
	}
	return rakshaErr
}
