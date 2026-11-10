package eval

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

const (
	BenchmarkScorecardSchemaV1 = "benchmark_scorecard_v1"
	PolicyBalanced             = "balanced"
	PolicyLatencyCritical      = "latency_critical"
	PolicyQualityFirst         = "quality_first"
)

// BenchmarkRun captures an individual scenario evaluation outcome for a model.
type BenchmarkRun struct {
	RunID      string    `json:"run_id"`
	ScenarioID string    `json:"scenario_id"`
	ModelID    string    `json:"model_id"`
	Passed     bool      `json:"passed"`
	DurationMs float64   `json:"duration_ms"`
	TokensUsed int       `json:"tokens_used"`
	Variance   float64   `json:"variance"`
	Timestamp  time.Time `json:"timestamp"`
}

// ModelScorecard aggregates evaluation metrics and rank for a distinct model.
type ModelScorecard struct {
	Rank                 int     `json:"rank"`
	ModelID              string  `json:"model_id"`
	CompositeScore       float64 `json:"composite_score"`
	ScenariosEvaluated   int     `json:"scenarios_evaluated"`
	PassRate             float64 `json:"pass_rate"`
	LatencyP50Ms         float64 `json:"latency_p50_ms"`
	LatencyP99Ms         float64 `json:"latency_p99_ms"`
	TokenCostPerThousand float64 `json:"token_cost_per_thousand"`
	StabilityIndex       float64 `json:"stability_index"`
}

// BenchmarkScorecard represents the top-level versioned CAS-ready scorecard.
type BenchmarkScorecard struct {
	SchemaVersion        string           `json:"schema_version"`
	ScorecardID          string           `json:"scorecard_id"`
	CreatedAt            string           `json:"created_at"`
	RankingPolicy        string           `json:"ranking_policy"`
	Leaderboard          []ModelScorecard `json:"leaderboard"`
	QuarantinedRunsCount int              `json:"quarantined_runs_count"`
}

// ScorecardSynthesizer aggregates runs and generates verified scorecards.
type ScorecardSynthesizer struct {
	defaultPolicy string
}

// NewScorecardSynthesizer instantiates a scorecard synthesizer.
func NewScorecardSynthesizer(policy string) *ScorecardSynthesizer {
	if policy == "" {
		policy = PolicyBalanced
	}
	return &ScorecardSynthesizer{defaultPolicy: policy}
}

// IsValidRun checks if a run passes basic static integrity constraints.
func IsValidRun(r BenchmarkRun) bool {
	if r.RunID == "" || r.ModelID == "" || r.ScenarioID == "" {
		return false
	}
	if r.DurationMs < 0 || math.IsNaN(r.DurationMs) || math.IsInf(r.DurationMs, 0) {
		return false
	}
	if r.TokensUsed < 0 {
		return false
	}
	if math.IsNaN(r.Variance) || math.IsInf(r.Variance, 0) || r.Variance < 0 {
		return false
	}
	return true
}

// Synthesize processes runs, filters quarantined items, and yields a ranked scorecard.
func (s *ScorecardSynthesizer) Synthesize(scorecardID string, runs []BenchmarkRun, policy string) (*BenchmarkScorecard, []string, error) {
	if scorecardID == "" {
		return nil, nil, errors.New("scorecard_id cannot be empty")
	}
	if policy == "" {
		policy = s.defaultPolicy
	}

	validRuns, quarantineReasons := s.partitionRuns(runs)
	if len(validRuns) == 0 {
		return nil, quarantineReasons, errors.New("no valid runs available to synthesize scorecard")
	}

	grouped := groupRunsByModel(validRuns)
	leaderboard := make([]ModelScorecard, 0, len(grouped))

	for modelID, mRuns := range grouped {
		sc := computeModelScorecard(modelID, mRuns, policy)
		leaderboard = append(leaderboard, sc)
	}

	sortLeaderboard(leaderboard)
	for i := range leaderboard {
		leaderboard[i].Rank = i + 1
	}

	card := &BenchmarkScorecard{
		SchemaVersion:        BenchmarkScorecardSchemaV1,
		ScorecardID:          scorecardID,
		CreatedAt:            time.Now().UTC().Format(time.RFC3339),
		RankingPolicy:        policy,
		Leaderboard:          leaderboard,
		QuarantinedRunsCount: len(quarantineReasons),
	}
	return card, quarantineReasons, nil
}

