package graph_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/semantic/graph"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Satisfies CRIT-ZPARQL-PLANNER-CONTRACT-SPEC:
// Logical and physical execution planner specification defining predicate pushdown,
// index seek/scan selection, join ordering, and topological cost estimation contracts.
func TestPlanner_ContractSpecification(t *testing.T) {
	// 1. Verify formal specification document exists
	specCandidates := []string{
		filepath.Join("..", "..", "..", "docs", "specs", "SPEC-ZPARQL-INDEXED-QUERY-PLANNER.md"),
		filepath.Join("docs", "specs", "SPEC-ZPARQL-INDEXED-QUERY-PLANNER.md"),
		filepath.Join("..", "..", "..", "docs", "specs", "SPEC-ZPARQL-QUERY-PLANNER.md"),
		filepath.Join("docs", "specs", "SPEC-ZPARQL-QUERY-PLANNER.md"),
	}
	var specContent string
	var found bool
	for _, p := range specCandidates {
		if data, err := fileutil.ReadFile(p); err == nil {
			specContent = string(data)
			found = true
			break
		}
	}
	require.True(t, found, "SPEC-ZPARQL-QUERY-PLANNER.md must exist in docs/specs/")
	require.Contains(t, specContent, "CRIT-ZPARQL-PLANNER-CONTRACT-SPEC")
	require.Contains(t, specContent, "CRIT-ZPARQL-INDEX-SCAN-COMPLEXITY-PROOF")
	require.Contains(t, specContent, "CRIT-ZPARQL-CYCLIC-TRAVERSAL-RECURSION-NEGATIVE")
	require.Contains(t, specContent, "Predicate Pushdown")
	require.Contains(t, specContent, "IndexSeek")

	// 2. Test Planner Contract with Predicate Pushdown and Index Seek
	planner := graph.NewQueryPlanner()
	ctx := context.Background()

	ast := &graph.QueryAST{
		StartNode: graph.NodePattern{
			Variable: "b",
			Kind:     "backlog_item",
			ID:       "BLI-001",
		},
		Steps: []graph.TraversalStep{
			{
				Edge: graph.EdgePattern{
					Relation:  "criteria_refs",
					Direction: "outgoing",
				},
				Node: graph.NodePattern{
					Variable: "c",
					Kind:     "criteria",
				},
			},
		},
		Where: []graph.Predicate{
			{Field: "b.status", Operator: "=", Value: "planned"},
			{Field: "c.category", Operator: "=", Value: "functional"},
		},
		ReturnVars: []string{"b", "c"},
	}

	plan, err := planner.Plan(ctx, ast)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Equal(t, "index_accelerated", plan.Strategy)
	require.True(t, plan.PushdownDone)
	require.Greater(t, plan.EstimatedCost, 0.0)

	// Verify Operator Tree
	root := plan.Root
	require.NotNil(t, root)
	require.Equal(t, graph.OpIndexSeek, root.Type)
	require.Equal(t, "BLI-001", root.TargetID)
	require.NotEmpty(t, root.Filter)
	require.Equal(t, "b.status", root.Filter[0].Field)

	step1 := root.Next
	require.NotNil(t, step1)
	require.Equal(t, graph.OpTraverseEdge, step1.Type)
	require.Equal(t, "criteria_refs", step1.Relation)
	require.NotEmpty(t, step1.Filter)
	require.Equal(t, "c.category", step1.Filter[0].Field)

	proj := step1.Next
	require.NotNil(t, proj)
	require.Equal(t, graph.OpProject, proj.Type)
}

