package logstore

import (
	"testing"
)

func TestLooksLikeBinaryOrWireGarbage_SymbolsAndLanguages(t *testing.T) {
	validPrompts := []string{
		"@$$$%@#$%$%@$%",
		"!@#$%^%%#$%@#!^",
		"$$$",
		"???",
		"!@#$",
		"$$",
		"fgfuhc)",
		"வணக்கம்",                                // Tamil
		"வணக்கம் எப்படி இருக்கிறீர்கள்?", // Tamil sentence
		"你好，世界",                              // Chinese
		"مرحبا بك",                              // Arabic
		"Привет мир",                            // Russian
		"नमस्ते आप कैसे हैं",                  // Hindi
		"എന്തൊക്കെയുണ്ട് വിശേഷങ്ങൾ?",          // Malayalam
		"హలో ఎలా ఉన్నారు?",                      // Telugu
		"P@ssw0rd!#$2026",
		"console.log('test') && rm -rf /*",
	}

	for _, p := range validPrompts {
		if looksLikeBinaryOrWireGarbage(p) {
			t.Errorf("Expected prompt %q to NOT be classified as wire garbage, but it was", p)
		}
	}

	garbage := []string{
		"cursor.exe attestation blob",
		"\ufffd\ufffd\ufffd\ufffd\ufffd\ufffd\ufffd\ufffd\ufffd\ufffd garbage",
		"Intel(R) Core(TM) i7 CPU @ 2.60GHz",
	}

	for _, g := range garbage {
		if !looksLikeBinaryOrWireGarbage(g) {
			t.Errorf("Expected %q to be classified as wire garbage, but it was not", g)
		}
	}
}
