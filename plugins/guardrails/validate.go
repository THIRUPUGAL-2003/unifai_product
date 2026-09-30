package guardrails

import (
	"fmt"

	"github.com/google/cel-go/cel"
)

// ValidateConfig checks that guardrail providers and rules are wired correctly.
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	plugin := &GuardrailsPlugin{
		config:      cfg,
		celPrograms: make(map[int]cel.Program),
		providers:   make(map[int]Provider),
	}

	if err := plugin.initializeProviders(); err != nil {
		return err
	}

	enabledProviders := make(map[int]struct{}, len(cfg.GuardrailProviders))
	for _, providerCfg := range cfg.GuardrailProviders {
		if providerCfg.Enabled {
			enabledProviders[providerCfg.ID] = struct{}{}
		}
	}

	for _, rule := range cfg.GuardrailRules {
		if !rule.Enabled {
			continue
		}
		if len(rule.ProviderConfigIDs) == 0 {
			name := rule.Name
			if name == "" {
				name = fmt.Sprintf("%d", rule.ID)
			}
			return fmt.Errorf("rule %q must link at least one provider", name)
		}
		for _, providerID := range rule.ProviderConfigIDs {
			if _, ok := enabledProviders[providerID]; !ok {
				name := rule.Name
				if name == "" {
					name = fmt.Sprintf("%d", rule.ID)
				}
				return fmt.Errorf("rule %q references unknown or disabled provider id %d", name, providerID)
			}
		}
	}

	return plugin.compileRules()
}
