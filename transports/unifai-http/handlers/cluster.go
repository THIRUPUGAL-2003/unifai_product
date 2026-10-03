package handlers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/cluster"
	"github.com/unifai/unifai/framework/configstore"
	"github.com/unifai/unifai/framework/loadbalancer"
	"github.com/valyala/fasthttp"
)

type clusterConfigPayload struct {
	Enabled bool     `json:"enabled"`
	Type    string   `json:"type"`
	Region  string   `json:"region"`
	Peers   []string `json:"peers"`
	Gossip  *struct {
		Port   int `json:"port"`
		Config *struct {
			TimeoutSeconds   int `json:"timeout_seconds"`
			SuccessThreshold int `json:"success_threshold"`
			FailureThreshold int `json:"failure_threshold"`
		} `json:"config"`
	} `json:"gossip,omitempty"`
	GRPC *struct {
		Port               int `json:"port"`
		DialTimeoutSeconds int `json:"dial_timeout_seconds"`
	} `json:"grpc,omitempty"`
}

// normalizeClusterPeer accepts host:port or an http(s):// gateway URL and returns the trimmed form.
func normalizeClusterPeer(peer string) (string, error) {
	peer = strings.TrimSpace(peer)
	if peer == "" {
		return "", fmt.Errorf("empty peer")
	}
	raw := peer
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		if strings.Contains(raw, "://") {
			return "", fmt.Errorf("peer %q: only http:// or https:// URLs are supported", peer)
		}
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("peer %q: use host:port or http(s)://host:port", peer)
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("peer %q: give only the gateway address, without a path", peer)
	}
	if u.Port() == "" && !strings.HasPrefix(peer, "http") {
		return "", fmt.Errorf("peer %q: port is required (host:port)", peer)
	}
	return strings.TrimRight(peer, "/"), nil
}

func (h *WorkspaceHandler) clusterWarnings(ctx *fasthttp.RequestCtx, cfg clusterConfigPayload) []string {
	warnings := []string{}
	if !cfg.Enabled {
		return warnings
	}
	if cluster.ClusterReplicateSecret() == "" {
		warnings = append(warnings, "CLUSTER_REPLICATE_SECRET is not set on this node, so nothing is replicated. Set the same value on every node and restart.")
	}
	if h.store == nil || h.store.KVStore == nil {
		warnings = append(warnings, "No KV store is running on this node, so there is no state to replicate.")
	}
	if len(cfg.Peers) == 0 {
		warnings = append(warnings, "Cluster mode is on but no peers are listed.")
	}
	gossipPort := "7946"
	if cfg.Gossip != nil && cfg.Gossip.Port > 0 {
		gossipPort = fmt.Sprint(cfg.Gossip.Port)
	}
	self := strings.ToLower(string(ctx.Host()))
	for _, peer := range cfg.Peers {
		hostPort := peer
		if u, err := url.Parse(peer); err == nil && u.Host != "" {
			hostPort = u.Host
		}
		if strings.EqualFold(hostPort, self) {
			warnings = append(warnings, fmt.Sprintf("Peer %q is this node; list only the other nodes.", peer))
		}
		if strings.HasSuffix(hostPort, ":"+gossipPort) {
			warnings = append(warnings, fmt.Sprintf("Peer %q uses the gossip port. Replication calls the gateway HTTP address (the port the dashboard/API listens on).", peer))
		}
	}
	return warnings
}

func (h *WorkspaceHandler) getClusterConfig(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	secretConfigured := cluster.ClusterReplicateSecret() != ""
	row, err := store.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingCluster)
	if isStoreNotFound(err) {
		SendJSON(ctx, map[string]any{
			"enabled": false, "type": "mesh", "region": "unknown", "peers": []string{},
			"node":                        map[string]any{"address": string(ctx.Host()), "mode": "standalone"},
			"replicate_secret_configured": secretConfigured,
			"warnings":                    []string{},
		})
		return
	}
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to load cluster config")
		return
	}
	var cfg clusterConfigPayload
	if err := json.Unmarshal([]byte(row.Data), &cfg); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to parse cluster config")
		return
	}
	mode := "standalone"
	if cfg.Enabled {
		mode = "cluster"
	}
	peers := cfg.Peers
	if peers == nil {
		peers = []string{}
	}
	cfg.Peers = peers
	SendJSON(ctx, map[string]any{
		"enabled": cfg.Enabled, "type": firstNonEmpty(cfg.Type, "mesh"),
		"region": firstNonEmpty(cfg.Region, "unknown"), "peers": peers,
		"gossip": cfg.Gossip, "grpc": cfg.GRPC,
		"node":                        map[string]any{"address": string(ctx.Host()), "mode": mode},
		"replicate_secret_configured": secretConfigured,
		"warnings":                    h.clusterWarnings(ctx, cfg),
	})
}

