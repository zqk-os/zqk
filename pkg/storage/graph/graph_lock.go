package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/graph/provider"
)

// GraphLock provides distributed locking using the graph database
type GraphLock struct {
	pool       provider.ConnectionPool
	resourceID string
	ownerID    string
	locked     bool
}

// NewGraphLock creates a new graph-based lock for the given resource
func NewGraphLock(pool provider.ConnectionPool, resourceID, ownerID string) *GraphLock {
	return &GraphLock{
		pool:       pool,
		resourceID: resourceID,
		ownerID:    ownerID,
	}
}

// TryLock attempts to acquire an exclusive lock (non-blocking)
func (gl *GraphLock) TryLock(ctx context.Context) (bool, error) {
	conn, err := gl.pool.GetConnection(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = gl.pool.ReturnConnection(conn) }()

	query := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query: `
			MERGE (l:Lock {id: $resourceID})
			ON CREATE SET l.owner = $ownerID, l.timestamp = $timestamp
			WITH l
			RETURN l.owner = $ownerID AS acquired
		`,
		Params: map[string]any{
			"resourceID": gl.resourceID,
			"ownerID":    gl.ownerID,
			"timestamp":  time.Now().UnixNano(),
		},
	}

	result, err := conn.ExecuteQuery(ctx, query)
	if err != nil {
		return false, err
	}

	if len(result.Rows) == 0 {
		return false, fmt.Errorf("unexpected query result: no rows returned")
	}

	acquired, ok := result.Rows[0]["acquired"].(bool)
	if !ok {
		return false, fmt.Errorf("unexpected query result: acquired field is not a boolean")
	}

	if acquired {
		gl.locked = true
	}
	return acquired, nil
}

// Unlock releases the graph lock
func (gl *GraphLock) Unlock(ctx context.Context) error {
	if !gl.locked {
		return fmt.Errorf("lock not held")
	}

	conn, err := gl.pool.GetConnection(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = gl.pool.ReturnConnection(conn) }()

	query := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query: `
			MATCH (l:Lock {id: $resourceID, owner: $ownerID})
			DELETE l
		`,
		Params: map[string]any{
			"resourceID": gl.resourceID,
			"ownerID":    gl.ownerID,
		},
	}

	_, err = conn.ExecuteQuery(ctx, query)
	if err != nil {
		return err
	}

	gl.locked = false
	return nil
}

// LockWithTimeout attempts to acquire a lock with a timeout
func (gl *GraphLock) LockWithTimeout(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		acquired, err := gl.TryLock(ctx)
		if err != nil {
			return err
		}
		if acquired {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for graph lock")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// IsLocked returns whether the lock is currently held by this instance
func (gl *GraphLock) IsLocked() bool {
	return gl.locked
}
