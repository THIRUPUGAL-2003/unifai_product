package gemini

import (
	"context"
	"fmt"
	"strings"

	"github.com/unifai/unifai/core/providers/utils"
	"github.com/unifai/unifai/core/schemas"
)

// ToUnifAISpeechRequest converts a GeminiGenerationRequest to a UnifAISpeechRequest
func (request *GeminiGenerationRequest) ToUnifAISpeechRequest(ctx *schemas.UnifAIContext) *schemas.UnifAISpeechRequest {
	provider, model := schemas.ParseModelString(request.Model, "")

	unifaiReq := &schemas.UnifAISpeechRequest{
		Provider: provider,
		Model:    model,
	}

	// Extract text input from contents
	var textInput string
	for _, content := range request.Contents {
		for _, part := range content.Parts {
			if part.Text != "" {
				textInput += part.Text
			}
		}
	}

	unifaiReq.Input = &schemas.SpeechInput{
		Input: textInput,
	}

	// Convert generation config to parameters
	if request.GenerationConfig.SpeechConfig != nil || len(request.GenerationConfig.ResponseModalities) > 0 {
		unifaiReq.Params = &schemas.SpeechParameters{}

		// Extract voice config from speech config
		if request.GenerationConfig.SpeechConfig != nil {
			// Handle single-speaker voice config
			if request.GenerationConfig.SpeechConfig.VoiceConfig != nil {
				unifaiReq.Params.VoiceConfig = &schemas.SpeechVoiceInput{}

				if request.GenerationConfig.SpeechConfig.VoiceConfig.PrebuiltVoiceConfig != nil {
					voiceName := request.GenerationConfig.SpeechConfig.VoiceConfig.PrebuiltVoiceConfig.VoiceName
					unifaiReq.Params.VoiceConfig.Voice = &voiceName
				}
			} else if request.GenerationConfig.SpeechConfig.MultiSpeakerVoiceConfig != nil {
				// Handle multi-speaker voice config
				// Convert to UnifAI's MultiVoiceConfig format
				if len(request.GenerationConfig.SpeechConfig.MultiSpeakerVoiceConfig.SpeakerVoiceConfigs) > 0 {
					unifaiReq.Params.VoiceConfig = &schemas.SpeechVoiceInput{}
					multiVoiceConfig := make([]schemas.VoiceConfig, 0, len(request.GenerationConfig.SpeechConfig.MultiSpeakerVoiceConfig.SpeakerVoiceConfigs))

					for _, speakerConfig := range request.GenerationConfig.SpeechConfig.MultiSpeakerVoiceConfig.SpeakerVoiceConfigs {
						if speakerConfig.VoiceConfig != nil && speakerConfig.VoiceConfig.PrebuiltVoiceConfig != nil {
							multiVoiceConfig = append(multiVoiceConfig, schemas.VoiceConfig{
								Speaker: speakerConfig.Speaker,
								Voice:   speakerConfig.VoiceConfig.PrebuiltVoiceConfig.VoiceName,
							})
						}
					}

					unifaiReq.Params.VoiceConfig.MultiVoiceConfig = multiVoiceConfig
				}
			}
		}

		// Store response modalities in extra params if needed
		if len(request.GenerationConfig.ResponseModalities) > 0 {
			if unifaiReq.Params.ExtraParams == nil {
				unifaiReq.Params.ExtraParams = make(map[string]interface{})
			}
			modalities := make([]string, len(request.GenerationConfig.ResponseModalities))
			for i, mod := range request.GenerationConfig.ResponseModalities {
				modalities[i] = string(mod)
			}
			unifaiReq.Params.ExtraParams["response_modalities"] = modalities
		}
	}

	return unifaiReq
}

