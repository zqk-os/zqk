package rollup

// EffortStats holds aggregated hours and distribution metrics for a given effort dimension.
type EffortStats struct {
	TotalHours float64 `json:"total_hours"`
	AvgHours   float64 `json:"avg_hours"`
	MinHours   float64 `json:"min_hours"`
	MaxHours   float64 `json:"max_hours"`
	Formatted  string  `json:"formatted"`
}

// RollupSummary aggregates counts, effort totals, and distributions from child objects
// (e.g. backlog items under a milestone or priority plan).
type RollupSummary struct {
	TotalCount      int         `json:"total_count"`
	CompletedCount  int         `json:"completed_count"`
	InProgressCount int         `json:"in_progress_count"`
	HaltedCount     int         `json:"halted_count"`
	OpenCount       int         `json:"open_count"`
	PercentComplete float64     `json:"percent_complete"`
	EstimatedEffort EffortStats `json:"estimated_effort"`
	ActualEffort    EffortStats `json:"actual_effort"`
}

// ToMap serializes RollupSummary to a standard map suitable for object overlays.
func (s RollupSummary) ToMap() map[string]any {
	return map[string]any{
		"total_count":       s.TotalCount,
		"completed_count":   s.CompletedCount,
		"in_progress_count": s.InProgressCount,
		"halted_count":      s.HaltedCount,
		"open_count":        s.OpenCount,
		"percent_complete":  s.PercentComplete,
		"estimated_effort": map[string]any{
			"total_hours": s.EstimatedEffort.TotalHours,
			"avg_hours":   s.EstimatedEffort.AvgHours,
			"min_hours":   s.EstimatedEffort.MinHours,
			"max_hours":   s.EstimatedEffort.MaxHours,
			"formatted":   s.EstimatedEffort.Formatted,
		},
		"actual_effort": map[string]any{
			"total_hours": s.ActualEffort.TotalHours,
			"avg_hours":   s.ActualEffort.AvgHours,
			"min_hours":   s.ActualEffort.MinHours,
			"max_hours":   s.ActualEffort.MaxHours,
			"formatted":   s.ActualEffort.Formatted,
		},
	}
}
