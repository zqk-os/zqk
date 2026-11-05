package vector

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/events"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Registry manages registered evaluation vectors.
type Registry struct {
	mu          sync.RWMutex
	vectors     map[string]*Vector
	evaluator   Evaluator
	bus         events.EventBus
	projectRoot string
}

// NewRegistry creates a new Vector Registry.
func NewRegistry(projectRoot string, bus events.EventBus) *Registry {
	if bus == nil {
		bus = events.GetDefaultBus()
	}
	r := &Registry{
		vectors:     make(map[string]*Vector),
		evaluator:   DefaultEvaluator,
		bus:         bus,
		projectRoot: projectRoot,
	}
	r.seedDefaultVectors()
	return r
}

// seedDefaultVectors initializes standard vectors across all core dimensions.
func (r *Registry) seedDefaultVectors() {
	defaults := []Vector{
		{
			ID:           "VEC-SECURITY",
			Dimension:    DimensionSecurity,
			PriorityRank: 95,
			Weight:       0.90,
			Threshold:    0.85,
			Status:       "pending",
			Description:  "Security posture, permissions, and credential boundaries",
		},
		{
			ID:           "VEC-RELIABILITY",
			Dimension:    DimensionReliability,
			PriorityRank: 90,
			Weight:       0.85,
			Threshold:    0.85,
			Status:       "pending",
			Description:  "System reliability, test coverage, and failure recovery",
		},
		{
			ID:           "VEC-GOVERNANCE",
			Dimension:    DimensionGovernance,
			PriorityRank: 85,
			Weight:       0.80,
			Threshold:    0.80,
			Status:       "pending",
			Description:  "Kernel object integrity, policy compliance, and auditability",
		},
		{
			ID:           "VEC-EFFICIENCY",
			Dimension:    DimensionEfficiency,
			PriorityRank: 80,
			Weight:       0.75,
			Threshold:    0.75,
			Status:       "pending",
			Description:  "Token efficiency, latency, and resource hygiene",
		},
		{
			ID:           "VEC-CAPABILITY",
			Dimension:    DimensionCapability,
			PriorityRank: 75,
			Weight:       0.70,
			Threshold:    0.75,
			Status:       "pending",
			Description:  "Feature capability, functional completion, and API breadth",
		},
		{
			ID:           "VEC-READABILITY",
			Dimension:    DimensionReadability,
			PriorityRank: 70,
			Weight:       0.65,
			Threshold:    0.70,
			Status:       "pending",
			Description:  "Code readability, documentation completeness, and clarity",
		},
		{
			ID:           "VEC-PORTABILITY",
			Dimension:    DimensionPortability,
			PriorityRank: 65,
			Weight:       0.60,
			Threshold:    0.70,
			Status:       "pending",
			Description:  "Cross-platform neutrality, path sanitization, and environment safety",
		},
	}

	for _, v := range defaults {
		vecCopy := v
		r.vectors[v.ID] = &vecCopy
	}
}

// Register registers or updates a vector.
func (r *Registry) Register(v Vector) error {
	if err := v.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	vecCopy := v
	r.vectors[v.ID] = &vecCopy
	return nil
}

// Get retrieves a vector by ID.
func (r *Registry) Get(id string) (Vector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	vec, ok := r.vectors[id]
	if !ok {
		return Vector{}, false
	}
	return *vec, true
}

// List returns all registered vectors.
// If prioritized is true, vectors are sorted descending by PriorityRank.
func (r *Registry) List(prioritized bool) []Vector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Vector, 0, len(r.vectors))
	for _, v := range r.vectors {
		out = append(out, *v)
	}

	if prioritized {
		sort.Slice(out, func(i, j int) bool {
			if out[i].PriorityRank != out[j].PriorityRank {
				return out[i].PriorityRank > out[j].PriorityRank
			}
			return out[i].Weight > out[j].Weight
		})
	} else {
		sort.Slice(out, func(i, j int) bool {
			return out[i].ID < out[j].ID
		})
	}

	return out
}

