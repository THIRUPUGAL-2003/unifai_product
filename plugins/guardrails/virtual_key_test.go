package guardrails

import (
	"context"
	"testing"

	"github.com/raksha/raksha/core/schemas"
)

type dummyLogger struct{}

func (d *dummyLogger) Debug(msg string, args ...any) {}
func (d *dummyLogger) Info(msg string, args ...any)  {}
func (d *dummyLogger) Warn(msg string, args ...any)  {}
func (d *dummyLogger) Error(msg string, args ...any) {}
func (d *dummyLogger) Fatal(msg string, args ...any) {}
func (d *dummyLogger) SetLevel(level schemas.LogLevel) {}
func (d *dummyLogger) SetOutputType(outputType schemas.LoggerOutputType) {}
func (d *dummyLogger) LogHTTPRequest(level schemas.LogLevel, msg string) schemas.LogEventBuilder { return nil }


func TestVirtualKeyBinding(t *testing.T) {
	cfg := &Config{
		GuardrailProviders: []GuardrailProvider{
			{
				ID:           1,
				ProviderName: "regex",
				PolicyName:   "Block Secret",
				Enabled:      true,
				Config: map[string]interface{}{
					"patterns": []interface{}{
						map[string]interface{}{"pattern": `TOP_SECRET`, "description": "secret"},
					},
				},
			},
		},
		GuardrailRules: []GuardrailRule{
			{
				ID:                10,
				Name:              "VK Scoped Rule",
				Enabled:           true,
				CELExpression:     "true",
				ApplyTo:           "input",
				ProviderConfigIDs: []int{1},
				VirtualKeyIDs:     []string{"vk-scoped-123"},
			},
			{
				ID:                20,
				Name:              "Global Rule",
				Enabled:           true,
				CELExpression:     "true",
				ApplyTo:           "input",
				ProviderConfigIDs: []int{1},
				VirtualKeyIDs:     []string{}, // Global rule applies to all
			},
		},
	}

	p, err := Init(context.Background(), cfg, &dummyLogger{})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	gp := p.(*GuardrailsPlugin)

	secretText := "This contains TOP_SECRET content"
	cleanText := "This is safe text"

	makeReq := func(text string) *schemas.RakshaRequest {
		return &schemas.RakshaRequest{
			ChatRequest: &schemas.RakshaChatRequest{
				Model: "gpt-4",
				Input: []schemas.ChatMessage{
					{
						Role: "user",
						Content: &schemas.ChatMessageContent{
							ContentBlocks: []schemas.ChatContentBlock{
								{
									Type: schemas.ChatContentBlockTypeText,
									Text: &text,
								},
							},
						},
					},
				},
			},
		}
	}

	// 1. Request with matching virtual key should be evaluated and blocked on violation
	ctxMatching := &schemas.RakshaContext{}
	ctxMatching.SetValue(schemas.RakshaContextKeyGovernanceVirtualKeyID, "vk-scoped-123")
	_, shortCircuit, err := gp.PreLLMHook(ctxMatching, makeReq(secretText))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shortCircuit == nil {
		t.Fatal("expected short circuit for matching VK with violation")
	}

	// 2. Clean request with matching virtual key passes
	_, shortCircuitClean, err := gp.PreLLMHook(ctxMatching, makeReq(cleanText))
	if err != nil || shortCircuitClean != nil {
		t.Fatalf("expected clean request to pass, got sc=%v, err=%v", shortCircuitClean, err)
	}

	// 3. Test rule with non-matching VK when only scoped rule is present
	cfgScopedOnly := &Config{
		GuardrailProviders: cfg.GuardrailProviders,
		GuardrailRules: []GuardrailRule{
			cfg.GuardrailRules[0], // only VK Scoped Rule
		},
	}
	pScopedOnly, _ := Init(context.Background(), cfgScopedOnly, &dummyLogger{})
	gpScopedOnly := pScopedOnly.(*GuardrailsPlugin)

	ctxOtherVK := &schemas.RakshaContext{}
	ctxOtherVK.SetValue(schemas.RakshaContextKeyGovernanceVirtualKeyID, "vk-other-999")
	_, scOther, _ := gpScopedOnly.PreLLMHook(ctxOtherVK, makeReq(secretText))
	if scOther != nil {
		t.Fatal("expected request with different VK to skip scoped rule, but it was blocked")
	}

	// 4. Request with no VK should also skip scoped rule
	ctxNoVK := &schemas.RakshaContext{}
	_, scNoVK, _ := gpScopedOnly.PreLLMHook(ctxNoVK, makeReq(secretText))
	if scNoVK != nil {
		t.Fatal("expected request without VK to skip scoped rule, but it was blocked")
	}

	// 5. Raw VK matching
	ctxRawVK := &schemas.RakshaContext{}
	ctxRawVK.SetValue(schemas.RakshaContextKeyVirtualKey, "vk-scoped-123")
	_, scRaw, _ := gpScopedOnly.PreLLMHook(ctxRawVK, makeReq(secretText))
	if scRaw == nil {
		t.Fatal("expected raw VK header match to trigger rule and block violation")
	}

	// 6. Global rule (empty VirtualKeyIDs) blocks all keys on violation
	cfgGlobalOnly := &Config{
		GuardrailProviders: cfg.GuardrailProviders,
		GuardrailRules: []GuardrailRule{
			cfg.GuardrailRules[1], // only Global Rule
		},
	}
	pGlobalOnly, _ := Init(context.Background(), cfgGlobalOnly, &dummyLogger{})
	gpGlobalOnly := pGlobalOnly.(*GuardrailsPlugin)

	_, scGlobalNoVK, _ := gpGlobalOnly.PreLLMHook(ctxNoVK, makeReq(secretText))
	if scGlobalNoVK == nil {
		t.Fatal("expected global rule to block violation even without VK")
	}

	_, scGlobalAnyVK, _ := gpGlobalOnly.PreLLMHook(ctxOtherVK, makeReq(secretText))
	if scGlobalAnyVK == nil {
		t.Fatal("expected global rule to block violation with any VK")
	}
}