// Satisfies CRIT-ZPARQL-INDEX-SCAN-COMPLEXITY-PROOF:
// Traversal engine achieves bound execution complexity proportional to matched subgraph volume O(K)
// using indexed edges rather than scanning full graph space O(N).
func TestPlanner_IndexScanComplexityProof(t *testing.T) {
	ctx := context.Background()
	storage := graph.NewIndexedGraphStorage()

	// 1. Populate large background graph (N = 10,000 nodes, 20,000 edges)
	totalBackgroundNodes := 10000
	for i := 1; i <= totalBackgroundNodes; i++ {
		nodeID := fmt.Sprintf("BG-NODE-%05d", i)
		storage.AddNode(nodeID, "dummy_kind", map[string]any{"seq": i})
		if i > 1 {
			parentID := fmt.Sprintf("BG-NODE-%05d", i-1)
			storage.AddEdge(parentID, "noise_edge", nodeID)
		}
	}
	require.Equal(t, totalBackgroundNodes, storage.TotalNodes())

	// 2. Create targeted matched subgraph (K = 4 nodes in a 3-hop path)
	// (BLI-100) -[criteria_refs]-> (CRIT-201) -[test_case_refs]-> (TST-301) -[req_refs]-> (REQ-401)
	storage.AddNode("BLI-100", "backlog_item", map[string]any{"status": "planned"})
	storage.AddNode("CRIT-201", "criteria", map[string]any{"status": "originated"})
	storage.AddNode("TST-301", "test_case", map[string]any{"status": "active"})
	storage.AddNode("REQ-401", "requirement", map[string]any{"status": "complete"})

	storage.AddEdge("BLI-100", "criteria_refs", "CRIT-201")
	storage.AddEdge("CRIT-201", "test_case_refs", "TST-301")
	storage.AddEdge("TST-301", "req_refs", "REQ-401")

	// 3. Plan query anchored at BLI-100
	planner := graph.NewQueryPlanner()
	ast := &graph.QueryAST{
		StartNode: graph.NodePattern{
			Variable: "b",
			Kind:     "backlog_item",
			ID:       "BLI-100",
		},
		Steps: []graph.TraversalStep{
			{
				Edge: graph.EdgePattern{Relation: "criteria_refs", Direction: "outgoing"},
				Node: graph.NodePattern{Variable: "c", Kind: "criteria"},
			},
			{
				Edge: graph.EdgePattern{Relation: "test_case_refs", Direction: "outgoing"},
				Node: graph.NodePattern{Variable: "t", Kind: "test_case"},
			},
			{
				Edge: graph.EdgePattern{Relation: "req_refs", Direction: "outgoing"},
				Node: graph.NodePattern{Variable: "r", Kind: "requirement"},
			},
		},
		ReturnVars: []string{"b", "c", "t", "r"},
	}

	plan, err := planner.Plan(ctx, ast)
	require.NoError(t, err)

	// 4. Reset storage metrics and execute plan
	storage.ResetMetrics()
	engine := graph.NewTraversalEngine(graph.DefaultTraversalOptions())
	result, err := engine.Execute(ctx, plan, storage)
	require.NoError(t, err)
	require.Equal(t, 1, result.TotalMatched)
	require.Len(t, result.MatchedPaths[0], 4)

	// Verify exact node IDs in path
	require.Equal(t, "BLI-100", result.MatchedPaths[0][0]["id"])
	require.Equal(t, "CRIT-201", result.MatchedPaths[0][1]["id"])
	require.Equal(t, "TST-301", result.MatchedPaths[0][2]["id"])
	require.Equal(t, "REQ-401", result.MatchedPaths[0][3]["id"])

	// 5. Complexity Proof Verification:
	// Total nodes inspected must be O(K) (exactly K = 4 nodes), NOT O(N) (10,000+ nodes)
	nodesInspected, edgesInspected := storage.Metrics()
	t.Logf("Complexity verification: N=%d, K=4 | nodesInspected=%d, edgesInspected=%d",
		storage.TotalNodes(), nodesInspected, edgesInspected)

	require.Equal(t, 4, nodesInspected, "Indexed traversal must touch exactly K matched nodes")
	require.Less(t, nodesInspected, 10, "Nodes inspected must be O(K), completely independent of N=10,000")
	require.Less(t, edgesInspected, 10, "Edges inspected must be O(K)")
}

