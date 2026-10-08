package azure

import (
	"context"
	"testing"
	"time"

	"github.com/gateway/gateway/core/schemas"
)

func TestAzureEntraScopes(t *testing.T) {
	// Test default scope
	defaultScopes := getAzureScopes(nil)
	if len(defaultScopes) != 1 || defaultScopes[0] != DefaultAzureScope {
		t.Fatalf("expected DefaultAzureScope, got %v", defaultScopes)
	}

	// Test custom scopes
	customScopes := getAzureScopes([]string{"https://management.azure.com/.default", "  "})
	if len(customScopes) != 1 || customScopes[0] != "https://management.azure.com/.default" {
		t.Fatalf("expected trimmed custom scope, got %v", customScopes)
	}
}

func TestAzureEntraProviderCredentialCaching(t *testing.T) {
	provider := &AzureProvider{}

	tenantID := "test-tenant-uuid"
	clientID := "test-client-uuid"
	clientSecret := "test-client-secret-value"

	cred1, err := provider.getOrCreateAuth(tenantID, clientID, clientSecret)
	if err != nil {
		t.Fatalf("expected successful credential creation, got %v", err)
	}

	cred2, err := provider.getOrCreateAuth(tenantID, clientID, clientSecret)
	if err != nil {
		t.Fatalf("expected successful credential retrieval, got %v", err)
	}

	// Verify caching - should return exact same instance
	if cred1 != cred2 {
		t.Fatalf("expected cached credential instance, got different instance")
	}
}

func TestAzureEntraAuthHeaders_APIKeyFallback(t *testing.T) {
	provider := &AzureProvider{}
	ctx := schemas.NewGatewayContext(context.Background(), time.Time{})

	key := schemas.Key{
		Value: *schemas.NewSecretVar("my-azure-api-key"),
	}

	headers, gErr := provider.getAzureAuthHeaders(ctx, key, false)
	if gErr != nil {
		t.Fatalf("unexpected error: %v", gErr)
	}
	if headers["api-key"] != "my-azure-api-key" {
		t.Fatalf("expected api-key header, got %v", headers)
	}

	// Test Anthropic model x-api-key header
	headersAnthropic, gErr := provider.getAzureAuthHeaders(ctx, key, true)
	if gErr != nil {
		t.Fatalf("unexpected error: %v", gErr)
	}
	if headersAnthropic["x-api-key"] != "my-azure-api-key" {
		t.Fatalf("expected x-api-key header, got %v", headersAnthropic)
	}
}
