// Package llmtests provides comprehensive test utilities and configurations for the UnifAI system.
// It includes comprehensive test implementations covering all major AI provider scenarios,
// including text completion, chat, tool calling, image processing, and end-to-end workflows.
package llmtests

import (
	"context"
	"time"

	unifai "github.com/unifai/unifai/core"
	"github.com/unifai/unifai/core/schemas"
)

// Constants for test configuration
const (
	// TestTimeout defines the maximum duration for comprehensive tests
	// Set to 20 minutes to allow for complex multi-step operations
	TestTimeout = 20 * time.Minute
)

// getUnifAI initializes and returns a UnifAI instance for comprehensive testing.
// It sets up the comprehensive test account, plugin, and logger configuration.
//
// Environment variables are expected to be set by the system or test runner before calling this function.
// The account configuration will read API keys and settings from these environment variables.
//
// Returns:
//   - *unifai.UnifAI: A configured UnifAI instance ready for comprehensive testing
//   - error: Any error that occurred during UnifAI initialization
//
// The function:
//  1. Creates a comprehensive test account instance
//  2. Configures UnifAI with the account and default logger
func getUnifAI(ctx context.Context) (*unifai.UnifAI, error) {
	account := ComprehensiveTestAccount{}

	// Initialize UnifAI
	b, err := unifai.Init(ctx, schemas.UnifAIConfig{
		Account: &account,
		Logger:  unifai.NewDefaultLogger(schemas.LogLevelDebug),
	})
	if err != nil {
		return nil, err
	}

	return b, nil
}

// SetupTest initializes a test environment with timeout context
func SetupTest() (*unifai.UnifAI, context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	client, err := getUnifAI(ctx)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}

	return client, ctx, cancel, nil
}
