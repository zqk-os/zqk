package eval

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// GateConfig holds the configuration parameters for the AutoEvalGate.
type GateConfig struct {
	MinPassRate    float64  `json:"min_pass_rate" yaml:"min_pass_rate"`
	MaxVariance    float64  `json:"max_variance" yaml:"max_variance"`
	RequiredModels []string `json:"required_models,omitempty" yaml:"required_models,omitempty"`
}

// ModelVarianceSummary encapsulates the variance statistics for a specific model.
type ModelVarianceSummary struct {
	Model             string  `json:"model"`
	TotalScenarios    int     `json:"total_scenarios"`
	PassedScenarios   int     `json:"passed_scenarios"`
	PassRate          float64 `json:"pass_rate"`
	DeviationFromMean float64 `json:"deviation_from_mean"`
}

// GateDecision represents the final gating evaluation decision and statistical summary.
type GateDecision struct {
	Passed         bool                             `json:"passed"`
	MeanPassRate   float64                          `json:"mean_pass_rate"`
	Variance       float64                          `json:"variance"`
	StdDev         float64                          `json:"std_dev"`
	ModelSummaries map[string]*ModelVarianceSummary `json:"model_summaries"`
	Violations     []string                         `json:"violations,omitempty"`
	EvaluatedAt    time.Time                        `json:"evaluated_at"`
}

// AutoEvalGate evaluates model convergence batches against strict quality gates and variance limits.
type AutoEvalGate struct {
	config GateConfig
}

// Validate checks static invariants on GateConfig.
func (c *GateConfig) Validate() error {
	if c.MinPassRate < 0.0 || c.MinPassRate > 1.0 {
		return errfmt.Errorf("min_pass_rate must be between 0.0 and 1.0 (got %f)", c.MinPassRate)
	}
	if c.MaxVariance < 0.0 {
		return errfmt.Errorf("max_variance must be non-negative (got %f)", c.MaxVariance)
	}
	return nil
}

// NewAutoEvalGate constructs and validates a new AutoEvalGate.
func NewAutoEvalGate(config GateConfig) (*AutoEvalGate, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &AutoEvalGate{config: config}, nil
}

