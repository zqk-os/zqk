package liveness_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/liveness"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestPulsePacket_StaticBounds verifies Static Floor Invariants:
// Valid packets encode/decode cleanly; invalid packets fail validation.
func TestPulsePacket_StaticBounds(t *testing.T) {
	// 1. Valid packet creation and validation
	pkt := liveness.NewPulsePacket("worker-alpha", 12345, 1000, "phase_test")
	if err := pkt.Validate(); err != nil {
		t.Fatalf("expected valid packet, got error: %v", err)
	}

	encoded, err := pkt.Encode()
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	decoded, err := liveness.DecodePulsePacket(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.HolonID != "worker-alpha" {
		t.Errorf("expected holon_id worker-alpha, got %s", decoded.HolonID)
	}
	if decoded.PID != 12345 {
		t.Errorf("expected pid 12345, got %d", decoded.PID)
	}
	if decoded.CadenceMS != 1000 {
		t.Errorf("expected cadence_ms 1000, got %d", decoded.CadenceMS)
	}
	if decoded.Phase != "phase_test" {
		t.Errorf("expected phase phase_test, got %s", decoded.Phase)
	}

	// 2. Negative boundary: empty HolonID
	invalid1 := liveness.NewPulsePacket("", 123, 1000, "idle")
	if err := invalid1.Validate(); err == nil {
		t.Errorf("expected error for empty holon_id, got nil")
	}

	// 3. Negative boundary: invalid PID
	invalid2 := liveness.NewPulsePacket("worker-1", 0, 1000, "idle")
	if err := invalid2.Validate(); err == nil {
		t.Errorf("expected error for zero pid, got nil")
	}

	// 4. Negative boundary: cadence out of bounds (< MinCadenceMS)
	invalid3 := liveness.NewPulsePacket("worker-1", 123, 10, "idle")
	if err := invalid3.Validate(); err == nil {
		t.Errorf("expected error for cadence below minimum, got nil")
	}

	// 5. Decode malformed bytes
	if _, err := liveness.DecodePulsePacket([]byte("{malformed json")); err == nil {
		t.Errorf("expected error decoding malformed bytes, got nil")
	}
	if _, err := liveness.DecodePulsePacket(nil); err == nil {
		t.Errorf("expected error decoding nil bytes, got nil")
	}
}

func tempSocketPath(t *testing.T) string {
	d, err := fileutil.MkdirTemp("/tmp", "zqk_liv_*")
	if err != nil {
		t.Fatalf("failed to create short temp dir: %v", err)
	}
	t.Cleanup(func() {
		if rmErr := fileutil.RemoveAll(d); rmErr != nil {
			t.Logf("cleanup temp dir failed: %v", rmErr)
		}
	})
	return filepath.Join(d, "liv.sock")
}

func setupTestListener(t *testing.T, tracker *liveness.DecayTracker) (string, *liveness.PulseListener, context.Context, context.CancelFunc) {
	t.Helper()
	sockPath := tempSocketPath(t)
	if tracker == nil {
		tracker = liveness.NewDecayTracker(nil, nil)
	}
	listener := liveness.NewPulseListener(sockPath, tracker)
	ctx, cancel := context.WithCancel(context.Background())
	if err := listener.Start(ctx); err != nil {
		cancel()
		t.Fatalf("failed to start listener: %v", err)
	}
	t.Cleanup(func() {
		if stopErr := listener.Stop(); stopErr != nil {
			t.Logf("failed to stop listener: %v", stopErr)
		}
	})
	return sockPath, listener, ctx, cancel
}

// TestPulseEmitter_NonBlockingMissingSocket verifies Operational & Negative Boundary:
// Emitting to a nonexistent socket returns an immediate error without blocking or panicking.
func TestPulseEmitter_NonBlockingMissingSocket(t *testing.T) {
	nonExistentSocket := filepath.Join(t.TempDir(), "nonexistent.sock")
	emitter := liveness.NewEmitter(nonExistentSocket)

	pkt := liveness.NewPulsePacket("worker-standalone", os.Getpid(), 1000, "unit_test")

	start := time.Now()
	err := emitter.Emit(pkt)
	duration := time.Since(start)

	if err == nil {
		t.Fatalf("expected dial error to missing socket, got nil")
	}

	// Guarantee zero-blocking: dial timeout is 50ms, so duration should be under 200ms
	if duration > 200*time.Millisecond {
		t.Errorf("emission to missing socket blocked too long: %v", duration)
	}
}

// TestPulseBus_LoopbackAndDecay verifies Operational Dynamic Proof:
// Listener receives pulses, decay tracker records them, and sweeps detect ischemia when pulses halt.
func TestPulseBus_LoopbackAndDecay(t *testing.T) {
	var ischemiaCount int32
	var reperfusionCount int32
	var lastIschemicID string
	var mu sync.Mutex

	tracker := liveness.NewDecayTracker(
		func(rec *liveness.HolonRecord) {
			atomic.AddInt32(&ischemiaCount, 1)
			mu.Lock()
			lastIschemicID = rec.LastPacket.HolonID
			mu.Unlock()
		},
		func(rec *liveness.HolonRecord) {
			atomic.AddInt32(&reperfusionCount, 1)
		},
	)

	sockPath, _, _, cancel := setupTestListener(t, tracker)
	defer cancel()

	emitter := liveness.NewEmitter(sockPath)

	// Send pulse for holon-1 with very short cadence (50ms, ischemia threshold = 100ms)
	pkt1 := liveness.NewPulsePacket("holon-test-1", os.Getpid(), 50, "init")
	if err := emitter.Emit(pkt1); err != nil {
		t.Fatalf("failed to emit pulse: %v", err)
	}

	// Wait briefly for datagram to be read
	time.Sleep(50 * time.Millisecond)

	// Verify pulse recorded in tracker
	vitals := tracker.VitalitySnapshot()
	if len(vitals) != 1 {
		t.Fatalf("expected 1 holon in vitality snapshot, got %d", len(vitals))
	}
	if vitals[0].HolonID != "holon-test-1" {
		t.Errorf("expected holon-test-1, got %s", vitals[0].HolonID)
	}
	if vitals[0].Ischemic {
		t.Errorf("holon should be active, not ischemic")
	}

	// Advance time past deadline (100ms threshold) and sweep
	futureTime := time.Now().Add(250 * time.Millisecond)
	breached := tracker.Sweep(futureTime)

	if len(breached) != 1 || breached[0] != "holon-test-1" {
		t.Errorf("expected sweep to catch holon-test-1 breach, got %v", breached)
	}

	// Allow background callback to run
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&ischemiaCount) != 1 {
		t.Errorf("expected ischemia handler to fire 1 time, got %d", atomic.LoadInt32(&ischemiaCount))
	}
	mu.Lock()
	if lastIschemicID != "holon-test-1" {
		t.Errorf("expected lastIschemicID holon-test-1, got %s", lastIschemicID)
	}
	mu.Unlock()

	// Reperfusion test: emit a new pulse for the ischemic holon
	pkt2 := liveness.NewPulsePacket("holon-test-1", os.Getpid(), 50, "recovered")
	if err := emitter.Emit(pkt2); err != nil {
		t.Fatalf("failed to emit reperfusion pulse: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&reperfusionCount) != 1 {
		t.Errorf("expected reperfusion handler to fire 1 time, got %d", atomic.LoadInt32(&reperfusionCount))
	}

	vitalsAfter := tracker.VitalitySnapshot()
	if len(vitalsAfter) != 1 || vitalsAfter[0].Ischemic {
		t.Errorf("holon should have recovered from ischemia after reperfusion pulse")
	}
}

// TestPulseTicker_AutonomousLifecycle verifies that PulseTicker runs autonomously and stops cleanly.
func TestPulseTicker_AutonomousLifecycle(t *testing.T) {
	tracker := liveness.NewDecayTracker(nil, nil)
	sockPath, _, ctx, cancel := setupTestListener(t, tracker)
	defer cancel()

	ticker := liveness.StartPulseTicker(ctx, sockPath, "ticker-worker", "phase_1", 50)
	time.Sleep(120 * time.Millisecond)

	ticker.UpdatePhase("phase_2")
	time.Sleep(120 * time.Millisecond)

	ticker.Stop()

	vitals := tracker.VitalitySnapshot()
	if len(vitals) != 1 {
		t.Fatalf("expected 1 holon in snapshot, got %d", len(vitals))
	}
	if vitals[0].HolonID != "ticker-worker" {
		t.Errorf("expected ticker-worker, got %s", vitals[0].HolonID)
	}
	if vitals[0].Phase != "phase_2" {
		t.Errorf("expected updated phase_2, got %s", vitals[0].Phase)
	}
	if vitals[0].PulseCount < 2 {
		t.Errorf("expected at least 2 pulses received, got %d", vitals[0].PulseCount)
	}
}

// TestPulseBus_HelpersAndProjectResolution tests convenience helpers, project socket routing, and removal.
func TestPulseBus_HelpersAndProjectResolution(t *testing.T) {
	tracker := liveness.NewDecayTracker(nil, nil)
	sockPath, listener, ctx, cancel := setupTestListener(t, tracker)
	defer cancel()

	if listener.Tracker() == nil {
		t.Errorf("expected non-nil tracker from listener")
	}

	// Test EmitQuick
	if err := liveness.EmitQuick(sockPath, "quick-holon", "executing", 100); err != nil {
		t.Fatalf("EmitQuick failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	vitals := tracker.VitalitySnapshot()
	if len(vitals) != 1 || vitals[0].HolonID != "quick-holon" {
		t.Fatalf("expected quick-holon in vitality snapshot, got %v", vitals)
	}

	// Test Tracker Remove
	tracker.Remove("quick-holon")
	if len(tracker.VitalitySnapshot()) != 0 {
		t.Errorf("expected empty vitality snapshot after remove")
	}

	// Test project path helper constructor wiring
	projRoot := t.TempDir()
	projEmitter := liveness.NewEmitterForProject(projRoot)
	if projEmitter == nil {
		t.Errorf("expected non-nil project emitter")
	}

	projListener := liveness.NewProjectPulseListener(projRoot, nil)
	if projListener == nil {
		t.Errorf("expected non-nil project listener")
	}

	pt := liveness.StartPulseTickerForProject(ctx, projRoot, "proj-worker", "idle", 100)
	if pt == nil {
		t.Errorf("expected non-nil project ticker")
	}
	pt.Stop()
}
