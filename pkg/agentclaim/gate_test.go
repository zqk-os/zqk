package agentclaim

import (
	"strings"
	"testing"
	"time"
)

func TestGateWrite_deniesWithoutClaim(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	dec, err := GateWrite(root, "zqk-cursor-agent", GateOptions{})
	if err != nil {
		t.Fatalf("GateWrite: %v", err)
	}
	if dec.Allowed || dec.Reason != ReasonNoLiveClaim {
		t.Fatalf("unclaimed write must fail closed: %+v", dec)
	}
}

func TestGateWrite_allowsAfterArmCheckin(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	if err := ArmCheckin(root, "ATK-1", "zqk-cursor-agent", "agent_task", time.Minute); err != nil {
		t.Fatalf("ArmCheckin: %v", err)
	}
	dec, err := GateWrite(root, "zqk-cursor-agent", GateOptions{})
	if err != nil {
		t.Fatalf("GateWrite: %v", err)
	}
	if !dec.Allowed || dec.Reason != ReasonAllowed {
		t.Fatalf("claimed seat must be allowed to write: %+v", dec)
	}
	if len(dec.TaskIDs) != 1 || dec.TaskIDs[0] != "ATK-1" {
		t.Fatalf("task ids = %v", dec.TaskIDs)
	}
}

func TestGateWrite_emptyClaimantDenied(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	dec, err := GateWrite(root, "  ", GateOptions{})
	if err != nil {
		t.Fatalf("GateWrite: %v", err)
	}
	if dec.Allowed || dec.Reason != ReasonEmptyClaimant {
		t.Fatalf("empty claimant must fail closed: %+v", dec)
	}
}

func TestGateWrite_assignmentMustMatchHeldClaim(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	if err := ArmCheckin(root, "ATK-held", "seat-a", "agent_task", time.Minute); err != nil {
		t.Fatalf("ArmCheckin: %v", err)
	}
	dec, err := GateWrite(root, "seat-a", GateOptions{Assignment: "ATK-other", RequireAssignment: true})
	if err != nil {
		t.Fatalf("GateWrite: %v", err)
	}
	if dec.Allowed || dec.Reason != ReasonAssignmentNotHeld {
		t.Fatalf("wrong assignment must fail: %+v", dec)
	}
}

func TestGateWrite_autoAssignStillDeniesWrite(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	dec, err := GateWrite(root, "seat-b", GateOptions{
		Mode: GateModeAutoAssign,
		AutoClaim: func() (string, error) {
			if err := ArmCheckin(root, "ATK-auto", "seat-b", "agent_task", time.Minute); err != nil {
				return "", err
			}
			return "ATK-auto", nil
		},
	})
	if err != nil {
		t.Fatalf("GateWrite: %v", err)
	}
	if dec.Allowed {
		t.Fatal("auto-assign must interrupt the write, not allow it")
	}
	if dec.Reason != ReasonAutoAssigned || dec.AutoAssignedID != "ATK-auto" {
		t.Fatalf("auto-assign decision: %+v", dec)
	}
	if !strings.Contains(dec.Message, "ATK-auto") {
		t.Fatalf("interrupt must name the assigned task: %q", dec.Message)
	}
}

func TestGateWrite_breakGlass(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	dec, err := GateWrite(root, "nobody", GateOptions{BreakGlass: true})
	if err != nil {
		t.Fatalf("GateWrite: %v", err)
	}
	if !dec.Allowed || dec.Reason != ReasonBreakGlass {
		t.Fatalf("break-glass: %+v", dec)
	}
}

func TestClearCheckin_removesSeatStamp(t *testing.T) {
	t.Parallel()
	root := tempDir(t)
	if err := ArmCheckin(root, "ATK-9", "seat-c", "agent_task", time.Minute); err != nil {
		t.Fatalf("ArmCheckin: %v", err)
	}
	if err := ClearCheckin(root, "ATK-9"); err != nil {
		t.Fatalf("ClearCheckin: %v", err)
	}
	dec, err := GateWrite(root, "seat-c", GateOptions{})
	if err != nil {
		t.Fatalf("GateWrite: %v", err)
	}
	if dec.Allowed {
		t.Fatalf("released claim must not authorize writes: %+v", dec)
	}
}