func (h *WorkspaceHandler) updateClusterConfig(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var payload clusterConfigPayload
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	if payload.Type == "" {
		payload.Type = "mesh"
	}
	if payload.Type != "mesh" && payload.Type != "broker" {
		SendError(ctx, fasthttp.StatusBadRequest, "type must be mesh or broker")
		return
	}
	peers := make([]string, 0, len(payload.Peers))
	seen := map[string]bool{}
	for _, peer := range payload.Peers {
		if strings.TrimSpace(peer) == "" {
			continue
		}
		normalized, err := normalizeClusterPeer(peer)
		if err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, err.Error())
			return
		}
		if seen[strings.ToLower(normalized)] {
			continue
		}
		seen[strings.ToLower(normalized)] = true
		peers = append(peers, normalized)
	}
	payload.Peers = peers
	if payload.Enabled && len(payload.Peers) == 0 {
		SendError(ctx, fasthttp.StatusBadRequest, "add at least one peer when cluster mode is enabled")
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save cluster config")
		return
	}
	if err := store.UpsertWorkspaceSetting(ctx, configstore.WorkspaceSettingCluster, string(raw)); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save cluster config")
		return
	}
	ApplyClusterRuntime(ctx, store, h.store)
	mode := "standalone"
	if payload.Enabled {
		mode = "cluster"
	}
	SendJSON(ctx, map[string]any{
		"enabled": payload.Enabled, "type": payload.Type, "region": firstNonEmpty(payload.Region, "unknown"),
		"peers": payload.Peers, "gossip": payload.Gossip, "grpc": payload.GRPC,
		"node":                        map[string]any{"address": string(ctx.Host()), "mode": mode},
		"replicate_secret_configured": cluster.ClusterReplicateSecret() != "",
		"warnings":                    h.clusterWarnings(ctx, payload),
	})
}

type loadBalancerConfigPayload struct {
	Enabled                   bool `json:"enabled"`
	DirectionSelectionEnabled bool `json:"direction_selection_enabled"`
	RouteSelectionEnabled     bool `json:"route_selection_enabled"`
	RerouteFailedDirections   bool `json:"reroute_failed_directions"`
	PruneFailedFallbacks      bool `json:"prune_failed_fallbacks"`
}

func defaultLoadBalancerConfig() loadBalancerConfigPayload {
	return loadBalancerConfigPayload{
		Enabled:                   false,
		DirectionSelectionEnabled: true,
		RouteSelectionEnabled:     true,
	}
}

func (h *WorkspaceHandler) loadBalancerConfig(ctx *fasthttp.RequestCtx) loadBalancerConfigPayload {
	if h.workspace == nil {
		return defaultLoadBalancerConfig()
	}
	row, err := h.workspace.GetWorkspaceSetting(ctx, configstore.WorkspaceSettingLoadBalancer)
	if err != nil {
		return defaultLoadBalancerConfig()
	}
	var cfg loadBalancerConfigPayload
	if err := json.Unmarshal([]byte(row.Data), &cfg); err != nil {
		return defaultLoadBalancerConfig()
	}
	return cfg
}

func (h *WorkspaceHandler) getLoadBalancerConfig(ctx *fasthttp.RequestCtx) {
	if h.requireStore(ctx) == nil {
		return
	}
	SendJSON(ctx, h.loadBalancerConfig(ctx))
}

func (h *WorkspaceHandler) updateLoadBalancerConfig(ctx *fasthttp.RequestCtx) {
	store := h.requireStore(ctx)
	if store == nil {
		return
	}
	var payload loadBalancerConfigPayload
	if err := json.Unmarshal(ctx.PostBody(), &payload); err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid request payload")
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save load balancer config")
		return
	}
	if err := store.UpsertWorkspaceSetting(ctx, configstore.WorkspaceSettingLoadBalancer, string(raw)); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to save load balancer config")
		return
	}
	_ = loadbalancer.ReloadFromStore(ctx, store)
	if h.store != nil && h.store.ConfigStore != nil {
		ReloadLoadBalancerProviderKeys(ctx, h.store.ConfigStore)
	}
	SendJSON(ctx, payload)
}

func (h *WorkspaceHandler) getLoadBalancerRoutes(ctx *fasthttp.RequestCtx) {
	if h.requireStore(ctx) == nil {
		return
	}
	cfg := h.loadBalancerConfig(ctx)
	directions := []map[string]any{}
	routes := []map[string]any{}
	if h.store != nil && h.store.ConfigStore != nil {
		providers, err := h.store.ConfigStore.GetProviders(ctx)
		if err == nil {
			for _, provider := range providers {
				keys, keyErr := h.store.ConfigStore.GetProviderKeys(ctx, schemas.ModelProvider(provider.Name))
				keyCount := 0
				if keyErr == nil {
					keyCount = len(keys)
					for _, key := range keys {
						enabled := key.Enabled == nil || *key.Enabled
						status := "healthy"
						if !enabled {
							status = "disabled"
						}
						weight := key.Weight
						if weight <= 0 {
							weight = 1
						}
						routes = append(routes, map[string]any{
							"provider": provider.Name, "key_id": key.ID, "key_name": key.Name,
							"weight": weight, "enabled": enabled, "status": status, "models": key.Models,
						})
					}
				}
				status := "not_configured"
				if keyCount > 0 {
					anyEnabled := false
					for _, key := range keys {
						if key.Enabled == nil || *key.Enabled {
							anyEnabled = true
							break
						}
					}
					if anyEnabled {
						status = "healthy"
					} else {
						status = "disabled"
					}
				}
				directions = append(directions, map[string]any{
					"provider": provider.Name, "key_count": keyCount, "status": status,
				})
			}
		}
	}
	SendJSON(ctx, map[string]any{"config": cfg, "directions": directions, "routes": routes})
}
