// BLI-STARTER-COMMUNITY-029 / PRI-STARTER-COMMUNITY-029 coverage elevation
package ambience

import (
	"context"
	"testing"
	"time"
)

func TestSignalAggregator_FlushOnCountAndStop(t *testing.T) {
	t.Parallel()
	mesh := NewInMemoryEventMesh()
	agg := NewSignalAggregator(mesh, 50*time.Millisecond, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := agg.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := agg.Start(ctx); err != nil {
		t.Fatal("second start")
	}
	ev := AmbientEvent{Type: EventFileModified, URI: "a.go", Timestamp: time.Now().Unix()}
	if err := mesh.Publish(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if err := mesh.Publish(ctx, AmbientEvent{Type: EventFileModified, URI: "b.go", Timestamp: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-agg.Aggregated():
		if len(got) < 2 {
			t.Fatalf("flush count: %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for aggregate flush")
	}
	if _, err := agg.PredictIntent(ev); err != nil {
		t.Fatal(err)
	}
	if err := agg.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestSignalAggregator_FlushOnWindow(t *testing.T) {
	t.Parallel()
	mesh := NewInMemoryEventMesh()
	agg := NewSignalAggregator(mesh, 20*time.Millisecond, 99)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := agg.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := mesh.Publish(ctx, AmbientEvent{Type: EventFocusChanged, URI: "x"}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-agg.Aggregated():
		if len(got) != 1 {
			t.Fatalf("window flush: %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout window flush")
	}
	_ = agg.Stop()
}

func TestInMemoryEventMesh_CanceledAndSourcing(t *testing.T) {
	t.Parallel()
	mesh := NewInMemoryEventMesh()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := mesh.Publish(ctx, AmbientEvent{Type: EventTestFailed}); err == nil {
		t.Fatal("canceled ctx")
	}
	mesh.EnableEventSourcing(t.TempDir(), nil)
	live := context.Background()
	if err := mesh.Publish(live, AmbientEvent{Type: EventTestFailed, URI: "x.yaml"}); err != nil {
		t.Fatal(err)
	}
}

func TestFSEventsEngine_PredictGoSum(t *testing.T) {
	t.Parallel()
	e := NewFSEventsEngine(NewInMemoryEventMesh())
	got, err := e.PredictIntent(AmbientEvent{Type: EventFileModified, URI: "go.sum"})
	if err != nil || got.Action != "mod_tidy" {
		t.Fatalf("%+v %v", got, err)
	}
}
