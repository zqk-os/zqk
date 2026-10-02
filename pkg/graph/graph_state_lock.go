package graph

import (
	"context"
	"time"

	"github.com/zqk-os/zqk/pkg/graph/provider"
)

const (
	queryAcquireLock = `
		MERGE (l:Lock {id: $resourceID})
		ON CREATE SET l.owner = $ownerID, l.expiresAt = $expiresAt
		ON MATCH SET l.owner = CASE WHEN l.expiresAt < $now THEN $ownerID ELSE l.owner END,
					 l.expiresAt = CASE WHEN l.expiresAt < $now THEN $expiresAt ELSE l.expiresAt END
		WITH l
		RETURN l.owner = $ownerID AS acquired
	`
	queryReleaseLock = `
		MATCH (l:Lock {id: $resourceID, owner: $ownerID})
		DELETE l
	`
)

type graphStateLocker struct {
	pool    provider.ConnectionPool
	ownerID string
}

// NewGraphStateLocker creates a StateLocker backed by the graph database.
func NewGraphStateLocker(pool provider.ConnectionPool, ownerID string) StateLocker {
	return &graphStateLocker{
		pool:    pool,
		ownerID: ownerID,
	}
}

func (l *graphStateLocker) Lock(ctx context.Context, resourceID string, lockTTL time.Duration, waitTimeout time.Duration) (func() error, error) {
	deadline := time.Now().Add(waitTimeout)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		acquired, err := l.tryAcquire(ctx, resourceID, lockTTL)
		if err != nil {
			return nil, err
		}
		if acquired {
			// Return a release function
			return func() error {
				return l.release(ctx, resourceID)
			}, nil
		}

		if time.Now().After(deadline) {
			return nil, ErrLockTimeout
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			// Retry
		}
	}
}

func (l *graphStateLocker) tryAcquire(ctx context.Context, resourceID string, lockTTL time.Duration) (bool, error) {
	conn, err := l.pool.GetConnection(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = l.pool.ReturnConnection(conn) }()

	now := time.Now().UnixNano()
	expiresAt := time.Now().Add(lockTTL).UnixNano()

	query := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query:    queryAcquireLock,
		Params: map[string]any{
			"resourceID": resourceID,
			"ownerID":    l.ownerID,
			"now":        now,
			"expiresAt":  expiresAt,
		},
	}

	result, err := conn.ExecuteQuery(ctx, query)
	if err != nil {
		return false, err
	}

	return result.ExtractBooleanField("acquired")
}

func (l *graphStateLocker) release(ctx context.Context, resourceID string) error {
	conn, err := l.pool.GetConnection(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = l.pool.ReturnConnection(conn) }()

	query := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query:    queryReleaseLock,
		Params: map[string]any{
			"resourceID": resourceID,
			"ownerID":    l.ownerID,
		},
	}

	_, err = conn.ExecuteQuery(ctx, query)
	return err
}
