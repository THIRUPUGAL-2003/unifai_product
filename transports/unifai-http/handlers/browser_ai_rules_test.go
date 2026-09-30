package handlers

import (
	"context"
	"regexp"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/unifai/unifai/framework/logstore"
	"github.com/valyala/fasthttp"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestImportRules_SkipsDuplicateNamesAndPatterns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_import="+t.Name()), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	m := logstore.NewBrowserAIManager(db)
	if err := m.AutoMigrate(context.Background()); err != nil {
		t.Skipf("migrate: %v", err)
	}
	existing := logstore.BrowserGuardRule{Name: "PAN card", RuleType: "regex", Pattern: `\b[A-Z]{5}[0-9]{4}[A-Z]\b`, Action: "BLOCK", Active: true}
	if err := m.CreateRule(context.Background(), &existing); err != nil {
		t.Fatal(err)
	}
	h := &BrowserAIHandler{manager: m}

	body := `{"overwrite":false,"rules":[
		{"name":"pan card","pattern":"\\b[A-Z]{5}[0-9]{4}[A-Z]\\b"},
		{"name":"PAN (copy)","pattern":"\\b[A-Z]{5}[0-9]{4}[A-Z]\\b"},
		{"name":"Salary","pattern":"salary"},
		{"name":"salary","pattern":"payroll"},
		{"name":"Payroll words","pattern":"salary"},
		{"name":"Aadhaar","pattern":"\\b\\d{4}\\s\\d{4}\\s\\d{4}\\b"}
	]}`
	var rc fasthttp.RequestCtx
	rc.Request.SetBody([]byte(body))
	h.importRules(&rc)
	var res map[string]any
	_ = sonic.Unmarshal(rc.Response.Body(), &res)
	if res["imported"].(float64) != 2 || res["already_exists"].(float64) != 2 || res["duplicates_in_file"].(float64) != 2 {
		t.Fatalf("want 2 imported, 2 already existing, 2 in-file duplicates; got %s", rc.Response.Body())
	}
	rules, _ := m.GetRules(context.Background())
	if len(rules) != 3 {
		t.Errorf("want 3 rules in DB, got %d", len(rules))
	}
}

func TestNormalizeImportedRulePattern(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		matchTexts  []string
		noMatchText []string
	}{
		{
			name:        "Comma separated keywords",
			input:       "salary, bonus, ctc",
			expected:    `\b(?:salary|bonus|ctc)\b`,
			matchTexts:  []string{"what is my salary?", "yearly bonus announcement", "my ctc is high"},
			noMatchText: []string{"unrelated text here", "salaries"},
		},
		{
			name:        "Semicolon and newline separated with special characters",
			input:       "api.key;\nauth.token;\n$secret",
			expected:    `(?:\bapi\.key\b|\bauth\.token\b|\$secret\b)`,
			matchTexts:  []string{"here is api.key", "check auth.token please", "this is $secret value"},
			noMatchText: []string{"apikey without dot", "authtoken"},
		},
		{
			name:        "Pipe separated keywords",
			input:       "password|credentials|private_key",
			expected:    `\b(?:password|credentials|private_key)\b`,
			matchTexts:  []string{"enter password now", "leaked credentials found", "export private_key"},
			noMatchText: []string{"normal message"},
		},
		{
			name:        "Existing valid regex preserved",
			input:       `(?i)\b(bearer\s+[a-z0-9_\-\.]+)\b`,
			expected:    `(?i)\b(bearer\s+[a-z0-9_\-\.]+)\b`,
			matchTexts:  []string{"Bearer abc.123"},
			noMatchText: []string{"no token"},
		},
		{
			name:        "Single keyword with comma trimmed",
			input:       "confidential,",
			expected:    `\bconfidential\b`,
			matchTexts:  []string{"strictly confidential doc"},
			noMatchText: []string{"public doc"},
		},
		{
			name:        "Regex with comma quantifier preserved",
			input:       `[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`,
			expected:    `[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`,
			matchTexts:  []string{"mail me at john.doe@example.com"},
			noMatchText: []string{"no address here"},
		},
		{
			name:        "Regex alternation preserved",
			input:       `\d{4}|[A-Z]{5}\d{4}[A-Z]`,
			expected:    `\d{4}|[A-Z]{5}\d{4}[A-Z]`,
			matchTexts:  []string{"pin 1234", "PAN ABCDE1234F"},
			noMatchText: []string{"no digits"},
		},
		{
			name:        "Invalid regex keywords fall back to escaped list",
			input:       "c++, c#",
			expected:    `(?:\bc\+\+|\bc#)`,
			matchTexts:  []string{"I write c++ daily", "and c# too"},
			noMatchText: []string{"python only"},
		},
		{
			name:        "Empty or whitespace",
			input:       "   ",
			expected:    "",
			matchTexts:  nil,
			noMatchText: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := normalizeImportedRulePattern(tt.input)
			if actual != tt.expected {
				t.Errorf("normalizeImportedRulePattern(%q) = %q, expected %q", tt.input, actual, tt.expected)
			}
			if actual != "" && len(tt.matchTexts) > 0 {
				re, err := regexp.Compile(actual)
				if err != nil {
					t.Fatalf("compiled regex error for pattern %q: %v", actual, err)
				}
				for _, matchText := range tt.matchTexts {
					if !re.MatchString(matchText) {
						t.Errorf("expected pattern %q to match %q, but it didn't", actual, matchText)
					}
				}
				for _, noMatch := range tt.noMatchText {
					if re.MatchString(noMatch) {
						t.Errorf("expected pattern %q NOT to match %q, but it did", actual, noMatch)
					}
				}
			}
		})
	}
}
