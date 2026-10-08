package logstore

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gateway/gateway/core"
)

type mockRetentionManager struct {
	mu             sync.Mutex
	deletedLogs    int64
	deletedMCPLogs int64
	lastLogCutoff  time.Time
	lastMCPCutoff  time.Time
	batchCalls     int
}

func (m *mockRetentionManager) DeleteLogsBatch(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastLogCutoff = cutoff
	m.batchCalls++
	// Simulate deleting 5 logs on first batch, 0 on second
	if m.deletedLogs == 0 {
		m.deletedLogs = 5
		return 5, nil
	}
	return 0, nil
}

func (m *mockRetentionManager) DeleteMCPToolLogsBatch(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastMCPCutoff = cutoff
	if m.deletedMCPLogs == 0 {
		m.deletedMCPLogs = 3
		return 3, nil
	}
	return 0, nil
}

func TestLogsCleaner_DisabledWhenZero(t *testing.T) {
	mock := &mockRetentionManager{}
	cleaner := NewLogsCleaner(mock, CleanerConfig{RetentionDays: 0}, gateway.NewNoOpLogger())

	ctx := context.Background()
	cleaner.cleanupOldLogs(ctx)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.deletedLogs > 0 || mock.deletedMCPLogs > 0 {
		t.Fatalf("expected 0 deleted logs when retention is 0, got logs=%d, mcp=%d", mock.deletedLogs, mock.deletedMCPLogs)
	}
}

func TestLogsCleaner_DeletesOldLogsAndMCPLogs(t *testing.T) {
	mock := &mockRetentionManager{}
	cleaner := NewLogsCleaner(mock, CleanerConfig{RetentionDays: 7}, gateway.NewNoOpLogger())

	ctx := context.Background()
	cleaner.cleanupOldLogs(ctx)

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if mock.deletedLogs != 5 {
		t.Fatalf("expected 5 deleted logs, got %d", mock.deletedLogs)
	}
	if mock.deletedMCPLogs != 3 {
		t.Fatalf("expected 3 deleted mcp logs, got %d", mock.deletedMCPLogs)
	}

	// Verify cutoff is ~7 days ago
	expectedCutoff := time.Now().UTC().AddDate(0, 0, -7)
	diff := mock.lastLogCutoff.Sub(expectedCutoff)
	if diff < -time.Minute || diff > time.Minute {
		t.Fatalf("cutoff diff too large: got %v, expected ~%v", mock.lastLogCutoff, expectedCutoff)
	}
}

func TestLogsCleaner_UpdateRetentionDaysAndTrigger(t *testing.T) {
	mock := &mockRetentionManager{}
	cleaner := NewLogsCleaner(mock, CleanerConfig{RetentionDays: 0}, gateway.NewNoOpLogger())

	// Update to 30 days
	cleaner.UpdateRetentionDays(30)
	cleaner.mu.Lock()
	if cleaner.config.RetentionDays != 30 {
		cleaner.mu.Unlock()
		t.Fatalf("expected 30 days, got %d", cleaner.config.RetentionDays)
	}
	cleaner.mu.Unlock()

	// Verify triggerCh received signal
	select {
	case <-cleaner.triggerCh:
		// success
	default:
		t.Fatal("expected triggerCh to have a notification")
	}
}

func TestLogsCleaner_StartAndStopRoutine(t *testing.T) {
	mock := &mockRetentionManager{}
	cleaner := NewLogsCleaner(mock, CleanerConfig{RetentionDays: 14}, gateway.NewNoOpLogger())

	cleaner.StartCleanupRoutine()
	// Allow routine to initialize and run initial pass
	time.Sleep(50 * time.Millisecond)

	cleaner.StopCleanupRoutine()

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.deletedLogs != 5 {
		t.Fatalf("expected 5 deleted logs from initial pass, got %d", mock.deletedLogs)
	}
}

// simulateLog represents a log entry with created_at for in-memory testing
type simulateLog struct {
	id        string
	createdAt time.Time
}

type memoryStoreRetentionManager struct {
	mu      sync.Mutex
	logs    []simulateLog
	mcpLogs []simulateLog
}

func (s *memoryStoreRetentionManager) DeleteLogsBatch(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept []simulateLog
	deleted := int64(0)
	for _, l := range s.logs {
		if l.createdAt.Before(cutoff) && int(deleted) < batchSize {
			deleted++
		} else {
			kept = append(kept, l)
		}
	}
	s.logs = kept
	return deleted, nil
}

func (s *memoryStoreRetentionManager) DeleteMCPToolLogsBatch(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept []simulateLog
	deleted := int64(0)
	for _, l := range s.mcpLogs {
		if l.createdAt.Before(cutoff) && int(deleted) < batchSize {
			deleted++
		} else {
			kept = append(kept, l)
		}
	}
	s.mcpLogs = kept
	return deleted, nil
}

func TestLogsCleaner_RetentionStoreDeleteLoopSimulation(t *testing.T) {
	now := time.Now().UTC()
	store := &memoryStoreRetentionManager{
		logs: []simulateLog{
			{id: "log-old-10d", createdAt: now.AddDate(0, 0, -10)}, // Older than 7d -> will delete
			{id: "log-mid-2d", createdAt: now.AddDate(0, 0, -2)},   // Within 7d -> keep
		},
		mcpLogs: []simulateLog{
			{id: "mcp-old-8d", createdAt: now.AddDate(0, 0, -8)}, // Older than 7d -> will delete
			{id: "mcp-new-12h", createdAt: now.Add(-12 * time.Hour)}, // 12h old: within 7d and within 1d -> keep
		},
	}

	cleaner := NewLogsCleaner(store, CleanerConfig{RetentionDays: 7}, gateway.NewNoOpLogger())

	// Step 1: Run cleanup pass (7 days retention)
	cleaner.cleanupOldLogs(context.Background())

	store.mu.Lock()
	if len(store.logs) != 1 || store.logs[0].id != "log-mid-2d" {
		t.Fatalf("expected only 'log-mid-2d' to be retained, got: %+v", store.logs)
	}
	if len(store.mcpLogs) != 1 || store.mcpLogs[0].id != "mcp-new-12h" {
		t.Fatalf("expected only 'mcp-new-12h' to be retained, got: %+v", store.mcpLogs)
	}
	store.mu.Unlock()

	// Step 2: New logs stored into DB (simulating continuous traffic)
	store.mu.Lock()
	store.logs = append(store.logs, simulateLog{id: "log-today", createdAt: now})
	store.mcpLogs = append(store.mcpLogs, simulateLog{id: "mcp-today", createdAt: now})
	store.mu.Unlock()

	// Step 3: User changes retention to 1 day
	cleaner.UpdateRetentionDays(1)

	// Step 4: Run next cleanup pass (1 day retention)
	cleaner.cleanupOldLogs(context.Background())

	store.mu.Lock()
	// log-mid-2d was created 2 days ago -> older than 1d, so it must now be deleted!
	// log-today was created today -> kept!
	if len(store.logs) != 1 || store.logs[0].id != "log-today" {
		t.Fatalf("expected only 'log-today' to remain after 1d retention cleanup, got: %+v", store.logs)
	}
	// mcp-new-12h (12h old) and mcp-today (0h old) are both within 1 day -> both kept!
	if len(store.mcpLogs) != 2 {
		t.Fatalf("expected 2 mcp logs (mcp-new-12h, mcp-today) to remain after 1d retention cleanup, got: %+v", store.mcpLogs)
	}
	store.mu.Unlock()
}
