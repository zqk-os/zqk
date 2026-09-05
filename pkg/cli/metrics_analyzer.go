package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqktime"
	"gopkg.in/yaml.v3"
)

// MetricsAnalyzer analyzes command metrics to identify improvements
type MetricsAnalyzer struct {
	store MetricsStore
}

// NewMetricsAnalyzer creates a new metrics analyzer
func NewMetricsAnalyzer(store MetricsStore) *MetricsAnalyzer {
	return &MetricsAnalyzer{
		store: store,
	}
}

// AnalysisResult contains analysis findings
type AnalysisResult struct {
	HighFailureRateCommands []CommandIssue   `json:"high_failure_rate_commands"`
	FrequentTimeouts        []CommandIssue   `json:"frequent_timeouts"`
	SlowCommands            []CommandIssue   `json:"slow_commands"`
	ImprovementSuggestions  []Suggestion     `json:"improvement_suggestions"`
	ChurnIndicators         []ChurnIndicator `json:"churn_indicators"`
}

// CommandIssue identifies a problematic command
type CommandIssue struct {
	Command         string        `json:"command"`
	NormalizedCmd   string        `json:"normalized_cmd"`
	Issue           string        `json:"issue"`
	Severity        string        `json:"severity"` // "high", "medium", "low"
	Metric          string        `json:"metric"`
	Value           any           `json:"value"`
	Baseline        time.Duration `json:"baseline"`
	InvocationCount int           `json:"invocation_count"`
}

// Suggestion provides improvement recommendations
type Suggestion struct {
	Type        string `json:"type"` // "timeout", "error_handling", "optimization", "documentation"
	Command     string `json:"command"`
	Description string `json:"description"`
	Priority    string `json:"priority"` // "high", "medium", "low"
	Impact      string `json:"impact"`   // Estimated impact description
}

// ChurnIndicator identifies patterns that suggest user confusion or inefficiency
type ChurnIndicator struct {
	Pattern     string   `json:"pattern"`
	Description string   `json:"description"`
	Frequency   int      `json:"frequency"`
	Commands    []string `json:"commands"`
	Severity    string   `json:"severity"`
}

// Analyze performs comprehensive analysis of command metrics
func (a *MetricsAnalyzer) Analyze() (*AnalysisResult, error) {
	if a.store == nil {
		return nil, errfmt.Errorf("metrics store not set")
	}

	allMetrics, err := a.store.GetAllMetrics()
	if err != nil {
		return nil, errfmt.Newf("failed to get metrics").Wrap(err)
	}

	result := &AnalysisResult{
		HighFailureRateCommands: make([]CommandIssue, 0),
		FrequentTimeouts:        make([]CommandIssue, 0),
		SlowCommands:            make([]CommandIssue, 0),
		ImprovementSuggestions:  make([]Suggestion, 0),
		ChurnIndicators:         make([]ChurnIndicator, 0),
	}

	// Analyze each command
	for _, metrics := range allMetrics {
		// Skip commands with insufficient invocation count
		if metrics.InvocationCount < 3 {
			continue
		}

		// High failure rate
		if metrics.ErrorRate > 10.0 {
			result.HighFailureRateCommands = append(result.HighFailureRateCommands, a.createCommandIssue(
				metrics, "High error rate", "error_rate", metrics.ErrorRate, 10.0, 25.0, 50.0))
		}

		// Frequent timeouts
		if metrics.TimeoutRate > 5.0 {
			result.FrequentTimeouts = append(result.FrequentTimeouts, a.createCommandIssue(
				metrics, "Frequent timeouts", "timeout_rate", metrics.TimeoutRate, 5.0, 15.0, 30.0))
		}

		// Slow commands (average > 30 seconds)
		if metrics.AvgDuration > 30*time.Second {
			result.SlowCommands = append(result.SlowCommands, a.createCommandIssue(
				metrics, "Slow execution", "avg_duration", metrics.AvgDuration.Seconds(), 30.0, 60.0, 120.0))
		}
	}

	// Generate suggestions
	result.ImprovementSuggestions = a.generateSuggestions(result)

	// Detect churn indicators
	result.ChurnIndicators = a.detectChurnIndicators(allMetrics)

	return result, nil
}

// createCommandIssue creates a CommandIssue from metrics with consistent structure
func (a *MetricsAnalyzer) createCommandIssue(metrics *CommandMetrics, issue, metric string, value, low, medium, high float64) CommandIssue {
	var valueInterface any = value
	if metric == "avg_duration" {
		valueInterface = time.Duration(value * float64(time.Second)).String()
	}

	return CommandIssue{
		Command:         metrics.Command,
		NormalizedCmd:   metrics.NormalizedCmd,
		Issue:           issue,
		Severity:        a.calculateSeverity(value, low, medium, high),
		Metric:          metric,
		Value:           valueInterface,
		Baseline:        metrics.BaselineDuration,
		InvocationCount: metrics.InvocationCount,
	}
}

