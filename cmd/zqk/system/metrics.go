package system

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// NewMetricsCmd creates a command to view command metrics
func NewMetricsCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"View command execution metrics and statistics",
		"View command execution metrics including timing, frequency, success rates, and timeout statistics.",
		"",
		"This command provides insights into:",
		"  - Command execution times (baseline, fastest, slowest, average)",
		"  - Invocation frequency",
		"  - Success and failure rates",
		"  - Timeout occurrences",
		"  - Error rates",
	).
		AddExample("List all command metrics", "%s system metrics").
		AddExample("View metrics for a specific command", "%s system metrics --command \"%s system check\"").
		AddExample("Output as JSON", "%s system metrics --format json").
		AddExample("Show only commands with failures", "%s system metrics --filter failures").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemMetricsCommandBuilder(), &cobra.Command{
		Use:  "metrics",
		Args: cobra.NoArgs,
		RunE: runMetrics,
	})

	cmd.Flags().String("command", "", "Filter by specific command pattern")
	cmd.Flags().String("filter", "", "Filter by type (failures, timeouts, slow)")
	cmd.Flags().Int("limit", 0, "Limit number of results (0 = all)")
	cmd.Flags().Bool("summary", false, "Generate detailed analysis summary report")
	cmd.Flags().Bool("all-time", false, "Aggregate across current and all day-rolled historical chunks")
	cmd.Flags().Duration("window", 0, "Window duration to look back across historical chunks (e.g. 24h, 72h, 168h)")
	cmd.Flags().String("day", "", "View metrics for a specific date (YYYY-MM-DD or YYYYMMDD)")

	// Apply help builder to command after flags are declared
	helpBuilder.ApplyToCommand(cmd)

	// Add subcommands
	cmd.AddCommand(NewFileLockMetricsCmd())
	cmd.AddCommand(NewSchedulerHealthMetricsCmd())
	cmd.AddCommand(NewMetricsFeedCmd())

	cli.AddCommonFlags(cmd)
	return cmd
}

// metricsOptions contains all options for metrics command execution
type metricsOptions struct {
	*cli.Context
	CommandFilter string
	TypeFilter    string
	Limit         int
	Summary       bool
	AllTime       bool
	Window        time.Duration
	Day           string
}

func runMetrics(cmd *cobra.Command, args []string) error {
	projectRoot, err := resolveCommandProjectRoot(cmd)
	if err != nil {
		return err
	}

	// Extract command-specific flags (these aren't in context yet, but could be added)
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	commandFilter, _ := cmd.Flags().GetString("command")
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	typeFilter, _ := cmd.Flags().GetString("filter")
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	limit, _ := cmd.Flags().GetInt("limit")
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	summary, _ := cmd.Flags().GetBool("summary")
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	allTime, _ := cmd.Flags().GetBool("all-time")
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	window, _ := cmd.Flags().GetDuration("window")
	//nolint:errcheck // Flag get - error indicates flag not set, default used
	day, _ := cmd.Flags().GetString("day")

	// Create options struct with context and command-specific flags
	opts := &metricsOptions{
		Context:       cli.GetContext(cmd),
		CommandFilter: commandFilter,
		TypeFilter:    typeFilter,
		Limit:         limit,
		Summary:       summary,
		AllTime:       allTime,
		Window:        window,
		Day:           day,
	}

	store, err := openCommandMetricsStore(projectRoot)
	if err != nil {
		return err
	}

	var allMetrics map[string]*clipkg.CommandMetrics
	if opts.Day != "" {
		allMetrics, err = store.GetMetricsForDate(opts.Day)
	} else if opts.AllTime {
		allMetrics, err = store.GetAllTimeMetrics()
	} else if opts.Window > 0 {
		allMetrics, err = store.GetMetricsSince(opts.Window)
	} else {
		allMetrics, err = store.GetAllMetrics()
	}
	if err != nil {
		return errfmt.Newf("failed to get metrics").Wrap(err)
	}

	// Apply filters
	filtered := filterMetrics(allMetrics, opts.CommandFilter, opts.TypeFilter)

	// Sort by invocation count (most used first)
	sorted := sortMetrics(filtered)

	// Apply limit
	if opts.Limit > 0 && opts.Limit < len(sorted) {
		sorted = sorted[:opts.Limit]
	}

	if opts.Summary {
		return outputMetricsSummary(cmd, store, sorted, opts)
	}

	// Output using format from context
	return outputMetrics(cmd, sorted, opts)
}

