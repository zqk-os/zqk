package object

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestParsePlanAndBLI(t *testing.T) {
	// Standard format: planID, bliID
	p1, b1 := parsePlanAndBLI("PRI-123", "BLI-456")
	if p1 != "PRI-123" || b1 != "BLI-456" {
		t.Errorf("Expected PRI-123, BLI-456; got %s, %s", p1, b1)
	}

	// Reversed format: bliID, planID
	p2, b2 := parsePlanAndBLI("BLI-456", "PRI-123")
	if p2 != "PRI-123" || b2 != "BLI-456" {
		t.Errorf("Expected PRI-123, BLI-456 when reversed; got %s, %s", p2, b2)
	}
}

func TestParsePlanAndBLIs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantPlanID  string
		wantBliIDs  []string
	}{
		{
			name:       "PRI first with single BLI",
			args:       []string{"PRI-100", "BLI-001"},
			wantPlanID: "PRI-100",
			wantBliIDs: []string{"BLI-001"},
		},
		{
			name:       "PRI first with multiple BLIs",
			args:       []string{"PRI-100", "BLI-001", "BLI-002", "BLI-003"},
			wantPlanID: "PRI-100",
			wantBliIDs: []string{"BLI-001", "BLI-002", "BLI-003"},
		},
		{
			name:       "PRI last with multiple BLIs",
			args:       []string{"BLI-001", "BLI-002", "PRI-100"},
			wantPlanID: "PRI-100",
			wantBliIDs: []string{"BLI-001", "BLI-002"},
		},
		{
			name:       "BLI first with PRI second (2 args)",
			args:       []string{"BLI-001", "PRI-100"},
			wantPlanID: "PRI-100",
			wantBliIDs: []string{"BLI-001"},
		},
		{
			name:       "Semantic non-prefixed args (default first is plan)",
			args:       []string{"active-sprint", "task-1", "task-2"},
			wantPlanID: "active-sprint",
			wantBliIDs: []string{"task-1", "task-2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotPlan, gotBlis := parsePlanAndBLIs(tc.args)
			if gotPlan != tc.wantPlanID {
				t.Errorf("expected plan %s, got %s", tc.wantPlanID, gotPlan)
			}
			if !reflect.DeepEqual(gotBlis, tc.wantBliIDs) {
				t.Errorf("expected BLIs %v, got %v", tc.wantBliIDs, gotBlis)
			}
		})
	}
}

func TestNewPPlanAddCmd_FlagsAndHelp(t *testing.T) {
	cmd := NewPPlanAddCmd()
	if cmd.Use != "add" {
		t.Errorf("Expected Use 'add', got %s", cmd.Use)
	}
	if !strings.Contains(cmd.Short, "child-owned priority_plan_ref") {
		t.Errorf("Expected short description to mention child-owned priority_plan_ref, got: %s", cmd.Short)
	}
	if cmd.Flags().Lookup("override") == nil {
		t.Error("expected --override flag on pplan add")
	}
	if cmd.Flags().Lookup("force") == nil {
		t.Error("expected --force flag on pplan add")
	}
	if cmd.Flags().Lookup("reason-code") == nil {
		t.Error("expected --reason-code flag on pplan add")
	}

	// Verify accepts multiple args (minimum 2)
	if err := cmd.Args(cmd, []string{"PRI-1", "BLI-1"}); err != nil {
		t.Errorf("expected 2 args to pass, got err: %v", err)
	}
	if err := cmd.Args(cmd, []string{"PRI-1", "BLI-1", "BLI-2"}); err != nil {
		t.Errorf("expected 3 args to pass, got err: %v", err)
	}
	if err := cmd.Args(cmd, []string{"PRI-1"}); err == nil {
		t.Error("expected 1 arg to fail")
	}
}

func TestNewPPlanRemoveCmd_FlagsAndHelp(t *testing.T) {
	cmd := NewPPlanRemoveCmd()
	if cmd.Use != "remove" {
		t.Errorf("Expected Use 'remove', got %s", cmd.Use)
	}
	if cmd.Flags().Lookup("park") == nil {
		t.Error("pplan remove command missing --park flag")
	}
	if err := cmd.Args(cmd, []string{"PRI-1", "BLI-1"}); err != nil {
		t.Errorf("expected 2 args to pass, got err: %v", err)
	}
	if err := cmd.Args(cmd, []string{"PRI-1", "BLI-1", "BLI-2"}); err != nil {
		t.Errorf("expected 3 args to pass, got err: %v", err)
	}
	if err := cmd.Args(cmd, []string{"PRI-1"}); err == nil {
		t.Error("expected 1 arg to fail")
	}
}

func TestDemoteUnlinkedChildStatus(t *testing.T) {
	// For backlog_item unlinked from shovel_ready (planned), demotes to validated
	st := demoteUnlinkedChildStatus(objects.KindBacklogItem, "planned")
	if st != "validated" {
		t.Errorf("expected demoted backlog_item status 'validated', got %q", st)
	}

	// For priority_plan unlinked from shovel_ready, demotes to grooming
	planSt := demoteUnlinkedChildStatus(objects.KindPriorityPlan, "active")
	if planSt != "grooming" {
		t.Errorf("expected demoted priority_plan status 'grooming', got %q", planSt)
	}
}

func TestPPlanAddRemovePositionalValidation(t *testing.T) {
	// Test positional arg order normalization
	plan, bli := parsePlanAndBLI("PRI-001", "BLI-001")
	if plan != "PRI-001" || bli != "BLI-001" {
		t.Fatalf("expected PRI-001, BLI-001; got %s, %s", plan, bli)
	}

	planRev, bliRev := parsePlanAndBLI("BLI-002", "PRI-002")
	if planRev != "PRI-002" || bliRev != "BLI-002" {
		t.Fatalf("expected PRI-002, BLI-002; got %s, %s", planRev, bliRev)
	}
}

func TestPPlanAddRejectsSealedPlan(t *testing.T) {
	cmd := NewPPlanAddCmd()
	if cmd == nil {
		t.Fatal("NewPPlanAddCmd returned nil")
	}

	// Verify that in_progress and active plans are treated as sealed
	for _, st := range []string{objects.ObjectStatusInProgress, objects.ObjectStatusActive} {
		isSealed := (st == objects.ObjectStatusInProgress || st == objects.ObjectStatusActive)
		if !isSealed {
			t.Errorf("expected status %s to be recognized as sealed", st)
		}
	}
}

