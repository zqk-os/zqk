package hostload

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/brand"
)

func TestScale(t *testing.T) {
	t.Parallel()
	if Scale(8, LevelHeadroom) != 8 {
		t.Fatalf("headroom")
	}
	if Scale(8, LevelTight) != 4 {
		t.Fatalf("tight")
	}
	if Scale(8, LevelStarve) != 1 {
		t.Fatalf("starve")
	}
	if Scale(1, LevelStarve) != 1 {
		t.Fatalf("min stays 1")
	}
}

func TestSnapshotLevel_IdleAndLoad(t *testing.T) {
	t.Parallel()
	head := Snapshot{NCPU: 10, Load1: 2, IdleFraction: 0.5}
	if head.Level() != LevelHeadroom {
		t.Fatalf("got %v", head.Level())
	}
	tightIdle := Snapshot{NCPU: 10, Load1: 2, IdleFraction: 0.15}
	if tightIdle.Level() != LevelTight {
		t.Fatalf("tight idle got %v", tightIdle.Level())
	}
	starveIdle := Snapshot{NCPU: 10, Load1: 2, IdleFraction: 0.02}
	if starveIdle.Level() != LevelStarve {
		t.Fatalf("starve idle got %v", starveIdle.Level())
	}
	starveLoad := Snapshot{NCPU: 4, Load1: 8, IdleFraction: 0.9}
	if starveLoad.Level() != LevelStarve {
		t.Fatalf("starve load got %v", starveLoad.Level())
	}
}

type fixedSampler struct{ snap Snapshot }

func (f fixedSampler) Snapshot() Snapshot { return f.snap }

func TestOverBudget_UsesSampler(t *testing.T) {
	setSamplerForTest(fixedSampler{snap: Snapshot{
		NCPU: 8, Load1: 0, IdleFraction: 0.01, SampledAt: time.Now(),
	}})
	t.Cleanup(func() { setSamplerForTest(nil) })
	if !OverBudget(2, 8) {
		t.Fatal("starve should treat 2 of 8 as over budget")
	}
	if OverBudget(1, 8) {
		t.Fatal("one slot is the starve budget")
	}
}

func TestOverBudgetExcludingSelf_selfHeatKeepsSlots(t *testing.T) {
	setSamplerForTest(fixedSampler{snap: Snapshot{
		NCPU: 8, Load1: 7, IdleFraction: 0.05, SampledAt: time.Now(),
	}})
	t.Cleanup(func() { setSamplerForTest(nil) })
	if !OverBudget(8, 8) {
		t.Fatal("raw sample must still see self-heat as pressure")
	}
	if OverBudgetExcludingSelf(8, 8) {
		t.Fatal("held slots must not yield because we used the cores")
	}
}

func TestOverBudgetExcludingSelf_foreignStarveRefusesExtra(t *testing.T) {
	setSamplerForTest(fixedSampler{snap: Snapshot{
		NCPU: 8, Load1: 20, IdleFraction: 0.02, SampledAt: time.Now(),
	}})
	t.Cleanup(func() { setSamplerForTest(nil) })
	if OverBudgetExcludingSelf(1, 8) {
		t.Fatal("one unit must still make progress under starve")
	}
	if !OverBudgetExcludingSelf(2, 8) {
		t.Fatal("foreign starve must refuse extra validation slots")
	}
}

func TestCurrentLevel_DisabledEnvIgnoresStarveSampler(t *testing.T) {
	t.Setenv(brand.EnvVar("HOSTLOAD_DISABLE"), "1")
	setSamplerForTest(fixedSampler{snap: Snapshot{
		NCPU: 8, Load1: 20, IdleFraction: 0.01, SampledAt: time.Now(),
	}})
	t.Cleanup(func() { setSamplerForTest(nil) })
	if CurrentLevel() != LevelHeadroom {
		t.Fatalf("disabled sensing must be headroom, got %v", CurrentLevel())
	}
}
