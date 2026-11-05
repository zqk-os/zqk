package ambient

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/liveness"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	testLivenessWaitTimeout = 3 * time.Second
	testLivenessPollTick    = 25 * time.Millisecond
	testLivenessCadenceFast = 60
)

func newTestLivenessEngine(t *testing.T) (*AmbientEngine, string) {
	t.Helper()
	tmpDir, err := fileutil.MkdirTemp("/tmp", "zqk_liv_*")
	if err != nil {
		t.Fatalf("failed to create short temp dir: %v", err)
	}
	t.Cleanup(func() {
		if rmErr := fileutil.RemoveAll(tmpDir); rmErr != nil {
			t.Logf("cleanup temp dir failed: %v", rmErr)
		}
	})
	sockPath := filepath.Join(tmpDir, "liv.sock")

	cfg := DefaultAmbientEngineConfig()
	cfg.EnableLiveness = true
	cfg.LivenessSocketPath = sockPath

	eng, err := NewAmbientEngine(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	return eng, sockPath
}

func startTestLivenessEngineWithSetup(t *testing.T, setup func(*AmbientEngine)) (*AmbientEngine, string, context.CancelFunc) {
	t.Helper()
	eng, sockPath := newTestLivenessEngine(t)
	if setup != nil {
		setup(eng)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := eng.Start(ctx); err != nil {
		cancel()
		t.Fatalf("engine start failed: %v", err)
	}
	t.Cleanup(func() {
		if stopErr := eng.Stop(); stopErr != nil {
			// Engine may already be stopped in rebind test
		}
	})
	return eng, sockPath, cancel
}

func startTestLivenessEngine(t *testing.T) (*AmbientEngine, string, context.CancelFunc) {
	return startTestLivenessEngineWithSetup(t, nil)
}

func emitTestPulse(t *testing.T, sockPath, holonID string, cadence int, phase string) {
	t.Helper()
	emitter := liveness.NewEmitter(sockPath)
	pkt := liveness.NewPulsePacket(holonID, os.Getpid(), cadence, phase)
	if err := emitter.Emit(pkt); err != nil {
		t.Fatalf("pulse emit failed: %v", err)
	}
}

func TestAmbientEngine_LivenessHostingStartupShutdown(t *testing.T) {
	t.Run("StartupAndCleanShutdown", testLivenessStartupShutdown)
	t.Run("NilReceiverSafety", testLivenessNilSafety)
}

func testLivenessStartupShutdown(t *testing.T) {
	eng, sockPath := newTestLivenessEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := eng.Start(ctx); err != nil {
		t.Fatalf("engine start failed: %v", err)
	}

	if !eng.IsRunning() || eng.LivenessListener() == nil || eng.DecayTracker() == nil {
		t.Fatal("expected running engine with initialized liveness listener and tracker")
	}

	if _, err := os.Stat(sockPath); err != nil {
		t.Fatalf("expected liveness socket file to exist: %v", err)
	}

	if err := eng.Stop(); err != nil {
		t.Fatalf("engine stop failed: %v", err)
	}

	if eng.State() != EngineStateStopped {
		t.Fatalf("expected engine state stopped, got: %s", eng.State())
	}
}

func testLivenessNilSafety(t *testing.T) {
	var nilEngine *AmbientEngine
	if nilEngine.LivenessListener() != nil || nilEngine.DecayTracker() != nil {
		t.Fatal("expected nil listener and tracker for nil engine")
	}
	if vitals := nilEngine.VitalitySnapshot(); vitals != nil {
		t.Fatal("expected nil vitals snapshot for nil engine")
	}
	if vitMap := nilEngine.VitalityMap(); vitMap != nil {
		t.Fatal("expected nil vitals map for nil engine")
	}
}

func TestAmbientEngine_LivenessPulseIngestAndVitality(t *testing.T) {
	t.Run("PulseIngestAndSnapshot", testLivenessPulseIngest)
	t.Run("ComponentHealthTelemetry", testLivenessComponentHealth)
}

func testLivenessPulseIngest(t *testing.T) {
	eng, sockPath, cancel := startTestLivenessEngine(t)
	defer cancel()

	holonID := "holon-test-alpha"
	pid := os.Getpid()
	emitTestPulse(t, sockPath, holonID, 200, "active_processing")

	deadline := time.Now().Add(testLivenessWaitTimeout)
	recorded := false
	for time.Now().Before(deadline) {
		vitMap := eng.VitalityMap()
		if v, ok := vitMap[holonID]; ok {
			if v.PID != pid || v.Ischemic {
				t.Fatalf("unexpected vitality state: %+v", v)
			}
			recorded = true
			break
		}
		time.Sleep(testLivenessPollTick)
	}

	if !recorded {
		t.Fatalf("timeout waiting for holon %s to be recorded in vitality snapshot", holonID)
	}
}

func testLivenessComponentHealth(t *testing.T) {
	eng, sockPath, cancel := startTestLivenessEngine(t)
	defer cancel()

	emitTestPulse(t, sockPath, "holon-health-probe", 300, "healthy")
	time.Sleep(50 * time.Millisecond)

	status := eng.Status()
	if _, ok := status.Subcomponents["liveness_listener"]; !ok {
		t.Fatal("expected liveness_listener in status subcomponents")
	}

	comps, count := eng.collectComponentHealth()
	if count < 1 {
		t.Fatalf("expected positive active holons, got %d", count)
	}
	cHealth, hasComp := comps["liveness_listener"]
	if !hasComp || cHealth.Status != TelemetryStatusHealthy {
		t.Fatalf("expected healthy liveness_listener in component health, got: %+v", cHealth)
	}
}

func TestAmbientEngine_LivenessIschemiaReflexTrigger(t *testing.T) {
	t.Run("IschemiaDetection", testLivenessIschemiaDetection)
	t.Run("ReperfusionRecovery", testLivenessReperfusionRecovery)
}

func testLivenessIschemiaDetection(t *testing.T) {
	actionCh := make(chan ReflexAction, 8)
	var handlerMu sync.Mutex
	var collected []ReflexAction

	eng, sockPath, cancel := startTestLivenessEngineWithSetup(t, func(e *AmbientEngine) {
		regErr := e.ReflexLoop().RegisterHandler(ReflexActionRestartHolon, func(_ context.Context, action ReflexAction) error {
			handlerMu.Lock()
			collected = append(collected, action)
			handlerMu.Unlock()
			select {
			case actionCh <- action:
			default:
			}
			return nil
		})
		if regErr != nil {
			t.Fatalf("failed to register reflex action handler: %v", regErr)
		}
	})
	defer cancel()

	holonID := "holon-ischemic-victim"
	emitTestPulse(t, sockPath, holonID, testLivenessCadenceFast, "failing")

	select {
	case act := <-actionCh:
		if act.TargetHolon != holonID || act.Type != ReflexActionRestartHolon {
			t.Fatalf("unexpected reflex action: %+v", act)
		}
	case <-time.After(testLivenessWaitTimeout):
		t.Fatalf("timed out waiting for reflex action on ischemic holon %s", holonID)
	}

	vitMap := eng.VitalityMap()
	if v, ok := vitMap[holonID]; !ok || !v.Ischemic {
		t.Fatalf("expected holon %s to be recorded as ischemic: %+v", holonID, v)
	}
}

func testLivenessReperfusionRecovery(t *testing.T) {
	eng, sockPath, cancel := startTestLivenessEngine(t)
	defer cancel()

	holonID := "holon-reperfusion-target"
	emitTestPulse(t, sockPath, holonID, testLivenessCadenceFast, "phase1")

	deadline := time.Now().Add(testLivenessWaitTimeout)
	for time.Now().Before(deadline) {
		if v, ok := eng.VitalityMap()[holonID]; ok && v.Ischemic {
			break
		}
		time.Sleep(testLivenessPollTick)
	}

	if !eng.VitalityMap()[holonID].Ischemic {
		t.Fatalf("holon did not become ischemic within deadline")
	}

	emitTestPulse(t, sockPath, holonID, 500, "recovered")

	recovered := false
	recDeadline := time.Now().Add(testLivenessWaitTimeout)
	for time.Now().Before(recDeadline) {
		if v, ok := eng.VitalityMap()[holonID]; ok && !v.Ischemic {
			recovered = true
			break
		}
		time.Sleep(testLivenessPollTick)
	}

	if !recovered {
		t.Fatalf("holon failed to reperfuse after new pulse")
	}
}

func TestAmbientEngine_LivenessBoundaryRejections(t *testing.T) {
	t.Run("MalformedDatagramDiscard", testLivenessMalformedDiscard)
	t.Run("SocketUnbindAndRecreate", testLivenessSocketRebind)
	t.Run("PurgeAndQueryVitality", testLivenessPurgeAndQuery)
}

func testLivenessMalformedDiscard(t *testing.T) {
	eng, sockPath, cancel := startTestLivenessEngine(t)
	defer cancel()

	conn, dialErr := net.Dial("unixgram", sockPath)
	if dialErr != nil {
		t.Fatalf("failed to dial liveness socket: %v", dialErr)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Errorf("conn close failed: %v", closeErr)
		}
	}()

	invalidBytes := []byte("INVALID_NON_JSON_CORRUPT_DATAGRAM")
	if _, writeErr := conn.Write(invalidBytes); writeErr != nil {
		t.Fatalf("failed to write corrupt payload: %v", writeErr)
	}

	oversized := make([]byte, liveness.MaxPulsePacketBytes+512)
	for i := range oversized {
		oversized[i] = 'X'
	}
	if _, overErr := conn.Write(oversized); overErr != nil {
		// Kernel may reject oversized datagram
	}

	time.Sleep(50 * time.Millisecond)

	if !eng.IsRunning() {
		t.Fatal("engine crashed or halted after malformed datagrams")
	}
}

func testLivenessSocketRebind(t *testing.T) {
	eng, sockPath, cancel := startTestLivenessEngine(t)
	cancel()
	if stopErr := eng.Stop(); stopErr != nil {
		t.Fatalf("initial stop failed: %v", stopErr)
	}

	eng2, err := NewAmbientEngine(eng.Config())
	if err != nil {
		t.Fatalf("failed to create second engine: %v", err)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	if startErr := eng2.Start(ctx2); startErr != nil {
		t.Fatalf("rebind start failed: %v", startErr)
	}
	if _, statErr := os.Stat(sockPath); statErr != nil {
		t.Fatalf("expected re-bound socket to exist: %v", statErr)
	}
	if stopErr := eng2.Stop(); stopErr != nil {
		t.Fatalf("re-bound stop failed: %v", stopErr)
	}
}

func testLivenessPurgeAndQuery(t *testing.T) {
	eng, sockPath, cancel := startTestLivenessEngine(t)
	defer cancel()

	const targetHolon = "holon-purge-target"
	emitTestPulse(t, sockPath, targetHolon, 250, "running")

	deadline := time.Now().Add(testLivenessWaitTimeout)
	for time.Now().Before(deadline) {
		if _, ok := eng.HolonVitality(targetHolon); ok {
			break
		}
		time.Sleep(testLivenessPollTick)
	}

	vit, ok := eng.HolonVitality(targetHolon)
	if !ok || vit.HolonID != targetHolon {
		t.Fatalf("expected holon %s to be tracked, got: %+v (ok=%v)", targetHolon, vit, ok)
	}

	if _, untrackedOk := eng.HolonVitality("holon-non-existent"); untrackedOk {
		t.Fatal("expected untracked holon to return false")
	}

	if !eng.RemoveHolon(targetHolon) {
		t.Fatal("expected RemoveHolon to succeed")
	}

	if _, purgedOk := eng.HolonVitality(targetHolon); purgedOk {
		t.Fatal("expected purged holon to no longer be tracked")
	}

	var nilEng *AmbientEngine
	if nilEng.RemoveHolon(targetHolon) {
		t.Fatal("expected nil engine RemoveHolon to return false")
	}
	if _, nilOk := nilEng.HolonVitality(targetHolon); nilOk {
		t.Fatal("expected nil engine HolonVitality to return false")
	}
}
