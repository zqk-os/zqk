package system

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/telemetry/llmtranscript"
	"gopkg.in/yaml.v3"
)

// NewLLMMetricsCmd creates a subcommand to analyze LLM transcript telemetry,
// rank model correctness, and generate prompt honing feedback streams.
func NewLLMMetricsCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Scrutinize LLM interaction transcripts, rank model correctness, and generate prompt honing guidance",
		"Analyze streaming interaction transcript logs across conversation sessions.",
		"Computes empirical tool call accuracy, self-correction recovery efficiency,",
		"token economics, composite model rankings, and diagnostic prompt tuning recommendations.",
	).
		AddExample("Evaluate all sessions in default brain directory", "%s system metrics llm").
		AddExample("Evaluate a specific conversation transcript", "%s system metrics llm --file path/to/transcript.jsonl").
		AddExample("Output evaluation report as JSON", "%s system metrics llm --format json")

	cmd := &cobra.Command{
		Use:   "llm",
		Short: "Scrutinize LLM interaction transcripts and rank model correctness",
		RunE:  runLLMMetrics,
	}

	cmd.Flags().String("dir", "", "Directory containing conversation transcript logs (default: ~/.gemini/antigravity-cli/brain)")
	cmd.Flags().String("file", "", "Path to a specific transcript.jsonl file")
	cmd.Flags().String("model", "", "Model label override (default: auto-detected from environment/settings)")
	cmd.Flags().StringP("format", "f", "table", "Output format (table, json, yaml)")

	helpBuilder.ApplyToCommand(cmd)
	return cmd
}

func runLLMMetrics(cmd *cobra.Command, args []string) error {
	filePath, _ := cmd.Flags().GetString("file")
	dirPath, _ := cmd.Flags().GetString("dir")
	modelOverride, _ := cmd.Flags().GetString("model")
	format, _ := cmd.Flags().GetString("format")

	homeDir, _ := os.UserHomeDir()
	appDataDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")

	if modelOverride == "" {
		modelOverride = llmtranscript.DetectModelFromSettings(appDataDir)
		if modelOverride == "" {
			modelOverride = "Gemini 3.8 Flash (Medium)"
		}
	}

	var report *llmtranscript.EvaluationReport
	var err error

	if filePath != "" {
		steps, pErr := llmtranscript.ParseTranscriptFile(filePath)
		if pErr != nil {
			return fmt.Errorf("failed to parse transcript file %s: %w", filePath, pErr)
		}
		turns := llmtranscript.ExtractTurns(steps, modelOverride)
		rankings := llmtranscript.EvaluateTurns(turns)
		report = &llmtranscript.EvaluationReport{
			ConversationsAnalyzed: 1,
			TotalSteps:            len(steps),
			TotalTurns:            len(turns),
			ModelRankings:         rankings,
		}
	} else {
		if dirPath == "" {
			dirPath = filepath.Join(appDataDir, "brain")
		}
		report, err = llmtranscript.AnalyzeDirectory(dirPath, modelOverride)
		if err != nil {
			return fmt.Errorf("failed to analyze directory %s: %w", dirPath, err)
		}
	}

	switch format {
	case "json":
		data, mErr := json.MarshalIndent(report, "", "  ")
		if mErr != nil {
			return mErr
		}
		fmt.Println(string(data))
		return nil
	case "yaml":
		data, mErr := yaml.Marshal(report)
		if mErr != nil {
			return mErr
		}
		fmt.Println(string(data))
		return nil
	default:
		printTableReport(report)
		return nil
	}
}

func printTableReport(report *llmtranscript.EvaluationReport) {
	fmt.Printf("\n=== LLM Interaction Telemetry & Model Correctness Report ===\n")
	fmt.Printf("Conversations Analyzed: %d | Total Steps: %d | Total Evaluated Turns: %d\n\n",
		report.ConversationsAnalyzed, report.TotalSteps, report.TotalTurns)

	if len(report.ModelRankings) == 0 {
		fmt.Println("No model interaction turns detected.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "RANK\tMODEL\tSCORE\tTOOL ACCURACY\tRECOVERY RATE\tTOOL CALLS\tTOKENS/SUCCESS")
	fmt.Fprintln(w, "----\t-----\t-----\t-------------\t-------------\t----------\t--------------")

	for _, m := range report.ModelRankings {
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
	w.Flush()

	// Failure breakdown & prompt honing recommendations
	for _, m := range report.ModelRankings {
		if len(m.TopFailurePatterns) > 0 {
			fmt.Printf("\n--- Empirical Failure Archetypes: %s ---\n", m.Model)
			for _, p := range m.TopFailurePatterns {
				fmt.Printf("  • [%s] %s (count: %d)\n", p.Tool, p.Pattern, p.Count)
				if p.Sample != "" {
					fmt.Printf("    Sample: %s\n", truncateSample(p.Sample, 80))
				}
			}
		}

		if len(m.PromptHoningTips) > 0 {
			fmt.Printf("\n--- Actionable Prompt Honing Guidance: %s ---\n", m.Model)
			for _, tip := range m.PromptHoningTips {
				fmt.Printf("  → %s\n", tip)
			}
		}
	}
	fmt.Println()
}

func truncateSample(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
