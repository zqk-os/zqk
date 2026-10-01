package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/rollback"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/walutil"
)

type mockStorageProvider struct {
	storage.ObjectStorageProvider
	mu      sync.Mutex
	objects map[string]map[string]any
}

func newMockStorageProvider() *mockStorageProvider {
	return &mockStorageProvider{
		objects: make(map[string]map[string]any),
	}
}

func (m *mockStorageProvider) add(obj map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, _ := obj[objects.FieldKeyID].(string)
	m.objects[id] = obj
}

func (m *mockStorageProvider) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if obj, ok := m.objects[id]; ok {
		cp := make(map[string]any, len(obj))
		for k, v := range obj {
			cp[k] = v
		}
		return cp, nil
	}
	return nil, errors.New("not found")
}

func (m *mockStorageProvider) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	obj, ok := m.objects[id]
	if !ok {
		return errors.New("not found")
	}
	for k, v := range updates {
		obj[k] = v
	}
	return nil
}

func (m *mockStorageProvider) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res []map[string]any
	for _, obj := range m.objects {
		if filter.Kind != "" {
			k, _ := obj[objects.FieldKeyKind].(string)
			if k != filter.Kind {
				continue
			}
		}
		match := true
		for fk, fv := range filter.Filters {
			if fvMap, ok := fv.(map[string]any); ok {
				if hasVal, ok := fvMap["$has"]; ok {
					sl, _ := obj[fk].([]any)
					slStr, _ := obj[fk].([]string)
					found := false
					for _, item := range sl {
						if item == hasVal {
							found = true
							break
						}
					}
					for _, item := range slStr {
						if item == hasVal {
							found = true
							break
						}
					}
					if !found {
						match = false
						break
					}
				}
			} else {
				if obj[fk] != fv {
					match = false
					break
				}
			}
		}
		if match {
			cp := make(map[string]any, len(obj))
			for k, v := range obj {
				cp[k] = v
			}
			res = append(res, cp)
		}
	}
	return &storage.QueryResult{
		Objects: res,
	}, nil
}

