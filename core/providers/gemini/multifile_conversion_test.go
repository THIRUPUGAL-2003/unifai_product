package gemini

import (
	"context"
	"testing"
	"time"

	"github.com/gateway/gateway/core/providers/anthropic"
	"github.com/gateway/gateway/core/providers/openai"
	"github.com/gateway/gateway/core/schemas"
)

func TestMultiFileGeminiConversion(t *testing.T) {
	ctx := schemas.NewGatewayContext(context.Background(), time.Now())

	text1 := "Attached file: test1.py\n\n--- extracted content ---\nprint('hello')"
	emptyText := ""
	text2 := "Attached file: test2.csv\n\n--- extracted content ---\na,b,c\n1,2,3"

	req := &schemas.GatewayChatRequest{
		Model: "gemini-1.5-pro",
		Input: []schemas.ChatMessage{
			{
				Role: schemas.ChatMessageRoleUser,
				Content: &schemas.ChatMessageContent{
					ContentBlocks: []schemas.ChatContentBlock{
						{Type: schemas.ChatContentBlockTypeText, Text: &emptyText},
						{Type: schemas.ChatContentBlockTypeText, Text: &text1},
						{Type: schemas.ChatContentBlockTypeText, Text: &text2},
					},
				},
			},
		},
	}

	geminiReq, err := ToGeminiChatCompletionRequest(ctx, req)
	if err != nil {
		t.Fatalf("ToGeminiChatCompletionRequest failed: %v", err)
	}

	if len(geminiReq.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(geminiReq.Contents))
	}

	parts := geminiReq.Contents[0].Parts
	if len(parts) != 1 {
		t.Fatalf("expected consecutive text parts to be merged into 1 part, got %d", len(parts))
	}

	if parts[0].Text == "" {
		t.Fatalf("expected non-empty text part in Gemini")
	}

	t.Logf("Gemini merged text length: %d", len(parts[0].Text))
}

func TestMultiFileAnthropicConversion(t *testing.T) {
	ctx := schemas.NewGatewayContext(context.Background(), time.Now())

	text1 := "First file: a.py"
	text2 := "Second file: b.csv"

	req := &schemas.GatewayChatRequest{
		Model: "claude-3-5-sonnet-20241022",
		Input: []schemas.ChatMessage{
			{
				Role: schemas.ChatMessageRoleUser,
				Content: &schemas.ChatMessageContent{
					ContentBlocks: []schemas.ChatContentBlock{
						{Type: schemas.ChatContentBlockTypeText, Text: &text1},
						{Type: schemas.ChatContentBlockTypeText, Text: &text2},
					},
				},
			},
		},
	}

	anthropicReq, err := anthropic.ToAnthropicChatRequest(ctx, req)
	if err != nil {
		t.Fatalf("ToAnthropicChatRequest failed: %v", err)
	}

	if len(anthropicReq.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(anthropicReq.Messages))
	}

	blocks := anthropicReq.Messages[0].Content.ContentBlocks
	if len(blocks) != 1 {
		t.Fatalf("expected consecutive text blocks to be merged into 1 block for Anthropic, got %d", len(blocks))
	}

	if blocks[0].Text == nil || *blocks[0].Text == "" {
		t.Fatalf("expected non-empty merged text block for Anthropic")
	}

	t.Logf("Anthropic merged text: %s", *blocks[0].Text)
}

func TestMultiFileOpenAIConversion(t *testing.T) {
	text1 := "First file: a.py"
	text2 := "Second file: b.csv"
	emptyText := "   "

	input := []schemas.ChatMessage{
		{
			Role: schemas.ChatMessageRoleUser,
			Content: &schemas.ChatMessageContent{
				ContentBlocks: []schemas.ChatContentBlock{
					{Type: schemas.ChatContentBlockTypeText, Text: &emptyText},
					{Type: schemas.ChatContentBlockTypeText, Text: &text1},
					{Type: schemas.ChatContentBlockTypeText, Text: &text2},
				},
			},
		},
	}

	openaiMessages := openai.ConvertGatewayMessagesToOpenAIMessages(input)
	if len(openaiMessages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(openaiMessages))
	}

	msg := openaiMessages[0]
	if msg.Content == nil || msg.Content.ContentStr == nil {
		t.Fatalf("expected pure text blocks to be combined into ContentStr for OpenAI")
	}

	if *msg.Content.ContentStr == "" {
		t.Fatalf("expected non-empty ContentStr for OpenAI")
	}

	t.Logf("OpenAI combined ContentStr: %s", *msg.Content.ContentStr)
}
