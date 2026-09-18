package scheduler

import (
	"testing"
	"time"
)

func TestSchedulerLifecycle_IsRunning(t *testing.T) {
	s := &Scheduler{}
	if s.IsRunning() {
		t.Fatal("expected unstarted scheduler to not be running")
	}
}

func TestSchedulerLifecycle_ShutdownTimeouts(t *testing.T) {
	if schedulerCronStopTimeout != 2*time.Second {
		t.Errorf("expected 2s schedulerCronStopTimeout, got %v", schedulerCronStopTimeout)
	}
	if schedulerTriggeredPoolStopTimeout != 5*time.Second {
		t.Errorf("expected 5s schedulerTriggeredPoolStopTimeout, got %v", schedulerTriggeredPoolStopTimeout)
	}
}
