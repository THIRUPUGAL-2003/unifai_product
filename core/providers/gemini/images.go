package gemini

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
)

// ToRakshaImageGenerationRequest converts a Gemini generation request to a Raksha image generation request
func (request *GeminiGenerationRequest) ToRakshaImageGenerationRequest(ctx *schemas.RakshaContext) *schemas.RakshaImageGenerationRequest {
	if request == nil {
		return nil
	}

	// Parse provider from model string (e.g., "openai/gpt-image-1" -> provider="openai", model="gpt-image-1")
	// This allows cross-provider routing through the GenAI endpoint
	provider, model := schemas.ParseModelString(request.Model, "")

	rakshaReq := &schemas.RakshaImageGenerationRequest{
		Provider: provider,
		Model:    model,
		Input:    &schemas.ImageGenerationInput{},
		Params:   &schemas.ImageGenerationParameters{},
	}

	fallbacks := schemas.ParseFallbacks(request.Fallbacks)
	rakshaReq.Fallbacks = fallbacks

	// First, try to extract prompt from Imagen format (instances)
	if len(request.Instances) > 0 && request.Instances[0].Prompt != "" {
		rakshaReq.Input.Prompt = request.Instances[0].Prompt

		// Extract Imagen parameters
		if request.Parameters != nil {
			if request.Parameters.SampleCount != nil {
				rakshaReq.Params.N = request.Parameters.SampleCount
			}
			// Convert Imagen size format to standard format
			if request.Parameters.SampleImageSize != nil || request.Parameters.AspectRatio != nil {
				size := convertImagenFormatToSize(request.Parameters.SampleImageSize, request.Parameters.AspectRatio)
				if size != "" && strings.ToLower(size) != "auto" {
					rakshaReq.Params.Size = &size
				}
			}

			// Map additional parameters to ExtraParams if not in Raksha schema
			if rakshaReq.Params.ExtraParams == nil {
				rakshaReq.Params.ExtraParams = make(map[string]interface{})
			}

			if request.Parameters.PersonGeneration != nil {
				rakshaReq.Params.ExtraParams["personGeneration"] = *request.Parameters.PersonGeneration
			}
			if request.Parameters.Seed != nil {
				rakshaReq.Params.Seed = request.Parameters.Seed
			}
			if request.Parameters.NegativePrompt != nil {
				rakshaReq.Params.NegativePrompt = request.Parameters.NegativePrompt
			}
			if request.Parameters.Language != nil {
				rakshaReq.Params.ExtraParams["language"] = *request.Parameters.Language
			}
			if request.Parameters.EnhancePrompt != nil {
				rakshaReq.Params.ExtraParams["enhancePrompt"] = *request.Parameters.EnhancePrompt
			}
			if request.Parameters.AddWatermark != nil {
				rakshaReq.Params.ExtraParams["addWatermark"] = *request.Parameters.AddWatermark
			}
			if len(request.Parameters.SafetySettings) > 0 {
				rakshaReq.Params.ExtraParams["safetySettings"] = request.Parameters.SafetySettings
			}
		}
		return rakshaReq
	}

	// Fall back to standard Gemini format (contents)
	if len(request.Contents) > 0 {
		for _, content := range request.Contents {
			for _, part := range content.Parts {
				if part != nil && part.Text != "" {
					rakshaReq.Input.Prompt = part.Text
					break
				}
			}
			if rakshaReq.Input.Prompt != "" {
				break
			}
		}
	}

	if request.GenerationConfig.ImageConfig != nil {
		ic := request.GenerationConfig.ImageConfig
		if strings.TrimSpace(ic.ImageSize) != "" || strings.TrimSpace(ic.AspectRatio) != "" {
			size := convertImagenFormatToSize(&ic.ImageSize, &ic.AspectRatio)
			if size != "" {
				rakshaReq.Params.Size = &size
			}
		}
	}

	return rakshaReq
}

