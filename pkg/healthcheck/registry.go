package healthcheck

import "context"

// Monitor is a pluggable health check. Implementations register with the global registry.
type Monitor interface {
	ID() string   // e.g. "scheduler_events"
	Name() string // human-readable name
	Run(ctx context.Context, projectRoot string) (*Result, error)
}

// Result is the outcome of a monitor run.
type Result struct {
	Status  string         `json:"status"`  // "ok" | "degraded" | "fail"
	Summary string         `json:"summary"` // one-line summary
	Details map[string]any `json:"details,omitempty"`
}

// Registry holds registered monitors and their runtime config (enabled, etc.).
type Registry interface {
	Register(m Monitor)
	List() []Monitor
	Get(id string) (Monitor, bool)
	IsEnabled(id string) bool
	SetEnabled(id string, enabled bool) error
	Run(ctx context.Context, projectRoot string, id string) (*Result, error)
}
