package ambient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
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

// Named sentinel errors for middle-brain ambient state persistence and channel boundary handling.
var (
	ErrCorruptAmbientState  = errors.New("corrupt ambient state file")
	ErrEmptyAmbientState    = errors.New("empty ambient state file")
	ErrStatePathRequired    = errors.New("ambient state file path is required")
	ErrNilTelemetrySnapshot = errors.New("telemetry snapshot cannot be nil")
	ErrNilTelemetryChannel  = errors.New("telemetry channel cannot be nil")
	ErrTelemetryChannelFull = errors.New("telemetry channel buffer exhausted")
	ErrUninitializedStream  = errors.New("uninitialized river stream")
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
	mu          sync.RWMutex
	registry    *HolonLifecycleRegistry
	hydrator    *StreamHydrator
	engine      *RiverConvergenceEngine
	history     []*MiddleBrainTelemetrySnapshot
	limit       int
	subscribers map[string]chan<- *MiddleBrainTelemetrySnapshot
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
		registry:    registry,
		hydrator:    hydrator,
		engine:      engine,
		history:     make([]*MiddleBrainTelemetrySnapshot, 0, limit),
		limit:       limit,
		subscribers: make(map[string]chan<- *MiddleBrainTelemetrySnapshot),
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
			if s == nil {
				warnings = append(warnings, "uninitialized stream encountered")
				continue
			}
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
		for _, r := range rivers {
			if r == nil {
				warnings = append(warnings, "uninitialized river stream encountered")
				continue
			}
			if len(r.PooledVector) == 0 {
				warnings = append(warnings, fmt.Sprintf("river %s has uninitialized pooled vector", r.ID))
			}
		}
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

// BroadcastResult captures the outcome of broadcasting a telemetry snapshot across subscribers.
type BroadcastResult struct {
	Delivered int      `json:"delivered"`
	Exhausted int      `json:"exhausted"`
	Warnings  []string `json:"warnings,omitempty"`
}

// RegisterSubscriber registers a named channel to receive broadcast telemetry snapshots.
// Fails closed with ErrNilTelemetryChannel if ch is nil, or an error if id is empty.
func (a *HolonTelemetryAggregator) RegisterSubscriber(id string, ch chan<- *MiddleBrainTelemetrySnapshot) error {
	if a == nil {
		return errors.New("nil telemetry aggregator receiver")
	}
	if ch == nil {
		return ErrNilTelemetryChannel
	}
	if strings.TrimSpace(id) == "" {
		return errors.New("subscriber id is required")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.subscribers == nil {
		a.subscribers = make(map[string]chan<- *MiddleBrainTelemetrySnapshot)
	}
	a.subscribers[id] = ch
	return nil
}

// UnregisterSubscriber removes a named subscriber channel.
// Returns true if the subscriber was registered and removed.
func (a *HolonTelemetryAggregator) UnregisterSubscriber(id string) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.subscribers == nil {
		return false
	}
	_, exists := a.subscribers[id]
	delete(a.subscribers, id)
	return exists
}

// SubscriberCount returns the count of currently registered subscriber channels.
// Nil receiver returns 0 safely.
func (a *HolonTelemetryAggregator) SubscriberCount() int {
	if a == nil {
		return 0
	}
	a.mu.RLock()
	count := len(a.subscribers)
	a.mu.RUnlock()
	return count
}

// Broadcast dispatches a telemetry snapshot across all registered subscriber channels non-blockingly.
// It guarantees zero daemon hangs, zero goroutine leaks, and zero panics when channels are saturated or nil.
func (a *HolonTelemetryAggregator) Broadcast(snap *MiddleBrainTelemetrySnapshot) BroadcastResult {
	if a == nil {
		return BroadcastResult{
			Warnings: []string{"nil aggregator cannot broadcast"},
		}
	}
	if snap == nil {
		return BroadcastResult{
			Warnings: []string{"nil snapshot rejected from broadcast"},
		}
	}

	a.mu.RLock()
	subs := make(map[string]chan<- *MiddleBrainTelemetrySnapshot, len(a.subscribers))
	for k, v := range a.subscribers {
		subs[k] = v
	}
	a.mu.RUnlock()

	var res BroadcastResult
	for id, ch := range subs {
		if ch == nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("subscriber %q channel is nil", id))
			continue
		}
		select {
		case ch <- snap:
			res.Delivered++
		default:
			// Saturated/exhausted channel: non-blocking drop to prevent daemon stall or goroutine leaks
			res.Exhausted++
			res.Warnings = append(res.Warnings, fmt.Sprintf("subscriber %q telemetry channel exhausted", id))
		}
	}
	return res
}

// PersistState snapshots and saves current middle-brain ambient state to disk.
func (a *HolonTelemetryAggregator) PersistState(ctx context.Context, filePath string) error {
	if a == nil {
		return errors.New("nil telemetry aggregator receiver")
	}
	snap := a.Snapshot(ctx)
	return SaveTelemetryStateFile(filePath, snap)
}

