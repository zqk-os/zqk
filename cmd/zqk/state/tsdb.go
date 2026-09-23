package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

var (
	tsdbCyanBold   = color.New(color.FgCyan, color.Bold).SprintFunc()
	tsdbWhiteBold  = color.New(color.FgWhite, color.Bold).SprintFunc()
	tsdbYellowBold = color.New(color.FgYellow, color.Bold).SprintFunc()
	tsdbGreenBold  = color.New(color.FgGreen, color.Bold).SprintFunc()
	tsdbRedBold    = color.New(color.FgRed, color.Bold).SprintFunc()
	tsdbDim        = color.New(color.Faint).SprintFunc()
)

func newTsdbCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewStateTsdbCommandBuilder()
	cmd.Aliases = []string{"timeseries", "metrics-history"}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			projectRoot := proc.ProjectRoot()
			if projectRoot == "" {
				projectRoot = cli.ResolveProjectRoot(".")
			}

			limit, _ := cmd.Flags().GetInt("limit")
			if limit <= 0 {
				limit = 20
			}
			sinceStr, _ := cmd.Flags().GetString("since")
			jobIDFilter, _ := cmd.Flags().GetString("job-id")
			format, _ := cmd.Flags().GetString("format")

			return RunStateTSDB(cmd, projectRoot, sinceStr, jobIDFilter, limit, format)
		})(cmd, args)
	}
	return cmd
}

// RunStateTSDB queries and outputs kernel TSDB telemetry.
func RunStateTSDB(cmd *cobra.Command, projectRoot string, sinceStr string, jobIDFilter string, limit int, format string) error {
	var sinceDur time.Duration
	if sinceStr != "" {
		d, err := time.ParseDuration(sinceStr)
		if err == nil {
			sinceDur = d
		} else {
			switch strings.ToLower(sinceStr) {
			case "1d", "today", "24h":
				sinceDur = 24 * time.Hour
			case "7d", "1w", "week":
				sinceDur = 7 * 24 * time.Hour
			case "30d", "1m", "month":
				sinceDur = 30 * 24 * time.Hour
			}
		}
	}

	telem := ReadTSDBTelemetry(projectRoot, sinceDur, limit)

	// If job-id filter applied, filter summaries
	if jobIDFilter != "" {
		filtered := make([]TSDBJobSummary, 0)
		for _, js := range telem.JobSummaries {
			if strings.Contains(strings.ToLower(js.JobID), strings.ToLower(jobIDFilter)) {
				filtered = append(filtered, js)
			}
		}
		telem.JobSummaries = filtered

		filteredPts := make([]TSDBPointView, 0)
		for _, pt := range telem.RecentPoints {
			if pt.Tags != nil && strings.Contains(strings.ToLower(pt.Tags["job_id"]), strings.ToLower(jobIDFilter)) {
				filteredPts = append(filteredPts, pt)
			}
		}
		telem.RecentPoints = filteredPts
	}

	if format == "json" {
		return cli.FormatOutputAs(cmd, cli.FormatJSON, telem)
	}

	out := FormatTSDBTelemetryText(telem, sinceStr)
	return cli.WriteOutput(cmd, []byte(out))
}

