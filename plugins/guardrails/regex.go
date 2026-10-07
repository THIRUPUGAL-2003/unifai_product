package guardrails

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gateway/gateway/core/schemas"
)

type RegexProvider struct {
	id       int
	patterns []RegexPattern
}

type RegexPattern struct {
	Pattern     string
	Description string
	Flags       string
	compiled    *regexp.Regexp
}

func NewRegexProvider(config GuardrailProvider) (*RegexProvider, error) {
	provider := &RegexProvider{
		id: config.ID,
	}

	patternsRaw, ok := config.Config["patterns"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("patterns configuration missing or invalid")
	}

	for _, pRaw := range patternsRaw {
		var patternStr, desc, flags string

		switch p := pRaw.(type) {
		case string:
			patternStr = strings.TrimSpace(p)
		case map[string]interface{}:
			patternStr, _ = p["pattern"].(string)
			desc, _ = p["description"].(string)
			flags, _ = p["flags"].(string)
		default:
			continue
		}

		if patternStr == "" {
			continue
		}

		expr := patternStr
		if flags == "i" {
			expr = "(?i)" + expr
		}

		compiled, err := regexp.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern %q: %w", patternStr, err)
		}

		provider.patterns = append(provider.patterns, RegexPattern{
			Pattern:     patternStr,
			Description: desc,
			Flags:       flags,
			compiled:    compiled,
		})
	}

	return provider, nil
}

func (p *RegexProvider) ValidateInput(ctx *schemas.GatewayContext, req *schemas.GatewayRequest) error {
	if req == nil {
		return nil
	}
	var texts []string
	if req.ChatRequest != nil {
		for _, msg := range req.ChatRequest.Input {
			if strings.EqualFold(string(msg.Role), "assistant") {
				continue
			}
			texts = append(texts, extractChatMessageTexts(msg)...)
		}
	}
	// Anthropic /v1/messages and OpenAI /v1/responses arrive as Responses requests.
	if req.ResponsesRequest != nil {
		if params := req.ResponsesRequest.Params; params != nil && params.Instructions != nil && *params.Instructions != "" {
			texts = append(texts, *params.Instructions)
		}
		for _, msg := range req.ResponsesRequest.Input {
			if msg.Role != nil && strings.EqualFold(string(*msg.Role), "assistant") {
				continue
			}
			texts = append(texts, extractResponsesMessageTexts(msg)...)
		}
	}
	for _, content := range texts {
		if err := p.matchBlocked(content, "input"); err != nil {
			return err
		}
	}
	return nil
}

func (p *RegexProvider) ValidateOutput(ctx *schemas.GatewayContext, req *schemas.GatewayRequest, resp *schemas.GatewayResponse) error {
	for _, content := range extractChatOutputTexts(resp) {
		if err := p.matchBlocked(content, "output"); err != nil {
			return err
		}
	}
	return nil
}

// MatchText runs patterns against an arbitrary string (used for streamed accumulation).
func (p *RegexProvider) MatchText(content, phase string) error {
	return p.matchBlocked(content, phase)
}

