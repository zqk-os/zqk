package community

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Schema identifier for Repo-Resolver convergence sessions.
const (
	RepoResolverConvergenceSchema = "zqk_repo_resolver_convergence_v1"
)

// Policy option actions for pipeline telemetry convergence steering.
const (
	PolicyActionNone               = "none"
	PolicyActionAdvanceCycle       = "advance_convergence_cycle"
	PolicyActionRetryFlaky         = "retry_flaky_stages"
	PolicyActionQuarantineTest     = "quarantine_failing_test"
	PolicyActionOptimizeBottleneck = "optimize_duration_bottleneck"
	PolicyActionCacheArtifacts     = "cache_build_artifacts"
	PolicyActionAdjustTimeout      = "adjust_timeout_limits"
	PolicyActionParallelize        = "parallelize_independent_steps"
	PolicyActionTriageFailures     = "triage_pipeline_failures"
)

// Policy impact classifications.
const (
	PolicyImpactSafe       = "safe"
	PolicyImpactModerate   = "moderate"
	PolicyImpactAggressive = "aggressive"
)

// Convergence status classifications.
const (
	ConvergenceStatusConverged      = "converged"
	ConvergenceStatusDegraded       = "degraded"
	ConvergenceStatusActionRequired = "action_required"
	ConvergenceStatusPending        = "pending"
	ConvergenceStatusFailed         = "failed"
)

// Domain errors for repo-resolver convergence operations.
var (
	ErrInvalidSessionID      = errors.New("repo-resolver: session_id is required")
	ErrInvalidPolicyConfig   = errors.New("repo-resolver: invalid policy configuration")
	ErrNilConvergenceSession = errors.New("repo-resolver: nil convergence session receiver")
	ErrNilTelemetryEngine    = errors.New("repo-resolver: telemetry engine uninitialized")
)

// ConvergencePolicyOption represents a configurable rule or heuristic for pipeline steering.
type ConvergencePolicyOption struct {
	PolicyID       string        `json:"policy_id" yaml:"policy_id"`
	Name           string        `json:"name" yaml:"name"`
	Description    string        `json:"description" yaml:"description"`
	Action         string        `json:"action" yaml:"action"`
	Impact         string        `json:"impact" yaml:"impact"`
	Weight         float64       `json:"weight" yaml:"weight"`
	Enabled        bool          `json:"enabled" yaml:"enabled"`
	ThresholdScore float64       `json:"threshold_score,omitempty" yaml:"threshold_score,omitempty"`
	MaxDuration    time.Duration `json:"max_duration,omitempty" yaml:"max_duration,omitempty"`
	TargetStep     string        `json:"target_step,omitempty" yaml:"target_step,omitempty"`
}

// Validate checks the structural invariants of a ConvergencePolicyOption.
func (p *ConvergencePolicyOption) Validate() error {
	if p == nil {
		return ErrInvalidPolicyConfig
	}
	if strings.TrimSpace(p.PolicyID) == "" {
		return fmt.Errorf("%w: policy_id is required", ErrInvalidPolicyConfig)
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidPolicyConfig)
	}
	if p.Weight < 0.0 || p.Weight > 1.0 {
		return fmt.Errorf("%w: weight must be between 0.0 and 1.0", ErrInvalidPolicyConfig)
	}
	switch p.Impact {
	case PolicyImpactSafe, PolicyImpactModerate, PolicyImpactAggressive, "":
		// Valid impacts
	default:
		return fmt.Errorf("%w: unknown impact %q", ErrInvalidPolicyConfig, p.Impact)
	}
	switch p.Action {
	case PolicyActionNone, PolicyActionAdvanceCycle, PolicyActionRetryFlaky,
		PolicyActionQuarantineTest, PolicyActionOptimizeBottleneck,
		PolicyActionCacheArtifacts, PolicyActionAdjustTimeout,
		PolicyActionParallelize, PolicyActionTriageFailures, "":
		// Valid actions
	default:
		return fmt.Errorf("%w: unknown action %q", ErrInvalidPolicyConfig, p.Action)
	}
	return nil
}

