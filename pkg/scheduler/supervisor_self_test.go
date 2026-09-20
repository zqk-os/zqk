package scheduler

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestRefuseSupervisorSelfStop_selfAndParent(t *testing.T) {
	t.Parallel()
	if err := RefuseSupervisorSelfStop(os.Getpid()); err == nil {
		t.Fatal("expected refuse when target is the caller")
	} else if !errors.Is(err, ErrSupervisorSelfStop) {
		t.Fatalf("errors.Is(ErrSupervisorSelfStop) = false; got %v", err)
	}
	if parent := os.Getppid(); parent > 1 {
		if err := RefuseSupervisorSelfStop(parent); err == nil {
			t.Fatal("expected refuse when target is the caller parent")
		}
	}
}

func TestRefuseSupervisorSelfStop_unrelated(t *testing.T) {
	t.Parallel()
	if err := RefuseSupervisorSelfStop(1_000_000_007); err != nil {
		t.Fatalf("unrelated pid: %v", err)
	}
}

func TestSignalSchedulerByPID_refusesCallerPIDFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := writePIDFile(root); err != nil {
		t.Fatal(err)
	}
	_, err := SignalSchedulerByPID(root, syscall.SIGTERM)
	if err == nil {
		t.Fatal("expected refuse when pid file is the test process")
	}
	if !errors.Is(err, ErrSupervisorSelfStop) {
		t.Fatalf("errors.Is(ErrSupervisorSelfStop) = false; got %v", err)
	}
}
