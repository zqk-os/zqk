// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package shovelready

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

// Helper: build a baseline valid shovel-ready BLI map.
func createTestBLI(id string, tier string, effort string, ws string) map[string]any {
	return map[string]any{
		objects.FieldKeyID:              id,
		objects.FieldKeyRequirementRefs: []string{"REQ-TEST-1"},
		objects.FieldKeyCriteriaRefs:    []string{"CRIT-TEST-1"},
		objects.FieldKeyEstimatedEffort: effort,
		objects.FieldKeyPriorityTier:    tier,
		objects.FieldKeyPersonaRefs:     []string{"PER-CODER"},
		objects.FieldKeyWorkstreamRef:   ws,
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
	}
}

// ---------------------------------------------------------------------------
// 1. ShovelReadyEvaluator & shovel_ready.go Coverage Tests
// ---------------------------------------------------------------------------

func TestCycle210_Evaluator_HolesAndOptions(t *testing.T) {
	t.Parallel()

	// Verify uncovered StakeholderRefs branch in shovel_ready.go
	bliWithStakeholderRefs := map[string]any{
		objects.FieldKeyID:              "BLI-STK-REFS",
		objects.FieldKeyRequirementRefs: []string{"REQ-1"},
		objects.FieldKeyCriteriaRefs:    []string{"CRIT-1"},
		objects.FieldKeyEstimatedEffort: "3h",
		objects.FieldKeyStakeholderRefs: []string{"STK-LEAD-1"},
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
	}
	res := Evaluate(bliWithStakeholderRefs)
	if !res.Ready {
		t.Fatalf("expected bliWithStakeholderRefs to be ready, missing: %v", res.Missing)
	}

	// Evaluator with custom bypass prefix
	customBypassEval := NewEvaluator(WithBypassPrefix("DRAFT-"))
	draftBLI := map[string]any{objects.FieldKeyID: "DRAFT-123"}
	if !customBypassEval.IsReady(draftBLI) {
		t.Fatalf("expected custom bypass prefix DRAFT- to be ready")
	}

	// Evaluator with strict effort
	strictEval := NewEvaluator(WithStrictEffort(true))
	bliWithOnlyTier := map[string]any{
		objects.FieldKeyID:              "BLI-NO-EFFORT",
		objects.FieldKeyRequirementRefs: []string{"REQ-1"},
		objects.FieldKeyCriteriaRefs:    []string{"CRIT-1"},
		objects.FieldKeyPriorityTier:    "P0",
		objects.FieldKeyPersonaRefs:     []string{"PER-1"},
	}
	if strictEval.IsReady(bliWithOnlyTier) {
		t.Fatalf("strict effort evaluator should reject BLI missing explicit estimated_effort")
	}

	// Evaluator with custom validator
	customValidatorEval := NewEvaluator(WithCustomValidator(func(b map[string]any) []string {
		if _, ok := b["security_review"]; !ok {
			return []string{"missing_security_review"}
		}
		return nil
	}))
	normalReady := createTestBLI("BLI-SEC-1", "P1", "2h", "ws-core")
	secRes := customValidatorEval.Evaluate(normalReady)
	if secRes.Ready || !slices.Contains(secRes.Missing, "missing_security_review") {
		t.Fatalf("expected custom validator to flag missing_security_review: %v", secRes.Missing)
	}

	normalReady["security_review"] = "approved"
	if !customValidatorEval.IsReady(normalReady) {
		t.Fatalf("expected item with security_review to be ready")
	}
}

