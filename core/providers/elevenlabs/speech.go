package elevenlabs

import (
	"github.com/unifai/unifai/core/schemas"
)

func ToElevenlabsSpeechRequest(unifaiReq *schemas.UnifAISpeechRequest) *ElevenlabsSpeechRequest {
	if unifaiReq == nil || unifaiReq.Input == nil {
		return nil
	}

	elevenlabsReq := &ElevenlabsSpeechRequest{
		ModelID: unifaiReq.Model,
		Text:    unifaiReq.Input.Input,
	}

	if unifaiReq.Params != nil {
		elevenlabsReq.ExtraParams = unifaiReq.Params.ExtraParams
		voiceSettings := ElevenlabsVoiceSettings{}
		hasVoiceSettings := false

		if unifaiReq.Params.Speed != nil {
			voiceSettings.Speed = *unifaiReq.Params.Speed
			hasVoiceSettings = true
		}

		if unifaiReq.Params.ExtraParams != nil {
			if stability, ok := schemas.SafeExtractFloat64Pointer(unifaiReq.Params.ExtraParams["stability"]); ok {
				delete(elevenlabsReq.ExtraParams, "stability")
				voiceSettings.Stability = *stability
				hasVoiceSettings = true
			}
			if useSpeakerBoost, ok := schemas.SafeExtractBoolPointer(unifaiReq.Params.ExtraParams["use_speaker_boost"]); ok {
				delete(elevenlabsReq.ExtraParams, "use_speaker_boost")
				voiceSettings.UseSpeakerBoost = *useSpeakerBoost
				hasVoiceSettings = true
			}
			if similarityBoost, ok := schemas.SafeExtractFloat64Pointer(unifaiReq.Params.ExtraParams["similarity_boost"]); ok {
				delete(elevenlabsReq.ExtraParams, "similarity_boost")
				voiceSettings.SimilarityBoost = *similarityBoost
				hasVoiceSettings = true
			}
			if style, ok := schemas.SafeExtractFloat64Pointer(unifaiReq.Params.ExtraParams["style"]); ok {
				delete(elevenlabsReq.ExtraParams, "style")
				voiceSettings.Style = *style
				hasVoiceSettings = true
			}
			if seed, ok := schemas.SafeExtractIntPointer(unifaiReq.Params.ExtraParams["seed"]); ok {
				delete(elevenlabsReq.ExtraParams, "seed")
				elevenlabsReq.Seed = seed
			}
			if previousText, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["previous_text"]); ok {
				delete(elevenlabsReq.ExtraParams, "previous_text")
				elevenlabsReq.PreviousText = previousText
			}
			if nextText, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["next_text"]); ok {
				delete(elevenlabsReq.ExtraParams, "next_text")
				elevenlabsReq.NextText = nextText
			}
			if previousRequestIDs, ok := schemas.SafeExtractStringSlice(unifaiReq.Params.ExtraParams["previous_request_ids"]); ok {
				delete(elevenlabsReq.ExtraParams, "previous_request_ids")
				elevenlabsReq.PreviousRequestIDs = previousRequestIDs
			}
			if nextRequestIDs, ok := schemas.SafeExtractStringSlice(unifaiReq.Params.ExtraParams["next_request_ids"]); ok {
				delete(elevenlabsReq.ExtraParams, "next_request_ids")
				elevenlabsReq.NextRequestIDs = nextRequestIDs
			}
			if applyTextNormalization, ok := schemas.SafeExtractStringPointer(unifaiReq.Params.ExtraParams["apply_text_normalization"]); ok {
				delete(elevenlabsReq.ExtraParams, "apply_text_normalization")
				elevenlabsReq.ApplyTextNormalization = applyTextNormalization
			}
			if applyLanguageTextNormalization, ok := schemas.SafeExtractBoolPointer(unifaiReq.Params.ExtraParams["apply_language_text_normalization"]); ok {
				delete(elevenlabsReq.ExtraParams, "apply_language_text_normalization")
				elevenlabsReq.ApplyLanguageTextNormalization = applyLanguageTextNormalization
			}
			if usePVCAsIVC, ok := schemas.SafeExtractBoolPointer(unifaiReq.Params.ExtraParams["use_pvc_as_ivc"]); ok {
				delete(elevenlabsReq.ExtraParams, "use_pvc_as_ivc")
				elevenlabsReq.UsePVCAsIVC = usePVCAsIVC
			}
		}

		if hasVoiceSettings {
			elevenlabsReq.VoiceSettings = &voiceSettings
		}

		if unifaiReq.Params.LanguageCode != nil {
			elevenlabsReq.LanguageCode = unifaiReq.Params.LanguageCode
		}

		if len(unifaiReq.Params.PronunciationDictionaryLocators) > 0 {
			elevenlabsReq.PronunciationDictionaryLocators = make([]ElevenlabsPronunciationDictionaryLocator, len(unifaiReq.Params.PronunciationDictionaryLocators))
			for i, locator := range unifaiReq.Params.PronunciationDictionaryLocators {
				elevenlabsReq.PronunciationDictionaryLocators[i] = ElevenlabsPronunciationDictionaryLocator{
					PronunciationDictionaryID: locator.PronunciationDictionaryID,
					VersionID:                 locator.VersionID,
				}
			}
		}
	}

	return elevenlabsReq
}