// ConvergencePolicyDecision records the evaluation of a single policy option.
type ConvergencePolicyDecision struct {
	PolicyID       string  `json:"policy_id" yaml:"policy_id"`
	Action         string  `json:"action" yaml:"action"`
	Applicable     bool    `json:"applicable" yaml:"applicable"`
	MatchScore     float64 `json:"match_score" yaml:"match_score"`
	DecisionReason string  `json:"decision_reason" yaml:"decision_reason"`
}

// ConvergenceEvaluation summarizes the outcome of policy evaluation against telemetry.
type ConvergenceEvaluation struct {
	EvaluationID      string                      `json:"evaluation_id" yaml:"evaluation_id"`
	SessionID         string                      `json:"session_id" yaml:"session_id"`
	Timestamp         time.Time                   `json:"timestamp" yaml:"timestamp"`
	Status            string                      `json:"status" yaml:"status"`
	ConvergenceScore  float64                     `json:"convergence_score" yaml:"convergence_score"`
	SelectedPolicy    *ConvergencePolicyOption    `json:"selected_policy,omitempty" yaml:"selected_policy,omitempty"`
	EvaluatedPolicies []ConvergencePolicyDecision `json:"evaluated_policies,omitempty" yaml:"evaluated_policies,omitempty"`
	Rationale         string                      `json:"rationale" yaml:"rationale"`
	RecommendedAction string                      `json:"recommended_action" yaml:"recommended_action"`
	TargetSteps       []string                    `json:"target_steps,omitempty" yaml:"target_steps,omitempty"`
	Bottlenecks       []string                    `json:"bottlenecks,omitempty" yaml:"bottlenecks,omitempty"`
}

// RepoResolverConvergenceSession orchestrates pipeline convergence tracking and policy evaluation.
type RepoResolverConvergenceSession struct {
	mu              sync.RWMutex
	SessionID       string                     `json:"session_id" yaml:"session_id"`
	Schema          string                     `json:"schema" yaml:"schema"`
	PersonaRef      string                     `json:"persona_ref" yaml:"persona_ref"`
	CreatedAt       time.Time                  `json:"created_at" yaml:"created_at"`
	Status          string                     `json:"status" yaml:"status"`
	Policies        []ConvergencePolicyOption  `json:"policies" yaml:"policies"`
	TelemetryEngine *PipelineTelemetryEngine   `json:"-" yaml:"-"`
	TrackingMatrix  *ConvergenceTrackingMatrix `json:"tracking_matrix,omitempty" yaml:"tracking_matrix,omitempty"`
	Evaluations     []ConvergenceEvaluation    `json:"evaluations,omitempty" yaml:"evaluations,omitempty"`
}