func looksLikeRegex(s string) bool {
	for _, sub := range []string{"^", "$", "\\d", "\\w", "\\s", "\\b", "(?:", "(?=", "[", "]", "{", "}", "*", "+", "|", "(?"} {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func (p *RegexProvider) matchBlocked(content, phase string) error {
	for _, pattern := range p.patterns {
		if pattern.compiled.MatchString(content) {
			desc := strings.TrimSpace(pattern.Description)
			if desc != "" && !looksLikeRegex(desc) && desc != pattern.Pattern {
				return fmt.Errorf("%s detected", desc)
			}
			return fmt.Errorf("restricted content detected")
		}
	}
	return nil
}

func extractChatMessageTexts(msg schemas.ChatMessage) []string {
	if msg.Content == nil {
		return nil
	}
	var texts []string
	if msg.Content.ContentStr != nil && *msg.Content.ContentStr != "" {
		texts = append(texts, *msg.Content.ContentStr)
	}
	for _, block := range msg.Content.ContentBlocks {
		if block.Type == schemas.ChatContentBlockTypeText && block.Text != nil && *block.Text != "" {
			texts = append(texts, *block.Text)
		}
		if block.Type == schemas.ChatContentBlockTypeRefusal && block.Refusal != nil && *block.Refusal != "" {
			texts = append(texts, *block.Refusal)
		}
		if block.Type == schemas.ChatContentBlockTypeFile {
			if text := inlineTextFileContent(block.File); text != "" {
				texts = append(texts, text)
			}
		}
	}
	return texts
}

// maxInlineFileScanBytes caps how much of an attached text file is decoded for pattern matching.
const maxInlineFileScanBytes = 2 << 20

var textFileExtensions = []string{".txt", ".csv", ".tsv", ".json", ".md", ".log", ".xml", ".yaml", ".yml", ".html", ".htm", ".sql", ".ini", ".env"}

// inlineTextFileContent returns the decoded body of an inline text-like attachment
// (plain text, CSV, JSON, ...). Binary formats (PDF, Office, images) return "".
func inlineTextFileContent(file *schemas.ChatInputFile) string {
	if file == nil || file.FileData == nil || *file.FileData == "" {
		return ""
	}
	data := *file.FileData
	mime := ""
	if file.FileType != nil {
		mime = strings.ToLower(strings.TrimSpace(*file.FileType))
	}
	isBase64 := true
	if strings.HasPrefix(data, "data:") {
		comma := strings.IndexByte(data, ',')
		if comma < 0 {
			return ""
		}
		header := strings.ToLower(data[5:comma])
		isBase64 = strings.HasSuffix(header, ";base64")
		mime = strings.TrimSuffix(header, ";base64")
		data = data[comma+1:]
	}
	if !isTextLikeFile(mime, file.Filename) {
		return ""
	}
	if !isBase64 {
		return truncateForScan(data)
	}
	encoded := data
	if len(encoded) > base64.StdEncoding.EncodedLen(maxInlineFileScanBytes) {
		encoded = encoded[:base64.StdEncoding.EncodedLen(maxInlineFileScanBytes)]
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		// Anthropic-style {type:"text"} sources carry plain text in file_data.
		return truncateForScan(data)
	}
	if !utf8.Valid(decoded) {
		return ""
	}
	return string(decoded)
}

func isTextLikeFile(mime string, filename *string) bool {
	if strings.HasPrefix(mime, "text/") || mime == "application/json" || mime == "application/xml" ||
		mime == "application/x-yaml" || mime == "application/yaml" || mime == "application/sql" {
		return true
	}
	if filename != nil {
		name := strings.ToLower(*filename)
		for _, ext := range textFileExtensions {
			if strings.HasSuffix(name, ext) {
				return true
			}
		}
	}
	return false
}

func truncateForScan(s string) string {
	if len(s) > maxInlineFileScanBytes {
		return s[:maxInlineFileScanBytes]
	}
	return s
}

func extractChatOutputTexts(resp *schemas.GatewayResponse) []string {
	if resp == nil {
		return nil
	}
	var texts []string
	if resp.ChatResponse != nil {
		for _, choice := range resp.ChatResponse.Choices {
			if choice.ChatNonStreamResponseChoice != nil && choice.ChatNonStreamResponseChoice.Message != nil {
				texts = append(texts, extractChatMessageTexts(*choice.ChatNonStreamResponseChoice.Message)...)
			}
		}
	}
	if resp.ResponsesResponse != nil {
		for _, msg := range resp.ResponsesResponse.Output {
			texts = append(texts, extractResponsesMessageTexts(msg)...)
		}
	}
	return texts
}

func extractResponsesMessageTexts(msg schemas.ResponsesMessage) []string {
	if msg.Content == nil {
		return nil
	}
	var texts []string
	if msg.Content.ContentStr != nil && *msg.Content.ContentStr != "" {
		texts = append(texts, *msg.Content.ContentStr)
	}
	for _, block := range msg.Content.ContentBlocks {
		if block.Text != nil && *block.Text != "" {
			texts = append(texts, *block.Text)
		}
		if block.ResponsesOutputMessageContentRefusal != nil && block.ResponsesOutputMessageContentRefusal.Refusal != "" {
			texts = append(texts, block.ResponsesOutputMessageContentRefusal.Refusal)
		}
		if f := block.ResponsesInputMessageContentBlockFile; f != nil {
			if text := inlineTextFileContent(&schemas.ChatInputFile{FileData: f.FileData, Filename: f.Filename, FileType: f.FileType}); text != "" {
				texts = append(texts, text)
			}
		}
	}
	return texts
}
