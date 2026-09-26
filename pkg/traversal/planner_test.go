package traversal_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/traversal"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Satisfies CRIT-ZPARQL-PLANNER-CONTRACT-SPEC:
// Static Floor: Query planner traversal contract, physical/logical plan representation,
// index seek/scan selection, predicate pushdown, and cost estimation.
func TestZPARQL_PlannerContractSpec(t *testing.T) {
	// 1. Verify specification document exists and contains required contracts
	specCandidates := []string{
		filepath.Join("..", "..", "docs", "specs", "SPEC-ZPARQL-INDEXED-QUERY-PLANNER.md"),
		filepath.Join("docs", "specs", "SPEC-ZPARQL-INDEXED-QUERY-PLANNER.md"),
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
	require.True(t, found, "SPEC-ZPARQL-INDEXED-QUERY-PLANNER.md must exist in docs/specs/")
	require.Contains(t, specContent, "CRIT-ZPARQL-PLANNER-CONTRACT-SPEC")
	require.Contains(t, specContent, "CRIT-ZPARQL-INDEX-SCAN-COMPLEXITY-PROOF")
	require.Contains(t, specContent, "CRIT-ZPARQL-CYCLIC-TRAVERSAL-RECURSION-NEGATIVE")
	require.Contains(t, specContent, "IndexSeek")
	require.Contains(t, specContent, "KindScan")
	require.Contains(t, specContent, "Predicate Pushdown")

	// 2. Query Planner Execution Contract Test
	idx := traversal.NewGraphIndex()
	idx.AddNode("BLI-101", "backlog_item", map[string]any{"title": "Test BLI", "priority_tier": "P1"})
	planner := traversal.NewQueryPlanner(idx)

	// Plan for indexed seed ID
	planID, err := planner.Plan(traversal.QuerySpec{
		StartID:  "BLI-101",
		Relation: "criteria_refs",
		FieldFilters: map[string]any{
			"status": "planned",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, planID)
	require.True(t, planID.IndexSeekUsed, "planner must select IndexSeek when start ID is specified")
	require.True(t, planID.PushdownDone, "planner must indicate predicate pushdown was applied")
	require.Equal(t, traversal.OpEdgeExpand, planID.Root.Type)
	require.Equal(t, traversal.OpIndexSeek, planID.Root.Children[0].Type)

	// Plan for kind scan
	planKind, err := planner.Plan(traversal.QuerySpec{
		TargetKind: "backlog_item",
	})
	require.NoError(t, err)
	require.False(t, planKind.IndexSeekUsed)
	require.Equal(t, traversal.OpKindScan, planKind.Root.Type)
}

// Satisfies CRIT-ZPARQL-INDEX-SCAN-COMPLEXITY-PROOF:
// Dynamic Behavior: Traversal engine achieves bound execution complexity proportional to
// matched subgraph volume O(K) using indexed edges rather than scanning full graph space O(N).
func TestZPARQL_IndexScanComplexityProof(t *testing.T) {
	ctx := context.Background()
	idx := traversal.NewGraphIndex()

	// 1. Populate a large graph with N = 10,000 noise nodes
	const totalN = 10000
	for i := 1; i <= totalN; i++ {
		noiseID := fmt.Sprintf("NOISE-%05d", i)
		idx.AddNode(noiseID, "telemetry_record", map[string]any{"seq": i})
	}
	require.Equal(t, totalN, idx.TotalNodes())

	// 2. Insert a targeted chain subgraph of K = 5 linked entities
	// Root -> Node1 -> Node2 -> Node3 -> Leaf
	lineage := []string{"CHAIN-ROOT", "CHAIN-NODE-1", "CHAIN-NODE-2", "CHAIN-NODE-3", "CHAIN-LEAF"}
	for _, id := range lineage {
		idx.AddNode(id, "backlog_item", map[string]any{"id": id, "active": true})
	}
	for i := 0; i < len(lineage)-1; i++ {
		idx.AddEdge(lineage[i], "depends_on", lineage[i+1])
	}

	engine := traversal.NewTraversalEngine(idx)

	// 3. Execute traversal over the targeted lineage
	result, err := engine.Traverse(ctx, "CHAIN-ROOT", "depends_on", 10)
	require.NoError(t, err)

	// PROOF VERIFICATION:
	// a) Matched subgraph volume K = 5
	const K = 5
	require.Equal(t, K, len(result.MatchedEntities), "expected to match exactly K entities")
	require.Equal(t, lineage, result.VisitedNodes)

	// b) Complexity proof: Total operations executed must be O(K) (e.g. <= 20)
	// and completely decoupled from total graph space N = 10,000
	require.LessOrEqual(t, result.OperationsCount, 25,
		"traversal operations (%d) must be strictly O(K) bounded and negligible compared to N (%d)",
		result.OperationsCount, totalN)

	// Verify constant ratio: OperationsCount / K << N
	ratio := float64(result.OperationsCount) / float64(totalN)
	require.Less(t, ratio, 0.005, "ratio of operations to total graph space must be < 0.5%%")
}

// Satisfies CRIT-ZPARQL-CYCLIC-TRAVERSAL-RECURSION-NEGATIVE:
// Negative Invariant: Path traversal engine enforces maximum recursion depth limits
// and visited-node bitsets to guarantee termination and fail-closed abortion when encountering cyclic graph topologies.
func TestZPARQL_CyclicTraversalRecursionNegative(t *testing.T) {
	ctx := context.Background()

	t.Run("cycle detection: 3-node cycle A -> B -> C -> A terminates safely", func(t *testing.T) {
		idx := traversal.NewGraphIndex()
		idx.AddNode("NODE-A", "item", nil)
		idx.AddNode("NODE-B", "item", nil)
		idx.AddNode("NODE-C", "item", nil)

		// Create cycle
		idx.AddEdge("NODE-A", "next", "NODE-B")
		idx.AddEdge("NODE-B", "next", "NODE-C")
		idx.AddEdge("NODE-C", "next", "NODE-A")

		engine := traversal.NewTraversalEngine(idx)
		result, err := engine.Traverse(ctx, "NODE-A", "next", 10)

		require.NoError(t, err, "cycle-safe traversal must terminate without infinite loop")
		require.True(t, result.CycleDetected, "engine must flag that a cycle was detected")
		require.Equal(t, 3, len(result.MatchedEntities), "each node in cycle must be visited once")
	})

	t.Run("self-loop: A -> A terminates safely", func(t *testing.T) {
		idx := traversal.NewGraphIndex()
		idx.AddNode("SELF-LOOP-NODE", "item", nil)
		idx.AddEdge("SELF-LOOP-NODE", "next", "SELF-LOOP-NODE")

		engine := traversal.NewTraversalEngine(idx)
		result, err := engine.Traverse(ctx, "SELF-LOOP-NODE", "next", 5)

		require.NoError(t, err)
		require.True(t, result.CycleDetected)
		require.Equal(t, 1, len(result.MatchedEntities))
	})

	t.Run("depth bound fail-closed abortion: exceed max depth", func(t *testing.T) {
		idx := traversal.NewGraphIndex()
		// Chain of 6 nodes
		for i := 1; i <= 6; i++ {
			idx.AddNode(fmt.Sprintf("D-%d", i), "item", nil)
			if i > 1 {
				idx.AddEdge(fmt.Sprintf("D-%d", i-1), "child", fmt.Sprintf("D-%d", i))
			}
		}

		engine := traversal.NewTraversalEngine(idx)

		// Set maxDepth = 3 on a path of length 6
		_, err := engine.Traverse(ctx, "D-1", "child", 3)
		require.Error(t, err)
		require.True(t, errors.Is(err, traversal.ErrCyclicRecursionExceeded),
			"must return ErrCyclicRecursionExceeded when recursion depth bound is exceeded")
	})
}