func (request *GeminiGenerationRequest) ToRakshaImageEditRequest(ctx *schemas.RakshaContext) *schemas.RakshaImageEditRequest {
	if request == nil {
		return nil
	}

	// Parse provider from model string (e.g., "openai/gpt-image-1" -> provider="openai", model="gpt-image-1")
	// This allows cross-provider routing through the GenAI endpoint
	provider, model := schemas.ParseModelString(request.Model, "")

	rakshaReq := &schemas.RakshaImageEditRequest{
		Provider: provider,
		Model:    model,
		Input:    &schemas.ImageEditInput{},
		Params:   &schemas.ImageEditParameters{},
	}

	fallbacks := schemas.ParseFallbacks(request.Fallbacks)
	rakshaReq.Fallbacks = fallbacks

	// Initialize ExtraParams if not present
	if rakshaReq.Params.ExtraParams == nil {
		rakshaReq.Params.ExtraParams = make(map[string]interface{})
	}

	// First, try to extract prompt from Imagen format (instances)
	if len(request.Instances) > 0 && request.Instances[0].Prompt != "" {
		rakshaReq.Input.Prompt = request.Instances[0].Prompt

		// Extract all images from ReferenceImages using a loop
		var images []schemas.ImageInput
		var mask []byte
		var maskMode string
		var dilation *float64
		var maskClasses []int

		for _, refImage := range request.Instances[0].ReferenceImages {
			if refImage.ReferenceType == "REFERENCE_TYPE_RAW" {
				// Decode base64 image data
				imageBytes, err := base64.StdEncoding.DecodeString(refImage.ReferenceImage.BytesBase64Encoded)
				if err != nil {
					continue // Skip invalid images
				}
				images = append(images, schemas.ImageInput{
					Image: imageBytes,
				})
			} else if refImage.ReferenceType == "REFERENCE_TYPE_MASK" {
				// Extract mask data if present
				if refImage.ReferenceImage.BytesBase64Encoded != "" {
					maskBytes, err := base64.StdEncoding.DecodeString(refImage.ReferenceImage.BytesBase64Encoded)
					if err == nil {
						mask = maskBytes
					}
				}
				// Extract mask configuration
				if refImage.MaskImageConfig != nil {
					if refImage.MaskImageConfig.MaskMode != "" {
						maskMode = refImage.MaskImageConfig.MaskMode
					}
					if refImage.MaskImageConfig.Dilation != nil {
						dilation = refImage.MaskImageConfig.Dilation
					}
					if len(refImage.MaskImageConfig.MaskClasses) > 0 {
						maskClasses = refImage.MaskImageConfig.MaskClasses
					}

				}
			}
		}

		// Set mask if present
		if len(mask) > 0 {
			rakshaReq.Params.Mask = mask
		}

		// Store mask configuration in ExtraParams
		if maskMode != "" {
			rakshaReq.Params.ExtraParams["maskMode"] = maskMode
		}
		if dilation != nil {
			rakshaReq.Params.ExtraParams["dilation"] = *dilation
		}
		if len(maskClasses) > 0 {
			rakshaReq.Params.ExtraParams["maskClasses"] = maskClasses
		}

		if len(images) == 0 {
			return nil // No valid images found
		}
		rakshaReq.Input.Images = images

		// Extract Imagen parameters
		if request.Parameters != nil {
			if request.Parameters.SampleCount != nil {
				rakshaReq.Params.N = request.Parameters.SampleCount
			}
			// Convert Imagen size format to standard format
			if request.Parameters.SampleImageSize != nil || request.Parameters.AspectRatio != nil {
				size := convertImagenFormatToSize(request.Parameters.SampleImageSize, request.Parameters.AspectRatio)
				if size != "" && strings.ToLower(size) != "auto" {
					rakshaReq.Params.Size = &size
				}
			}

			// Extract output format and compression from OutputOptions
			if request.Parameters.OutputOptions != nil {
				if request.Parameters.OutputOptions.MimeType != nil {
					outputFormat := convertMimeTypeToExtension(*request.Parameters.OutputOptions.MimeType)
					if outputFormat != "" {
						rakshaReq.Params.OutputFormat = &outputFormat
					}
				}
				if request.Parameters.OutputOptions.CompressionQuality != nil {
					rakshaReq.Params.OutputCompression = request.Parameters.OutputOptions.CompressionQuality
				}
			}

			// Extract edit mode and map to type
			if request.Parameters.EditMode != nil {
				editType := mapImagenEditModeToType(*request.Parameters.EditMode)
				if editType != "" {
					rakshaReq.Params.Type = &editType
				}
			}

			if request.Parameters.Seed != nil {
				rakshaReq.Params.Seed = request.Parameters.Seed
			}
			if request.Parameters.NegativePrompt != nil {
				rakshaReq.Params.NegativePrompt = request.Parameters.NegativePrompt
			}

			if request.Parameters.PersonGeneration != nil {
				rakshaReq.Params.ExtraParams["personGeneration"] = *request.Parameters.PersonGeneration
			}
			if request.Parameters.Language != nil {
				rakshaReq.Params.ExtraParams["language"] = *request.Parameters.Language
			}
			if request.Parameters.EnhancePrompt != nil {
				rakshaReq.Params.ExtraParams["enhancePrompt"] = *request.Parameters.EnhancePrompt
			}
			if request.Parameters.AddWatermark != nil {
				rakshaReq.Params.ExtraParams["addWatermark"] = *request.Parameters.AddWatermark
			}
			if len(request.Parameters.SafetySettings) > 0 {
				rakshaReq.Params.ExtraParams["safetySettings"] = request.Parameters.SafetySettings
			}
			if request.Parameters.GuidanceScale != nil {
				rakshaReq.Params.ExtraParams["guidanceScale"] = *request.Parameters.GuidanceScale
			}
			if request.Parameters.BaseSteps != nil {
				rakshaReq.Params.ExtraParams["baseSteps"] = *request.Parameters.BaseSteps
			}
			if request.Parameters.IncludeRaiReason != nil {
				rakshaReq.Params.ExtraParams["includeRaiReason"] = *request.Parameters.IncludeRaiReason
			}
			if request.Parameters.IncludeSafetyAttributes != nil {
				rakshaReq.Params.ExtraParams["includeSafetyAttributes"] = *request.Parameters.IncludeSafetyAttributes
			}
			if request.Parameters.StorageUri != nil {
				rakshaReq.Params.ExtraParams["storageUri"] = *request.Parameters.StorageUri
			}
		}
		return rakshaReq
	}

	// Fall back to standard Gemini format (contents)
	if len(request.Contents) > 0 {
		var images []schemas.ImageInput
		for _, content := range request.Contents {
			for _, part := range content.Parts {
				if part != nil {
					if part.Text != "" {
						rakshaReq.Input.Prompt = part.Text
					}
					// Extract images from InlineData
					if part.InlineData != nil && part.InlineData.Data != "" {
						imageBytes, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
						if err == nil {
							images = append(images, schemas.ImageInput{
								Image: imageBytes,
							})
						}
					}
				}
			}
		}
		if len(images) > 0 {
			rakshaReq.Input.Images = images
		}
	}

	if request.GenerationConfig.ImageConfig != nil {
		ic := request.GenerationConfig.ImageConfig
		if strings.TrimSpace(ic.ImageSize) != "" || strings.TrimSpace(ic.AspectRatio) != "" {
			size := convertImagenFormatToSize(&ic.ImageSize, &ic.AspectRatio)
			if size != "" {
				rakshaReq.Params.Size = &size
			}
		}
	}

	return rakshaReq
}