func TestCycle210_Evaluator_BatchAndFilter(t *testing.T) {
	t.Parallel()

	eval := NewEvaluator()
	ready1 := createTestBLI("BLI-R1", "P0", "1h", "ws-1")
	ready2 := createTestBLI("BLI-R2", "P1", "2h", "ws-1")
	unready := map[string]any{objects.FieldKeyID: "BLI-UNREADY"}

	items := []map[string]any{ready1, unready, ready2}
	results := eval.EvaluateBatch(items)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if !results[0].Ready || results[1].Ready || !results[2].Ready {
		t.Fatalf("unexpected batch results: %+v", results)
	}

	filtered := eval.FilterReady(items)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 ready items, got %d", len(filtered))
	}
	if filtered[0][objects.FieldKeyID] != "BLI-R1" || filtered[1][objects.FieldKeyID] != "BLI-R2" {
		t.Fatalf("unexpected filtered items: %+v", filtered)
	}
}

// ---------------------------------------------------------------------------
// 2. DependencyGraphAnalyzer Coverage Tests
// ---------------------------------------------------------------------------

func TestCycle210_DependencyGraph_BasicAndAcyclic(t *testing.T) {
	t.Parallel()

	analyzer := NewDependencyGraphAnalyzer()
	if count := analyzer.NodeCount(); count != 0 {
		t.Fatalf("expected initial node count 0, got %d", count)
	}

	// Register linear DAG: A -> B -> C (A depends on B, B depends on C)
	analyzer.AddNode("BLI-A", []string{"BLI-B"}, false)
	analyzer.AddNode("BLI-B", []string{"BLI-C"}, false)
	analyzer.AddNode("BLI-C", nil, false)

	if count := analyzer.NodeCount(); count != 3 {
		t.Fatalf("expected node count 3, got %d", count)
	}

	nodeB, ok := analyzer.GetNode("BLI-B")
	if !ok || nodeB.ID != "BLI-B" || len(nodeB.Dependencies) != 1 || nodeB.Dependencies[0] != "BLI-C" {
		t.Fatalf("unexpected nodeB: %+v", nodeB)
	}
	if _, found := analyzer.GetNode("UNKNOWN"); found {
		t.Fatalf("expected UNKNOWN node to not be found")
	}

	cyclePath, err := analyzer.ValidateAcyclic()
	if err != nil {
		t.Fatalf("expected acyclic graph, got err: %v, path: %v", err, cyclePath)
	}

	order, err := analyzer.GetTopologicalOrder()
	if err != nil {
		t.Fatalf("topological order failed: %v", err)
	}
	// In A -> B -> C, C must precede B, and B must precede A
	posC := slices.Index(order, "BLI-C")
	posB := slices.Index(order, "BLI-B")
	posA := slices.Index(order, "BLI-A")
	if posC > posB || posB > posA {
		t.Fatalf("invalid topological order: %v (positions: C=%d, B=%d, A=%d)", order, posC, posB, posA)
	}
}

func TestCycle210_DependencyGraph_CycleDetection(t *testing.T) {
	t.Parallel()

	analyzer := NewDependencyGraphAnalyzer()
	// Create cycle: A -> B -> C -> A
	analyzer.AddNode("BLI-A", []string{"BLI-B"}, false)
	analyzer.AddNode("BLI-B", []string{"BLI-C"}, false)
	analyzer.AddNode("BLI-C", []string{"BLI-A"}, false)

	cyclePath, err := analyzer.ValidateAcyclic()
	if err == nil {
		t.Fatalf("expected cycle error, got nil")
	}
	if !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("expected ErrCycleDetected, got: %v", err)
	}
	if len(cyclePath) == 0 {
		t.Fatalf("expected non-empty cycle path, got empty")
	}

	// Topological order must fail on cycle
	if _, err := analyzer.GetTopologicalOrder(); err == nil {
		t.Fatalf("expected error from GetTopologicalOrder on cyclic graph")
	}

	// Remove node to break cycle
	analyzer.RemoveNode("BLI-C")
	if _, err := analyzer.ValidateAcyclic(); err != nil {
		t.Fatalf("expected graph to be acyclic after removing node C: %v", err)
	}
}

