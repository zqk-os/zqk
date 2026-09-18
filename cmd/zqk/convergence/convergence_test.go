package convergence_test

import (
	"testing"

	"github.com/zqk-os/zqk/cmd/zqk/convergence"
	"github.com/zqk-os/zqk/pkg/convergerollup"
)

func TestConvergenceNestVeneer_CliParity(t *testing.T) {
	topCmd := convergence.NewConvergenceCmd()
	if topCmd == nil {
		t.Fatalf("expected non-nil top-level convergence command")
	}

	subCommands := []string{"nest-spawn", "nest-link", "nest-status"}
	for _, sub := range subCommands {
		subCmd, _, err := topCmd.Find([]string{sub})
		if err != nil || subCmd == nil {
			t.Errorf("expected subcommand %s under top-level convergence, got err: %v", sub, err)
		}
	}

	// Verify RunE function parity with scheduler nest handlers
	nestSpawnCmd, _, _ := topCmd.Find([]string{"nest-spawn"})
	if nestSpawnCmd == nil || nestSpawnCmd.RunE == nil {
		t.Errorf("expected non-nil RunE on top-level convergence nest-spawn")
	}

	nestStatusCmd, _, _ := topCmd.Find([]string{"nest-status"})
	if nestStatusCmd == nil || nestStatusCmd.RunE == nil {
		t.Errorf("expected non-nil RunE on top-level convergence nest-status")
	}

	// Empty parent-session-id is rejected by shared nest-status logic (no CLI/storage bootstrap required).
	_, err := convergerollup.NestStatus("", 0, nil)
	if err == nil {
		t.Errorf("expected error when parent-session-id is empty")
	}
}
