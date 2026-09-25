package steward

import (
	"errors"
	"time"
)

// MinimumShovelReadyRunway defines the target lead of planned priority plans ahead of execution.
const MinimumShovelReadyRunway = 2

// Errors for the kernel steward and runway monitor.
var (
	ErrNilRunwayMonitor     = errors.New("steward: runway monitor cannot be nil")
	ErrStarvationRisk       = errors.New("steward: pipeline starvation risk detected; shovel-ready runway below threshold")
	ErrInvalidConfiguration = errors.New("steward: invalid daemon configuration")
)

// PriorityPlanSummary summarizes a priority plan's shovel-readiness for runway metrics.
type PriorityPlanSummary struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	ShovelReady bool   `json:"shovel_ready"`
}

// RunwayStatus holds metrics on the active roadmap lead and pipeline buffer.
type RunwayStatus struct {
	ShovelReadyBuffer int                   `json:"shovel_ready_buffer"`
	TargetBuffer      int                   `json:"target_buffer"`
	BufferDeficit     int                   `json:"buffer_deficit"`
	IsStarvationRisk  bool                  `json:"is_starvation_risk"`
	ActivePlans       []PriorityPlanSummary `json:"active_plans"`
	MeasuredAt        time.Time             `json:"measured_at"`
}

// AmbientSignal represents a proactive system or replenishment alert raised by the steward.
type AmbientSignal struct {
	SignalID  string    `json:"signal_id"`
	Priority  string    `json:"priority"` // e.g. "P4-RUNWAY-REPLENISHMENT"
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// SweepResult reports artifacts and telemetry from a single steward maintenance sweep.
type SweepResult struct {
	SweepID          string          `json:"sweep_id"`
	LocksReaped      int             `json:"locks_reaped"`
	TempFilesReaped  int             `json:"temp_files_reaped"`
	WALCompacted     bool            `json:"wal_compacted"`
	Runway           RunwayStatus    `json:"runway"`
	SignalsGenerated []AmbientSignal `json:"signals_generated"`
	Duration         time.Duration   `json:"duration"`
}
