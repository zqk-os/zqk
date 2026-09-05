package providers

import (
	"context"
	"fmt"
	"sort"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/hivemind"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// MemGraphMemoryStore implements hivemind.MemoryStore using MemGraph.
type MemGraphMemoryStore struct {
	graphConn provider.GraphConnection
}

// NewMemGraphMemoryStore creates a new MemGraph memory store.
func NewMemGraphMemoryStore(conn provider.GraphConnection) *MemGraphMemoryStore {
	return &MemGraphMemoryStore{
		graphConn: conn,
	}
}

// RetrieveSemantically performs K-Nearest Neighbors search in the vector index.
func (s *MemGraphMemoryStore) RetrieveSemantically(ctx context.Context, query string, k int) ([]hivemind.MemoryResult, error) {
	vq := provider.VectorQuery{
		Query: query,
		Limit: k,
	}
	res, err := s.graphConn.ExecuteVectorQuery(ctx, vq)
	if err != nil {
		return nil, fmt.Errorf(ConstVectorSearchFailedW, err)
	}

	var results []hivemind.MemoryResult
	for _, row := range res.Rows {
		// Mapping row data to MemoryResult
		id, _ := row[objects.FieldKeyID].(string)
		score, _ := row[objects.FieldKeyScore].(float32)
		results = append(results, hivemind.MemoryResult{
			ID:       id,
			Score:    score,
			Metadata: row,
		})
	}
	return results, nil
}

// FindObjectsMissingVectors finds objects that lack vector embeddings.
func (s *MemGraphMemoryStore) FindObjectsMissingVectors(ctx context.Context, batchSize int) ([]string, error) {
	// Stub implementation
	return nil, nil
}

// LinkVectorID links a generated vector ID back to the graph object.
func (s *MemGraphMemoryStore) LinkVectorID(ctx context.Context, objectID string, vectorID string) error {
	// Stub implementation
	return nil
}

// RetrieveGraphContext fetches structural neighbors for a given ID.
func (s *MemGraphMemoryStore) RetrieveGraphContext(ctx context.Context, id string, depth int) (*hivemind.GraphSubgraph, error) {
	traversal := provider.TraversalQuery{
		StartNodeID: id,
		MaxDepth:    depth,
	}
	res, err := s.graphConn.ExecuteTraversal(ctx, traversal)
	if err != nil {
		return nil, fmt.Errorf(ConstGraphTraversalFailedW, err)
	}

	// Mapping graph query results to GraphSubgraph
	nodes := make(map[string]any)
	for _, node := range res.Nodes {
		nodes[node.ID] = node.Properties
	}
	edges := make([]map[string]any, 0)
	for _, edge := range res.Edges {
		edges = append(edges, map[string]any{
			"from":               edge.FromID,
			"to":                 edge.ToID,
			objects.FieldKeyType: edge.Type,
			"properties":         edge.Properties,
		})
	}
	return &hivemind.GraphSubgraph{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// QueryHybrid performs a combined semantic-structural query (Hierarchical Retrieval).
// It retrieves semantic chunks and groups them by their parent structural objects (Macro-to-Micro flow).
func (s *MemGraphMemoryStore) QueryHybrid(ctx context.Context, query string, limit int, constraints hivemind.HybridConstraints) ([]hivemind.MemoryResult, error) {
	if limit <= 0 {
		limit = 10
	}
	depth := constraints.MaxDepth
	if depth <= 0 {
		depth = 1
	}

	// Refactored to first query SystemObject indices to bound the domain,
	// and explicitly constrain the secondary vector search to chunks belonging
	// only to those parent objects, eliminating Cartesian memory bloat.

	var queryStr string
	filterProps := make(map[string]any)

	if constraints.Kind != "" {
		queryStr = fmt.Sprintf(`
MATCH (macro:SystemObject {kind: $kind})
MATCH (c)-[:PART_OF|DEPENDS_ON*1..%d]->(macro)
WITH c, macro, vector.similarity(c.embedding, $vector) AS score
ORDER BY score DESC LIMIT %d
RETURN c.id AS chunk_id, macro.id AS parent_id, properties(macro) AS parent_props, score
`, depth, limit)
		filterProps[objects.FieldKeyKind] = constraints.Kind
	} else {
		queryStr = fmt.Sprintf(`
MATCH (macro:SystemObject)
MATCH (c)-[:PART_OF|DEPENDS_ON*1..%d]->(macro)
WITH c, macro, vector.similarity(c.embedding, $vector) AS score
ORDER BY score DESC LIMIT %d
RETURN c.id AS chunk_id, macro.id AS parent_id, properties(macro) AS parent_props, score
`, depth, limit)
	}

	// Pass the NLP query so a potential interceptor/embedding service could generate $vector
	filterProps["nlp_query"] = query

	vq := provider.VectorQuery{
		Query: queryStr,
		Limit: limit,
		Filter: provider.NodeFilter{
			Properties: filterProps,
		},
	}

	res, err := s.graphConn.ExecuteVectorQuery(ctx, vq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute hybrid query: %w", err)
	}

	if res == nil || len(res.Rows) == 0 {
		return nil, nil
	}

	macroMap := make(map[string]*hivemind.MemoryResult)

	for _, row := range res.Rows {
		chunkID, _ := row["chunk_id"].(string)
		parentID, ok := row["parent_id"].(string)
		if !ok || parentID == "" {
			parentID = chunkID
		}

		parentProps, _ := row["parent_props"].(map[string]any)
		var score float32
		if s, ok := row[objects.FieldKeyScore].(float64); ok {
			score = float32(s)
		} else if s, ok := row[objects.FieldKeyScore].(float32); ok {
			score = s
		} else {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Warn(fmt.Sprintf("Failed to parse score for chunk %s: %v", chunkID, row[objects.FieldKeyScore])).Log()
		}

		chunkMetadata := map[string]any{
			objects.FieldKeyID: chunkID,
		}

		macro, exists := macroMap[parentID]
		if !exists {
			macroMap[parentID] = &hivemind.MemoryResult{
				ID:    parentID,
				Score: score,
				Metadata: map[string]any{
					"properties": parentProps,
					"chunks":     []any{chunkMetadata},
				},
			}
		} else {
			if score > macro.Score {
				macro.Score = score
			}
			chunks := macro.Metadata["chunks"].([]any)
			macro.Metadata["chunks"] = append(chunks, chunkMetadata)
		}
	}

	var finalResults []hivemind.MemoryResult
	for _, macro := range macroMap {
		finalResults = append(finalResults, *macro)
	}

	sort.Slice(finalResults, func(i, j int) bool {
		return finalResults[i].Score > finalResults[j].Score
	})

	return finalResults, nil
}

// Upsert adds or updates the vector embedding for a given object ID.
func (s *MemGraphMemoryStore) Upsert(ctx context.Context, id string, vector []float32) error {
	// Simple Cypher to upsert the vector on the node
	// In MemGraph, assuming a node with label :ZQK_Object and an index on id
	q := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query:    `MATCH (n:ZQK_Object {id: $id}) SET n.embedding = $vector`,
		Params: map[string]any{
			objects.FieldKeyID: id,
			"vector":           vector,
		},
	}
	_, err := s.graphConn.ExecuteQuery(ctx, q)
	return err
}
