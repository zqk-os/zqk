package object

import (
	"strings"
	"testing"
)

func TestParsePlanAndBLI(t *testing.T) {
	// Standard format: planID, bliID
	p1, b1 := parsePlanAndBLI("PLAN-123", "ITEM-456")
	if p1 != "PLAN-123" || b1 != "ITEM-456" {
		t.Errorf("Expected PLAN-123, ITEM-456; got %s, %s", p1, b1)
	}

	// Reversed format: bliID, planID
	p2, b2 := parsePlanAndBLI("ITEM-456", "PLAN-123")
	if p2 != "PLAN-123" || b2 != "ITEM-456" {
		t.Errorf("Expected PLAN-123, ITEM-456 when reversed; got %s, %s", p2, b2)
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
}

func TestNewPPlanRemoveCmd_FlagsAndHelp(t *testing.T) {
	cmd := NewPPlanRemoveCmd()
	if cmd.Use != "remove" {
		t.Errorf("Expected Use 'remove', got %s", cmd.Use)
	}
}

func TestPPlanAddRemovePositionalValidation(t *testing.T) {
	// Test positional arg order normalization
	plan, bli := parsePlanAndBLI("PLAN-001", "ITEM-001")
	if plan != "PLAN-001" || bli != "ITEM-001" {
		t.Fatalf("expected PLAN-001, ITEM-001; got %s, %s", plan, bli)
	}

	planRev, bliRev := parsePlanAndBLI("ITEM-002", "PLAN-002")
	if planRev != "PLAN-002" || bliRev != "ITEM-002" {
		t.Fatalf("expected PLAN-002, ITEM-002; got %s, %s", planRev, bliRev)
	}
}
