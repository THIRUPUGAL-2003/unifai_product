package handlers

import (
	"regexp"
	"testing"
)

func TestStripEvalMarkdown(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain json",
			input:    `{"pattern": "\\bfoo\\b"}`,
			expected: `{"pattern": "\\bfoo\\b"}`,
		},
		{
			name:     "code fence with json tag",
			input:    "```json\n{\"pattern\": \"\\\\bfoo\\\\b\"}\n```",
			expected: `{"pattern": "\\bfoo\\b"}`,
		},
		{
			name: "preamble before markdown fence",
			input: "Here is your generated regex:\n```json\n{\n  \"pattern\": \"\\\\b(secret|token)\\\\b\",\n  \"focus\": \"credentials\"\n}\n```\nLet me know if you need changes.",
			expected: "{\n  \"pattern\": \"\\\\b(secret|token)\\\\b\",\n  \"focus\": \"credentials\"\n}",
		},
		{
			name:     "fence with regex tag",
			input:    "```regex\n\\b[0-9]{12}\\b\n```",
			expected: `\b[0-9]{12}\b`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripEvalMarkdown(tc.input)
			if got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestSanitizeGeneratedRegex(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "js style slashes with i flag",
			input:    `/\b(api_key|secret)\b/i`,
			expected: `\b(api_key|secret)\b`,
		},
		{
			name:     "js style slashes without flag",
			input:    `/\b(password)\b/`,
			expected: `\b(password)\b`,
		},
		{
			name:     "inline flags prefix (?i) and (?m)",
			input:    `(?i)\bcredentials\b`,
			expected: `\bcredentials\b`,
		},
		{
			name:     "ascii 0x08 backspace converted to \\b",
			input:    "\x08password\x08",
			expected: `\bpassword\b`,
		},
		{
			name:     "unsupported lookahead stripped if remainder compiles",
			input:    `(?=.*\d)\bsecret\b`,
			expected: `\bsecret\b`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeGeneratedRegex(tc.input)
			if got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
			if _, err := regexp.Compile("(?i)" + got); err != nil {
				t.Fatalf("pattern %q failed to compile in RE2: %v", got, err)
			}
		})
	}
}

func TestIsUsableGeneratedRegex(t *testing.T) {
	usable := []string{
		`credit card`,
		`salary slip`,
		`patient health record`,
		`\b[A-Z]{5}[0-9]{4}[A-Z]\b`,
		`\b\d{4}\s?\d{4}\s?\d{4}\b`,
		`\b(secret|password|token)\b`,
		`api[_-]?key`,
	}
	for _, pat := range usable {
		if !isUsableGeneratedRegex(pat) {
			t.Errorf("expected %q to be usable regex, but got false", pat)
		}
	}

	unusable := []string{
		"",
		"a",
		"[]",
		"This policy blocks all credit card numbers from employees when sent to chat",
		"http://example.com/pattern",
		"you should match password here",
		"[unclosed[regex",
	}
	for _, pat := range unusable {
		if isUsableGeneratedRegex(pat) {
			t.Errorf("expected %q to be unusable, but got true", pat)
		}
	}
}

func TestParseGeneratedRegexPayload(t *testing.T) {
	t.Run("clean json", func(t *testing.T) {
		raw := `{"pattern": "\\bsecret\\b", "focus": "auth", "notes": "ok"}`
		pat, focus, notes := parseGeneratedRegexPayload(raw)
		if pat != `\bsecret\b` || focus != "auth" || notes != "ok" {
			t.Fatalf("unexpected parse: pat=%q focus=%q notes=%q", pat, focus, notes)
		}
	})

	t.Run("embedded json in prose", func(t *testing.T) {
		raw := `Sure, here is the requested JSON format:
{"pattern": "\\b(aadhaar|pan)\\b", "focus": "pii", "notes": "identity cards"}
Hope this helps!`
		pat, focus, notes := parseGeneratedRegexPayload(raw)
		if pat != `\b(aadhaar|pan)\b` || focus != "pii" || notes != "identity cards" {
			t.Fatalf("unexpected parse: pat=%q focus=%q notes=%q", pat, focus, notes)
		}
	})

	t.Run("regex fallback line", func(t *testing.T) {
		raw := "credit card"
		pat, _, _ := parseGeneratedRegexPayload(raw)
		if pat != "credit card" {
			t.Fatalf("expected 'credit card', got %q", pat)
		}
	})
}

func TestParseAIBotDecision(t *testing.T) {
	positives := []string{
		`{"violation": true}`,
		`{"violation": 1}`,
		`{"violated": true}`,
		`{"is_violation": true}`,
		`{"blocked": true}`,
		`violation: true`,
		`true`,
		`violation`,
		`blocked`,
		"After analyzing the prompt, here is my decision:\n{\"violation\": true}",
	}
	for _, p := range positives {
		v, ok := parseAIBotDecision(p)
		if !ok || !v {
			t.Errorf("expected positive violation for %q, got ok=%v, v=%v", p, ok, v)
		}
	}

	negatives := []string{
		`{"violation": false}`,
		`{"violation": 0}`,
		`{"violated": false}`,
		`{"is_violation": false}`,
		`violation: false`,
		`false`,
		`clear`,
		`safe`,
		`allowed`,
		"The text is completely safe and does not break policy.\n{\"violation\": false}",
	}
	for _, n := range negatives {
		v, ok := parseAIBotDecision(n)
		if !ok || v {
			t.Errorf("expected safe decision (false) for %q, got ok=%v, v=%v", n, ok, v)
		}
	}
}
