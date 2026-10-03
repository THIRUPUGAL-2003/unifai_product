package governance

import (
	"context"
	"sync"
	"time"

	unifai "github.com/unifai/unifai/core"
	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/circuitbreaker"
	"github.com/unifai/unifai/framework/configstore"
)

type circuitBreakerCtxKey string

const (
	circuitBreakerPrimaryProviderKey circuitBreakerCtxKey = "governance-circuit-breaker-primary-provider"
	circuitBreakerPrimaryModelKey    circuitBreakerCtxKey = "governance-circuit-breaker-primary-model"
)

var (
	cbPolicySyncMu   sync.Mutex
	cbPolicySyncedAt time.Time
)

// syncCircuitBreakerPoliciesFromStore refreshes in-memory CB policies from the
// config store. Needed when CRUD landed on another process/replica, or when
// this process started before policies existed. Throttled to avoid a DB read
// on every request.
func (p *GovernancePlugin) syncCircuitBreakerPoliciesFromStore() {
	if p == nil || p.configStore == nil {
		return
	}
	ws, ok := configstore.AsWorkspaceStore(p.configStore)
	if !ok || ws == nil {
		return
	}
	// At most one refresh every 5s, including when no policies exist (otherwise every request
	// would hit the DB). The slot is claimed before querying so concurrent requests skip
	// instead of queueing behind the read.
	cbPolicySyncMu.Lock()
	if time.Since(cbPolicySyncedAt) < 5*time.Second {
		cbPolicySyncMu.Unlock()
		return
	}
	cbPolicySyncedAt = time.Now()
	cbPolicySyncMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rows, err := ws.ListCircuitBreakerPolicies(ctx)
	if err != nil {
		return
	}
	circuitbreaker.Default.LoadPolicies(rows)
}

func (p *GovernancePlugin) applyCircuitBreakerFailover(ctx *schemas.UnifAIContext, req *schemas.UnifAIRequest) bool {
	p.syncCircuitBreakerPoliciesFromStore()
	provider, model, _ := req.GetRequestFields()
	if model == "" {
		return false
	}
	decision, ok := circuitbreaker.Default.ApplyFailover(provider, model)
	if !ok || decision == nil {
		return false
	}
	ctx.SetValue(circuitBreakerPrimaryProviderKey, decision.FromProv)
	ctx.SetValue(circuitBreakerPrimaryModelKey, decision.FromModel)
	req.SetProvider(schemas.ModelProvider(decision.ToProv))
	req.SetModel(decision.ToModel)
	schemas.AppendToContextList(ctx, schemas.UnifAIContextKeyRoutingEnginesUsed, schemas.RoutingEngineCircuitBreaker)
	if p.logger != nil {
		p.logger.Info("[Governance] Circuit breaker failover policy=%s %s/%s → %s/%s",
			decision.PolicyName, decision.FromProv, decision.FromModel, decision.ToProv, decision.ToModel)
	}
	ctx.AppendRoutingEngineLog(schemas.RoutingEngineCircuitBreaker, schemas.LogLevelInfo,
		"Failover: "+decision.FromProv+"/"+decision.FromModel+" → "+decision.ToProv+"/"+decision.ToModel)
	return true
}

func (p *GovernancePlugin) evaluateCircuitBreakerTrip(ctx *schemas.UnifAIContext, result *schemas.UnifAIResponse, err *schemas.UnifAIError) {
	requestType, provider, originalModel, resolvedModel := unifai.GetResponseFields(result, err)
	if unifai.IsStreamRequestType(requestType) && !unifai.IsFinalChunk(ctx) {
		return
	}
	p.syncCircuitBreakerPoliciesFromStore()
	model := originalModel
	if model == "" {
		model = resolvedModel
	}
	if model == "" {
		return
	}
	// If we failed over on this request, the upstream primary was not called — do not trip.
	if prov, _ := ctx.Value(circuitBreakerPrimaryProviderKey).(string); prov != "" {
		return
	}
	headers := collectProviderResponseHeaders(ctx, result, err)
	if len(headers) == 0 {
		return
	}
	if name, tripped := circuitbreaker.Default.EvaluateTrip(provider, model, headers); tripped {
		if p.logger != nil {
			p.logger.Info("[Governance] Circuit breaker tripped policy=%s provider=%s model=%s", name, provider, model)
		}
		ctx.AppendRoutingEngineLog(schemas.RoutingEngineCircuitBreaker, schemas.LogLevelWarn,
			"Tripped policy "+name+" — routing to fallback until cooldown expires")
	}
}

func collectProviderResponseHeaders(ctx *schemas.UnifAIContext, result *schemas.UnifAIResponse, err *schemas.UnifAIError) map[string]string {
	if result != nil {
		if extra := result.GetExtraFields(); extra != nil && extra.ProviderResponseHeaders != nil {
			return extra.ProviderResponseHeaders
		}
	}
	if raw, ok := ctx.Value(schemas.UnifAIContextKeyProviderResponseHeaders).(map[string]string); ok && len(raw) > 0 {
		return raw
	}
	return nil
}

// circuitBreakerPoliciesActive is a cheap check used by PreRequestHook gating.
func circuitBreakerPoliciesActive() bool {
	return circuitbreaker.Default.HasPolicies()
}
