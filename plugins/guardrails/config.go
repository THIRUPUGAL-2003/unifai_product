package guardrails

// Config defines the root configuration for the Guardrails feature.
type Config struct {
	GuardrailRules     []GuardrailRule     `json:"guardrail_rules,omitempty"`
	GuardrailProviders []GuardrailProvider `json:"guardrail_providers,omitempty"`
}

// GuardrailRule defines a single rule with a CEL expression to evaluate.
type GuardrailRule struct {
	ID                int      `json:"id"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Enabled           bool     `json:"enabled"`
	CELExpression     string   `json:"cel_expression"`
	ApplyTo           string   `json:"apply_to"` // "input", "output", "both"
	SamplingRate      int      `json:"sampling_rate,omitempty"`
	Timeout           int      `json:"timeout,omitempty"`
	ProviderConfigIDs []int    `json:"provider_config_ids,omitempty"`
}

// GuardrailProvider defines the configuration for a guardrail provider (e.g., regex).
type GuardrailProvider struct {
	ID           int                    `json:"id"`
	ProviderName string                 `json:"provider_name"`
	PolicyName   string                 `json:"policy_name"`
	Enabled      bool                   `json:"enabled"`
	Timeout      int                    `json:"timeout,omitempty"`
	Config       map[string]interface{} `json:"config,omitempty"`
}
