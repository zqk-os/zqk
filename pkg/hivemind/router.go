package hivemind

import (
	"context"
	"fmt"
	"sort"

	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/objects"
)

// RouterConfig contains configuration for the dynamic query router.
type RouterConfig struct {
	OversampleFactor int // Factor by which to multiply the limit for over-sampling (broad constraints)
}

// DynamicQueryRouter determines the optimal execution path for hybrid queries.
type DynamicQueryRouter struct {
	graphConn provider.GraphConnection
	config    RouterConfig
}

// NewDynamicQueryRouter creates a new DynamicQueryRouter.
func NewDynamicQueryRouter(conn provider.GraphConnection, config RouterConfig) *DynamicQueryRouter {
	if config.OversampleFactor <= 0 {
		config.OversampleFactor = 5 // default oversample factor
	}
	return &DynamicQueryRouter{
		graphConn: conn,
		config:    config,
	}
}

// RouteHybridQuery routes the query based on constraints selectivity.
// It switches between exact brute-force over pre-filtered sets for high selectivity constraints,
// and over-sampled post-filtering for broad constraints.
func (r *DynamicQueryRouter) RouteHybridQuery(ctx context.Context, vector []float32, limit int, constraints HybridConstraints) ([]MemoryResult, error) {
	if limit <= 0 {
		limit = 10
	}

	var vq provider.VectorQuery

	// If a specific Kind is required, we consider this high selectivity.
	// For high selectivity, we use exact brute-force over the pre-filtered set.
	if constraints.Kind != "" {
		// Construct Cypher query for exact brute-force vector similarity on filtered nodes.
		// NOTE: In MemGraph, vector.similarity() in a WHERE/RETURN without CALL vector_search performs a brute-force calculation.
		queryStr := fmt.Sprintf(`MATCH (n)
WHERE n.%s = $kind
RETURN n, vector.similarity(n.embedding, $vector) AS similarity
ORDER BY similarity DESC LIMIT $limit`, objects.FieldKeyKind)

		vq = provider.VectorQuery{
			Query:  queryStr,
			Vector: vector,
			Limit:  limit,
			Filter: provider.NodeFilter{
				Properties: map[string]any{objects.FieldKeyKind: constraints.Kind},
			},
		}
	} else {
		// Broad constraint: Over-sampled post-filtering using the vector index.
		// provider.VectorQuery with an empty Query field allows the provider to use its optimal vector search index.
		vq = provider.VectorQuery{
			Vector: vector,
			Limit:  limit * r.config.OversampleFactor,
		}
	}

	res, err := r.graphConn.ExecuteVectorQuery(ctx, vq)
	if err != nil {
		return nil, fmt.Errorf("vector query execution failed: %w", err)
	}

	var results []MemoryResult
	for _, row := range res.Rows {
		id, _ := row[objects.FieldKeyID].(string)

		// Extract score. It might be returned as 'score' or 'similarity'
		var score float32
		if s, ok := row[objects.FieldKeyScore].(float64); ok {
			score = float32(s)
		} else if s, ok := row[objects.FieldKeyScore].(float32); ok {
			score = s
		} else if s, ok := row["similarity"].(float64); ok {
			score = float32(s)
		} else if s, ok := row["similarity"].(float32); ok {
			score = s
		}

		// Apply post-filtering if we used over-sampled index search
		if constraints.Kind != "" {
			nodeKind, _ := row[objects.FieldKeyKind].(string)
			if nodeKind == "" {
				// Sometimes properties are nested in "properties" map if the provider returns it that way
				if props, ok := row["properties"].(map[string]any); ok {
					nodeKind, _ = props[objects.FieldKeyKind].(string)
				}
			}
			if nodeKind != constraints.Kind {
				continue
			}
		}

		results = append(results, MemoryResult{
			ID:       id,
			Score:    score,
			Metadata: row,
		})
	}

	// Re-sort results descending (especially important after post-filtering)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Truncate to the original requested limit
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}
