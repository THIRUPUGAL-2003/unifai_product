package llmtests

import (
	"context"
	"os"
	"testing"

	raksha "github.com/raksha/raksha/core"
	"github.com/raksha/raksha/core/schemas"
)

// RunTextCompletionTest tests text completion functionality
func RunTextCompletionTest(t *testing.T, client *raksha.Raksha, ctx context.Context, testConfig ComprehensiveTestConfig) {
	if !testConfig.Scenarios.TextCompletion || testConfig.TextModel == "" {
		t.Logf("⏭️ Text completion not supported for provider %s", testConfig.Provider)
		return
	}

	t.Run("TextCompletion", func(t *testing.T) {
		if os.Getenv("SKIP_PARALLEL_TESTS") != "true" {
			t.Parallel()
		}

		prompt := "In fruits, A is for apple and B is for"
		request := &schemas.RakshaTextCompletionRequest{
			Provider: testConfig.Provider,
			Model:    testConfig.TextModel,
			Input: &schemas.TextCompletionInput{
				PromptStr: &prompt,
			},
			Params: &schemas.TextCompletionParameters{
				MaxTokens: raksha.Ptr(100),
			},
			Fallbacks: testConfig.TextCompletionFallbacks,
		}

		// Use retry framework with enhanced validation
		retryConfig := GetTestRetryConfigForScenario("TextCompletion", testConfig)
		retryContext := TestRetryContext{
			ScenarioName: "TextCompletion",
			ExpectedBehavior: map[string]interface{}{
				"should_continue_prompt": true,
				"should_be_coherent":     true,
			},
			TestMetadata: map[string]interface{}{
				"provider": testConfig.Provider,
				"model":    testConfig.TextModel,
				"prompt":   prompt,
			},
		}

		// Enhanced validation expectations
		expectations := GetExpectationsForScenario("TextCompletion", testConfig, map[string]interface{}{})
		expectations = ModifyExpectationsForProvider(expectations, testConfig.Provider)
		// Note: Removed strict keyword checks as LLMs are non-deterministic
		// Tests focus on functionality, not exact content

		// Create TextCompletion retry config
		textCompletionRetryConfig := TextCompletionRetryConfig{
			MaxAttempts: retryConfig.MaxAttempts,
			BaseDelay:   retryConfig.BaseDelay,
			MaxDelay:    retryConfig.MaxDelay,
			Conditions:  []TextCompletionRetryCondition{}, // Add specific text completion retry conditions as needed
			OnRetry:     retryConfig.OnRetry,
			OnFinalFail: retryConfig.OnFinalFail,
		}

		response, rakshaErr := WithTextCompletionTestRetry(t, textCompletionRetryConfig, retryContext, expectations, "TextCompletion", func() (*schemas.RakshaTextCompletionResponse, *schemas.RakshaError) {
			bfCtx := schemas.NewRakshaContext(ctx, schemas.NoDeadline)
			return client.TextCompletionRequest(bfCtx, request)
		})

		if rakshaErr != nil {
			t.Fatalf("❌ TextCompletion request failed after retries: %v", GetErrorMessage(rakshaErr))
		}

		content := GetTextCompletionContent(response)
		t.Logf("✅ Text completion result: %s", content)
	})
}
