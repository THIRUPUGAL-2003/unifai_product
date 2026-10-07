package elevenlabs

var (
	// Maps provider-specific finish reasons to Gateway format
	gatewayToElevenlabsSpeechFormat = map[string]string{
		"":     "mp3_44100_128",
		"mp3":  "mp3_44100_128",
		"opus": "opus_48000_128",
		"wav":  "pcm_44100",
		"pcm":  "pcm_44100",
	}

	// Maps Gateway finish reasons to provider-specific format
	elevenlabsSpeechFormatToGateway = map[string]string{
		"mp3_44100_128":  "mp3",
		"opus_48000_128": "opus",
		"pcm_44100":      "wav",
	}
)

// ConvertGatewaySpeechFormatToElevenlabs converts Gateway speech format to Elevenlabs format
func ConvertGatewaySpeechFormatToElevenlabs(format string) string {
	if elevenlabsFormat, ok := gatewayToElevenlabsSpeechFormat[format]; ok {
		return elevenlabsFormat
	}
	return format
}

// ConvertElevenlabsSpeechFormatToGateway converts Elevenlabs speech format to Gateway format
func ConvertElevenlabsSpeechFormatToGateway(format string) string {
	if gatewayFormat, ok := elevenlabsSpeechFormatToGateway[format]; ok {
		return gatewayFormat
	}
	return format
}