func TestCycle210_DependencyGraph_AddFromBLI(t *testing.T) {
	t.Parallel()

	analyzer := NewDependencyGraphAnalyzer()
	analyzer.AddNodeFromBLI(nil)
	analyzer.AddNodeFromBLI(map[string]any{})
	if analyzer.NodeCount() != 0 {
		t.Fatalf("empty or nil BLI must not add node")
	}

	bli := map[string]any{
		objects.FieldKeyID:              "BLI-DEP-TEST",
		"depends_on":                    "BLI-DEP-1",
		"dependencies":                  []string{"BLI-DEP-2", "BLI-DEP-1"},
		"dependency_refs":               []any{"BLI-DEP-3", 123},
		objects.FieldKeyRiskBlockerRefs: []string{"BLI-BLOCKER-1"},
		objects.FieldKeyStatus:          objects.ObjectStatusBlocked,
	}

	analyzer.AddNodeFromBLI(bli)
	node, ok := analyzer.GetNode("BLI-DEP-TEST")
	if !ok {
		t.Fatalf("node not added from BLI")
	}
	if !node.IsBlocked {
		t.Fatalf("expected node to be blocked")
	}
	if len(node.Dependencies) < 4 {
		t.Fatalf("expected at least 4 dependencies parsed, got %d (%v)", len(node.Dependencies), node.Dependencies)
	}
}

func TestCycle210_DependencyGraph_BlockedAncestorsAndMissingRefs(t *testing.T) {
	t.Parallel()

	analyzer := NewDependencyGraphAnalyzer()
	// Tree:
	// BLI-LEAF depends on BLI-MID1 and BLI-MID2
	// BLI-MID1 depends on BLI-ROOT (which is blocked)
	// BLI-MID2 depends on BLI-EXTERNAL (missing from graph)
	analyzer.AddNode("BLI-LEAF", []string{"BLI-MID1", "BLI-MID2"}, false)
	analyzer.AddNode("BLI-MID1", []string{"BLI-ROOT"}, false)
	analyzer.AddNode("BLI-MID2", []string{"BLI-EXTERNAL"}, false)
	analyzer.AddNode("BLI-ROOT", nil, true)

	// Blocked ancestors check
	blockers, isBlocked := analyzer.ResolveBlockedAncestors("BLI-LEAF")
	if !isBlocked {
		t.Fatalf("expected BLI-LEAF to have blocked ancestors")
	}
	if !slices.Contains(blockers, "BLI-ROOT") {
		t.Fatalf("expected BLI-ROOT in blockers, got: %v", blockers)
	}

	// Unblocked leaf check
	analyzer.AddNode("BLI-CLEAN", []string{"BLI-MID2"}, false)
	cleanBlockers, cleanBlocked := analyzer.ResolveBlockedAncestors("BLI-CLEAN")
	if cleanBlocked || len(cleanBlockers) > 0 {
		t.Fatalf("expected BLI-CLEAN to be unblocked, got blockers: %v", cleanBlockers)
	}

	// Missing references check
	knownIDs := map[string]struct{}{
		"BLI-LEAF": {},
		"BLI-MID1": {},
		"BLI-MID2": {},
		"BLI-ROOT": {},
	}
	missing := analyzer.HandleMissingReferences(knownIDs)
	if !slices.Contains(missing, "BLI-EXTERNAL") {
		t.Fatalf("expected BLI-EXTERNAL to be identified as missing ref, got: %v", missing)
	}
}

// ---------------------------------------------------------------------------
// 3. ReadinessScorer Coverage Tests
// ---------------------------------------------------------------------------