func filterMetrics(metrics map[string]*clipkg.CommandMetrics, cmdFilter, typeFilter string) []*clipkg.CommandMetrics {
	result := make([]*clipkg.CommandMetrics, 0)

	for _, m := range metrics {
		// Command filter
		if cmdFilter != emptyValue && !contains(m.NormalizedCmd, cmdFilter) {
			continue
		}

		// Type filter
		switch typeFilter {
		case "failures":
			if m.FailureCount == 0 {
				continue
			}
		case "timeouts":
			if m.TimeoutCount == 0 {
				continue
			}
		case "slow":
			// Commands that are slower than 10 seconds on average
			if m.AvgDuration < 10*time.Second {
				continue
			}
		}

		result = append(result, m)
	}

	return result
}

func sortMetrics(metrics []*clipkg.CommandMetrics) []*clipkg.CommandMetrics {
	sorted := make([]*clipkg.CommandMetrics, len(metrics))
	copy(sorted, metrics)

	sort.Slice(sorted, func(i, j int) bool {
		// Sort by invocation count (descending)
		return sorted[i].InvocationCount > sorted[j].InvocationCount
	})

	return sorted
}

func outputMetricsTable(cmd *cobra.Command, metrics []*clipkg.CommandMetrics) error {
	if len(metrics) == 0 {
		return cli.WriteOutput(cmd, []byte("No metrics found.\n"))
	}

	var buf bytes.Buffer
	// One-line summary for quick observability
	totalInv, totalFail, totalTimeout := 0, 0, 0
	for _, m := range metrics {
		totalInv += m.InvocationCount
		totalFail += m.FailureCount
		totalTimeout += m.TimeoutCount
	}
	errPct := 0.0
	if totalInv > 0 {
		errPct = 100 * float64(totalFail) / float64(totalInv)
	}
	fmt.Fprintf(&buf, "Commands: %d | Invocations: %d | Failures: %d (%.1f%%) | Timeouts: %d\n\n",
		len(metrics), totalInv, totalFail, errPct, totalTimeout)
	buf.WriteString("Command Metrics Summary\n")
	buf.WriteString("=======================\n\n")
	fmt.Fprintf(&buf, "%-50s %8s %8s %8s %10s %10s %8s %8s\n",
		"Command", "Count", "Success", "Failed", "Avg Time", "Baseline", "Err %", "Timeout")
	buf.WriteString("------------------------------------------------------------------------------------------------------------------------\n")

	for _, m := range metrics {
		fmt.Fprintf(&buf, "%-50s %8d %8d %8d %10s %10s %7.1f%% %8d\n",
			truncate(m.NormalizedCmd, 50),
			m.InvocationCount,
			m.SuccessCount,
			m.FailureCount,
			formatDuration(m.AvgDuration),
			formatDuration(m.BaselineDuration),
			m.ErrorRate,
			m.TimeoutCount)
	}

	fmt.Fprintf(&buf, "\nTotal commands: %d\n", len(metrics))
	return cli.WriteOutput(cmd, buf.Bytes())
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%.1fm", d.Minutes())
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func outputMetrics(cmd *cobra.Command, metrics []*clipkg.CommandMetrics, _ *metricsOptions) error {
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, metrics)
	default:
		return outputMetricsTable(cmd, metrics)
	}
}

func outputMetricsSummary(cmd *cobra.Command, store clipkg.MetricsStore, _ []*clipkg.CommandMetrics, _ *metricsOptions) error {
	analyzer := clipkg.NewMetricsAnalyzer(store)

	analysis, err := analyzer.Analyze()
	if err != nil {
		return errfmt.Newf("failed to analyze metrics").Wrap(err)
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, analyzer.AnalysisReportPayload(analysis))
	default:
		report := analyzer.GenerateReport(analysis)
		return cli.WriteOutput(cmd, []byte(report))
	}
}
