package steward_test

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/kernel/steward"
)

// TST-KERNEL-RUNWAY-STEWARD-SUITE: Composite Execution Organizer Test Harness
// Verifies:
// 1. Static Floor: CRIT-RUNWAY-STATIC-BUFFER-MEASUREMENT
// 2. Operational Proof: CRIT-RUNWAY-OPERATIONAL-STEWARD-SWEEP
// 3. Negative Invariant: CRIT-RUNWAY-ADVERSARIAL-STARVATION-ALERT

type mockHygiene struct {
	locks int
	temps int
}

func (m *mockHygiene) ReapStaleLocks(ctx context.Context) (int, error) { return m.locks, nil }
func (m *mockHygiene) ReapTempFiles(ctx context.Context) (int, error)  { return m.temps, nil }
func (m *mockHygiene) CompactWAL(ctx context.Context) error            { return nil }

type mockPlans struct {
	plans []steward.PriorityPlanSummary
}

func (m *mockPlans) ListCandidatePlans(ctx context.Context) ([]steward.PriorityPlanSummary, error) {
	return m.plans, nil
}

// TestRunwayStaticBufferMeasurement asserts that runway lead metrics calculate
// shovel-ready buffer count and target deficits correctly.
func TestRunwayStaticBufferMeasurement(t *testing.T) {
	monitor := steward.NewRunwayMonitor(2)

	t.Run("Adequate Runway Lead (>= 2 plans)", func(t *testing.T) {
		plans := []steward.PriorityPlanSummary{
			{ID: "PRI-001", Status: "active", ShovelReady: true},
			{ID: "PRI-002", Status: "planned", ShovelReady: true},
		}

		status, signals := monitor.EvaluateRunway(plans)
		if status.ShovelReadyBuffer != 2 {
			t.Errorf("expected 2 shovel-ready plans, got %d", status.ShovelReadyBuffer)
		}
		if status.BufferDeficit != 0 {
			t.Errorf("expected deficit 0, got %d", status.BufferDeficit)
		}
		if status.IsStarvationRisk {
			t.Errorf("expected IsStarvationRisk == false")
		}
		if len(signals) != 0 {
			t.Errorf("expected 0 starvation alerts, got %d", len(signals))
		}
	})

	t.Run("Depleted Runway Lead (< 2 plans)", func(t *testing.T) {
		plans := []steward.PriorityPlanSummary{
			{ID: "PRI-001", Status: "active", ShovelReady: true},
		}

		status, signals := monitor.EvaluateRunway(plans)
		if status.ShovelReadyBuffer != 1 {
			t.Errorf("expected 1 shovel-ready plan, got %d", status.ShovelReadyBuffer)
		}
		if status.BufferDeficit != 1 {
			t.Errorf("expected deficit 1, got %d", status.BufferDeficit)
		}
		if !status.IsStarvationRisk {
			t.Errorf("expected IsStarvationRisk == true")
		}
		if len(signals) != 1 {
			t.Errorf("expected 1 starvation signal, got %d", len(signals))
		}
	})
}

// TestRunwayOperationalStewardSweep asserts that the maintenance sweep reaps locks,
// cleans temp files, and records runway status.
func TestRunwayOperationalStewardSweep(t *testing.T) {
	hygiene := &mockHygiene{locks: 3, temps: 2}
	plans := &mockPlans{
		plans: []steward.PriorityPlanSummary{
			{ID: "PRI-ACTIVE", Status: "active", ShovelReady: true},
			{ID: "PRI-NEXT", Status: "planned", ShovelReady: true},
		},
	}
	monitor := steward.NewRunwayMonitor(2)
	daemon := steward.NewDaemon(hygiene, plans, monitor)

	sweep, err := daemon.ExecuteSweep(context.Background())
	if err != nil {
		t.Fatalf("unexpected error during sweep: %v", err)
	}

	if sweep.LocksReaped != 3 {
		t.Errorf("expected 3 locks reaped, got %d", sweep.LocksReaped)
	}
	if sweep.TempFilesReaped != 2 {
		t.Errorf("expected 2 temp files reaped, got %d", sweep.TempFilesReaped)
	}
	if !sweep.WALCompacted {
		t.Errorf("expected WALCompacted == true")
	}
	if sweep.Runway.ShovelReadyBuffer != 2 {
		t.Errorf("expected runway buffer 2, got %d", sweep.Runway.ShovelReadyBuffer)
	}
}

// TestRunwayAdversarialStarvationAlert asserts that a low runway buffer raises
// a proactive ambient replenishment signal (P4-RUNWAY-REPLENISHMENT) to prevent loop idleness.
func TestRunwayAdversarialStarvationAlert(t *testing.T) {
	hygiene := &mockHygiene{}
	plans := &mockPlans{
		plans: []steward.PriorityPlanSummary{
			// Empty candidate plans: 0 buffer!
		},
	}
	monitor := steward.NewRunwayMonitor(2)
	daemon := steward.NewDaemon(hygiene, plans, monitor)

	sweep, err := daemon.ExecuteSweep(context.Background())
	if err != nil {
		t.Fatalf("unexpected error during sweep: %v", err)
	}

	if !sweep.Runway.IsStarvationRisk {
		t.Errorf("expected starvation risk on empty runway")
	}
	if len(sweep.SignalsGenerated) != 1 {
		t.Fatalf("expected 1 replenishment signal generated, got %d", len(sweep.SignalsGenerated))
	}

	sig := sweep.SignalsGenerated[0]
	if sig.Priority != "P4-RUNWAY-REPLENISHMENT" {
		t.Errorf("expected signal priority P4-RUNWAY-REPLENISHMENT, got %s", sig.Priority)
	}
}