// Evaluate evaluates vectors and emits events.
func (r *Registry) Evaluate(ctx context.Context, all bool) ([]EvaluationResult, error) {
	r.mu.Lock()
	vecList := make([]*Vector, 0, len(r.vectors))
	for _, v := range r.vectors {
		vecList = append(vecList, v)
	}
	r.mu.Unlock()

	var results []EvaluationResult
	for _, vec := range vecList {
		res, err := r.evaluator(ctx, r.projectRoot, vec)
		if err != nil {
			return nil, fmt.Errorf("evaluating vector %s: %w", vec.ID, err)
		}

		r.mu.Lock()
		vec.CurrentScore = res.Score
		vec.Status = res.Status
		vec.LastEvaluated = res.EvaluatedAt
		r.mu.Unlock()

		results = append(results, res)

		// Dispatch vector.evaluated event
		if r.bus != nil {
			_ = r.bus.Publish(ctx, EventVectorEvaluated, res)
			if res.Breached {
				// Dispatch vector.threshold_breached event
				_ = r.bus.Publish(ctx, EventVectorThresholdBreach, res)
			}
		}
	}

	// Persist state
	_ = r.Save()

	return results, nil
}

// Steer applies a steering delta to a vector and emits vector.steered event.
func (r *Registry) Steer(ctx context.Context, delta VectorSteerDelta) (*Vector, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	vec, ok := r.vectors[delta.VectorID]
	if !ok {
		return nil, fmt.Errorf("vector %q not found", delta.VectorID)
	}

	oldVec := *vec

	if delta.WeightDelta != nil {
		vec.Weight = math.Max(0.0, math.Min(1.0, vec.Weight+*delta.WeightDelta))
	}
	if delta.NewWeight != nil {
		vec.Weight = math.Max(0.0, math.Min(1.0, *delta.NewWeight))
	}
	if delta.RankDelta != nil {
		vec.PriorityRank = int(math.Max(1, math.Min(100, float64(vec.PriorityRank+*delta.RankDelta))))
	}
	if delta.NewRank != nil {
		vec.PriorityRank = int(math.Max(1, math.Min(100, float64(*delta.NewRank))))
	}
	if delta.NewThreshold != nil {
		vec.Threshold = math.Max(0.0, math.Min(1.0, *delta.NewThreshold))
	}

	// Round weight & threshold to 2 decimal places
	vec.Weight = math.Round(vec.Weight*100) / 100
	vec.Threshold = math.Round(vec.Threshold*100) / 100

	steerEventPayload := map[string]any{
		"vector_id": vec.ID,
		"dimension": vec.Dimension,
		"old": map[string]any{
			"weight":        oldVec.Weight,
			"priority_rank": oldVec.PriorityRank,
			"threshold":     oldVec.Threshold,
		},
		"new": map[string]any{
			"weight":        vec.Weight,
			"priority_rank": vec.PriorityRank,
			"threshold":     vec.Threshold,
		},
		"reason":    delta.Reason,
		"timestamp": time.Now().UTC(),
	}

	if r.bus != nil {
		_ = r.bus.Publish(ctx, EventVectorSteered, steerEventPayload)
	}

	return vec, nil
}

// RegistryPath returns the JSON file path for persisting vector state.
func (r *Registry) RegistryPath() string {
	root := r.projectRoot
	if root == "" {
		root = paths.ResolveProjectRoot(".")
	}
	return filepath.Join(root, paths.ProjectDataDir, "vectors.json")
}

// Save persists the vectors to disk via fileutil.
func (r *Registry) Save() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	path := r.RegistryPath()
	dir := filepath.Dir(path)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(r.vectors, "", "  ")
	if err != nil {
		return err
	}

	return fileutil.WriteFile(path, data, paths.FilePerm644)
}

// Load loads persisted vectors from disk via fileutil.
func (r *Registry) Load() error {
	path := r.RegistryPath()
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return err
	}

	var loaded map[string]*Vector
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range loaded {
		r.vectors[k] = v
	}
	return nil
}
