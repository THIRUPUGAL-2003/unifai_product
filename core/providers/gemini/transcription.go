package gemini

import (
	"fmt"
	"strings"

	"github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
)

// ToGatewayTranscriptionRequest converts a GeminiGenerationRequest to a GatewayTranscriptionRequest
func (request *GeminiGenerationRequest) ToGatewayTranscriptionRequest(ctx *schemas.GatewayContext) (*schemas.GatewayTranscriptionRequest, error) {
	provider, model := schemas.ParseModelString(request.Model, "")

	gatewayReq := &schemas.GatewayTranscriptionRequest{
		Provider: provider,
		Model:    model,
	}

	// Extract audio data and prompt from contents
	var promptText string
	var audioData []byte
	var audioMimeType string

	for _, content := range request.Contents {
		for _, part := range content.Parts {
			// Extract text prompt
			if part.Text != "" {
				if promptText != "" {
					promptText += " "
				}
				promptText += part.Text
			}

			// Extract audio data from inline data
			if part.InlineData != nil && strings.HasPrefix(strings.ToLower(part.InlineData.MIMEType), "audio/") {
				decodedData, err := decodeBase64StringToBytes(part.InlineData.Data)
				if err != nil {
					return nil, fmt.Errorf("failed to decode base64 audio data: %v", err)
				}
				audioData = append(audioData, decodedData...)
				if audioMimeType == "" {
					audioMimeType = part.InlineData.MIMEType
				}
			}

			// Extract audio data from file data (would need to be fetched separately in real scenario)
			// For now, we just note the file URI in extra params
			if part.FileData != nil && strings.HasPrefix(strings.ToLower(part.FileData.MIMEType), "audio/") {
				if gatewayReq.Params == nil {
					gatewayReq.Params = &schemas.TranscriptionParameters{}
				}
				if gatewayReq.Params.ExtraParams == nil {
					gatewayReq.Params.ExtraParams = make(map[string]interface{})
				}
				gatewayReq.Params.ExtraParams["file_uri"] = part.FileData.FileURI
				if audioMimeType == "" {
					audioMimeType = part.FileData.MIMEType
				}
			}
		}
	}

	// Set the audio input
	gatewayReq.Input = &schemas.TranscriptionInput{
		File: audioData,
	}

	// Set parameters
	if gatewayReq.Params == nil {
		gatewayReq.Params = &schemas.TranscriptionParameters{}
	}

	// Set prompt if provided
	if promptText != "" {
		gatewayReq.Params.Prompt = &promptText
	}

	// Handle safety settings from request
	if len(request.SafetySettings) > 0 {
		if gatewayReq.Params.ExtraParams == nil {
			gatewayReq.Params.ExtraParams = make(map[string]interface{})
		}
		gatewayReq.Params.ExtraParams["safety_settings"] = request.SafetySettings
	}

	// Handle cached content
	if request.CachedContent != "" {
		if gatewayReq.Params.ExtraParams == nil {
			gatewayReq.Params.ExtraParams = make(map[string]interface{})
		}
		gatewayReq.Params.ExtraParams["cached_content"] = request.CachedContent
	}

	// Handle labels
	if len(request.Labels) > 0 {
		if gatewayReq.Params.ExtraParams == nil {
			gatewayReq.Params.ExtraParams = make(map[string]interface{})
		}
		gatewayReq.Params.ExtraParams["labels"] = request.Labels
	}

	return gatewayReq, nil
}

func ToGeminiTranscriptionRequest(gatewayReq *schemas.GatewayTranscriptionRequest) *GeminiGenerationRequest {
	if gatewayReq == nil {
		return nil
	}

	// Create the base Gemini generation request
	geminiReq := &GeminiGenerationRequest{
		Model: gatewayReq.Model,
	}

	// Convert parameters to generation config
	if gatewayReq.Params != nil {
		geminiReq.ExtraParams = gatewayReq.Params.ExtraParams
		// Handle extra parameters
		if gatewayReq.Params.ExtraParams != nil {
			// Safety settings
			if safetySettings, ok := schemas.SafeExtractFromMap(gatewayReq.Params.ExtraParams, "safety_settings"); ok {
				delete(geminiReq.ExtraParams, "safety_settings")
				if settings, ok := SafeExtractSafetySettings(safetySettings); ok {
					geminiReq.SafetySettings = settings
				}
			}

			// Cached content
			if cachedContent, ok := schemas.SafeExtractString(gatewayReq.Params.ExtraParams["cached_content"]); ok {
				delete(geminiReq.ExtraParams, "cached_content")
				geminiReq.CachedContent = cachedContent
			}

			// Labels
			if labels, ok := schemas.SafeExtractFromMap(gatewayReq.Params.ExtraParams, "labels"); ok {
				if labelMap, ok := schemas.SafeExtractStringMap(labels); ok {
					delete(geminiReq.ExtraParams, "labels")
					geminiReq.Labels = labelMap
				}
			}
		}
	}

	// Determine the prompt text
	var prompt string
	if gatewayReq.Params != nil && gatewayReq.Params.Prompt != nil {
		prompt = *gatewayReq.Params.Prompt
	} else {
		prompt = "Generate a transcript of the speech."
	}

	// Create parts for the transcription request
	parts := []*Part{
		{
			Text: prompt,
		},
	}

	// Add audio file if present
	if len(gatewayReq.Input.File) > 0 {
		parts = append(parts, &Part{
			InlineData: &Blob{
				MIMEType: utils.DetectAudioMimeType(gatewayReq.Input.File),
				Data:     encodeBytesToBase64String(gatewayReq.Input.File),
			},
		})
	}

	geminiReq.Contents = []Content{
		{
			Parts: parts,
		},
	}

	return geminiReq
}

// ToGatewayTranscriptionResponse converts a GenerateContentResponse to a GatewayTranscriptionResponse
func (response *GenerateContentResponse) ToGatewayTranscriptionResponse() *schemas.GatewayTranscriptionResponse {
	gatewayResp := &schemas.GatewayTranscriptionResponse{}

	// Process candidates to extract text content
	if len(response.Candidates) > 0 {
		candidate := response.Candidates[0]
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			var textContent string

			// Extract text content from all parts
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					textContent += part.Text
				}
			}

			if textContent != "" {
				gatewayResp.Text = textContent
				gatewayResp.Task = schemas.Ptr("transcribe")

				// Set usage information with modality details
				gatewayResp.Usage = convertGeminiUsageMetadataToTranscriptionUsage(response.UsageMetadata)
			}
		}
	}

	return gatewayResp
}

// ToGeminiTranscriptionResponse converts a GatewayTranscriptionResponse to Gemini's GenerateContentResponse
func ToGeminiTranscriptionResponse(gatewayResp *schemas.GatewayTranscriptionResponse) *GenerateContentResponse {
	if gatewayResp == nil {
		return nil
	}

	genaiResp := &GenerateContentResponse{}

	candidate := &Candidate{
		Content: &Content{
			Parts: []*Part{
				{
					Text: gatewayResp.Text,
				},
			},
			Role: string(RoleModel),
		},
	}

	// Set usage metadata from transcription usage with modality details
	genaiResp.UsageMetadata = convertGatewayTranscriptionUsageToGeminiUsageMetadata(gatewayResp.Usage)

	genaiResp.Candidates = []*Candidate{candidate}
	return genaiResp
}
