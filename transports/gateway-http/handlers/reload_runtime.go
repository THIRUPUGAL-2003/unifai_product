package handlers

import (
	"context"
	"encoding/json"

	"github.com/gateway/gateway/core/schemas"
	"github.com/gateway/gateway/framework/cluster"
	"github.com/gateway/gateway/framework/configstore"
	"github.com/gateway/gateway/framework/connectors"
	"github.com/gateway/gateway/framework/loadbalancer"
	"github.com/gateway/gateway/framework/mcptoolgroups"
	"github.com/gateway/gateway/transports/gateway-http/lib"
)

// ReloadEnterpriseRuntimeFromStore refreshes all workspace-backed runtimes.
func ReloadEnterpriseRuntimeFromStore(store configstore.WorkspaceStore, cfg *lib.Config) {
	if store == nil {
		return
	}
	ctx := context.Background()
	_ = connectors.ReloadFromStore(ctx, store)
	_ = mcptoolgroups.ReloadFromStore(ctx, store)
	_ = loadbalancer.ReloadFromStore(ctx, store)
	ReloadAuditSettingsFromStore(store)
	if cfg != nil && cfg.ConfigStore != nil {
		ReloadLoadBalancerProviderKeys(ctx, cfg.ConfigStore)
	}
	ApplyClusterRuntime(ctx, store, cfg)
}

// ReloadLoadBalancerProviderKeys snapshots provider keys for adaptive route selection.
func ReloadLoadBalancerProviderKeys(ctx context.Context, store configstore.ConfigStore) {
	if store == nil {
		return
	}
	providers, err := store.GetProviders(ctx)
	if err != nil {
		return
	}
	keys := make([]loadbalancer.ProviderKey, 0)
	for _, provider := range providers {
		providerKeys, err := store.GetProviderKeys(ctx, schemas.ModelProvider(provider.Name))
		if err != nil {
			continue
		}
		// Weight 0 means "drained" once any sibling key has a real weight; when no key
		// has one (weights never set), every key counts equally.
		anyWeighted := false
		for _, key := range providerKeys {
			if key.Weight > 0 {
				anyWeighted = true
				break
			}
		}
		for _, key := range providerKeys {
			enabled := key.Enabled == nil || *key.Enabled
			weight := key.Weight
			if weight <= 0 {
				if anyWeighted {
					continue
				}
				weight = 1
			}
			keys = append(keys, loadbalancer.ProviderKey{
				ID: key.ID, Provider: provider.Name, Weight: float64(weight), Enabled: enabled,
				Models: key.Models, BlacklistedModels: key.BlacklistedModels,
			})
		}
	}
	loadbalancer.Default.SetProviderKeys(keys)
}

// ApplyClusterRuntime wires cluster sync hooks from saved workspace config.
func ApplyClusterRuntime(ctx context.Context, store configstore.WorkspaceStore, cfg *lib.Config) {
	if store == nil || cfg == nil || cfg.KVStore == nil {
		return
	}
	row, err := store.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingCluster)
	if err != nil {
		cluster.Runtime.Configure(false, nil, cfg.KVStore)
		return
	}
	var payload struct {
		Enabled bool     `json:"enabled"`
		Peers   []string `json:"peers"`
	}
	if err := json.Unmarshal([]byte(row.Data), &payload); err != nil {
		cluster.Runtime.Configure(false, nil, cfg.KVStore)
		return
	}
	cluster.Runtime.Configure(payload.Enabled, payload.Peers, cfg.KVStore)
}

// ReloadConnectorsFromStore reloads connector runtime settings.
func ReloadConnectorsFromStore(store configstore.WorkspaceStore) {
	ReloadEnterpriseRuntimeFromStore(store, nil)
}
