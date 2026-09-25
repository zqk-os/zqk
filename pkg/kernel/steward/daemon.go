package steward

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// HygieneProvider abstracts system cleanup operations for the steward sweep.
type HygieneProvider interface {
	ReapStaleLocks(ctx context.Context) (int, error)
	ReapTempFiles(ctx context.Context) (int, error)
	CompactWAL(ctx context.Context) error
}

// PlanProvider abstracts querying active and planned priority plans.
type PlanProvider interface {
	ListCandidatePlans(ctx context.Context) ([]PriorityPlanSummary, error)
}

// RunwayMonitor evaluates active pipeline buffer and triggers replenishment signals.
type RunwayMonitor struct {
	targetBuffer int
}

// NewRunwayMonitor creates a runway monitor with configurable target buffer depth.
func NewRunwayMonitor(targetBuffer int) *RunwayMonitor {
	if targetBuffer <= 0 {
		targetBuffer = MinimumShovelReadyRunway
	}
	return &RunwayMonitor{
		targetBuffer: targetBuffer,
	}
}

// EvaluateRunway inspects priority plans and computes buffer metrics and starvation risks.
func (rm *RunwayMonitor) EvaluateRunway(plans []PriorityPlanSummary) (RunwayStatus, []AmbientSignal) {
	shovelReadyCount := 0
	for _, p := range plans {
		if p.ShovelReady || p.Status == "planned" || p.Status == "active" {
			shovelReadyCount++
		}
	}

	deficit := rm.targetBuffer - shovelReadyCount
	if deficit < 0 {
		deficit = 0
	}

	starvationRisk := shovelReadyCount < rm.targetBuffer

	status := RunwayStatus{
		ShovelReadyBuffer: shovelReadyCount,
		TargetBuffer:      rm.targetBuffer,
		BufferDeficit:     deficit,
		IsStarvationRisk:  starvationRisk,
		ActivePlans:       plans,
		MeasuredAt:        time.Now(),
	}

	var signals []AmbientSignal
	if starvationRisk {
		signals = append(signals, AmbientSignal{
			SignalID:  fmt.Sprintf("SIG-RUNWAY-%d", time.Now().UnixNano()),
			Priority:  "P4-RUNWAY-REPLENISHMENT",
			Message:   fmt.Sprintf("Roadmap runway lead (%d plans) is below target buffer (%d). Autonomous steward replenishing queue.", shovelReadyCount, rm.targetBuffer),
			Timestamp: time.Now(),
		})
	}

	return status, signals
}

// Daemon coordinates continuous kernel stewardship sweeps and runway replenishment.
type Daemon struct {
	mu           sync.Mutex
	hygiene      HygieneProvider
	plans        PlanProvider
	monitor      *RunwayMonitor
	sweepHistory []SweepResult
}

// NewDaemon creates a new steward daemon instance.
func NewDaemon(hygiene HygieneProvider, plans PlanProvider, monitor *RunwayMonitor) *Daemon {
	if monitor == nil {
		monitor = NewRunwayMonitor(MinimumShovelReadyRunway)
	}
	return &Daemon{
		hygiene:      hygiene,
		plans:        plans,
		monitor:      monitor,
		sweepHistory: make([]SweepResult, 0),
	}
}

// ExecuteSweep runs a single comprehensive kernel hygiene and runway sweep.
func (d *Daemon) ExecuteSweep(ctx context.Context) (*SweepResult, error) {
	start := time.Now()
	sweep := &SweepResult{
		SweepID: fmt.Sprintf("SWEEP-%d", time.Now().UnixNano()),
	}

	// 1. I/O Hygiene: Reap stale locks and temp files
	if d.hygiene != nil {
		locks, err := d.hygiene.ReapStaleLocks(ctx)
		if err == nil {
			sweep.LocksReaped = locks
		}

		temps, err := d.hygiene.ReapTempFiles(ctx)
		if err == nil {
			sweep.TempFilesReaped = temps
		}

		err = d.hygiene.CompactWAL(ctx)
		sweep.WALCompacted = (err == nil)
	}

	// 2. Runway Lead Measurement
	var candidates []PriorityPlanSummary
	if d.plans != nil {
		c, err := d.plans.ListCandidatePlans(ctx)
		if err == nil {
			candidates = c
		}
	}

	runway, signals := d.monitor.EvaluateRunway(candidates)
	sweep.Runway = runway
	sweep.SignalsGenerated = signals
	sweep.Duration = time.Since(start)

	d.mu.Lock()
	d.sweepHistory = append(d.sweepHistory, *sweep)
	d.mu.Unlock()

	return sweep, nil
}
