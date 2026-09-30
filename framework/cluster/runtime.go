package cluster

import (
	"sync"

	"github.com/unifai/unifai/framework/kvstore"
)

// SyncDelegate bridges cluster config to the in-memory KV store.
type SyncDelegate struct {
	mu      sync.Mutex
	enabled bool
	peers   []string
	store   *kvstore.Store
}

// Runtime is the process-wide cluster sync runtime.
var Runtime = &SyncDelegate{}

// Configure enables or disables KV replication hooks and peer fan-out.
func (r *SyncDelegate) Configure(enabled bool, peers []string, store *kvstore.Store) {
	r.mu.Lock()
	r.enabled = enabled
	r.peers = append([]string(nil), peers...)
	r.store = store
	r.mu.Unlock()
	if store == nil {
		return
	}
	if enabled {
		store.SetDelegate(r)
		return
	}
	store.SetDelegate(nil)
}

// Apply is kept for backward compatibility.
func (r *SyncDelegate) Apply(enabled bool, store *kvstore.Store) {
	r.Configure(enabled, nil, store)
}

func (r *SyncDelegate) OnSet(key string, valueJSON []byte, writtenAt int64, expiresAt int64) {
	r.mu.Lock()
	enabled := r.enabled
	r.mu.Unlock()
	if !enabled {
		return
	}
	r.fanOut("set", key, valueJSON, writtenAt, expiresAt, 0)
}

func (r *SyncDelegate) OnDelete(key string, deletedAt int64) {
	r.mu.Lock()
	enabled := r.enabled
	r.mu.Unlock()
	if !enabled {
		return
	}
	r.fanOut("delete", key, nil, 0, 0, deletedAt)
}
