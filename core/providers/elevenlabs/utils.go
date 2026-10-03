package elevenlabs

var (
	// Maps provider-specific finish reasons to Raksha format
	rakshaToElevenlabsSpeechFormat = map[string]string{
		"":     "mp3_44100_128",
		"mp3":  "mp3_44100_128",
		"opus": "opus_48000_128",
		"wav":  "pcm_44100",
		"pcm":  "pcm_44100",
	}

	// Maps Raksha finish reasons to provider-specific format
	elevenlabsSpeechFormatToRaksha = map[string]string{
		"mp3_44100_128":  "mp3",
		"opus_48000_128": "opus",
		"pcm_44100":      "wav",
	}
)

// ConvertRakshaSpeechFormatToElevenlabs converts Raksha speech format to Elevenlabs format
func ConvertRakshaSpeechFormatToElevenlabs(format string) string {
	if elevenlabsFormat, ok := rakshaToElevenlabsSpeechFormat[format]; ok {
		return elevenlabsFormat
	}
	return format
}

// ConvertElevenlabsSpeechFormatToRaksha converts Elevenlabs speech format to Raksha format
func ConvertElevenlabsSpeechFormatToRaksha(format string) string {
	if rakshaFormat, ok := elevenlabsSpeechFormatToRaksha[format]; ok {
		return rakshaFormat
	}
	return format
}
