package whatsnext

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Pipeline telemetry schema and file name constants.
const (
	PipelineTelemetrySchema   = "zqk_pipeline_telemetry_feedback_v1"
	PipelineTelemetryFileName = "pipeline_telemetry.json"
)

// Failure categorization constants for pipeline execution telemetry.
const (
	FailureCategoryNone               = "none"
	FailureCategoryTest               = "test_failure"
	FailureCategoryLint               = "lint_failure"
	FailureCategoryBuild              = "build_failure"
	FailureCategoryTimeout            = "timeout"
	FailureCategoryPayloadGate        = "payload_gate_failure"
	FailureCategoryResourceExhaustion = "resource_exhaustion"
)

// Recommended convergence action constants.
const (
	ActionNone                 = "none"
	ActionAdvanceCycle         = "advance_execution_cycle"
	ActionTriageFailures       = "triage_pipeline_failures"
	ActionRemediateFlakes      = "remediate_flaky_stages"
	ActionOptimizeDuration     = "optimize_stage_duration"
	ActionVerifyCleanWorkspace = "verify_clean_workspace"
)

// Pipeline status classifications.
const (
	StatusPassed    = "passed"
	StatusFailed    = "failed"
	StatusDegraded  = "degraded"
	StatusConverged = "converged"
)

var (
	ErrMalformedTelemetryFeedback = errors.New("telemetry feedback: malformed or unparseable JSON payload")
	ErrNegativeDuration           = errors.New("telemetry feedback: duration cannot be negative")
)

// PipelineTelemetrySummary models build/pipeline execution telemetry and convergence guidance.
type PipelineTelemetrySummary struct {
	Schema              string        `json:"schema"`
	RunID               string        `json:"run_id"`
	PipelineID          string        `json:"pipeline_id,omitempty"`
	Status              string        `json:"status"`
	Duration            time.Duration `json:"duration"`
	DurationMs          int64         `json:"duration_ms"`
	FailureCategory     string        `json:"failure_category"`
	FailureDetails      []string      `json:"failure_details,omitempty"`
	RecommendedAction   string        `json:"recommended_action"`
	TotalJobs           int           `json:"total_jobs,omitempty"`
	TotalSteps          int           `json:"total_steps,omitempty"`
	FailedSteps         int           `json:"failed_steps,omitempty"`
	ThroughputPerMinute float64       `json:"throughput_per_minute,omitempty"`
	ConvergenceScore    float64       `json:"convergence_score,omitempty"`
	Timestamp           time.Time     `json:"timestamp"`
}

// Validate checks structural and logical invariants of a PipelineTelemetrySummary.
func (s *PipelineTelemetrySummary) Validate() error {
	if s == nil {
		return nil
	}
	if s.Duration < 0 || s.DurationMs < 0 {
		return ErrNegativeDuration
	}
	return nil
}

// ParsePipelineTelemetryBytes safely parses raw telemetry bytes with graceful fallback for empty inputs.
func ParsePipelineTelemetryBytes(data []byte) (*PipelineTelemetrySummary, error) {
	clean := strings.TrimSpace(string(data))
	if len(clean) == 0 {
		return nil, nil
	}

	var summary PipelineTelemetrySummary
	if err := json.Unmarshal([]byte(clean), &summary); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedTelemetryFeedback, err)
	}

	if err := summary.Validate(); err != nil {
		return nil, err
	}

	if summary.Schema == "" {
		summary.Schema = PipelineTelemetrySchema
	}
	if summary.DurationMs == 0 && summary.Duration > 0 {
		summary.DurationMs = summary.Duration.Milliseconds()
	} else if summary.Duration == 0 && summary.DurationMs > 0 {
		summary.Duration = time.Duration(summary.DurationMs) * time.Millisecond
	}

	return &summary, nil
}

// LoadPipelineTelemetrySummary loads and decodes the pipeline telemetry feedback summary from project storage.
func LoadPipelineTelemetrySummary(projectRoot string) (*PipelineTelemetrySummary, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return nil, nil
	}

	targetPath := filepath.Join(paths.StateDirPath(projectRoot), PipelineTelemetryFileName)
	if !fileutil.Exists(targetPath) {
		return nil, nil
	}

	data, err := fileutil.ReadFile(targetPath)
	if err != nil {
		return nil, err
	}

	return ParsePipelineTelemetryBytes(data)
}

// IngestPipelineTelemetry writes a pipeline telemetry summary atomically into the project state directory.
func IngestPipelineTelemetry(projectRoot string, summary *PipelineTelemetrySummary) error {
	if strings.TrimSpace(projectRoot) == "" {
		return errors.New("telemetry feedback: projectRoot cannot be empty")
	}
	if summary == nil {
		return errors.New("telemetry feedback: summary cannot be nil")
	}
	if err := summary.Validate(); err != nil {
		return err
	}

	stateDir := paths.StateDirPath(projectRoot)
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		return fmt.Errorf("telemetry feedback: create state dir: %w", err)
	}

	if summary.Schema == "" {
		summary.Schema = PipelineTelemetrySchema
	}
	if summary.Timestamp.IsZero() {
		summary.Timestamp = time.Now().UTC()
	}

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("telemetry feedback: marshal json: %w", err)
	}

	targetPath := filepath.Join(stateDir, PipelineTelemetryFileName)
	if err := fileutil.WriteFile(targetPath, data, paths.FilePerm644); err != nil {
		return fmt.Errorf("telemetry feedback: write file: %w", err)
	}
	return nil
}

// EvaluatePipelineConvergenceCue returns actionable advice for whats-next based on pipeline telemetry.
func EvaluatePipelineConvergenceCue(summary *PipelineTelemetrySummary) string {
	if summary == nil {
		return ""
	}

	status := strings.ToLower(strings.TrimSpace(summary.Status))
	if status == StatusPassed || status == StatusConverged {
		if summary.RecommendedAction == ActionAdvanceCycle {
			return "Pipeline healthy: All build and verification stages converged. Ready to advance execution cycle."
		}
		return ""
	}

	if status == StatusFailed || summary.FailedSteps > 0 {
		category := summary.FailureCategory
		if category == "" || category == FailureCategoryNone {
			category = "pipeline failure"
		}
		action := summary.RecommendedAction
		if action == "" || action == ActionNone {
			action = ActionTriageFailures
		}
		return fmt.Sprintf("Pipeline alert: %s detected (run_id=%s, failed_steps=%d) — recommended action: %s.",
			category, summary.RunID, summary.FailedSteps, action)
	}

	if status == StatusDegraded {
		return fmt.Sprintf("Pipeline degraded: duration (%s) exceeds baseline — recommended action: %s.",
			summary.Duration.Round(time.Second), summary.RecommendedAction)
	}

	return ""
}
