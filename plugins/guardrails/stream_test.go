package guardrails

import (
	"context"
	"testing"

	"github.com/unifai/unifai/core/schemas"
)

func newStreamTestPlugin(t *testing.T) *GuardrailsPlugin {
	t.Helper()
	cfg := &Config{
		GuardrailProviders: []GuardrailProvider{{
			ID: 1, ProviderName: "regex", Enabled: true,
			Config: map[string]interface{}{
				"patterns": []interface{}{map[string]interface{}{"pattern": `\b\d{3}-\d{2}-\d{4}\b`, "description": "SSN"}},
			},
		}},
		GuardrailRules: []GuardrailRule{{
			ID: 1, Name: "No SSN", Enabled: true, CELExpression: "true", ApplyTo: "output", ProviderConfigIDs: []int{1},
		}},
	}
	p, err := Init(context.Background(), cfg, &dummyLogger{})
	if err != nil {
		t.Fatal(err)
	}
	return p.(*GuardrailsPlugin)
}

func chatDelta(text string) *schemas.UnifAIResponse {
	return &schemas.UnifAIResponse{ChatResponse: &schemas.UnifAIChatResponse{Choices: []schemas.UnifAIResponseChoice{{
		ChatStreamResponseChoice: &schemas.ChatStreamResponseChoice{Delta: &schemas.ChatStreamResponseChoiceDelta{Content: &text}},
	}}}}
}

func responsesDelta(text string) *schemas.UnifAIResponse {
	return &schemas.UnifAIResponse{ResponsesStreamResponse: &schemas.UnifAIResponsesStreamResponse{
		Type: schemas.ResponsesStreamResponseTypeOutputTextDelta, Delta: &text,
	}}
}

func TestStreamBlocksMatchSplitAcrossChunksAndDropsRest(t *testing.T) {
	for name, mk := range map[string]func(string) *schemas.UnifAIResponse{"chat": chatDelta, "responses": responsesDelta} {
		p := newStreamTestPlugin(t)
		ctx := &schemas.UnifAIContext{}
		chunks := []string{"your number is 123", "-45-", "6789 ok", " more text"}
		var results []*schemas.UnifAIError
		for _, c := range chunks {
			_, uerr, _ := p.PostLLMHook(ctx, mk(c), nil)
			results = append(results, uerr)
		}
		if results[0] != nil || results[1] != nil {
			t.Fatalf("%s: chunks before the match must pass, got %+v %+v", name, results[0], results[1])
		}
		if results[2] == nil || results[2].StreamControl != nil {
			t.Fatalf("%s: chunk completing the SSN must return a violation error, got %+v", name, results[2])
		}
		if results[3] == nil || results[3].StreamControl == nil || results[3].StreamControl.SkipStream == nil || !*results[3].StreamControl.SkipStream {
			t.Fatalf("%s: chunks after a block must be skipped, got %+v", name, results[3])
		}
	}
}

func TestStreamCleanOutputPasses(t *testing.T) {
	p := newStreamTestPlugin(t)
	ctx := &schemas.UnifAIContext{}
	for _, c := range []string{"hello ", "world ", "123-45"} {
		if _, uerr, _ := p.PostLLMHook(ctx, responsesDelta(c), nil); uerr != nil {
			t.Fatalf("clean chunk %q blocked: %+v", c, uerr)
		}
	}
}
