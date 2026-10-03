package gemini

import (
	"context"
	"fmt"
	"strings"

	"github.com/raksha/raksha/core/providers/utils"
	"github.com/raksha/raksha/core/schemas"
)

// ToRakshaSpeechRequest converts a GeminiGenerationRequest to a RakshaSpeechRequest
func (request *GeminiGenerationRequest) ToRakshaSpeechRequest(ctx *schemas.RakshaContext) *schemas.RakshaSpeechRequest {
	provider, model := schemas.ParseModelString(request.Model, "")

	rakshaReq := &schemas.RakshaSpeechRequest{
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

	rakshaReq.Input = &schemas.SpeechInput{
		Input: textInput,
	}

	// Convert generation config to parameters
	if request.GenerationConfig.SpeechConfig != nil || len(request.GenerationConfig.ResponseModalities) > 0 {
		rakshaReq.Params = &schemas.SpeechParameters{}

		// Extract voice config from speech config
		if request.GenerationConfig.SpeechConfig != nil {
			// Handle single-speaker voice config
			if request.GenerationConfig.SpeechConfig.VoiceConfig != nil {
				rakshaReq.Params.VoiceConfig = &schemas.SpeechVoiceInput{}

				if request.GenerationConfig.SpeechConfig.VoiceConfig.PrebuiltVoiceConfig != nil {
					voiceName := request.GenerationConfig.SpeechConfig.VoiceConfig.PrebuiltVoiceConfig.VoiceName
					rakshaReq.Params.VoiceConfig.Voice = &voiceName
				}
			} else if request.GenerationConfig.SpeechConfig.MultiSpeakerVoiceConfig != nil {
				// Handle multi-speaker voice config
				// Convert to Raksha's MultiVoiceConfig format
				if len(request.GenerationConfig.SpeechConfig.MultiSpeakerVoiceConfig.SpeakerVoiceConfigs) > 0 {
					rakshaReq.Params.VoiceConfig = &schemas.SpeechVoiceInput{}
					multiVoiceConfig := make([]schemas.VoiceConfig, 0, len(request.GenerationConfig.SpeechConfig.MultiSpeakerVoiceConfig.SpeakerVoiceConfigs))

					for _, speakerConfig := range request.GenerationConfig.SpeechConfig.MultiSpeakerVoiceConfig.SpeakerVoiceConfigs {
						if speakerConfig.VoiceConfig != nil && speakerConfig.VoiceConfig.PrebuiltVoiceConfig != nil {
							multiVoiceConfig = append(multiVoiceConfig, schemas.VoiceConfig{
								Speaker: speakerConfig.Speaker,
								Voice:   speakerConfig.VoiceConfig.PrebuiltVoiceConfig.VoiceName,
							})
						}
					}

					rakshaReq.Params.VoiceConfig.MultiVoiceConfig = multiVoiceConfig
				}
			}
		}

		// Store response modalities in extra params if needed
		if len(request.GenerationConfig.ResponseModalities) > 0 {
			if rakshaReq.Params.ExtraParams == nil {
				rakshaReq.Params.ExtraParams = make(map[string]interface{})
			}
			modalities := make([]string, len(request.GenerationConfig.ResponseModalities))
			for i, mod := range request.GenerationConfig.ResponseModalities {
				modalities[i] = string(mod)
			}
			rakshaReq.Params.ExtraParams["response_modalities"] = modalities
		}
	}

	return rakshaReq
}

// ToGeminiSpeechRequest converts a RakshaSpeechRequest to a GeminiGenerationRequest
func ToGeminiSpeechRequest(rakshaReq *schemas.RakshaSpeechRequest) (*GeminiGenerationRequest, error) {
	if rakshaReq == nil {
		return nil, fmt.Errorf("rakshaReq is nil")
	}
	// Here we confirm if the response_format is wav or empty string
	// If its anything else, we will return an error
	if rakshaReq.Params != nil && rakshaReq.Params.ResponseFormat != "" && rakshaReq.Params.ResponseFormat != "wav" {
		return nil, fmt.Errorf("gemini does not support response_format: %s. Only wav or empty string is supported which defaults to wav", rakshaReq.Params.ResponseFormat)
	}
	// Create the base Gemini generation request
	geminiReq := &GeminiGenerationRequest{
		Model: rakshaReq.Model,
	}
	// Convert parameters to generation config
	geminiReq.GenerationConfig.ResponseModalities = []Modality{ModalityAudio}
	// Convert speech input to Gemini format
	if rakshaReq.Input != nil && rakshaReq.Input.Input != "" {
		geminiReq.Contents = []Content{
			{
				Parts: []*Part{
					{
						Text: rakshaReq.Input.Input,
					},
				},
			},
		}
		// Add speech config to generation config if voice config is provided
		if rakshaReq.Params != nil && rakshaReq.Params.VoiceConfig != nil {
			// Handle both single voice and multi-voice configurations
			if rakshaReq.Params.VoiceConfig.Voice != nil || len(rakshaReq.Params.VoiceConfig.MultiVoiceConfig) > 0 {
				addSpeechConfigToGenerationConfig(&geminiReq.GenerationConfig, rakshaReq.Params.VoiceConfig)
			}
			geminiReq.ExtraParams = rakshaReq.Params.ExtraParams
		}
	}
	return geminiReq, nil
}

// ToRakshaSpeechResponse converts a GenerateContentResponse to a RakshaSpeechResponse
func (response *GenerateContentResponse) ToRakshaSpeechResponse(ctx context.Context) (*schemas.RakshaSpeechResponse, error) {
	rakshaResp := &schemas.RakshaSpeechResponse{}

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
				responseFormat := ctx.Value(RakshaContextKeyResponseFormat).(string)
				// Gemini returns PCM audio (s16le, 24000 Hz, mono)
				// Convert to WAV for standard playable output format
				if responseFormat == "wav" {
					wavData, err := utils.ConvertPCMToWAV(audioData, utils.DefaultGeminiPCMConfig())
					if err != nil {
						return nil, fmt.Errorf("failed to convert PCM to WAV: %v", err)
					}
					rakshaResp.Audio = wavData
				} else {
					rakshaResp.Audio = audioData
				}
			}

			// Set usage information
			if response.UsageMetadata != nil {
				rakshaResp.Usage = convertGeminiUsageMetadataToSpeechUsage(response.UsageMetadata)
			}
		}
	}
	return rakshaResp, nil
}

// ToGeminiSpeechResponse converts a RakshaSpeechResponse to Gemini's GenerateContentResponse
func ToGeminiSpeechResponse(rakshaResp *schemas.RakshaSpeechResponse) *GenerateContentResponse {
	if rakshaResp == nil {
		return nil
	}

	genaiResp := &GenerateContentResponse{}

	candidate := &Candidate{
		Content: &Content{
			Parts: []*Part{
				{
					InlineData: &Blob{
						Data:     encodeBytesToBase64String(rakshaResp.Audio),
						MIMEType: utils.DetectAudioMimeType(rakshaResp.Audio),
					},
				},
			},
			Role: string(RoleModel),
		},
	}

	// Set usage metadata if present
	if rakshaResp.Usage != nil {
		genaiResp.UsageMetadata = convertRakshaSpeechUsageToGeminiUsageMetadata(rakshaResp.Usage)
	}

	genaiResp.Candidates = []*Candidate{candidate}
	return genaiResp
}
