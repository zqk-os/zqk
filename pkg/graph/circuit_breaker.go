package graph

import (
	"context"
	"math/rand"
	"time"

	"github.com/lanceman/zqk/pkg/circuitbreaker"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/mutation"
)

func isTransientDBError(err error) bool {
	// Simplify for now, just consider anything as transient to backoff.
	return true
}

// TransactionContext interface to mock neo4j.TransactionContext
type TransactionContext interface {
	Run(ctx context.Context, cypher string, params map[string]interface{}) (Result, error)
}

type Result interface {
	Single() (Record, error)
}

type Record interface {
	AsMap() map[string]interface{}
}

// GraphImpl is a placeholder for the actual GraphConnection that has session
type GraphImpl struct {
	Session Session
}

type Session interface {
	ExecuteWrite(ctx context.Context, work func(tx TransactionContext) (interface{}, error)) error
	ExecuteRead(ctx context.Context, work func(tx TransactionContext) (interface{}, error)) error
}

func (g *GraphImpl) HasCommittedMutation(ctx context.Context, ik string) (bool, error) {
	query := `MATCH (m:MutationAudit {idempotency_key: $ik}) RETURN count(m) > 0 AS exists;`

	var result map[string]interface{}
	err := g.Session.ExecuteRead(ctx, func(tx TransactionContext) (interface{}, error) {
		res, err := tx.Run(ctx, query, map[string]interface{}{"ik": ik})
		if err != nil {
			return nil, err
		}
		records, err := res.Single()
		if err != nil {
			return nil, err
		}
		result = records.AsMap()
		return result, nil
	})

	if err != nil {
		return false, err
	}
	if result == nil {
		return false, nil
	}
	return result["exists"].(bool), nil
}

func (g *GraphImpl) CommitWithResilience(ctx context.Context, ik string, mutations []mutation.Mutation, taskID string) error {
	breaker := circuitbreaker.NewCircuitBreaker()
	retryDelay := 100 * time.Millisecond
	maxRetries := 5

	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := breaker.AllowRequest(); err != nil {
			return errfmt.Errorf("commit rejected by circuit: %w", err)
		}

		err := g.Session.ExecuteWrite(ctx, func(tx TransactionContext) (interface{}, error) {
			// Execute Cypher transaction here...
			return nil, nil
		})

		if err == nil {
			breaker.RecordSuccess()
			return nil
		}

		if !isTransientDBError(err) {
			breaker.RecordFailure()
			return errfmt.Errorf("non-transient commit failed: %w", err)
		}

		breaker.RecordFailure()

		delay := retryDelay << uint(attempt)
		jitter := time.Duration(rand.Intn(int(delay/2) + 1))
		time.Sleep(delay + jitter)
	}

	return errfmt.Errorf("commit exhausted %d retries", maxRetries)
}