// convertImagenFormatToSize converts Imagen sampleImageSize and aspectRatio to standard WxH format
// Supports aspect ratios: "1:1", "3:4", "4:3", "9:16", "16:9" (supported for Imagen models)
func convertImagenFormatToSize(sampleImageSize *string, aspectRatio *string) string {
	// Default size based on imageSize parameter
	baseSize := 1024
	if sampleImageSize != nil {
		normalizedSize := strings.ToLower(strings.TrimSpace(*sampleImageSize))
		switch normalizedSize {
		case "1k":
			baseSize = 1024
		case "2k":
			baseSize = 2048
		case "4k":
			baseSize = 4096
		}
	}

	// Apply aspect ratio
	if aspectRatio != nil {
		switch strings.TrimSpace(*aspectRatio) {
		case "1:1":
			return strconv.Itoa(baseSize) + "x" + strconv.Itoa(baseSize)
		case "3:4":
			return strconv.Itoa(baseSize*3/4) + "x" + strconv.Itoa(baseSize)
		case "4:3":
			return strconv.Itoa(baseSize) + "x" + strconv.Itoa(baseSize*3/4)
		case "9:16":
			return strconv.Itoa(baseSize*9/16) + "x" + strconv.Itoa(baseSize)
		case "16:9":
			return strconv.Itoa(baseSize) + "x" + strconv.Itoa(baseSize*9/16)
		}
	}

	// Default to square
	return strconv.Itoa(baseSize) + "x" + strconv.Itoa(baseSize)
}

func (response *GenerateContentResponse) ToRakshaImageGenerationResponse() (*schemas.RakshaImageGenerationResponse, *schemas.RakshaError) {
	rakshaResp := &schemas.RakshaImageGenerationResponse{
		ID:    response.ResponseID,
		Model: response.ModelVersion,
		Data:  []schemas.ImageData{},
	}

	// Process candidates to extract image data
	if len(response.Candidates) > 0 {
		candidate := response.Candidates[0]
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			var imageData []schemas.ImageData
			var imageMetadata []schemas.ImageGenerationResponseParameters

			// Extract image data from all parts
			for idx, part := range candidate.Content.Parts {
				// Check that part is not nil before accessing its fields
				if part != nil && part.InlineData != nil {
					imageData = append(imageData, schemas.ImageData{
						B64JSON: part.InlineData.Data,
						Index:   idx,
					})
					// Convert MIME type to file extension for OutputFormat
					outputFormat := convertMimeTypeToExtension(part.InlineData.MIMEType)
					imageMetadata = append(imageMetadata, schemas.ImageGenerationResponseParameters{
						OutputFormat: outputFormat,
					})
				}
			}

			// Set usage information with modality details
			rakshaResp.Usage = convertGeminiUsageMetadataToImageUsage(response.UsageMetadata)
			// Only assign imageData when it has elements
			if len(imageData) > 0 {
				rakshaResp.Data = imageData
				// Only set ImageGenerationResponseParameters when metadata exists
				if len(imageMetadata) > 0 {
					rakshaResp.ImageGenerationResponseParameters = &imageMetadata[0]
				}
			}
		} else {
			return nil, &schemas.RakshaError{
				IsRakshaError: false,
				Error: &schemas.ErrorField{
					Message: candidate.FinishMessage,
					Code:    schemas.Ptr(string(candidate.FinishReason)),
				},
			}
		}
	} else {
		return nil, &schemas.RakshaError{
			IsRakshaError: false,
			Error: &schemas.ErrorField{
				Message: "No candidates found in response",
			},
		}
	}

	return rakshaResp, nil
}

