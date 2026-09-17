# Resilience and Audit Streaming Architecture

## Overview
This document specifies the Circuit Breaker pattern for Neo4j/Memgraph partitions and the Event Bus / WAL tailer architecture for real-time audit streaming in ZQK.

*Note: All implementations strictly adhere to ZQK's `errfmt` and `logging.Fluent` policies.*

## 1. Circuit Breaker for Graph Commits

The graph commit layer wraps `neo4j.ExecuteWrite` with a circuit breaker and bounded exponential backoff to handle transient network partitions gracefully, preventing orchestrator panics.

### Go Implementation (`pkg/graph/circuit_breaker.go`)
```go
package graph

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mutation"
)

type CircuitBreaker struct {
	mu              sync.Mutex
	state           string // "closed", "open", "half-open"
	lastFailure     time.Time
	failureCount    int
	halfOpenAllowed bool
}

func (cb *CircuitBreaker) AllowRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	now := time.Now()
	switch cb.state {
	case "closed":
		return nil
	case "open":
		if now.Sub(cb.lastFailure) > 5*time.Second {
			cb.state = "half-open"
			cb.halfOpenAllowed = true
			return nil
		}
		return errfmt.Newf("circuit_open: db partition suspected")
	case "half-open":
		if !cb.halfOpenAllowed {
			return errfmt.Newf("circuit_half_open_probing")
		}
		return nil
	default:
		return errfmt.Newf("unknown_circuit_state")
	}
}

// ... RecordSuccess and RecordFailure methods ...

func (g *GraphImpl) CommitWithResilience(ctx context.Context, ik string, mutations []mutation.Mutation, taskID string) error {
	breaker := &CircuitBreaker{}
	retryDelay := 100 * time.Millisecond
	maxRetries := 5

	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := breaker.AllowRequest(); err != nil {
			return errfmt.Errorf("commit rejected by circuit").Wrap(err)
		}

		err := g.session.ExecuteWrite(ctx, func(tx neo4j.TransactionContext) (interface{}, error) {
			// Execute Cypher transaction here...
			return nil, nil
		})

		if err == nil {
			breaker.RecordSuccess()
			return nil
		}

		if !isTransientDBError(err) {
			breaker.RecordFailure()
			return errfmt.Errorf("non-transient commit failed").Wrap(err)
		}

		breaker.RecordFailure()
		
		delay := retryDelay << uint(attempt)
		jitter := time.Duration(rand.Intn(int(delay / 2)))
		time.Sleep(delay + jitter)
	}

	return errfmt.Errorf("commit exhausted %d retries", maxRetries)
}
```

## 2. Real-Time Audit Event Bus

Instead of blocking DB reads, ZQK uses an in-memory event bus backed by an FS WAL tailer to stream live mutations directly to the CLI (`zqk audit stream`).

### Core Event Bus (`pkg/audit/stream.go`)
```go
package audit

import (
	"context"
	"sync"
	"github.com/lanceman/zqk/pkg/logging"
)

type AuditStream struct {
	mu          sync.RWMutex
	subscribers map[chan AuditRecord]context.CancelFunc
}

func NewAuditStream() *AuditStream {
	return &AuditStream{ subscribers: make(map[chan AuditRecord]context.CancelFunc) }
}

func (a *AuditStream) Publish(ctx context.Context, record AuditRecord) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	
	for ch, cancel := range a.subscribers {
		select {
		case ch <- record:
		default:
			// Drop slow subscribers to prevent backpressure
			cancel()
			delete(a.subscribers, ch)
            logging.FluentEvent(logging.GetLogger()).Warn("Dropped slow audit subscriber").Log()
		}
	}
}
```

### Integration Notes
- Ensure all logging within the loop uses `logging.FluentEvent(logging.GetLogger())` per ZQK's `POL-CODE-007`.
- All errors returned from these modules must be strictly wrapped using `errfmt` to preserve traceability.
