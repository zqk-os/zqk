package eval

import (
	"context"
	"math"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// MatrixConfig defines the parameters for a multi-scenario, multi-model evaluation run.
type MatrixConfig struct {
	Templates []*EvalTemplate `json:"templates" yaml:"templates"`
	Models    []string        `json:"models" yaml:"models"`
	Targets   []string        `json:"targets" yaml:"targets"`
	SessionID string          `json:"session_id,omitempty" yaml:"session_id,omitempty"`
}

// ModelScoreSummary encapsulates statistical aggregation for a specific model across scenario evaluations.
type ModelScoreSummary struct {
	Model         string  `json:"model"`
	RunsCount     int     `json:"runs_count"`
	PassedCount   int     `json:"passed_count"`
	PassRate      float64 `json:"pass_rate"`
	MeanScore     float64 `json:"mean_score"`
	ScoreVariance float64 `json:"score_variance"`
}

// MatrixResult encapsulates the aggregate outcome of executing an evaluation matrix.
type MatrixResult struct {
	SessionID             string                        `json:"session_id,omitempty"`
	AssessedAt            time.Time                     `json:"assessed_at"`
	TotalScenarios        int                           `json:"total_scenarios"`
	PassedScenarios       int                           `json:"passed_scenarios"`
	AverageCompositeScore float64                       `json:"average_composite_score"`
	ModelConvergence      map[string]*ModelScoreSummary `json:"model_convergence"`
	Runs                  []*EvalResult                 `json:"runs"`
	Remediations          []string                      `json:"remediations,omitempty"`
}

// MatrixEngine orchestrates matrix-based scenario evaluations across multiple templates, models, and targets.
type MatrixEngine struct {
	projectRoot string
	baseEngine  *Engine
}

// NewMatrixEngine initializes a new MatrixEngine for the given project workspace.
func NewMatrixEngine(projectRoot string) *MatrixEngine {
	return &MatrixEngine{
		projectRoot: projectRoot,
		baseEngine:  NewEngine(projectRoot),
	}
}

// ExecuteMatrix evaluates all combinations of templates and models/targets, returning aggregated convergence stats.
func (me *MatrixEngine) ExecuteMatrix(ctx context.Context, cfg MatrixConfig) (*MatrixResult, error) {
	if len(cfg.Templates) == 0 {
		return nil, errfmt.Errorf("matrix evaluation requires at least one eval template")
	}

	models := cfg.Models
	if len(models) == 0 {
		models = []string{"default"}
	}

	targets := cfg.Targets
	if len(targets) == 0 {
		targets = []string{""}
	}

	matrixRes := &MatrixResult{
		SessionID:        cfg.SessionID,
		AssessedAt:       time.Now().UTC(),
		ModelConvergence: make(map[string]*ModelScoreSummary),
		Runs:             make([]*EvalResult, 0),
		Remediations:     make([]string, 0),
	}

	modelScores := make(map[string][]float64)
	modelPasses := make(map[string]int)

	for _, model := range models {
		modelScores[model] = make([]float64, 0)
		modelPasses[model] = 0
	}

	var sumAllScores float64

	for _, tmpl := range cfg.Templates {
		for _, target := range targets {
			for _, model := range models {
				runRes, err := me.baseEngine.Execute(ctx, tmpl, target)
				if err != nil {
					return nil, errfmt.Errorf("template %s evaluation failed for model %s: %w", tmpl.TemplateID, model, err)
				}

				matrixRes.Runs = append(matrixRes.Runs, runRes)
				matrixRes.TotalScenarios++

				if runRes.Passed {
					matrixRes.PassedScenarios++
					modelPasses[model]++
				}

				modelScores[model] = append(modelScores[model], runRes.CompositeScore)
				sumAllScores += runRes.CompositeScore

				if len(runRes.Remediations) > 0 {
					matrixRes.Remediations = append(matrixRes.Remediations, runRes.Remediations...)
				}
			}
		}
	}

	if matrixRes.TotalScenarios > 0 {
		matrixRes.AverageCompositeScore = sumAllScores / float64(matrixRes.TotalScenarios)
	}

	// Compute convergence metrics per model
	for _, model := range models {
		scores := modelScores[model]
		n := len(scores)
		if n == 0 {
			continue
		}

		var mean float64
		for _, s := range scores {
			mean += s
		}
		mean = mean / float64(n)

		var variance float64
		for _, s := range scores {
			variance += math.Pow(s-mean, 2)
		}
		if n > 1 {
			variance = variance / float64(n-1)
		} else {
			variance = 0.0
		}

		matrixRes.ModelConvergence[model] = &ModelScoreSummary{
			Model:         model,
			RunsCount:     n,
			PassedCount:   modelPasses[model],
			PassRate:      float64(modelPasses[model]) / float64(n),
			MeanScore:     mean,
			ScoreVariance: variance,
		}
	}

	return matrixRes, nil
}
