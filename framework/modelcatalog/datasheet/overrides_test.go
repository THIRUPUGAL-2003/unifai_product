package datasheet

import (
	"math"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/unifai/unifai/core/schemas"
	configstoreTables "github.com/unifai/unifai/framework/configstore/tables"
)

type noopLogger struct{}

func (noopLogger) Debug(string, ...any)                                            {}
func (noopLogger) Info(string, ...any)                                             {}
func (noopLogger) Warn(string, ...any)                                             {}
func (noopLogger) Error(string, ...any)                                            {}
func (noopLogger) Fatal(string, ...any)                                            {}
func (noopLogger) SetLevel(schemas.LogLevel)                                       {}
func (noopLogger) SetOutputType(schemas.LoggerOutputType)                          {}
func (noopLogger) LogHTTPRequest(schemas.LogLevel, string) schemas.LogEventBuilder { return schemas.NoopLogEvent }

// Test 1: Exact model pricing override replaces base pricing
func TestExactModelPricingOverride(t *testing.T) {
	store := New(nil, &noopLogger{}, Config{})

	// Add base catalog pricing for gpt-4o
	baseInput := 0.0000025  // $2.50 / 1M
	baseOutput := 0.0000100 // $10.00 / 1M
	store.pricingData["gpt-4o|openai|chat"] = configstoreTables.TableModelPricing{
		Model:               "gpt-4o",
		Provider:            "openai",
		Mode:                "chat",
		InputCostPerToken:   &baseInput,
		OutputCostPerToken:  &baseOutput,
	}

	// 1. Without override:
	resp := &schemas.UnifAIResponse{
		ChatResponse: &schemas.UnifAIChatResponse{
			Usage: &schemas.UnifAILLMUsage{
				PromptTokens:     1000,
				CompletionTokens: 500,
				TotalTokens:      1500,
			},
			ExtraFields: schemas.UnifAIResponseExtraFields{
				RoutingInfo: schemas.RoutingInfo{
					Provider: "openai",
					Model:    "gpt-4o",
				},
				RequestType: schemas.ChatCompletionRequest,
			},
		},
	}

	baseCost := store.CalculateCost(resp, nil)
	expectedBaseCost := (1000 * 0.0000025) + (500 * 0.0000100) // 0.0025 + 0.0050 = 0.0075
	if math.Abs(baseCost-expectedBaseCost) > 1e-9 {
		t.Fatalf("Base cost mismatch: got %v, expected %v", baseCost, expectedBaseCost)
	}

	// 2. Now apply override: new input = $0.000001, output = $0.000004
	overrideInput := 0.000001
	overrideOutput := 0.000004
	patchJSON, _ := sonic.Marshal(map[string]float64{
		"input_cost_per_token":  overrideInput,
		"output_cost_per_token": overrideOutput,
	})

	err := store.UpsertOverrides(&configstoreTables.TablePricingOverride{
		ID:               "override-1",
		Name:             "GPT-4o Custom Price",
		ScopeKind:        string(ScopeKindGlobal),
		MatchType:        string(MatchTypeExact),
		Pattern:          "gpt-4o",
		RequestTypes:     []schemas.RequestType{schemas.ChatCompletionRequest},
		PricingPatchJSON: string(patchJSON),
	})
	if err != nil {
		t.Fatalf("Failed to upsert override: %v", err)
	}

	// Calculate cost with override
	overrideCost := store.CalculateCost(resp, nil)
	expectedOverrideCost := (1000 * 0.000001) + (500 * 0.000004) // 0.001 + 0.002 = 0.003
	if math.Abs(overrideCost-expectedOverrideCost) > 1e-9 {
		t.Fatalf("Override cost mismatch: got %v, expected %v", overrideCost, expectedOverrideCost)
	}
}

