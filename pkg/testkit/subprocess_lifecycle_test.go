package testkit

import (
	"context"
	"syscall"
	"testing"
	"time"
)

// TestSubprocessLifecycleExecutionOrganizer serves as the unified Execution Organizer
// for TST-TST-SUBPROCESS-LIFECYCLE-HYGIENE, linking:
//   - CRIT-TST-SUBPROCESS-STATIC-GUARD
//   - CRIT-TST-SUBPROCESS-MANAGED-RUNNER
//   - CRIT-TST-SUBPROCESS-REAPER-ADVERSARIAL
func TestSubprocessLifecycleExecutionOrganizer(t *testing.T) {
	t.Run("Stage1_OperationalProof_ManagedCommandProcessGroupScoping", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cmd := ManagedCommand(t, ctx, "sleep", "10")
		if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
			t.Fatalf("expected SysProcAttr.Setpgid to be true, got %+v", cmd.SysProcAttr)
		}

		if err := cmd.Start(); err != nil {
			t.Fatalf("failed to start ManagedCommand: %v", err)
		}

		pid := cmd.Process.Pid
		if pid <= 0 {
			t.Fatalf("expected valid PID > 0, got %d", pid)
		}

		// Verify process is alive
		if err := syscall.Kill(pid, 0); err != nil {
			t.Fatalf("expected child PID %d to be alive: %v", pid, err)
		}

		// Terminate process group
		if err := KillManagedProcessGroup(cmd); err != nil {
			t.Fatalf("failed to kill process group: %v", err)
		}

		// Wait for command to exit
		_ = cmd.Wait()

		// Verify process is gone
		time.Sleep(50 * time.Millisecond)
		err := syscall.Kill(pid, 0)
		if err == nil {
			t.Fatalf("expected child PID %d to be terminated, but still responsive to signal 0", pid)
		}
	})

	t.Run("Stage2_AdversarialBoundary_ContextCancelReapsProcessTree", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		cmd := ManagedCommand(t, ctx, "sleep", "10")
		if err := cmd.Start(); err != nil {
			t.Fatalf("failed to start ManagedCommand: %v", err)
		}

		pid := cmd.Process.Pid
		// Wait for context cancellation to fire cmd.Cancel (which kills process group)
		err := cmd.Wait()
		if err == nil {
			t.Fatalf("expected command to exit with error on cancellation, got nil")
		}

		time.Sleep(50 * time.Millisecond)
		if killErr := syscall.Kill(pid, 0); killErr == nil {
			t.Fatalf("expected child PID %d to be reaped by context cancellation, but still alive", pid)
		}
	})

	t.Run("Stage3_NegativeInvariant_VerifyNoSubprocessLeaksClean", func(t *testing.T) {
		// Run leak verification to confirm no lingering child processes exist
		VerifyNoSubprocessLeaks(t)
	})
}
