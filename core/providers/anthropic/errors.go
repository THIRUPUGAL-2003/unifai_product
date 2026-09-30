package anthropic

import (
	"fmt"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	schemas "github.com/unifai/unifai/core/schemas"
	"github.com/valyala/fasthttp"
)

// ToAnthropicChatCompletionError converts a UnifAIError to AnthropicMessageError
func ToAnthropicChatCompletionError(unifaiErr *schemas.UnifAIError) *AnthropicMessageError {
	if unifaiErr == nil {
		return nil
	}

	// Safely extract type and message from nested error
	errorType := "api_error"
	message := ""
	if unifaiErr.Error != nil {
		if unifaiErr.Error.Type != nil && *unifaiErr.Error.Type != "" {
			errorType = *unifaiErr.Error.Type
		}
		message = unifaiErr.Error.Message
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

// ToAnthropicResponsesStreamError converts a UnifAIError to Anthropic responses streaming error in SSE format
func ToAnthropicResponsesStreamError(unifaiErr *schemas.UnifAIError) string {
	if unifaiErr == nil {
		return ""
	}

	anthropicErr := ToAnthropicChatCompletionError(unifaiErr)

	// Marshal to JSON
	jsonData, err := providerUtils.MarshalSorted(anthropicErr)
	if err != nil {
		return ""
	}

	// Format as Anthropic SSE error event
	return fmt.Sprintf("event: error\ndata: %s\n\n", jsonData)
}

func parseAnthropicError(resp *fasthttp.Response) *schemas.UnifAIError {
	var errorResp AnthropicError
	unifaiErr := providerUtils.HandleProviderAPIError(resp, &errorResp)
	if errorResp.Error != nil {
		if unifaiErr.Error == nil {
			unifaiErr.Error = &schemas.ErrorField{}
		}
		unifaiErr.Error.Type = &errorResp.Error.Type
		unifaiErr.Error.Message = errorResp.Error.Message
	}
	return unifaiErr
}
