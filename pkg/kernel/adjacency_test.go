package kernel_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/dna"
	"github.com/zqk-os/zqk/pkg/kernel"
)

func TestAdjacencyEngine_BasicGraphOperations(t *testing.T) {
	ctx := context.Background()
	engine := kernel.NewMemoryAdjacencyEngine(nil)

	uA, _ := dna.NewURN("kernel", "node", "A")
	uB, _ := dna.NewURN("kernel", "node", "B")
	uC, _ := dna.NewURN("kernel", "node", "C")

	// Add A -> B
	edgeAB := kernel.Edge{
		Source: uA,
		Target: uB,
		Kind:   "causal",
		Weight: 1.0,
	}
	if err := engine.AddEdge(ctx, edgeAB); err != nil {
		t.Fatalf("AddEdge AB failed: %v", err)
	}

	// Add B -> C
	edgeBC := kernel.Edge{
		Source: uB,
		Target: uC,
		Kind:   "depends_on",
		Weight: 2.5,
	}
	if err := engine.AddEdge(ctx, edgeBC); err != nil {
		t.Fatalf("AddEdge BC failed: %v", err)
	}

	// Verify Outbound
	outA, err := engine.GetOutbound(ctx, uA)
	if err != nil {
		t.Fatalf("GetOutbound A failed: %v", err)
	}
	if len(outA) != 1 || outA[0].Target.String() != uB.String() {
		t.Errorf("expected 1 outbound edge from A to B, got: %v", outA)
	}

	// Verify Inbound
	inC, err := engine.GetInbound(ctx, uC)
	if err != nil {
		t.Fatalf("GetInbound C failed: %v", err)
	}
	if len(inC) != 1 || inC[0].Source.String() != uB.String() {
		t.Errorf("expected 1 inbound edge to C from B, got: %v", inC)
	}

	// Verify DAG has no cycles
	hasCycle, err := engine.HasCycle(ctx)
	if err != nil {
		t.Fatalf("HasCycle check failed: %v", err)
	}
	if hasCycle {
		t.Errorf("expected DAG to have no cycles")
	}

	// Remove Edge
	if err := engine.RemoveEdge(ctx, uA, uB, "causal"); err != nil {
		t.Fatalf("RemoveEdge failed: %v", err)
	}
	outAAfter, _ := engine.GetOutbound(ctx, uA)
	if len(outAAfter) != 0 {
		t.Errorf("expected 0 outbound edges after remove, got %d", len(outAAfter))
	}
}

func TestAdjacencyEngine_CycleDetection(t *testing.T) {
	ctx := context.Background()
	engine := kernel.NewMemoryAdjacencyEngine(nil)

	u1, _ := dna.NewURN("cell", "task", "1")
	u2, _ := dna.NewURN("cell", "task", "2")
	u3, _ := dna.NewURN("cell", "task", "3")

	// Build 1 -> 2 -> 3 -> 1
	_ = engine.AddEdge(ctx, kernel.Edge{Source: u1, Target: u2, Kind: "depends_on"})
	_ = engine.AddEdge(ctx, kernel.Edge{Source: u2, Target: u3, Kind: "depends_on"})
	_ = engine.AddEdge(ctx, kernel.Edge{Source: u3, Target: u1, Kind: "depends_on"})

	hasCycle, err := engine.HasCycle(ctx)
	if err != nil {
		t.Fatalf("HasCycle failed: %v", err)
	}
	if !hasCycle {
		t.Errorf("expected cycle detection to report true")
	}

	cycles, err := engine.DetectCycles(ctx)
	if err != nil {
		t.Fatalf("DetectCycles failed: %v", err)
	}
	if len(cycles) == 0 {
		t.Fatalf("expected detected cycles, got 0")
	}
}

func TestPlaneBoundaryEnforcer(t *testing.T) {
	ctx := context.Background()
	resolver := kernel.NewPlaneMapResolver()
	enforcer := kernel.NewPlaneBoundaryEnforcer(resolver)
	engine := kernel.NewMemoryAdjacencyEngine(enforcer)

	uDraft, _ := dna.NewURN("cell", "item", "draft-1")
	uStaged, _ := dna.NewURN("cell", "item", "staged-1")
	uPromoted, _ := dna.NewURN("cell", "item", "promoted-1")
	uApoptotic, _ := dna.NewURN("cell", "item", "apoptotic-1")

	resolver.SetPlane(uDraft, dna.PlaneDraft)
	resolver.SetPlane(uStaged, dna.PlaneStaged)
	resolver.SetPlane(uPromoted, dna.PlanePromoted)
	resolver.SetPlane(uApoptotic, dna.PlaneApoptotic)

	// 1. Apoptotic boundary test: Draft -> Apoptotic should be strictly rejected
	err := engine.AddEdge(ctx, kernel.Edge{
		Source: uDraft,
		Target: uApoptotic,
		Kind:   "relates_to",
	})
	if err == nil {
		t.Errorf("expected error adding edge to apoptotic target, got nil")
	}

	// 2. Draft causal isolation: Draft -> Promoted with "causal" edge without staging should be rejected
	err = engine.AddEdge(ctx, kernel.Edge{
		Source: uDraft,
		Target: uPromoted,
		Kind:   "causal",
	})
	if err == nil {
		t.Errorf("expected error on direct causal link from Draft to Promoted, got nil")
	}

	// 3. Staged -> Promoted with "causal" edge SHOULD be permitted
	err = engine.AddEdge(ctx, kernel.Edge{
		Source: uStaged,
		Target: uPromoted,
		Kind:   "causal",
	})
	if err != nil {
		t.Fatalf("expected Staged -> Promoted causal edge to be permitted, got: %v", err)
	}

	// 4. Promoted -> Draft causal dependency should be strictly rejected
	err = engine.AddEdge(ctx, kernel.Edge{
		Source: uPromoted,
		Target: uDraft,
		Kind:   "depends_on",
	})
	if err == nil {
		t.Errorf("expected error when Promoted depends on Draft, got nil")
	}

	// 5. Non-causal informational reference Draft -> Promoted should be permitted
	err = engine.AddEdge(ctx, kernel.Edge{
		Source: uDraft,
		Target: uPromoted,
		Kind:   "references",
	})
	if err != nil {
		t.Errorf("expected non-causal reference from Draft to Promoted to succeed, got: %v", err)
	}
}

func TestPlaneIsolation(t *testing.T) {
	TestPlaneBoundaryEnforcer(t)
}

func TestAdjacencyEngine_Concurrency(t *testing.T) {
	ctx := context.Background()
	engine := kernel.NewMemoryAdjacencyEngine(nil)

	const workerCount = 10
	const edgesPerWorker = 50

	var wg sync.WaitGroup
	wg.Add(workerCount)

	for w := 0; w < workerCount; w++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < edgesPerWorker; i++ {
				uSrc, _ := dna.NewURN("cell", "worker", fmt.Sprintf("node-%d-%d", workerID, i))
				uTgt, _ := dna.NewURN("cell", "worker", fmt.Sprintf("node-%d-%d", workerID, i+1))
				_ = engine.AddEdge(ctx, kernel.Edge{
					Source: uSrc,
					Target: uTgt,
					Kind:   "concurrency_test",
				})
				_, _ = engine.GetOutbound(ctx, uSrc)
			}
		}(w)
	}

	wg.Wait()
}