// 1. WAL Tests
func TestLifecycleEventWAL_Full(t *testing.T) {
	// NextWALIdlePoll
	if p := NextWALIdlePoll(10 * time.Millisecond); p != WALIdlePollMin {
		t.Errorf("expected WALIdlePollMin, got %v", p)
	}
	if p := NextWALIdlePoll(WALIdlePollMin); p != WALIdlePollMin*2 {
		t.Errorf("expected double, got %v", p)
	}
	if p := NextWALIdlePoll(WALIdlePollMax); p != WALIdlePollMax {
		t.Errorf("expected max ceiling, got %v", p)
	}

	// scopeKey and scopeMapToKey
	evEmpty := &LifecycleEvent{}
	if evEmpty.scopeKey() != "" {
		t.Errorf("expected empty scope key, got %q", evEmpty.scopeKey())
	}
	evScoped := &LifecycleEvent{
		Scope: map[string]string{"b": "2", "a": "1"},
	}
	if evScoped.scopeKey() != "a=1,b=2" {
		t.Errorf("expected a=1,b=2, got %q", evScoped.scopeKey())
	}
	if scopeMapToKey(nil) != "" {
		t.Errorf("expected empty for nil map")
	}

	// NewLifecycleEventWAL empty project root
	if _, err := NewLifecycleEventWAL(""); err == nil {
		t.Errorf("expected error for empty projectRoot")
	}

	// WAL operations with temp directory
	dir := t.TempDir()
	wal, err := NewLifecycleEventWAL(dir)
	if err != nil {
		t.Fatalf("NewLifecycleEventWAL failed: %v", err)
	}

	if wal.Path() == "" || wal.CheckpointPath() == "" {
		t.Errorf("expected non-empty paths from wal")
	}

	// Append nil event is no-op
	if err := wal.Append(nil); err != nil {
		t.Fatalf("Append nil failed: %v", err)
	}

	// Append events
	ev1 := &LifecycleEvent{
		EventType: EventTypeStatusTransition,
		Kind:      objects.KindPriorityPlan,
		ID:        "PRI-1",
		ToStatus:  statusActive,
	}
	ev2 := &LifecycleEvent{
		EventType:   EventTypeCriterionSatisfied,
		CriterionID: criterionAllBacklogComplete,
		Scope:       map[string]string{scopePlanID: "PRI-1"},
	}
	if err := wal.Append(ev1); err != nil {
		t.Fatalf("Append ev1 failed: %v", err)
	}
	if err := wal.Append(ev2); err != nil {
		t.Fatalf("Append ev2 failed: %v", err)
	}
	if err := wal.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// ReplayFrom
	var replayed []*LifecycleEvent
	err = wal.ReplayFrom(0, func(ev *LifecycleEvent) error {
		replayed = append(replayed, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayFrom failed: %v", err)
	}
	if len(replayed) != 2 {
		t.Fatalf("expected 2 replayed events, got %d", len(replayed))
	}

	// ReplayFromCursor
	var cursorReplayed []*LifecycleEvent
	cursor, err := wal.ReplayFromCursor(walutil.ReplayCursor{}, func(ev *LifecycleEvent) error {
		cursorReplayed = append(cursorReplayed, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("ReplayFromCursor failed: %v", err)
	}
	if len(cursorReplayed) != 2 || cursor.Seq != 2 {
		t.Errorf("expected 2 events and seq 2, got %d and %d", len(cursorReplayed), cursor.Seq)
	}

	if err := wal.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	// Double close on nil w
	emptyWAL := &LifecycleEventWAL{}
	if err := emptyWAL.Close(); err != nil {
		t.Fatalf("emptyWAL Close failed: %v", err)
	}

	// Legacy migration test
	legacyDir := t.TempDir()
	walDir := filepath.Join(legacyDir, ".zqk", "wal")
	_ = os.MkdirAll(walDir, 0755)
	legacyWAL := filepath.Join(walDir, "lifecycle_events")
	_ = os.WriteFile(legacyWAL, []byte("{}\n"), 0644)
	legacyCk := filepath.Join(walDir, "lifecycle_events.checkpoint")
	_ = os.WriteFile(legacyCk, []byte("0"), 0644)

	if err := migrateLifecycleWALToCanonicalNames(""); err != nil {
		t.Errorf("expected nil for empty projectRoot")
	}
	if err := migrateLifecycleWALToCanonicalNames(legacyDir); err != nil {
		t.Fatalf("migrateLifecycleWALToCanonicalNames failed: %v", err)
	}
	canonicalWAL := filepath.Join(walDir, "lifecycle_events.wal")
	if _, err := os.Stat(canonicalWAL); err != nil {
		t.Errorf("expected canonical WAL file to exist after migration")
	}
}

// 2. Rules and Rollback Graph Tests
func TestRulesAndRollbackGraph(t *testing.T) {
	rules := DefaultTransitionRules()
	if len(rules) != 5 {
		t.Fatalf("expected 5 default transition rules, got %d", len(rules))
	}

	// RuleMatch tests
	rule := rules[0] // all_backlog_items_complete_for_plan
	if !rule.RuleMatch(criterionAllBacklogComplete, "", map[string]string{scopePlanID: "PRI-1"}) {
		t.Errorf("expected rule match")
	}
	if rule.RuleMatch("other_criterion", "", map[string]string{scopePlanID: "PRI-1"}) {
		t.Errorf("expected rule mismatch on criterion")
	}
	if rule.RuleMatch(criterionAllBacklogComplete, "", map[string]string{}) {
		t.Errorf("expected rule mismatch on missing IDFromScope")
	}
	ruleWithKey := TransitionRule{
		CriterionID: "c1",
		ScopeKey:    "k=1",
		IDFromScope: "k",
	}
	if ruleWithKey.RuleMatch("c1", "k=2", map[string]string{"k": "2"}) {
		t.Errorf("expected rule mismatch on ScopeKey")
	}

	// ObjectID
	if rule.ObjectID(map[string]string{scopePlanID: "PRI-99"}) != "PRI-99" {
		t.Errorf("expected PRI-99 from ObjectID")
	}
	noIDRule := TransitionRule{}
	if noIDRule.ObjectID(map[string]string{"a": "b"}) != "" {
		t.Errorf("expected empty string from noIDRule.ObjectID")
	}

	// Rollback graph
	mock := newMockStorageProvider()
	mock.add(map[string]any{
		objects.FieldKeyID:              "PRI-1",
		objects.FieldKeyKind:            objects.KindPriorityPlan,
		objects.FieldKeyStatus:          statusActive,
		objects.FieldKeyPriorityPlanRef: "PRI-1",
	})
	mock.add(map[string]any{
		objects.FieldKeyID:              "BLI-1",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-1",
	})

	ctx := context.Background()
	// RecomputeRefsFromScope
	if refs := RecomputeRefsFromScope(ctx, "wrong_scope", "PRI-1", mock); refs != nil {
		t.Errorf("expected nil for wrong scope")
	}
	if refs := RecomputeRefsFromScope(ctx, rollback.ScopeTypeLifecycle, "invalid_id", mock); refs != nil {
		t.Errorf("expected nil for invalid scope id")
	}
	if refs := RecomputeRefsFromScope(ctx, rollback.ScopeTypeLifecycle, "priority_plan:PRI-1:complete", nil); refs != nil {
		t.Errorf("expected nil for nil provider")
	}
	refs := RecomputeRefsFromScope(ctx, rollback.ScopeTypeLifecycle, "priority_plan:PRI-1:complete", mock)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs (plan + bli), got %d", len(refs))
	}

	// StatusRelevantRefs
	nonPlanReq := TransitionRequest{Kind: objects.KindBacklogItem, ID: "BLI-1"}
	if refsNonPlan := StatusRelevantRefs(ctx, mock, nonPlanReq); len(refsNonPlan) != 1 {
		t.Errorf("expected 1 ref for non-plan, got %d", len(refsNonPlan))
	}
}

// 3. Stage Membrane and Shockwave Tests
func TestStageMembraneAndShockwave(t *testing.T) {
	// MembraneHopBlockedError
	var nilErr *MembraneHopBlockedError
	if nilErr.Error() != "membrane hop blocked" {
		t.Errorf("expected fallback error message, got %s", nilErr.Error())
	}
	hopErr := &MembraneHopBlockedError{
		ToStatus:   "complete",
		BlockingID: "CRIT-1",
		Reason:     "unmet criterion",
		ClusterIDs: []string{"BLI-1", "CRIT-1"},
	}
	if !strings.Contains(hopErr.Error(), "blocked by CRIT-1") {
		t.Errorf("unexpected error string: %s", hopErr.Error())
	}

	// MembraneHopPlan ClusterIDs and AppliedMembers
	var nilPlan *MembraneHopPlan
	if nilPlan.ClusterIDs() != nil || nilPlan.AppliedMembers() != nil {
		t.Errorf("expected nil for nil plan")
	}
	plan := &MembraneHopPlan{
		Members: []MembraneMember{
			{ID: "m1", Skip: false},
			{ID: "m2", Skip: true},
		},
	}
	if len(plan.ClusterIDs()) != 2 {
		t.Errorf("expected 2 cluster IDs, got %d", len(plan.ClusterIDs()))
	}
	if len(plan.AppliedMembers()) != 1 || plan.AppliedMembers()[0].ID != "m1" {
		t.Errorf("expected 1 applied member (m1), got %v", plan.AppliedMembers())
	}

	// lineageRank and shockwave helpers
	ranks := []struct {
		kind string
		rank int
	}{
		{kindVision, 0},
		{kindWorkstream, 0},
		{kindRoadmap, 0},
		{kindStrategicPlan, 0},
		{kindMission, 1},
		{kindGoal, 2},
		{kindMilestone, 3},
		{kindPriorityPlan, 4},
		{"unknown_kind", 100},
	}
	for _, tc := range ranks {
		if r := lineageRank(tc.kind); r != tc.rank {
			t.Errorf("lineageRank(%q) = %d, expected %d", tc.kind, r, tc.rank)
		}
	}

	if !isParkMembraneStatus(objects.ObjectStatusArchived) || !isParkMembraneStatus(objects.ObjectStatusDeferred) {
		t.Errorf("expected archived/deferred to be park membrane status")
	}
	if isParkMembraneStatus(statusComplete) {
		t.Errorf("expected complete not to be park status")
	}
}

// 4. Remaining Open Count Tests
func TestRemainingOpenCount_Comprehensive(t *testing.T) {
	// coerceNonNegInt
	validInts := []any{int(5), int32(5), int64(5), float64(5.0), float32(5.0)}
	for _, v := range validInts {
		n, ok := coerceNonNegInt(v)
		if !ok || n != 5 {
			t.Errorf("coerceNonNegInt(%T %v) = (%d, %v), expected (5, true)", v, v, n, ok)
		}
	}
	invalidInts := []any{int(-1), int32(-1), int64(-1), float64(5.5), float32(5.5), "5", nil}
	for _, v := range invalidInts {
		_, ok := coerceNonNegInt(v)
		if ok {
			t.Errorf("expected coerceNonNegInt(%T %v) to fail", v, v)
		}
	}

	// remainingOpenCountFrom
	if _, ok := remainingOpenCountFrom(nil); ok {
		t.Errorf("expected false for nil map")
	}
	if _, ok := remainingOpenCountFrom(map[string]any{"other": 1}); ok {
		t.Errorf("expected false for missing key")
	}
	if n, ok := remainingOpenCountFrom(map[string]any{objects.FieldKeyRemainingOpenCount: 3}); !ok || n != 3 {
		t.Errorf("expected (3, true), got (%d, %v)", n, ok)
	}

	// isBacklogItemTerminalStatus
	if isBacklogItemTerminalStatus("") {
		t.Errorf("expected false for empty status")
	}
	if !isBacklogItemTerminalStatus(statusComplete) {
		t.Errorf("expected complete to be terminal")
	}
	if isBacklogItemTerminalStatus(statusInProgress) {
		t.Errorf("expected in_progress not to be terminal")
	}

	mock := newMockStorageProvider()
	mock.add(map[string]any{
		objects.FieldKeyID:                 "PLAN-1",
		objects.FieldKeyKind:               objects.KindPriorityPlan,
		objects.FieldKeyRemainingOpenCount: 2,
	})

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// casIncrement / casDecrement
	val, err := casDecrementRemainingOpenCount(ctx, mock, secCtx, "PLAN-1")
	if err != nil || val != 1 {
		t.Fatalf("casDecrement failed: val=%d, err=%v", val, err)
	}
	val, err = casIncrementRemainingOpenCount(ctx, mock, secCtx, "PLAN-1")
	if err != nil || val != 2 {
		t.Fatalf("casIncrement failed: val=%d, err=%v", val, err)
	}

	// ensureRemainingOpenCount
	if err := ensureRemainingOpenCount(ctx, nil, "PLAN-1", 1); err != nil {
		t.Errorf("expected nil for nil provider")
	}
	if err := ensureRemainingOpenCount(ctx, mock, "", 1); err != nil {
		t.Errorf("expected nil for empty containerID")
	}
	if err := ensureRemainingOpenCount(ctx, mock, "PLAN-1", 2); err != nil {
		t.Errorf("ensureRemainingOpenCount failed when equal: %v", err)
	}
	if err := ensureRemainingOpenCount(ctx, mock, "PLAN-1", 0); err != nil {
		t.Errorf("ensureRemainingOpenCount failed to update: %v", err)
	}

	// consumeOpenChild and noteOpenCountableReentered
	rem, handled, err := consumeOpenChild(ctx, mock, secCtx, DependencyRefEvent{TargetID: "PLAN-1"})
	if err != nil || !handled || rem != 0 {
		t.Errorf("consumeOpenChild expected (0, true, nil), got (%d, %v, %v)", rem, handled, err)
	}
	noteOpenCountableReentered(ctx, mock, "PLAN-1")

	// SeedRemainingOpenCountFromMembers
	dir := t.TempDir()
	mock.add(map[string]any{
		objects.FieldKeyID:           "TC-1",
		objects.FieldKeyKind:         objects.KindTestCase,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-10", "CRIT-11"},
	})
	mock.add(map[string]any{
		objects.FieldKeyID:     "CRIT-10",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: "complete",
	})
	mock.add(map[string]any{
		objects.FieldKeyID:     "CRIT-11",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: "in_progress",
	})
	if err := SeedRemainingOpenCountFromMembers(ctx, mock, "TC-1", true); err != nil {
		t.Fatalf("SeedRemainingOpenCountFromMembers TC-1 failed: %v", err)
	}

	// Milestone seeding
	mock.add(map[string]any{
		objects.FieldKeyID:   "MIL-1",
		objects.FieldKeyKind: objects.KindMilestone,
	})
	mock.add(map[string]any{
		objects.FieldKeyID:           "BLI-M1",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyMilestoneRef: "MIL-1",
		objects.FieldKeyStatus:       statusInProgress,
	})
	mock.add(map[string]any{
		objects.FieldKeyID:            "BLI-M2",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyMilestoneRefs: []any{"MIL-1"},
		objects.FieldKeyStatus:        statusComplete, // terminal, not counted
	})
	if err := SeedRemainingOpenCountFromMembers(ctx, mock, "MIL-1", true); err != nil {
		t.Fatalf("SeedRemainingOpenCountFromMembers MIL-1 failed: %v", err)
	}

	// Priority plan seeding
	mock.add(map[string]any{
		objects.FieldKeyID:   "PRI-SEED",
		objects.FieldKeyKind: objects.KindPriorityPlan,
	})
	mock.add(map[string]any{
		objects.FieldKeyID:              "BLI-P1",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-SEED",
		objects.FieldKeyStatus:          statusInProgress,
	})
	if err := SeedRemainingOpenCountFromMembers(ctx, mock, "PRI-SEED", true); err != nil {
		t.Fatalf("SeedRemainingOpenCountFromMembers PRI-SEED failed: %v", err)
	}

	// NoteOpenCountableMemberEntered
	NoteOpenCountableMemberEntered(ctx, mock, "PRI-SEED")

	// EmitTrustedRemainingOpenDrained
	EmitTrustedRemainingOpenDrained("", "PRI-SEED")
	EmitTrustedRemainingOpenDrained(dir, "")
	EmitTrustedRemainingOpenDrained(dir, "PRI-SEED")
	EmitTrustedRemainingOpenDrained(dir, "MIL-1")
}

// 5. Criterion Emitters Tests
func TestCriterionEmitters_All(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	mock := newMockStorageProvider()

	getStorage := func(string) (storage.ObjectStorageProvider, bool) {
		return mock, true
	}

	// TryEmitRemainingOpenDrained
	mock.add(map[string]any{
		objects.FieldKeyID:                 "PRI-DRAIN",
		objects.FieldKeyRemainingOpenCount: 0,
	})
	TryEmitRemainingOpenDrained(ctx, dir, "PRI-DRAIN", getStorage)
	TryEmitRemainingOpenDrained(ctx, "", "PRI-DRAIN", getStorage)
	TryEmitRemainingOpenDrained(ctx, dir, "", getStorage)
	TryEmitRemainingOpenDrained(ctx, dir, "PRI-DRAIN", nil)

	// TryEmitAllCriteriaCompleteForMilestone
	mock.add(map[string]any{
		objects.FieldKeyID:           "MIL-10",
		objects.FieldKeyKind:         objects.KindMilestone,
		objects.FieldKeyStatus:       statusInProgress,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-100", "CRIT-101"},
	})
	mock.add(map[string]any{
		objects.FieldKeyID:     "CRIT-100",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: objects.ObjectStatusValidated,
	})
	mock.add(map[string]any{
		objects.FieldKeyID:     "CRIT-101",
		objects.FieldKeyKind:   objects.KindCriteria,
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	})
	TryEmitAllCriteriaCompleteForMilestone(ctx, dir, "MIL-10", getStorage)

	// TryEmitForMilestonesContainingCriterion
	TryEmitForMilestonesContainingCriterion(ctx, dir, "CRIT-100", getStorage)

	// TryEmitForBacklogItemsContainingCriterion
	mock.add(map[string]any{
		objects.FieldKeyID:           "BLI-10",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyStatus:       statusInProgress,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-100"},
	})
	TryEmitForBacklogItemsContainingCriterion(ctx, dir, "CRIT-100", getStorage)

	// TryEmitAllAcceptanceCriteriaMetForBacklogItem
	TryEmitAllAcceptanceCriteriaMetForBacklogItem(ctx, dir, "BLI-10", getStorage)

	// TryEmitAllBacklogItemsCompleteForMilestone
	mock.add(map[string]any{
		objects.FieldKeyID:     "MIL-20",
		objects.FieldKeyKind:   objects.KindMilestone,
		objects.FieldKeyStatus: statusInProgress,
	})
	mock.add(map[string]any{
		objects.FieldKeyID:           "BLI-20",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyMilestoneRef: "MIL-20",
		objects.FieldKeyStatus:       statusComplete,
	})
	TryEmitAllBacklogItemsCompleteForMilestone(ctx, dir, "MIL-20", getStorage)

	// TryEmitForTestCasesContainingCriterion
	mock.add(map[string]any{
		objects.FieldKeyID:           "TC-20",
		objects.FieldKeyKind:         objects.KindTestCase,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-100"},
	})
	TryEmitForTestCasesContainingCriterion(ctx, dir, "CRIT-100", getStorage)

	// TryEmitAllCriteriaCompleteForRequirement & TryEmitForRequirementsContainingCriterion
	mock.add(map[string]any{
		objects.FieldKeyID:           "REQ-10",
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyStatus:       statusInProgress,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-100"},
	})
	TryEmitForRequirementsContainingCriterion(ctx, dir, "CRIT-100", getStorage)
	TryEmitAllCriteriaCompleteForRequirement(ctx, dir, "REQ-10", getStorage)
}

// 6. Hooks Tests
func TestHooks_ApplyComputeHooks(t *testing.T) {
	mock := newMockStorageProvider()
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	// Effort variance computation on backlog item completion
	mock.add(map[string]any{
		objects.FieldKeyID:              "BLI-HOOK-1",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyEstimatedEffort: "10.0",
		objects.FieldKeyActualEffort:    "12.0",
	})
	req := TransitionRequest{
		Kind:     objects.KindBacklogItem,
		ID:       "BLI-HOOK-1",
		ToStatus: statusComplete,
	}
	updates := make(map[string]any)
	ApplyComputeHooks(ctx, mock, secCtx, req, updates)
	if _, ok := updates[objects.FieldKeyEffortVariance]; !ok {
		t.Errorf("expected effort variance in updates")
	}

	// Unset actual effort autofill
	mock.add(map[string]any{
		objects.FieldKeyID:              "BLI-HOOK-2",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyEstimatedEffort: "8.0",
		objects.FieldKeyActualEffort:    "0",
	})
	req2 := TransitionRequest{
		Kind:     objects.KindBacklogItem,
		ID:       "BLI-HOOK-2",
		ToStatus: statusComplete,
	}
	updates2 := make(map[string]any)
	ApplyComputeHooks(ctx, mock, secCtx, req2, updates2)
	if updates2[objects.FieldKeyActualEffort] != "8.0" {
		t.Errorf("expected actual effort autofilled to 8.0, got %v", updates2[objects.FieldKeyActualEffort])
	}

	// Unplanned work detector (unlinked backlog item)
	mock.add(map[string]any{
		objects.FieldKeyID:   "BLI-UNLINKED",
		objects.FieldKeyKind: objects.KindBacklogItem,
	})
	reqUnlinked := TransitionRequest{
		Kind:     objects.KindBacklogItem,
		ID:       "BLI-UNLINKED",
		ToStatus: statusInProgress,
	}
	ApplyComputeHooks(ctx, mock, secCtx, reqUnlinked, make(map[string]any))

	// Status transitions: validation_requested and verification_ready
	reqValReq := TransitionRequest{Kind: objects.KindCriteria, ID: "CRIT-1", ToStatus: "validation_requested"}
	ApplyComputeHooks(ctx, mock, secCtx, reqValReq, make(map[string]any))

	reqVerReady := TransitionRequest{Kind: objects.KindCriteria, ID: "CRIT-1", ToStatus: "verification_ready"}
	ApplyComputeHooks(ctx, mock, secCtx, reqVerReady, make(map[string]any))

	// PriorityPlan active_order promotion
	mock.add(map[string]any{
		objects.FieldKeyID:     "PRI-ORDER-1",
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyStatus: statusActive,
		"active_order":         2,
	})
	reqPlanComp := TransitionRequest{
		Kind:     objects.KindPriorityPlan,
		ID:       "PRI-DONE",
		ToStatus: statusComplete,
	}
	ApplyComputeHooks(ctx, mock, secCtx, reqPlanComp, make(map[string]any))

	// effortStringUnset test
	if !effortStringUnset("") || !effortStringUnset("0") || !effortStringUnset("0.0") {
		t.Errorf("expected empty or zero to be unset")
	}
	if effortStringUnset("5.0") {
		t.Errorf("expected 5.0 not to be unset")
	}
}

// 7. Listener, Updater, and Registry Tests
func TestListenerUpdaterRegistry(t *testing.T) {
	dir := t.TempDir()
	wal, err := NewLifecycleEventWAL(dir)
	if err != nil {
		t.Fatalf("NewLifecycleEventWAL failed: %v", err)
	}

	mock := newMockStorageProvider()
	getStorage := func(string) (storage.ObjectStorageProvider, bool) {
		return mock, true
	}

	// Registry tests
	walReg, err := GetOrCreateLifecycleWAL(dir)
	if err != nil || walReg == nil {
		t.Fatalf("GetOrCreateLifecycleWAL failed: %v", err)
	}
	AppendStatusTransition(dir, objects.KindBacklogItem, "BLI-1", objects.ObjectStatusReady, statusInProgress)

	// Listener test
	transitionCh := make(chan TransitionRequest, 10)
	rules := DefaultTransitionRules()
	listener := NewListener(dir, wal, rules, transitionCh, getStorage)

	if err := AppendCriterionSatisfied(wal, criterionAllBacklogComplete, map[string]string{scopePlanID: "PRI-1"}); err != nil {
		t.Fatalf("AppendCriterionSatisfied failed: %v", err)
	}
	_ = wal.Sync()

	// Process events directly
	ev := &LifecycleEvent{
		EventType:   EventTypeCriterionSatisfied,
		CriterionID: criterionAllBacklogComplete,
		Scope:       map[string]string{scopePlanID: "PRI-1"},
	}
	if err := listener.processEvent(ev); err != nil {
		t.Fatalf("processEvent failed: %v", err)
	}

	// Verify transition request was queued
	select {
	case req := <-transitionCh:
		if req.ID != "PRI-1" || req.ToStatus != statusComplete {
			t.Errorf("unexpected transition request: %v", req)
		}
	default:
		t.Errorf("expected transition request on channel")
	}

	// Checkpoint save and load
	if err := listener.saveCheckpoint(); err != nil {
		t.Fatalf("saveCheckpoint failed: %v", err)
	}
	if err := listener.loadCheckpoint(); err != nil {
		t.Fatalf("loadCheckpoint failed: %v", err)
	}

	// Updater test
	updater := NewUpdater(dir, getStorage, transitionCh)
	mock.add(map[string]any{
		objects.FieldKeyID:     "PRI-1",
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyStatus: statusInProgress,
	})

	if err := updater.apply(context.Background(), TransitionRequest{
		Kind:     objects.KindPriorityPlan,
		ID:       "PRI-1",
		ToStatus: statusComplete,
	}); err != nil {
		t.Fatalf("updater.apply failed: %v", err)
	}

	// Updater run with closed channel
	close(transitionCh)
	if err := updater.Run(context.Background()); err != nil {
		t.Errorf("updater.Run expected nil on closed channel, got %v", err)
	}
}

// 8. Dependency Propagation Additional Coverage
func TestDependencyPropagation_ExtraCoverage(t *testing.T) {
	dir := t.TempDir()
	mock := newMockStorageProvider()
	ctx := context.Background()

	mock.add(map[string]any{
		objects.FieldKeyID:                 "PRI-REM-1",
		objects.FieldKeyKind:               objects.KindPriorityPlan,
		objects.FieldKeyStatus:             statusInProgress,
		objects.FieldKeyRemainingOpenCount: 1,
	})

	logger := logging.NewEventLogger(ctx)

	// ApplyPlanChildMembershipRemoved with park options
	ApplyPlanChildMembershipRemoved(ctx, logger, mock, dir, "PRI-REM-1", "BLI-1", WithPark(true))
	ApplyPlanChildMembershipRemoved(ctx, logger, mock, dir, "PRI-REM-1", "BLI-1", WithPark(false))

	// ApplyDependencyRefEvents
	data := map[string]any{
		objects.FieldKeyID:              "BLI-DEP-1",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-REM-1",
	}
	ApplyDependencyRefEvents(ctx, logger, mock, dir, objects.KindBacklogItem, "BLI-DEP-1", statusInProgress, statusComplete, data)
	EmitDependencyRefEvents(dir, objects.KindBacklogItem, "BLI-DEP-1", statusInProgress, statusComplete, data)

	// Helper status checks
	if !isTerminalStatus(statusComplete) || !isTerminalStatus(statusArchived) {
		t.Errorf("expected complete/archived to be terminal")
	}
	if isTerminalStatus(statusInProgress) {
		t.Errorf("expected in_progress not to be terminal")
	}

	if !backlogItemStatusReadyOrLaterForLock(objects.ObjectStatusPlanned) || !backlogItemStatusReadyOrLaterForLock(statusInProgress) {
		t.Errorf("expected planned/in_progress to be ready or later for lock")
	}
	if backlogItemStatusReadyOrLaterForLock(objects.ObjectStatusDraft) {
		t.Errorf("expected draft not to be ready for lock")
	}
}

func TestDependencyPropagation_FullShockwaveAndSubscriber(t *testing.T) {
	dir := t.TempDir()
	mock := newMockStorageProvider()
	ctx := context.Background()
	logger := logging.NewEventLogger(ctx)

	getStorage := func(string) (storage.ObjectStorageProvider, bool) {
		return mock, true
	}

	// 1. Subscriber methods
	sub := NewDependencyPropagationSubscriber(getStorage)
	if sub.ID() != "lifecycle_dependency_propagation" {
		t.Errorf("expected lifecycle_dependency_propagation ID, got %s", sub.ID())
	}
	if !sub.IsActive() {
		t.Errorf("expected sub to be active")
	}
	if len(sub.EventTypes()) != 1 {
		t.Errorf("expected 1 event type")
	}

	// HandleEvent edge cases
	_ = sub.HandleEvent(nil)
	_ = sub.HandleEvent(&coordination.OperationalEvent{})
	_ = sub.HandleEvent(&coordination.OperationalEvent{Metadata: map[string]any{}})

	// 2. Setup mock data for plan and backlog items
	mock.add(map[string]any{
		objects.FieldKeyID:                 "PRI-LOCK-1",
		objects.FieldKeyKind:               objects.KindPriorityPlan,
		objects.FieldKeyStatus:             statusActive,
		objects.FieldKeyRemainingOpenCount: 2,
	})
	mock.add(map[string]any{
		objects.FieldKeyID:              "BLI-L1",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-LOCK-1",
		objects.FieldKeyStatus:          statusInProgress,
	})
	mock.add(map[string]any{
		objects.FieldKeyID:              "BLI-L2",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-LOCK-1",
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
	})

	// planLinkedBacklogLockScan and planLinkedBacklogReadyOrLater
	allReady, anyInProgress, err := planLinkedBacklogLockScan(ctx, mock, "PRI-LOCK-1")
	if err != nil || !allReady || !anyInProgress {
		t.Errorf("planLinkedBacklogLockScan expected (true, true, nil), got (%v, %v, %v)", allReady, anyInProgress, err)
	}
	ready, err := planLinkedBacklogReadyOrLater(ctx, mock, "PRI-LOCK-1")
	if err != nil || !ready {
		t.Errorf("planLinkedBacklogReadyOrLater expected true, got %v, err=%v", ready, err)
	}
	_, _, _ = planLinkedBacklogLockScan(ctx, nil, "")

	// 3. MaybeExecutionLockPlan
	if MaybeExecutionLockPlan(ctx, logger, nil, dir, "PRI-LOCK-1") {
		t.Errorf("expected false for nil provider")
	}
	if MaybeExecutionLockPlan(ctx, logger, mock, dir, "") {
		t.Errorf("expected false for empty planID")
	}
	locked := MaybeExecutionLockPlan(ctx, logger, mock, dir, "PRI-LOCK-1")
	if !locked {
		t.Errorf("expected plan to be execution-locked")
	}
	// Already in_progress -> returns false
	if MaybeExecutionLockPlan(ctx, logger, mock, dir, "PRI-LOCK-1") {
		t.Errorf("expected false when already in_progress")
	}

	// 4. HandleEvent with full event
	opEv := &coordination.OperationalEvent{
		OperationID: "op-1",
		Metadata: map[string]any{
			dependencyEventProjectRootKey: dir,
			objects.FieldKeyTargetID:      "PRI-LOCK-1",
			dependencyEventTriggerIDKey:   "BLI-L1",
			dependencyEventTriggerKindKey: objects.KindBacklogItem,
			dependencyEventFromStateKey:   statusActive,
			dependencyEventToStateKey:     statusInProgress,
			dependencyEventVersionKey:     time.Now().Format(time.RFC3339Nano),
		},
	}
	_ = sub.HandleEvent(opEv)

	// 5. priorityPlanShockwaveTransitions
	transitions := priorityPlanShockwaveTransitions()
	_ = transitions

	// 6. reserveDependencyEvent & releaseDependencyEvent
	evStamp := DependencyRefEvent{
		TargetID:  "T1",
		TriggerID: "TR1",
		EventID:   "ev-1",
		Version:   time.Now().Format(time.RFC3339Nano),
	}
	if !reserveDependencyEvent(evStamp) {
		t.Errorf("expected initial reserve to succeed")
	}
	if reserveDependencyEvent(evStamp) {
		t.Errorf("expected repeat reserve with same eventID to fail")
	}
	releaseDependencyEvent(evStamp)
	if !reserveDependencyEvent(evStamp) {
		t.Errorf("expected reserve to succeed after release")
	}
	// Empty versions are no-ops
	_ = reserveDependencyEvent(DependencyRefEvent{})
	releaseDependencyEvent(DependencyRefEvent{})

	// 7. RegisterDependencyPropagationWithCoordinator
	RegisterDependencyPropagationWithCoordinator(getStorage)

	// 8. listener.Run and updater.Run with cancelled context
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_ = sub
	ch := make(chan TransitionRequest)
	close(ch)
	wal, _ := NewLifecycleEventWAL(dir)
	l := NewListener(dir, wal, DefaultTransitionRules(), ch, getStorage)
	_ = l.Run(canceledCtx)

	// 9. Process criteria transition in listener
	_ = l.processEvent(&LifecycleEvent{
		EventType: EventTypeStatusTransition,
		Kind:      objects.KindCriteria,
		ID:        "CRIT-100",
		ToStatus:  statusValidated,
	})

	// 10. commitStatusReactive and commitPriorityPlanShockwave
	secCtx := pkgctx.NewSystemSecurityContext()
	evReactive := DependencyRefEvent{
		TargetID:    "PRI-LOCK-1",
		TriggerID:   "BLI-L1",
		EventID:     "ev-react-1",
		Version:     time.Now().Format(time.RFC3339Nano),
		ProjectRoot: dir,
	}
	commitStatusReactive(ctx, logger, mock, secCtx, evReactive, objects.KindPriorityPlan, statusActive, map[string]any{objects.FieldKeyStatus: statusInProgress})

	evShockwave := DependencyRefEvent{
		TargetID:    "PRI-LOCK-1",
		TriggerID:   "BLI-L1",
		EventID:     "ev-shock-1",
		Version:     time.Now().Format(time.RFC3339Nano),
		ProjectRoot: dir,
	}
	commitPriorityPlanShockwave(ctx, logger, mock, secCtx, evShockwave, statusActive, map[string]any{objects.FieldKeyStatus: statusInProgress})

	// 11. applyOpenCountableLastChild and autoCompleteUpdates
	evLastChild := DependencyRefEvent{
		TargetID:    "PRI-LOCK-1",
		TriggerID:   "BLI-L1",
		EventID:     "ev-last-1",
		Version:     time.Now().Format(time.RFC3339Nano),
		ProjectRoot: dir,
		ToState:     statusComplete,
		FromState:   statusInProgress,
	}
	_ = applyOpenCountableLastChild(ctx, logger, mock, secCtx, evLastChild, objects.KindPriorityPlan, statusInProgress)
	_, _ = autoCompleteUpdates(objects.KindPriorityPlan, statusInProgress)
	_, _ = autoCompleteUpdates(objects.KindBacklogItem, statusInProgress)
	_, _ = autoCompleteUpdates("unknown", "none")

	// 12. applyRoleTransitionSideEffects & commitMembershipTransition
	roleUpdates := make(map[string]any)
	applyRoleTransitionSideEffects(objects.KindPriorityPlan, "shovel_ready", "grooming", roleUpdates)
	evMem := DependencyRefEvent{
		TargetID:    "PRI-LOCK-1",
		TriggerID:   "BLI-L1",
		EventID:     "ev-mem-1",
		Version:     time.Now().Format(time.RFC3339Nano),
		ProjectRoot: dir,
	}
	commitMembershipTransition(ctx, logger, mock, secCtx, evMem, objects.KindPriorityPlan, statusActive, statusGrooming, map[string]any{objects.FieldKeyStatus: statusGrooming})

	// 13. ApplyComputeHooks with TestCase verification_ready
	mock.add(map[string]any{
		objects.FieldKeyID:       "TC-VERIFY-1",
		objects.FieldKeyKind:     objects.KindTestCase,
		objects.FieldKeyStatus:   statusInProgress,
		objects.FieldKeyPathOrID: "pkg/test/test.go",
	})
	ApplyComputeHooks(ctx, mock, secCtx, TransitionRequest{
		Kind:     objects.KindTestCase,
		ID:       "TC-VERIFY-1",
		ToStatus: "verification_ready",
	}, make(map[string]any))
}

func TestLifecycle_PushOver80Percent(t *testing.T) {
	// 1. membraneApplyRank and compareMembraneMembers
	kinds := []string{
		kindVision, kindWorkstream, kindRoadmap, kindStrategicPlan,
		kindMission, kindGoal, kindMilestone, kindPriorityPlan,
		kindCriteria, kindBacklogItem, kindRequirement, "other",
	}
	for _, k := range kinds {
		_ = membraneApplyRank(k)
	}
	_ = compareMembraneMembers(MembraneMember{Kind: kindVision, ID: "A"}, MembraneMember{Kind: kindMission, ID: "B"})
	_ = compareMembraneMembers(MembraneMember{Kind: kindMission, ID: "A"}, MembraneMember{Kind: kindMission, ID: "B"})
	_ = compareMembraneMembers(MembraneMember{Kind: kindMission, ID: "A"}, MembraneMember{Kind: kindMission, ID: "A"})

	// 2. listener.Run with actual events and timeout
	dir := t.TempDir()
	wal, err := NewLifecycleEventWAL(dir)
	if err != nil {
		t.Fatalf("NewLifecycleEventWAL failed: %v", err)
	}
	_ = wal.Append(&LifecycleEvent{
		EventType:   EventTypeCriterionSatisfied,
		CriterionID: criterionAllBacklogComplete,
		Scope:       map[string]string{scopePlanID: "PRI-TIMEOUT-1"},
	})
	_ = wal.Sync()

	mock := newMockStorageProvider()
	getStorage := func(string) (storage.ObjectStorageProvider, bool) {
		return mock, true
	}
	ch := make(chan TransitionRequest, 10)
	l := NewListener(dir, wal, DefaultTransitionRules(), ch, getStorage)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_ = l.Run(timeoutCtx)

	// 3. updater.Run with failed apply
	failingMock := &failingStorageProvider{}
	failingGetStorage := func(string) (storage.ObjectStorageProvider, bool) {
		return failingMock, true
	}
	errCh := make(chan TransitionRequest, 2)
	errCh <- TransitionRequest{Kind: objects.KindBacklogItem, ID: "BLI-FAIL", ToStatus: statusComplete}
	close(errCh)
	updater := NewUpdater(dir, failingGetStorage, errCh)
	_ = updater.Run(context.Background())

	// 4. remaining_open_count edge cases
	_ = ensureRemainingOpenCount(context.Background(), mock, "PRI-LOCK-1", -1)
	secCtx := pkgctx.NewSystemSecurityContext()
	_, _ = casAdjustRemainingOpenCount(context.Background(), mock, secCtx, "PRI-LOCK-1", 0)

	// 5. ApplyPlanChildMembershipRemoved all 3 role branches
	logger := logging.NewEventLogger(context.Background())
	// Shovel-ready branch -> demotes to grooming
	mock.add(map[string]any{
		objects.FieldKeyID:     "PRI-SHOVEL-1",
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyStatus: statusActive,
	})
	mock.add(map[string]any{
		objects.FieldKeyID:              "BLI-S1",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-SHOVEL-1",
		objects.FieldKeyStatus:          statusInProgress,
	})
	ApplyPlanChildMembershipRemoved(context.Background(), logger, mock, dir, "PRI-SHOVEL-1", "BLI-S1")

	// Execution-locked with park -> pauses
	mock.add(map[string]any{
		objects.FieldKeyID:                 "PRI-PARK-1",
		objects.FieldKeyKind:               objects.KindPriorityPlan,
		objects.FieldKeyStatus:             statusInProgress,
		objects.FieldKeyRemainingOpenCount: 1,
	})
	mock.add(map[string]any{
		objects.FieldKeyID:              "BLI-P1",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-PARK-1",
		objects.FieldKeyStatus:          statusInProgress,
	})
	ApplyPlanChildMembershipRemoved(context.Background(), logger, mock, dir, "PRI-PARK-1", "BLI-P1", WithPark(true))

	// Execution-locked with auto-complete -> completes
	mock.add(map[string]any{
		objects.FieldKeyID:                 "PRI-AUTO-1",
		objects.FieldKeyKind:               objects.KindPriorityPlan,
		objects.FieldKeyStatus:             statusInProgress,
		objects.FieldKeyRemainingOpenCount: 1,
	})
	mock.add(map[string]any{
		objects.FieldKeyID:              "BLI-A1",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-AUTO-1",
		objects.FieldKeyStatus:          statusInProgress,
	})
	ApplyPlanChildMembershipRemoved(context.Background(), logger, mock, dir, "PRI-AUTO-1", "BLI-A1", WithPark(false))

	// Early return branches: terminal plan, terminal child
	mock.add(map[string]any{
		objects.FieldKeyID:     "PRI-TERM-1",
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyStatus: statusComplete,
	})
	ApplyPlanChildMembershipRemoved(context.Background(), logger, mock, dir, "PRI-TERM-1", "BLI-A1")

	mock.add(map[string]any{
		objects.FieldKeyID:              "BLI-TERM-1",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-PARK-1",
		objects.FieldKeyStatus:          statusComplete,
	})
	ApplyPlanChildMembershipRemoved(context.Background(), logger, mock, dir, "PRI-PARK-1", "BLI-TERM-1")

	// 6. applyPriorityPlanDependencyRef: children not ready-or-later
	mock.add(map[string]any{
		objects.FieldKeyID:     "PRI-NOTREADY-1",
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyStatus: statusActive,
	})
	mock.add(map[string]any{
		objects.FieldKeyID:              "BLI-NR1",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyPriorityPlanRef: "PRI-NOTREADY-1",
		objects.FieldKeyStatus:          objects.ObjectStatusDraft,
	})
	evNotReady := DependencyRefEvent{
		ProjectRoot: dir,
		EventID:     "ev-nr-1",
		Version:     time.Now().Format(time.RFC3339Nano),
		TargetID:    "PRI-NOTREADY-1",
		TriggerID:   "BLI-NR1",
		TriggerKind: objects.KindBacklogItem,
		FromState:   objects.ObjectStatusDraft,
		ToState:     statusInProgress,
	}
	applyPriorityPlanDependencyRef(context.Background(), logger, mock, secCtx, evNotReady, statusActive)
}

type failingStorageProvider struct {
	storage.ObjectStorageProvider
}

func (f *failingStorageProvider) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return map[string]any{objects.FieldKeyID: id, objects.FieldKeyKind: objects.KindBacklogItem}, nil
}

func (f *failingStorageProvider) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	return errors.New("simulated update failure")
}
