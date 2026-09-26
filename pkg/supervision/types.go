package supervision

import "time"

// FailingPhase identifies the phase where task execution failed.
type FailingPhase string

const (
	PhaseVerification FailingPhase = "verification"
	PhaseCompile      FailingPhase = "compile"
	PhaseExecution    FailingPhase = "execution"
	PhaseValidation   FailingPhase = "validation"
	PhaseTimeout      FailingPhase = "timeout"
	PhaseUnknown      FailingPhase = "unknown"
)

// FailureDiagnostic encapsulates structured telemetry from a failed task attempt.
// Designed to be project-agnostic without vendor-specific or language-locked dependencies.
type FailureDiagnostic struct {
	Attempt           int          `json:"attempt"`
	ExitCode          int          `json:"exit_code"`
	Phase             FailingPhase `json:"phase"`
	FailureAnchor     string       `json:"failure_anchor,omitempty"`     // E.g. file, symbol, or command
	RawStderr         string       `json:"raw_stderr,omitempty"`
	RawStdout         string       `json:"raw_stdout,omitempty"`
	FocusedDiagnostic string       `json:"focused_diagnostic"`           // Extracted signal without noise
	ObservedAnomaly   string       `json:"observed_anomaly,omitempty"`   // Contrastive description of failure
	Timestamp         time.Time    `json:"timestamp"`
}

// NegativeHypothesis captures an invalidated strategy to prevent cyclical retry thrashing.
type NegativeHypothesis struct {
	Attempt             int      `json:"attempt"`
	HypothesisSummary   string   `json:"hypothesis_summary"`
	InvalidatedApproach string   `json:"invalidated_approach"`
	ForbiddenActions    []string `json:"forbidden_actions,omitempty"`
}

// CanonicalExemplar provides an idiomatic reference pattern for a given task category.
type CanonicalExemplar struct {
	Category        string `json:"category"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	PatternCode     string `json:"pattern_code"`
	AntiPatternCode string `json:"anti_pattern_code,omitempty"`
}

// ProgressiveScope directs retries to narrow, focused sub-gates before full-suite execution.
type ProgressiveScope struct {
	SuggestedSubGateCommand string `json:"suggested_sub_gate_command"`
	FocusedTarget           string `json:"focused_target"`
	IterationRationale      string `json:"iteration_rationale"`
}

// SupervisionEnvelope is the top-level container for adaptive retry guidance.
type SupervisionEnvelope struct {
	TaskID             string               `json:"task_id"`
	Attempt            int                  `json:"attempt"`
	MaxAttempts        int                  `json:"max_attempts"`
	Diagnostic         *FailureDiagnostic   `json:"diagnostic,omitempty"`
	NegativeBoundaries []NegativeHypothesis `json:"negative_boundaries,omitempty"`
	Exemplars          []CanonicalExemplar  `json:"exemplars,omitempty"`
	ProgressiveScope   *ProgressiveScope    `json:"progressive_scope,omitempty"`
}

// SupervisionPolicy configures retry thresholds, diagnostic extractors, and narrowing policies.
type SupervisionPolicy struct {
	MaxAttempts            int           `json:"max_attempts"`
	EnableHypothesisPrune  bool          `json:"enable_hypothesis_pruning"`
	EnableExemplarInject   bool          `json:"enable_exemplar_injection"`
	ProgressiveScopeLimit  int           `json:"progressive_scope_limit"`
	DiagnosticTailLines    int           `json:"diagnostic_tail_lines"`
	ExecutionWatchdogGrace time.Duration `json:"execution_watchdog_grace"`
}

// DefaultSupervisionPolicy returns standard, safe defaults for adaptive task supervision.
func DefaultSupervisionPolicy() SupervisionPolicy {
	return SupervisionPolicy{
		MaxAttempts:            3,
		EnableHypothesisPrune:  true,
		EnableExemplarInject:   true,
		ProgressiveScopeLimit:  5,
		DiagnosticTailLines:    40,
		ExecutionWatchdogGrace: 30 * time.Second,
	}
}
