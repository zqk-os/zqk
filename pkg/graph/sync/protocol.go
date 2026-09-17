package sync

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SyncProtocol manages synchronization of graph backend state with real-time operations
// and the Continuous Alignment & Planning (CAP) loop.
type SyncProtocol interface {
	// Sync performs a synchronization run to align real-time events.
	Sync(ctx context.Context) error

	// RegisterOperation registers a real-time operation for synchronization.
	RegisterOperation(ctx context.Context, op SyncOperation) error

	// AcquireConsensusLock acquires a distributed lock for a resource to ensure consensus.
	AcquireConsensusLock(ctx context.Context, resourceID string, ttl time.Duration) (func() error, error)
}

// SyncOperation represents a state mutation that needs to be synchronized.
type SyncOperation struct {
	ID        string
	Type      string
	TargetID  string
	Payload   map[string]any
	Timestamp time.Time
}

// memorySyncProtocol provides a basic in-memory implementation of SyncProtocol.
type memorySyncProtocol struct {
	mu         sync.Mutex
	operations []SyncOperation
	locks      map[string]time.Time
}

// NewMemorySyncProtocol creates a new in-memory SyncProtocol.
func NewMemorySyncProtocol() SyncProtocol {
	return &memorySyncProtocol{
		operations: make([]SyncOperation, 0),
		locks:      make(map[string]time.Time),
	}
}

// Sync processes all registered operations.
func (p *memorySyncProtocol) Sync(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Process operations...
	// In a real implementation, this would apply operations to the graph DB.
	p.operations = nil // Clear after sync
	return nil
}

// RegisterOperation adds an operation to the sync queue.
func (p *memorySyncProtocol) RegisterOperation(ctx context.Context, op SyncOperation) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.operations = append(p.operations, op)
	return nil
}

// ErrLockAcquisitionFailed is returned when a consensus lock cannot be acquired.
var ErrLockAcquisitionFailed = fmt.Errorf("failed to acquire consensus lock")

// AcquireConsensusLock acquires a lock for consensus.
func (p *memorySyncProtocol) AcquireConsensusLock(ctx context.Context, resourceID string, ttl time.Duration) (func() error, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	if exp, exists := p.locks[resourceID]; exists && now.Before(exp) {
		return nil, ErrLockAcquisitionFailed
	}

	p.locks[resourceID] = now.Add(ttl)

	release := func() error {
		p.mu.Lock()
		defer p.mu.Unlock()
		delete(p.locks, resourceID)
		return nil
	}
	return release, nil
}