// RestoreState loads a persisted state snapshot from disk and appends it to history.
func (a *HolonTelemetryAggregator) RestoreState(filePath string) (*MiddleBrainTelemetrySnapshot, error) {
	snap, err := LoadTelemetryStateFile(filePath)
	if err != nil {
		return nil, err
	}
	if a != nil {
		a.mu.Lock()
		if len(a.history) >= a.limit {
			a.history = a.history[1:]
		}
		a.history = append(a.history, snap)
		a.mu.Unlock()
	}
	return snap, nil
}

// SaveTelemetryStateFile persists a MiddleBrainTelemetrySnapshot to disk at the given path.
// It fails closed with actionable error receipts on missing path or nil snapshot.
func SaveTelemetryStateFile(filePath string, snap *MiddleBrainTelemetrySnapshot) error {
	if strings.TrimSpace(filePath) == "" {
		return ErrStatePathRequired
	}
	if snap == nil {
		return ErrNilTelemetrySnapshot
	}
	return fileutil.WriteSecureJSONIndent(filePath, snap)
}

// LoadTelemetryStateFile reads and validates a persisted ambient middle-brain telemetry snapshot from disk.
// Fails closed with actionable error receipts on missing file, unreadable file, empty file, or malformed JSON.
func LoadTelemetryStateFile(filePath string) (*MiddleBrainTelemetrySnapshot, error) {
	if strings.TrimSpace(filePath) == "" {
		return nil, ErrStatePathRequired
	}
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil, fmt.Errorf("%w: state file does not exist: %s", ErrCorruptAmbientState, filePath)
		}
		if fileutil.IsPermission(err) {
			return nil, fmt.Errorf("%w: permission denied reading state file: %s (%v)", ErrCorruptAmbientState, filePath, err)
		}
		return nil, fmt.Errorf("%w: failed to read state file: %v", ErrCorruptAmbientState, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("%w: state file %q is empty", ErrEmptyAmbientState, filePath)
	}

	var snap MiddleBrainTelemetrySnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("%w: malformed json in %q: %v", ErrCorruptAmbientState, filePath, err)
	}
	if snap.HealthStatus == "" {
		snap.HealthStatus = TelemetryHealthQuiescent
	}
	return &snap, nil
}

// EmitTelemetryToChannel emits a telemetry snapshot to a channel non-blockingly.
// Returns ErrNilTelemetryChannel if ch is nil, ErrNilTelemetrySnapshot if snap is nil,
// and ErrTelemetryChannelFull if the channel buffer is exhausted.
func EmitTelemetryToChannel(ctx context.Context, ch chan<- *MiddleBrainTelemetrySnapshot, snap *MiddleBrainTelemetrySnapshot) error {
	if ch == nil {
		return ErrNilTelemetryChannel
	}
	if snap == nil {
		return ErrNilTelemetrySnapshot
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	select {
	case ch <- snap:
		return nil
	default:
		return ErrTelemetryChannelFull
	}
}

// EmitTelemetryWithTimeout attempts to emit a snapshot to a channel with a bounded timeout before failing closed.
func EmitTelemetryWithTimeout(ctx context.Context, ch chan<- *MiddleBrainTelemetrySnapshot, snap *MiddleBrainTelemetrySnapshot, timeout time.Duration) error {
	if ch == nil {
		return ErrNilTelemetryChannel
	}
	if snap == nil {
		return ErrNilTelemetrySnapshot
	}
	if timeout <= 0 {
		return EmitTelemetryToChannel(ctx, ch, snap)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var doneCh <-chan struct{}
	if ctx != nil {
		doneCh = ctx.Done()
	}

	select {
	case ch <- snap:
		return nil
	case <-doneCh:
		return ctx.Err()
	case <-timer.C:
		return ErrTelemetryChannelFull
	}
}

// ValidateRiverStream validates whether an IntentRiver has been properly initialized.
// Returns ErrUninitializedStream if river is nil or has uninitialized pooled vector / empty ID.
func ValidateRiverStream(river *IntentRiver) error {
	if river == nil {
		return fmt.Errorf("%w: river is nil", ErrUninitializedStream)
	}
	if strings.TrimSpace(river.ID) == "" {
		return fmt.Errorf("%w: river ID is empty", ErrUninitializedStream)
	}
	if river.PooledVector == nil {
		return fmt.Errorf("%w: river %q has uninitialized pooled vector", ErrUninitializedStream, river.ID)
	}
	return nil
}

// ValidateIntentStream validates whether an IntentStream has been properly initialized.
func ValidateIntentStream(stream *IntentStream) error {
	if stream == nil {
		return fmt.Errorf("%w: stream is nil", ErrUninitializedStream)
	}
	if strings.TrimSpace(stream.ID) == "" {
		return fmt.Errorf("%w: stream ID is empty", ErrUninitializedStream)
	}
	return nil
}