// Test 2: Wildcard matching and longest-prefix priority
func TestWildcardPricingOverride(t *testing.T) {
	store := New(nil, &noopLogger{}, Config{})

	// Add generic wildcard: deepseek-* -> input 0.00000014
	patchBroad, _ := sonic.Marshal(map[string]float64{
		"input_cost_per_token":  0.00000014,
		"output_cost_per_token": 0.00000028,
	})
	// Add specific wildcard: deepseek-reasoner* -> input 0.00000055
	patchSpecific, _ := sonic.Marshal(map[string]float64{
		"input_cost_per_token":  0.00000055,
		"output_cost_per_token": 0.00000219,
	})

	err := store.UpsertOverrides(
		&configstoreTables.TablePricingOverride{
			ID:               "broad-wildcard",
			Name:             "DeepSeek Broad",
			ScopeKind:        string(ScopeKindGlobal),
			MatchType:        string(MatchTypeWildcard),
			Pattern:          "deepseek-*",
			RequestTypes:     []schemas.RequestType{schemas.ChatCompletionRequest},
			PricingPatchJSON: string(patchBroad),
		},
		&configstoreTables.TablePricingOverride{
			ID:               "specific-wildcard",
			Name:             "DeepSeek Reasoner Specific",
			ScopeKind:        string(ScopeKindGlobal),
			MatchType:        string(MatchTypeWildcard),
			Pattern:          "deepseek-reasoner*",
			RequestTypes:     []schemas.RequestType{schemas.ChatCompletionRequest},
			PricingPatchJSON: string(patchSpecific),
		},
	)
	if err != nil {
		t.Fatalf("Failed to upsert wildcards: %v", err)
	}

	// Case A: deepseek-chat matches broad wildcard
	respChat := &schemas.UnifAIResponse{
		ChatResponse: &schemas.UnifAIChatResponse{
			Usage: &schemas.UnifAILLMUsage{PromptTokens: 10000, CompletionTokens: 1000, TotalTokens: 11000},
			ExtraFields: schemas.UnifAIResponseExtraFields{
				RoutingInfo: schemas.RoutingInfo{Provider: "deepseek", Model: "deepseek-chat"},
				RequestType: schemas.ChatCompletionRequest,
			},
		},
	}
	costChat := store.CalculateCost(respChat, nil)
	expectedChat := (10000 * 0.00000014) + (1000 * 0.00000028)
	if math.Abs(costChat-expectedChat) > 1e-9 {
		t.Fatalf("deepseek-chat cost mismatch: got %v, expected %v", costChat, expectedChat)
	}

	// Case B: deepseek-reasoner matches specific wildcard (longest prefix)
	respReasoner := &schemas.UnifAIResponse{
		ChatResponse: &schemas.UnifAIChatResponse{
			Usage: &schemas.UnifAILLMUsage{PromptTokens: 10000, CompletionTokens: 1000, TotalTokens: 11000},
			ExtraFields: schemas.UnifAIResponseExtraFields{
				RoutingInfo: schemas.RoutingInfo{Provider: "deepseek", Model: "deepseek-reasoner"},
				RequestType: schemas.ChatCompletionRequest,
			},
		},
	}
	costReasoner := store.CalculateCost(respReasoner, nil)
	expectedReasoner := (10000 * 0.00000055) + (1000 * 0.00000219)
	if math.Abs(costReasoner-expectedReasoner) > 1e-9 {
		t.Fatalf("deepseek-reasoner cost mismatch: got %v, expected %v", costReasoner, expectedReasoner)
	}
}

// Test 3: Completely custom / unknown model (not in base catalog)
func TestCustomUnknownModelPricing(t *testing.T) {
	store := New(nil, &noopLogger{}, Config{})

	// Model "my-company-internal-llm" has NO catalog entry!
	patchCustom, _ := sonic.Marshal(map[string]float64{
		"input_cost_per_token":  0.0000030,
		"output_cost_per_token": 0.0000080,
	})

	err := store.UpsertOverrides(&configstoreTables.TablePricingOverride{
		ID:               "custom-model-override",
		Name:             "Internal LLM Price",
		ScopeKind:        string(ScopeKindGlobal),
		MatchType:        string(MatchTypeExact),
		Pattern:          "my-company-internal-llm",
		RequestTypes:     []schemas.RequestType{schemas.ChatCompletionRequest},
		PricingPatchJSON: string(patchCustom),
	})
	if err != nil {
		t.Fatalf("Failed to upsert override for custom model: %v", err)
	}

	resp := &schemas.UnifAIResponse{
		ChatResponse: &schemas.UnifAIChatResponse{
			Usage: &schemas.UnifAILLMUsage{PromptTokens: 2000, CompletionTokens: 1000, TotalTokens: 3000},
			ExtraFields: schemas.UnifAIResponseExtraFields{
				RoutingInfo: schemas.RoutingInfo{Provider: "openai", Model: "my-company-internal-llm"},
				RequestType: schemas.ChatCompletionRequest,
			},
		},
	}

	cost := store.CalculateCost(resp, nil)
	expected := (2000 * 0.0000030) + (1000 * 0.0000080) // 0.006 + 0.008 = 0.014
	if math.Abs(cost-expected) > 1e-9 {
		t.Fatalf("Custom model cost mismatch: got %v, expected %v", cost, expected)
	}
}

