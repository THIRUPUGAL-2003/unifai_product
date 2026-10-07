// Package llmtests provides comprehensive test utilities and configurations for the Gateway system.
// It includes comprehensive test implementations covering all major AI provider scenarios,
// including text completion, chat, tool calling, image processing, and end-to-end workflows.
package llmtests

import (
	"context"
	"time"

	gateway "github.com/gateway/gateway/core"
	"github.com/gateway/gateway/core/schemas"
)

// Constants for test configuration
const (
	// TestTimeout defines the maximum duration for comprehensive tests
	// Set to 20 minutes to allow for complex multi-step operations
	TestTimeout = 20 * time.Minute
)

// getGateway initializes and returns a Gateway instance for comprehensive testing.
// It sets up the comprehensive test account, plugin, and logger configuration.
//
// Environment variables are expected to be set by the system or test runner before calling this function.
// The account configuration will read API keys and settings from these environment variables.
//
// Returns:
//   - *gateway.Gateway: A configured Gateway instance ready for comprehensive testing
//   - error: Any error that occurred during Gateway initialization
//
// The function:
//  1. Creates a comprehensive test account instance
//  2. Configures Gateway with the account and default logger
func getGateway(ctx context.Context) (*gateway.Gateway, error) {
	account := ComprehensiveTestAccount{}

	// Initialize Gateway
	b, err := gateway.Init(ctx, schemas.GatewayConfig{
		Account: &account,
		Logger:  gateway.NewDefaultLogger(schemas.LogLevelDebug),
	})
	if err != nil {
		return nil, err
	}

	return b, nil
}

// SetupTest initializes a test environment with timeout context
func SetupTest() (*gateway.Gateway, context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	client, err := getGateway(ctx)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}

	return client, ctx, cancel, nil
}
