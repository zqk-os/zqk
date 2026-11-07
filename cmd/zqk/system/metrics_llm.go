package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/telemetry/llmtranscript"
)

// NewLLMMetricsCmd creates a subcommand to analyze LLM transcript telemetry,
// rank model correctness, and generate prompt honing feedback streams.
func NewLLMMetricsCmd() *cobra.Command {
	return clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemMetricsLlmCommandBuilder(), &cobra.Command{
		Use:  "llm",
		Args: cobra.NoArgs,
		RunE: runLLMMetrics,
	})
}

func runLLMMetrics(cmd *cobra.Command, args []string) error {
	filePath, err := cmd.Flags().GetString("file")
	if err != nil {
		return err
	}
	dirPath, err := cmd.Flags().GetString("dir")
	if err != nil {
		return err
	}
	modelOverride, err := cmd.Flags().GetString("model")
	if err != nil {
		return err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}

	modelOverride = resolveModelOverride(modelOverride)
	report, err := buildEvaluationReport(filePath, dirPath, modelOverride)
	if err != nil {
		return err
	}

	return renderReport(cmd, report, format)
}

func resolveModelOverride(override string) string {
	if override != "" {
		return override
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "Gemini 3.8 Flash (Medium)"
	}
	appDataDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	detected := llmtranscript.DetectModelFromSettings(appDataDir)
	if detected != "" {
		return detected
	}
	return "Gemini 3.8 Flash (Medium)"
}

func buildEvaluationReport(filePath, dirPath, model string) (*llmtranscript.EvaluationReport, error) {
	if filePath != "" {
		steps, err := llmtranscript.ParseTranscriptFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse transcript file %s: %w", filePath, err)
		}
		turns := llmtranscript.ExtractTurns(steps, model)
		rankings := llmtranscript.EvaluateTurns(turns)
		return &llmtranscript.EvaluationReport{
			ConversationsAnalyzed: 1,
			TotalSteps:            len(steps),
			TotalTurns:            len(turns),
			ModelRankings:         rankings,
		}, nil
	}

	if dirPath == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			dirPath = filepath.Join(homeDir, ".gemini", "antigravity-cli", "brain")
		}
	}
	return llmtranscript.AnalyzeDirectory(dirPath, model)
}

func renderReport(cmd *cobra.Command, report *llmtranscript.EvaluationReport, format string) error {
	switch format {
	case "json":
		return cli.FormatOutputAs(cmd, cli.FormatJSON, report)
	case "yaml":
		return cli.FormatOutputAs(cmd, cli.FormatYAML, report)
	default:
		return cli.WriteOutput(cmd, []byte(formatTableReport(report)))
	}
}

func formatTableReport(report *llmtranscript.EvaluationReport) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "\n=== LLM Interaction Telemetry & Model Correctness Report ===\n")
	fmt.Fprintf(&buf, "Conversations Analyzed: %d | Total Steps: %d | Total Evaluated Turns: %d\n\n",
		report.ConversationsAnalyzed, report.TotalSteps, report.TotalTurns)

	if len(report.ModelRankings) == 0 {
		buf.WriteString("No model interaction turns detected.\n")
		return buf.String()
	}

	writeRankingTable(&buf, report.ModelRankings)
	writeModelPatternsAndTips(&buf, report.ModelRankings)
	buf.WriteByte('\n')
	return buf.String()
}

func writeRankingTable(buf *strings.Builder, rankings []llmtranscript.ModelMetrics) {
	w := tabwriter.NewWriter(buf, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "RANK\tMODEL\tSCORE\tTOOL ACCURACY\tRECOVERY RATE\tTOOL CALLS\tTOKENS/SUCCESS")
	fmt.Fprintln(w, "----\t-----\t-----\t-------------\t-------------\t----------\t--------------")

	for _, m := range rankings {
		fmt.Fprintf(w, "#%d\t%s\t%.1f/100\t%.1f%%\t%.1f%%\t%d/%d\t%.0f\n",
			m.Rank,
			m.Model,
			m.CompositeScore,
			m.ToolAccuracy*100,
			m.SelfCorrectionRate*100,
			m.SuccessfulToolCalls,
			m.TotalToolCalls,
			m.TokensPerSuccess,
		)
	}
	_ = w.Flush()
}

func writeModelPatternsAndTips(buf *strings.Builder, rankings []llmtranscript.ModelMetrics) {
	for _, m := range rankings {
		if len(m.TopFailurePatterns) > 0 {
			fmt.Fprintf(buf, "\n--- Empirical Failure Archetypes: %s ---\n", m.Model)
			for _, p := range m.TopFailurePatterns {
				fmt.Fprintf(buf, "  • [%s] %s (count: %d)\n", p.Tool, p.Pattern, p.Count)
				if p.Sample != "" {
					fmt.Fprintf(buf, "    Sample: %s\n", truncateSample(p.Sample, 80))
				}
			}
		}

		if len(m.PromptHoningTips) > 0 {
			fmt.Fprintf(buf, "\n--- Actionable Prompt Honing Guidance: %s ---\n", m.Model)
			for _, tip := range m.PromptHoningTips {
				fmt.Fprintf(buf, "  → %s\n", tip)
			}
		}
	}
}

func truncateSample(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