func TestCycle210_ReadinessScorer_WeightsAndThreshold(t *testing.T) {
	t.Parallel()

	// Zero sum weights fallback
	zeroWeights := ScoringWeights{}
	norm := zeroWeights.Normalize()
	if norm.RequirementWeight <= 0 {
		t.Fatalf("expected normalized fallback to DefaultScoringWeights")
	}

	// Threshold bounds
	scorerZeroThresh := NewReadinessScorer(DefaultScoringWeights, -0.5)
	if scorerZeroThresh.threshold != 0.80 {
		t.Fatalf("expected threshold <=0 to default to 0.80, got %f", scorerZeroThresh.threshold)
	}
	scorerMaxThresh := NewReadinessScorer(DefaultScoringWeights, 1.5)
	if scorerMaxThresh.threshold != 1.0 {
		t.Fatalf("expected threshold > 1.0 to clamp to 1.0, got %f", scorerMaxThresh.threshold)
	}

	scorer := DefaultReadinessScorer()

	// Nil BLI score
	nilScore := scorer.Score(nil, true)
	if nilScore.TotalScore != 0.0 || nilScore.PassedThreshold {
		t.Fatalf("expected nil BLI to have 0.0 total score and not pass threshold")
	}

	// Fully shovel-ready BLI
	readyBLI := createTestBLI("BLI-FULL", "P1", "2h", "ws-1")
	fullScore := scorer.Score(readyBLI, true)
	if math.Abs(fullScore.TotalScore-1.0) > 0.001 {
		t.Fatalf("expected full score 1.0, got %f", fullScore.TotalScore)
	}
	if !fullScore.PassedThreshold {
		t.Fatalf("expected full score to pass threshold")
	}

	// Score with blockers
	blockedScore := scorer.Score(readyBLI, false)
	if blockedScore.DependencyScore != 0.0 {
		t.Fatalf("expected 0 dependency score when blocked, got %f", blockedScore.DependencyScore)
	}
	if blockedScore.TotalScore >= fullScore.TotalScore {
		t.Fatalf("blocked score (%f) must be lower than full score (%f)", blockedScore.TotalScore, fullScore.TotalScore)
	}

	// Technical spec substitute
	bliWithSpec := map[string]any{
		objects.FieldKeyID:                "BLI-TSPEC",
		objects.FieldKeyTechnicalSpecRefs: []string{"TDE-1"},
		objects.FieldKeyCriteriaRefs:      []string{"CRIT-1"},
		objects.FieldKeyEstimatedEffort:   "4h",
		objects.FieldKeyPersonaRefs:       []string{"PER-1"},
	}
	specScore := scorer.Score(bliWithSpec, true)
	if specScore.RequirementScore <= 0 {
		t.Fatalf("technical_spec_refs must satisfy RequirementScore")
	}

	// Batch scoring
	batchScores := scorer.ScoreBatch([]map[string]any{readyBLI, nil, bliWithSpec}, map[string]bool{
		"BLI-FULL":  true,
		"BLI-TSPEC": false,
	})
	if len(batchScores) != 2 {
		t.Fatalf("expected 2 non-nil scores in batch, got %d", len(batchScores))
	}
	if !batchScores["BLI-FULL"].PassedThreshold {
		t.Fatalf("expected BLI-FULL to pass threshold in batch")
	}
}

// ---------------------------------------------------------------------------
// 4. BatchAllocator & PriorityPlanPacker Coverage Tests
// ---------------------------------------------------------------------------

func TestCycle210_ParseEffortHours(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  float64
	}{
		{"", 2.0},
		{"   ", 2.0},
		{"2h", 2.0},
		{"1.5h", 1.5},
		{"1d", 8.0},
		{"0.5d", 4.0},
		{"30m", 0.5},
		{"60m", 1.0},
		{"3.5", 3.5},
		{"small", 2.0},
		{"s", 2.0},
		{"medium", 4.0},
		{"m", 4.0},
		{"large", 8.0},
		{"l", 8.0},
		{"invalid-random", 2.0},
	}

	for _, tc := range tests {
		got := ParseEffortHours(tc.input)
		if math.Abs(got-tc.want) > 0.001 {
			t.Errorf("ParseEffortHours(%q) = %f, want %f", tc.input, got, tc.want)
		}
	}
}

