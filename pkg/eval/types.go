package eval

import (
	"time"
)

// EvalTemplate defines the declarative specification for scenario evaluations.
type EvalTemplate struct {
	SchemaVersion int                      `yaml:"schema_version" json:"schema_version"`
	Kind          string                   `yaml:"kind" json:"kind"`
	TemplateID    string                   `yaml:"template_id" json:"template_id"`
	Title         string                   `yaml:"title" json:"title"`
	Description   string                   `yaml:"description" json:"description"`
	Target        TargetSpec               `yaml:"target" json:"target"`
	FastPath      FastPathSpec             `yaml:"fast_path" json:"fast_path"`
	Dimensions    map[string]DimensionSpec `yaml:"dimensions" json:"dimensions"`
	Remediation   RemediationSpec          `yaml:"remediation" json:"remediation"`
}

// TargetSpec defines target file globs and exclusion boundaries.
type TargetSpec struct {
	Kind     string   `yaml:"kind" json:"kind"` // e.g. "file_glob", "path"
	Patterns []string `yaml:"patterns" json:"patterns"`
	Exclude  []string `yaml:"exclude" json:"exclude"`
}

// FastPathSpec defines pre-token structural and AST inspection rules.
type FastPathSpec struct {
	Enabled       bool     `yaml:"enabled" json:"enabled"`
	MaxComplexity int      `yaml:"max_complexity" json:"max_complexity"`
	Rules         []string `yaml:"rules" json:"rules"`
}

// DimensionSpec defines scoring threshold and weights for a quality dimension.
type DimensionSpec struct {
	Weight    float64 `yaml:"weight" json:"weight"`
	Threshold float64 `yaml:"threshold" json:"threshold"`
	Rubric    string  `yaml:"rubric" json:"rubric"`
}

// RemediationSpec defines automated kernel remediation dispatching on failure.
type RemediationSpec struct {
	AutoMint               bool   `yaml:"auto_mint" json:"auto_mint"`
	Persona                string `yaml:"persona" json:"persona"`
	Priority               string `yaml:"priority" json:"priority"`
	LinkConvergenceSession bool   `yaml:"link_convergence_session" json:"link_convergence_session"`
}

// Violation represents an evaluated failure against a fast-path rule or dimension threshold.
type Violation struct {
	File      string  `json:"file"`
	Rule      string  `json:"rule"`
	Severity  string  `json:"severity"` // "blocker", "warning"
	Message   string  `json:"message"`
	Actual    float64 `json:"actual,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
}

// EvalResult encapsulates the full outcome of an evaluation run.
type EvalResult struct {
	TemplateID      string             `json:"template_id"`
	Target          string             `json:"target"`
	AssessedAt      time.Time          `json:"assessed_at"`
	Passed          bool               `json:"passed"`
	CompositeScore  float64            `json:"composite_score"`
	DimensionScores map[string]float64 `json:"dimension_scores"`
	Violations      []Violation        `json:"violations"`
	Remediations    []string           `json:"remediations,omitempty"`
}
