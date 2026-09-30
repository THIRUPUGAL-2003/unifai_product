package cluster

import (
	"bytes"
	"encoding/json"
	"testing"
)

type mockKVStore struct {
	setCalls    int
	deleteCalls int
	lastKey     string
	lastVal     []byte
}

func (m *mockKVStore) SetRemote(key string, valueJSON []byte, writtenAt int64, expiresAt int64) error {
	m.setCalls++
	m.lastKey = key
	m.lastVal = valueJSON
	return nil
}

func (m *mockKVStore) DeleteRemote(key string, deletedAt int64) error {
	m.deleteCalls++
	m.lastKey = key
	return nil
}

func TestSyncReplication(t *testing.T) {
	mockStore := &mockKVStore{}

	// Test Set replication
	setMsg := ReplicationMessage{
		Op:        "set",
		Key:       "user:test:budget",
		Value:     []byte(`{"limit":100}`),
		WrittenAt: 123456,
		ExpiresAt: 234567,
	}

	if err := ApplyReplication(mockStore, setMsg); err != nil {
		t.Fatalf("unexpected error applying set replication: %v", err)
	}
	if mockStore.setCalls != 1 || mockStore.lastKey != "user:test:budget" {
		t.Fatalf("set replication failed to invoke store properly")
	}

	// Test Delete replication
	delMsg := ReplicationMessage{
		Op:        "delete",
		Key:       "user:test:budget",
		DeletedAt: 123456,
	}
	if err := ApplyReplication(mockStore, delMsg); err != nil {
		t.Fatalf("unexpected error applying delete replication: %v", err)
	}
	if mockStore.deleteCalls != 1 {
		t.Fatalf("delete replication failed to invoke store properly")
	}

	// Test ReadReplicationBody with nil
	if _, err := ReadReplicationBody(nil); err == nil {
		t.Fatalf("expected error on nil reader")
	}

	// Test DecodeReplicationMessage
	raw, _ := json.Marshal(setMsg)
	decoded, err := DecodeReplicationMessage(raw)
	if err != nil {
		t.Fatalf("failed to decode replication message: %v", err)
	}
	if decoded.Key != "user:test:budget" {
		t.Fatalf("decoded key mismatch: got %s, want user:test:budget", decoded.Key)
	}

	// Test ReadReplicationBody with valid bytes
	readMsg, err := ReadReplicationBody(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("failed to read replication body: %v", err)
	}
	if readMsg.Key != "user:test:budget" {
		t.Fatalf("readMsg key mismatch")
	}

	// Test IsReplicationRequest
	if !IsReplicationRequest("1") {
		t.Fatalf("expected true for replication header '1'")
	}
	if IsReplicationRequest("0") || IsReplicationRequest("") {
		t.Fatalf("expected false for invalid replication headers")
	}

	// Without CLUSTER_REPLICATE_SECRET configured, verification must fail closed.
	t.Setenv("CLUSTER_REPLICATE_SECRET", "")
	if VerifyReplicateSecret("anything") {
		t.Fatalf("expected VerifyReplicateSecret to fail when secret unset")
	}
	t.Setenv("CLUSTER_REPLICATE_SECRET", "peer-shared-secret-min-32-chars!!!!!!")
	if !VerifyReplicateSecret("peer-shared-secret-min-32-chars!!!!!!") {
		t.Fatalf("expected matching cluster replicate secret to verify")
	}
	if VerifyReplicateSecret("wrong-secret") {
		t.Fatalf("expected mismatched cluster replicate secret to fail")
	}
}
