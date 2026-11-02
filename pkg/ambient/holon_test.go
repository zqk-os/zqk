package ambient

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// mockHolon implements Holon for unit verification.
type mockHolon struct {
	id          string
	name        string
	role        string
	ecosystem   string
	startFn     func(ctx context.Context) error
	stopFn      func() error
	state       HolonState
	restarts    int
	startedChan chan struct{}
}

func newMockHolon(id string, ecosystem string) *mockHolon {
	return &mockHolon{
		id:          id,
		name:        "mock-" + id,
		role:        "worker",
		ecosystem:   ecosystem,
		state:       HolonStateRegistered,
		startedChan: make(chan struct{}),
	}
}

func (m *mockHolon) ID() string        { return m.id }
func (m *mockHolon) Name() string      { return m.name }
func (m *mockHolon) Role() string      { return m.role }
func (m *mockHolon) Ecosystem() string { return m.ecosystem }

func (m *mockHolon) Start(ctx context.Context) error {
	m.state = HolonStateRunning
	close(m.startedChan)
	if m.startFn != nil {
		return m.startFn(ctx)
	}
	<-ctx.Done()
	return ctx.Err()
}

func (m *mockHolon) Stop() error {
	m.state = HolonStateStopped
	if m.stopFn != nil {
		return m.stopFn()
	}
	return nil
}

func (m *mockHolon) Status() HolonStatus {
	return HolonStatus{
		ID:        m.id,
		Name:      m.name,
		Role:      m.role,
		Ecosystem: m.ecosystem,
		Health: HolonHealth{
			State:        m.state,
			Healthy:      m.state == HolonStateRunning,
			RestartCount: m.restarts,
		},
	}
}

func setupSupervisorWithHolon(t *testing.T, id string) (*HolonSupervisor, *mockHolon) {
	t.Helper()
	sup := NewHolonSupervisor(nil)
	h := newMockHolon(id, "intent-pipeline")
	if err := sup.Register(h); err != nil {
		t.Fatalf("unexpected registration failure: %v", err)
	}
	return sup, h
}

func TestHolonSupervisor_Lifecycle(t *testing.T) {
	sup, h := setupSupervisorWithHolon(t, "holon-alpha")

	if got, ok := sup.Get("holon-alpha"); !ok || got.ID() != "holon-alpha" {
		t.Fatalf("expected to get holon-alpha, got %v", got)
	}

	if err := sup.Start("holon-alpha"); err != nil {
		t.Fatalf("failed to start holon: %v", err)
	}

	select {
	case <-h.startedChan:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for holon start")
	}

	statuses := sup.Status()
	if len(statuses) != 1 || statuses[0].Health.State != HolonStateRunning {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}

	if err := sup.Stop("holon-alpha"); err != nil {
		t.Fatalf("failed to stop holon: %v", err)
	}

	// Idempotent stop check
	if err := sup.Stop("holon-alpha"); err != nil {
		t.Fatalf("subsequent stop failed: %v", err)
	}

	if err := sup.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
}

func TestHolonSupervisor_NegativeRegistration(t *testing.T) {
	sup := NewHolonSupervisor(context.Background())

	if err := sup.Register(nil); err == nil {
		t.Fatal("expected error registering nil holon")
	}

	emptyID := &mockHolon{id: ""}
	if err := sup.Register(emptyID); err == nil {
		t.Fatal("expected error registering empty ID holon")
	}

	valid := newMockHolon("h-unique", "spinal-cord")
	if err := sup.Register(valid); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	if err := sup.Register(valid); err == nil {
		t.Fatal("expected error on duplicate registration")
	}

	if err := sup.Start("nonexistent"); err == nil {
		t.Fatal("expected error starting nonexistent holon")
	}

	if err := sup.Stop("nonexistent"); err == nil {
		t.Fatal("expected error stopping nonexistent holon")
	}
}

func TestHolonSupervisor_PanicRecovery(t *testing.T) {
	sup := NewHolonSupervisor(nil)
	panicky := newMockHolon("panicky-holon", "amygdala")
	panicky.startFn = func(ctx context.Context) error {
		panic("simulated fatal holon panic")
	}

	if err := sup.Register(panicky); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	if err := sup.Start(panicky.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Wait for panic handler to execute and cleanup cancelFns
	time.Sleep(100 * time.Millisecond)

	// Supervisor shutdown must succeed cleanly with zero hangs
	if err := sup.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown after panic failed: %v", err)
	}
}

func TestHolonSupervisor_ConcurrentAccess(t *testing.T) {
	sup := NewHolonSupervisor(nil)
	var wg sync.WaitGroup
	var registered atomic.Int64

	for i := 0; i < 20; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_holon_concurrent", "concurrent holon test registration").StartSimple(func() {
			defer wg.Done()
			id := "concurrent-holon-" + time.Now().Format("150405.000000000") + "-" + string(rune('a'+i))
			h := newMockHolon(id, "neural-mesh")
			if err := sup.Register(h); err == nil {
				registered.Add(1)
				_ = sup.Start(id)
			}
		})
	}

	wg.Wait()

	if registered.Load() == 0 {
		t.Fatal("expected at least one registered holon")
	}

	if err := sup.Shutdown(3 * time.Second); err != nil {
		t.Fatalf("shutdown after concurrency test failed: %v", err)
	}
}

func TestHolonSupervisor_StartAlreadyRunningAndStoppedSupervisor(t *testing.T) {
	sup, _ := setupSupervisorWithHolon(t, "h-stopped-test")

	if err := sup.Start("h-stopped-test"); err != nil {
		t.Fatalf("initial start failed: %v", err)
	}

	// Start already running
	if err := sup.Start("h-stopped-test"); err == nil {
		t.Fatal("expected error starting already running holon")
	}

	if err := sup.Shutdown(2 * time.Second); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	// Registering or starting on stopped supervisor
	hLate := newMockHolon("h-late", "limbic")
	if err := sup.Register(hLate); err == nil {
		t.Fatal("expected error registering on stopped supervisor")
	}
	if err := sup.Start("h-stopped-test"); err == nil {
		t.Fatal("expected error starting on stopped supervisor")
	}

	// Subsequent shutdown is safe
	if err := sup.Shutdown(time.Second); err != nil {
		t.Fatalf("subsequent shutdown failed: %v", err)
	}
}

func TestHolonSupervisor_StopFailurePropagation(t *testing.T) {
	sup := NewHolonSupervisor(nil)
	failing := newMockHolon("failing-holon", "cerebellum")
	failing.stopFn = func() error {
		return errors.New("underlying holon failed to unmount")
	}

	if err := sup.Register(failing); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if err := sup.Start("failing-holon"); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if err := sup.Stop("failing-holon"); err == nil {
		t.Fatal("expected stop error propagation")
	}

	_ = sup.Shutdown(time.Second)
}
