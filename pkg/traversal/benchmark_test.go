package traversal_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/zqk-os/zqk/pkg/traversal"
)

func BenchmarkZPARQL_Parse(b *testing.B) {
	query := `
		MATCH (p:priority_plan {status: "in_progress"})-[:workstream_refs]->(w:workstream),
		      (b:backlog_item)-[:priority_plan_ref]->(p)
		WHERE b.status != "complete" AND b.priority_tier = "P1"
		RETURN p.title AS plan, b.id AS bli_id
		ORDER BY b.id ASC
		LIMIT 10 OFFSET 5;
	`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := traversal.ParseZPARQL(query)
		if err != nil {
			b.Fatalf("parse failed: %v", err)
		}
	}
}

func buildBenchmarkGraph(numNodes int) *traversal.GraphIndex {
	idx := traversal.NewGraphIndex()
	// Create plans, blis, and criteria
	numPlans := numNodes / 10
	if numPlans < 1 {
		numPlans = 1
	}

	for p := 0; p < numPlans; p++ {
		planID := fmt.Sprintf("PRI-%04d", p)
		idx.AddNode(planID, "priority_plan", map[string]any{
			"title":  fmt.Sprintf("Plan %d", p),
			"status": "in_progress",
		})
	}

	for i := 0; i < numNodes; i++ {
		bliID := fmt.Sprintf("BLI-%05d", i)
		planID := fmt.Sprintf("PRI-%04d", i%numPlans)
		idx.AddNode(bliID, "backlog_item", map[string]any{
			"title":             fmt.Sprintf("Backlog Item %d", i),
			"status":            "planned",
			"priority_tier":     "P1",
			"priority_plan_ref": planID,
		})
		idx.AddEdge(planID, "items", bliID)

		critID := fmt.Sprintf("CRIT-%05d", i)
		idx.AddNode(critID, "criteria", map[string]any{
			"title": fmt.Sprintf("Criterion %d", i),
		})
		idx.AddEdge(bliID, "criteria_refs", critID)
	}

	return idx
}

func BenchmarkZPARQL_MultiHopTraversal_100Nodes(b *testing.B) {
	idx := buildBenchmarkGraph(100)
	executor := traversal.NewQueryExecutor(idx)
	query, _ := traversal.ParseZPARQL(`
		MATCH (p:priority_plan)-[:items]->(b:backlog_item)-[:criteria_refs]->(c:criteria)
		WHERE b.priority_tier = "P1"
		RETURN p.title AS plan, b.id AS bli, c.title AS criterion
		LIMIT 20;
	`)

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := executor.Execute(ctx, query)
		if err != nil {
			b.Fatalf("traversal failed: %v", err)
		}
	}
}

func BenchmarkZPARQL_MultiHopTraversal_1000Nodes(b *testing.B) {
	idx := buildBenchmarkGraph(1000)
	executor := traversal.NewQueryExecutor(idx)
	query, _ := traversal.ParseZPARQL(`
		MATCH (p:priority_plan)-[:items]->(b:backlog_item)-[:criteria_refs]->(c:criteria)
		WHERE b.priority_tier = "P1"
		RETURN p.title AS plan, b.id AS bli, c.title AS criterion
		LIMIT 50;
	`)

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := executor.Execute(ctx, query)
		if err != nil {
			b.Fatalf("traversal failed: %v", err)
		}
	}
}

func BenchmarkZPARQL_MultiHopTraversal_10000Nodes(b *testing.B) {
	idx := buildBenchmarkGraph(10000)
	executor := traversal.NewQueryExecutor(idx)
	query, _ := traversal.ParseZPARQL(`
		MATCH (p:priority_plan {status: "in_progress"})-[:items]->(b:backlog_item)
		WHERE b.priority_tier = "P1"
		RETURN p.title AS plan, b.id AS bli
		LIMIT 100;
	`)

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := executor.Execute(ctx, query)
		if err != nil {
			b.Fatalf("traversal failed: %v", err)
		}
	}
}