// DefaultConvergencePolicies returns canonical baseline convergence policies.
func DefaultConvergencePolicies() []ConvergencePolicyOption {
	return []ConvergencePolicyOption{
		{
			PolicyID:       "pol-advance-converged",
			Name:           "Advance Converged Cycle",
			Description:    "Promotes pipeline and advances cycle when all stages pass with full convergence",
			Action:         PolicyActionAdvanceCycle,
			Impact:         PolicyImpactSafe,
			Weight:         1.0,
			Enabled:        true,
			ThresholdScore: 1.0,
		},
		{
			PolicyID:       "pol-triage-critical-failures",
			Name:           "Triage Critical Pipeline Failures",
			Description:    "Requires immediate investigation when convergence score falls below acceptable threshold",
			Action:         PolicyActionTriageFailures,
			Impact:         PolicyImpactAggressive,
			Weight:         0.95,
			Enabled:        true,
			ThresholdScore: 0.50,
		},
		{
			PolicyID:       "pol-retry-flaky-stages",
			Name:           "Retry Flaky Pipeline Stages",
			Description:    "Recommends targeted retry for intermittent step failures with mixed outcomes",
			Action:         PolicyActionRetryFlaky,
			Impact:         PolicyImpactModerate,
			Weight:         0.85,
			Enabled:        true,
			ThresholdScore: 0.85,
		},
		{
			PolicyID:    "pol-quarantine-persistent-failures",
			Name:        "Quarantine Persistent Test Failures",
			Description: "Isolates failing test cases to unblock non-affected integration paths",
			Action:      PolicyActionQuarantineTest,
			Impact:      PolicyImpactModerate,
			Weight:      0.80,
			Enabled:     true,
			TargetStep:  "unit-test",
		},
		{
			PolicyID:    "pol-optimize-bottleneck",
			Name:        "Optimize Stage Bottlenecks",
			Description: "Identifies steps exceeding latency budget and recommends caching or parallelization",
			Action:      PolicyActionOptimizeBottleneck,
			Impact:      PolicyImpactSafe,
			Weight:      0.70,
			Enabled:     true,
			MaxDuration: 10 * time.Second,
		},
		{
			PolicyID:    "pol-cache-artifacts",
			Name:        "Cache Upstream Build Artifacts",
			Description: "Recommends build artifact caching when build steps dominate total execution time",
			Action:      PolicyActionCacheArtifacts,
			Impact:      PolicyImpactSafe,
			Weight:      0.65,
			Enabled:     true,
			TargetStep:  "build",
		},
	}
}

// NewRepoResolverConvergenceSession constructs a new thread-safe convergence session.
func NewRepoResolverConvergenceSession(sessionID, personaRef string, policies []ConvergencePolicyOption) (*RepoResolverConvergenceSession, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, ErrInvalidSessionID
	}
	if len(policies) == 0 {
		policies = DefaultConvergencePolicies()
	}
	for _, p := range policies {
		if err := p.Validate(); err != nil {
			return nil, err
		}
	}
	if personaRef == "" {
		personaRef = RepoResolverRole
	}

	return &RepoResolverConvergenceSession{
		SessionID:       sessionID,
		Schema:          RepoResolverConvergenceSchema,
		PersonaRef:      personaRef,
		CreatedAt:       time.Now().UTC(),
		Status:          ConvergenceStatusPending,
		Policies:        policies,
		TelemetryEngine: NewPipelineTelemetryEngine(DefaultMaxDiagnosticBatchSize),
		Evaluations:     make([]ConvergenceEvaluation, 0),
	}, nil
}

