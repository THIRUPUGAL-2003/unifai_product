package gemini

import (
	"fmt"
	"strings"

	"github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
)

// ToUnifAITranscriptionRequest converts a GeminiGenerationRequest to a UnifAITranscriptionRequest
func (request *GeminiGenerationRequest) ToUnifAITranscriptionRequest(ctx *schemas.UnifAIContext) (*schemas.UnifAITranscriptionRequest, error) {
	provider, model := schemas.ParseModelString(request.Model, "")

	unifaiReq := &schemas.UnifAITranscriptionRequest{
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
				if unifaiReq.Params == nil {
					unifaiReq.Params = &schemas.TranscriptionParameters{}
				}
				if unifaiReq.Params.ExtraParams == nil {
					unifaiReq.Params.ExtraParams = make(map[string]interface{})
				}
				unifaiReq.Params.ExtraParams["file_uri"] = part.FileData.FileURI
				if audioMimeType == "" {
					audioMimeType = part.FileData.MIMEType
				}
			}
		}
	}

	// Set the audio input
	unifaiReq.Input = &schemas.TranscriptionInput{
		File: audioData,
	}

	// Set parameters
	if unifaiReq.Params == nil {
		unifaiReq.Params = &schemas.TranscriptionParameters{}
	}

	// Set prompt if provided
	if promptText != "" {
		unifaiReq.Params.Prompt = &promptText
	}

	// Handle safety settings from request
	if len(request.SafetySettings) > 0 {
		if unifaiReq.Params.ExtraParams == nil {
			unifaiReq.Params.ExtraParams = make(map[string]interface{})
		}
		unifaiReq.Params.ExtraParams["safety_settings"] = request.SafetySettings
	}

	// Handle cached content
	if request.CachedContent != "" {
		if unifaiReq.Params.ExtraParams == nil {
			unifaiReq.Params.ExtraParams = make(map[string]interface{})
		}
		unifaiReq.Params.ExtraParams["cached_content"] = request.CachedContent
	}

	// Handle labels
	if len(request.Labels) > 0 {
		if unifaiReq.Params.ExtraParams == nil {
			unifaiReq.Params.ExtraParams = make(map[string]interface{})
		}
		unifaiReq.Params.ExtraParams["labels"] = request.Labels
	}

	return unifaiReq, nil
}

func ToGeminiTranscriptionRequest(unifaiReq *schemas.UnifAITranscriptionRequest) *GeminiGenerationRequest {
	if unifaiReq == nil {
		return nil
	}

	// Create the base Gemini generation request
	geminiReq := &GeminiGenerationRequest{
		Model: unifaiReq.Model,
	}

	// Convert parameters to generation config
	if unifaiReq.Params != nil {
		geminiReq.ExtraParams = unifaiReq.Params.ExtraParams
		// Handle extra parameters
		if unifaiReq.Params.ExtraParams != nil {
			// Safety settings
			if safetySettings, ok := schemas.SafeExtractFromMap(unifaiReq.Params.ExtraParams, "safety_settings"); ok {
				delete(geminiReq.ExtraParams, "safety_settings")
				if settings, ok := SafeExtractSafetySettings(safetySettings); ok {
					geminiReq.SafetySettings = settings
				}
			}

			// Cached content
			if cachedContent, ok := schemas.SafeExtractString(unifaiReq.Params.ExtraParams["cached_content"]); ok {
				delete(geminiReq.ExtraParams, "cached_content")
				geminiReq.CachedContent = cachedContent
			}

			// Labels
			if labels, ok := schemas.SafeExtractFromMap(unifaiReq.Params.ExtraParams, "labels"); ok {
				if labelMap, ok := schemas.SafeExtractStringMap(labels); ok {
					delete(geminiReq.ExtraParams, "labels")
					geminiReq.Labels = labelMap
				}
			}
		}
	}

	// Determine the prompt text
	var prompt string
	if unifaiReq.Params != nil && unifaiReq.Params.Prompt != nil {
		prompt = *unifaiReq.Params.Prompt
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
	if len(unifaiReq.Input.File) > 0 {
		parts = append(parts, &Part{
			InlineData: &Blob{
				MIMEType: utils.DetectAudioMimeType(unifaiReq.Input.File),
				Data:     encodeBytesToBase64String(unifaiReq.Input.File),
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

// ToUnifAITranscriptionResponse converts a GenerateContentResponse to a UnifAITranscriptionResponse
func (response *GenerateContentResponse) ToUnifAITranscriptionResponse() *schemas.UnifAITranscriptionResponse {
	unifaiResp := &schemas.UnifAITranscriptionResponse{}

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
				unifaiResp.Text = textContent
				unifaiResp.Task = schemas.Ptr("transcribe")

				// Set usage information with modality details
				unifaiResp.Usage = convertGeminiUsageMetadataToTranscriptionUsage(response.UsageMetadata)
			}
		}
	}

	return unifaiResp
}

// ToGeminiTranscriptionResponse converts a UnifAITranscriptionResponse to Gemini's GenerateContentResponse
func ToGeminiTranscriptionResponse(unifaiResp *schemas.UnifAITranscriptionResponse) *GenerateContentResponse {
	if unifaiResp == nil {
		return nil
	}

	genaiResp := &GenerateContentResponse{}

	candidate := &Candidate{
		Content: &Content{
			Parts: []*Part{
				{
					Text: unifaiResp.Text,
				},
			},
			Role: string(RoleModel),
		},
	}

	// Set usage metadata from transcription usage with modality details
	genaiResp.UsageMetadata = convertUnifAITranscriptionUsageToGeminiUsageMetadata(unifaiResp.Usage)

	genaiResp.Candidates = []*Candidate{candidate}
	return genaiResp
}
