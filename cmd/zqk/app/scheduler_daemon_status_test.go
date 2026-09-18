package app

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestCheckSchedulerDaemonStatus_NoWarningForReadCommands verifies that the root-level
// scheduler guard does NOT emit warnings for read-only commands (like "get", "list", "version").
// Previously the guard unconditionally warned "Scheduler daemon is down" on every command,
// causing false alarms when the scheduler was momentarily unreachable.
func TestCheckSchedulerDaemonStatus_NoWarningForReadCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cmdName    string
		wantNoWarn bool // if true, the non-mutating path should suppress the generic warning
	}{
		{name: "get is read-only", cmdName: "get", wantNoWarn: true},
		{name: "list is read-only", cmdName: "list", wantNoWarn: true},
		{name: "version is read-only", cmdName: "version", wantNoWarn: true},
		{name: "status is read-only", cmdName: "status", wantNoWarn: true},
		{name: "count is read-only", cmdName: "count", wantNoWarn: true},
		{name: "create is mutating", cmdName: "create", wantNoWarn: false},
		{name: "update is mutating", cmdName: "update", wantNoWarn: false},
		{name: "delete is mutating", cmdName: "delete", wantNoWarn: false},
		{name: "workflow is mutating", cmdName: "workflow", wantNoWarn: false},
		{name: "system is mutating", cmdName: "system", wantNoWarn: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isMutatingCommand(buildMockCmd(tc.cmdName))
			if tc.wantNoWarn && got {
				t.Errorf("command %q classified as mutating, expected read-only", tc.cmdName)
			}
			if !tc.wantNoWarn && !got {
				t.Errorf("command %q classified as read-only, expected mutating", tc.cmdName)
			}
		})
	}
}

// buildMockCmd creates a minimal cobra command hierarchy for testing command classification.
func buildMockCmd(name string) *cobra.Command {
	root := &cobra.Command{Use: "zqk"}
	child := &cobra.Command{Use: name}
	root.AddCommand(child)
	return child
}