func (a *MetricsAnalyzer) calculateSeverity(value, low, medium, high float64) string {
	if value >= high {
		return "high"
	}
	if value >= medium {
		return "medium"
	}
	if value >= low {
		return "low"
	}
	return "low"
}

func (a *MetricsAnalyzer) generateSuggestions(result *AnalysisResult) []Suggestion {
	suggestions := make([]Suggestion, 0)

	// Suggestions for high failure rate commands
	for _, issue := range result.HighFailureRateCommands {
		suggestions = append(suggestions, Suggestion{
			Type:        "error_handling",
			Command:     issue.NormalizedCmd,
			Description: fmt.Sprintf("Command has %.1f%% error rate. Review error handling and user guidance.", issue.Value),
			Priority:    issue.Severity,
			Impact:      "Reduced user frustration and improved reliability",
		})
	}

	// Suggestions for frequent timeouts
	for _, issue := range result.FrequentTimeouts {
		suggestions = append(suggestions, Suggestion{
			Type:        "timeout",
			Command:     issue.NormalizedCmd,
			Description: fmt.Sprintf("Command times out %.1f%% of the time. Consider optimizing or increasing timeout.", issue.Value),
			Priority:    issue.Severity,
			Impact:      "Improved command reliability and user experience",
		})
	}

	// Suggestions for slow commands
	for _, issue := range result.SlowCommands {
		suggestions = append(suggestions, Suggestion{
			Type:        "optimization",
			Command:     issue.NormalizedCmd,
			Description: fmt.Sprintf("Command averages %s execution time. Consider optimization or progress indicators.", issue.Value),
			Priority:    issue.Severity,
			Impact:      "Faster command execution and better user experience",
		})
	}

	return suggestions
}

func (a *MetricsAnalyzer) detectChurnIndicators(metrics map[string]*CommandMetrics) []ChurnIndicator {
	indicators := make([]ChurnIndicator, 0)

	// Pattern: Multiple similar commands with failures (suggests user confusion)
	similarCommands := make(map[string][]*CommandMetrics)
	for _, m := range metrics {
		// Group by base command (first word)
		base := getBaseCommand(m.NormalizedCmd)
		similarCommands[base] = append(similarCommands[base], m)
	}

	for base, cmds := range similarCommands {
		if len(cmds) > 3 {
			// Multiple variations suggest confusion
			failureCount := 0
			for _, cmd := range cmds {
				failureCount += cmd.FailureCount
			}
			if failureCount > 5 {
				indicators = append(indicators, ChurnIndicator{
					Pattern:     "multiple_variations",
					Description: fmt.Sprintf("Multiple variations of '%s' command with failures suggest user confusion", base),
					Frequency:   len(cmds),
					Commands:    getCommandNames(cmds),
					Severity:    "medium",
				})
			}
		}
	}

	// Pattern: Commands with high retry rate (same command run multiple times quickly)
	// This would require timestamp data which we don't have in current metrics
	// Could be enhanced with per-invocation timestamps

	return indicators
}

func getBaseCommand(cmd string) string {
	// Extract first word as base command
	for i, r := range cmd {
		if r == ' ' {
			return cmd[:i]
		}
	}
	return cmd
}

func getCommandNames(cmds []*CommandMetrics) []string {
	names := make([]string, len(cmds))
	for i, cmd := range cmds {
		names[i] = cmd.NormalizedCmd
	}
	return names
}