func (s *ScorecardSynthesizer) partitionRuns(runs []BenchmarkRun) ([]BenchmarkRun, []string) {
	valid := make([]BenchmarkRun, 0, len(runs))
	var reasons []string
	seen := make(map[string]struct{})

	for _, r := range runs {
		if !IsValidRun(r) {
			reasons = append(reasons, fmt.Sprintf("malformed run %s: invalid metric bounds", r.RunID))
			continue
		}
		if _, exists := seen[r.RunID]; exists {
			reasons = append(reasons, fmt.Sprintf("duplicate run %s quarantined", r.RunID))
			continue
		}
		seen[r.RunID] = struct{}{}
		valid = append(valid, r)
	}
	return valid, reasons
}

func groupRunsByModel(runs []BenchmarkRun) map[string][]BenchmarkRun {
	grouped := make(map[string][]BenchmarkRun)
	for _, r := range runs {
		grouped[r.ModelID] = append(grouped[r.ModelID], r)
	}
	return grouped
}

func computeModelScorecard(modelID string, runs []BenchmarkRun, policy string) ModelScorecard {
	passedCount := 0
	latencies := make([]float64, 0, len(runs))
	totalTokens := 0
	totalVariance := 0.0

	for _, r := range runs {
		if r.Passed {
			passedCount++
		}
		latencies = append(latencies, r.DurationMs)
		totalTokens += r.TokensUsed
		totalVariance += r.Variance
	}

	sort.Float64s(latencies)
	n := float64(len(runs))
	passRate := float64(passedCount) / n
	p50 := percentile(latencies, 50)
	p99 := percentile(latencies, 99)
	avgVariance := totalVariance / n
	stability := 1.0 / (1.0 + avgVariance)
	costPerK := (float64(totalTokens) / n) / 1000.0 * 0.002

	composite := calculateComposite(passRate, p50, stability, policy)

	return ModelScorecard{
		ModelID:              modelID,
		CompositeScore:       math.Round(composite*100) / 100,
		ScenariosEvaluated:   len(runs),
		PassRate:             math.Round(passRate*1000) / 1000,
		LatencyP50Ms:         math.Round(p50*10) / 10,
		LatencyP99Ms:         math.Round(p99*10) / 10,
		TokenCostPerThousand: math.Round(costPerK*10000) / 10000,
		StabilityIndex:       math.Round(stability*1000) / 1000,
	}
}

func calculateComposite(passRate, p50, stability float64, policy string) float64 {
	latencyFactor := math.Max(0, 100.0-(p50/20.0))
	qualityFactor := passRate * 100.0
	stabilityFactor := stability * 100.0

	switch policy {
	case PolicyLatencyCritical:
		return (0.3 * qualityFactor) + (0.5 * latencyFactor) + (0.2 * stabilityFactor)
	case PolicyQualityFirst:
		return (0.7 * qualityFactor) + (0.1 * latencyFactor) + (0.2 * stabilityFactor)
	default:
		return (0.5 * qualityFactor) + (0.3 * latencyFactor) + (0.2 * stabilityFactor)
	}
}

func percentile(sorted []float64, pct float64) float64 {
	if len(sorted) == 0 {
		return 0.0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := (pct / 100.0) * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))
	weight := rank - float64(lower)
	return sorted[lower]*(1.0-weight) + sorted[upper]*weight
}

func sortLeaderboard(lb []ModelScorecard) {
	sort.Slice(lb, func(i, j int) bool {
		if lb[i].CompositeScore != lb[j].CompositeScore {
			return lb[i].CompositeScore > lb[j].CompositeScore
		}
		return lb[i].ModelID < lb[j].ModelID
	})
}
