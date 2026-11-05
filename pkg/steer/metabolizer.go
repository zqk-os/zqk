package steer

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/events"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/vector"
)

// Metabolizer executes and records natural language steering transformations.
type Metabolizer struct {
	mu          sync.RWMutex
	registry    *vector.Registry
	bus         events.EventBus
	projectRoot string
}

// NewMetabolizer creates a new steering metabolizer.
func NewMetabolizer(projectRoot string, reg *vector.Registry, bus events.EventBus) *Metabolizer {
	if bus == nil {
		bus = events.GetDefaultBus()
	}
	if reg == nil {
		reg = vector.NewRegistry(projectRoot, bus)
	}
	return &Metabolizer{
		registry:    reg,
		bus:         bus,
		projectRoot: projectRoot,
	}
}

// StatePath returns the file path for persisting steering state.
func (m *Metabolizer) StatePath() string {
	root := m.projectRoot
	if root == "" {
		root = paths.ResolveProjectRoot(".")
	}
	return filepath.Join(root, paths.ProjectDataDir, "steer", "state.json")
}

// Metabolize parses natural language intent, adjusts vector weights/ranks and scheduler weights,
// persists state, and dispatches the 'steering.applied' event.
func (m *Metabolizer) Metabolize(ctx context.Context, intentText string) (*SteeringOutcome, error) {
	intent := ParseIntent(intentText)

	m.mu.Lock()
	defer m.mu.Unlock()

	sign := 1.0
	if intent.Direction == "reduce" {
		sign = -1.0
	}

	// Base weight adjustment step
	baseDelta := 0.10 * intent.Intensity * sign
	rankDelta := int(math.Round(5.0 * intent.Intensity * sign))

	var adjustments []SteeringAdjustment
	schedulerWeights := make(map[string]float64)

	// Apply adjustments to each targeted vector
	for _, vecID := range intent.TargetVectorIDs {
		oldVec, found := m.registry.Get(vecID)
		if !found {
			continue
		}

		delta := baseDelta
		rd := rankDelta
		steerDelta := vector.VectorSteerDelta{
			VectorID:    vecID,
			WeightDelta: &delta,
			RankDelta:   &rd,
			Reason:      intentText,
		}

		newVec, err := m.registry.Steer(ctx, steerDelta)
		if err != nil {
			return nil, err
		}

		adj := SteeringAdjustment{
			VectorID:    vecID,
			Dimension:   newVec.Dimension,
			OldWeight:   oldVec.Weight,
			NewWeight:   newVec.Weight,
			OldRank:     oldVec.PriorityRank,
			NewRank:     newVec.PriorityRank,
			WeightDelta: math.Round((newVec.Weight-oldVec.Weight)*100) / 100,
		}
		adjustments = append(adjustments, adj)

		// Set scheduler weight offset
		schedulerKey := string(newVec.Dimension) + "_queue_weight"
		schedulerWeights[schedulerKey] = newVec.Weight
	}

	outcome := &SteeringOutcome{
		Intent:           intent,
		Adjustments:      adjustments,
		SchedulerWeights: schedulerWeights,
		AppliedAt:        time.Now().UTC(),
		IsReset:          false,
	}

	// Persist state
	if err := m.saveState(outcome); err != nil {
		return nil, err
	}

	// Dispatch steering.applied event
	if m.bus != nil {
		_ = m.bus.Publish(ctx, EventSteeringApplied, outcome)
	}

	return outcome, nil
}

// Reset restores vector weights, ranks, and scheduler offsets to baseline defaults.
func (m *Metabolizer) Reset(ctx context.Context) (*SteeringOutcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Re-seed registry to default state
	newReg := vector.NewRegistry(m.projectRoot, m.bus)
	_ = newReg.Save()
	m.registry = newReg

	outcome := &SteeringOutcome{
		Intent: SteeringIntent{
			RawText:   "reset",
			Direction: "reset",
			Intensity: 1.0,
		},
		Adjustments:      nil,
		SchedulerWeights: make(map[string]float64),
		AppliedAt:        time.Now().UTC(),
		IsReset:          true,
	}

	_ = m.saveState(outcome)

	if m.bus != nil {
		_ = m.bus.Publish(ctx, EventSteeringApplied, outcome)
	}

	return outcome, nil
}

func (m *Metabolizer) saveState(outcome *SteeringOutcome) error {
	p := m.StatePath()
	dir := filepath.Dir(p)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return err
	}

	offsets := make(map[string]float64)
	for _, adj := range outcome.Adjustments {
		offsets[adj.VectorID] = adj.WeightDelta
	}

	st := SteeringState{
		ActiveOffsets:    offsets,
		LastIntent:       outcome.Intent.RawText,
		LastAppliedAt:    outcome.AppliedAt,
		SchedulerWeights: outcome.SchedulerWeights,
		Adjustments:      outcome.Adjustments,
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}

	return fileutil.WriteFile(p, data, paths.FilePerm644)
}

// LoadState reads persisted steering state from disk.
func (m *Metabolizer) LoadState() (*SteeringState, error) {
	p := m.StatePath()
	data, err := fileutil.ReadFile(p)
	if err != nil {
		return nil, err
	}

	var st SteeringState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}

	return &st, nil
}
