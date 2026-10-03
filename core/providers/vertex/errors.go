package vertex

import (
	"errors"
	"strings"

	"github.com/bytedance/sonic"
	providerUtils "github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
	"github.com/valyala/fasthttp"
)

func parseVertexError(resp *fasthttp.Response) *schemas.RakshaError {
	var openAIErr schemas.RakshaError
	var vertexErr []VertexError

	decodedBody, err := providerUtils.CheckAndDecodeBody(resp)
	if err != nil {
		rakshaErr := providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseDecode, err)
		return rakshaErr
	}

	// Check for empty response
	trimmed := strings.TrimSpace(string(decodedBody))
	if len(trimmed) == 0 {
		rakshaErr := &schemas.RakshaError{
			IsRakshaError: false,
			StatusCode:     schemas.Ptr(resp.StatusCode()),
			Error: &schemas.ErrorField{
				Message: schemas.ErrProviderResponseEmpty,
			},
		}
		return rakshaErr
	}

	// Check for HTML error response before attempting JSON parsing
	if providerUtils.IsHTMLResponse(resp, decodedBody) {
		rakshaErr := &schemas.RakshaError{
			IsRakshaError: false,
			StatusCode:     schemas.Ptr(resp.StatusCode()),
			Error: &schemas.ErrorField{
				Message: schemas.ErrProviderResponseHTML,
				Error:   errors.New(string(decodedBody)),
			},
			ExtraFields: schemas.RakshaErrorExtraFields{
				RawResponse: string(decodedBody),
			},
		}
		return rakshaErr
	}

	createError := func(message string) *schemas.RakshaError {
		rakshaErr := providerUtils.NewProviderAPIError(message, nil, resp.StatusCode(), nil, nil)
		var rawResponse interface{}
		if err := sonic.Unmarshal(decodedBody, &rawResponse); err != nil {
			rawResponse = string(decodedBody)
		}
		rakshaErr.ExtraFields.RawResponse = rawResponse
		return rakshaErr
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
					rakshaErr := providerUtils.NewRakshaOperationError(schemas.ErrProviderResponseUnmarshal, err)
					return rakshaErr
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