// Satisfies CRIT-ZPARQL-CYCLIC-TRAVERSAL-RECURSION-NEGATIVE:
// Path traversal engine enforces maximum recursion depth limits and visited-node bitsets
// to guarantee termination and fail-closed abortion when encountering cyclic graph topologies.
func TestPlanner_CyclicTraversalNegative(t *testing.T) {
	ctx := context.Background()

	// 1. Build Cyclic Graph: A -> B -> C -> A
	storage := graph.NewIndexedGraphStorage()
	storage.AddNode("NODE-A", "item", map[string]any{"name": "A"})
	storage.AddNode("NODE-B", "item", map[string]any{"name": "B"})
	storage.AddNode("NODE-C", "item", map[string]any{"name": "C"})

	storage.AddEdge("NODE-A", "depends_on", "NODE-B")
	storage.AddEdge("NODE-B", "depends_on", "NODE-C")
	storage.AddEdge("NODE-C", "depends_on", "NODE-A") // Cycle back to A

	// Multi-hop AST traversing depends_on across 3 steps
	planner := graph.NewQueryPlanner()
	cyclicAST := &graph.QueryAST{
		StartNode: graph.NodePattern{
			Variable: "n1",
			Kind:     "item",
			ID:       "NODE-A",
		},
		Steps: []graph.TraversalStep{
			{
				Edge: graph.EdgePattern{Relation: "depends_on", Direction: "outgoing"},
				Node: graph.NodePattern{Variable: "n2", Kind: "item"},
			},
			{
				Edge: graph.EdgePattern{Relation: "depends_on", Direction: "outgoing"},
				Node: graph.NodePattern{Variable: "n3", Kind: "item"},
			},
			{
				Edge: graph.EdgePattern{Relation: "depends_on", Direction: "outgoing"},
				Node: graph.NodePattern{Variable: "n4", Kind: "item"},
			},
		},
	}

	plan, err := planner.Plan(ctx, cyclicAST)
	require.NoError(t, err)

	// Negative Test 1: CyclePolicyFailClosed -> Must abort fail-closed with ErrCyclicGraphDetected
	failClosedEngine := graph.NewTraversalEngine(graph.TraversalOptions{
		MaxDepth:    10,
		CyclePolicy: graph.CyclePolicyFailClosed,
	})
	_, err = failClosedEngine.Execute(ctx, plan, storage)
	require.Error(t, err)
	require.ErrorIs(t, err, graph.ErrCyclicGraphDetected)
	require.Contains(t, err.Error(), "cyclic graph topology detected")

	// Test 2: CyclePolicyDeduplicate -> Safely prunes cycle without infinite loop
	dedupEngine := graph.NewTraversalEngine(graph.TraversalOptions{
		MaxDepth:    10,
		CyclePolicy: graph.CyclePolicyDeduplicate,
	})
	result, err := dedupEngine.Execute(ctx, plan, storage)
	require.NoError(t, err)
	require.Empty(t, result.MatchedPaths, "Cycle edge pruned, yielding 0 complete 3-hop simple paths")

	// Negative Test 3: Depth Bound Enforcer -> MaxDepth exceeded aborts
	deepStorage := graph.NewIndexedGraphStorage()
	for i := 1; i <= 20; i++ {
		deepStorage.AddNode(fmt.Sprintf("D-%d", i), "item", nil)
		if i > 1 {
			deepStorage.AddEdge(fmt.Sprintf("D-%d", i-1), "next", fmt.Sprintf("D-%d", i))
		}
	}

	deepAST := &graph.QueryAST{
		StartNode: graph.NodePattern{Variable: "d1", Kind: "item", ID: "D-1"},
		Steps: []graph.TraversalStep{
			{Edge: graph.EdgePattern{Relation: "next"}, Node: graph.NodePattern{Variable: "d2"}},
			{Edge: graph.EdgePattern{Relation: "next"}, Node: graph.NodePattern{Variable: "d3"}},
			{Edge: graph.EdgePattern{Relation: "next"}, Node: graph.NodePattern{Variable: "d4"}},
		},
	}
	deepPlan, err := planner.Plan(ctx, deepAST)
	require.NoError(t, err)

	shallowEngine := graph.NewTraversalEngine(graph.TraversalOptions{
		MaxDepth:    2, // Lower than required 3 hops
		CyclePolicy: graph.CyclePolicyFailClosed,
	})
	_, err = shallowEngine.Execute(ctx, deepPlan, deepStorage)
	require.Error(t, err)
	require.ErrorIs(t, err, graph.ErrMaxRecursionExceeded)
	require.Contains(t, err.Error(), "maximum traversal recursion depth exceeded")
}