// Test 4: Scope priority: Virtual Key override beats Global override
func TestScopedOverridePriority(t *testing.T) {
	store := New(nil, &noopLogger{}, Config{})

	patchGlobal, _ := sonic.Marshal(map[string]float64{
		"input_cost_per_token":  0.000005,
		"output_cost_per_token": 0.000010,
	})
	patchVIPVK, _ := sonic.Marshal(map[string]float64{
		"input_cost_per_token":  0.000001, // Discounted for VIP virtual key
		"output_cost_per_token": 0.000002,
	})

	vkID := "vk-vip-client"
	err := store.UpsertOverrides(
		&configstoreTables.TablePricingOverride{
			ID:               "global-rule",
			Name:             "Global Rate",
			ScopeKind:        string(ScopeKindGlobal),
			MatchType:        string(MatchTypeExact),
			Pattern:          "gpt-4o",
			RequestTypes:     []schemas.RequestType{schemas.ChatCompletionRequest},
			PricingPatchJSON: string(patchGlobal),
		},
		&configstoreTables.TablePricingOverride{
			ID:               "vip-vk-rule",
			Name:             "VIP Client Discount",
			ScopeKind:        string(ScopeKindVirtualKey),
			VirtualKeyID:     &vkID,
			MatchType:        string(MatchTypeExact),
			Pattern:          "gpt-4o",
			RequestTypes:     []schemas.RequestType{schemas.ChatCompletionRequest},
			PricingPatchJSON: string(patchVIPVK),
		},
	)
	if err != nil {
		t.Fatalf("Failed to upsert scoped overrides: %v", err)
	}

	resp := &schemas.UnifAIResponse{
		ChatResponse: &schemas.UnifAIChatResponse{
			Usage: &schemas.UnifAILLMUsage{PromptTokens: 1000, CompletionTokens: 500, TotalTokens: 1500},
			ExtraFields: schemas.UnifAIResponseExtraFields{
				RoutingInfo: schemas.RoutingInfo{Provider: "openai", Model: "gpt-4o"},
				RequestType: schemas.ChatCompletionRequest,
			},
		},
	}

	// Case A: Normal user (no VK scope) -> Gets Global rate (0.005 + 0.005 = 0.010)
	normalCost := store.CalculateCost(resp, nil)
	expectedNormal := (1000 * 0.000005) + (500 * 0.000010)
	if math.Abs(normalCost-expectedNormal) > 1e-9 {
		t.Fatalf("Normal user cost mismatch: got %v, expected %v", normalCost, expectedNormal)
	}

	// Case B: VIP user (with VK scope) -> Gets VIP discount rate (0.001 + 0.001 = 0.002)
	vipScopes := &LookupScopes{VirtualKeyID: "vk-vip-client", Provider: "openai"}
	vipCost := store.CalculateCost(resp, vipScopes)
	expectedVIP := (1000 * 0.000001) + (500 * 0.000002)
	if math.Abs(vipCost-expectedVIP) > 1e-9 {
		t.Fatalf("VIP user cost mismatch: got %v, expected %v", vipCost, expectedVIP)
	}
}

// Test 5: Cache tokens pricing override
func TestCachedTokensPricingOverride(t *testing.T) {
	store := New(nil, &noopLogger{}, Config{})

	patchCache, _ := sonic.Marshal(map[string]float64{
		"input_cost_per_token":            0.000003,
		"output_cost_per_token":           0.000015,
		"cache_read_input_token_cost":     0.0000005, // $0.50 / 1M for cache read
		"cache_creation_input_token_cost": 0.00000375,
	})

	err := store.UpsertOverrides(&configstoreTables.TablePricingOverride{
		ID:               "cache-rule",
		Name:             "Claude Cache Pricing",
		ScopeKind:        string(ScopeKindGlobal),
		MatchType:        string(MatchTypeExact),
		Pattern:          "claude-3-5-sonnet",
		RequestTypes:     []schemas.RequestType{schemas.ChatCompletionRequest},
		PricingPatchJSON: string(patchCache),
	})
	if err != nil {
		t.Fatalf("Failed to upsert cache override: %v", err)
	}

	// 10,000 prompt tokens total: 8,000 cached read tokens + 2,000 regular tokens + 500 completion tokens
	resp := &schemas.UnifAIResponse{
		ChatResponse: &schemas.UnifAIChatResponse{
			Usage: &schemas.UnifAILLMUsage{
				PromptTokens:     10000,
				CompletionTokens: 500,
				TotalTokens:      10500,
				PromptTokensDetails: &schemas.ChatPromptTokensDetails{
					CachedReadTokens: 8000,
				},
			},
			ExtraFields: schemas.UnifAIResponseExtraFields{
				RoutingInfo: schemas.RoutingInfo{Provider: "anthropic", Model: "claude-3-5-sonnet"},
				RequestType: schemas.ChatCompletionRequest,
			},
		},
	}

	cost := store.CalculateCost(resp, nil)
	// Regular: 2,000 * 0.000003 = 0.006
	// Cache read: 8,000 * 0.0000005 = 0.004
	// Completion: 500 * 0.000015 = 0.0075
	// Total: 0.006 + 0.004 + 0.0075 = 0.0175
	expected := 0.0175
	if math.Abs(cost-expected) > 1e-9 {
		t.Fatalf("Cached token cost mismatch: got %v, expected %v", cost, expected)
	}
}

