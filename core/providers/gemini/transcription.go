package gemini

import (
	"fmt"
	"strings"

	"github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
)

// ToRakshaTranscriptionRequest converts a GeminiGenerationRequest to a RakshaTranscriptionRequest
func (request *GeminiGenerationRequest) ToRakshaTranscriptionRequest(ctx *schemas.RakshaContext) (*schemas.RakshaTranscriptionRequest, error) {
	provider, model := schemas.ParseModelString(request.Model, "")

	rakshaReq := &schemas.RakshaTranscriptionRequest{
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
				if rakshaReq.Params == nil {
					rakshaReq.Params = &schemas.TranscriptionParameters{}
				}
				if rakshaReq.Params.ExtraParams == nil {
					rakshaReq.Params.ExtraParams = make(map[string]interface{})
				}
				rakshaReq.Params.ExtraParams["file_uri"] = part.FileData.FileURI
				if audioMimeType == "" {
					audioMimeType = part.FileData.MIMEType
				}
			}
		}
	}

	// Set the audio input
	rakshaReq.Input = &schemas.TranscriptionInput{
		File: audioData,
	}

	// Set parameters
	if rakshaReq.Params == nil {
		rakshaReq.Params = &schemas.TranscriptionParameters{}
	}

	// Set prompt if provided
	if promptText != "" {
		rakshaReq.Params.Prompt = &promptText
	}

	// Handle safety settings from request
	if len(request.SafetySettings) > 0 {
		if rakshaReq.Params.ExtraParams == nil {
			rakshaReq.Params.ExtraParams = make(map[string]interface{})
		}
		rakshaReq.Params.ExtraParams["safety_settings"] = request.SafetySettings
	}

	// Handle cached content
	if request.CachedContent != "" {
		if rakshaReq.Params.ExtraParams == nil {
			rakshaReq.Params.ExtraParams = make(map[string]interface{})
		}
		rakshaReq.Params.ExtraParams["cached_content"] = request.CachedContent
	}

	// Handle labels
	if len(request.Labels) > 0 {
		if rakshaReq.Params.ExtraParams == nil {
			rakshaReq.Params.ExtraParams = make(map[string]interface{})
		}
		rakshaReq.Params.ExtraParams["labels"] = request.Labels
	}

	return rakshaReq, nil
}

func ToGeminiTranscriptionRequest(rakshaReq *schemas.RakshaTranscriptionRequest) *GeminiGenerationRequest {
	if rakshaReq == nil {
		return nil
	}

	// Create the base Gemini generation request
	geminiReq := &GeminiGenerationRequest{
		Model: rakshaReq.Model,
	}

	// Convert parameters to generation config
	if rakshaReq.Params != nil {
		geminiReq.ExtraParams = rakshaReq.Params.ExtraParams
		// Handle extra parameters
		if rakshaReq.Params.ExtraParams != nil {
			// Safety settings
			if safetySettings, ok := schemas.SafeExtractFromMap(rakshaReq.Params.ExtraParams, "safety_settings"); ok {
				delete(geminiReq.ExtraParams, "safety_settings")
				if settings, ok := SafeExtractSafetySettings(safetySettings); ok {
					geminiReq.SafetySettings = settings
				}
			}

			// Cached content
			if cachedContent, ok := schemas.SafeExtractString(rakshaReq.Params.ExtraParams["cached_content"]); ok {
				delete(geminiReq.ExtraParams, "cached_content")
				geminiReq.CachedContent = cachedContent
			}

			// Labels
			if labels, ok := schemas.SafeExtractFromMap(rakshaReq.Params.ExtraParams, "labels"); ok {
				if labelMap, ok := schemas.SafeExtractStringMap(labels); ok {
					delete(geminiReq.ExtraParams, "labels")
					geminiReq.Labels = labelMap
				}
			}
		}
	}

	// Determine the prompt text
	var prompt string
	if rakshaReq.Params != nil && rakshaReq.Params.Prompt != nil {
		prompt = *rakshaReq.Params.Prompt
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
	if len(rakshaReq.Input.File) > 0 {
		parts = append(parts, &Part{
			InlineData: &Blob{
				MIMEType: utils.DetectAudioMimeType(rakshaReq.Input.File),
				Data:     encodeBytesToBase64String(rakshaReq.Input.File),
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

// ToRakshaTranscriptionResponse converts a GenerateContentResponse to a RakshaTranscriptionResponse
func (response *GenerateContentResponse) ToRakshaTranscriptionResponse() *schemas.RakshaTranscriptionResponse {
	rakshaResp := &schemas.RakshaTranscriptionResponse{}

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
				rakshaResp.Text = textContent
				rakshaResp.Task = schemas.Ptr("transcribe")

				// Set usage information with modality details
				rakshaResp.Usage = convertGeminiUsageMetadataToTranscriptionUsage(response.UsageMetadata)
			}
		}
	}

	return rakshaResp
}

// ToGeminiTranscriptionResponse converts a RakshaTranscriptionResponse to Gemini's GenerateContentResponse
func ToGeminiTranscriptionResponse(rakshaResp *schemas.RakshaTranscriptionResponse) *GenerateContentResponse {
	if rakshaResp == nil {
		return nil
	}

	genaiResp := &GenerateContentResponse{}

	candidate := &Candidate{
		Content: &Content{
			Parts: []*Part{
				{
					Text: rakshaResp.Text,
				},
			},
			Role: string(RoleModel),
		},
	}

	// Set usage metadata from transcription usage with modality details
	genaiResp.UsageMetadata = convertRakshaTranscriptionUsageToGeminiUsageMetadata(rakshaResp.Usage)

	genaiResp.Candidates = []*Candidate{candidate}
	return genaiResp
}