func TestCycle210_CandidateFromBLI(t *testing.T) {
	t.Parallel()

	emptyCandidate := CandidateFromBLI(nil, 0.0, false)
	if emptyCandidate.ID != "" {
		t.Fatalf("expected empty candidate for nil input")
	}

	bli := map[string]any{
		objects.FieldKeyID:              "BLI-CANDIDATE-1",
		objects.FieldKeyPriority:        "p1",
		objects.FieldKeyWorkstreamRefs:  []string{"ws-core", "ws-extra"},
		objects.FieldKeyRequirementRefs: []string{"REQ-1"},
		objects.FieldKeyEstimatedEffort: "4h",
	}
	c := CandidateFromBLI(bli, 0.95, true)
	if c.ID != "BLI-CANDIDATE-1" || c.PriorityTier != "p1" || c.WorkstreamRef != "ws-core" {
		t.Fatalf("unexpected candidate: %+v", c)
	}
	if c.EstimatedHours != 4.0 || c.ReadinessScore != 0.95 || !c.IsReady {
		t.Fatalf("unexpected candidate metrics: %+v", c)
	}
}

func TestCycle210_BatchAllocator_Allocation(t *testing.T) {
	t.Parallel()

	constraints := DefaultBatchConstraints()
	constraints.MinBatchSize = 2
	constraints.MaxBatchSize = 3
	constraints.MaxCapacityHours = 10.0

	allocator := NewBatchAllocator(constraints)

	// Test invalid constraints
	badAllocator := NewBatchAllocator(BatchConstraints{MaxBatchSize: 0})
	if _, _, err := badAllocator.Allocate(nil); !errors.Is(err, ErrInvalidConstraints) {
		t.Fatalf("expected ErrInvalidConstraints, got: %v", err)
	}

	// Test no ready items
	unreadyCandidate := BacklogItemCandidate{ID: "BLI-U1", IsReady: false, ReadinessScore: 0.5}
	if _, unallocated, err := allocator.Allocate([]BacklogItemCandidate{unreadyCandidate}); !errors.Is(err, ErrNoShovelReadyItems) {
		t.Fatalf("expected ErrNoShovelReadyItems, got: %v", err)
	} else if len(unallocated) != 1 {
		t.Fatalf("expected unready item to be returned in unallocated")
	}

	// Prepare candidates across two workstreams
	candidates := []BacklogItemCandidate{
		{ID: "BLI-WS1-P0", WorkstreamRef: "ws-1", PriorityTier: "P0", EstimatedHours: 3.0, ReadinessScore: 1.0, IsReady: true},
		{ID: "BLI-WS1-P1", WorkstreamRef: "ws-1", PriorityTier: "P1", EstimatedHours: 4.0, ReadinessScore: 0.95, IsReady: true},
		{ID: "BLI-WS1-P2", WorkstreamRef: "ws-1", PriorityTier: "P2", EstimatedHours: 2.0, ReadinessScore: 0.90, IsReady: true},
		{ID: "BLI-WS2-P0", WorkstreamRef: "ws-2", PriorityTier: "P0", EstimatedHours: 5.0, ReadinessScore: 1.0, IsReady: true},
		{ID: "BLI-WS2-P1", WorkstreamRef: "ws-2", PriorityTier: "P1", EstimatedHours: 4.0, ReadinessScore: 0.85, IsReady: true},
		{ID: "BLI-LOW-SCORE", WorkstreamRef: "ws-1", PriorityTier: "P0", EstimatedHours: 1.0, ReadinessScore: 0.50, IsReady: true},
	}

	batches, unallocated, err := allocator.Allocate(candidates)
	if err != nil {
		t.Fatalf("allocation failed: %v", err)
	}

	if len(batches) != 2 {
		t.Fatalf("expected 2 batches (one per workstream), got %d", len(batches))
	}
	if batches[0].WorkstreamRef != "ws-1" || len(batches[0].Items) != 3 {
		t.Fatalf("unexpected batch 0: %+v", batches[0])
	}
	if batches[1].WorkstreamRef != "ws-2" || len(batches[1].Items) != 2 {
		t.Fatalf("unexpected batch 1: %+v", batches[1])
	}

	// BLI-LOW-SCORE should be in unallocated
	foundLowScore := false
	for _, it := range unallocated {
		if it.ID == "BLI-LOW-SCORE" {
			foundLowScore = true
			break
		}
	}
	if !foundLowScore {
		t.Fatalf("expected BLI-LOW-SCORE to be in unallocated")
	}

	// Mixed workstreams allowed allocation
	mixedConstraints := constraints
	mixedConstraints.EnforceSingleWorkstream = false
	mixedAllocator := NewBatchAllocator(mixedConstraints)
	mixedBatches, _, err := mixedAllocator.Allocate(candidates)
	if err != nil {
		t.Fatalf("mixed allocation failed: %v", err)
	}
	if len(mixedBatches) == 0 {
		t.Fatalf("expected mixed batches")
	}
}

