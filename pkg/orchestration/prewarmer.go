package orchestration

import (
	"context"

	"github.com/zqk-os/zqk/pkg/graph/provider"
)

// IntentPrewarmer listens to the Intent Prediction engine and pre-warms the Neo4j/MemGraph cache
// with required objects before the user or agent explicitly requests them.
type IntentPrewarmer struct {
	pool provider.ConnectionPool
}

// NewIntentPrewarmer creates a new IntentPrewarmer.
func NewIntentPrewarmer(pool provider.ConnectionPool) *IntentPrewarmer {
	return &IntentPrewarmer{
		pool: pool,
	}
}

// Prewarm runs a background query based on the prediction to fetch related nodes into the database's page cache.
func (p *IntentPrewarmer) Prewarm(ctx context.Context, prediction IntentPrediction) error {
	// We want to touch relevant objects based on the prediction.
	// For instance, if TargetObject is "requirement" or "test_suite", we can run a Cypher query
	// to load those object definitions or related nodes into the graph DB cache.

	if prediction.TargetObject == "" {
		return nil // nothing to prewarm
	}

	// This Cypher query explicitly touches nodes of the expected kind.
	// This action forces the graph DB (Neo4j/MemGraph) to load these objects into its memory cache.
	query := `
		MATCH (n)
		WHERE n.kind = $targetObject
		OPTIONAL MATCH (n)-[r]-(m)
		RETURN n.id, count(m)
		LIMIT 50
	`
	params := map[string]interface{}{
		"targetObject": prediction.TargetObject,
	}

	return p.pool.Execute(ctx, func(conn provider.GraphConnection) error {
		_, err := conn.ExecuteQuery(ctx, provider.Query{
			Language: provider.QueryLanguageCypher,
			Query:    query,
			Params:   params,
		})
		return err
	})
}
