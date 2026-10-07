package openai

import (
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net/http"

	providerUtils "github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
)

// ToOpenAIVideoGenerationRequest converts a Gateway Video Request to OpenAI format
func ToOpenAIVideoGenerationRequest(gatewayReq *schemas.GatewayVideoGenerationRequest) (*OpenAIVideoGenerationRequest, error) {
	if gatewayReq == nil || gatewayReq.Input == nil || gatewayReq.Input.Prompt == "" {
		return nil, fmt.Errorf("gateway request, input, or prompt is nil/empty")
	}

	req := &OpenAIVideoGenerationRequest{
		Model:  gatewayReq.Model,
		Prompt: gatewayReq.Input.Prompt,
	}

	if gatewayReq.Input.InputReference != nil {
		// convert base64 to bytes
		sanitizedURL, err := schemas.SanitizeImageURL(*gatewayReq.Input.InputReference)
		if err != nil {
			return nil, fmt.Errorf("invalid input reference: %w", err)
		}
		urlInfo := schemas.ExtractURLTypeInfo(sanitizedURL)
		if urlInfo.DataURLWithoutPrefix != nil {
			bytes, err := base64.StdEncoding.DecodeString(*urlInfo.DataURLWithoutPrefix)
			if err != nil {
				return nil, fmt.Errorf("failed to decode base64 input reference: %w", err)
			}
			req.InputReference = bytes
		} else {
			return nil, fmt.Errorf("input_reference must be a base64 data URL (e.g. data:image/png;base64,...)")
		}
	}

	if gatewayReq.Params != nil {
		if gatewayReq.Params.Seconds != nil {
			req.Seconds = gatewayReq.Params.Seconds
		}

		// Validate and set size
		if gatewayReq.Params.Size != "" {
			// Check if the provided size is valid
			if ValidOpenAIVideoSizes[gatewayReq.Params.Size] {
				req.Size = gatewayReq.Params.Size
			} else {
				// Invalid size provided, use default
				req.Size = string(DefaultOpenAIVideoSize)
			}
		} else {
			// No size provided, use default
			req.Size = string(DefaultOpenAIVideoSize)
		}

		req.ExtraParams = gatewayReq.Params.ExtraParams
	}

	return req, nil
}

func ToOpenAIVideoRemixRequest(gatewayReq *schemas.GatewayVideoRemixRequest) (*OpenAIVideoRemixRequest, error) {
	if gatewayReq == nil || gatewayReq.Input == nil || gatewayReq.Input.Prompt == "" {
		return nil, fmt.Errorf("gateway request, input, or prompt is nil/empty")
	}

	req := &OpenAIVideoRemixRequest{
		Prompt: gatewayReq.Input.Prompt,
	}

	return req, nil
}

func ToGatewayVideoRemixRequest(openaiReq *OpenAIVideoRemixRequest) *schemas.GatewayVideoRemixRequest {
	if openaiReq == nil || openaiReq.Prompt == "" {
		return nil
	}

	provider := openaiReq.Provider
	if provider == "" {
		provider = schemas.OpenAI
	}

	return &schemas.GatewayVideoRemixRequest{
		ID:       openaiReq.ID,
		Provider: provider,
		Input: &schemas.VideoGenerationInput{
			Prompt: openaiReq.Prompt,
		},
	}
}

func (req *OpenAIVideoGenerationRequest) ToGatewayVideoGenerationRequest(ctx *schemas.GatewayContext) *schemas.GatewayVideoGenerationRequest {
	if req == nil {
		return nil
	}

	provider, model := schemas.ParseModelString(req.Model, "")

	input := &schemas.VideoGenerationInput{
		Prompt: req.Prompt,
	}
	if req.InputReference != nil {
		input.InputReference = schemas.Ptr(providerUtils.FileBytesToBase64DataURL(req.InputReference))
	}

	return &schemas.GatewayVideoGenerationRequest{
		Provider:  provider,
		Model:     model,
		Input:     input,
		Params:    &req.VideoGenerationParameters,
		Fallbacks: schemas.ParseFallbacks(req.Fallbacks),
	}
}

// parseVideoGenerationFormDataBodyFromRequest parses the video generation request and writes it to the multipart form.
func parseVideoGenerationFormDataBodyFromRequest(writer *multipart.Writer, openaiReq *OpenAIVideoGenerationRequest, providerName schemas.ModelProvider) *schemas.GatewayError {
	// Add prompt field (required)
	if openaiReq.Prompt == "" {
		return providerUtils.NewGatewayOperationError("prompt is required", nil)
	}
	if err := writer.WriteField("prompt", openaiReq.Prompt); err != nil {
		return providerUtils.NewGatewayOperationError("failed to write prompt field", err)
	}

	// Add optional model field
	if openaiReq.Model != "" {
		if err := writer.WriteField("model", openaiReq.Model); err != nil {
			return providerUtils.NewGatewayOperationError("failed to write model field", err)
		}
	}

	// Add optional seconds field
	if openaiReq.Seconds != nil {
		if err := writer.WriteField("seconds", *openaiReq.Seconds); err != nil {
			return providerUtils.NewGatewayOperationError("failed to write seconds field", err)
		}
	}

	// Add optional size field
	if openaiReq.Size != "" {
		if err := writer.WriteField("size", openaiReq.Size); err != nil {
			return providerUtils.NewGatewayOperationError("failed to write size field", err)
		}
	}

	// Add optional input_reference field (image or video file)
	if len(openaiReq.InputReference) > 0 {
		// Detect MIME type
		mimeType := http.DetectContentType(openaiReq.InputReference)

		// Validate and set proper MIME type
		validMimeTypes := map[string]bool{
			"image/jpeg": true,
			"image/png":  true,
			"image/webp": true,
			"video/mp4":  true,
		}

		if !validMimeTypes[mimeType] {
			// Default to image/png if not detected properly
			mimeType = "image/png"
		}

		// Determine filename based on MIME type
		var filename string
		switch mimeType {
		case "image/jpeg":
			filename = "input_reference.jpg"
		case "image/webp":
			filename = "input_reference.webp"
		case "video/mp4":
			filename = "input_reference.mp4"
		default:
			filename = "input_reference.png"
		}

		// Create form part with proper Content-Type header
		part, err := writer.CreatePart(map[string][]string{
			"Content-Disposition": {fmt.Sprintf(`form-data; name="input_reference"; filename="%s"`, filename)},
			"Content-Type":        {mimeType},
		})
		if err != nil {
			return providerUtils.NewGatewayOperationError("failed to create form part for input_reference", err)
		}
		if _, err := part.Write(openaiReq.InputReference); err != nil {
			return providerUtils.NewGatewayOperationError("failed to write input_reference file data", err)
		}
	}

	// Close the multipart writer
	if err := writer.Close(); err != nil {
		return providerUtils.NewGatewayOperationError("failed to close multipart writer", err)
	}

	return nil
}