// ToGeminiSpeechRequest converts a UnifAISpeechRequest to a GeminiGenerationRequest
func ToGeminiSpeechRequest(unifaiReq *schemas.UnifAISpeechRequest) (*GeminiGenerationRequest, error) {
	if unifaiReq == nil {
		return nil, fmt.Errorf("unifaiReq is nil")
	}
	// Here we confirm if the response_format is wav or empty string
	// If its anything else, we will return an error
	if unifaiReq.Params != nil && unifaiReq.Params.ResponseFormat != "" && unifaiReq.Params.ResponseFormat != "wav" {
		return nil, fmt.Errorf("gemini does not support response_format: %s. Only wav or empty string is supported which defaults to wav", unifaiReq.Params.ResponseFormat)
	}
	// Create the base Gemini generation request
	geminiReq := &GeminiGenerationRequest{
		Model: unifaiReq.Model,
	}
	// Convert parameters to generation config
	geminiReq.GenerationConfig.ResponseModalities = []Modality{ModalityAudio}
	// Convert speech input to Gemini format
	if unifaiReq.Input != nil && unifaiReq.Input.Input != "" {
		geminiReq.Contents = []Content{
			{
				Parts: []*Part{
					{
						Text: unifaiReq.Input.Input,
					},
				},
			},
		}
		// Add speech config to generation config if voice config is provided
		if unifaiReq.Params != nil && unifaiReq.Params.VoiceConfig != nil {
			// Handle both single voice and multi-voice configurations
			if unifaiReq.Params.VoiceConfig.Voice != nil || len(unifaiReq.Params.VoiceConfig.MultiVoiceConfig) > 0 {
				addSpeechConfigToGenerationConfig(&geminiReq.GenerationConfig, unifaiReq.Params.VoiceConfig)
			}
			geminiReq.ExtraParams = unifaiReq.Params.ExtraParams
		}
	}
	return geminiReq, nil
}

// ToUnifAISpeechResponse converts a GenerateContentResponse to a UnifAISpeechResponse
func (response *GenerateContentResponse) ToUnifAISpeechResponse(ctx context.Context) (*schemas.UnifAISpeechResponse, error) {
	unifaiResp := &schemas.UnifAISpeechResponse{}

	// Process candidates to extract audio content
	if len(response.Candidates) > 0 {
		candidate := response.Candidates[0]
		if candidate.Content != nil && len(candidate.Content.Parts) > 0 {
			var audioData []byte
			// Extract audio data from all parts
			for _, part := range candidate.Content.Parts {
				if part.InlineData != nil && len(part.InlineData.Data) > 0 {
					// Check if this is audio data
					if strings.HasPrefix(part.InlineData.MIMEType, "audio/") {
						decodedData, err := decodeBase64StringToBytes(part.InlineData.Data)
						if err != nil {
							return nil, fmt.Errorf("failed to decode base64 audio data: %v", err)
						}
						audioData = append(audioData, decodedData...)
					}
				}
			}
			if len(audioData) > 0 {
				responseFormat := ctx.Value(UnifAIContextKeyResponseFormat).(string)
				// Gemini returns PCM audio (s16le, 24000 Hz, mono)
				// Convert to WAV for standard playable output format
				if responseFormat == "wav" {
					wavData, err := utils.ConvertPCMToWAV(audioData, utils.DefaultGeminiPCMConfig())
					if err != nil {
						return nil, fmt.Errorf("failed to convert PCM to WAV: %v", err)
					}
					unifaiResp.Audio = wavData
				} else {
					unifaiResp.Audio = audioData
				}
			}

			// Set usage information
			if response.UsageMetadata != nil {
				unifaiResp.Usage = convertGeminiUsageMetadataToSpeechUsage(response.UsageMetadata)
			}
		}
	}
	return unifaiResp, nil
}

// ToGeminiSpeechResponse converts a UnifAISpeechResponse to Gemini's GenerateContentResponse
func ToGeminiSpeechResponse(unifaiResp *schemas.UnifAISpeechResponse) *GenerateContentResponse {
	if unifaiResp == nil {
		return nil
	}

	genaiResp := &GenerateContentResponse{}

	candidate := &Candidate{
		Content: &Content{
			Parts: []*Part{
				{
					InlineData: &Blob{
						Data:     encodeBytesToBase64String(unifaiResp.Audio),
						MIMEType: utils.DetectAudioMimeType(unifaiResp.Audio),
					},
				},
			},
			Role: string(RoleModel),
		},
	}

	// Set usage metadata if present
	if unifaiResp.Usage != nil {
		genaiResp.UsageMetadata = convertUnifAISpeechUsageToGeminiUsageMetadata(unifaiResp.Usage)
	}

	genaiResp.Candidates = []*Candidate{candidate}
	return genaiResp
}
