package hostload

import (
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestHostloadLevel_TransitionsAndEdgeScales(t *testing.T) {
	t.Parallel()

	// Verify constant ordering
	if LevelHeadroom >= LevelTight || LevelTight >= LevelStarve {
		t.Fatalf("expected LevelHeadroom < LevelTight < LevelStarve, got %d, %d, %d",
			LevelHeadroom, LevelTight, LevelStarve)
	}

	testCases := []struct {
		name     string
		n        int
		level    Level
		expected int
	}{
		{name: "negative n headroom", n: -5, level: LevelHeadroom, expected: -5},
		{name: "negative n tight", n: -5, level: LevelTight, expected: -5},
		{name: "negative n starve", n: -5, level: LevelStarve, expected: -5},
		{name: "zero n headroom", n: 0, level: LevelHeadroom, expected: 0},
		{name: "zero n tight", n: 0, level: LevelTight, expected: 0},
		{name: "zero n starve", n: 0, level: LevelStarve, expected: 0},
		{name: "one n headroom", n: 1, level: LevelHeadroom, expected: 1},
		{name: "one n tight", n: 1, level: LevelTight, expected: 1},
		{name: "one n starve", n: 1, level: LevelStarve, expected: 1},
		{name: "two n headroom", n: 2, level: LevelHeadroom, expected: 2},
		{name: "two n tight", n: 2, level: LevelTight, expected: 1},
		{name: "two n starve", n: 2, level: LevelStarve, expected: 1},
		{name: "three n tight", n: 3, level: LevelTight, expected: 1},
		{name: "four n tight", n: 4, level: LevelTight, expected: 2},
		{name: "five n tight", n: 5, level: LevelTight, expected: 2},
		{name: "eight n headroom", n: 8, level: LevelHeadroom, expected: 8},
		{name: "eight n tight", n: 8, level: LevelTight, expected: 4},
		{name: "eight n starve", n: 8, level: LevelStarve, expected: 1},
		{name: "unknown level default", n: 8, level: Level(99), expected: 8},
	}

	for _, tc := range testCases {
		got := Scale(tc.n, tc.level)
		if got != tc.expected {
			t.Errorf("Scale(%d, %v) [%s] = %d; want %d", tc.n, tc.level, tc.name, got, tc.expected)
		}
	}
}

func TestHostloadSnapshot_LevelMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		snap     Snapshot
		expected Level
	}{
		{
			name:     "both unavailable fails open to headroom",
			snap:     Snapshot{NCPU: 4, Load1: -1, IdleFraction: -1},
			expected: LevelHeadroom,
		},
		{
			name:     "zero ncpu ignores load and uses idle",
			snap:     Snapshot{NCPU: 0, Load1: 10, IdleFraction: 0.5},
			expected: LevelHeadroom,
		},
		{
			name:     "negative ncpu ignores load",
			snap:     Snapshot{NCPU: -1, Load1: 10, IdleFraction: 0.15},
			expected: LevelTight,
		},
		{
			name:     "idle headroom and load headroom",
			snap:     Snapshot{NCPU: 4, Load1: 2.0, IdleFraction: 0.50},
			expected: LevelHeadroom,
		},
		{
			name:     "idle headroom and load tight",
			snap:     Snapshot{NCPU: 4, Load1: 4.5, IdleFraction: 0.50},
			expected: LevelTight,
		},
		{
			name:     "idle headroom and load starve",
			snap:     Snapshot{NCPU: 4, Load1: 7.0, IdleFraction: 0.50},
			expected: LevelStarve,
		},
		{
			name:     "idle tight and load headroom",
			snap:     Snapshot{NCPU: 4, Load1: 2.0, IdleFraction: 0.15},
			expected: LevelTight,
		},
		{
			name:     "idle starve and load headroom",
			snap:     Snapshot{NCPU: 4, Load1: 2.0, IdleFraction: 0.05},
			expected: LevelStarve,
		},
		{
			name:     "idle tight and load starve",
			snap:     Snapshot{NCPU: 4, Load1: 8.0, IdleFraction: 0.15},
			expected: LevelStarve,
		},
		{
			name:     "idle boundary exactly idleTight 0.25 is headroom",
			snap:     Snapshot{NCPU: 4, Load1: 0, IdleFraction: 0.25},
			expected: LevelHeadroom,
		},
		{
			name:     "idle boundary just below idleTight is tight",
			snap:     Snapshot{NCPU: 4, Load1: 0, IdleFraction: 0.249},
			expected: LevelTight,
		},
		{
			name:     "idle boundary exactly idleStarve 0.08 is tight",
			snap:     Snapshot{NCPU: 4, Load1: 0, IdleFraction: 0.08},
			expected: LevelTight,
		},
		{
			name:     "idle boundary just below idleStarve is starve",
			snap:     Snapshot{NCPU: 4, Load1: 0, IdleFraction: 0.079},
			expected: LevelStarve,
		},
		{
			name:     "load boundary exactly loadTight 1.0 is tight",
			snap:     Snapshot{NCPU: 4, Load1: 4.0, IdleFraction: -1},
			expected: LevelTight,
		},
		{
			name:     "load boundary exactly loadStarve 1.6 is starve",
			snap:     Snapshot{NCPU: 4, Load1: 6.4, IdleFraction: -1},
			expected: LevelStarve,
		},
	}

	for _, tc := range tests {
		got := tc.snap.Level()
		if got != tc.expected {
			t.Errorf("[%s] snap.Level() = %v; want %v", tc.name, got, tc.expected)
		}
	}
}