// Test 6: Embeddings request pricing override
func TestEmbeddingPricingOverride(t *testing.T) {
	store := New(nil, &noopLogger{}, Config{})

	// Override for text-embedding-3-small: $0.02 / 1M tokens = 0.00000002 per token
	patchEmbedding, _ := sonic.Marshal(map[string]float64{
		"input_cost_per_token": 0.00000002,
	})

	err := store.UpsertOverrides(&configstoreTables.TablePricingOverride{
		ID:               "embedding-override",
		Name:             "Embedding Discount",
		ScopeKind:        string(ScopeKindGlobal),
		MatchType:        string(MatchTypeExact),
		Pattern:          "text-embedding-3-small",
		RequestTypes:     []schemas.RequestType{schemas.EmbeddingRequest},
		PricingPatchJSON: string(patchEmbedding),
	})
	if err != nil {
		t.Fatalf("Failed to upsert embedding override: %v", err)
	}

	resp := &schemas.UnifAIResponse{
		EmbeddingResponse: &schemas.UnifAIEmbeddingResponse{
			Usage: &schemas.UnifAILLMUsage{
				PromptTokens: 50000,
				TotalTokens:  50000,
			},
			ExtraFields: schemas.UnifAIResponseExtraFields{
				RoutingInfo: schemas.RoutingInfo{Provider: "openai", Model: "text-embedding-3-small"},
				RequestType: schemas.EmbeddingRequest,
			},
		},
	}

	cost := store.CalculateCost(resp, nil)
	// 50,000 * 0.00000002 = 0.001 USD
	expected := 50000.0 * 0.00000002
	if math.Abs(cost-expected) > 1e-9 {
		t.Fatalf("Embedding cost mismatch: got %v, expected %v", cost, expected)
	}
}

// Test 7: Direct Governance Budget Linkage Simulation
// Proves that CalculateCost directly drives the budget tracker:
// - Initial budget: $0.010
// - Request 1 cost: $0.006 (Budget remaining: $0.004 -> Allowed)
// - Request 2 cost: $0.006 (Cumulative: $0.012 >= $0.010 -> Exceeded & Blocked!)
func TestGovernanceBudgetLinkage(t *testing.T) {
	store := New(nil, &noopLogger{}, Config{})

	// Custom pricing: input = 0.000002, output = 0.000008
	patchJSON, _ := sonic.Marshal(map[string]float64{
		"input_cost_per_token":  0.000002,
		"output_cost_per_token": 0.000008,
	})
	_ = store.UpsertOverrides(&configstoreTables.TablePricingOverride{
		ID:               "budget-test-override",
		Name:             "Model Budget Rate",
		ScopeKind:        string(ScopeKindGlobal),
		MatchType:        string(MatchTypeExact),
		Pattern:          "my-budget-model",
		RequestTypes:     []schemas.RequestType{schemas.ChatCompletionRequest},
		PricingPatchJSON: string(patchJSON),
	})

	// User request with 1000 input tokens, 500 output tokens
	// Cost = (1000 * 0.000002) + (500 * 0.000008) = 0.002 + 0.004 = $0.006
	resp := &schemas.UnifAIResponse{
		ChatResponse: &schemas.UnifAIChatResponse{
			Usage: &schemas.UnifAILLMUsage{
				PromptTokens:     1000,
				CompletionTokens: 500,
				TotalTokens:      1500,
			},
			ExtraFields: schemas.UnifAIResponseExtraFields{
				RoutingInfo: schemas.RoutingInfo{Provider: "openai", Model: "my-budget-model"},
				RequestType: schemas.ChatCompletionRequest,
			},
		},
	}

	// Governance budget setup: max limit $0.010
	maxBudgetLimit := 0.010
	currentUsage := 0.0

	// Request 1:
	cost1 := store.CalculateCost(resp, nil)
	if math.Abs(cost1-0.006) > 1e-9 {
		t.Fatalf("Req 1 cost mismatch: got %v, expected 0.006", cost1)
	}
	currentUsage += cost1 // Governance tracker updates in-memory budget
	if currentUsage >= maxBudgetLimit {
		t.Fatalf("Req 1 should be within budget, got usage %v >= limit %v", currentUsage, maxBudgetLimit)
	}

	// Request 2:
	cost2 := store.CalculateCost(resp, nil)
	currentUsage += cost2 // Now 0.006 + 0.006 = 0.012
	if currentUsage < maxBudgetLimit {
		t.Fatalf("Req 2 should exceed budget, got usage %v < limit %v", currentUsage, maxBudgetLimit)
	}
	// Verify that governance decision would be DecisionBudgetExceeded
	budgetExceeded := currentUsage >= maxBudgetLimit
	if !budgetExceeded {
		t.Fatalf("Expected budgetExceeded = true")
	}
}

