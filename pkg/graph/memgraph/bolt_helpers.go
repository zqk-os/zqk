package memgraph

import (
	"context"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/graph/provider"
)

const (
	queryLabelMaxLen      = 80
	queryLabelTruncateLen = 77
	queryLabelEllipsis    = "..."
	emptyValue            = ""
	errQueryExecFailed    = "query execution failed: %w"
	errCollectFailed      = "failed to collect result: %w"
	propertyIDKey         = "id"
)

// executeBoltQuery executes a Cypher query using Bolt protocol
func (c *memgraphConnection) executeBoltQuery(ctx context.Context, query string, params map[string]any) ([]*neo4j.Record, error) {
	if c.HasOpenTransaction() {
		tx := c.GetOpenTransaction()
		if memTx, ok := tx.(*memgraphTransaction); ok && memTx.explicitTx != nil {
			return c.executeBoltQueryInTransaction(ctx, memTx.explicitTx, query, params)
		}
	}

	session, err := c.getOrCreateSession(ctx)
	if err != nil {
		return nil, err
	}

	label := query
	if len(label) > queryLabelMaxLen {
		label = label[:queryLabelTruncateLen] + queryLabelEllipsis
	}

	start := time.Now()
	result, err := session.Run(ctx, query, params)
	if err != nil {
		provider.GetGlobalGraphProviderMetricsCollector().RecordQuery(label, time.Since(start), 0, err)
		return nil, errfmt.Errorf(errQueryExecFailed, err)
	}

	records, err := result.Collect(ctx)
	dur := time.Since(start)
	rows := 0
	if err == nil {
		rows = len(records)
	}
	provider.GetGlobalGraphProviderMetricsCollector().RecordQuery(label, dur, rows, err)
	if err != nil {
		return nil, errfmt.Errorf(errCollectFailed, err)
	}

	return records, nil
}

// executeBoltQueryInTransaction executes a query within a transaction
//
//nolint:unused // Helper function - reserved for future use
func (c *memgraphConnection) executeBoltQueryInTransaction(ctx context.Context, tx neo4j.ManagedTransaction, query string, params map[string]any) ([]*neo4j.Record, error) {
	result, err := tx.Run(ctx, query, params)
	if err != nil {
		return nil, errfmt.Errorf(errQueryExecFailed, err)
	}

	// Consume the result
	records, err := result.Collect(ctx)
	if err != nil {
		return nil, errfmt.Errorf(errCollectFailed, err)
	}

	return records, nil
}

// convertNeo4jNode converts a Neo4j node to provider.Node
func convertNeo4jNode(neo4jNode neo4j.Node) *provider.Node {
	node := &provider.Node{
		ID:         emptyValue,
		Labels:     neo4jNode.Labels,
		Properties: make(map[string]any),
	}

	// Extract properties
	for key, value := range neo4jNode.Props {
		if key == propertyIDKey {
			if idStr, ok := value.(string); ok {
				node.ID = idStr
			}
		} else {
			node.Properties[key] = value
		}
	}

	// If no ID in properties, try to use element ID
	if node.ID == emptyValue {
		node.ID = neo4jNode.ElementId
	}

	return node
}

// convertNeo4jRelationship converts a Neo4j relationship to provider.Edge
//
//nolint:gocritic // Neo4j driver provides Relationship by value; copy is expected
func convertNeo4jRelationship(rel neo4j.Relationship, fromID, toID string) *provider.Edge {
	edge := &provider.Edge{
		FromID:     fromID,
		ToID:       toID,
		Type:       rel.Type,
		Properties: make(map[string]any),
	}

	// Extract properties
	for key, value := range rel.Props {
		edge.Properties[key] = value
	}

	return edge
}
