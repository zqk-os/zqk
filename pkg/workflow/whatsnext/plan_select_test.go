package whatsnext

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

func TestPlanStatusEligible(t *testing.T) {
	t.Parallel()
	for _, st := range []string{"in_progress", "paused", "active", "grooming", "prioritizing", "ACTIVE"} {
		if !planStatusEligible(st) {
			t.Fatalf("expected eligible: %q", st)
		}
	}
	for _, st := range []string{"complete", "archived", "", "exploring"} {
		if planStatusEligible(st) {
			t.Fatalf("expected not eligible: %q", st)
		}
	}
}

func TestPriorityPlanScoreBonusConstant(t *testing.T) {
	t.Parallel()
	if priorityPlanScoreBonus < 1000 {
		t.Fatalf("bonus too small to outrank typical strategic link counts: %d", priorityPlanScoreBonus)
	}
}

func TestPlanStatusExecutionReady(t *testing.T) {
	t.Parallel()
	for _, st := range []string{"in_progress", "active", "paused", "ACTIVE"} {
		if !planStatusExecutionReady(st) {
			t.Fatalf("expected execution-ready: %q", st)
		}
	}
	for _, st := range []string{"grooming", "prioritizing", "complete", ""} {
		if planStatusExecutionReady(st) {
			t.Fatalf("expected not execution-ready: %q", st)
		}
	}
}

func TestActiveOrderPenalty(t *testing.T) {
	t.Parallel()
	if got := activeOrderPenalty(map[string]any{objects.FieldKeyActiveOrder: 0}); got != 0 {
		t.Fatalf("order 0: got %d", got)
	}
	if got := activeOrderPenalty(map[string]any{objects.FieldKeyActiveOrder: 9999}); got != 9999 {
		t.Fatalf("order 9999: got %d", got)
	}
	if got := activeOrderPenalty(map[string]any{objects.FieldKeyStatus: "active"}); got < 1000 {
		t.Fatalf("unset order on active should be large penalty, got %d", got)
	}
	// in_progress ≡ top-of-stack (active @ 0) even when shockwave cleared active_order.
	if got := activeOrderPenalty(map[string]any{objects.FieldKeyStatus: "in_progress"}); got != 0 {
		t.Fatalf("in_progress with nil order: got %d want 0", got)
	}
	if got := activeOrderPenalty(map[string]any{
		objects.FieldKeyStatus:      "in_progress",
		objects.FieldKeyActiveOrder: 5,
	}); got != 0 {
		t.Fatalf("in_progress ignores stale active_order: got %d want 0", got)
	}
}

func TestInProgressPlanRanksAboveShovelReadyActive(t *testing.T) {
	t.Parallel()
	// Regression: clearing active_order on shockwave must not let active@1 beat in_progress.
	inProgScore := 1 + planExecutionStatusBonus("in_progress") -
		activeOrderPenalty(map[string]any{objects.FieldKeyStatus: "in_progress"})
	activeScore := 1 + planExecutionStatusBonus("active") -
		activeOrderPenalty(map[string]any{
			objects.FieldKeyStatus:      "active",
			objects.FieldKeyActiveOrder: 1,
		})
	if inProgScore <= activeScore {
		t.Fatalf("in_progress (nil order) score %d should beat active@1 score %d", inProgScore, activeScore)
	}
}

func TestCountOpenLinkedBLIs_IgnoresTerminal(t *testing.T) {
	t.Parallel()
	mock := &mockStorageForTest{
		items: []map[string]any{
			{objects.FieldKeyPriorityPlanRef: "PRI-1", objects.FieldKeyStatus: "planned"},
			{objects.FieldKeyPriorityPlanRef: "PRI-1", objects.FieldKeyStatus: "complete"},
			{objects.FieldKeyPriorityPlanRef: "PRI-1", objects.FieldKeyStatus: "archived"},
			{objects.FieldKeyPriorityPlanRef: "PRI-2", objects.FieldKeyStatus: "exploring"},
		},
	}
	if got := countOpenLinkedBLIs(context.Background(), mock, "PRI-1", nil); got != 1 {
		t.Fatalf("PRI-1 open count: got %d want 1", got)
	}
	if got := countLinkedBLIs(context.Background(), mock, "PRI-1"); got != 3 {
		t.Fatalf("PRI-1 total count: got %d want 3", got)
	}
}

func TestDeriveAgentInstructionFromBacklog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		counts   map[string]int
		expected string
	}{
		{
			name:     "Shutdown when all completed",
			counts:   map[string]int{"completed": 5},
			expected: "shutdown",
		},
		{
			name:     "Continue when in progress work",
			counts:   map[string]int{"in_progress": 2, "planned": 3},
			expected: "continue",
		},
		{
			name:     "Execute tests when verifying",
			counts:   map[string]int{"verifying": 1},
			expected: "execute_tests",
		},
		{
			name:     "Shutdown when no active work items",
			counts:   map[string]int{},
			expected: "shutdown",
		},
		{
			name:     "Exploring-only grooming is not shutdown",
			counts:   map[string]int{"exploring": 2},
			expected: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := deriveAgentInstructionFromBacklog(tc.counts)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
func TestCountBacklogByStatus_AggregatesActivePlans(t *testing.T) {
	t.Parallel()
	mock := &mockStorageForTest{
		items: []map[string]any{
			{objects.FieldKeyPriorityPlanRef: "PRI-1", objects.FieldKeyStatus: "planned"},
			{objects.FieldKeyPriorityPlanRef: "PRI-2", objects.FieldKeyStatus: "planned"},
			{objects.FieldKeyPriorityPlanRef: "PRI-OTHER", objects.FieldKeyStatus: "planned"},
		},
	}
	out := countBacklogByStatus(context.Background(), mock, "PRI-1", []string{"PRI-1", "PRI-2"}, nil)
	if out["planned"] != 2 {
		t.Errorf("expected 2 planned BLIs across active plans, got %d", out["planned"])
	}
}

type mockStorageForTest struct {
	storagepkg.ObjectStorageProvider
	items []map[string]any
}

func (m *mockStorageForTest) List(ctx context.Context, secCtx *storagepkg.SecurityContext, storageCtx *storagepkg.StorageContext, filter storagepkg.ListFilter) (*storagepkg.QueryResult, error) {
	return &storagepkg.QueryResult{Objects: m.items}, nil
}
