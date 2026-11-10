package router

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	// ErrInvalidBudget denotes an unallowable zero or negative token budget.
	ErrInvalidBudget = errors.New("token budget must be strictly positive")

	// ErrBudgetExhausted denotes that the allocated session token budget has been breached.
	ErrBudgetExhausted = errors.New("session token budget exhausted")

	// ErrNoViableModel denotes that no candidate model satisfies quality or variance criteria.
	ErrNoViableModel = errors.New("no viable model meets routing constraints")

	// ErrInvalidRule indicates missing scenario identifiers or out-of-range thresholds.
	ErrInvalidRule = errors.New("invalid routing rule configuration")

	// ErrInvalidTier indicates an unrecognized or unconfigured model tier.
	ErrInvalidTier = errors.New("invalid model tier")
)

// RoutingRule defines quality, latency, and variance constraints for scenario execution.
type RoutingRule struct {
	ScenarioID   string    `json:"scenario_id"`
	MinPassRate  float64   `json:"min_pass_rate"`
	MaxVariance  float64   `json:"max_variance"`
	MaxLatencyMs float64   `json:"max_latency_ms"`
	FallbackTier ModelTier `json:"fallback_tier"`
}

// Validate verifies that rule parameters are within valid logical ranges.
func (r RoutingRule) Validate() error {
	if strings.TrimSpace(r.ScenarioID) == "" {
		return fmt.Errorf("%w: scenario_id cannot be blank", ErrInvalidRule)
	}
	if r.MinPassRate < 0.0 || r.MinPassRate > 1.0 {
		return fmt.Errorf("%w: min_pass_rate must be between 0.0 and 1.0", ErrInvalidRule)
	}
	if r.MaxVariance < 0.0 {
		return fmt.Errorf("%w: max_variance must be non-negative", ErrInvalidRule)
	}
	if r.MaxLatencyMs <= 0.0 {
		return fmt.Errorf("%w: max_latency_ms must be strictly positive", ErrInvalidRule)
	}
	return nil
}

// ModelCostProfile holds cost parameters and model tier categorization.
type ModelCostProfile struct {
	ModelID   string    `json:"model_id"`
	Tier      ModelTier `json:"tier"`
	CostPer1k float64   `json:"cost_per_1k"` // Normalized cost per 1,000 tokens
}

// ModelTelemetry encapsulates live empirical benchmark scorecard metrics.
type ModelTelemetry struct {
	ModelID   string  `json:"model_id"`
	PassRate  float64 `json:"pass_rate"`
	Variance  float64 `json:"variance"`
	LatencyMs float64 `json:"latency_ms"`
	Degraded  bool    `json:"degraded"`
}

