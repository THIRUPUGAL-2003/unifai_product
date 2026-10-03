// Package llmtests provides comprehensive test utilities and configurations for the Raksha system.
// It includes comprehensive test implementations covering all major AI provider scenarios,
// including text completion, chat, tool calling, image processing, and end-to-end workflows.
package llmtests

import (
	"context"
	"time"

	raksha "github.com/raksha/raksha/core"
	"github.com/raksha/raksha/core/schemas"
)

// Constants for test configuration
const (
	// TestTimeout defines the maximum duration for comprehensive tests
	// Set to 20 minutes to allow for complex multi-step operations
	TestTimeout = 20 * time.Minute
)

// getRaksha initializes and returns a Raksha instance for comprehensive testing.
// It sets up the comprehensive test account, plugin, and logger configuration.
//
// Environment variables are expected to be set by the system or test runner before calling this function.
// The account configuration will read API keys and settings from these environment variables.
//
// Returns:
//   - *raksha.Raksha: A configured Raksha instance ready for comprehensive testing
//   - error: Any error that occurred during Raksha initialization
//
// The function:
//  1. Creates a comprehensive test account instance
//  2. Configures Raksha with the account and default logger
func getRaksha(ctx context.Context) (*raksha.Raksha, error) {
	account := ComprehensiveTestAccount{}

	// Initialize Raksha
	b, err := raksha.Init(ctx, schemas.RakshaConfig{
		Account: &account,
		Logger:  raksha.NewDefaultLogger(schemas.LogLevelDebug),
	})
	if err != nil {
		return nil, err
	}

	return b, nil
}

// SetupTest initializes a test environment with timeout context
func SetupTest() (*raksha.Raksha, context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	client, err := getRaksha(ctx)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}

	return client, ctx, cancel, nil
}
