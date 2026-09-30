package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/unifai/unifai/core/schemas"
	"github.com/unifai/unifai/framework/cluster"
	"github.com/unifai/unifai/framework/configstore/tables"
	"github.com/unifai/unifai/framework/kvstore"
	"github.com/unifai/unifai/transports/unifai-http/lib"
	"github.com/valyala/fasthttp"
)

type clusterTestStore struct {
	*scimTestWorkspaceStore
	providers []tables.TableProvider
}

func newClusterTestStore() *clusterTestStore {
	return &clusterTestStore{
		scimTestWorkspaceStore: newSCIMTestStore(),
		providers:              make([]tables.TableProvider, 0),
	}
}

func (s *clusterTestStore) GetProviders(ctx context.Context) ([]tables.TableProvider, error) {
	return s.providers, nil
}

func (s *clusterTestStore) GetProviderKeys(ctx context.Context, provider schemas.ModelProvider) ([]schemas.Key, error) {
	return []schemas.Key{}, nil
}

func TestClusterConfigLifecycleAndReplication(t *testing.T) {
	testStore := newClusterTestStore()
	rawKV, err := kvstore.New(kvstore.Config{
		DefaultTTL: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("failed to init test kvstore: %v", err)
	}
	defer rawKV.Close()

	handler := &WorkspaceHandler{
		workspace: testStore,
		store: &lib.Config{
			ConfigStore: testStore,
			KVStore:     rawKV,
		},
	}

	// 1. Initial GET /api/cluster -> should return standalone default
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("GET")
		ctx.Request.SetRequestURI("/api/cluster")
		handler.getClusterConfig(ctx)

		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for initial getClusterConfig, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if resp["enabled"] != false {
			t.Errorf("Expected enabled=false initially, got %v", resp["enabled"])
		}
		if resp["type"] != "mesh" {
			t.Errorf("Expected default type mesh, got %v", resp["type"])
		}
		node := resp["node"].(map[string]any)
		if node["mode"] != "standalone" {
			t.Errorf("Expected mode standalone, got %v", node["mode"])
		}
	}

	// 2. PUT /api/cluster with invalid type -> should return 400 Bad Request
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("PUT")
		ctx.Request.SetRequestURI("/api/cluster")
		badPayload, _ := json.Marshal(map[string]any{
			"enabled": true,
			"type":    "invalid_cluster_type",
		})
		ctx.Request.SetBody(badPayload)
		handler.updateClusterConfig(ctx)

		if ctx.Response.StatusCode() != http.StatusBadRequest {
			t.Fatalf("Expected 400 for invalid cluster type, got %d", ctx.Response.StatusCode())
		}
	}

	// 3. PUT /api/cluster with valid config -> should succeed (200 OK)
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("PUT")
		ctx.Request.SetRequestURI("/api/cluster")
		validPayload, _ := json.Marshal(map[string]any{
			"enabled": true,
			"type":    "mesh",
			"region":  "us-east-1",
			"peers":   []string{"http://node-2:8080", "http://node-3:8080"},
		})
		ctx.Request.SetBody(validPayload)
		handler.updateClusterConfig(ctx)

		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for valid updateClusterConfig, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
	}

	// 4. GET /api/cluster after update -> should reflect cluster mode and peers
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("GET")
		ctx.Request.SetRequestURI("/api/cluster")
		handler.getClusterConfig(ctx)

		if ctx.Response.StatusCode() != http.StatusOK {
			t.Fatalf("Expected 200 for getClusterConfig, got %d", ctx.Response.StatusCode())
		}
		var resp map[string]any
		_ = json.Unmarshal(ctx.Response.Body(), &resp)
		if resp["enabled"] != true {
			t.Errorf("Expected enabled=true, got %v", resp["enabled"])
		}
		if resp["region"] != "us-east-1" {
			t.Errorf("Expected region us-east-1, got %v", resp["region"])
		}
		peers := resp["peers"].([]any)
		if len(peers) != 2 {
			t.Fatalf("Expected 2 peers, got %d", len(peers))
		}
		node := resp["node"].(map[string]any)
		if node["mode"] != "cluster" {
			t.Errorf("Expected mode cluster, got %v", node["mode"])
		}
	}

	// 5. Test Cluster KV Replication security (/internal/cluster/kv)
	const secret = "super-secret-cluster-token-1234567890"
	t.Setenv("CLUSTER_REPLICATE_SECRET", secret)

	// 5a. Missing replication header -> 403 Forbidden
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("POST")
		ctx.Request.SetRequestURI("/internal/cluster/kv")
		handler.clusterKVReplicate(ctx)
		if ctx.Response.StatusCode() != http.StatusForbidden {
			t.Fatalf("Expected 403 for missing replication header, got %d", ctx.Response.StatusCode())
		}
	}

	// 5b. Invalid secret -> 401 Unauthorized
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("POST")
		ctx.Request.SetRequestURI("/internal/cluster/kv")
		ctx.Request.Header.Set(cluster.ReplicateHeaderName(), "1")
		ctx.Request.Header.Set(cluster.ReplicateSecretHeaderName(), "wrong-secret")
		handler.clusterKVReplicate(ctx)
		if ctx.Response.StatusCode() != http.StatusUnauthorized {
			t.Fatalf("Expected 401 for wrong secret, got %d", ctx.Response.StatusCode())
		}
	}

	// 5c. Valid replication set -> 204 No Content
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("POST")
		ctx.Request.SetRequestURI("/internal/cluster/kv")
		ctx.Request.Header.Set(cluster.ReplicateHeaderName(), "1")
		ctx.Request.Header.Set(cluster.ReplicateSecretHeaderName(), secret)
		msg := cluster.ReplicationMessage{
			Op:        "set",
			Key:       "cluster:test:session",
			Value:     []byte(`{"user":"bob"}`),
			WrittenAt: time.Now().Unix(),
			ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
		}
		raw, _ := json.Marshal(msg)
		ctx.Request.SetBody(raw)
		handler.clusterKVReplicate(ctx)

		if ctx.Response.StatusCode() != http.StatusNoContent {
			t.Fatalf("Expected 204 for successful cluster KV replication, got %d body=%s", ctx.Response.StatusCode(), string(ctx.Response.Body()))
		}
	}

	// 5d. Valid replication delete -> 204 No Content
	{
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod("POST")
		ctx.Request.SetRequestURI("/internal/cluster/kv")
		ctx.Request.Header.Set(cluster.ReplicateHeaderName(), "1")
		ctx.Request.Header.Set(cluster.ReplicateSecretHeaderName(), secret)
		msg := cluster.ReplicationMessage{
			Op:        "delete",
			Key:       "cluster:test:session",
			DeletedAt: time.Now().Unix(),
		}
		raw, _ := json.Marshal(msg)
		ctx.Request.SetBody(raw)
		handler.clusterKVReplicate(ctx)

		if ctx.Response.StatusCode() != http.StatusNoContent {
			t.Fatalf("Expected 204 for successful cluster KV delete, got %d", ctx.Response.StatusCode())
		}
	}
}

func TestLoadBalancerRoutesConfig(t *testing.T) {
	testStore := newClusterTestStore()
	testStore.providers = append(testStore.providers, tables.TableProvider{
		Name:      "openai",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})

	handler := &WorkspaceHandler{
		workspace: testStore,
		store: &lib.Config{
			ConfigStore: testStore,
		},
	}

	// GET /api/load-balancer/routes
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.Header.SetMethod("GET")
	ctx.Request.SetRequestURI("/api/load-balancer/routes")
	handler.getLoadBalancerRoutes(ctx)

	if ctx.Response.StatusCode() != http.StatusOK {
		t.Fatalf("Expected 200 for getLoadBalancerRoutes, got %d", ctx.Response.StatusCode())
	}
	var resp map[string]any
	_ = json.Unmarshal(ctx.Response.Body(), &resp)
	if resp["config"] == nil || resp["directions"] == nil || resp["routes"] == nil {
		t.Fatalf("Expected config, directions, and routes in load balancer response")
	}
}