func TestHostloadSnapshot_ExcludingSelf_Boundaries(t *testing.T) {
	t.Parallel()

	s := Snapshot{
		NCPU:         4,
		Load1:        3.0,
		IdleFraction: 0.20,
		SampledAt:    time.Now(),
	}

	// selfUnits <= 0 is a no-op
	noOpNegative := s.excludingSelf(-2)
	if noOpNegative.IdleFraction != s.IdleFraction || noOpNegative.Load1 != s.Load1 {
		t.Fatalf("negative selfUnits should be no-op, got %+v", noOpNegative)
	}

	// NCPU <= 0 is a no-op
	badNCPU := Snapshot{NCPU: 0, Load1: 3.0, IdleFraction: 0.20}
	noOpNCPU := badNCPU.excludingSelf(2)
	if noOpNCPU.IdleFraction != badNCPU.IdleFraction || noOpNCPU.Load1 != badNCPU.Load1 {
		t.Fatalf("zero NCPU should be no-op, got %+v", noOpNCPU)
	}

	// Valid exclusion with clamping
	adjusted := s.excludingSelf(2) // frac = 2/4 = 0.50
	expectedIdle := 0.20 + 0.50
	expectedLoad := 3.0 - 2.0
	if adjusted.IdleFraction != expectedIdle {
		t.Errorf("expected IdleFraction %.2f, got %.2f", expectedIdle, adjusted.IdleFraction)
	}
	if adjusted.Load1 != expectedLoad {
		t.Errorf("expected Load1 %.2f, got %.2f", expectedLoad, adjusted.Load1)
	}

	// Clamp IdleFraction to 1.0 and Load1 to 0.0
	overAdjusted := s.excludingSelf(10) // frac = 10/4 = 2.50
	if overAdjusted.IdleFraction != 1.0 {
		t.Errorf("expected IdleFraction clamped to 1.0, got %.2f", overAdjusted.IdleFraction)
	}
	if overAdjusted.Load1 != 0.0 {
		t.Errorf("expected Load1 clamped to 0.0, got %.2f", overAdjusted.Load1)
	}

	// Negative values in snapshot remain negative (unavailable)
	unavailSnap := Snapshot{NCPU: 4, Load1: -1, IdleFraction: -1}
	unavailAdj := unavailSnap.excludingSelf(2)
	if unavailAdj.IdleFraction != -1 || unavailAdj.Load1 != -1 {
		t.Errorf("unavailable values must remain -1, got %+v", unavailAdj)
	}
}

