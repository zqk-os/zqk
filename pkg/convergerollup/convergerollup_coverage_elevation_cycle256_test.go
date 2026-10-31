package convergerollup

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestCollectCVSTreeBFS_ErrorsAndValidation(t *testing.T) {
	t.Parallel()

	dummyLoader := func(id string) ([]string, string, string, error) {
		return nil, "active", "c1_scope", nil
	}

	// 1. rootID empty
	nodes, err := CollectCVSTreeBFS("", 3, dummyLoader)
	if err == nil || nodes != nil {
		t.Fatalf("expected error on empty rootID, got nodes=%v, err=%v", nodes, err)
	}

	// 2. nodeFor nil
	nodes, err = CollectCVSTreeBFS("CVS-root", 3, nil)
	if err == nil || nodes != nil {
		t.Fatalf("expected error on nil nodeFor, got nodes=%v, err=%v", nodes, err)
	}

	// 3. maxDepth < 0
	nodes, err = CollectCVSTreeBFS("CVS-root", -1, dummyLoader)
	if err == nil || nodes != nil {
		t.Fatalf("expected error on negative maxDepth, got nodes=%v, err=%v", nodes, err)
	}

	// 4. nodeFor returns error on root
	errBoom := errors.New("boom root")
	nodes, err = CollectCVSTreeBFS("CVS-root", 3, func(id string) ([]string, string, string, error) {
		return nil, "", "", errBoom
	})
	if err == nil || !strings.Contains(err.Error(), "boom root") {
		t.Fatalf("expected boom root error, got err=%v", err)
	}

	// 5. nodeFor returns error on child
	errChildBoom := errors.New("boom child")
	nodes, err = CollectCVSTreeBFS("CVS-root", 3, func(id string) ([]string, string, string, error) {
		if id == "CVS-root" {
			return []string{"CVS-child1"}, "active", "c1", nil
		}
		return nil, "", "", errChildBoom
	})
	if err == nil || !strings.Contains(err.Error(), "boom child") {
		t.Fatalf("expected boom child error, got err=%v", err)
	}

	// 6. Non-CVS refs, empty refs, duplicate refs, and maxDepth limit
	nodes, err = CollectCVSTreeBFS("CVS-root", 1, func(id string) ([]string, string, string, error) {
		if id == "CVS-root" {
			return []string{"", "NOT_CVS_ID", "CVS-child1", "CVS-child1", "CVS-child2"}, "active", "c1", nil
		}
		return []string{"CVS-grandchild"}, "active", "c1", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes (root, child1, child2), got %d: %v", len(nodes), nodes)
	}
}

func TestValidateNestLink_Elevation(t *testing.T) {
	t.Parallel()

	// 1. maxDepth <= 0 uses default
	tree := map[string][]string{
		"CVS-coord": {"CVS-child"},
		"CVS-child": nil,
		"CVS-new":   nil,
	}
	loader := func(id string) ([]string, string, string, error) {
		return tree[id], "active", "c1", nil
	}
	err := ValidateNestLink("CVS-coord", "CVS-coord", "CVS-new", 0, loader)
	if err != nil {
		t.Fatalf("expected success with maxDepth 0 (default), got: %v", err)
	}

	// 2. coordinator tree loader error
	errCoordFail := errors.New("coord lookup fail")
	failCoordLoader := func(id string) ([]string, string, string, error) {
		if id == "CVS-coord" {
			return nil, "", "", errCoordFail
		}
		return nil, "active", "c1", nil
	}
	err = ValidateNestLink("CVS-coord", "CVS-coord", "CVS-new", 3, failCoordLoader)
	if err == nil || !strings.Contains(err.Error(), "coord lookup fail") {
		t.Fatalf("expected coord lookup fail, got: %v", err)
	}

	// 3. child tree loader error
	errChildFail := errors.New("child lookup fail")
	failChildLoader := func(id string) ([]string, string, string, error) {
		if id == "CVS-new" {
			return nil, "", "", errChildFail
		}
		return nil, "active", "c1", nil
	}
	err = ValidateNestLink("CVS-coord", "CVS-coord", "CVS-new", 3, failChildLoader)
	if err == nil || !strings.Contains(err.Error(), "child lookup fail") {
		t.Fatalf("expected child lookup fail, got: %v", err)
	}

	// 4. parent not found under coordinator
	err = ValidateNestLink("CVS-coord", "CVS-unknown", "CVS-new", 3, loader)
	if err == nil || !strings.Contains(err.Error(), "not found under coordinator") {
		t.Fatalf("expected parent not found error, got: %v", err)
	}

	// 5. exceeding max depth
	// Root (depth 0) -> C1 (depth 1) -> C2 (depth 2). MaxDepth = 2. C2 cannot nest child (depth 3).
	deepTree := map[string][]string{
		"CVS-root": {"CVS-lvl1"},
		"CVS-lvl1": {"CVS-lvl2"},
		"CVS-lvl2": nil,
		"CVS-lvl3": nil,
	}
	deepLoader := func(id string) ([]string, string, string, error) {
		return deepTree[id], "active", "c1", nil
	}
	err = ValidateNestLink("CVS-root", "CVS-lvl2", "CVS-lvl3", 2, deepLoader)
	if err == nil || !strings.Contains(err.Error(), "would exceed max depth 2") {
		t.Fatalf("expected max depth exceeded error, got: %v", err)
	}

	// 6. cycle detection: child tree contains coordinatorID or parentID
	cycleTree := map[string][]string{
		"CVS-coord": {"CVS-p"},
		"CVS-p":     nil,
		"CVS-c":     {"CVS-coord"},
	}
	cycleLoader := func(id string) ([]string, string, string, error) {
		return cycleTree[id], "active", "c1", nil
	}
	err = ValidateNestLink("CVS-coord", "CVS-p", "CVS-c", 3, cycleLoader)
	if err == nil || !strings.Contains(err.Error(), "would create a cycle") {
		t.Fatalf("expected cycle error, got: %v", err)
	}

	// 7. self nesting
	err = ValidateNestLink("CVS-coord", "CVS-same", "CVS-same", 3, loader)
	if err == nil || !strings.Contains(err.Error(), "cannot nest CVS under itself") {
		t.Fatalf("expected self-nest error, got: %v", err)
	}

	// 8. non-CVS IDs
	err = ValidateNestLink("CVS-coord", "NOTCVS-1", "CVS-c", 3, loader)
	if err == nil || !strings.Contains(err.Error(), "requires CVS-* ids") {
		t.Fatalf("expected CVS prefix error, got: %v", err)
	}

	// 9. empty IDs
	err = ValidateNestLink("CVS-coord", "", "CVS-c", 3, loader)
	if err == nil || !strings.Contains(err.Error(), "requires parent and child CVS ids") {
		t.Fatalf("expected requires parent/child error, got: %v", err)
	}
}

func TestNestStatus_Elevation(t *testing.T) {
	t.Parallel()

	// 1. parentID empty
	res, err := NestStatus("", 3, nil)
	if err == nil || res != nil {
		t.Fatalf("expected error on empty parentID, got res=%v, err=%v", res, err)
	}

	// 2. maxDepth <= 0 defaults to DefaultOverseerCVSTreeMaxDepth
	res, err = NestStatus("CVS-p", 0, func(id string) ([]string, string, string, error) {
		return nil, "active", "c1", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.MaxDepth != DefaultOverseerCVSTreeMaxDepth {
		t.Fatalf("expected maxDepth=%d, got %d", DefaultOverseerCVSTreeMaxDepth, res.MaxDepth)
	}

	// 3. loader error returns error
	boomErr := errors.New("loader boom")
	res, err = NestStatus("CVS-p", 2, func(id string) ([]string, string, string, error) {
		return nil, "", "", boomErr
	})
	if err == nil || !strings.Contains(err.Error(), "loader boom") {
		t.Fatalf("expected loader boom error, got: %v", err)
	}
}

func TestDetectCVSRefCycle_Elevation(t *testing.T) {
	t.Parallel()

	dummyLoader := func(id string) ([]string, error) {
		return nil, nil
	}

	// 1. root empty
	cycle, ok := DetectCVSRefCycle("", dummyLoader, 3)
	if ok || cycle != nil {
		t.Fatalf("expected no cycle for empty root, got ok=%v, cycle=%v", ok, cycle)
	}

	// 2. refsFor nil
	cycle, ok = DetectCVSRefCycle("CVS-1", nil, 3)
	if ok || cycle != nil {
		t.Fatalf("expected no cycle for nil refsFor, got ok=%v, cycle=%v", ok, cycle)
	}

	// 3. maxDepth < 0
	cycle, ok = DetectCVSRefCycle("CVS-1", dummyLoader, -1)
	if ok || cycle != nil {
		t.Fatalf("expected no cycle for negative maxDepth, got ok=%v, cycle=%v", ok, cycle)
	}

	// 4. Non-CVS or empty refs are skipped
	cycle, ok = DetectCVSRefCycle("CVS-1", func(id string) ([]string, error) {
		if id == "CVS-1" {
			return []string{"", "OTHER-123", "CVS-2"}, nil
		}
		return nil, nil
	}, 3)
	if ok || cycle != nil {
		t.Fatalf("expected no cycle when non-CVS refs present, got ok=%v, cycle=%v", ok, cycle)
	}

	// 5. refsFor returns error
	cycle, ok = DetectCVSRefCycle("CVS-1", func(id string) ([]string, error) {
		return nil, errors.New("lookup failed")
	}, 3)
	if ok || cycle != nil {
		t.Fatalf("expected no cycle on lookup error, got ok=%v, cycle=%v", ok, cycle)
	}

	// 6. Deep tree stops at maxDepth without loop
	cycle, ok = DetectCVSRefCycle("CVS-1", func(id string) ([]string, error) {
		return []string{"CVS-2"}, nil
	}, 1)
	if ok || cycle != nil {
		t.Fatalf("expected no cycle when depth limit reached, got ok=%v, cycle=%v", ok, cycle)
	}

	// 7. Multi-hop cycle: 1 -> 2 -> 3 -> 1
	cycle, ok = DetectCVSRefCycle("CVS-1", func(id string) ([]string, error) {
		switch id {
		case "CVS-1":
			return []string{"CVS-2"}, nil
		case "CVS-2":
			return []string{"CVS-3"}, nil
		case "CVS-3":
			return []string{"CVS-1"}, nil
		default:
			return nil, nil
		}
	}, 5)
	if !ok || len(cycle) != 4 || cycle[0] != "CVS-1" || cycle[3] != "CVS-1" {
		t.Fatalf("expected 4-element cycle ending in CVS-1, got: %v", cycle)
	}
}

func TestRollupStatus_ElevationEdgeCases(t *testing.T) {
	t.Parallel()

	// 1. delta trending away
	tbDelta := TestBundleInput{
		ReadyForSessionCompletion: true,
		DeltaAssessment:           "trending_away",
	}
	status, blockers, ready, _ := ComputeRollupStatus(tbDelta, 0, 0, nil, "", nil)
	if status != RollupStatusBlocked || ready {
		t.Fatalf("expected RollupStatusBlocked and ready=false, got status=%v, ready=%v", status, ready)
	}
	foundDelta := false
	for _, b := range blockers {
		if b.Code == "delta_trending_away" {
			foundDelta = true
		}
	}
	if !foundDelta {
		t.Fatalf("expected delta_trending_away blocker, got %v", blockers)
	}

	// 2. zqkEnvGateExit != 0
	tbNormal := TestBundleInput{ReadyForSessionCompletion: true}
	status, blockers, ready, _ = ComputeRollupStatus(tbNormal, 0, 1, nil, "", nil)
	if status != RollupStatusBlocked || ready {
		t.Fatalf("expected RollupStatusBlocked, got %v", status)
	}
	foundZE := false
	for _, b := range blockers {
		if b.Code == "zqk_env_literals_gate" {
			foundZE = true
		}
	}
	if !foundZE {
		t.Fatalf("expected zqk_env_literals_gate blocker, got %v", blockers)
	}

	// 3. matrixErr != ""
	status, blockers, ready, _ = ComputeRollupStatus(tbNormal, 0, 0, nil, "matrix read error", nil)
	if status != RollupStatusBlocked || ready {
		t.Fatalf("expected RollupStatusBlocked for matrixErr, got %v", status)
	}
	foundMatrix := false
	for _, b := range blockers {
		if b.Code == "vetting_matrix" && b.Detail == "matrix read error" {
			foundMatrix = true
		}
	}
	if !foundMatrix {
		t.Fatalf("expected vetting_matrix blocker, got %v", blockers)
	}

	// 4. child with status error
	children := []ChildSessionInput{
		{ID: "CVS-child-err", Status: objects.ObjectStatusError, Blockers: []string{"generic error"}},
	}
	status, blockers, ready, _ = ComputeRollupStatus(tbNormal, 0, 0, nil, "", children)
	if status != RollupStatusBlocked || ready {
		t.Fatalf("expected RollupStatusBlocked for child status error, got %v", status)
	}

	// 5. child with "not_convergence_session" in blocker detail
	childrenNotCVS := []ChildSessionInput{
		{ID: "CVS-child-not", Status: objects.ObjectStatusActive, Blockers: []string{"target is not_convergence_session object"}},
	}
	status, blockers, ready, _ = ComputeRollupStatus(tbNormal, 0, 0, nil, "", childrenNotCVS)
	if status != RollupStatusBlocked || ready {
		t.Fatalf("expected RollupStatusBlocked for not_convergence_session blocker, got %v", status)
	}
}

func TestPrimaryMeasurementOutcome_Elevation(t *testing.T) {
	t.Parallel()

	// 1. ambiguousPartialBlockers with empty codes
	if ambiguousPartialBlockers(nil) {
		t.Fatalf("expected false for nil codes")
	}
	if ambiguousPartialBlockers([]string{}) {
		t.Fatalf("expected false for empty codes")
	}

	// 2. ambiguousPartialBlockers with soft codes only
	if !ambiguousPartialBlockers([]string{"bundle_gate_not_ready", "gates_skipped"}) {
		t.Fatalf("expected true for only soft codes")
	}

	// 3. ambiguousPartialBlockers with mixed codes
	if ambiguousPartialBlockers([]string{"bundle_gate_not_ready", "some_hard_blocker"}) {
		t.Fatalf("expected false for mixed codes")
	}

	// 4. Unknown/unclassified RollupStatus
	outcome, reason := ComputePrimaryMeasurementOutcome(RollupStatus("unknown_status"), nil, "", 0, 0, false)
	if outcome != MeasurementYieldsAmbiguousOutcome || reason != "unclassified" {
		t.Fatalf("expected unclassified ambiguous outcome, got outcome=%s, reason=%s", outcome, reason)
	}

	// 5. Blocked status with test_bundles_failing
	outcome, reason = ComputePrimaryMeasurementOutcome(
		RollupStatusBlocked,
		[]Blocker{{Code: "test_bundles_failing", Detail: "failing"}},
		"", 0, 0, false,
	)
	if outcome != MeasurementYieldsDivergence || reason != "bundle_health_bad_signal" {
		t.Fatalf("expected bundle_health_bad_signal, got %s: %s", outcome, reason)
	}

	// 6. Blocked status with delta_trending_away
	outcome, reason = ComputePrimaryMeasurementOutcome(
		RollupStatusBlocked,
		[]Blocker{{Code: "delta_trending_away", Detail: "trending"}},
		"", 0, 0, false,
	)
	if outcome != MeasurementYieldsDivergence || reason != "bundle_health_bad_signal" {
		t.Fatalf("expected bundle_health_bad_signal, got %s: %s", outcome, reason)
	}
}

func TestOverseerAndRollup_ConcurrencyAndRace(t *testing.T) {
	t.Parallel()

	const iterations = 50
	var wg sync.WaitGroup

	tree := []CVSTreeNode{
		{ID: "CVS-coord", Depth: 0, Status: "active", CurrentPhase: "c1"},
		{ID: "CVS-child1", Depth: 1, Status: "active", CurrentPhase: "c2"},
		{ID: "CVS-child2", Depth: 1, Status: "completed", CurrentPhase: "c3"},
	}

	rollup := map[string]any{
		"recommended_next_action":     "Execute step 1",
		"ready_for_parent_completion": false,
		"rollup_status":               "partial",
	}

	for i := 0; i < iterations; i++ {
		wg.Add(4)

		goroutinelabels.NewGoroutine("test_active_child_ids", "inspect active children").StartSimple(func() {
			defer wg.Done()
			active := ActiveChildIDsUnderCoordinator(tree)
			if len(active) != 1 || active[0] != "CVS-child1" {
				t.Errorf("unexpected active child ids: %v", active)
			}
		})

		goroutinelabels.NewGoroutine("test_arbitrated_parent", "build arbitrated parent message").StartSimple(func() {
			defer wg.Done()
			msg := BuildArbitratedParentMessage(rollup, tree, "CVS-coord")
			if !strings.Contains(msg, "CVS-child1") || !strings.Contains(msg, "Execute step 1") {
				t.Errorf("unexpected message: %s", msg)
			}
		})

		goroutinelabels.NewGoroutine("test_compute_rollup", "compute rollup status concurrently").StartSimple(func() {
			defer wg.Done()
			tb := TestBundleInput{ReadyForSessionCompletion: true}
			st, _, _, _ := ComputeRollupStatus(tb, 0, 0, nil, "", nil)
			if st != RollupStatusSatisfied {
				t.Errorf("unexpected status: %s", st)
			}
		})

		goroutinelabels.NewGoroutine("test_detect_cycle", "detect cycle concurrently").StartSimple(func() {
			defer wg.Done()
			_, _ = DetectCVSRefCycle("CVS-coord", func(id string) ([]string, error) {
				return []string{"CVS-child1"}, nil
			}, 3)
		})
	}

	wg.Wait()
}
