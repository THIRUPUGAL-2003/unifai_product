package anthropic

import (
	"fmt"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	schemas "github.com/gateway/gateway/core/schemas"
	"github.com/valyala/fasthttp"
)

// ToAnthropicChatCompletionError converts a GatewayError to AnthropicMessageError
func ToAnthropicChatCompletionError(gatewayErr *schemas.GatewayError) *AnthropicMessageError {
	if gatewayErr == nil {
		return nil
	}

	// Safely extract type and message from nested error
	errorType := "api_error"
	message := ""
	if gatewayErr.Error != nil {
		if gatewayErr.Error.Type != nil && *gatewayErr.Error.Type != "" {
			errorType = *gatewayErr.Error.Type
		}
		message = gatewayErr.Error.Message
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

// ToAnthropicResponsesStreamError converts a GatewayError to Anthropic responses streaming error in SSE format
func ToAnthropicResponsesStreamError(gatewayErr *schemas.GatewayError) string {
	if gatewayErr == nil {
		return ""
	}

	anthropicErr := ToAnthropicChatCompletionError(gatewayErr)

	// Marshal to JSON
	jsonData, err := providerUtils.MarshalSorted(anthropicErr)
	if err != nil {
		return ""
	}

	// Format as Anthropic SSE error event
	return fmt.Sprintf("event: error\ndata: %s\n\n", jsonData)
}

func parseAnthropicError(resp *fasthttp.Response) *schemas.GatewayError {
	var errorResp AnthropicError
	gatewayErr := providerUtils.HandleProviderAPIError(resp, &errorResp)
	if errorResp.Error != nil {
		if gatewayErr.Error == nil {
			gatewayErr.Error = &schemas.ErrorField{}
		}
		gatewayErr.Error.Type = &errorResp.Error.Type
		gatewayErr.Error.Message = errorResp.Error.Message
	}
	return gatewayErr
}
