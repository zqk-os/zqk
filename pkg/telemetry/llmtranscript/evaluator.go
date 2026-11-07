package llmtranscript

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// EvaluateTurns processes interaction turns and calculates empirical model correctness metrics,
// composite rankings, and actionable prompt honing guidance.
func EvaluateTurns(turns []Turn) []ModelMetrics {
	byModel := make(map[string][]Turn)
	for _, turn := range turns {
		model := turn.Model
		if model == "" {
			model = "Default Model"
		}
		byModel[model] = append(byModel[model], turn)
	}

	var results []ModelMetrics

	for model, mTurns := range byModel {
		metrics := ModelMetrics{
			Model:      model,
			TotalTurns: len(mTurns),
		}

		patternCounts := make(map[string]*FailurePattern)

		for _, t := range mTurns {
			metrics.TotalInputTokens += t.InputTokens
			metrics.TotalOutputTokens += t.OutputTokens
			metrics.TotalCacheReadTokens += t.CacheReadTokens

			metrics.TotalToolCalls += len(t.Executions)
			for _, exec := range t.Executions {
				if exec.Success {
					metrics.SuccessfulToolCalls++
				} else {
					metrics.FailedToolCalls++
					categorizeFailure(exec, patternCounts)
				}
			}

			if t.IsSelfCorrection {
				metrics.SelfCorrectionAttempts++
				if t.CorrectedPrevious {
					metrics.SelfCorrectionSuccesses++
				}
			}
		}

		if metrics.TotalToolCalls > 0 {
			metrics.ToolAccuracy = float64(metrics.SuccessfulToolCalls) / float64(metrics.TotalToolCalls)
		} else {
			metrics.ToolAccuracy = 1.0
		}

		if metrics.SelfCorrectionAttempts > 0 {
			metrics.SelfCorrectionRate = float64(metrics.SelfCorrectionSuccesses) / float64(metrics.SelfCorrectionAttempts)
		} else {
			metrics.SelfCorrectionRate = 1.0
		}

		totalTokens := metrics.TotalInputTokens + metrics.TotalOutputTokens
		denom := math.Max(1.0, float64(metrics.SuccessfulToolCalls))
		metrics.TokensPerSuccess = float64(totalTokens) / denom

		// Compute Composite Score:
		// 50% Accuracy, 25% Self-Correction Recovery, 25% Token Efficiency
		accScore := metrics.ToolAccuracy * 50.0
		recScore := metrics.SelfCorrectionRate * 25.0

		// Token economy: benchmark at 25,000 tokens per success (scales between 0 and 25)
		tokenScore := 25.0
		if metrics.TokensPerSuccess > 0 {
			ratio := 15000.0 / metrics.TokensPerSuccess
			if ratio > 1.0 {
				tokenScore = 25.0
			} else {
				tokenScore = math.Max(0.0, ratio*25.0)
			}
		}
		metrics.CompositeScore = math.Round((accScore+recScore+tokenScore)*100) / 100

		// Aggregate top failure patterns
		var patterns []FailurePattern
		for _, p := range patternCounts {
			patterns = append(patterns, *p)
		}
		sort.Slice(patterns, func(i, j int) bool {
			return patterns[i].Count > patterns[j].Count
		})
		if len(patterns) > 5 {
			patterns = patterns[:5]
		}
		metrics.TopFailurePatterns = patterns

		// Synthesize prompt honing recommendations
		metrics.PromptHoningTips = generatePromptHoningTips(patterns)

		results = append(results, metrics)
	}

	// Sort by CompositeScore descending
	sort.Slice(results, func(i, j int) bool {
		if results[i].CompositeScore == results[j].CompositeScore {
			return results[i].ToolAccuracy > results[j].ToolAccuracy
		}
		return results[i].CompositeScore > results[j].CompositeScore
	})

	for i := range results {
		results[i].Rank = i + 1
	}

	return results
}

// categorizeFailure groups a tool failure into standard archetypes for prompt diagnosis.
func categorizeFailure(exec ToolExecution, patterns map[string]*FailurePattern) {
	key := exec.ToolName + ":generic_error"
	patternName := "Runtime or Tool Error"
	rec := "Inspect tool invocation flags, syntax, and inputs against specification."

	errLower := strings.ToLower(exec.ErrorMessage)

	switch {
	case strings.Contains(errLower, "syntax error"):
		key = exec.ToolName + ":syntax_error"
		patternName = "Command Syntax Error"
		rec = "Avoid complex shell quoting or unsupported nested subshells; break down complex scripts."
	case strings.Contains(errLower, "target not found") || strings.Contains(errLower, "target must exactly match"):
		key = exec.ToolName + ":target_not_found"
		patternName = "Target Not Found in Edit"
		rec = "Read the exact target line range immediately prior to edit_file to guarantee character-for-character whitespace parity."
	case strings.Contains(errLower, "no such file") || strings.Contains(errLower, "cannot find"):
		key = exec.ToolName + ":file_not_found"
		patternName = "File or Path Not Found"
		rec = "Verify directory contents and path existence using filesystem exploration before reading or modifying."
	case strings.Contains(errLower, "permission denied"):
		key = exec.ToolName + ":permission_denied"
		patternName = "Permission Denied"
		rec = "Ensure commands and scripts have appropriate execution permissions and access rights."
	case exec.ExitCode != 0:
		key = fmt.Sprintf("%s:exit_%d", exec.ToolName, exec.ExitCode)
		patternName = fmt.Sprintf("Command Non-Zero Exit (%d)", exec.ExitCode)
		rec = "Inspect return codes and stderr output; run sanity checks or add error guards."
	}

	if existing, ok := patterns[key]; ok {
		existing.Count++
		if existing.Sample == "" && exec.ErrorMessage != "" {
			existing.Sample = exec.ErrorMessage
		}
	} else {
		patterns[key] = &FailurePattern{
			Tool:           exec.ToolName,
			Pattern:        patternName,
			Count:          1,
			Sample:         exec.ErrorMessage,
			Recommendation: rec,
		}
	}
}

// generatePromptHoningTips builds prescriptive prompt additions based on empirical failure patterns.
func generatePromptHoningTips(patterns []FailurePattern) []string {
	var tips []string
	seen := make(map[string]bool)

	for _, p := range patterns {
		tip := fmt.Sprintf("[%s] %s: %s", p.Tool, p.Pattern, p.Recommendation)
		if !seen[tip] {
			tips = append(tips, tip)
			seen[tip] = true
		}
	}

	if len(tips) == 0 {
		tips = append(tips, "Execution accuracy is optimal; maintain standard prompt directives.")
	}

	return tips
}

// AnalyzeDirectory scans a root folder (such as ~/.gemini/antigravity-cli/brain/)
// for all transcript.jsonl files, parses them, and returns an EvaluationReport.
func AnalyzeDirectory(rootDir string, defaultModel string) (*EvaluationReport, error) {
	var allSteps []TranscriptStep
	conversationsCount := 0

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if info.Name() == "transcript.jsonl" {
			steps, pErr := ParseTranscriptFile(path)
			if pErr == nil && len(steps) > 0 {
				conversationsCount++
				allSteps = append(allSteps, steps...)
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", rootDir, err)
	}

	turns := ExtractTurns(allSteps, defaultModel)
	rankings := EvaluateTurns(turns)

	return &EvaluationReport{
		GeneratedAt:           time.Now(),
		ConversationsAnalyzed: conversationsCount,
		TotalSteps:            len(allSteps),
		TotalTurns:            len(turns),
		ModelRankings:         rankings,
	}, nil
}
