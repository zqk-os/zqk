package do

import (
	"strings"
	"testing"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

func TestNewDoCmd(t *testing.T) {
	cmd := NewDoCmd()
	if cmd == nil {
		t.Fatal("NewDoCmd returned nil")
	}
	if cmd.Use != "do [task_or_bli_id]" {
		t.Errorf("cmd.Use = %q, want do [task_or_bli_id]", cmd.Use)
	}
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "auto-exec" {
		t.Errorf("cmd.Aliases = %v, want auto-exec as primary alias", cmd.Aliases)
	}
	if cmd.Flags().Lookup("dry-run") == nil {
		t.Error("expected --dry-run flag")
	}
	if cmd.Flags().Lookup("verify") == nil {
		t.Error("expected --verify flag")
	}
}

func TestRenderDoSummary(t *testing.T) {
	cmd := NewDoCmd()
	var buf strings.Builder
	cmd.SetOut(&buf)

	res := &clipkg.AutoExecResult{
		TargetBLIID:     "BLI-TEST-001",
		Status:          "in_progress",
		Claimant:        "ACC-SYSTEM",
		VerifiedTestIDs: []string{"TST-001"},
		LatchedCritIDs:  []string{"CRIT-001"},
		Steps: []clipkg.AutoExecStepResult{
			{Step: "discovery", StepStatus: "ok", Detail: "Resolved BLI-TEST-001"},
			{Step: "claim", StepStatus: "ok", Detail: "Claimed for ACC-SYSTEM"},
		},
	}

	renderDoSummary(cmd, res)
	out := buf.String()

	if !strings.Contains(out, "BLI-TEST-001") {
		t.Errorf("expected target BLI in output: %s", out)
	}
	if !strings.Contains(out, "Hand off to your agent") {
		t.Errorf("expected agent handoff instructions in output: %s", out)
	}
	if !strings.Contains(out, "Prompt:") {
		t.Errorf("expected agent prompt in output: %s", out)
	}
	if !strings.Contains(out, "TST-001") {
		t.Errorf("expected verified test ID in output: %s", out)
	}
}
