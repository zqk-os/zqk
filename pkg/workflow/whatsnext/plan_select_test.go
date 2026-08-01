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
	if got := activeOrderPenalty(map[string]any{}); got < 1000 {
		t.Fatalf("unset order should be large penalty, got %d", got)
	}
}

func TestCountOpenLinkedBLIs_IgnoresTerminal(t *testing.T) {
	t.Parallel()
	mock := &mockStorageForTest{
		items: []map[string]any{
			{objects.FieldKeyPriorityPlanRef: "PLAN-1", objects.FieldKeyStatus: "planned"},
			{objects.FieldKeyPriorityPlanRef: "PLAN-1", objects.FieldKeyStatus: "complete"},
			{objects.FieldKeyPriorityPlanRef: "PLAN-1", objects.FieldKeyStatus: "archived"},
			{objects.FieldKeyPriorityPlanRef: "PLAN-2", objects.FieldKeyStatus: "exploring"},
		},
	}
	if got := countOpenLinkedBLIs(nil, mock, "PLAN-1"); got != 1 {
		t.Fatalf("PLAN-1 open count: got %d want 1", got)
	}
	if got := countLinkedBLIs(nil, mock, "PLAN-1"); got != 3 {
		t.Fatalf("PLAN-1 total count: got %d want 3", got)
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
			{objects.FieldKeyPriorityPlanRef: "PLAN-1", objects.FieldKeyStatus: "planned"},
			{objects.FieldKeyPriorityPlanRef: "PLAN-2", objects.FieldKeyStatus: "planned"},
			{objects.FieldKeyPriorityPlanRef: "PLAN-OTHER", objects.FieldKeyStatus: "planned"},
		},
	}
	out := countBacklogByStatus(nil, mock, "PLAN-1", []string{"PLAN-1", "PLAN-2"}, nil)
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
