package cluster

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	replicateHeader       = "X-Cluster-Replicate"
	replicateSecretHeader = "X-Cluster-Replicate-Secret"
)

// ReplicateHeaderName returns the header used to identify peer replication requests.
func ReplicateHeaderName() string { return replicateHeader }

// ReplicateSecretHeaderName returns the shared-secret header peers must send.
func ReplicateSecretHeaderName() string { return replicateSecretHeader }

// ReplicationMessage is the HTTP payload for cross-node KV sync.
type ReplicationMessage struct {
	Op        string `json:"op"`
	Key       string `json:"key"`
	Value     []byte `json:"value,omitempty"`
	WrittenAt int64  `json:"written_at,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	DeletedAt int64  `json:"deleted_at,omitempty"`
}

var httpClient = &http.Client{Timeout: 5 * time.Second}

// ClusterReplicateSecret returns CLUSTER_REPLICATE_SECRET from the environment.
func ClusterReplicateSecret() string {
	return strings.TrimSpace(os.Getenv("CLUSTER_REPLICATE_SECRET"))
}

// VerifyReplicateSecret checks the peer shared secret (constant-time).
// Fail-closed: empty configured secret rejects all replication requests.
func VerifyReplicateSecret(provided string) bool {
	configured := ClusterReplicateSecret()
	if configured == "" {
		return false
	}
	a := sha256.Sum256([]byte(strings.TrimSpace(provided)))
	b := sha256.Sum256([]byte(configured))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

func (r *SyncDelegate) fanOut(op, key string, value []byte, writtenAt, expiresAt, deletedAt int64) {
	r.mu.Lock()
	enabled := r.enabled
	peers := append([]string(nil), r.peers...)
	r.mu.Unlock()
	if !enabled || len(peers) == 0 {
		return
	}
	secret := ClusterReplicateSecret()
	if secret == "" {
		// Do not fan out without a shared secret — peers would reject anyway.
		return
	}
	msg := ReplicationMessage{Op: op, Key: key, Value: value, WrittenAt: writtenAt, ExpiresAt: expiresAt, DeletedAt: deletedAt}
	body, err := json.Marshal(msg)
	if err != nil {
		return
	}
	var wg sync.WaitGroup
	for _, peer := range peers {
		peer = strings.TrimSpace(peer)
		if peer == "" {
			continue
		}
		wg.Add(1)
		go func(peer string) {
			defer wg.Done()
			url := peer
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
				url = "http://" + url
			}
			url = strings.TrimRight(url, "/") + "/internal/cluster/kv"
			req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(replicateHeader, "1")
			req.Header.Set(replicateSecretHeader, secret)
			resp, err := httpClient.Do(req)
			if err != nil {
				return
			}
			_ = resp.Body.Close()
		}(peer)
	}
	wg.Wait()
}

// ApplyReplication applies a remote replication message to the local KV store.
func ApplyReplication(store KVStore, msg ReplicationMessage) error {
	if store == nil {
		return fmt.Errorf("kv store is not configured")
	}
	switch msg.Op {
	case "set":
		return store.SetRemote(msg.Key, msg.Value, msg.WrittenAt, msg.ExpiresAt)
	case "delete":
		return store.DeleteRemote(msg.Key, msg.DeletedAt)
	default:
		return fmt.Errorf("unknown replication op %q", msg.Op)
	}
}

// KVStore is the subset of kvstore.Store used for replication.
type KVStore interface {
	SetRemote(key string, valueJSON []byte, writtenAt int64, expiresAt int64) error
	DeleteRemote(key string, deletedAt int64) error
}

// DecodeReplicationMessage parses a replication HTTP body.
func DecodeReplicationMessage(body []byte) (ReplicationMessage, error) {
	var msg ReplicationMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return ReplicationMessage{}, err
	}
	return msg, nil
}

// ReadReplicationBody reads and decodes a replication request body.
func ReadReplicationBody(r io.Reader) (ReplicationMessage, error) {
	if r == nil {
		return ReplicationMessage{}, fmt.Errorf("replication body reader is nil")
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		return ReplicationMessage{}, err
	}
	return DecodeReplicationMessage(raw)
}

// IsReplicationRequest reports whether the request originated from a peer node.
func IsReplicationRequest(headerValue string) bool {
	return strings.TrimSpace(headerValue) == "1"
}