// CalculateVariance computes the mean, sample variance, and standard deviation for a slice of rates.
func CalculateVariance(values []float64) (float64, float64, float64) {
	n := len(values)
	if n == 0 {
		return 0, 0, 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(n)
	if n == 1 {
		return mean, 0, 0
	}
	var sumSqDiff float64
	for _, v := range values {
		diff := v - mean
		sumSqDiff += diff * diff
	}
	variance := sumSqDiff / float64(n-1)
	return mean, variance, math.Sqrt(variance)
}

// EvaluateMatrix evaluates an executed MatrixResult against the auto-evaluation gate rules.
func (g *AutoEvalGate) EvaluateMatrix(ctx context.Context, res *MatrixResult) (*GateDecision, error) {
	if res == nil {
		return nil, errfmt.Errorf("matrix result cannot be nil")
	}
	if len(res.ModelConvergence) == 0 {
		return nil, errfmt.Errorf("matrix result contains no model convergence data")
	}

	rates := make([]float64, 0, len(res.ModelConvergence))
	models := make([]string, 0, len(res.ModelConvergence))
	summaries := make(map[string]*ModelVarianceSummary, len(res.ModelConvergence))

	for model, summary := range res.ModelConvergence {
		rates = append(rates, summary.PassRate)
		models = append(models, model)
		summaries[model] = &ModelVarianceSummary{
			Model:           model,
			TotalScenarios:  summary.RunsCount,
			PassedScenarios: summary.PassedCount,
			PassRate:        summary.PassRate,
		}
	}

	return g.buildDecision(models, rates, summaries)
}

// EvaluateOutcomes evaluates a raw map of model names to ScenarioOutcome slices.
func (g *AutoEvalGate) EvaluateOutcomes(ctx context.Context, outcomes map[string][]ScenarioOutcome) (*GateDecision, error) {
	if len(outcomes) == 0 {
		return nil, errfmt.Errorf("outcomes map cannot be empty")
	}

	rates := make([]float64, 0, len(outcomes))
	models := make([]string, 0, len(outcomes))
	summaries := make(map[string]*ModelVarianceSummary, len(outcomes))

	for model, list := range outcomes {
		if len(list) == 0 {
			return nil, errfmt.Errorf("model %q has no scenario outcomes", model)
		}
		var passed int
		for _, o := range list {
			if o.Success {
				passed++
			}
		}
		passRate := float64(passed) / float64(len(list))
		rates = append(rates, passRate)
		models = append(models, model)
		summaries[model] = &ModelVarianceSummary{
			Model:           model,
			TotalScenarios:  len(list),
			PassedScenarios: passed,
			PassRate:        passRate,
		}
	}

	return g.buildDecision(models, rates, summaries)
}

func (g *AutoEvalGate) checkViolations(mean, variance float64, models []string, summaries map[string]*ModelVarianceSummary) []string {
	var violations []string

	if mean < g.config.MinPassRate {
		violations = append(violations, fmt.Sprintf("mean pass rate %.2f%% below minimum required %.2f%%", mean*100, g.config.MinPassRate*100))
	}

	if g.config.MaxVariance > 0 && variance > g.config.MaxVariance {
		violations = append(violations, fmt.Sprintf("cross-model variance %.4f exceeds maximum tolerance %.4f", variance, g.config.MaxVariance))
	}

	for _, reqModel := range g.config.RequiredModels {
		if _, ok := summaries[reqModel]; !ok {
			violations = append(violations, fmt.Sprintf("required model %q was not evaluated", reqModel))
		}
	}

	return violations
}

func (g *AutoEvalGate) buildDecision(models []string, rates []float64, summaries map[string]*ModelVarianceSummary) (*GateDecision, error) {
	mean, variance, stdDev := CalculateVariance(rates)

	for _, s := range summaries {
		s.DeviationFromMean = s.PassRate - mean
	}

	violations := g.checkViolations(mean, variance, models, summaries)

	return &GateDecision{
		Passed:         len(violations) == 0,
		MeanPassRate:   mean,
		Variance:       variance,
		StdDev:         stdDev,
		ModelSummaries: summaries,
		Violations:     violations,
		EvaluatedAt:    time.Now().UTC(),
	}, nil
}

// FormatDashboard generates a text-formatted telemetry dashboard visualizing cross-model variance.
func (g *AutoEvalGate) FormatDashboard(d *GateDecision) string {
	if d == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("=== Convergence Session Auto-Evaluation Gate Dashboard ===\n")
	statusStr := "PASSED"
	if !d.Passed {
		statusStr = "FAILED"
	}
	fmt.Fprintf(&b, "Gate Status:       %s\n", statusStr)
	fmt.Fprintf(&b, "Evaluated At:      %s\n", d.EvaluatedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "Mean Pass Rate:    %.2f%%\n", d.MeanPassRate*100)
	fmt.Fprintf(&b, "Score Variance:    %.4f\n", d.Variance)
	fmt.Fprintf(&b, "Standard Dev:      %.4f\n", d.StdDev)
	b.WriteString("\nModel Telemetry Breakdown:\n")
	for model, s := range d.ModelSummaries {
		fmt.Fprintf(&b, "  • %-20s: %d/%d (%.2f%%) [dev: %+.2f%%]\n",
			model, s.PassedScenarios, s.TotalScenarios, s.PassRate*100, s.DeviationFromMean*100)
	}
	if len(d.Violations) > 0 {
		b.WriteString("\nGate Violations:\n")
		for _, v := range d.Violations {
			fmt.Fprintf(&b, "  [!] %s\n", v)
		}
	}
	b.WriteString("===========================================================\n")
	return b.String()
}
