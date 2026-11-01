package community

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// Canonical identifiers and roles for the repo_resolver persona.
const (
	RepoResolverRole     = "repo_resolver"
	RepoResolverSkillRef = "ASK-DEFAULT-FEED-CORRESPONDENCE"
)

// Pipeline step outcome classifications.
const (
	OutcomeSuccess = "success"
	OutcomeFailed  = "failed"
	OutcomeSkipped = "skipped"
	OutcomeTimeout = "timeout"
)

// Common error invariants for fail-closed telemetry parsing.
var (
	ErrMissingRequiredField = errors.New("pipeline telemetry: required metric field missing")
	ErrInvalidDuration      = errors.New("pipeline telemetry: metric duration must not be negative")
	ErrInvalidTimeSequence  = errors.New("pipeline telemetry: completion timestamp precedes start timestamp")
)

// PipelineStageMetric represents execution metrics for a discrete pipeline step or job.
type PipelineStageMetric struct {
	PipelineID      string            `json:"pipeline_id" yaml:"pipeline_id"`
	JobID           string            `json:"job_id" yaml:"job_id"`
	StepName        string            `json:"step_name" yaml:"step_name"`
	Outcome         string            `json:"outcome" yaml:"outcome"`
	Duration        time.Duration     `json:"duration" yaml:"duration"`
	StartedAt       time.Time         `json:"started_at" yaml:"started_at"`
	CompletedAt     time.Time         `json:"completed_at" yaml:"completed_at"`
	ThroughputItems int               `json:"throughput_items,omitempty" yaml:"throughput_items,omitempty"`
	ErrorMessage    string            `json:"error_message,omitempty" yaml:"error_message,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// Validate verifies the structural and logical invariants of a PipelineStageMetric.
func (m *PipelineStageMetric) Validate() error {
	if m == nil {
		return ErrMalformedPayload
	}
	if m.JobID == "" {
		return fmt.Errorf("%w: job_id is required", ErrMissingRequiredField)
	}
	if m.StepName == "" {
		return fmt.Errorf("%w: step_name is required", ErrMissingRequiredField)
	}
	if m.Duration < 0 {
		return ErrInvalidDuration
	}
	if !m.StartedAt.IsZero() && !m.CompletedAt.IsZero() && m.CompletedAt.Before(m.StartedAt) {
		return ErrInvalidTimeSequence
	}
	switch m.Outcome {
	case OutcomeSuccess, OutcomeFailed, OutcomeSkipped, OutcomeTimeout, "":
		// Valid outcomes
	default:
		return fmt.Errorf("%w: unknown outcome %q", ErrMalformedPayload, m.Outcome)
	}
	return nil
}

// StepMetrics contains aggregated statistics for an individual pipeline step.
type StepMetrics struct {
	StepName        string        `json:"step_name"`
	TotalExecutions int           `json:"total_executions"`
	SuccessCount    int           `json:"success_count"`
	FailureCount    int           `json:"failure_count"`
	TotalDuration   time.Duration `json:"total_duration"`
	AverageDuration time.Duration `json:"average_duration"`
	P95Duration     time.Duration `json:"p95_duration"`
}

// ConvergenceTrackingMatrix summarizes pipeline performance, throughput, and reliability.
type ConvergenceTrackingMatrix struct {
	TotalJobs        int                    `json:"total_jobs"`
	TotalSteps       int                    `json:"total_steps"`
	SuccessCount     int                    `json:"success_count"`
	FailureCount     int                    `json:"failure_count"`
	SkippedCount     int                    `json:"skipped_count"`
	TotalDuration    time.Duration          `json:"total_duration"`
	AverageDuration  time.Duration          `json:"average_duration"`
	P95Duration      time.Duration          `json:"p95_duration"`
	ThroughputItems  int                    `json:"throughput_items"`
	ConvergenceScore float64                `json:"convergence_score"`
	StepBreakdown    map[string]StepMetrics `json:"step_breakdown"`
	CalculatedAt     time.Time              `json:"calculated_at"`
}

// PipelineTelemetryEngine aggregates and tracks pipeline performance telemetry in a thread-safe manner.
type PipelineTelemetryEngine struct {
	mu      sync.RWMutex
	metrics []PipelineStageMetric
	maxCap  int
}

// NewPipelineTelemetryEngine creates an initialized telemetry engine with the given capacity limit.
func NewPipelineTelemetryEngine(maxCapacity int) *PipelineTelemetryEngine {
	if maxCapacity <= 0 {
		maxCapacity = DefaultMaxDiagnosticBatchSize
	}
	return &PipelineTelemetryEngine{
		metrics: make([]PipelineStageMetric, 0, maxCapacity),
		maxCap:  maxCapacity,
	}
}

// RecordStage validates and appends a single pipeline stage metric into the tracking store.
func (e *PipelineTelemetryEngine) RecordStage(metric PipelineStageMetric) error {
	if e == nil {
		return errors.New("pipeline telemetry: nil engine receiver")
	}
	if err := metric.Validate(); err != nil {
		return err
	}

	// Compute duration if not explicitly provided
	if metric.Duration == 0 && !metric.StartedAt.IsZero() && !metric.CompletedAt.IsZero() {
		metric.Duration = metric.CompletedAt.Sub(metric.StartedAt)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.metrics) >= e.maxCap {
		// Drop oldest entry to maintain bounded ring capacity
		e.metrics = append(e.metrics[1:], metric)
	} else {
		e.metrics = append(e.metrics, metric)
	}
	return nil
}

// RecordPayload parses raw JSON or YAML telemetry payload bytes, enforces bounds, and records the metric.
func (e *PipelineTelemetryEngine) RecordPayload(payload []byte) (*PipelineStageMetric, error) {
	if e == nil {
		return nil, errors.New("pipeline telemetry: nil engine receiver")
	}
	if len(payload) == 0 {
		return nil, ErrMalformedPayload
	}
	if len(payload) > DefaultMaxMetricPayloadBytes {
		return nil, ErrPayloadTooLarge
	}

	var metric PipelineStageMetric
	if err := json.Unmarshal(payload, &metric); err != nil {
		if yamlErr := yaml.Unmarshal(payload, &metric); yamlErr != nil {
			return nil, ErrMalformedPayload
		}
	}

	if err := e.RecordStage(metric); err != nil {
		return nil, err
	}
	return &metric, nil
}

// Count returns the number of recorded stage metrics.
func (e *PipelineTelemetryEngine) Count() int {
	if e == nil {
		return 0
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.metrics)
}

// GetConvergenceMatrix computes and returns the aggregated convergence matrix across all tracked metrics.
func (e *PipelineTelemetryEngine) GetConvergenceMatrix() *ConvergenceTrackingMatrix {
	if e == nil {
		return &ConvergenceTrackingMatrix{CalculatedAt: time.Now().UTC()}
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	matrix := &ConvergenceTrackingMatrix{
		TotalSteps:    len(e.metrics),
		StepBreakdown: make(map[string]StepMetrics),
		CalculatedAt:  time.Now().UTC(),
	}

	if len(e.metrics) == 0 {
		return matrix
	}

	uniqueJobs := make(map[string]struct{})
	stepDurations := make(map[string][]time.Duration)
	var allDurations []time.Duration

	for _, m := range e.metrics {
		if m.JobID != "" {
			uniqueJobs[m.JobID] = struct{}{}
		}
		matrix.TotalDuration += m.Duration
		matrix.ThroughputItems += m.ThroughputItems
		allDurations = append(allDurations, m.Duration)

		switch m.Outcome {
		case OutcomeSuccess:
			matrix.SuccessCount++
		case OutcomeFailed:
			matrix.FailureCount++
		case OutcomeSkipped:
			matrix.SkippedCount++
		}

		sm := matrix.StepBreakdown[m.StepName]
		sm.StepName = m.StepName
		sm.TotalExecutions++
		if m.Outcome == OutcomeSuccess {
			sm.SuccessCount++
		} else if m.Outcome == OutcomeFailed {
			sm.FailureCount++
		}
		sm.TotalDuration += m.Duration
		matrix.StepBreakdown[m.StepName] = sm
		stepDurations[m.StepName] = append(stepDurations[m.StepName], m.Duration)
	}

	matrix.TotalJobs = len(uniqueJobs)
	if matrix.TotalSteps > 0 {
		matrix.AverageDuration = matrix.TotalDuration / time.Duration(matrix.TotalSteps)
		sort.Slice(allDurations, func(i, j int) bool { return allDurations[i] < allDurations[j] })
		p95Idx := int(float64(len(allDurations)-1) * 0.95)
		matrix.P95Duration = allDurations[p95Idx]
		matrix.ConvergenceScore = float64(matrix.SuccessCount) / float64(matrix.TotalSteps)
	}

	// Compute per-step averages and percentiles
	for stepName, durations := range stepDurations {
		sm := matrix.StepBreakdown[stepName]
		if sm.TotalExecutions > 0 {
			sm.AverageDuration = sm.TotalDuration / time.Duration(sm.TotalExecutions)
			sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
			idx := int(float64(len(durations)-1) * 0.95)
			sm.P95Duration = durations[idx]
		}
		matrix.StepBreakdown[stepName] = sm
	}

	return matrix
}

// ExportMetricEvents converts recorded pipeline metrics into standard community MetricEvent structures.
func (e *PipelineTelemetryEngine) ExportMetricEvents(clusterHash string) []MetricEvent {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()

	events := make([]MetricEvent, 0, len(e.metrics))
	for idx, m := range e.metrics {
		ts := m.CompletedAt
		if ts.IsZero() {
			ts = m.StartedAt
		}
		if ts.IsZero() {
			ts = time.Now().UTC()
		}

		attrs := make(map[string]string)
		for k, v := range m.Metadata {
			attrs[k] = v
		}
		attrs["job_id"] = m.JobID
		attrs["pipeline_id"] = m.PipelineID
		attrs["step_name"] = m.StepName
		attrs["outcome"] = m.Outcome
		if m.ErrorMessage != "" {
			attrs["error_message"] = m.ErrorMessage
		}

		events = append(events, MetricEvent{
			EventID:       fmt.Sprintf("evt-pipeline-%d", idx+1),
			Timestamp:     ts,
			Category:      "pipeline",
			Action:        m.StepName,
			Attributes:    attrs,
			DurationMs:    m.Duration.Milliseconds(),
			ClusterHash:   clusterHash,
			ExecutionMode: "ci-pipeline",
		})
	}
	return events
}

// PersonaMetadata represents the parsed structure of a persona definition YAML.
type PersonaMetadata struct {
	ID             string   `yaml:"id"`
	Kind           string   `yaml:"kind"`
	Role           string   `yaml:"role"`
	Status         string   `yaml:"status"`
	Title          string   `yaml:"title"`
	Description    string   `yaml:"description"`
	AgentSkillRefs []string `yaml:"agent_skill_refs"`
}

// LoadPersonaFromCAS loads and decodes a persona YAML file directly from .zqk/process/personas/.
func LoadPersonaFromCAS(repoRoot, personaID string) (*PersonaMetadata, error) {
	indexFile := filepath.Join(repoRoot, paths.ProcessPersonasDir, ".persona.index")
	data, err := fileutil.ReadFile(indexFile)
	if err != nil {
		return nil, fmt.Errorf("read persona index: %w", err)
	}

	var idx struct {
		Mappings map[string]string `json:"mappings"`
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse persona index: %w", err)
	}

	hash, ok := idx.Mappings[personaID]
	if !ok {
		return nil, fmt.Errorf("persona %q not found in CAS index", personaID)
	}

	targetPath := filepath.Join(repoRoot, paths.ProcessPersonasDir, hash+".yaml")
	fileBytes, err := fileutil.ReadFile(targetPath)
	if err != nil {
		return nil, fmt.Errorf("read persona CAS file %s: %w", targetPath, err)
	}

	var meta PersonaMetadata
	if err := yaml.Unmarshal(fileBytes, &meta); err != nil {
		return nil, fmt.Errorf("parse persona yaml: %w", err)
	}
	return &meta, nil
}
