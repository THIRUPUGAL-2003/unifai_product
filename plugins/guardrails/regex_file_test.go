package guardrails

import (
	"encoding/base64"
	"testing"

	"github.com/raksha/raksha/core/schemas"
)

func strPtr(s string) *string { return &s }

func TestInlineTextFileContent(t *testing.T) {
	secret := "ssn 123-45-6789"
	enc := base64.StdEncoding.EncodeToString([]byte(secret))
	cases := []struct {
		name string
		file *schemas.ChatInputFile
		want string
	}{
		{"data url text/plain", &schemas.ChatInputFile{FileData: strPtr("data:text/plain;base64," + enc)}, secret},
		{"raw base64 csv by filename", &schemas.ChatInputFile{FileData: strPtr(enc), Filename: strPtr("a.csv")}, secret},
		{"json by file_type", &schemas.ChatInputFile{FileData: strPtr(enc), FileType: strPtr("application/json")}, secret},
		{"plain text source", &schemas.ChatInputFile{FileData: strPtr(secret + " !"), FileType: strPtr("text/plain")}, secret + " !"},
		{"pdf ignored", &schemas.ChatInputFile{FileData: strPtr("data:application/pdf;base64," + enc)}, ""},
		{"unknown binary ignored", &schemas.ChatInputFile{FileData: strPtr(enc), Filename: strPtr("a.bin")}, ""},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		if got := inlineTextFileContent(tc.file); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestRegexProviderBlocksSecretInsideTextFile(t *testing.T) {
	p, err := NewRegexProvider(GuardrailProvider{ID: 1, Config: map[string]interface{}{
		"patterns": []interface{}{map[string]interface{}{"pattern": `\b\d{3}-\d{2}-\d{4}\b`, "description": "SSN"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	data := "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte("ssn 123-45-6789"))
	msg := schemas.ChatMessage{Content: &schemas.ChatMessageContent{ContentBlocks: []schemas.ChatContentBlock{
		{Type: schemas.ChatContentBlockTypeText, Text: strPtr("see file")},
		{Type: schemas.ChatContentBlockTypeFile, File: &schemas.ChatInputFile{FileData: &data, Filename: strPtr("s.txt")}},
	}}}
	req := &schemas.RakshaRequest{ChatRequest: &schemas.RakshaChatRequest{Input: []schemas.ChatMessage{msg}}}
	if err := p.ValidateInput(nil, req); err == nil {
		t.Fatal("expected SSN inside attached text file to be blocked")
	}
}

func TestRegexProviderCoversResponsesRequestsAndOutput(t *testing.T) {
	p, err := NewRegexProvider(GuardrailProvider{ID: 1, Config: map[string]interface{}{
		"patterns": []interface{}{map[string]interface{}{"pattern": `\b\d{3}-\d{2}-\d{4}\b`, "description": "SSN"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	block := func(text string) []schemas.ResponsesMessage {
		return []schemas.ResponsesMessage{{Content: &schemas.ResponsesMessageContent{ContentBlocks: []schemas.ResponsesMessageContentBlock{
			{Type: schemas.ResponsesInputMessageContentBlockTypeText, Text: strPtr(text)},
		}}}}
	}
	if err := p.ValidateInput(nil, &schemas.RakshaRequest{ResponsesRequest: &schemas.RakshaResponsesRequest{Input: block("ssn 123-45-6789")}}); err == nil {
		t.Fatal("expected SSN in Responses/Anthropic input to be blocked")
	}
	instr := "my ssn is 123-45-6789"
	if err := p.ValidateInput(nil, &schemas.RakshaRequest{ResponsesRequest: &schemas.RakshaResponsesRequest{Params: &schemas.ResponsesParameters{Instructions: &instr}}}); err == nil {
		t.Fatal("expected SSN in Responses instructions to be blocked")
	}
	if err := p.ValidateInput(nil, &schemas.RakshaRequest{ResponsesRequest: &schemas.RakshaResponsesRequest{Input: block("hello")}}); err != nil {
		t.Fatalf("clean Responses input blocked: %v", err)
	}
	resp := &schemas.RakshaResponse{ResponsesResponse: &schemas.RakshaResponsesResponse{Output: block("here: 123-45-6789")}}
	if err := p.ValidateOutput(nil, nil, resp); err == nil {
		t.Fatal("expected SSN in Responses output to be blocked")
	}
}
