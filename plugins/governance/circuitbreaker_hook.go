package governance

import (
	"context"
	"strings"
	"sync"
	"time"

	raksha "github.com/raksha/raksha/core"
	"github.com/raksha/raksha/core/schemas"
	"github.com/raksha/raksha/framework/circuitbreaker"
	"github.com/raksha/raksha/framework/configstore"
	configstoreTables "github.com/raksha/raksha/framework/configstore/tables"
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

// vkAllowsEndpoint reports whether the virtual key may call provider/model, using the same
// blacklist and allowed-models rules as provider selection. A nil key allows everything.
func (p *GovernancePlugin) vkAllowsEndpoint(vk *configstoreTables.TableVirtualKey, provider, model string) bool {
	if vk == nil {
		return true
	}
	for _, cfg := range vk.ProviderConfigs {
		if !strings.EqualFold(strings.TrimSpace(cfg.Provider), strings.TrimSpace(provider)) {
			continue
		}
		if cfg.BlacklistedModels.IsBlocked(model) {
			return false
		}
		if p.modelCatalog != nil && p.inMemoryStore != nil {
			prov := schemas.ModelProvider(cfg.Provider)
			providerConfig, ok := p.inMemoryStore.GetConfiguredProviders()[prov]
			providerConfigPtr := &providerConfig
			if !ok {
				providerConfigPtr = nil
			}
			return p.modelCatalog.IsModelAllowedForProvider(prov, model, providerConfigPtr, cfg.AllowedModels)
		}
		return cfg.AllowedModels.IsAllowed(model)
	}
	return false
}

func (p *GovernancePlugin) applyCircuitBreakerFailover(ctx *schemas.RakshaContext, req *schemas.RakshaRequest, virtualKey *configstoreTables.TableVirtualKey) bool {
	p.syncCircuitBreakerPoliciesFromStore()
	provider, model, _ := req.GetRequestFields()
	if model == "" {
		return false
	}
	decision, ok := circuitbreaker.Default.ApplyFailover(provider, model)
	if !ok || decision == nil {
		return false
	}
	if !p.vkAllowsEndpoint(virtualKey, decision.ToProv, decision.ToModel) {
		ctx.AppendRoutingEngineLog(schemas.RoutingEngineCircuitBreaker, schemas.LogLevelWarn,
			"Circuit open for "+decision.FromProv+"/"+decision.FromModel+" but fallback "+decision.ToProv+"/"+decision.ToModel+
				" is not allowed for this virtual key — keeping the primary")
		return false
	}
	ctx.SetValue(schemas.RakshaContextKeyCircuitBreakerFailover,
		decision.PolicyName+"; "+decision.FromProv+"/"+decision.FromModel+" -> "+decision.ToProv+"/"+decision.ToModel)
	ctx.SetValue(circuitBreakerPrimaryProviderKey, decision.FromProv)
	ctx.SetValue(circuitBreakerPrimaryModelKey, decision.FromModel)
	req.SetProvider(schemas.ModelProvider(decision.ToProv))
	req.SetModel(decision.ToModel)
	schemas.AppendToContextList(ctx, schemas.RakshaContextKeyRoutingEnginesUsed, schemas.RoutingEngineCircuitBreaker)
	if p.logger != nil {
		p.logger.Info("[Governance] Circuit breaker failover policy=%s %s/%s → %s/%s",
			decision.PolicyName, decision.FromProv, decision.FromModel, decision.ToProv, decision.ToModel)
	}
	ctx.AppendRoutingEngineLog(schemas.RoutingEngineCircuitBreaker, schemas.LogLevelInfo,
		"Failover: "+decision.FromProv+"/"+decision.FromModel+" → "+decision.ToProv+"/"+decision.ToModel)
	return true
}

func (p *GovernancePlugin) evaluateCircuitBreakerTrip(ctx *schemas.RakshaContext, result *schemas.RakshaResponse, err *schemas.RakshaError) {
	requestType, provider, originalModel, resolvedModel := raksha.GetResponseFields(result, err)
	if raksha.IsStreamRequestType(requestType) && !raksha.IsFinalChunk(ctx) {
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
	keyID, _ := ctx.Value(schemas.RakshaContextKeySelectedKeyID).(string)
	if name, tripped := circuitbreaker.Default.EvaluateTrip(provider, model, keyID, headers); tripped {
		if p.logger != nil {
			p.logger.Info("[Governance] Circuit breaker tripped policy=%s provider=%s model=%s", name, provider, model)
		}
		ctx.AppendRoutingEngineLog(schemas.RoutingEngineCircuitBreaker, schemas.LogLevelWarn,
			"Tripped policy "+name+" — routing to fallback until cooldown expires")
	}
}

func collectProviderResponseHeaders(ctx *schemas.RakshaContext, result *schemas.RakshaResponse, err *schemas.RakshaError) map[string]string {
	if result != nil {
		if extra := result.GetExtraFields(); extra != nil && extra.ProviderResponseHeaders != nil {
			return extra.ProviderResponseHeaders
		}
	}
	if raw, ok := ctx.Value(schemas.RakshaContextKeyProviderResponseHeaders).(map[string]string); ok && len(raw) > 0 {
		return raw
	}
	return nil
}

// circuitBreakerPoliciesActive is a cheap check used by PreRequestHook gating.
func circuitBreakerPoliciesActive() bool {
	return circuitbreaker.Default.HasPolicies()
}