// GenerateReport generates a formatted report from analysis
func (a *MetricsAnalyzer) GenerateReport(analysis *AnalysisResult) string {
	report := "# Command Metrics Analysis Report\n\n"
	report += fmt.Sprintf("Generated: %s\n\n", zqktime.NowRFC3339UTC())

	// Summary
	report += "## Summary\n\n"
	report += fmt.Sprintf("- Commands with high failure rates: %d\n", len(analysis.HighFailureRateCommands))
	report += fmt.Sprintf("- Commands with frequent timeouts: %d\n", len(analysis.FrequentTimeouts))
	report += fmt.Sprintf("- Slow commands: %d\n", len(analysis.SlowCommands))
	report += fmt.Sprintf("- Improvement suggestions: %d\n", len(analysis.ImprovementSuggestions))
	report += fmt.Sprintf("- Churn indicators: %d\n\n", len(analysis.ChurnIndicators))

	// High failure rate commands
	if len(analysis.HighFailureRateCommands) > 0 {
		report += "## High Failure Rate Commands\n\n"
		sort.Slice(analysis.HighFailureRateCommands, func(i, j int) bool {
			return analysis.HighFailureRateCommands[i].InvocationCount > analysis.HighFailureRateCommands[j].InvocationCount
		})
		for _, issue := range analysis.HighFailureRateCommands {
			report += fmt.Sprintf("### %s\n", issue.NormalizedCmd)
			report += fmt.Sprintf("- **Severity**: %s\n", issue.Severity)
			report += fmt.Sprintf("- **Error Rate**: %.1f%%\n", issue.Value)
			report += fmt.Sprintf("- **Invocation Count**: %d\n", issue.InvocationCount)
			report += fmt.Sprintf("- **Baseline Duration**: %s\n\n", issue.Baseline)
		}
	}

	// Frequent timeouts
	if len(analysis.FrequentTimeouts) > 0 {
		report += "## Frequent Timeouts\n\n"
		for _, issue := range analysis.FrequentTimeouts {
			report += fmt.Sprintf("### %s\n", issue.NormalizedCmd)
			report += fmt.Sprintf("- **Severity**: %s\n", issue.Severity)
			report += fmt.Sprintf("- **Timeout Rate**: %.1f%%\n", issue.Value)
			report += fmt.Sprintf("- **Invocation Count**: %d\n\n", issue.InvocationCount)
		}
	}

	// Slow commands
	if len(analysis.SlowCommands) > 0 {
		report += "## Slow Commands\n\n"
		for _, issue := range analysis.SlowCommands {
			report += fmt.Sprintf("### %s\n", issue.NormalizedCmd)
			report += fmt.Sprintf("- **Severity**: %s\n", issue.Severity)
			report += fmt.Sprintf("- **Average Duration**: %s\n", issue.Value)
			report += fmt.Sprintf("- **Invocation Count**: %d\n\n", issue.InvocationCount)
		}
	}

	// Improvement suggestions
	if len(analysis.ImprovementSuggestions) > 0 {
		report += "## Improvement Suggestions\n\n"
		sort.Slice(analysis.ImprovementSuggestions, func(i, j int) bool {
			priorityOrder := map[string]int{"high": 3, "medium": 2, "low": 1}
			return priorityOrder[analysis.ImprovementSuggestions[i].Priority] > priorityOrder[analysis.ImprovementSuggestions[j].Priority]
		})
		for _, suggestion := range analysis.ImprovementSuggestions {
			report += fmt.Sprintf("### %s [%s]\n", suggestion.Command, suggestion.Priority)
			report += fmt.Sprintf("- **Type**: %s\n", suggestion.Type)
			report += fmt.Sprintf("- **Description**: %s\n", suggestion.Description)
			report += fmt.Sprintf("- **Impact**: %s\n\n", suggestion.Impact)
		}
	}

	// Churn indicators
	if len(analysis.ChurnIndicators) > 0 {
		report += "## Churn Indicators\n\n"
		report += "These patterns suggest user confusion or inefficiency:\n\n"
		for _, indicator := range analysis.ChurnIndicators {
			report += fmt.Sprintf("### %s [%s]\n", indicator.Pattern, indicator.Severity)
			report += fmt.Sprintf("- **Description**: %s\n", indicator.Description)
			report += fmt.Sprintf("- **Frequency**: %d\n", indicator.Frequency)
			if len(indicator.Commands) > 0 {
				report += fmt.Sprintf("- **Commands**: %v\n", indicator.Commands)
			}
			report += "\n"
		}
	}

	return report
}

// AnalysisReportPayload is the structured document for JSON/YAML/JSONL metrics summary output.
func (a *MetricsAnalyzer) AnalysisReportPayload(analysis *AnalysisResult) map[string]any {
	if analysis == nil {
		return map[string]any{
			"generated_at": zqktime.NowRFC3339UTC(),
		}
	}
	return map[string]any{
		"generated_at":               zqktime.NowRFC3339UTC(),
		"high_failure_rate_commands": analysis.HighFailureRateCommands,
		"frequent_timeouts":          analysis.FrequentTimeouts,
		"slow_commands":              analysis.SlowCommands,
		"improvement_suggestions":    analysis.ImprovementSuggestions,
		"churn_indicators":           analysis.ChurnIndicators,
		objects.FieldKeySummary: map[string]int{
			"high_failure_rate_commands_count": len(analysis.HighFailureRateCommands),
			"frequent_timeouts_count":          len(analysis.FrequentTimeouts),
			"slow_commands_count":              len(analysis.SlowCommands),
			"improvement_suggestions_count":    len(analysis.ImprovementSuggestions),
			"churn_indicators_count":           len(analysis.ChurnIndicators),
		},
	}
}

// GenerateReportJSON generates a JSON-formatted report from analysis
func (a *MetricsAnalyzer) GenerateReportJSON(analysis *AnalysisResult) ([]byte, error) {
	return json.MarshalIndent(a.AnalysisReportPayload(analysis), "", "  ")
}

// GenerateReportYAML generates a YAML-formatted report from analysis
func (a *MetricsAnalyzer) GenerateReportYAML(analysis *AnalysisResult) ([]byte, error) {
	return yaml.Marshal(a.AnalysisReportPayload(analysis))
}
