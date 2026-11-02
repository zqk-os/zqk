package ambient

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Named telemetry buffer and threshold constants (zero magic numbers).
const (
	DefaultTelemetryCapacity     = 512
	DefaultTelemetryHistoryLimit = 100
	DefaultTelemetryTimeout      = 5 * time.Second
	TelemetryHealthHealthy       = "healthy"
	TelemetryHealthDegraded      = "degraded"
	TelemetryHealthCritical      = "critical"
	TelemetryHealthQuiescent     = "quiescent"

	// High-water thresholds for alert triggers.
	MaxAllowedDivergences = 5
	MinHealthyHolonCount  = 1
)

// MiddleBrainTelemetrySnapshot represents an instantaneous state inspection
// across the ambient middle-brain holons, stream hydrators, and river convergence engine.
type MiddleBrainTelemetrySnapshot struct {
	Timestamp             time.Time     `json:"timestamp"`
	HealthStatus          string        `json:"health_status"`
	ActiveHolonCount      int           `json:"active_holon_count"`
	Holons                []HolonRecord `json:"holons"`
	StreamCount           int           `json:"stream_count"`
	TotalRaindrops        int           `json:"total_raindrops"`
	RiverCount            int           `json:"river_count"`
	TotalDropsIngested    int64         `json:"total_drops_ingested"`
	AverageAlignmentScore float64       `json:"average_alignment_score"`
	DivergenceCount       int64         `json:"divergence_count"`
	WarningCount          int           `json:"warning_count"`
	Warnings              []string      `json:"warnings,omitempty"`
}

// HolonTelemetryAggregator coordinates non-blocking telemetry extraction across
// the middle-brain components and provides nil-safe health evaluation.
type HolonTelemetryAggregator struct {
	mu       sync.RWMutex
	registry *HolonLifecycleRegistry
	hydrator *StreamHydrator
	engine   *RiverConvergenceEngine
	history  []*MiddleBrainTelemetrySnapshot
	limit    int
}

// NewHolonTelemetryAggregator creates a new thread-safe aggregator for middle-brain telemetry.
func NewHolonTelemetryAggregator(
	registry *HolonLifecycleRegistry,
	hydrator *StreamHydrator,
	engine *RiverConvergenceEngine,
) *HolonTelemetryAggregator {
	return &HolonTelemetryAggregator{
		registry: registry,
		hydrator: hydrator,
		engine:   engine,
		history:  make([]*MiddleBrainTelemetrySnapshot, 0, DefaultTelemetryHistoryLimit),
		limit:    DefaultTelemetryHistoryLimit,
	}
}

// Snapshot gathers an instantaneous telemetry record from all bound components.
// It is guaranteed to be nil-safe and will never panic on uninitialized subcomponents.
func (a *HolonTelemetryAggregator) Snapshot(ctx context.Context) *MiddleBrainTelemetrySnapshot {
	now := time.Now()
	if a == nil {
		return &MiddleBrainTelemetrySnapshot{
			Timestamp:    now,
			HealthStatus: TelemetryHealthQuiescent,
			Warnings:     []string{"nil telemetry aggregator receiver"},
			WarningCount: 1,
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	var warnings []string

	// 1. Holon registry telemetry
	var holons []HolonRecord
	activeHolons := 0
	if a.registry != nil {
		holons = a.registry.List()
		for _, h := range holons {
			if h.Phase == HolonPhaseRunning || h.Phase == HolonPhaseStarting {
				activeHolons++
			}
		}
	} else {
		warnings = append(warnings, "holon registry not configured")
	}

	// 2. Stream hydrator telemetry
	streamCount := 0
	totalDrops := 0
	if a.hydrator != nil {
		streams := a.hydrator.ListStreams()
		streamCount = len(streams)
		for _, s := range streams {
			totalDrops += len(s.Raindrops)
		}
	} else {
		warnings = append(warnings, "stream hydrator not configured")
	}

	// 3. River convergence telemetry
	riverCount := 0
	var totalIngested int64
	var avgAlignment float64
	var divergences int64
	if a.engine != nil {
		metrics := a.engine.Metrics()
		totalIngested = metrics.TotalDropsIngested
		avgAlignment = metrics.AverageAlignment
		divergences = metrics.DivergenceWarnings

		rivers := a.engine.ListRivers()
		riverCount = len(rivers)
	} else {
		warnings = append(warnings, "river convergence engine not configured")
	}

	// 4. Derive overall middle-brain health status
	status := deriveHealthStatus(activeHolons, riverCount, divergences, len(warnings))

	snapshot := &MiddleBrainTelemetrySnapshot{
		Timestamp:             now,
		HealthStatus:          status,
		ActiveHolonCount:      activeHolons,
		Holons:                holons,
		StreamCount:           streamCount,
		TotalRaindrops:        totalDrops,
		RiverCount:            riverCount,
		TotalDropsIngested:    totalIngested,
		AverageAlignmentScore: avgAlignment,
		DivergenceCount:       divergences,
		WarningCount:          len(warnings),
		Warnings:              warnings,
	}

	// Retain ring-buffer history
	if len(a.history) >= a.limit {
		a.history = a.history[1:]
	}
	a.history = append(a.history, snapshot)

	return snapshot
}

// History returns a copy of historical telemetry snapshots.
func (a *HolonTelemetryAggregator) History() []*MiddleBrainTelemetrySnapshot {
	if a == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()

	out := make([]*MiddleBrainTelemetrySnapshot, len(a.history))
	copy(out, a.history)
	return out
}

// deriveHealthStatus computes health state based on middle-brain operational metrics.
func deriveHealthStatus(activeHolons int, riverCount int, divergences int64, warningCount int) string {
	if activeHolons == 0 && riverCount == 0 && warningCount > 0 {
		return TelemetryHealthQuiescent
	}
	if divergences > MaxAllowedDivergences {
		return TelemetryHealthCritical
	}
	if warningCount > 0 || (activeHolons > 0 && riverCount == 0) {
		return TelemetryHealthDegraded
	}
	return TelemetryHealthHealthy
}

// String provides a human-readable summary of the telemetry snapshot.
func (s *MiddleBrainTelemetrySnapshot) String() string {
	if s == nil {
		return "Middle-Brain Telemetry: <nil>"
	}
	return fmt.Sprintf(
		"Middle-Brain Vitality: %s | Active Holons: %d | Streams: %d (%d drops) | Rivers: %d | Alignment: %.2f | Divergences: %d",
		s.HealthStatus,
		s.ActiveHolonCount,
		s.StreamCount,
		s.TotalRaindrops,
		s.RiverCount,
		s.AverageAlignmentScore,
		s.DivergenceCount,
	)
}
