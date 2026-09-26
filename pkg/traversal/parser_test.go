package traversal_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/traversal"
)

func TestParseZPARQL_ValidSimpleQuery(t *testing.T) {
	query := `MATCH (n:backlog_item) RETURN n.id AS id, n.title AS title;`
	ast, err := traversal.ParseZPARQL(query)
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.Equal(t, "1.0.0", ast.Version)
	require.Equal(t, "ZPARQLQuery", ast.Type)
	require.Len(t, ast.Patterns, 1)
	require.Len(t, ast.Patterns[0].Nodes, 1)
	require.Equal(t, "n", ast.Patterns[0].Nodes[0].Variable)
	require.Equal(t, "backlog_item", ast.Patterns[0].Nodes[0].Kind)
	require.Len(t, ast.Returns, 2)
	require.Equal(t, "id", ast.Returns[0].Alias)
	require.Equal(t, "title", ast.Returns[1].Alias)
}

func TestParseZPARQL_MultiHopAndClauses(t *testing.T) {
	query := `
		MATCH (p:priority_plan {status: "in_progress"})-[:workstream_refs]->(w:workstream),
		      (b:backlog_item)-[:priority_plan_ref]->(p)
		WHERE b.status != "complete" AND b.priority_tier = "P1"
		RETURN p.title AS plan, b.id AS bli_id
		ORDER BY b.id ASC
		LIMIT 10 OFFSET 5;
	`
	ast, err := traversal.ParseZPARQL(query)
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.Len(t, ast.Patterns, 2)

	// First pattern: (p)-[workstream_refs]->(w)
	require.Len(t, ast.Patterns[0].Nodes, 2)
	require.Equal(t, "p", ast.Patterns[0].Nodes[0].Variable)
	require.Equal(t, "priority_plan", ast.Patterns[0].Nodes[0].Kind)
	require.Equal(t, "in_progress", ast.Patterns[0].Nodes[0].Filters["status"])
	require.Len(t, ast.Patterns[0].Edges, 1)
	require.Equal(t, "workstream_refs", ast.Patterns[0].Edges[0].EdgeType)
	require.Equal(t, traversal.DirectionOutgoing, ast.Patterns[0].Edges[0].Direction)

	// Where clause
	require.NotNil(t, ast.Where)
	logical, ok := ast.Where.(traversal.LogicalExpr)
	require.True(t, ok)
	require.Equal(t, "AND", logical.Op)

	// Order by and Limit
	require.Len(t, ast.OrderBy, 1)
	require.Equal(t, "b.id", ast.OrderBy[0].Property)
	require.Equal(t, "ASC", ast.OrderBy[0].Direction)
	require.Equal(t, 10, ast.Limit)
	require.Equal(t, 5, ast.Offset)
}

func TestParseZPARQL_RangeQuantifier(t *testing.T) {
	query := `MATCH (b:backlog_item {id: "BLI-123"})-[:depends_on*1..4]->(dep:backlog_item) RETURN dep.id AS dep_id;`
	ast, err := traversal.ParseZPARQL(query)
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.Len(t, ast.Patterns[0].Edges, 1)
	require.Equal(t, 1, ast.Patterns[0].Edges[0].MinDepth)
	require.Equal(t, 4, ast.Patterns[0].Edges[0].MaxDepth)
}

func TestParseZPARQL_IncomingAndUndirectedEdges(t *testing.T) {
	queryIn := `MATCH (c:criteria)<-[:criteria_refs]-(b:backlog_item) RETURN c.id AS cid;`
	astIn, err := traversal.ParseZPARQL(queryIn)
	require.NoError(t, err)
	require.Equal(t, traversal.DirectionIncoming, astIn.Patterns[0].Edges[0].Direction)

	queryUndir := `MATCH (a:node)-[:rel]-(b:node) RETURN a.id, b.id;`
	astUndir, err := traversal.ParseZPARQL(queryUndir)
	require.NoError(t, err)
	require.Equal(t, traversal.DirectionUndirected, astUndir.Patterns[0].Edges[0].Direction)
}

func TestParseZPARQL_Aggregates(t *testing.T) {
	query := `MATCH (b:backlog_item) RETURN COUNT(*) AS total, COLLECT(b.id) AS ids;`
	ast, err := traversal.ParseZPARQL(query)
	require.NoError(t, err)
	require.Len(t, ast.Returns, 2)
	require.Equal(t, "COUNT", ast.Returns[0].Aggregate)
	require.Equal(t, "COLLECT", ast.Returns[1].Aggregate)
}

func TestParseZPARQL_NegativeInvariants(t *testing.T) {
	t.Run("Syntax Error - Missing MATCH", func(t *testing.T) {
		_, err := traversal.ParseZPARQL(`SELECT * FROM table;`)
		require.Error(t, err)
		require.Contains(t, err.Error(), traversal.ErrCodeZPARQLSyntaxError)
	})

	t.Run("Syntax Error - Unclosed Bracket", func(t *testing.T) {
		_, err := traversal.ParseZPARQL(`MATCH (n:backlog_item RETURN n.id;`)
		require.Error(t, err)
		require.Contains(t, err.Error(), traversal.ErrCodeZPARQLSyntaxError)
	})

	t.Run("Invalid Depth Range - Max Less Than Min", func(t *testing.T) {
		_, err := traversal.ParseZPARQL(`MATCH (a)-[:rel*3..1]->(b) RETURN b.id;`)
		require.Error(t, err)
		require.Contains(t, err.Error(), traversal.ErrCodeZPARQLInvalidDepthRange)
	})

	t.Run("Undefined Projection Variable", func(t *testing.T) {
		_, err := traversal.ParseZPARQL(`MATCH (a:node) RETURN ghost.id;`)
		require.Error(t, err)
		require.Contains(t, err.Error(), traversal.ErrCodeZPARQLUndefinedProjection)
	})
}

func TestExecuteZPARQL_EndToEnd(t *testing.T) {
	idx := traversal.NewGraphIndex()
	idx.AddNode("PRI-101", "priority_plan", map[string]any{"title": "Core V2", "status": "in_progress"})
	idx.AddNode("BLI-201", "backlog_item", map[string]any{"title": "Task A", "status": "planned", "priority_tier": "P1"})
	idx.AddNode("BLI-202", "backlog_item", map[string]any{"title": "Task B", "status": "complete", "priority_tier": "P2"})
	idx.AddNode("CRIT-301", "criteria", map[string]any{"title": "Coverage 90%"})

	idx.AddEdge("PRI-101", "items", "BLI-201")
	idx.AddEdge("PRI-101", "items", "BLI-202")
	idx.AddEdge("BLI-201", "criteria", "CRIT-301")

	executor := traversal.NewQueryExecutor(idx)

	// Query planned items under in_progress plan
	query := `
		MATCH (p:priority_plan)-[:items]->(b:backlog_item)
		WHERE b.status = "planned" AND b.priority_tier = "P1"
		RETURN p.title AS plan, b.id AS bli_id, b.title AS title;
	`
	ast, err := traversal.ParseZPARQL(query)
	require.NoError(t, err)

	res, err := executor.Execute(context.Background(), ast)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Rows, 1)
	require.Equal(t, "Core V2", res.Rows[0]["plan"])
	require.Equal(t, "BLI-201", res.Rows[0]["bli_id"])
	require.Equal(t, "Task A", res.Rows[0]["title"])
}
