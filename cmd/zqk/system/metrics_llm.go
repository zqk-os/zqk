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

	return renderReport(report, format)
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

func renderReport(report *llmtranscript.EvaluationReport, format string) error {
	switch format {
	case "json":
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	case "yaml":
		data, err := yaml.Marshal(report)
		if err != nil {
			return err
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

	printRankingTable(report.ModelRankings)
	printModelPatternsAndTips(report.ModelRankings)
	fmt.Println()
}

func printRankingTable(rankings []llmtranscript.ModelMetrics) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
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

func printModelPatternsAndTips(rankings []llmtranscript.ModelMetrics) {
	for _, m := range rankings {
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
}

func truncateSample(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
