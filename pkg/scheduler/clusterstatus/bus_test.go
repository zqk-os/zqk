package clusterstatus_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler/clusterstatus"
)

func TestBusFailClosedOnStale(t *testing.T) {
	dir := t.TempDir()
	bus := clusterstatus.NewBus(dir, 50*time.Millisecond)
	if err := bus.Emit(clusterstatus.Event{
		NodeID: "n1",
		RootID: "r1",
		JobID:  "j1",
		Phase:  clusterstatus.PhaseSucceeded,
	}); err != nil {
		t.Fatal(err)
	}
	d := bus.Decide(clusterstatus.Watch{RootID: "r1", JobID: "j1"})
	if !d.AllowForward {
		t.Fatalf("expected allow after success, got %+v", d)
	}
	time.Sleep(80 * time.Millisecond)
	d = bus.Decide(clusterstatus.Watch{RootID: "r1", JobID: "j1", TTL: 50 * time.Millisecond})
	if d.AllowForward || d.Phase != clusterstatus.PhaseStale {
		t.Fatalf("expected fail-closed stale, got %+v", d)
	}
	if _, err := filepath.Glob(filepath.Join(dir, paths.ProjectDataDir, paths.LogsDir, "cluster_status", "*.jsonl")); err != nil {
		t.Fatal(err)
	}
}
