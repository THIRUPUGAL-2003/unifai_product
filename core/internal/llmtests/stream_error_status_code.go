package llmtests

import (
	"context"
	"os"
	"testing"
	"time"

	unifai "github.com/unifai/unifai/core"
	"github.com/unifai/unifai/core/schemas"
)

// RunStreamErrorStatusCodeTest validates that pre-stream errors from providers carry
// the correct HTTP status code in UnifAIError.StatusCode. This is critical because
// the HTTP transport layer (sendStreamError) relies on this field to propagate the
// provider's actual status code to clients, rather than always returning 200 OK.
//
// The test sends a streaming request with a deliberately invalid model name.
// All providers (OpenAI, Anthropic, Bedrock) return 4xx status codes for such errors,
// and UnifAI must preserve those codes through the error chain.
func RunStreamErrorStatusCodeTest(t *testing.T, client *unifai.UnifAI, ctx context.Context, testConfig ComprehensiveTestConfig) {
	if !testConfig.Scenarios.CompletionStream {
		t.Logf("Completion stream not supported for provider %s, skipping stream error status code test", testConfig.Provider)
		return
	}

	// Skip providers that validate the model during key selection (deployment
	// mapping, or an allowlist as Fireworks builds its model list from config).
	// These reject invalid models BEFORE reaching the provider API, so no HTTP
	// request is made and there's no provider status code to propagate.
	deploymentBasedProviders := map[schemas.ModelProvider]bool{
		schemas.Azure:       true,
		schemas.Bedrock:     true,
		schemas.Vertex:      true,
		schemas.Replicate:   true,
		schemas.VLLM:        true,
		schemas.HuggingFace: true,
		schemas.Fireworks:   true,
	}
	if deploymentBasedProviders[testConfig.Provider] {
		t.Logf("Skipping StreamErrorStatusCode for %s (deployment-based key selection validates models before API call)", testConfig.Provider)
		return
	}

	t.Run("StreamErrorStatusCode", func(t *testing.T) {
		if os.Getenv("SKIP_PARALLEL_TESTS") != "true" {
			t.Parallel()
		}

		// Use a model name that is guaranteed to not exist across all providers.
		// This triggers a pre-stream validation error (400/404) rather than an in-stream error.
		invalidModel := "unifai-nonexistent-model-for-testing-12345"

		// Test with Chat Completion stream (most universally supported stream type)
		t.Run("ChatCompletionStream", func(t *testing.T) {
			messages := []schemas.ChatMessage{
				CreateBasicChatMessage("Hello"),
			}

			request := &schemas.UnifAIChatRequest{
				Provider: testConfig.Provider,
				Model:    invalidModel,
				Input:    messages,
				Params: &schemas.ChatParameters{
					MaxCompletionTokens: unifai.Ptr(10),
				},
			}

			bfCtx := schemas.NewUnifAIContext(ctx, schemas.NoDeadline)
			stream, unifaiErr := client.ChatCompletionStreamRequest(bfCtx, request)

			// We expect an error — the model doesn't exist
			if unifaiErr == nil {
				// If somehow no error, drain the stream and fail
				if stream != nil {
					for range stream {
					}
				}
				t.Fatal("❌ Expected error for invalid model in stream request, but got nil")
			}

			// Core assertion: the error must carry a provider HTTP status code
			if unifaiErr.StatusCode == nil {
				t.Fatalf("❌ UnifAIError.StatusCode is nil for provider %s — provider status code was not propagated. Error: %s",
					testConfig.Provider, GetErrorMessage(unifaiErr))
			}

			statusCode := *unifaiErr.StatusCode

			// The status code should be a 4xx client error (invalid model → 400, 404, or similar)
			if statusCode < 400 || statusCode >= 600 {
				t.Fatalf("❌ Expected 4xx/5xx status code for invalid model, got %d. Error: %s",
					statusCode, GetErrorMessage(unifaiErr))
			}

			// Should not be a UnifAI-generated error — it should come from the provider
			if unifaiErr.IsUnifAIError {
				// Some providers may have unifai-level validation that catches invalid models
				// before reaching the provider. Log but don't fail.
				t.Logf("⚠️  Error is a UnifAI error (not provider error) with status %d — this may indicate model validation happened before the provider call", statusCode)
			}

			t.Logf("✅ Stream error for invalid model returned status code %d (provider: %s)", statusCode, testConfig.Provider)
			t.Logf("   Error message: %s", GetErrorMessage(unifaiErr))
		})

		// Also test Responses stream if supported (Anthropic uses a different path)
		t.Run("ResponsesStream", func(t *testing.T) {
			messages := []schemas.ResponsesMessage{
				CreateBasicResponsesMessage("Hello"),
			}

			request := &schemas.UnifAIResponsesRequest{
				Provider: testConfig.Provider,
				Model:    invalidModel,
				Input:    messages,
				Params: &schemas.ResponsesParameters{
					MaxOutputTokens: unifai.Ptr(10),
				},
			}

			bfCtx := schemas.NewUnifAIContext(ctx, schemas.NoDeadline)
			stream, unifaiErr := client.ResponsesStreamRequest(bfCtx, request)

			if unifaiErr == nil {
				streamName := "responses stream"
				var timedOut bool
				unifaiErr, timedOut = waitForStreamError(t, stream, streamName)
				if timedOut {
					t.Fatalf("❌ Timed out waiting for invalid-model error on %s", streamName)
				}
				if unifaiErr == nil {
					t.Fatal("❌ Expected error for invalid model in responses stream request, but got nil")
				}
			}

			if unifaiErr.StatusCode == nil {
				t.Fatalf("❌ UnifAIError.StatusCode is nil for provider %s responses stream — provider status code was not propagated. Error: %s",
					testConfig.Provider, GetErrorMessage(unifaiErr))
			}

			statusCode := *unifaiErr.StatusCode

			if statusCode < 400 || statusCode >= 600 {
				t.Fatalf("❌ Expected 4xx/5xx status code for invalid model in responses stream, got %d. Error: %s",
					statusCode, GetErrorMessage(unifaiErr))
			}

			t.Logf("✅ Responses stream error for invalid model returned status code %d (provider: %s)", statusCode, testConfig.Provider)
		})
	})
}

func waitForStreamError(t *testing.T, stream chan *schemas.UnifAIStreamChunk, streamName string) (*schemas.UnifAIError, bool) {
	t.Helper()

	if stream == nil {
		return nil, false
	}

	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case chunk, ok := <-stream:
			if !ok {
				return nil, false
			}
			if chunk == nil {
				continue
			}
			if chunk.UnifAIError != nil {
				return chunk.UnifAIError, false
			}
		case <-timeout.C:
			t.Logf("⚠️ Timed out waiting for streamed error on %s", streamName)
			return nil, true
		}
	}
}