// FormatTSDBTelemetryText formats TSDB telemetry into a human-readable table.
func FormatTSDBTelemetryText(telem *TSDBTelemetry, sinceStr string) string {
	var buf strings.Builder

	buf.WriteString("\n⚡ ZQK Knowledge Kernel Time-Series Database (TSDB) Telemetry\n")
	buf.WriteString("──────────────────────────────────────────────────────────────────────────────────────────\n")

	windowStr := "All Time"
	if sinceStr != "" {
		windowStr = fmt.Sprintf("Last %s", sinceStr)
	}

	measStr := strings.Join(telem.Measurements, ", ")
	if measStr == "" {
		measStr = "none"
	}

	fmt.Fprintf(&buf, "Storage:      .zqk/scheduler/tsdb (%d data points across %d files, %s)\n",
		telem.TotalPoints, telem.TotalFiles, telem.DiskSizeStr)
	seriesStr := strings.Join(telem.ChunkStats.SeriesNames, ", ")
	if seriesStr != "" {
		seriesStr = " [" + seriesStr + "]"
	}
	fmt.Fprintf(&buf, "Chunk Store:  .zqk/metrics (%d chunks across %d series%s, %s)\n",
		telem.ChunkStats.TotalChunks, len(telem.ChunkStats.SeriesNames), seriesStr, telem.ChunkStats.DiskSizeStr)
	fmt.Fprintf(&buf, "Measurements: %s\n", tsdbCyanBold(measStr))
	fmt.Fprintf(&buf, "Time Window:  %s\n", tsdbYellowBold(windowStr))
	buf.WriteString("──────────────────────────────────────────────────────────────────────────────────────────\n")

	if len(telem.JobSummaries) == 0 {
		buf.WriteString(tsdbDim("  [No scheduler job execution data recorded in the specified window]\n\n"))
	} else {
		buf.WriteString(tsdbWhiteBold("Scheduler Job Execution Performance & Latency Matrix:\n\n"))
		idW := 28
		runsW := 6
		succW := 7
		failW := 5
		avgW := 12
		minMaxW := 18
		lastW := 10

		buf.WriteString(fmt.Sprintf("%-*s │ %-*s │ %-*s │ %-*s │ %-*s │ %-*s │ %-*s │ %s\n",
			idW, "JOB IDENTIFIER", runsW, "RUNS", succW, "SUCCESS", failW, "FAIL", avgW, "AVG LATENCY", minMaxW, "MIN / MAX", lastW, "LAST RUN", "LATENCY TREND"))
		buf.WriteString(tsdbDim(strings.Repeat("─", 106)) + "\n")

		for _, j := range telem.JobSummaries {
			lastRunStr := "--:--:--"
			if !j.LastRunAt.IsZero() {
				lastRunStr = j.LastRunAt.Format("15:04:05")
			}

			failBadge := fmt.Sprintf("%d", j.Failures)
			if j.Failures > 0 {
				failBadge = tsdbRedBold(failBadge)
			}

			avgDurStr := formatDurationMs(j.AvgDurationMs)
			minMaxStr := fmt.Sprintf("%s / %s", formatDurationMs(j.MinDurationMs), formatDurationMs(j.MaxDurationMs))

			displayID := j.JobID
			if len(displayID) > idW {
				displayID = displayID[:idW-3] + "..."
			}

			buf.WriteString(fmt.Sprintf("%-*s │ %-*d │ %-*d │ %-*s │ %-*s │ %-*s │ %-*s │ %s\n",
				idW, displayID, runsW, j.Executions, succW, j.Successes, failW, failBadge, avgW, avgDurStr, minMaxW, minMaxStr, lastW, lastRunStr, j.Sparkline))
		}
		buf.WriteString("\n")
	}

	if len(telem.RecentPoints) > 0 {
		buf.WriteString(tsdbWhiteBold(fmt.Sprintf("Recent Time-Series Points (Latest %d):\n", len(telem.RecentPoints))))
		buf.WriteString(tsdbDim(strings.Repeat("─", 80)) + "\n")
		for _, pt := range telem.RecentPoints {
			tStr := pt.Timestamp.Format("15:04:05")
			buf.WriteString(fmt.Sprintf("[%s] %s\n", tsdbDim(tStr), pt.Summary))
		}
		buf.WriteString("\n")
	}

	return buf.String()
}

func formatDurationMs(ms float64) string {
	if ms < 1.0 {
		return fmt.Sprintf("%.2fms", ms)
	}
	if ms < 1000.0 {
		return fmt.Sprintf("%.1fms", ms)
	}
	return fmt.Sprintf("%.2fs", ms/1000.0)
}
