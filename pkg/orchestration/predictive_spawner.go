package orchestration

import (
	"context"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/graph/provider"
)

// PredictiveSpawner is a heuristic engine that consumes ambient events
// and proactively spawns corresponding background tasks.
type PredictiveSpawner struct {
	pool provider.ConnectionPool
}

// NewPredictiveSpawner creates a new PredictiveSpawner.
func NewPredictiveSpawner(pool provider.ConnectionPool) *PredictiveSpawner {
	return &PredictiveSpawner{
		pool: pool,
	}
}

// AmbientEvent represents an ambient system or user event.
type AmbientEvent struct {
	ID      string
	Type    string
	Actor   string
	Payload map[string]interface{}
}

// IngestEvent processes an ambient event and applies heuristic rules to spawn tasks.
func (p *PredictiveSpawner) IngestEvent(ctx context.Context, event AmbientEvent) error {
	// Heuristic 1: If a user creates a new Go file, generate a test stub and run code quality vet.
	if event.Type == "FILE_CREATED" {
		if fileStr, ok := event.Payload["filepath"].(string); ok && strings.HasSuffix(fileStr, ".go") && !strings.HasSuffix(fileStr, "_test.go") {
			return p.handleNewGoFile(ctx, fileStr)
		}
	}

	return nil
}

// handleNewGoFile is the proactive heuristic for Go files.
func (p *PredictiveSpawner) handleNewGoFile(ctx context.Context, filepath string) error {
	// In a real implementation, this would trigger an agent to generate `filepath_test.go`
	// and submit a code-quality vet job to the scheduler.

	// Example node creation in Graph
	query := `
		MERGE (t:Task {id: $taskId})
		SET t.type = 'GENERATE_TEST_STUB', t.target = $filepath, t.status = 'PENDING'
		RETURN t.id
	`
	params := map[string]interface{}{
		"taskId":   fmt.Sprintf("TASK-GENERATE-TEST-%s", filepath),
		"filepath": filepath,
	}

	err := p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		_, err := conn.ExecuteQuery(ctx, provider.Query{
			Language: provider.QueryLanguageCypher,
			Query:    query,
			Params:   params,
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to schedule test generation task: %w", err)
	}

	return nil
}
