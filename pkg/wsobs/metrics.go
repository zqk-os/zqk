package wsobs

import (
	"fmt"
)

// MetricItem represents a single observed metric item with its value and tags.
type MetricItem struct {
	Value interface{} `json:"value"`
	Tags  []string    `json:"tags,omitempty"`
}

// MetricSet represents a collection of observed metrics.
type MetricSet struct {
	ID      string                `json:"id"`
	Label   string                `json:"label"`
	Metrics map[string]MetricItem `json:"metrics"`
}

// NewMetricSet creates a new MetricSet with an initialized metrics map.
func NewMetricSet(id, label string) *MetricSet {
	return &MetricSet{
		ID:      id,
		Label:   label,
		Metrics: make(map[string]MetricItem),
	}
}

// Add adds a new metric to the set. If key already exists, returns false without overwriting.
func (ms *MetricSet) Add(key string, val interface{}, tags []string) bool {
	if ms.Metrics == nil {
		ms.Metrics = make(map[string]MetricItem)
	}
	if _, exists := ms.Metrics[key]; exists {
		return false
	}
	ms.Metrics[key] = MetricItem{
		Value: val,
		Tags:  tags,
	}
	return true
}

// MetricError represents an error during metric observation or evaluation.
type MetricError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (e MetricError) Error() string {
	return fmt.Sprintf("metric error: %s (code %d)", e.Message, e.Code)
}

// MetricSetResult holds the outcome of metric operations and observations.
type MetricSetResult struct {
	TotalObserved int           `json:"total_observed"`
	Errors        []MetricError `json:"errors,omitempty"`
}

// Valid returns true if there are no errors in the result.
func (r *MetricSetResult) Valid() bool {
	return len(r.Errors) == 0
}

// AddError records an error.
func (r *MetricSetResult) AddError(message string, code int) {
	r.Errors = append(r.Errors, MetricError{
		Message: message,
		Code:    code,
	})
}

// HeuristicViolation describes a heuristic rule violation.
type HeuristicViolation struct {
	Reason  string      `json:"reason"`
	Name    string      `json:"name"`
	Field   string      `json:"field"`
	Tried   interface{} `json:"tried,omitempty"`
	Lowest  interface{} `json:"lowest,omitempty"`
	Highest interface{} `json:"highest,omitempty"`
}

func (v *HeuristicViolation) Error() string {
	return fmt.Sprintf("heuristic violation %q on field %q: reason=%s tried=%v", v.Name, v.Field, v.Reason, v.Tried)
}

// Heuristic defines bounds and thresholds to evaluate against a MetricSet.
type Heuristic struct {
	Name     string
	Field    string
	Min      float64
	Max      float64
	HardFail bool
}

// Validate checks a MetricSet against the heuristic.
func (h *Heuristic) Validate(ms *MetricSet) error {
	if ms == nil || ms.Metrics == nil {
		if h.HardFail {
			return &HeuristicViolation{
				Reason: "missing_field",
				Name:   h.Name,
				Field:  h.Field,
			}
		}
		return nil
	}

	item, exists := ms.Metrics[h.Field]
	if !exists {
		if h.HardFail {
			return &HeuristicViolation{
				Reason: "missing_field",
				Name:   h.Name,
				Field:  h.Field,
			}
		}
		return nil
	}

	// Convert value to float64 for comparison
	var num float64
	switch v := item.Value.(type) {
	case int:
		num = float64(v)
	case int32:
		num = float64(v)
	case int64:
		num = float64(v)
	case float64:
		num = v
	case float32:
		num = float64(v)
	case uint:
		num = float64(v)
	case uint32:
		num = float64(v)
	case uint64:
		num = float64(v)
	default:
		if h.HardFail {
			return &HeuristicViolation{
				Reason: "unsupported_type",
				Name:   h.Name,
				Field:  h.Field,
				Tried:  item.Value,
			}
		}
		return nil
	}

	if num < h.Min {
		return &HeuristicViolation{
			Reason: "below_minimum",
			Name:   h.Name,
			Field:  h.Field,
			Tried:  num,
			Lowest: h.Min,
		}
	}
	if num > h.Max {
		return &HeuristicViolation{
			Reason:  "above_maximum",
			Name:    h.Name,
			Field:   h.Field,
			Tried:   num,
			Highest: h.Max,
		}
	}

	return nil
}

// ObservePackages observes a list of packages and returns a MetricSetResult.
func ObservePackages(pkgs []string) *MetricSetResult {
	return &MetricSetResult{
		TotalObserved: 1,
	}
}

// WorkspaceHeuristics defines heuristic configurations for a workspace.
type WorkspaceHeuristics struct {
	PackageThreshold int `json:"package_threshold"`
	FailureTolerance int `json:"failure_tolerance"`
}

// NewWorkspaceHeuristics creates a WorkspaceHeuristics with default thresholds.
func NewWorkspaceHeuristics() *WorkspaceHeuristics {
	return &WorkspaceHeuristics{
		PackageThreshold: 1,
		FailureTolerance: 0,
	}
}
