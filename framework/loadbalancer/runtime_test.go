package loadbalancer

import (
	"testing"

	"github.com/raksha/raksha/core/schemas"
)

func TestSelectProviderKeySkipsKeysThatCannotServeModel(t *testing.T) {
	r := &Runtime{cfg: Config{Enabled: true, RouteSelectionEnabled: true}}
	r.SetProviderKeys([]ProviderKey{
		{ID: "mini-only", Provider: "openai", Weight: 1, Enabled: true, Models: schemas.WhiteList{"gpt-4o-mini"}},
		{ID: "all-but-4o", Provider: "openai", Weight: 1, Enabled: true, Models: schemas.WhiteList{"*"}, BlacklistedModels: schemas.BlackList{"gpt-4o"}},
		{ID: "all", Provider: "openai", Weight: 1, Enabled: true, Models: schemas.WhiteList{"*"}},
	})
	for i := 0; i < 50; i++ {
		id, ok := r.SelectProviderKey("openai", "gpt-4o")
		if !ok || id != "all" {
			t.Fatalf("gpt-4o must only pin key 'all', got %q ok=%v", id, ok)
		}
	}
	if _, ok := r.SelectProviderKey("anthropic", "claude"); ok {
		t.Fatalf("no keys for provider should not pin")
	}
}
