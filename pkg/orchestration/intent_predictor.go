package orchestration

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// IntentPrediction represents an anticipated action or object based on historical behavior.
type IntentPrediction struct {
	TargetAction string
	TargetObject string
	Confidence   float64
	Reasoning    string
}

// IntentPredictionHeuristicsEngine analyzes recent system events, messages, and CLI commands
// to proactively suggest or stage the next logical operations.
type IntentPredictionHeuristicsEngine struct {
	pool provider.ConnectionPool
}

// NewIntentPredictionHeuristicsEngine creates a new IntentPredictionHeuristicsEngine.
func NewIntentPredictionHeuristicsEngine(pool provider.ConnectionPool) *IntentPredictionHeuristicsEngine {
	return &IntentPredictionHeuristicsEngine{pool: pool}
}

// PredictFromRecentMessages queries the graph database for recent messages or commands and applies
// heuristics to generate predictions.
func (e *IntentPredictionHeuristicsEngine) PredictFromRecentMessages(ctx context.Context, since time.Time) ([]IntentPrediction, error) {
	var predictions []IntentPrediction

	query := `
		MATCH (m:Message)
		WHERE m.timestamp >= $since
		RETURN m.id AS id, m.destination AS destination, m.kind AS kind, properties(m) AS props
	`
	params := map[string]interface{}{
		"since": since.Unix(),
	}

	err := e.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		res, err := conn.ExecuteQuery(ctx, provider.Query{
			Language: provider.QueryLanguageCypher,
			Query:    query,
			Params:   params,
		})
		if err != nil {
			return err
		}

		for _, row := range res.Rows {
			kind, _ := row[objects.FieldKeyKind].(string)
			props, ok := row["props"].(map[string]interface{})
			if !ok {
				continue
			}

			// Heuristic 1: If user just created a "goal" via CLI, they will likely need to create a "requirement" next.
			if kind == "cli_command" {
				cmdPayload, _ := props["payload_command"].(string)
				if strings.Contains(paths.CLIInvocation(cmdPayload), "object create goal") {
					predictions = append(predictions, IntentPrediction{
						TargetAction: "create_requirement",
						TargetObject: "requirement",
						Confidence:   0.85,
						Reasoning:    "A requirement typically follows the creation of a new goal to define its constraints.",
					})
				}
				// Heuristic 2: If user runs a build command, they might want to view the resulting artifact or deploy it.
				if strings.Contains(cmdPayload, "make build") {
					predictions = append(predictions, IntentPrediction{
						TargetAction: "run_tests",
						TargetObject: "test_suite",
						Confidence:   0.70,
						Reasoning:    "Tests are typically run after a successful build.",
					})
				}
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to query recent messages for intent prediction: %w", err)
	}

	return predictions, nil
}
