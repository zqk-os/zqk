package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestEvaluateCommandSchedulerState(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("allow-degraded", true, "")

	rootResolver := func(p string) string { return "/test/resolved/root" }
	checkRunning := func(r string) bool { return true }

	root, running, degraded := EvaluateCommandSchedulerState(cmd, rootResolver, checkRunning)
	if root != "/test/resolved/root" {
		t.Errorf("expected /test/resolved/root, got %s", root)
	}
	if !running {
		t.Errorf("expected running=true")
	}
	if !degraded {
		t.Errorf("expected degraded=true")
	}
}

func TestConfirm_TestEnvironment(t *testing.T) {
	// In test environments, Confirm always returns false to avoid hanging
	if Confirm("test prompt?") {
		t.Errorf("expected Confirm to return false in test environment")
	}
}
