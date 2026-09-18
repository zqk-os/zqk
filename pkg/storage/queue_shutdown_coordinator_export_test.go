package storage

import (
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

// NewQueueShutdownCoordinatorForTest builds a coordinator for storage_test package tests.
// If cfg is nil, DefaultShutdownConfig is used.
func NewQueueShutdownCoordinatorForTest(cfg *ShutdownConfig, testOverride bool) *QueueShutdownCoordinator {
	if cfg == nil {
		cfg = DefaultShutdownConfig()
	}
	return &QueueShutdownCoordinator{
		queues:           make([]QueueShutdownHandler, 0),
		config:           cfg,
		shutdownComplete: make(chan struct{}),
		logger:           logging.NewEventLogger(pkgctx.NewSystemContext()),
		testOverride:     testOverride,
	}
}

// RegisteredQueueCountForTest returns the number of registered queues (for tests).
func (c *QueueShutdownCoordinator) RegisteredQueueCountForTest() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.queues)
}
