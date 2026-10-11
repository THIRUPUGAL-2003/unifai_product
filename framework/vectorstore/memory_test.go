package vectorstore

import (
	"context"
	"testing"
)

func TestMemoryStoreDirectAndNearest(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	if err := store.Add(ctx, "cache", "id-1", []float32{1, 0}, map[string]interface{}{"cache_key": "tenant"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetChunk(ctx, "cache", "id-1")
	if err != nil || got.Properties["cache_key"] != "tenant" {
		t.Fatalf("get chunk: %+v %v", got, err)
	}
	nearest, err := store.GetNearest(ctx, "cache", []float32{1, 0}, []Query{{Field: "cache_key", Operator: QueryOperatorEqual, Value: "tenant"}}, nil, 0.9, 1)
	if err != nil || len(nearest) != 1 || nearest[0].ID != "id-1" {
		t.Fatalf("nearest: %+v %v", nearest, err)
	}
	miss, err := store.GetNearest(ctx, "cache", []float32{1, 0}, []Query{{Field: "cache_key", Operator: QueryOperatorEqual, Value: "other"}}, nil, 0.1, 1)
	if err != nil || len(miss) != 0 {
		t.Fatalf("filter miss: %+v %v", miss, err)
	}
}
