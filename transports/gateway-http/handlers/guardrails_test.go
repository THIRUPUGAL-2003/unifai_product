package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gateway/gateway/transports/gateway-http/lib"
	"github.com/valyala/fasthttp"
)

func TestGuardrailsHandlers_RulesAndProvidersWorkflow(t *testing.T) {
	testStore := newSCIMTestStore()
	cfg := &lib.Config{
		ConfigStore: testStore,
		GuardrailsConfig: &lib.GuardrailsConfig{
			GuardrailRules:     []lib.GuardrailRule{},
			GuardrailProviders: []lib.GuardrailProvider{},
		},
	}
	handler := NewGuardrailsHandler(nil, cfg)

	// 1. Initial GET /api/guardrails/config -> empty rules & providers
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("GET")
		ctx.Request.SetRequestURI("/api/guardrails/config")
		handler.getConfig(ctx)

		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for initial getConfig, got %d", ctx.Response.StatusCode())
		}
		var resp lib.GuardrailsConfig
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if len(resp.GuardrailRules) != 0 || len(resp.GuardrailProviders) != 0 {
			t.Fatalf("Expected 0 rules and 0 providers, got %d rules, %d providers", len(resp.GuardrailRules), len(resp.GuardrailProviders))
		}
	}

	// 2. PUT /api/guardrails/providers -> add a regex provider
	providerID := 1
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("PUT")
		ctx.Request.SetRequestURI("/api/guardrails/providers")
		body, _ := json.Marshal(map[string]any{
			"guardrail_providers": []map[string]any{
				{
					"id":            providerID,
					"provider_name": "regex",
					"policy_name":   "Credit Card Detector",
					"enabled":       true,
					"config": map[string]any{
						"patterns": []map[string]any{
							{"pattern": `\b\d{4}[ -]?\d{4}[ -]?\d{4}[ -]?\d{4}\b`, "description": "Credit Card Number", "flags": "i"},
						},
					},
				},
			},
		})
		ctx.Request.SetBody(body)
		handler.updateProviders(ctx)

		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for updateProviders, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
	}

	// 3. PUT /api/guardrails/rules with rule linking to unknown provider -> should fail validation (400)
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("PUT")
		ctx.Request.SetRequestURI("/api/guardrails/rules")
		badRuleBody, _ := json.Marshal(map[string]any{
			"guardrail_rules": []map[string]any{
				{
					"id":                  100,
					"name":                "Broken Rule",
					"cel_expression":      "true",
					"apply_to":            "input",
					"enabled":             true,
					"provider_config_ids": []int{9999}, // unknown provider
				},
			},
		})
		ctx.Request.SetBody(badRuleBody)
		handler.updateRules(ctx)

		if ctx.Response.StatusCode() != http.StatusBadRequest {
			t.Fatalf("Expected 400 for unknown provider in rule, got %d", ctx.Response.StatusCode())
		}
	}

	// 4. PUT /api/guardrails/rules linking to valid providerID -> should succeed (200 OK)
	ruleID := 10
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("PUT")
		ctx.Request.SetRequestURI("/api/guardrails/rules")
		validRuleBody, _ := json.Marshal(map[string]any{
			"guardrail_rules": []map[string]any{
				{
					"id":                  ruleID,
					"name":                "Payment Guard",
					"description":         "Scans user prompts for card details",
					"cel_expression":      "true",
					"apply_to":            "both",
					"enabled":             true,
					"provider_config_ids": []int{providerID},
				},
			},
		})
		ctx.Request.SetBody(validRuleBody)
		handler.updateRules(ctx)

		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for valid updateRules, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
	}

	// 5. GET /api/guardrails/config -> should return both provider and rule intact
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("GET")
		ctx.Request.SetRequestURI("/api/guardrails/config")
		handler.getConfig(ctx)

		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for getConfig, got %d", ctx.Response.StatusCode())
		}
		var resp lib.GuardrailsConfig
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if len(resp.GuardrailProviders) != 1 {
			t.Fatalf("Expected 1 provider, got %d", len(resp.GuardrailProviders))
		}
		if resp.GuardrailProviders[0].PolicyName != "Credit Card Detector" {
			t.Errorf("Expected policy name 'Credit Card Detector', got %q", resp.GuardrailProviders[0].PolicyName)
		}
		if len(resp.GuardrailRules) != 1 {
			t.Fatalf("Expected 1 rule, got %d", len(resp.GuardrailRules))
		}
		if resp.GuardrailRules[0].Name != "Payment Guard" {
			t.Errorf("Expected rule name 'Payment Guard', got %q", resp.GuardrailRules[0].Name)
		}
		if resp.GuardrailRules[0].ApplyTo != "both" {
			t.Errorf("Expected apply_to 'both', got %q", resp.GuardrailRules[0].ApplyTo)
		}
	}
}
