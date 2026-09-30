package vertex

import (
	"errors"
	"strings"

	"github.com/bytedance/sonic"
	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
	"github.com/valyala/fasthttp"
)

func parseVertexError(resp *fasthttp.Response) *schemas.UnifAIError {
	var openAIErr schemas.UnifAIError
	var vertexErr []VertexError

	decodedBody, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		unifaiErr := providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseDecode, err)
		return unifaiErr
	}

	// Check for empty response
	trimmed := strings.TrimSpace(string(decodedBody))
	if len(trimmed) == 0 {
		unifaiErr := &schemas.UnifAIError{
			IsUnifAIError: false,
			StatusCode:     schemas.Ptr(resp.StatusCode()),
			Error: &schemas.ErrorField{
				Message: schemas.ErrProviderResponseEmpty,
			},
		}
		return unifaiErr
	}

	// Check for HTML error response before attempting JSON parsing
	if providerUtils.IsHTMLResponse(resp, decodedBody) {
		unifaiErr := &schemas.UnifAIError{
			IsUnifAIError: false,
			StatusCode:     schemas.Ptr(resp.StatusCode()),
			Error: &schemas.ErrorField{
				Message: schemas.ErrProviderResponseHTML,
				Error:   errors.New(string(decodedBody)),
			},
			ExtraFields: schemas.UnifAIErrorExtraFields{
				RawResponse: string(decodedBody),
			},
		}
		return unifaiErr
	}

	createError := func(message string) *schemas.UnifAIError {
		unifaiErr := providerUtils.NewProviderAPIError(message, nil, resp.StatusCode(), nil, nil)
		var rawResponse interface{}
		if err := sonic.Unmarshal(decodedBody, &rawResponse); err != nil {
			rawResponse = string(decodedBody)
		}
		unifaiErr.ExtraFields.RawResponse = rawResponse
		return unifaiErr
	}

	if err := sonic.Unmarshal(decodedBody, &openAIErr); err != nil || openAIErr.Error == nil {
		// Try Vertex error format if OpenAI format fails or is incomplete
		if err := sonic.Unmarshal(decodedBody, &vertexErr); err != nil {
			//try with single Vertex error format
			var vertexErr VertexError
			if err := sonic.Unmarshal(decodedBody, &vertexErr); err != nil {
				// Try VertexValidationError format (validation errors from Mistral endpoint)
				var validationErr VertexValidationError
				if err := sonic.Unmarshal(decodedBody, &validationErr); err != nil {
					unifaiErr := providerUtils.NewUnifAIOperationError(schemas.ErrProviderResponseUnmarshal, err)
					return unifaiErr
				}
				if len(validationErr.Detail) > 0 {
					return createError(validationErr.Detail[0].Msg)
				}
				return createError("Unknown error")
			}
			return createError(vertexErr.Error.Message)
		}
		if len(vertexErr) > 0 {
			return createError(vertexErr[0].Error.Message)
		}
		return createError("Unknown error")
	}
	// OpenAI error format succeeded with valid Error field
	return createError(openAIErr.Error.Message)
}
