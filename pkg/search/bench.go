package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	benchDefaultMaxTokens       = 2000
	benchBaselineMaxMatches     = 10000
	benchBytesPerToken          = 4
	benchInputCostPerMillionUSD = 3.0
	benchTurnsBasis             = 100
)

// BenchReport captures a side-by-side self-benchmark comparing unbudgeted raw search
// against token-budgeted indexed zgrep on the user's own repository (GH-1237).
type BenchReport struct {
	Query                           string  `json:"query" yaml:"query"`
	Repository                      string  `json:"repository" yaml:"repository"`
	BaselineLatencyMs               float64 `json:"baseline_latency_ms" yaml:"baseline_latency_ms"`
	BaselineBytes                   int     `json:"baseline_bytes" yaml:"baseline_bytes"`
	BaselineEstimatedTokens         int     `json:"baseline_estimated_tokens" yaml:"baseline_estimated_tokens"`
	BaselineMatches                 int     `json:"baseline_matches" yaml:"baseline_matches"`
	ZgrepLatencyMs                  float64 `json:"zgrep_latency_ms" yaml:"zgrep_latency_ms"`
	ZgrepBytes                      int     `json:"zgrep_bytes" yaml:"zgrep_bytes"`
	ZgrepEstimatedTokens            int     `json:"zgrep_estimated_tokens" yaml:"zgrep_estimated_tokens"`
	ZgrepMatches                    int     `json:"zgrep_matches" yaml:"zgrep_matches"`
	TokenReductionPct               float64 `json:"token_reduction_pct" yaml:"token_reduction_pct"`
	EstimatedCostSavedPer100TurnsUS float64 `json:"estimated_cost_saved_per_100_turns_usd" yaml:"estimated_cost_saved_per_100_turns_usd"`
}

// RunSelfBench executes an in-process side-by-side comparison of unbudgeted raw search
// vs. token-budgeted indexed zgrep on e.projectRoot.
func (e *Engine) RunSelfBench(ctx context.Context, opts SearchOptions) (BenchReport, error) {
	report := BenchReport{
		Query:      opts.Query,
		Repository: e.projectRoot,
	}

	// Arm 1: Unbudgeted raw text baseline (simulating grep -rn / rg without token capping).
	baselineOpts := opts
	baselineOpts.UseIndex = false
	baselineOpts.MaxTokens = 0
	baselineOpts.MaxMatches = benchBaselineMaxMatches
	baseStart := time.Now()
	baseRes, err := e.Search(ctx, baselineOpts)
	if err != nil {
		return report, fmt.Errorf("baseline search failed: %w", err)
	}
	report.BaselineLatencyMs = float64(time.Since(baseStart).Microseconds()) / 1000.0
	report.BaselineMatches = len(baseRes.Matches)
	var baseBuf bytes.Buffer
	for _, m := range baseRes.Matches {
		fmt.Fprintf(&baseBuf, "%s:%d:%s\n", m.File, m.Line, m.LineContent)
	}
	report.BaselineBytes = baseBuf.Len()
	report.BaselineEstimatedTokens = estimateTokensFromBytes(report.BaselineBytes)

	// Arm 2: Token-budgeted indexed zgrep JSON payload.
	zgrepOpts := opts
	zgrepOpts.UseIndex = true
	if zgrepOpts.MaxTokens <= 0 {
		zgrepOpts.MaxTokens = benchDefaultMaxTokens
	}
	// Warm the index if not already warm so warm query latency reflects steady-state agent turns.
	if _, idxErr := e.LoadOrBuildIndex(zgrepOpts); idxErr != nil {
		return report, fmt.Errorf("failed warming trigram index for benchmark: %w", idxErr)
	}
	zgrepStart := time.Now()
	zgrepRes, err := e.Search(ctx, zgrepOpts)
	if err != nil {
		return report, fmt.Errorf("zgrep search failed: %w", err)
	}
	report.ZgrepLatencyMs = float64(time.Since(zgrepStart).Microseconds()) / 1000.0
	report.ZgrepMatches = len(zgrepRes.Matches)
	zgrepPayload, err := json.Marshal(zgrepRes)
	if err != nil {
		return report, fmt.Errorf("failed encoding zgrep benchmark payload: %w", err)
	}
	report.ZgrepBytes = len(zgrepPayload)
	report.ZgrepEstimatedTokens = estimateTokensFromBytes(report.ZgrepBytes)

	if report.BaselineEstimatedTokens > 0 && report.BaselineEstimatedTokens > report.ZgrepEstimatedTokens {
		saved := report.BaselineEstimatedTokens - report.ZgrepEstimatedTokens
		report.TokenReductionPct = float64(saved) * 100.0 / float64(report.BaselineEstimatedTokens)
		report.EstimatedCostSavedPer100TurnsUS = (float64(saved) * benchTurnsBasis / 1_000_000.0) * benchInputCostPerMillionUSD
	}
	return report, nil
}

