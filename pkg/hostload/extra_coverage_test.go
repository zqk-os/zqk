// BLI-STARTER-COMMUNITY-027 / PRI-STARTER-COMMUNITY-027 coverage elevation
package hostload

import (
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestSnapshotLevel_StarveAndTightFromLoad(t *testing.T) {
	t.Parallel()
	starve := Snapshot{NCPU: 4, Load1: 8, IdleFraction: 0.5}
	if starve.Level() != LevelStarve {
		t.Fatalf("load starve: %v", starve.Level())
	}
	tight := Snapshot{NCPU: 4, Load1: 4.1, IdleFraction: 0.5}
	if tight.Level() != LevelTight {
		t.Fatalf("load tight: %v", tight.Level())
	}
	idleStarveSnap := Snapshot{NCPU: 4, Load1: 0, IdleFraction: 0.01}
	if idleStarveSnap.Level() != LevelStarve {
		t.Fatalf("idle starve: %v", idleStarveSnap.Level())
	}
}

func TestExcludingSelf_Clamps(t *testing.T) {
	t.Parallel()
	s := Snapshot{NCPU: 2, Load1: 1, IdleFraction: 0.9}
	got := s.excludingSelf(10)
	if got.IdleFraction != 1 {
		t.Fatalf("idle clamp: %v", got.IdleFraction)
	}
	if got.Load1 != 0 {
		t.Fatalf("load clamp: %v", got.Load1)
	}
	if s.excludingSelf(0).Load1 != 1 {
		t.Fatal("zero selfUnits is no-op")
	}
}

func TestCurrentLevelExcluding_Disabled(t *testing.T) {
	t.Setenv(zqkenv.HostloadDisable().Name(), "1")
	if CurrentLevel() != LevelHeadroom {
		t.Fatal("disabled CurrentLevel")
	}
	if CurrentLevelExcluding(8) != LevelHeadroom {
		t.Fatal("disabled CurrentLevelExcluding")
	}
}

func TestReadLoadAvgAndCPUTicks(t *testing.T) {
	t.Parallel()
	load, loadOK := readLoadAvg()
	if loadOK && load < 0 {
		t.Fatalf("negative load: %v", load)
	}
	idle, total, tickOK := readCPUTicks()
	if tickOK && total < idle {
		t.Fatalf("ticks idle=%d total=%d", idle, total)
	}
	_ = CurrentLevel()
	_ = CurrentLevelExcluding(1)
}

func TestOSSamplerCachesActiveSampler(t *testing.T) {
	a := activeSampler()
	b := activeSampler()
	if a != b {
		t.Fatal("activeSampler should cache")
	}
	snap := a.Snapshot()
	if snap.NCPU < 1 {
		t.Fatal("ncpu")
	}
	if snap.IdleFraction != 1 {
		t.Fatalf("under go test idle should be 1, got %v", snap.IdleFraction)
	}
}

func TestOSSampler_snapshotFromSampleEMA(t *testing.T) {
	t.Parallel()
	now := time.Now()
	s := newOSSampler()
	first := s.snapshotFromSample(now, 4, 1, false, 10, 20, true)
	if first.Load1 != -1 {
		t.Fatalf("load fail-open: %v", first.Load1)
	}
	if first.IdleFraction != -1 {
		t.Fatalf("first tick has no delta: %v", first.IdleFraction)
	}
	second := s.snapshotFromSample(now, 4, 0.5, true, 18, 40, true)
	if second.IdleFraction <= 0 {
		t.Fatalf("first EMA from delta idle: %v", second.IdleFraction)
	}
	third := s.snapshotFromSample(now, 4, 0.5, true, 26, 60, true)
	if third.IdleFraction <= 0 {
		t.Fatalf("blended EMA: %v", third.IdleFraction)
	}
	stuck := s.snapshotFromSample(now, 4, 0.5, true, 26, 60, true)
	if stuck.IdleFraction != -1 {
		t.Fatalf("no progress in ticks: %v", stuck.IdleFraction)
	}
	bad := s.snapshotFromSample(now, 4, 0.5, true, 0, 0, false)
	if bad.IdleFraction != -1 {
		t.Fatalf("tick fail: %v", bad.IdleFraction)
	}
}

func TestOverBudget_ZeroCapacity(t *testing.T) {
	t.Parallel()
	if OverBudget(99, 0) {
		t.Fatal("zero capacity must not over-budget")
	}
	if OverBudgetExcludingSelf(99, 0) {
		t.Fatal("zero capacity excluding-self")
	}
	if OverBudgetExcludingSelf(0, 8) {
		t.Fatal("want 0 is never over budget")
	}
	if OverBudgetExcludingSelf(-1, 8) {
		t.Fatal("negative want should clamp self and not panic")
	}
}

func TestScale_TightAndStarve(t *testing.T) {
	t.Parallel()
	if Scale(8, LevelStarve) != 1 {
		t.Fatal("starve")
	}
	if Scale(8, LevelTight) != 4 {
		t.Fatal("tight")
	}
	if Scale(1, LevelStarve) != 1 {
		t.Fatal("n<=1 unchanged")
	}
}