// Validate checks structural and logical invariants of the session.
func (s *RepoResolverConvergenceSession) Validate() error {
	if s == nil {
		return ErrNilConvergenceSession
	}
	if strings.TrimSpace(s.SessionID) == "" {
		return ErrInvalidSessionID
	}
	for _, p := range s.Policies {
		if err := p.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// RegisterPolicy appends or updates a policy option in the session.
func (s *RepoResolverConvergenceSession) RegisterPolicy(opt ConvergencePolicyOption) error {
	if s == nil {
		return ErrNilConvergenceSession
	}
	if err := opt.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.Policies {
		if existing.PolicyID == opt.PolicyID {
			s.Policies[i] = opt
			return nil
		}
	}
	s.Policies = append(s.Policies, opt)
	return nil
}

func (s *RepoResolverConvergenceSession) ensureTelemetryEngineLocked() *PipelineTelemetryEngine {
	if s.TelemetryEngine == nil {
		s.TelemetryEngine = NewPipelineTelemetryEngine(DefaultMaxDiagnosticBatchSize)
	}
	return s.TelemetryEngine
}

// IngestMetric records a single pipeline stage metric into the session's telemetry engine.
func (s *RepoResolverConvergenceSession) IngestMetric(metric PipelineStageMetric) error {
	if s == nil {
		return ErrNilConvergenceSession
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.ensureTelemetryEngineLocked().RecordStage(metric)
}

// IngestMetrics records multiple pipeline stage metrics into the session.
func (s *RepoResolverConvergenceSession) IngestMetrics(metrics []PipelineStageMetric) error {
	if s == nil {
		return ErrNilConvergenceSession
	}
	for _, m := range metrics {
		if err := s.IngestMetric(m); err != nil {
			return err
		}
	}
	return nil
}

// EvaluateConvergence evaluates all active policies against telemetry and computes convergence status.
func (s *RepoResolverConvergenceSession) EvaluateConvergence() (*ConvergenceEvaluation, error) {
	if s == nil {
		return nil, ErrNilConvergenceSession
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	engine := s.ensureTelemetryEngineLocked()
	matrix := engine.GetConvergenceMatrix()
	s.TrackingMatrix = matrix

	now := time.Now().UTC()
	evalID := fmt.Sprintf("eval-%s-%d", s.SessionID, len(s.Evaluations)+1)

	// Boundary condition: No telemetry metrics recorded yet
	if matrix.TotalSteps == 0 {
		evaluation := ConvergenceEvaluation{
			EvaluationID:      evalID,
			SessionID:         s.SessionID,
			Timestamp:         now,
			Status:            ConvergenceStatusPending,
			ConvergenceScore:  0.0,
			Rationale:         "No pipeline telemetry records available; awaiting execution metrics",
			RecommendedAction: PolicyActionNone,
		}
		s.Status = ConvergenceStatusPending
		s.Evaluations = append(s.Evaluations, evaluation)
		return &evaluation, nil
	}

	var decisions []ConvergencePolicyDecision
	var candidates []struct {
		policy   ConvergencePolicyOption
		decision ConvergencePolicyDecision
	}

	var failingSteps []string
	var bottlenecks []string

	for stepName, stepMetric := range matrix.StepBreakdown {
		if stepMetric.FailureCount > 0 {
			failingSteps = append(failingSteps, stepName)
		}
		if stepMetric.AverageDuration > 5*time.Second {
			bottlenecks = append(bottlenecks, stepName)
		}
	}
	sort.Strings(failingSteps)
	sort.Strings(bottlenecks)

	for _, policy := range s.Policies {
		if !policy.Enabled {
			continue
		}

		decision := ConvergencePolicyDecision{
			PolicyID: policy.PolicyID,
			Action:   policy.Action,
		}

		switch policy.Action {
		case PolicyActionAdvanceCycle:
			if matrix.ConvergenceScore >= 1.0 && matrix.FailureCount == 0 {
				decision.Applicable = true
				if len(bottlenecks) > 0 {
					decision.MatchScore = policy.Weight * 0.5
					decision.DecisionReason = fmt.Sprintf("Pipeline succeeded across %d steps but exhibits stage latency bottlenecks", matrix.TotalSteps)
				} else {
					decision.MatchScore = policy.Weight * 1.0
					decision.DecisionReason = fmt.Sprintf("Pipeline converged: 100%% success across %d steps", matrix.TotalSteps)
				}
			} else {
				decision.Applicable = false
				decision.DecisionReason = fmt.Sprintf("Pipeline has %d failures (score: %.2f < 1.0)", matrix.FailureCount, matrix.ConvergenceScore)
			}

		case PolicyActionTriageFailures:
			switch {
			case policy.ThresholdScore > 0 && matrix.ConvergenceScore < policy.ThresholdScore:
				decision.Applicable = true
				deficit := policy.ThresholdScore - matrix.ConvergenceScore
				decision.MatchScore = policy.Weight * (1.0 + deficit)
				decision.DecisionReason = fmt.Sprintf("Critical failure rate: convergence score %.2f is below threshold %.2f", matrix.ConvergenceScore, policy.ThresholdScore)
			case matrix.FailureCount > 0 && matrix.ConvergenceScore < 0.6:
				decision.Applicable = true
				decision.MatchScore = policy.Weight
				decision.DecisionReason = fmt.Sprintf("Multiple stage failures detected (%d failed)", matrix.FailureCount)
			}

		case PolicyActionRetryFlaky:
			hasFlaky := false
			for stepName, sm := range matrix.StepBreakdown {
				if sm.SuccessCount > 0 && sm.FailureCount > 0 {
					hasFlaky = true
					decision.Applicable = true
					decision.MatchScore = policy.Weight * 0.9
					decision.DecisionReason = fmt.Sprintf("Intermittent/flaky results detected in step %q (success: %d, failure: %d)", stepName, sm.SuccessCount, sm.FailureCount)
					break
				}
			}
			if !hasFlaky {
				decision.Applicable = false
				decision.DecisionReason = "No intermittent or flaky patterns detected"
			}

		case PolicyActionQuarantineTest:
			target := policy.TargetStep
			if target == "" {
				target = "unit-test"
			}
			if sm, exists := matrix.StepBreakdown[target]; exists && sm.FailureCount > 0 {
				decision.Applicable = true
				decision.MatchScore = policy.Weight * (float64(sm.FailureCount) / float64(sm.TotalExecutions))
				decision.DecisionReason = fmt.Sprintf("Persistent test failures in %q (%d failures)", target, sm.FailureCount)
			} else {
				decision.Applicable = false
				decision.DecisionReason = fmt.Sprintf("No test failures in target step %q", target)
			}

		case PolicyActionOptimizeBottleneck:
			maxDur := policy.MaxDuration
			if maxDur == 0 {
				maxDur = 5 * time.Second
			}
			isBottleneck := matrix.P95Duration > maxDur
			worstDuration := matrix.P95Duration
			worstStep := ""
			for stepName, sm := range matrix.StepBreakdown {
				if sm.AverageDuration > maxDur || sm.P95Duration > maxDur {
					isBottleneck = true
					if sm.AverageDuration > worstDuration {
						worstDuration = sm.AverageDuration
						worstStep = stepName
					}
				}
			}
			if isBottleneck {
				decision.Applicable = true
				overshoot := float64(worstDuration) / float64(maxDur)
				decision.MatchScore = policy.Weight * (1.0 + overshoot)
				if worstStep != "" {
					decision.DecisionReason = fmt.Sprintf("Stage %q duration %v exceeds latency threshold %v (%.1fx overshoot)", worstStep, worstDuration, maxDur, overshoot)
				} else {
					decision.DecisionReason = fmt.Sprintf("P95 stage duration %v exceeds latency threshold %v (%.1fx overshoot)", matrix.P95Duration, maxDur, overshoot)
				}
			} else {
				decision.Applicable = false
				decision.DecisionReason = fmt.Sprintf("Stage durations within acceptable threshold %v", maxDur)
			}

		case PolicyActionCacheArtifacts:
			target := policy.TargetStep
			if target == "" {
				target = "build"
			}
			if sm, exists := matrix.StepBreakdown[target]; exists && sm.TotalDuration > (matrix.TotalDuration/2) && matrix.TotalDuration > 1*time.Second {
				decision.Applicable = true
				ratio := float64(sm.TotalDuration) / float64(matrix.TotalDuration)
				decision.MatchScore = policy.Weight * (1.0 + ratio)
				decision.DecisionReason = fmt.Sprintf("Step %q accounts for >50%% of total duration (%v of %v)", target, sm.TotalDuration, matrix.TotalDuration)
			} else {
				decision.Applicable = false
				decision.DecisionReason = "Build duration does not dominate pipeline runtime"
			}

		default:
			decision.Applicable = false
			decision.DecisionReason = "Default policy evaluation skipped"
		}

		decisions = append(decisions, decision)
		if decision.Applicable {
			candidates = append(candidates, struct {
				policy   ConvergencePolicyOption
				decision ConvergencePolicyDecision
			}{policy: policy, decision: decision})
		}
	}

	// Rank candidates by composite match score
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].decision.MatchScore > candidates[j].decision.MatchScore
	})

	var selected *ConvergencePolicyOption
	var recommendedAction = PolicyActionNone
	var rationale = "Pipeline in steady state; no steering adjustments required"
	var sessionStatus = ConvergenceStatusConverged

	switch {
	case len(candidates) > 0:
		best := candidates[0]
		polCopy := best.policy
		selected = &polCopy
		recommendedAction = best.decision.Action
		rationale = best.decision.DecisionReason

		switch best.decision.Action {
		case PolicyActionAdvanceCycle:
			sessionStatus = ConvergenceStatusConverged
		case PolicyActionTriageFailures, PolicyActionQuarantineTest:
			sessionStatus = ConvergenceStatusActionRequired
		case PolicyActionRetryFlaky, PolicyActionOptimizeBottleneck, PolicyActionCacheArtifacts:
			sessionStatus = ConvergenceStatusDegraded
		default:
			sessionStatus = ConvergenceStatusActionRequired
		}
	case matrix.FailureCount > 0:
		sessionStatus = ConvergenceStatusDegraded
		recommendedAction = PolicyActionTriageFailures
		rationale = fmt.Sprintf("Pipeline has %d failures across %d steps", matrix.FailureCount, matrix.TotalSteps)
	}

	evaluation := ConvergenceEvaluation{
		EvaluationID:      evalID,
		SessionID:         s.SessionID,
		Timestamp:         now,
		Status:            sessionStatus,
		ConvergenceScore:  matrix.ConvergenceScore,
		SelectedPolicy:    selected,
		EvaluatedPolicies: decisions,
		Rationale:         rationale,
		RecommendedAction: recommendedAction,
		TargetSteps:       failingSteps,
		Bottlenecks:       bottlenecks,
	}

	s.Status = sessionStatus
	s.Evaluations = append(s.Evaluations, evaluation)
	return &evaluation, nil
}

// SteerPipelineConvergence evaluates convergence and returns the selected steering directive.
func (s *RepoResolverConvergenceSession) SteerPipelineConvergence() (*ConvergenceEvaluation, error) {
	return s.EvaluateConvergence()
}

// ExportSessionSummary generates a diagnostic map of the convergence session.
func (s *RepoResolverConvergenceSession) ExportSessionSummary() map[string]any {
	if s == nil {
		return map[string]any{"status": "nil_session"}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	summary := map[string]any{
		"session_id":        s.SessionID,
		"schema":            s.Schema,
		"persona_ref":       s.PersonaRef,
		"status":            s.Status,
		"created_at":        s.CreatedAt.Format(time.RFC3339),
		"policies_count":    len(s.Policies),
		"evaluations_count": len(s.Evaluations),
	}
	if s.TrackingMatrix != nil {
		summary["total_steps"] = s.TrackingMatrix.TotalSteps
		summary["success_count"] = s.TrackingMatrix.SuccessCount
		summary["failure_count"] = s.TrackingMatrix.FailureCount
		summary["convergence_score"] = s.TrackingMatrix.ConvergenceScore
	}
	if len(s.Evaluations) > 0 {
		latest := s.Evaluations[len(s.Evaluations)-1]
		summary["latest_action"] = latest.RecommendedAction
		summary["latest_rationale"] = latest.Rationale
	}
	return summary
}

// ParseConvergenceSessionBytes safely parses JSON or YAML convergence session configuration.
func ParseConvergenceSessionBytes(data []byte) (*RepoResolverConvergenceSession, error) {
	if len(data) == 0 {
		return nil, ErrMalformedPayload
	}
	if len(data) > DefaultMaxMetricPayloadBytes {
		return nil, ErrPayloadTooLarge
	}

	var session RepoResolverConvergenceSession
	if err := json.Unmarshal(data, &session); err != nil {
		if yamlErr := yaml.Unmarshal(data, &session); yamlErr != nil {
			return nil, ErrMalformedPayload
		}
	}

	if session.SessionID == "" {
		return nil, ErrInvalidSessionID
	}
	if session.Schema == "" {
		session.Schema = RepoResolverConvergenceSchema
	}
	if session.TelemetryEngine == nil {
		session.TelemetryEngine = NewPipelineTelemetryEngine(DefaultMaxDiagnosticBatchSize)
	}

	return &session, nil
}
