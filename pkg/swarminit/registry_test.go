package swarminit

import (
	"strings"
	"testing"
)

func TestRegistry_unknownExecutorFailClosed(t *testing.T) {
	t.Parallel()
	_, err := DefaultRegistry().Lookup("not_a_plugin")
	if err == nil || !strings.Contains(err.Error(), "unknown swarm-init executor") {
		t.Fatalf("want unknown executor, got %v", err)
	}
}

func TestDefaultRegistry_hasV1Executors(t *testing.T) {
	t.Parallel()
	r := DefaultRegistry()
	for _, id := range []string{
		ExecutorControlPlane,
		ExecutorBindSeats,
		ExecutorSeatWorkers,
		ExecutorCommsCheck,
		ExecutorChatBootstrap,
		ExecutorOrchestratePlan,
	} {
		if _, err := r.Lookup(id); err != nil {
			t.Fatalf("missing executor %s: %v", id, err)
		}
	}
}
