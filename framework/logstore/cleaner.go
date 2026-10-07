package logstore

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/gateway/gateway/core/schemas"
)

const (
	cleanupInterval      = 24 * time.Hour
	minJitter            = 15 * time.Minute
	maxJitter            = 30 * time.Minute
	batchSize            = 100
	defaultRetentionDays = 365
)

// LogRetentionManager defines the interface for managing log retention and deletion
type LogRetentionManager interface {
	DeleteLogsBatch(ctx context.Context, cutoff time.Time, batchSize int) (deletedCount int64, err error)
}

// MCPToolLogRetentionManager is implemented by stores that also expire MCP tool logs.
type MCPToolLogRetentionManager interface {
	DeleteMCPToolLogsBatch(ctx context.Context, cutoff time.Time, batchSize int) (deletedCount int64, err error)
}

// CleanerConfig holds configuration for the log cleaner
type CleanerConfig struct {
	RetentionDays int
}

// LogsCleaner manages the cleanup of old logs
type LogsCleaner struct {
	manager     LogRetentionManager
	config      CleanerConfig
	logger      schemas.Logger
	stopCleanup chan struct{}
	mu          sync.Mutex
}

// NewLogsCleaner creates a new LogsCleaner instance
func NewLogsCleaner(manager LogRetentionManager, config CleanerConfig, logger schemas.Logger) *LogsCleaner {
	return &LogsCleaner{
		manager: manager,
		config:  config,
		logger:  logger,
	}
}

// StartCleanupRoutine starts a goroutine that periodically cleans up old logs
func (c *LogsCleaner) StartCleanupRoutine() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return early if already running
	if c.stopCleanup != nil {
		c.logger.Debug("log cleanup routine already running")
		return
	}

	c.stopCleanup = make(chan struct{})
	stopCh := c.stopCleanup

	go func() {
		// At the beginning, we will cleanup the logs
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		c.cleanupOldLogs(ctx)
		cancel()
		// Calculate initial delay with jitter
		timer := time.NewTimer(calculateNextRunDuration())
		defer timer.Stop()
		for {
			select {
			case <-timer.C:
				// Run cleanup
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
				c.cleanupOldLogs(ctx)
				cancel()

				// Reset timer with new jitter for next run
				timer.Reset(calculateNextRunDuration())

			case <-stopCh:
				c.logger.Info("log cleanup routine stopped")
				return
			}
		}
	}()
	c.logger.Info("log cleanup routine started")
}

// StopCleanupRoutine gracefully stops the cleanup goroutine
func (c *LogsCleaner) StopCleanupRoutine() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return early if already stopped
	if c.stopCleanup == nil {
		c.logger.Debug("log cleanup routine already stopped")
		return
	}

	close(c.stopCleanup)
	c.stopCleanup = nil
}

// cleanupOldLogs deletes logs older than the retention period in batches
func (c *LogsCleaner) cleanupOldLogs(ctx context.Context) {
	retentionDays := c.config.RetentionDays
	if retentionDays < 1 {
		retentionDays = defaultRetentionDays
	}

	// Calculate cutoff time
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	c.logger.Info("starting log cleanup: deleting logs older than %s (retention: %d days)", cutoff.Format(time.RFC3339), retentionDays)

	c.deleteInBatches(ctx, "logs", cutoff, c.manager.DeleteLogsBatch)
	if mcp, ok := c.manager.(MCPToolLogRetentionManager); ok {
		c.deleteInBatches(ctx, "MCP tool logs", cutoff, mcp.DeleteMCPToolLogsBatch)
	}
}

func (c *LogsCleaner) deleteInBatches(ctx context.Context, kind string, cutoff time.Time, deleteBatch func(context.Context, time.Time, int) (int64, error)) {
	totalDeleted := int64(0)
	batchCount := 0

	for {
		select {
		case <-ctx.Done():
			c.logger.Warn("%s cleanup cancelled: %v", kind, ctx.Err())
			return
		default:
		}

		deleted, err := deleteBatch(ctx, cutoff, batchSize)
		if err != nil {
			c.logger.Error("failed to delete old %s: %v", kind, err)
			return
		}
		if deleted == 0 {
			break
		}

		totalDeleted += deleted
		batchCount++
		c.logger.Debug("deleted %s batch %d: %d rows", kind, batchCount, deleted)

		if deleted < int64(batchSize) {
			break
		}
	}

	if totalDeleted > 0 {
		c.logger.Info("%s cleanup completed: deleted %d rows in %d batches", kind, totalDeleted, batchCount)
	} else {
		c.logger.Debug("%s cleanup completed: nothing to delete", kind)
	}
}

// calculateNextRunDuration returns 24 hours plus a random jitter between 15-30 minutes
func calculateNextRunDuration() time.Duration {
	jitter := minJitter + time.Duration(rand.Int63n(int64(maxJitter-minJitter)))
	return cleanupInterval + jitter
}