func TestCycle210_PriorityPlanPacker_Pack(t *testing.T) {
	t.Parallel()

	packer := NewPriorityPlanPacker()
	constraints := DefaultBatchConstraints()
	constraints.MinBatchSize = 1
	constraints.MaxBatchSize = 2

	candidates := []BacklogItemCandidate{
		{ID: "BLI-P0", PriorityTier: "P0", EstimatedHours: 2.0, ReadinessScore: 1.0, IsReady: true},
		{ID: "BLI-P1", PriorityTier: "P1", EstimatedHours: 3.0, ReadinessScore: 0.9, IsReady: true},
		{ID: "BLI-P2", PriorityTier: "P2", EstimatedHours: 4.0, ReadinessScore: 0.85, IsReady: true},
	}

	planBatch, leftover, err := packer.PackPlan("PRI-TEST-001", "ws-core", candidates, constraints)
	if err != nil {
		t.Fatalf("PackPlan failed: %v", err)
	}
	if planBatch.BatchID != "PRI-TEST-001" {
		t.Fatalf("expected plan BatchID to be PRI-TEST-001, got %s", planBatch.BatchID)
	}
	if len(planBatch.Items) != 2 {
		t.Fatalf("expected 2 items packed, got %d", len(planBatch.Items))
	}
	if planBatch.Items[0].ID != "BLI-P0" || planBatch.Items[1].ID != "BLI-P1" {
		t.Fatalf("expected P0 and P1 packed, got %s and %s", planBatch.Items[0].ID, planBatch.Items[1].ID)
	}
	if len(leftover) != 1 || leftover[0].ID != "BLI-P2" {
		t.Fatalf("expected BLI-P2 in leftover, got %+v", leftover)
	}

	// Test packing with empty candidates
	if _, _, err := packer.PackPlan("PRI-EMPTY", "ws-core", nil, constraints); !errors.Is(err, ErrNoShovelReadyItems) {
		t.Fatalf("expected ErrNoShovelReadyItems for empty candidates, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 5. Concurrency and Resiliency Tests
// ---------------------------------------------------------------------------

func TestCycle210_Concurrency_EvaluatorAndScorer(t *testing.T) {
	t.Parallel()

	eval := NewEvaluator()
	scorer := DefaultReadinessScorer()
	graph := NewDependencyGraphAnalyzer()

	var wg sync.WaitGroup
	workers := 10
	iterations := 25

	for w := 0; w < workers; w++ {
		workerID := w
		wg.Add(1)
		builder := goroutinelabels.NewGoroutine(
			fmt.Sprintf("cycle210_eval_worker_%d", workerID),
			"concurrent shovel-ready evaluation and graph ingestion",
		)
		builder.StartSimple(func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				bliID := fmt.Sprintf("BLI-CONC-%d-%d", workerID, i)
				depID := fmt.Sprintf("BLI-CONC-%d-%d", workerID, (i+1)%iterations)
				bli := createTestBLI(bliID, "P1", "2h", "ws-conc")
				bli["depends_on"] = depID

				// Evaluate
				res := eval.Evaluate(bli)
				if !res.Ready {
					t.Errorf("worker %d item %s should be ready", workerID, bliID)
				}

				// Score
				score := scorer.Score(bli, true)
				if score.TotalScore < 0.99 {
					t.Errorf("worker %d item %s expected score ~1.0, got %f", workerID, bliID, score.TotalScore)
				}

				// Graph registration
				graph.AddNodeFromBLI(bli)
			}
		})
	}

	wg.Wait()

	if graph.NodeCount() != workers*iterations {
		t.Fatalf("expected %d nodes in graph, got %d", workers*iterations, graph.NodeCount())
	}
}

func TestCycle210_CoverageEdgeCases_Helpers(t *testing.T) {
	t.Parallel()

	// eval.Evaluate(nil) check
	eval := NewEvaluator()
	nilRes := eval.Evaluate(nil)
	if nilRes.Ready || len(nilRes.Missing) != 1 || nilRes.Missing[0] != "object" {
		t.Fatalf("expected nil BLI on evaluator to return missing object, got %+v", nilRes)
	}

	// PackPlan with multiple batches
	packer := NewPriorityPlanPacker()
	packConstraints := DefaultBatchConstraints()
	packConstraints.MinBatchSize = 1
	packConstraints.MaxBatchSize = 2
	packCandidates := []BacklogItemCandidate{
		{ID: "BLI-1", PriorityTier: "P0", EstimatedHours: 1.0, ReadinessScore: 1.0, IsReady: true},
		{ID: "BLI-2", PriorityTier: "P0", EstimatedHours: 1.0, ReadinessScore: 1.0, IsReady: true},
		{ID: "BLI-3", PriorityTier: "P1", EstimatedHours: 1.0, ReadinessScore: 1.0, IsReady: true},
		{ID: "BLI-4", PriorityTier: "P1", EstimatedHours: 1.0, ReadinessScore: 1.0, IsReady: true},
	}
	lead, leftover, err := packer.PackPlan("PRI-MULTI", "ws-1", packCandidates, packConstraints)
	if err != nil {
		t.Fatalf("PackPlan multi failed: %v", err)
	}
	if len(lead.Items) != 2 || len(leftover) != 2 {
		t.Fatalf("expected 2 lead items and 2 leftover items, got lead=%d leftover=%d", len(lead.Items), len(leftover))
	}

	// ResolveBlockedAncestors with status "blocked"
	graph := NewDependencyGraphAnalyzer()
	graph.AddNode("BLI-CHILD", []string{"BLI-PARENT"}, false)
	parentBLI := map[string]any{
		objects.FieldKeyID:     "BLI-PARENT",
		objects.FieldKeyStatus: objects.ObjectStatusBlocked,
	}
	graph.AddNodeFromBLI(parentBLI)
	blockers, blocked := graph.ResolveBlockedAncestors("BLI-CHILD")
	if !blocked || !slices.Contains(blockers, "BLI-PARENT") {
		t.Fatalf("expected BLI-PARENT with status blocked to be detected as blocker")
	}

	// computeAverageScore empty check
	if avg := computeAverageScore(nil); avg != 0.0 {
		t.Fatalf("expected 0.0 average score for nil items, got %f", avg)
	}

	// priorityTierScore edge cases
	if priorityTierScore("p0-urgent") != 400 {
		t.Fatalf("p0-urgent score mismatch")
	}
	if priorityTierScore("p1-high") != 300 {
		t.Fatalf("p1-high score mismatch")
	}
	if priorityTierScore("p2-medium") != 200 {
		t.Fatalf("p2-medium score mismatch")
	}
	if priorityTierScore("p3-low") != 100 {
		t.Fatalf("p3-low score mismatch")
	}
	if priorityTierScore("unrecognized") != 0 {
		t.Fatalf("unrecognized score mismatch")
	}

	// DefaultScoringWeights check
	if DefaultScoringWeights.RequirementWeight != 0.25 {
		t.Fatalf("default requirement weight mismatch")
	}
}
