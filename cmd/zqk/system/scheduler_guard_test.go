package system

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestEvaluateSchedulerGuard(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		requirement   schedulerCommandRequirement
		running       bool
		allowDegraded bool
		wantBlock     bool
		wantWarn      bool
	}{
		{
			name:          "required blocks when scheduler down and no override",
			requirement:   schedulerRequirementRequired,
			running:       false,
			allowDegraded: false,
			wantBlock:     true,
			wantWarn:      false,
		},
		{
			name:          "required warns with override",
			requirement:   schedulerRequirementRequired,
			running:       false,
			allowDegraded: true,
			wantBlock:     false,
			wantWarn:      true,
		},
		{
			name:          "optional warns when down",
			requirement:   schedulerRequirementOptional,
			running:       false,
			allowDegraded: false,
			wantBlock:     false,
			wantWarn:      true,
		},
		{
			name:          "no action when running",
			requirement:   schedulerRequirementRequired,
			running:       true,
			allowDegraded: false,
			wantBlock:     false,
			wantWarn:      false,
		},
		{
			name:          "no action for non dependent command",
			requirement:   schedulerRequirementNone,
			running:       false,
			allowDegraded: false,
			wantBlock:     false,
			wantWarn:      false,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			block, warn := evaluateSchedulerGuard(tc.requirement, tc.running, tc.allowDegraded)
			if block != tc.wantBlock || warn != tc.wantWarn {
				t.Fatalf("evaluateSchedulerGuard() = (%v,%v), want (%v,%v)", block, warn, tc.wantBlock, tc.wantWarn)
			}
		})
	}
}

func TestRequirementForSystemCommand(t *testing.T) {
	t.Parallel()
	if got := requirementForSystemCommand("aggregate-audit"); got != schedulerRequirementRequired {
		t.Fatalf("aggregate-audit requirement = %v, want required", got)
	}
	if got := requirementForSystemCommand("retention-status"); got != schedulerRequirementOptional {
		t.Fatalf("maintenance-request-cycle requirement = %v, want optional", got)
	}
	if got := requirementForSystemCommand("whoami"); got != schedulerRequirementNone {
		t.Fatalf("whoami requirement = %v, want none", got)
	}
}

func TestRunSystemSchedulerGuard_BlockAndOverride(t *testing.T) {
	// Do not use t.Parallel(): mutates package-level function pointers.
	origChecker := schedulerRunningChecker
	origResolver := resolveProjectRootForSchedulerGuard
	t.Cleanup(func() {
		schedulerRunningChecker = origChecker
		resolveProjectRootForSchedulerGuard = origResolver
	})
	resolveProjectRootForSchedulerGuard = func(string) string { return "/tmp/test-root" }
	schedulerRunningChecker = func(string) bool { return false }

	buildCmd := func(name string, allowDegraded bool) *cobra.Command {
		cmd := &cobra.Command{Use: name}
		cmd.Flags().Bool("allow-degraded", false, "")
		_ = cmd.Flags().Set("allow-degraded", map[bool]string{true: "true", false: "false"}[allowDegraded])
		return cmd
	}

	if err := runSystemSchedulerGuard(buildCmd("aggregate-audit", false), nil); err == nil {
		t.Fatalf("expected blocking error for required command when scheduler is down")
	} else if !strings.Contains(err.Error(), "requires scheduler-backed maintenance/caches") ||
		!strings.Contains(err.Error(), "SCHEDULER_DEGRADED_MODE_GUARDRAILS.md") {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := runSystemSchedulerGuard(buildCmd("aggregate-audit", true), nil); err != nil {
		t.Fatalf("expected allow-degraded override, got error: %v", err)
	}

	if err := runSystemSchedulerGuard(buildCmd("retention-status", false), nil); err != nil {
		t.Fatalf("optional command should not block when scheduler is down: %v", err)
	}
}
