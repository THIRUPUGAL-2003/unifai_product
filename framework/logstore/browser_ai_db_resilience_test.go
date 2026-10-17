package logstore

import (
	"strings"
	"testing"
)

func TestSanitizeDBString(t *testing.T) {
	// Test 1: Null byte removal
	withNull := "Hello\x00World\x00! \x00Test"
	clean := SanitizeDBString(withNull)
	if strings.ContainsRune(clean, '\x00') {
		t.Fatalf("Expected null bytes removed, got: %q", clean)
	}
	if clean != "HelloWorld! Test" {
		t.Fatalf("Expected 'HelloWorld! Test', got: %q", clean)
	}

	// Test 2: Invalid UTF-8 bytes sanitized
	invalidUTF8 := "Invalid \xff\xfe UTF-8"
	valid := SanitizeDBString(invalidUTF8)
	if strings.Contains(valid, "\xff") || strings.Contains(valid, "\xfe") {
		t.Fatalf("Expected invalid UTF-8 stripped, got: %q", valid)
	}
}

func TestSafePromptPreview_MultiByteRunes(t *testing.T) {
	// Test 1: Tamil prompt (3 bytes per character in UTF-8)
	tamilPrompt := "நான் ஒரு புதிய படம் உருவாக்க வேண்டும் - அழகான இயற்கை காட்சி"
	preview := SafePromptPreview(tamilPrompt, 20)
	if !strings.HasSuffix(preview, "...") {
		t.Fatalf("Expected ellipsis on truncated preview, got: %q", preview)
	}
	// Verify runes length <= 20
	runes := []rune(preview)
	if len(runes) > 20 {
		t.Fatalf("Expected rune length <= 20, got: %d (%q)", len(runes), preview)
	}

	// Test 2: Hindi prompt with emojis
	hindiPrompt := "अंतरिक्ष में उड़ती हुई एक सफेद बिल्ली 🐱🚀🌌 और सितारे"
	previewHindi := SafePromptPreview(hindiPrompt, 15)
	if len([]rune(previewHindi)) > 15 {
		t.Fatalf("Expected rune length <= 15, got: %d (%q)", len([]rune(previewHindi)), previewHindi)
	}

	// Test 3: Embedded null byte in prompt
	promptWithNull := "Generate\x00 an image of \x00a red sports car"
	cleanPreview := SafePromptPreview(promptWithNull, 100)
	if strings.ContainsRune(cleanPreview, '\x00') {
		t.Fatalf("Expected no null bytes in preview, got: %q", cleanPreview)
	}
}