func estimateTokensFromBytes(n int) int {
	if n <= 0 {
		return 0
	}
	return (n + benchBytesPerToken - 1) / benchBytesPerToken
}

// FormatBenchTable renders a crisp side-by-side human-readable self-benchmark table.
func FormatBenchTable(report BenchReport, nextStepHint string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "zgrep Self-Benchmark (%s)\n", report.Repository)
	fmt.Fprintf(&b, "Query: %q\n", report.Query)
	fmt.Fprintf(&b, "----------------------------------------------------------------------\n")
	fmt.Fprintf(&b, "%-26s %18s %18s\n", "Metric", "Raw Baseline", "zgrep (Budgeted)")
	fmt.Fprintf(&b, "----------------------------------------------------------------------\n")
	fmt.Fprintf(&b, "%-26s %15.2f ms %15.2f ms\n", "Wall-Clock Latency", report.BaselineLatencyMs, report.ZgrepLatencyMs)
	fmt.Fprintf(&b, "%-26s %18d %18d\n", "Matches Returned", report.BaselineMatches, report.ZgrepMatches)
	fmt.Fprintf(&b, "%-26s %18d %18d\n", "Payload Bytes", report.BaselineBytes, report.ZgrepBytes)
	fmt.Fprintf(&b, "%-26s %18d %18d\n", "Estimated BPE Tokens", report.BaselineEstimatedTokens, report.ZgrepEstimatedTokens)
	fmt.Fprintf(&b, "----------------------------------------------------------------------\n")
	fmt.Fprintf(&b, "Token Reduction:           %.1f%%\n", report.TokenReductionPct)
	fmt.Fprintf(&b, "Est. Saved / 100 Turns:    $%.2f (at $%.0f/1M input tokens)\n", report.EstimatedCostSavedPer100TurnsUS, benchInputCostPerMillionUSD)
	if strings.TrimSpace(nextStepHint) != "" {
		fmt.Fprintf(&b, "\nNext step: %s\n", nextStepHint)
	}
	return b.String()
}

// FormatBenchMarkdown renders a GitHub-Flavored Markdown self-benchmark table
// suitable for PR comments, RFCs, Slack summaries, and CI job summaries.
func FormatBenchMarkdown(report BenchReport, nextStepHint string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## zgrep Self-Benchmark (`%s`)\n\n", report.Repository)
	fmt.Fprintf(&b, "- **Query:** `%s`\n", report.Query)
	fmt.Fprintf(&b, "- **Token Reduction:** **%.1f%%**\n", report.TokenReductionPct)
	fmt.Fprintf(&b, "- **Est. Saved / 100 Turns:** **$%.2f** (at $%.0f/1M input tokens)\n\n", report.EstimatedCostSavedPer100TurnsUS, benchInputCostPerMillionUSD)
	b.WriteString("| Metric | Raw Baseline | zgrep (Budgeted) |\n")
	b.WriteString("|---|---:|---:|\n")
	fmt.Fprintf(&b, "| Wall-Clock Latency | %.2f ms | %.2f ms |\n", report.BaselineLatencyMs, report.ZgrepLatencyMs)
	fmt.Fprintf(&b, "| Matches Returned | %d | %d |\n", report.BaselineMatches, report.ZgrepMatches)
	fmt.Fprintf(&b, "| Payload Bytes | %d | %d |\n", report.BaselineBytes, report.ZgrepBytes)
	fmt.Fprintf(&b, "| Estimated BPE Tokens | %d | %d |\n", report.BaselineEstimatedTokens, report.ZgrepEstimatedTokens)
	if strings.TrimSpace(nextStepHint) != "" {
		fmt.Fprintf(&b, "\n> Next step: `%s`\n", nextStepHint)
	}
	return b.String()
}
