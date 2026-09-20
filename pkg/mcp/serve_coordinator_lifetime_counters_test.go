package mcp

import (
	"testing"
)

func TestServeCoordinator_LifetimeCounters(t *testing.T) {
	var sc *ServeCoordinator
	itNil, toNil := sc.GetServeCoordinatorStats()
	if itNil != 0 || toNil != 0 {
		t.Fatalf("expected nil stats (0, 0), got it=%d to=%d", itNil, toNil)
	}

	server := NewServer()
	handler := HandlerFunc(nil)
	transport := NewDefaultTransport()
	processor := NewMessageProcessor(server, handler, transport)
	lifecycle := NewServerLifecycleBuilder(server)

	sc = NewServeCoordinator(server, processor, lifecycle)
	itInit, toInit := sc.GetServeCoordinatorStats()
	if itInit != 0 || toInit != 0 {
		t.Fatalf("expected new stats (0, 0), got it=%d to=%d", itInit, toInit)
	}

	sc.loopIterationsTotal.Add(1)
	sc.timeoutShutdownsTotal.Add(1)

	itAfter, toAfter := sc.GetServeCoordinatorStats()
	if itAfter != 1 || toAfter != 1 {
		t.Fatalf("expected stats (1, 1), got it=%d to=%d", itAfter, toAfter)
	}
}
