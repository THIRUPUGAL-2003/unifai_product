package vectorstore

import (
	"context"
	"fmt"
	"math"
	"sync"
)

// MemoryStore is a process-local vector store. Direct cache lookups are
// point reads. Semantic lookups scan the namespace. Entries are lost on
// restart; connect Qdrant, Weaviate, Pinecone, or Redis to keep them.
type MemoryStore struct {
	mu         sync.RWMutex
	namespaces map[string]map[string]memoryEntry
}

type memoryEntry struct {
	embedding []float32
	metadata  map[string]interface{}
}

// NewMemoryStore returns an empty in-process store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{namespaces: map[string]map[string]memoryEntry{}}
}

func (m *MemoryStore) Ping(context.Context) error { return nil }

func (m *MemoryStore) CreateNamespace(_ context.Context, namespace string, _ int, _ map[string]VectorStoreProperties) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.namespaces[namespace] == nil {
		m.namespaces[namespace] = map[string]memoryEntry{}
	}
	return nil
}

func (m *MemoryStore) DeleteNamespace(_ context.Context, namespace string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.namespaces, namespace)
	return nil
}

func (m *MemoryStore) GetChunk(_ context.Context, namespace string, id string) (SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.namespaces[namespace][id]
	if !ok {
		return SearchResult{}, fmt.Errorf("not found")
	}
		return SearchResult{ID: id, Properties: memoryClone(entry.metadata)}, nil
}

func (m *MemoryStore) GetChunks(ctx context.Context, namespace string, ids []string) ([]SearchResult, error) {
	out := make([]SearchResult, 0, len(ids))
	for _, id := range ids {
		item, err := m.GetChunk(ctx, namespace, id)
		if err != nil {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func (m *MemoryStore) GetAll(_ context.Context, namespace string, queries []Query, _ []string, _ *string, limit int64) ([]SearchResult, *string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]SearchResult, 0)
	for id, entry := range m.namespaces[namespace] {
		if !memoryMatches(entry.metadata, queries) {
			continue
		}
		out = append(out, SearchResult{ID: id, Properties: memoryClone(entry.metadata)})
		if limit > 0 && int64(len(out)) >= limit {
			break
		}
	}
	return out, nil, nil
}

func (m *MemoryStore) GetNearest(_ context.Context, namespace string, vector []float32, queries []Query, _ []string, threshold float64, limit int64) ([]SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 {
		limit = 1
	}
	type scored struct {
		id    string
		score float64
		meta  map[string]interface{}
	}
	best := make([]scored, 0, limit)
	for id, entry := range m.namespaces[namespace] {
		if !memoryMatches(entry.metadata, queries) {
			continue
		}
		score := memoryCosine(vector, entry.embedding)
		if threshold > 0 && score < threshold {
			continue
		}
		item := scored{id: id, score: score, meta: entry.metadata}
		best = append(best, item)
		if int64(len(best)) > limit {
			worst := 0
			for i := 1; i < len(best); i++ {
				if best[i].score < best[worst].score {
					worst = i
				}
			}
			best = append(best[:worst], best[worst+1:]...)
		}
	}
	out := make([]SearchResult, 0, len(best))
	for _, item := range best {
		score := item.score
		out = append(out, SearchResult{ID: item.id, Score: &score, Properties: memoryClone(item.meta)})
	}
	return out, nil
}

func (m *MemoryStore) RequiresVectors() bool { return false }

func (m *MemoryStore) Add(_ context.Context, namespace string, id string, embedding []float32, metadata map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.namespaces[namespace] == nil {
		m.namespaces[namespace] = map[string]memoryEntry{}
	}
	copied := make([]float32, len(embedding))
	copy(copied, embedding)
	m.namespaces[namespace][id] = memoryEntry{embedding: copied, metadata: memoryClone(metadata)}
	return nil
}

func (m *MemoryStore) Delete(_ context.Context, namespace string, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ns := m.namespaces[namespace]; ns != nil {
		delete(ns, id)
	}
	return nil
}

func (m *MemoryStore) DeleteAll(_ context.Context, namespace string, queries []Query) ([]DeleteResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []DeleteResult
	for id, entry := range m.namespaces[namespace] {
		if !memoryMatches(entry.metadata, queries) {
			continue
		}
		delete(m.namespaces[namespace], id)
		out = append(out, DeleteResult{ID: id, Status: DeleteStatusSuccess})
	}
	return out, nil
}

func (m *MemoryStore) Close(context.Context, string) error { return nil }

func memoryMatches(metadata map[string]interface{}, queries []Query) bool {
	for _, q := range queries {
		if q.Operator != QueryOperatorEqual && q.Operator != "" {
			continue
		}
		got, ok := metadata[q.Field]
		if !ok || fmt.Sprint(got) != fmt.Sprint(q.Value) {
			return false
		}
	}
	return true
}

func memoryCosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		av, bv := float64(a[i]), float64(b[i])
		dot += av * bv
		na += av * av
		nb += bv * bv
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func memoryClone(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
