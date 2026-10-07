package gemini

import (
	"context"
	"fmt"
	"strings"

	"github.com/gateway/gateway/core/providers/utils"
	"github.com/gateway/gateway/core/schemas"
)

// ToGatewaySpeechRequest converts a GeminiGenerationRequest to a GatewaySpeechRequest
func (request *GeminiGenerationRequest) ToGatewaySpeechRequest(ctx *schemas.GatewayContext) *schemas.GatewaySpeechRequest {
	provider, model := schemas.ParseModelString(request.Model, "")

	gatewayReq := &schemas.GatewaySpeechRequest{
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

	gatewayReq.Input = &schemas.SpeechInput{
		Input: textInput,
	}

	// Convert generation config to parameters
	if request.GenerationConfig.SpeechConfig != nil || len(request.GenerationConfig.ResponseModalities) > 0 {
		gatewayReq.Params = &schemas.SpeechParameters{}

		// Extract voice config from speech config
		if request.GenerationConfig.SpeechConfig != nil {
			// Handle single-speaker voice config
			if request.GenerationConfig.SpeechConfig.VoiceConfig != nil {
				gatewayReq.Params.VoiceConfig = &schemas.SpeechVoiceInput{}

				if request.GenerationConfig.SpeechConfig.VoiceConfig.PrebuiltVoiceConfig != nil {
					voiceName := request.GenerationConfig.SpeechConfig.VoiceConfig.PrebuiltVoiceConfig.VoiceName
					gatewayReq.Params.VoiceConfig.Voice = &voiceName
				}
			} else if request.GenerationConfig.SpeechConfig.MultiSpeakerVoiceConfig != nil {
				// Handle multi-speaker voice config
				// Convert to Gateway's MultiVoiceConfig format
				if len(request.GenerationConfig.SpeechConfig.MultiSpeakerVoiceConfig.SpeakerVoiceConfigs) > 0 {
					gatewayReq.Params.VoiceConfig = &schemas.SpeechVoiceInput{}
					multiVoiceConfig := make([]schemas.VoiceConfig, 0, len(request.GenerationConfig.SpeechConfig.MultiSpeakerVoiceConfig.SpeakerVoiceConfigs))

					for _, speakerConfig := range request.GenerationConfig.SpeechConfig.MultiSpeakerVoiceConfig.SpeakerVoiceConfigs {
						if speakerConfig.VoiceConfig != nil && speakerConfig.VoiceConfig.PrebuiltVoiceConfig != nil {
							multiVoiceConfig = append(multiVoiceConfig, schemas.VoiceConfig{
								Speaker: speakerConfig.Speaker,
								Voice:   speakerConfig.VoiceConfig.PrebuiltVoiceConfig.VoiceName,
							})
						}
					}

					gatewayReq.Params.VoiceConfig.MultiVoiceConfig = multiVoiceConfig
				}
			}
		}

		// Store response modalities in extra params if needed
		if len(request.GenerationConfig.ResponseModalities) > 0 {
			if gatewayReq.Params.ExtraParams == nil {
				gatewayReq.Params.ExtraParams = make(map[string]interface{})
			}
			modalities := make([]string, len(request.GenerationConfig.ResponseModalities))
			for i, mod := range request.GenerationConfig.ResponseModalities {
				modalities[i] = string(mod)
			}
			gatewayReq.Params.ExtraParams["response_modalities"] = modalities
		}
	}

	return gatewayReq
}

// ToGeminiSpeechRequest converts a GatewaySpeechRequest to a GeminiGenerationRequest
func ToGeminiSpeechRequest(gatewayReq *schemas.GatewaySpeechRequest) (*GeminiGenerationRequest, error) {
	if gatewayReq == nil {
		return nil, fmt.Errorf("gatewayReq is nil")
	}
	// Here we confirm if the response_format is wav or empty string
	// If its anything else, we will return an error
	if gatewayReq.Params != nil && gatewayReq.Params.ResponseFormat != "" && gatewayReq.Params.ResponseFormat != "wav" {
		return nil, fmt.Errorf("gemini does not support response_format: %s. Only wav or empty string is supported which defaults to wav", gatewayReq.Params.ResponseFormat)
	}
	// Create the base Gemini generation request
	geminiReq := &GeminiGenerationRequest{
		Model: gatewayReq.Model,
	}
	// Convert parameters to generation config
	geminiReq.GenerationConfig.ResponseModalities = []Modality{ModalityAudio}
	// Convert speech input to Gemini format
	if gatewayReq.Input != nil && gatewayReq.Input.Input != "" {
		geminiReq.Contents = []Content{
			{
				Parts: []*Part{
					{
						Text: gatewayReq.Input.Input,
					},
				},
			},
		}
		// Add speech config to generation config if voice config is provided
		if gatewayReq.Params != nil && gatewayReq.Params.VoiceConfig != nil {
			// Handle both single voice and multi-voice configurations
			if gatewayReq.Params.VoiceConfig.Voice != nil || len(gatewayReq.Params.VoiceConfig.MultiVoiceConfig) > 0 {
				addSpeechConfigToGenerationConfig(&geminiReq.GenerationConfig, gatewayReq.Params.VoiceConfig)
			}
			geminiReq.ExtraParams = gatewayReq.Params.ExtraParams
		}
	}
	return geminiReq, nil
}

// ToGatewaySpeechResponse converts a GenerateContentResponse to a GatewaySpeechResponse
func (response *GenerateContentResponse) ToGatewaySpeechResponse(ctx context.Context) (*schemas.GatewaySpeechResponse, error) {
	gatewayResp := &schemas.GatewaySpeechResponse{}

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
				responseFormat := ctx.Value(GatewayContextKeyResponseFormat).(string)
				// Gemini returns PCM audio (s16le, 24000 Hz, mono)
				// Convert to WAV for standard playable output format
				if responseFormat == "wav" {
					wavData, err := utils.ConvertPCMToWAV(audioData, utils.DefaultGeminiPCMConfig())
					if err != nil {
						return nil, fmt.Errorf("failed to convert PCM to WAV: %v", err)
					}
					gatewayResp.Audio = wavData
				} else {
					gatewayResp.Audio = audioData
				}
			}

			// Set usage information
			if response.UsageMetadata != nil {
				gatewayResp.Usage = convertGeminiUsageMetadataToSpeechUsage(response.UsageMetadata)
			}
		}
	}
	return gatewayResp, nil
}

// ToGeminiSpeechResponse converts a GatewaySpeechResponse to Gemini's GenerateContentResponse
func ToGeminiSpeechResponse(gatewayResp *schemas.GatewaySpeechResponse) *GenerateContentResponse {
	if gatewayResp == nil {
		return nil
	}

	genaiResp := &GenerateContentResponse{}

	candidate := &Candidate{
		Content: &Content{
			Parts: []*Part{
				{
					InlineData: &Blob{
						Data:     encodeBytesToBase64String(gatewayResp.Audio),
						MIMEType: utils.DetectAudioMimeType(gatewayResp.Audio),
					},
				},
			},
			Role: string(RoleModel),
		},
	}

	// Set usage metadata if present
	if gatewayResp.Usage != nil {
		genaiResp.UsageMetadata = convertGatewaySpeechUsageToGeminiUsageMetadata(gatewayResp.Usage)
	}

	genaiResp.Candidates = []*Candidate{candidate}
	return genaiResp
}