// AdaptiveSessionRequest holds dispatch requirements for a convergence session task.
type AdaptiveSessionRequest struct {
	SessionID       string            `json:"session_id"`
	ScenarioID      string            `json:"scenario_id"`
	EstimatedTokens int               `json:"estimated_tokens"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// AdaptiveSessionResponse details the resolved model dispatch and budget telemetry.
type AdaptiveSessionResponse struct {
	SessionID     string    `json:"session_id"`
	SelectedModel string    `json:"selected_model"`
	SelectedTier  ModelTier `json:"selected_tier"`
	TokensUsed    int       `json:"tokens_used"`
	Cost          float64   `json:"cost"`
	FallbackUsed  bool      `json:"fallback_used"`
}

// AdaptiveRouter implements cost-optimal model routing and token budget governance.
type AdaptiveRouter struct {
	mu            sync.RWMutex
	rules         map[string]RoutingRule
	costProfiles  map[string]ModelCostProfile
	telemetry     map[string]ModelTelemetry
	totalBudget   int
	tokensUsed    int
	fallbackTier  ModelTier
	fallbackModel string
}

// NewAdaptiveRouter initializes an AdaptiveRouter with budget and fallback tier.
func NewAdaptiveRouter(totalBudget int, fallbackTier ModelTier, fallbackModel string) (*AdaptiveRouter, error) {
	if totalBudget <= 0 {
		return nil, ErrInvalidBudget
	}
	if !fallbackTier.Valid() {
		return nil, fmt.Errorf("%w: unrecognized fallback tier", ErrInvalidTier)
	}
	return &AdaptiveRouter{
		rules:         make(map[string]RoutingRule),
		costProfiles:  make(map[string]ModelCostProfile),
		telemetry:     make(map[string]ModelTelemetry),
		totalBudget:   totalBudget,
		fallbackTier:  fallbackTier,
		fallbackModel: fallbackModel,
	}, nil
}

// RegisterRule binds a scenario routing rule.
func (r *AdaptiveRouter) RegisterRule(rule RoutingRule) error {
	if err := rule.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rules[rule.ScenarioID] = rule
	return nil
}

// RegisterModelCost stores the cost profile for a candidate model.
func (r *AdaptiveRouter) RegisterModelCost(profile ModelCostProfile) error {
	if strings.TrimSpace(profile.ModelID) == "" {
		return ErrEmptyModel
	}
	if profile.CostPer1k < 0.0 {
		return fmt.Errorf("%w: cost cannot be negative", ErrInvalidRule)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.costProfiles[profile.ModelID] = profile
	return nil
}

// UpdateTelemetry updates empirical telemetry for a given model.
func (r *AdaptiveRouter) UpdateTelemetry(tel ModelTelemetry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.telemetry[tel.ModelID] = tel
}

// RouteSession resolves the cost-optimal model or fallback under budget limits.
func (r *AdaptiveRouter) RouteSession(_ context.Context, req AdaptiveSessionRequest) (*AdaptiveSessionResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if req.EstimatedTokens <= 0 {
		req.EstimatedTokens = 500 // baseline estimate
	}

	if r.tokensUsed+req.EstimatedTokens > r.totalBudget {
		return nil, ErrBudgetExhausted
	}

	rule, hasRule := r.rules[req.ScenarioID]
	candidates := r.gatherCandidateModels(rule, hasRule)

	if len(candidates) > 0 {
		best := candidates[0]
		cost := (float64(req.EstimatedTokens) / 1000.0) * best.CostPer1k
		r.tokensUsed += req.EstimatedTokens
		return &AdaptiveSessionResponse{
			SessionID:     req.SessionID,
			SelectedModel: best.ModelID,
			SelectedTier:  best.Tier,
			TokensUsed:    req.EstimatedTokens,
			Cost:          cost,
			FallbackUsed:  false,
		}, nil
	}

	// Fallback path
	if r.fallbackModel != "" {
		profile, found := r.costProfiles[r.fallbackModel]
		cost := 0.0
		tier := r.fallbackTier
		if found {
			cost = (float64(req.EstimatedTokens) / 1000.0) * profile.CostPer1k
			tier = profile.Tier
		}
		r.tokensUsed += req.EstimatedTokens
		return &AdaptiveSessionResponse{
			SessionID:     req.SessionID,
			SelectedModel: r.fallbackModel,
			SelectedTier:  tier,
			TokensUsed:    req.EstimatedTokens,
			Cost:          cost,
			FallbackUsed:  true,
		}, nil
	}

	return nil, ErrNoViableModel
}

// gatherCandidateModels identifies and sorts viable models by ascending cost.
func (r *AdaptiveRouter) gatherCandidateModels(rule RoutingRule, hasRule bool) []ModelCostProfile {
	var viable []ModelCostProfile
	for modelID, profile := range r.costProfiles {
		tel, hasTel := r.telemetry[modelID]
		if !hasTel || tel.Degraded {
			continue
		}
		if hasRule {
			if tel.PassRate < rule.MinPassRate || tel.Variance > rule.MaxVariance || tel.LatencyMs > rule.MaxLatencyMs {
				continue
			}
		}
		viable = append(viable, profile)
	}

	sort.Slice(viable, func(i, j int) bool {
		return viable[i].CostPer1k < viable[j].CostPer1k
	})
	return viable
}

// BudgetRemaining returns remaining token capacity.
func (r *AdaptiveRouter) BudgetRemaining() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rem := r.totalBudget - r.tokensUsed
	if rem < 0 {
		return 0
	}
	return rem
}