func ToGeminiImageGenerationRequest(rakshaReq *schemas.RakshaImageGenerationRequest) *GeminiGenerationRequest {
	if rakshaReq == nil {
		return nil
	}

	rakshaReq.Model = NormalizeModelName(rakshaReq.Model)

	// Create the base Gemini generation request
	geminiReq := &GeminiGenerationRequest{
		Model: rakshaReq.Model,
	}
	geminiReq.ExtraParams = rakshaReq.Params.ExtraParams

	// Set response modalities to indicate this is an image generation request
	geminiReq.GenerationConfig.ResponseModalities = []Modality{ModalityImage}

	// Convert parameters to generation config
	if rakshaReq.Params != nil {

		// Prefer explicit aspect_ratio; fall back to deriving aspect ratio + resolution from size.
		imageConfig := &GeminiImageConfig{}
		if rakshaReq.Params.Size != nil && strings.ToLower(*rakshaReq.Params.Size) != "auto" {
			aspectRatio, imageSize := utils.ConvertSizeToAspectRatioAndResolution(*rakshaReq.Params.Size)
			imageConfig.AspectRatio = aspectRatio
			imageConfig.ImageSize = imageSize
		}
		if rakshaReq.Params.AspectRatio != nil && *rakshaReq.Params.AspectRatio != "" {
			imageConfig.AspectRatio = *rakshaReq.Params.AspectRatio
		}
		if imageConfig.AspectRatio != "" || imageConfig.ImageSize != "" {
			geminiReq.GenerationConfig.ImageConfig = imageConfig
		}

		// Handle extra parameters
		if rakshaReq.Params.ExtraParams != nil {
			// Safety settings - support both camelCase (canonical) and snake_case (legacy) keys
			if safetySettings, ok := schemas.SafeExtractFromMap(rakshaReq.Params.ExtraParams, "safetySettings"); ok {
				delete(geminiReq.ExtraParams, "safetySettings")
				if settings, ok := SafeExtractSafetySettings(safetySettings); ok {
					geminiReq.SafetySettings = settings
				}
			} else if safetySettings, ok := schemas.SafeExtractFromMap(rakshaReq.Params.ExtraParams, "safety_settings"); ok {
				delete(geminiReq.ExtraParams, "safety_settings")
				if settings, ok := SafeExtractSafetySettings(safetySettings); ok {
					geminiReq.SafetySettings = settings
				}
			}

			// Cached content - support both camelCase (canonical) and snake_case (legacy) keys
			if cachedContent, ok := schemas.SafeExtractString(rakshaReq.Params.ExtraParams["cachedContent"]); ok {
				delete(geminiReq.ExtraParams, "cachedContent")
				geminiReq.CachedContent = cachedContent
			} else if cachedContent, ok := schemas.SafeExtractString(rakshaReq.Params.ExtraParams["cached_content"]); ok {
				delete(geminiReq.ExtraParams, "cached_content")
				geminiReq.CachedContent = cachedContent
			}

			// Labels
			if labels, ok := schemas.SafeExtractFromMap(rakshaReq.Params.ExtraParams, "labels"); ok {
				switch m := labels.(type) {
				case map[string]string:
					delete(geminiReq.ExtraParams, "labels")
					geminiReq.Labels = m
				case map[string]interface{}:
					out := make(map[string]string, len(m))
					for k, v := range m {
						if s, ok := schemas.SafeExtractString(v); ok {
							out[k] = s
						}
					}
					if len(out) > 0 {
						delete(geminiReq.ExtraParams, "labels")
						geminiReq.Labels = out
					}
				}
			}
		}
	}

	if rakshaReq.Input == nil {
		return nil
	}

	// Create parts for image gen request
	parts := []*Part{
		{
			Text: rakshaReq.Input.Prompt,
		},
	}

	geminiReq.Contents = []Content{
		{
			Role:  RoleUser,
			Parts: parts,
		},
	}

	// Note: Gemini image generation always returns a single image, so we do not propagate
	// rakshaReq.Params.N to GenerationConfig.CandidateCount. The N parameter is silently dropped.

	return geminiReq
}

// ToImagenImageGenerationRequest converts a Raksha Image Request to Imagen format
func ToImagenImageGenerationRequest(rakshaReq *schemas.RakshaImageGenerationRequest) *GeminiImagenRequest {
	if rakshaReq == nil || rakshaReq.Input == nil {
		return nil
	}

	rakshaReq.Model = NormalizeModelName(rakshaReq.Model)

	// Create instances array with prompt
	prompt := rakshaReq.Input.Prompt
	instances := []ImagenInstance{
		{
			Prompt: prompt,
		},
	}

	req := &GeminiImagenRequest{
		Instances:  instances,
		Parameters: GeminiImagenParameters{},
	}

	if rakshaReq.Params != nil {
		if rakshaReq.Params.N != nil {
			req.Parameters.SampleCount = rakshaReq.Params.N
		}

		// Handle size conversion
		if rakshaReq.Params.Size != nil && strings.ToLower(*rakshaReq.Params.Size) != "auto" {
			aspectRatio, imageSize := utils.ConvertSizeToAspectRatioAndResolution(*rakshaReq.Params.Size)
			if imageSize != "" {
				req.Parameters.SampleImageSize = &imageSize
			}
			if aspectRatio != "" {
				req.Parameters.AspectRatio = &aspectRatio
			}
		}

		// Explicit aspect_ratio overrides the size-derived ratio.
		if rakshaReq.Params.AspectRatio != nil && *rakshaReq.Params.AspectRatio != "" {
			req.Parameters.AspectRatio = rakshaReq.Params.AspectRatio
		}

		// Handle output format conversion to mimeType
		outputFormat := ""
		if rakshaReq.Params.OutputFormat != nil {
			outputFormat = *rakshaReq.Params.OutputFormat
		}

		if outputFormat != "" {
			mimeType := convertOutputFormatToMimeType(outputFormat)
			if mimeType != "" {
				if req.Parameters.OutputOptions == nil {
					req.Parameters.OutputOptions = &ImagenOutputOptions{}
				}
				req.Parameters.OutputOptions.MimeType = &mimeType
			}
		}

		if rakshaReq.Params.Seed != nil {
			req.Parameters.Seed = rakshaReq.Params.Seed
		}
		if rakshaReq.Params.NegativePrompt != nil {
			req.Parameters.NegativePrompt = rakshaReq.Params.NegativePrompt
		}

		// Handle extra parameters for Imagen-specific fields
		if rakshaReq.Params.ExtraParams != nil {
			req.ExtraParams = rakshaReq.Params.ExtraParams
			if addWatermark, ok := schemas.SafeExtractBoolPointer(rakshaReq.Params.ExtraParams["addWatermark"]); ok {
				delete(req.ExtraParams, "addWatermark")
				req.Parameters.AddWatermark = addWatermark
			}
			if sampleImageSize, ok := schemas.SafeExtractString(rakshaReq.Params.ExtraParams["sampleImageSize"]); ok {
				delete(req.ExtraParams, "sampleImageSize")
				req.Parameters.SampleImageSize = &sampleImageSize
			}

			if aspectRatio, ok := schemas.SafeExtractString(rakshaReq.Params.ExtraParams["aspectRatio"]); ok {
				delete(req.ExtraParams, "aspectRatio")
				req.Parameters.AspectRatio = &aspectRatio
			}

			if personGeneration, ok := schemas.SafeExtractString(rakshaReq.Params.ExtraParams["personGeneration"]); ok {
				delete(req.ExtraParams, "personGeneration")
				req.Parameters.PersonGeneration = &personGeneration
			}

			// Map language from ExtraParams
			if language, ok := schemas.SafeExtractString(rakshaReq.Params.ExtraParams["language"]); ok {
				delete(req.ExtraParams, "language")
				req.Parameters.Language = &language
			}

			// Map enhancePrompt from ExtraParams
			if enhancePrompt, ok := schemas.SafeExtractBoolPointer(rakshaReq.Params.ExtraParams["enhancePrompt"]); ok {
				delete(req.ExtraParams, "enhancePrompt")
				req.Parameters.EnhancePrompt = enhancePrompt
			}

			// Map safetySettings from ExtraParams
			if safetySettings, ok := schemas.SafeExtractFromMap(rakshaReq.Params.ExtraParams, "safetySettings"); ok {
				if settings, ok := SafeExtractSafetySettings(safetySettings); ok {
					delete(req.ExtraParams, "safetySettings")
					req.Parameters.SafetySettings = settings
				}
			}
		}
	}

	return req
}

