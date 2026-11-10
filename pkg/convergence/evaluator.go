package convergence

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

type ImpactClassification string

const (
	ImpactCritical ImpactClassification = "critical"
	ImpactHigh     ImpactClassification = "high"
	ImpactMedium   ImpactClassification = "medium"
	ImpactLow      ImpactClassification = "low"
)

type ActionType string

const (
	ActionRetry         ActionType = "retry"
	ActionCachePurge    ActionType = "cache_purge"
	ActionTaskIsolation ActionType = "task_isolation"
	ActionAgentDispatch ActionType = "agent_dispatch"
)

var (
	ErrInvalidRuleConfig     = errors.New("invalid convergence policy rule configuration")
	ErrCorruptedTelemetry    = errors.New("corrupted failure telemetry payload")
	ErrCyclicRemediationLoop = errors.New("remediation cycle limit exceeded: potential divergent loop")
	ErrNoMatchingRule        = errors.New("no matching convergence policy rule found")
	ErrRuleAlreadyExists     = errors.New("policy rule ID already registered")
)

type PolicyRule struct {
	ID            string               `json:"id" yaml:"id"`
	Name          string               `json:"name" yaml:"name"`
	TargetFilter  string               `json:"target_filter" yaml:"target_filter"`
	Impact        ImpactClassification `json:"impact" yaml:"impact"`
	Action        ActionType           `json:"action" yaml:"action"`
	MaxRetries    int                  `json:"max_retries" yaml:"max_retries"`
	BackoffBaseMs int                  `json:"backoff_base_ms" yaml:"backoff_base_ms"`
	Weight        int                  `json:"weight" yaml:"weight"`
	Enabled       bool                 `json:"enabled" yaml:"enabled"`
}

type TelemetryPayload struct {
	SessionID string            `json:"session_id"`
	JobID     string            `json:"job_id"`
	StepName  string            `json:"step_name"`
	ExitCode  int               `json:"exit_code"`
	ErrorMsg  string            `json:"error_msg"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type PolicyEvaluator struct {
	mu    sync.RWMutex
	rules map[string]*PolicyRule
}

func NewPolicyEvaluator() *PolicyEvaluator {
	return &PolicyEvaluator{
		rules: make(map[string]*PolicyRule),
	}
}

func ValidateRule(rule *PolicyRule) error {
	if rule == nil {
		return fmt.Errorf("%w: rule cannot be nil", ErrInvalidRuleConfig)
	}
	if strings.TrimSpace(rule.ID) == "" {
		return fmt.Errorf("%w: rule id cannot be empty", ErrInvalidRuleConfig)
	}
	if strings.TrimSpace(rule.Name) == "" {
		return fmt.Errorf("%w: rule name cannot be empty", ErrInvalidRuleConfig)
	}
	switch rule.Impact {
	case ImpactCritical, ImpactHigh, ImpactMedium, ImpactLow:
	default:
		return fmt.Errorf("%w: unknown impact classification: %s", ErrInvalidRuleConfig, rule.Impact)
	}
	switch rule.Action {
	case ActionRetry, ActionCachePurge, ActionTaskIsolation, ActionAgentDispatch:
	default:
		return fmt.Errorf("%w: unknown action type: %s", ErrInvalidRuleConfig, rule.Action)
	}
	if rule.Weight < 1 || rule.Weight > 100 {
		return fmt.Errorf("%w: weight must be within [1, 100], got %d", ErrInvalidRuleConfig, rule.Weight)
	}
	if rule.MaxRetries < 0 || rule.MaxRetries > 10 {
		return fmt.Errorf("%w: max_retries must be within [0, 10], got %d", ErrInvalidRuleConfig, rule.MaxRetries)
	}
	if rule.BackoffBaseMs <= 0 || rule.BackoffBaseMs > 60000 {
		return fmt.Errorf("%w: backoff_base_ms must be within (0, 60000], got %d", ErrInvalidRuleConfig, rule.BackoffBaseMs)
	}
	return nil
}

func ValidateTelemetry(payload *TelemetryPayload) error {
	if payload == nil {
		return fmt.Errorf("%w: payload cannot be nil", ErrCorruptedTelemetry)
	}
	if strings.TrimSpace(payload.SessionID) == "" {
		return fmt.Errorf("%w: session_id cannot be empty", ErrCorruptedTelemetry)
	}
	if strings.TrimSpace(payload.JobID) == "" {
		return fmt.Errorf("%w: job_id cannot be empty", ErrCorruptedTelemetry)
	}
	if payload.ExitCode < 0 {
		return fmt.Errorf("%w: exit_code cannot be negative: %d", ErrCorruptedTelemetry, payload.ExitCode)
	}
	return nil
}

func (pe *PolicyEvaluator) RegisterRule(rule *PolicyRule) error {
	if err := ValidateRule(rule); err != nil {
		return err
	}
	pe.mu.Lock()
	defer pe.mu.Unlock()

	if _, exists := pe.rules[rule.ID]; exists {
		return fmt.Errorf("%w: %s", ErrRuleAlreadyExists, rule.ID)
	}
	pe.rules[rule.ID] = rule
	return nil
}

func (pe *PolicyEvaluator) RuleCount() int {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return len(pe.rules)
}

func (pe *PolicyEvaluator) Evaluate(payload *TelemetryPayload) (*PolicyRule, error) {
	if err := ValidateTelemetry(payload); err != nil {
		return nil, err
	}

	pe.mu.RLock()
	defer pe.mu.RUnlock()

	var matched []*PolicyRule
	for _, r := range pe.rules {
		if !r.Enabled {
			continue
		}
		if pe.matchesFilter(r.TargetFilter, payload) {
			matched = append(matched, r)
		}
	}

	if len(matched) == 0 {
		return nil, ErrNoMatchingRule
	}

	best := matched[0]
	for _, candidate := range matched[1:] {
		if candidate.Weight > best.Weight {
			best = candidate
		} else if candidate.Weight == best.Weight {
			if impactRank(candidate.Impact) > impactRank(best.Impact) {
				best = candidate
			}
		}
	}

	return best, nil
}

func (pe *PolicyEvaluator) matchesFilter(filter string, payload *TelemetryPayload) bool {
	cleanFilter := strings.ToLower(strings.TrimSpace(filter))
	if cleanFilter == "" || cleanFilter == "*" {
		return true
	}

	target := strings.ToLower(fmt.Sprintf("%s %s %s", payload.JobID, payload.StepName, payload.ErrorMsg))
	parts := strings.FieldsFunc(cleanFilter, func(r rune) bool {
		return r == ',' || r == '|'
	})
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p != "" && strings.Contains(target, p) {
			return true
		}
	}
	return strings.Contains(target, cleanFilter)
}

func impactRank(impact ImpactClassification) int {
	switch impact {
	case ImpactCritical:
		return 4
	case ImpactHigh:
		return 3
	case ImpactMedium:
		return 2
	case ImpactLow:
		return 1
	default:
		return 0
	}
}