func TestHostloadSampler_EMASmoothingAndDecay(t *testing.T) {
	t.Parallel()

	sampler := newOSSampler()
	now := time.Now()
	ncpu := 4

	// Tick 1: First baseline sample, no prior delta -> IdleFraction should be -1
	snap1 := sampler.snapshotFromSample(now, ncpu, 1.0, true, 100, 200, true)
	if snap1.IdleFraction != -1.0 {
		t.Fatalf("tick 1 IdleFraction want -1.0, got %f", snap1.IdleFraction)
	}
	if snap1.Load1 != 1.0 {
		t.Fatalf("tick 1 Load1 want 1.0, got %f", snap1.Load1)
	}

	// Tick 2: Delta total = 100, delta idle = 40 (idle frac = 0.40)
	snap2 := sampler.snapshotFromSample(now, ncpu, 1.0, true, 140, 300, true)
	if snap2.IdleFraction < 0.399 || snap2.IdleFraction > 0.401 {
		t.Fatalf("tick 2 IdleFraction want 0.40, got %f", snap2.IdleFraction)
	}

	// Tick 3: Delta total = 100, delta idle = 80 (idle frac = 0.80)
	// EMA calculation: 0.5 * 0.80 + 0.5 * 0.40 = 0.60
	snap3 := sampler.snapshotFromSample(now, ncpu, 1.0, true, 220, 400, true)
	if snap3.IdleFraction < 0.599 || snap3.IdleFraction > 0.601 {
		t.Fatalf("tick 3 IdleFraction want 0.60, got %f", snap3.IdleFraction)
	}

	// Tick 4: Stagnant tick (total <= prevTot) -> returns -1.0
	snapStagnant := sampler.snapshotFromSample(now, ncpu, 1.0, true, 220, 400, true)
	if snapStagnant.IdleFraction != -1.0 {
		t.Fatalf("stagnant tick IdleFraction want -1.0, got %f", snapStagnant.IdleFraction)
	}

	// Tick 5: tickOK false -> returns -1.0
	snapTickBad := sampler.snapshotFromSample(now, ncpu, 1.0, true, 0, 0, false)
	if snapTickBad.IdleFraction != -1.0 {
		t.Fatalf("bad tick IdleFraction want -1.0, got %f", snapTickBad.IdleFraction)
	}

	// Tick 6: loadOK false -> Load1 set to -1
	snapLoadBad := sampler.snapshotFromSample(now, ncpu, 2.5, false, 280, 500, true)
	if snapLoadBad.Load1 != -1.0 {
		t.Fatalf("bad load Load1 want -1.0, got %f", snapLoadBad.Load1)
	}
}

func TestHostloadSampler_ConcurrentAccessSafety(t *testing.T) {
	t.Parallel()

	sampler := newOSSampler()
	var wg sync.WaitGroup
	workers := 16
	iterations := 50

	for i := 0; i < workers; i++ {
		wg.Add(1)
		workerID := i
		goroutinelabels.NewGoroutine("hostload_test", "concurrent sampling worker").StartSimple(func() {
			defer wg.Done()
			for j := 1; j <= iterations; j++ {
				idle := uint64(workerID*1000 + j*10)
				total := uint64(workerID*1000 + j*20)
				snap := sampler.snapshotFromSample(
					time.Now(),
					4,
					float64(workerID)/2.0,
					true,
					idle,
					total,
					true,
				)
				if snap.NCPU != 4 {
					t.Errorf("worker %d unexpected ncpu: %d", workerID, snap.NCPU)
				}
			}
		})
	}

	wg.Wait()
}

