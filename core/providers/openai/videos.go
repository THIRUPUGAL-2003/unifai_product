package openai

import (
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net/http"

	providerUtils "github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
)

// ToOpenAIVideoGenerationRequest converts a UnifAI Video Request to OpenAI format
func ToOpenAIVideoGenerationRequest(unifaiReq *schemas.UnifAIVideoGenerationRequest) (*OpenAIVideoGenerationRequest, error) {
	if unifaiReq == nil || unifaiReq.Input == nil || unifaiReq.Input.Prompt == "" {
		return nil, fmt.Errorf("unifai request, input, or prompt is nil/empty")
	}

	req := &OpenAIVideoGenerationRequest{
		Model:  unifaiReq.Model,
		Prompt: unifaiReq.Input.Prompt,
	}

	if unifaiReq.Input.InputReference != nil {
		// convert base64 to bytes
		sanitizedURL, err := schemas.SanitizeImageURL(*unifaiReq.Input.InputReference)
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

	if unifaiReq.Params != nil {
		if unifaiReq.Params.Seconds != nil {
			req.Seconds = unifaiReq.Params.Seconds
		}

		// Validate and set size
		if unifaiReq.Params.Size != "" {
			// Check if the provided size is valid
			if ValidOpenAIVideoSizes[unifaiReq.Params.Size] {
				req.Size = unifaiReq.Params.Size
			} else {
				// Invalid size provided, use default
				req.Size = string(DefaultOpenAIVideoSize)
			}
		} else {
			// No size provided, use default
			req.Size = string(DefaultOpenAIVideoSize)
		}

		req.ExtraParams = unifaiReq.Params.ExtraParams
	}

	return req, nil
}

func ToOpenAIVideoRemixRequest(unifaiReq *schemas.UnifAIVideoRemixRequest) (*OpenAIVideoRemixRequest, error) {
	if unifaiReq == nil || unifaiReq.Input == nil || unifaiReq.Input.Prompt == "" {
		return nil, fmt.Errorf("unifai request, input, or prompt is nil/empty")
	}

	req := &OpenAIVideoRemixRequest{
		Prompt: unifaiReq.Input.Prompt,
	}

	return req, nil
}

func ToUnifAIVideoRemixRequest(openaiReq *OpenAIVideoRemixRequest) *schemas.UnifAIVideoRemixRequest {
	if openaiReq == nil || openaiReq.Prompt == "" {
		return nil
	}

	provider := openaiReq.Provider
	if provider == "" {
		provider = schemas.OpenAI
	}

	return &schemas.UnifAIVideoRemixRequest{
		ID:       openaiReq.ID,
		Provider: provider,
		Input: &schemas.VideoGenerationInput{
			Prompt: openaiReq.Prompt,
		},
	}
}

func (req *OpenAIVideoGenerationRequest) ToUnifAIVideoGenerationRequest(ctx *schemas.UnifAIContext) *schemas.UnifAIVideoGenerationRequest {
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

	return &schemas.UnifAIVideoGenerationRequest{
		Provider:  provider,
		Model:     model,
		Input:     input,
		Params:    &req.VideoGenerationParameters,
		Fallbacks: schemas.ParseFallbacks(req.Fallbacks),
	}
}

// parseVideoGenerationFormDataBodyFromRequest parses the video generation request and writes it to the multipart form.
func parseVideoGenerationFormDataBodyFromRequest(writer *multipart.Writer, openaiReq *OpenAIVideoGenerationRequest, providerName schemas.ModelProvider) *schemas.UnifAIError {
	// Add prompt field (required)
	if openaiReq.Prompt == "" {
		return providerUtils.NewUnifAIOperationError("prompt is required", nil)
	}
	if err := writer.WriteField("prompt", openaiReq.Prompt); err != nil {
		return providerUtils.NewUnifAIOperationError("failed to write prompt field", err)
	}

	// Add optional model field
	if openaiReq.Model != "" {
		if err := writer.WriteField("model", openaiReq.Model); err != nil {
			return providerUtils.NewUnifAIOperationError("failed to write model field", err)
		}
	}

	// Add optional seconds field
	if openaiReq.Seconds != nil {
		if err := writer.WriteField("seconds", *openaiReq.Seconds); err != nil {
			return providerUtils.NewUnifAIOperationError("failed to write seconds field", err)
		}
	}

	// Add optional size field
	if openaiReq.Size != "" {
		if err := writer.WriteField("size", openaiReq.Size); err != nil {
			return providerUtils.NewUnifAIOperationError("failed to write size field", err)
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
			return providerUtils.NewUnifAIOperationError("failed to create form part for input_reference", err)
		}
		if _, err := part.Write(openaiReq.InputReference); err != nil {
			return providerUtils.NewUnifAIOperationError("failed to write input_reference file data", err)
		}
	}

	// Close the multipart writer
	if err := writer.Close(); err != nil {
		return providerUtils.NewUnifAIOperationError("failed to close multipart writer", err)
	}

	return nil
}
