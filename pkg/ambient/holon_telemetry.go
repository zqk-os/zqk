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
	return NewHolonTelemetryAggregatorWithLimit(registry, hydrator, engine, DefaultTelemetryHistoryLimit)
}

// NewHolonTelemetryAggregatorWithLimit creates an aggregator with a custom history capacity floor.
// If limit <= 0, DefaultTelemetryHistoryLimit is enforced as the fallback floor.
func NewHolonTelemetryAggregatorWithLimit(
	registry *HolonLifecycleRegistry,
	hydrator *StreamHydrator,
	engine *RiverConvergenceEngine,
	limit int,
) *HolonTelemetryAggregator {
	if limit <= 0 {
		limit = DefaultTelemetryHistoryLimit
	}
	return &HolonTelemetryAggregator{
		registry: registry,
		hydrator: hydrator,
		engine:   engine,
		history:  make([]*MiddleBrainTelemetrySnapshot, 0, limit),
		limit:    limit,
	}
}

// Snapshot gathers an instantaneous telemetry record from all bound components.
// It is guaranteed to be nil-safe and will never panic on uninitialized subcomponents.
func (a *HolonTelemetryAggregator) Snapshot(ctx context.Context) *MiddleBrainTelemetrySnapshot {
	now := time.Now()
	if a == nil {
		nilWarnings := []string{"nil telemetry aggregator receiver"}
		return &MiddleBrainTelemetrySnapshot{
			Timestamp:    now,
			HealthStatus: TelemetryHealthQuiescent,
			Warnings:     nilWarnings,
			WarningCount: len(nilWarnings),
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

// HistoryLimit returns the configured history retention limit.
// Nil receiver returns 0 safely.
func (a *HolonTelemetryAggregator) HistoryLimit() int {
	if a == nil {
		return 0
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.limit
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

// IsHealthy returns true if the snapshot indicates a healthy middle-brain state.
// Nil receiver returns false safely.
func (s *MiddleBrainTelemetrySnapshot) IsHealthy() bool {
	if s == nil {
		return false
	}
	return s.HealthStatus == TelemetryHealthHealthy
}

// IsDegraded returns true if the snapshot indicates a degraded middle-brain state.
// Nil receiver returns false safely.
func (s *MiddleBrainTelemetrySnapshot) IsDegraded() bool {
	if s == nil {
		return false
	}
	return s.HealthStatus == TelemetryHealthDegraded
}

// IsCritical returns true if the snapshot indicates a critical middle-brain state.
// Nil receiver returns false safely.
func (s *MiddleBrainTelemetrySnapshot) IsCritical() bool {
	if s == nil {
		return false
	}
	return s.HealthStatus == TelemetryHealthCritical
}

// IsQuiescent returns true if the snapshot indicates a quiescent/unconfigured state.
// Nil receiver returns true safely.
func (s *MiddleBrainTelemetrySnapshot) IsQuiescent() bool {
	if s == nil {
		return true
	}
	return s.HealthStatus == TelemetryHealthQuiescent
}

// HasWarnings returns true if any telemetry warnings are recorded.
// Nil receiver returns true safely.
func (s *MiddleBrainTelemetrySnapshot) HasWarnings() bool {
	if s == nil {
		return true
	}
	return s.WarningCount > 0 || len(s.Warnings) > 0
}

// HolonIDs extracts the list of active/registered holon IDs recorded in the snapshot.
// Nil receiver returns nil safely.
func (s *MiddleBrainTelemetrySnapshot) HolonIDs() []string {
	if s == nil || len(s.Holons) == 0 {
		return nil
	}
	ids := make([]string, len(s.Holons))
	for i, h := range s.Holons {
		ids[i] = h.ID
	}
	return ids
}