func TestHostloadOverBudget_ComprehensiveBoundaries(t *testing.T) {
	// Zero or negative capacity is never over budget
	if OverBudget(10, 0) {
		t.Error("OverBudget(10, 0) should be false")
	}
	if OverBudget(10, -2) {
		t.Error("OverBudget(10, -2) should be false")
	}
	if OverBudgetExcludingSelf(10, 0) {
		t.Error("OverBudgetExcludingSelf(10, 0) should be false")
	}
	if OverBudgetExcludingSelf(10, -5) {
		t.Error("OverBudgetExcludingSelf(10, -5) should be false")
	}

	// Test with explicit fixed sampler:
	// IdleFraction = 0.02 (starve), Load1 = 8.0 (on 8 cores, ratio = 1.0 -> tight)
	// Combined Level() is LevelStarve.
	starveSnap := Snapshot{NCPU: 8, Load1: 8.0, IdleFraction: 0.02, SampledAt: time.Now()}
	setSamplerForTest(fixedSampler{snap: starveSnap})
	t.Cleanup(func() { setSamplerForTest(nil) })

	// Under Starve, Scale(8, Starve) is 1.
	if OverBudget(1, 8) {
		t.Error("OverBudget(1, 8) under starve should be false (1 is permitted)")
	}
	if !OverBudget(2, 8) {
		t.Error("OverBudget(2, 8) under starve should be true")
	}

	// Under Starve with want=1, self=0:
	// CurrentLevelExcluding(0) is Starve, Scale(8, Starve) is 1.
	// want=1 <= 1, so false.
	if OverBudgetExcludingSelf(1, 8) {
		t.Error("OverBudgetExcludingSelf(1, 8) should be false")
	}
	// want=2, self=1:
	// frac=1/8=0.125. IdleFraction becomes 0.02 + 0.125 = 0.145 (LevelTight).
	// Load1 becomes 8.0 - 1 = 7.0 / 8 = 0.875 (LevelHeadroom).
	// Worse of the two is LevelTight. Scale(8, Tight) = 4.
	// want=2 <= 4, so not over budget!
	if OverBudgetExcludingSelf(2, 8) {
		t.Error("OverBudgetExcludingSelf(2, 8) should be false because self occupancy softens starve to tight")
	}
}

func TestHostloadCurrentLevel_DisabledToggling(t *testing.T) {
	starveSnap := Snapshot{NCPU: 4, Load1: 12.0, IdleFraction: 0.01, SampledAt: time.Now()}
	setSamplerForTest(fixedSampler{snap: starveSnap})
	t.Cleanup(func() { setSamplerForTest(nil) })

	// Normal active sensing reflects Starve
	if CurrentLevel() != LevelStarve {
		t.Fatalf("expected LevelStarve when enabled, got %v", CurrentLevel())
	}
	if CurrentLevelExcluding(0) != LevelStarve {
		t.Fatalf("expected LevelStarve when enabled, got %v", CurrentLevelExcluding(0))
	}

	// Disable via env
	t.Setenv(zqkenv.HostloadDisable().Name(), "1")
	if !Disabled() {
		t.Fatal("expected Disabled() to be true when env var is set")
	}
	if CurrentLevel() != LevelHeadroom {
		t.Fatalf("expected LevelHeadroom when disabled, got %v", CurrentLevel())
	}
	if CurrentLevelExcluding(0) != LevelHeadroom {
		t.Fatalf("expected LevelHeadroom when disabled, got %v", CurrentLevelExcluding(0))
	}

	// Re-enable via empty env
	t.Setenv(zqkenv.HostloadDisable().Name(), "")
	if Disabled() {
		t.Fatal("expected Disabled() to be false when env var is cleared")
	}
	if CurrentLevel() != LevelStarve {
		t.Fatalf("expected LevelStarve after re-enabling, got %v", CurrentLevel())
	}
}
