package healthcheck_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lanceman/zqk/pkg/healthcheck"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

type mockMonitor struct {
	id      string
	name    string
	result  *healthcheck.Result
	err     error
	runFunc func(ctx context.Context, projectRoot string) (*healthcheck.Result, error)
}

func (m *mockMonitor) ID() string   { return m.id }
func (m *mockMonitor) Name() string { return m.name }
func (m *mockMonitor) Run(ctx context.Context, projectRoot string) (*healthcheck.Result, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, projectRoot)
	}
	return m.result, m.err
}

func TestRegistry_RegistrationAndListing(t *testing.T) {
	t.Parallel()

	r := healthcheck.NewRegistry("")

	// Register nil should not panic or add
	r.Register(nil)
	if len(r.List()) != 0 {
		t.Fatalf("expected 0 monitors, got %d", len(r.List()))
	}

	m1 := &mockMonitor{id: "m1", name: "Monitor 1"}
	m2 := &mockMonitor{id: "m2", name: "Monitor 2"}

	r.Register(m1)
	r.Register(m2)

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(list))
	}

	gotM1, ok := r.Get("m1")
	if !ok || gotM1.ID() != "m1" {
		t.Fatalf("expected to get m1, got ok=%v, monitor=%v", ok, gotM1)
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Fatal("expected ok=false for nonexistent monitor")
	}

	if !r.IsEnabled("m1") {
		t.Fatal("expected m1 to be enabled by default")
	}
}

func TestRegistry_EnableDisableAndPersistence(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	r := healthcheck.NewRegistry(tmpDir)

	m1 := &mockMonitor{id: "m1", name: "Monitor 1"}
	r.Register(m1)

	// SetEnabled on nonexistent monitor returns error
	if err := r.SetEnabled("nonexistent", false); !errors.Is(err, fileutil.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}

	// Disable m1
	if err := r.SetEnabled("m1", false); err != nil {
		t.Fatalf("SetEnabled failed: %v", err)
	}
	if r.IsEnabled("m1") {
		t.Fatal("expected m1 to be disabled")
	}

	// Create new registry pointing to same dir to test persistence
	r2 := healthcheck.NewRegistry(tmpDir)
	r2.Register(m1)
	if r2.IsEnabled("m1") {
		t.Fatal("expected m1 to stay disabled after reload from config")
	}

	// Test BindProjectRoot
	tmpDir2 := t.TempDir()
	r3 := healthcheck.NewRegistry("")
	r3.Register(m1)
	if err := r3.BindProjectRoot(tmpDir2); err != nil {
		t.Fatalf("BindProjectRoot failed: %v", err)
	}
}

func TestRegistry_RunSingleMonitor(t *testing.T) {
	t.Parallel()

	r := healthcheck.NewRegistry("")

	// Nonexistent monitor
	_, err := r.Run(context.Background(), "", "nonexistent")
	if !errors.Is(err, fileutil.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}

	// Disabled monitor
	m1 := &mockMonitor{id: "m1", name: "Monitor 1", result: &healthcheck.Result{Status: objects.ObjectStatusOk, Summary: "all good"}}
	r.Register(m1)
	_ = r.SetEnabled("m1", false)

	res, err := r.Run(context.Background(), "", "m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != objects.ObjectStatusSkip {
		t.Fatalf("expected status skip, got %s", res.Status)
	}

	// Re-enable and run
	_ = r.SetEnabled("m1", true)
	res, err = r.Run(context.Background(), "", "m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != objects.ObjectStatusOk || res.Summary != "all good" {
		t.Fatalf("unexpected result: %#v", res)
	}
}

func TestRegistry_RunAllMonitorsAggregations(t *testing.T) {
	t.Parallel()

	t.Run("all_ok", func(t *testing.T) {
		r := healthcheck.NewRegistry("")
		r.Register(&mockMonitor{id: "m1", result: &healthcheck.Result{Status: objects.ObjectStatusOk, Summary: "m1 ok"}})
		r.Register(&mockMonitor{id: "m2", result: &healthcheck.Result{Status: objects.ObjectStatusOk, Summary: "m2 ok"}})

		res, err := r.Run(context.Background(), "", "")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != objects.ObjectStatusOk {
			t.Fatalf("expected status ok, got %s", res.Status)
		}
	})

	t.Run("degraded_priority", func(t *testing.T) {
		r := healthcheck.NewRegistry("")
		r.Register(&mockMonitor{id: "m1", result: &healthcheck.Result{Status: objects.ObjectStatusOk, Summary: "m1 ok"}})
		r.Register(&mockMonitor{id: "m2", result: &healthcheck.Result{Status: objects.ObjectStatusDegraded, Summary: "m2 degraded"}})

		res, err := r.Run(context.Background(), "", "")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != objects.ObjectStatusDegraded {
			t.Fatalf("expected status degraded, got %s", res.Status)
		}
	})

	t.Run("fail_priority_over_degraded", func(t *testing.T) {
		r := healthcheck.NewRegistry("")
		r.Register(&mockMonitor{id: "m1", result: &healthcheck.Result{Status: objects.ObjectStatusDegraded, Summary: "m1 degraded"}})
		r.Register(&mockMonitor{id: "m2", result: &healthcheck.Result{Status: objects.ObjectStatusFail, Summary: "m2 fail"}})

		res, err := r.Run(context.Background(), "", "")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != objects.ObjectStatusFail {
			t.Fatalf("expected status fail, got %s", res.Status)
		}
	})

	t.Run("unknown_status_treated_as_degraded", func(t *testing.T) {
		r := healthcheck.NewRegistry("")
		r.Register(&mockMonitor{id: "m1", result: &healthcheck.Result{Status: objects.ObjectStatusError, Summary: "m1 unknown"}})

		res, err := r.Run(context.Background(), "", "")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != objects.ObjectStatusDegraded {
			t.Fatalf("expected status degraded for unknown status, got %s", res.Status)
		}
	})

	t.Run("monitor_error_treated_as_fail", func(t *testing.T) {
		r := healthcheck.NewRegistry("")
		r.Register(&mockMonitor{id: "m1", err: errors.New("timeout connecting")})

		res, err := r.Run(context.Background(), "", "")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != objects.ObjectStatusFail {
			t.Fatalf("expected status fail on error, got %s", res.Status)
		}
	})

	t.Run("nil_result_treated_as_fail", func(t *testing.T) {
		r := healthcheck.NewRegistry("")
		r.Register(&mockMonitor{id: "m1", result: nil})

		res, err := r.Run(context.Background(), "", "")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if res.Status != objects.ObjectStatusFail {
			t.Fatalf("expected status fail on nil result (anti-false-green), got %s", res.Status)
		}
	})
}