// convertMimeTypeToExtension converts MIME type to file extension
// Maps "image/png" -> "png", "image/jpeg" -> "jpeg", "image/webp" -> "webp"
// For unknown MIME types, extracts the subtype after '/' as fallback
func convertMimeTypeToExtension(mimeType string) string {
	if mimeType == "" {
		return ""
	}
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if semi := strings.Index(mimeType, ";"); semi != -1 {
		mimeType = strings.TrimSpace(mimeType[:semi])
	}
	switch mimeType {
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg":
		return "jpeg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	default:
		// Extract subtype after '/' if present, otherwise return empty string
		if idx := strings.Index(mimeType, "/"); idx != -1 && idx+1 < len(mimeType) {
			return mimeType[idx+1:]
		}
		return ""
	}
}

// convertOutputFormatToMimeType converts Raksha output_format to Imagen mimeType
// Maps "png" -> "image/png", "jpg"/"jpeg" -> "image/jpeg", "webp" -> "image/webp"
// Returns empty string for unsupported formats
func convertOutputFormatToMimeType(outputFormat string) string {
	format := strings.ToLower(strings.TrimSpace(outputFormat))
	switch format {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

// ToRakshaImageGenerationResponse converts an Imagen response to Raksha format
func (response *GeminiImagenResponse) ToRakshaImageGenerationResponse() *schemas.RakshaImageGenerationResponse {
	if response == nil {
		return nil
	}

	rakshaResp := &schemas.RakshaImageGenerationResponse{
		Data: make([]schemas.ImageData, len(response.Predictions)),
	}

	// Convert each prediction to ImageData
	for i, prediction := range response.Predictions {
		rakshaResp.Data[i] = schemas.ImageData{
			B64JSON: prediction.BytesBase64Encoded,
			Index:   i,
		}

		// Set output format from MIME type if available
		if prediction.MimeType != "" && i == 0 {
			// Convert MIME type to file extension for OutputFormat
			outputFormat := convertMimeTypeToExtension(prediction.MimeType)
			rakshaResp.ImageGenerationResponseParameters = &schemas.ImageGenerationResponseParameters{
				OutputFormat: outputFormat,
			}
		}
	}

	return rakshaResp
}

// ToGeminiImageGenerationResponse converts a RakshaImageGenerationResponse back to Gemini format
func ToGeminiImageGenerationResponse(ctx context.Context, rakshaResp *schemas.RakshaImageGenerationResponse) (*GenerateContentResponse, error) {
	if rakshaResp == nil {
		return nil, nil
	}

	geminiResp := &GenerateContentResponse{
		ResponseID:   rakshaResp.ID,
		ModelVersion: rakshaResp.Model,
	}

	// Convert image data to candidate parts
	if len(rakshaResp.Data) > 0 {
		parts := make([]*Part, 0, len(rakshaResp.Data))
		for i := range rakshaResp.Data {
			imageData := &rakshaResp.Data[i]
			// Determine MIME type - convert file extension back to MIME type
			mimeType := "image/png" // default
			if rakshaResp.ImageGenerationResponseParameters != nil && rakshaResp.ImageGenerationResponseParameters.OutputFormat != "" {
				mimeType = convertOutputFormatToMimeType(rakshaResp.ImageGenerationResponseParameters.OutputFormat)
				if mimeType == "" {
					// Fallback: if conversion fails, assume PNG
					mimeType = "image/png"
				}
			}
			if imageData.B64JSON == "" && imageData.URL != "" {
				// Fetch image from URL with context-aware timeout and size limit
				downloadedImage, err := downloadImageFromURL(ctx, imageData.URL)
				if err != nil {
					return nil, fmt.Errorf("failed to download image from URL: %w", err)
				}
				imageData.B64JSON = downloadedImage
			}
			part := &Part{
				InlineData: &Blob{
					Data:     imageData.B64JSON,
					MIMEType: mimeType,
				},
			}
			parts = append(parts, part)
		}

		geminiResp.Candidates = []*Candidate{
			{
				Content: &Content{
					Role:  RoleModel,
					Parts: parts,
				},
				FinishReason: FinishReasonStop,
			},
		}
	}

	// Convert usage metadata with modality details
	geminiResp.UsageMetadata = convertRakshaImageUsageToGeminiUsageMetadata(rakshaResp.Usage)

	return geminiResp, nil
}

func ToGeminiImageEditRequest(rakshaReq *schemas.RakshaImageEditRequest) *GeminiGenerationRequest {
	if rakshaReq == nil || rakshaReq.Input == nil || len(rakshaReq.Input.Images) == 0 {
		return nil
	}

	rakshaReq.Model = NormalizeModelName(rakshaReq.Model)

	// Create the base Gemini generation request
	geminiReq := &GeminiGenerationRequest{
		Model: rakshaReq.Model,
	}
	// Set response modalities to indicate this is an image generation request
	geminiReq.GenerationConfig.ResponseModalities = []Modality{ModalityImage}

	// Convert parameters to generation config
	if rakshaReq.Params != nil {
		geminiReq.ExtraParams = rakshaReq.Params.ExtraParams

		// Derive aspect ratio + resolution from size (edit params carry no typed aspect_ratio).
		if rakshaReq.Params.Size != nil && strings.ToLower(*rakshaReq.Params.Size) != "auto" {
			aspectRatio, imageSize := utils.ConvertSizeToAspectRatioAndResolution(*rakshaReq.Params.Size)
			if aspectRatio != "" || imageSize != "" {
				geminiReq.GenerationConfig.ImageConfig = &GeminiImageConfig{
					ImageSize:   imageSize,
					AspectRatio: aspectRatio,
				}
			}
		}

		// Handle extra parameters
		if rakshaReq.Params.ExtraParams != nil {
			// Safety settings - support both camelCase (canonical) and snake_case (legacy) keys
			if safetySettings, ok := schemas.SafeExtractFromMap(rakshaReq.Params.ExtraParams, "safetySettings"); ok {
				delete(geminiReq.ExtraParams, "safetySettings")
				if settings, ok := SafeExtractSafetySettings(safetySettings); ok {
					geminiReq.SafetySettings = settings
				}
			} else if safetySettings, ok := schemas.SafeExtractFromMap(rakshaReq.Params.ExtraParams, "safety_settings"); ok {
				delete(geminiReq.ExtraParams, "safety_settings")
				if settings, ok := SafeExtractSafetySettings(safetySettings); ok {
					geminiReq.SafetySettings = settings
				}
			}

			// Cached content - support both camelCase (canonical) and snake_case (legacy) keys
			if cachedContent, ok := schemas.SafeExtractString(rakshaReq.Params.ExtraParams["cachedContent"]); ok {
				delete(geminiReq.ExtraParams, "cachedContent")
				geminiReq.CachedContent = cachedContent
			} else if cachedContent, ok := schemas.SafeExtractString(rakshaReq.Params.ExtraParams["cached_content"]); ok {
				delete(geminiReq.ExtraParams, "cached_content")
				geminiReq.CachedContent = cachedContent
			}

			// Labels
			if labels, ok := schemas.SafeExtractFromMap(rakshaReq.Params.ExtraParams, "labels"); ok {
				switch m := labels.(type) {
				case map[string]string:
					delete(geminiReq.ExtraParams, "labels")
					geminiReq.Labels = m
				case map[string]interface{}:
					out := make(map[string]string, len(m))
					for k, v := range m {
						if s, ok := schemas.SafeExtractString(v); ok {
							out[k] = s
						}
					}
					if len(out) > 0 {
						delete(geminiReq.ExtraParams, "labels")
						geminiReq.Labels = out
					}
				}
			}
		}
	}

	if rakshaReq.Input == nil {
		return nil
	}

	// Create parts for image gen request
	parts := []*Part{
		{
			Text: rakshaReq.Input.Prompt,
		},
	}

	for _, image := range rakshaReq.Input.Images {
		// Detect MIME type from image bytes
		mimeType := http.DetectContentType(image.Image)
		// Fallback to PNG if detection fails
		if mimeType == "application/octet-stream" || mimeType == "" {
			mimeType = "image/png"
		}

		parts = append(parts, &Part{
			InlineData: &Blob{
				MIMEType: mimeType,
				Data:     encodeBytesToBase64String(image.Image),
			},
		})
	}

	geminiReq.Contents = []Content{
		{
			Role:  RoleUser,
			Parts: parts,
		},
	}

	return geminiReq
}

// extractIntArray safely extracts an array of integers from an interface{} value
func extractIntArray(v interface{}) []int {
	if v == nil {
		return nil
	}

	// Handle []interface{} (common JSON unmarshaling result)
	if arr, ok := v.([]interface{}); ok {
		result := make([]int, 0, len(arr))
		for _, item := range arr {
			switch val := item.(type) {
			case int:
				result = append(result, val)
			case int64:
				result = append(result, int(val))
			case float64:
				result = append(result, int(val))
			}
		}
		return result
	}

	// Handle []int directly
	if arr, ok := v.([]int); ok {
		return arr
	}

	// Handle []int64
	if arr, ok := v.([]int64); ok {
		result := make([]int, len(arr))
		for i, val := range arr {
			result[i] = int(val)
		}
		return result
	}

	return nil
}

// mapTypeToImagenEditMode maps Raksha image edit type to Imagen editMode
// Supported edit modes:
//   - "inpainting" -> EDIT_MODE_INPAINT_INSERTION: Add objects from a given prompt
//   - "outpainting" -> EDIT_MODE_OUTPAINT: Extend image beyond its borders
//   - "inpaint_removal" -> EDIT_MODE_INPAINT_REMOVAL: Remove objects and fill in background
//   - "bgswap" -> EDIT_MODE_BGSWAP: Swap background while preserving foreground objects
func mapTypeToImagenEditMode(editType string) string {
	switch strings.ToLower(editType) {
	case "inpainting":
		return "EDIT_MODE_INPAINT_INSERTION"
	case "outpainting":
		return "EDIT_MODE_OUTPAINT"
	case "inpaint_removal":
		return "EDIT_MODE_INPAINT_REMOVAL"
	case "bgswap":
		return "EDIT_MODE_BGSWAP"
	default:
		return ""
	}
}

// mapImagenEditModeToType maps Imagen editMode to Raksha image edit type
// This is the reverse mapping of mapTypeToImagenEditMode
func mapImagenEditModeToType(editMode string) string {
	switch strings.ToUpper(editMode) {
	case "EDIT_MODE_INPAINT_INSERTION":
		return "inpainting"
	case "EDIT_MODE_OUTPAINT":
		return "outpainting"
	case "EDIT_MODE_INPAINT_REMOVAL":
		return "inpainting"
	case "EDIT_MODE_BGSWAP":
		return "bgswap"
	default:
		return ""
	}
}

// ToImagenImageEditRequest converts a RakshaImageEditRequest to Imagen edit format
// Mask modes (via ExtraParams["maskMode"]):
//   - MASK_MODE_USER_PROVIDED: Use the mask from Params.Mask (default if mask is provided)
//   - MASK_MODE_BACKGROUND: Auto-generated mask from background segmentation
//   - MASK_MODE_FOREGROUND: Auto-generated mask from foreground segmentation
//   - MASK_MODE_SEMANTIC: Auto-generated mask from semantic segmentation with mask class
//
// Mask dilation (via ExtraParams["dilation"]):
//   - Optional float in range [0, 1]. Percentage of image width to dilate (grow) the mask by
//   - Recommended values by edit mode:
//   - EDIT_MODE_INPAINT_INSERTION: 0.01
//   - EDIT_MODE_INPAINT_REMOVAL: 0.01
//   - EDIT_MODE_BGSWAP: 0.0
//   - EDIT_MODE_OUTPAINT: 0.01-0.03
//
// Mask classes (via ExtraParams["maskClasses"]):
//   - Optional list of integers. Mask classes for MASK_MODE_SEMANTIC mode
func ToImagenImageEditRequest(rakshaReq *schemas.RakshaImageEditRequest) *GeminiImagenRequest {
	if rakshaReq == nil || rakshaReq.Input == nil || len(rakshaReq.Input.Images) == 0 {
		return nil
	}

	rakshaReq.Model = NormalizeModelName(rakshaReq.Model)

	req := &GeminiImagenRequest{
		Parameters: GeminiImagenParameters{},
	}

	var refImages []ImagenReferenceImage
	refID := 1

	for _, img := range rakshaReq.Input.Images {
		refImages = append(refImages, ImagenReferenceImage{
			ReferenceType: "REFERENCE_TYPE_RAW",
			ReferenceID:   refID,
			ReferenceImage: ImagenReferenceData{
				BytesBase64Encoded: base64.StdEncoding.EncodeToString(img.Image),
			},
		})
		refID++
	}

	// Handle mask configuration
	if rakshaReq.Params != nil {
		var maskMode string
		var hasMaskData bool
		var dilation *float64
		var maskClasses []int
		req.ExtraParams = rakshaReq.Params.ExtraParams
		// Check if user provided a mask
		if len(rakshaReq.Params.Mask) > 0 {
			hasMaskData = true
			maskMode = "MASK_MODE_USER_PROVIDED" // Default when mask is provided
		}

		// Extract optional parameters from ExtraParams
		if rakshaReq.Params.ExtraParams != nil {
			// Allow override or specification of mask mode
			if v, ok := schemas.SafeExtractString(rakshaReq.Params.ExtraParams["maskMode"]); ok {
				delete(req.ExtraParams, "maskMode")
				maskMode = v
			}

			// Extract dilation (range [0, 1])
			if v, ok := schemas.SafeExtractFloat64Pointer(rakshaReq.Params.ExtraParams["dilation"]); ok {
				// Validate dilation is in valid range
				if *v >= 0 && *v <= 1 {
					delete(req.ExtraParams, "dilation")
					dilation = v
				}
			}

			// Extract maskClasses (for MASK_MODE_SEMANTIC)
			if v, ok := rakshaReq.Params.ExtraParams["maskClasses"]; ok {
				delete(req.ExtraParams, "maskClasses")
				maskClasses = extractIntArray(v)
			}
		}

		// Add mask reference if we have mask data or a mask mode is specified
		if hasMaskData || maskMode != "" {
			maskRef := ImagenReferenceImage{
				ReferenceType: "REFERENCE_TYPE_MASK",
				ReferenceID:   refID,
				MaskImageConfig: &ImagenMaskImageConfig{
					MaskMode:    maskMode,
					Dilation:    dilation,
					MaskClasses: maskClasses,
				},
			}

			// Only include mask data if provided
			if hasMaskData {
				maskRef.ReferenceImage = ImagenReferenceData{
					BytesBase64Encoded: base64.StdEncoding.EncodeToString(rakshaReq.Params.Mask),
				}
			}

			refImages = append(refImages, maskRef)
		}
	}

	req.Instances = append(req.Instances, ImagenInstance{
		ReferenceImages: refImages,
		Prompt:          rakshaReq.Input.Prompt,
	})

	// Set parameters
	if rakshaReq.Params != nil {
		if rakshaReq.Params.N != nil {
			req.Parameters.SampleCount = rakshaReq.Params.N
		}

		// Derive aspect ratio + resolution from size (edit params carry no typed aspect_ratio).
		if rakshaReq.Params.Size != nil && strings.ToLower(*rakshaReq.Params.Size) != "auto" {
			aspectRatio, imageSize := utils.ConvertSizeToAspectRatioAndResolution(*rakshaReq.Params.Size)
			if imageSize != "" {
				req.Parameters.SampleImageSize = &imageSize
			}
			if aspectRatio != "" {
				req.Parameters.AspectRatio = &aspectRatio
			}
		}

		if rakshaReq.Params.OutputFormat != nil {
			mimeType := convertOutputFormatToMimeType(*rakshaReq.Params.OutputFormat)
			if mimeType != "" {
				req.Parameters.OutputOptions = &ImagenOutputOptions{MimeType: &mimeType}
			}
		}
		if rakshaReq.Params.OutputCompression != nil {
			if req.Parameters.OutputOptions == nil {
				req.Parameters.OutputOptions = &ImagenOutputOptions{}
			}
			req.Parameters.OutputOptions.CompressionQuality = rakshaReq.Params.OutputCompression
		}

		// Map Raksha type to Imagen editMode
		if rakshaReq.Params.Type != nil {
			editMode := mapTypeToImagenEditMode(*rakshaReq.Params.Type)
			if editMode != "" {
				req.Parameters.EditMode = &editMode
			}
		}

		if rakshaReq.Params.NegativePrompt != nil {
			req.Parameters.NegativePrompt = rakshaReq.Params.NegativePrompt
		}

		if rakshaReq.Params.Seed != nil {
			req.Parameters.Seed = rakshaReq.Params.Seed
		}

		if rakshaReq.Params.ExtraParams != nil {
			// Only use editMode from ExtraParams if Type was not set
			if rakshaReq.Params.Type == nil {
				if v, ok := schemas.SafeExtractString(rakshaReq.Params.ExtraParams["editMode"]); ok {
					delete(req.ExtraParams, "editMode")
					req.Parameters.EditMode = &v
				}
			}
			if v, ok := schemas.SafeExtractIntPointer(rakshaReq.Params.ExtraParams["guidanceScale"]); ok {
				delete(req.ExtraParams, "guidanceScale")
				req.Parameters.GuidanceScale = v
			}
			if v, ok := schemas.SafeExtractIntPointer(rakshaReq.Params.ExtraParams["baseSteps"]); ok {
				delete(req.ExtraParams, "baseSteps")
				req.Parameters.BaseSteps = v
			}
			if v, ok := schemas.SafeExtractBoolPointer(rakshaReq.Params.ExtraParams["addWatermark"]); ok {
				delete(req.ExtraParams, "addWatermark")
				req.Parameters.AddWatermark = v
			}
			if v, ok := schemas.SafeExtractBoolPointer(rakshaReq.Params.ExtraParams["includeRaiReason"]); ok {
				delete(req.ExtraParams, "includeRaiReason")
				req.Parameters.IncludeRaiReason = v
			}
			if v, ok := schemas.SafeExtractBoolPointer(rakshaReq.Params.ExtraParams["includeSafetyAttributes"]); ok {
				delete(req.ExtraParams, "includeSafetyAttributes")
				req.Parameters.IncludeSafetyAttributes = v
			}
			if v, ok := schemas.SafeExtractString(rakshaReq.Params.ExtraParams["personGeneration"]); ok {
				delete(req.ExtraParams, "personGeneration")
				req.Parameters.PersonGeneration = &v
			}
			if v, ok := schemas.SafeExtractString(rakshaReq.Params.ExtraParams["language"]); ok {
				delete(req.ExtraParams, "language")
				req.Parameters.Language = &v
			}
			if v, ok := schemas.SafeExtractString(rakshaReq.Params.ExtraParams["storageUri"]); ok {
				delete(req.ExtraParams, "storageUri")
				req.Parameters.StorageUri = &v
			}
			if v, ok := SafeExtractSafetySettings(rakshaReq.Params.ExtraParams["safetySettings"]); ok {
				delete(req.ExtraParams, "safetySettings")
				req.Parameters.SafetySettings = v
			}
		}
	}

	return req
}
