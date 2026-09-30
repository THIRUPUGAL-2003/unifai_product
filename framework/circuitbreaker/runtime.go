// Package circuitbreaker provides the in-process runtime for enterprise circuit
// breaker policies: trip on provider response header signals and fail over
// primary provider+model to a configured fallback while the circuit is open.
package circuitbreaker

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/unifai/unifai/core/schemas"
	configstoreTables "github.com/unifai/unifai/framework/configstore/tables"
)

// State is the live open/closed state for one policy.
type State struct {
	Status    string    `json:"status"`
	OpenedAt  time.Time `json:"opened_at"`
	Cooldown  int64     `json:"cooldown"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Signal describes one response-header trip condition.
type Signal struct {
	Source         string `json:"source"`
	HeaderName     string `json:"header_name"`
	HeaderValue    string `json:"header_value,omitempty"`
	HeaderContains string `json:"header_contains,omitempty"`
}

// Condition groups signals with an AND/OR operator.
type Condition struct {
	Operator string   `json:"operator"`
	Signals  []Signal `json:"signals"`
}

// Policy is the runtime view of a circuit breaker policy row.
type Policy struct {
	Name             string
	Enabled          bool
	PrimaryProvider  string
	PrimaryModel     string
	PrimaryKeyIDs    []string
	FallbackProvider string
	FallbackModel    string
	DefaultCooldown  string
	CooldownHeader   string
	Condition        Condition
}

// Runtime holds policy config and open-circuit state.
type Runtime struct {
	mu       sync.RWMutex
	policies []Policy
	circuits sync.Map // policy name -> State
}

// Default is the process-wide circuit breaker runtime shared by governance and HTTP APIs.
var Default = &Runtime{}

func policyFromRow(row configstoreTables.TableCircuitBreakerPolicy) Policy {
	p := Policy{
		Name:             row.Name,
		Enabled:          row.Enabled,
		PrimaryProvider:  row.PrimaryProvider,
		PrimaryModel:     row.PrimaryModel,
		PrimaryKeyIDs:    append([]string(nil), row.ParsedPrimaryKeyIDs...),
		FallbackProvider: row.FallbackProvider,
		FallbackModel:    row.FallbackModel,
		DefaultCooldown:  row.DefaultCooldown,
		CooldownHeader:   row.CooldownHeader,
	}
	if row.ParsedCondition != nil {
		raw, _ := json.Marshal(row.ParsedCondition)
		_ = json.Unmarshal(raw, &p.Condition)
	}
	if p.DefaultCooldown == "" {
		p.DefaultCooldown = "30s"
	}
	if p.Condition.Operator == "" {
		p.Condition.Operator = "OR"
	}
	return p
}

// LoadPolicies replaces the in-memory policy list (call after DB read or CRUD).
func (r *Runtime) LoadPolicies(rows []configstoreTables.TableCircuitBreakerPolicy) {
	loaded := make([]Policy, 0, len(rows))
	for _, row := range rows {
		loaded = append(loaded, policyFromRow(row))
	}
	r.mu.Lock()
	r.policies = loaded
	r.mu.Unlock()
}

// HasPolicies reports whether any policies are loaded.
func (r *Runtime) HasPolicies() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.policies) > 0
}

// Reset clears the open state for a policy.
func (r *Runtime) Reset(name string) {
	r.circuits.Delete(name)
}

// DeletePolicy removes policy state when a policy is deleted.
func (r *Runtime) DeletePolicy(name string) {
	r.Reset(name)
}

// ListStates returns non-expired open circuits.
func (r *Runtime) ListStates() map[string]State {
	now := time.Now()
	out := make(map[string]State)
	r.circuits.Range(func(key, value any) bool {
		state, ok := value.(State)
		if !ok {
			return true
		}
		if now.After(state.ExpiresAt) {
			r.circuits.Delete(key)
			return true
		}
		out[strings.TrimSpace(fmtKey(key))] = state
		return true
	})
	return out
}

func fmtKey(key any) string {
	if s, ok := key.(string); ok {
		return s
	}
	return ""
}

// FailoverDecision describes a pre-request failover swap.
type FailoverDecision struct {
	PolicyName string
	FromProv   string
	FromModel  string
	ToProv     string
	ToModel    string
}

// ApplyFailover swaps a primary endpoint to its fallback when the circuit is open.
func (r *Runtime) ApplyFailover(provider schemas.ModelProvider, model string) (*FailoverDecision, bool) {
	prov := strings.TrimSpace(string(provider))
	mod := strings.TrimSpace(model)
	if prov == "" || mod == "" {
		return nil, false
	}
	r.mu.RLock()
	policies := append([]Policy(nil), r.policies...)
	r.mu.RUnlock()

	now := time.Now()
	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}
		if !endpointMatches(policy.PrimaryProvider, policy.PrimaryModel, prov, mod) {
			continue
		}
		if !r.isOpen(policy.Name, now) {
			continue
		}
		return &FailoverDecision{
			PolicyName: policy.Name,
			FromProv:   prov,
			FromModel:  mod,
			ToProv:     strings.TrimSpace(policy.FallbackProvider),
			ToModel:    strings.TrimSpace(policy.FallbackModel),
		}, true
	}
	return nil, false
}

// EvaluateTrip opens the circuit when response headers match a policy signal for a primary call.
func (r *Runtime) EvaluateTrip(provider schemas.ModelProvider, model string, headers map[string]string) (string, bool) {
	prov := strings.TrimSpace(string(provider))
	mod := strings.TrimSpace(model)
	if prov == "" || mod == "" {
		return "", false
	}
	r.mu.RLock()
	policies := append([]Policy(nil), r.policies...)
	r.mu.RUnlock()

	normalized := normalizeHeaders(headers)
	now := time.Now()
	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}
		if !endpointMatches(policy.PrimaryProvider, policy.PrimaryModel, prov, mod) {
			continue
		}
		if r.isOpen(policy.Name, now) {
			continue
		}
		if !signalsMatch(policy.Condition, normalized) {
			continue
		}
		cooldown := parseCooldown(policy.DefaultCooldown, policy.CooldownHeader, normalized)
		r.trip(policy.Name, cooldown, now)
		return policy.Name, true
	}
	return "", false
}

func (r *Runtime) isOpen(name string, now time.Time) bool {
	raw, ok := r.circuits.Load(name)
	if !ok {
		return false
	}
	state, ok := raw.(State)
	if !ok {
		r.circuits.Delete(name)
		return false
	}
	if now.After(state.ExpiresAt) {
		r.circuits.Delete(name)
		return false
	}
	return state.Status == "open"
}

func (r *Runtime) trip(name string, cooldown time.Duration, now time.Time) {
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	r.circuits.Store(name, State{
		Status:    "open",
		OpenedAt:  now,
		Cooldown:  int64(cooldown.Seconds()),
		ExpiresAt: now.Add(cooldown),
	})
}

func endpointMatches(policyProvider, policyModel, reqProvider, reqModel string) bool {
	if !strings.EqualFold(strings.TrimSpace(policyProvider), strings.TrimSpace(reqProvider)) {
		return false
	}
	pm := strings.TrimSpace(policyModel)
	rm := strings.TrimSpace(reqModel)
	if pm == "" {
		return true
	}
	_, parsedPolicyModel := schemas.ParseModelString(pm, schemas.ModelProvider(policyProvider))
	_, parsedReqModel := schemas.ParseModelString(rm, schemas.ModelProvider(reqProvider))
	if parsedPolicyModel != "" && parsedReqModel != "" {
		return strings.EqualFold(parsedPolicyModel, parsedReqModel)
	}
	return strings.EqualFold(pm, rm)
}

func normalizeHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		out[strings.ToLower(strings.TrimSpace(k))] = v
	}
	// Providers (esp. Mistral via Kong) use *-req-minute / *-tokens-minute names while
	// the UI/docs default to OpenAI-style *-requests / *-tokens. Mirror aliases so a
	// policy written for one naming scheme still trips on the other.
	aliasPairs := [][2]string{
		{"x-ratelimit-remaining-requests", "x-ratelimit-remaining-req-minute"},
		{"x-ratelimit-remaining-tokens", "x-ratelimit-remaining-tokens-minute"},
		{"x-ratelimit-limit-requests", "x-ratelimit-limit-req-minute"},
		{"x-ratelimit-limit-tokens", "x-ratelimit-limit-tokens-minute"},
	}
	for _, pair := range aliasPairs {
		a, b := pair[0], pair[1]
		if _, ok := out[a]; !ok {
			if v, ok := out[b]; ok {
				out[a] = v
			}
		}
		if _, ok := out[b]; !ok {
			if v, ok := out[a]; ok {
				out[b] = v
			}
		}
	}
	return out
}

func headerValue(headers map[string]string, name string) (string, bool) {
	if val, ok := headers[name]; ok {
		return val, true
	}
	return "", false
}

func signalsMatch(cond Condition, headers map[string]string) bool {
	if len(cond.Signals) == 0 {
		return false
	}
	op := strings.ToUpper(strings.TrimSpace(cond.Operator))
	if op == "" {
		op = "OR"
	}
	matchOne := func(sig Signal) bool {
		name := strings.ToLower(strings.TrimSpace(sig.HeaderName))
		if name == "" {
			return false
		}
		val, ok := headerValue(headers, name)
		if !ok {
			return false
		}
		if hv := strings.TrimSpace(sig.HeaderValue); hv != "" {
			return strings.EqualFold(val, hv)
		}
		if hc := strings.TrimSpace(sig.HeaderContains); hc != "" {
			return strings.Contains(strings.ToLower(val), strings.ToLower(hc))
		}
		return strings.TrimSpace(val) != ""
	}
	if op == "AND" {
		for _, sig := range cond.Signals {
			if !matchOne(sig) {
				return false
			}
		}
		return true
	}
	for _, sig := range cond.Signals {
		if matchOne(sig) {
			return true
		}
	}
	return false
}

func parseCooldown(defaultCooldown, cooldownHeader string, headers map[string]string) time.Duration {
	if ch := strings.TrimSpace(cooldownHeader); ch != "" {
		if raw, ok := headers[strings.ToLower(ch)]; ok {
			if d, err := time.ParseDuration(strings.TrimSpace(raw)); err == nil && d > 0 {
				return d
			}
		}
	}
	if d, err := time.ParseDuration(strings.TrimSpace(defaultCooldown)); err == nil && d > 0 {
		return d
	}
	return 30 * time.Second
}
