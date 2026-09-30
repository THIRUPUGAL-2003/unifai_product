package elevenlabs

var (
	// Maps provider-specific finish reasons to UnifAI format
	unifaiToElevenlabsSpeechFormat = map[string]string{
		"":     "mp3_44100_128",
		"mp3":  "mp3_44100_128",
		"opus": "opus_48000_128",
		"wav":  "pcm_44100",
		"pcm":  "pcm_44100",
	}

	// Maps UnifAI finish reasons to provider-specific format
	elevenlabsSpeechFormatToUnifAI = map[string]string{
		"mp3_44100_128":  "mp3",
		"opus_48000_128": "opus",
		"pcm_44100":      "wav",
	}
)

// ConvertUnifAISpeechFormatToElevenlabs converts UnifAI speech format to Elevenlabs format
func ConvertUnifAISpeechFormatToElevenlabs(format string) string {
	if elevenlabsFormat, ok := unifaiToElevenlabsSpeechFormat[format]; ok {
		return elevenlabsFormat
	}
	return format
}

// ConvertElevenlabsSpeechFormatToUnifAI converts Elevenlabs speech format to UnifAI format
func ConvertElevenlabsSpeechFormatToUnifAI(format string) string {
	if unifaiFormat, ok := elevenlabsSpeechFormatToUnifAI[format]; ok {
		return unifaiFormat
	}
	return format
}
