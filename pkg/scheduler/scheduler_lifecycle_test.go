package scheduler

import (
	"testing"
)

func TestSchedulerLifecycle_IsRunning(t *testing.T) {
	s := &Scheduler{}
	if s.IsRunning() {
		t.Fatal("expected unstarted scheduler to not be running")
	}
}
// tdd refresh
