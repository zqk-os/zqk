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

	if telem.HygieneStats.MaxFileDescriptors > 0 {
		buf.WriteString(tsdbWhiteBold("System Vitals & Resource Hygiene:\n"))
		fdStatus := tsdbGreenBold("[✓ HEALTHY]")
		if telem.HygieneStats.Status == "ATTENTION" {
			fdStatus = tsdbYellowBold("[⚠️ ATTENTION]")
		} else if telem.HygieneStats.Status == "CRITICAL" {
			fdStatus = tsdbRedBold("[✗ CRITICAL]")
		}
		fmt.Fprintf(&buf, "  File Descriptors: %d / %d (%.2f%%) %s\n",
			telem.HygieneStats.OpenFileDescriptors, telem.HygieneStats.MaxFileDescriptors,
			telem.HygieneStats.FDUsagePercent, fdStatus)
		fmt.Fprintf(&buf, "  Storage Volume:   %s (%d files in .zqk) │ Stale Locks: %d │ Orphaned Temp: %d │ Drafts: %d\n",
			telem.HygieneStats.StorageSizeStr, telem.HygieneStats.TotalStorageFiles,
			telem.HygieneStats.StaleLocksCount, telem.HygieneStats.OrphanedTempCount,
			telem.HygieneStats.DraftObjectsCount)
		buf.WriteString(tsdbDim(strings.Repeat("─", 90)) + "\n")
	}

	if len(telem.KindVolumes) > 0 {
		buf.WriteString(tsdbWhiteBold("Kernel Object Volumes (CAS Storage):\n"))
		var items []string
		for _, kv := range telem.KindVolumes {
			items = append(items, fmt.Sprintf("%s: %s", tsdbDim(kv.Kind), tsdbCyanBold(fmt.Sprintf("%d", kv.CurrentCount))))
		}
		for i := 0; i < len(items); i += 4 {
			end := i + 4
			if end > len(items) {
				end = len(items)
			}
			buf.WriteString("  " + strings.Join(items[i:end], " │ ") + "\n")
		}
		buf.WriteString(tsdbDim(strings.Repeat("─", 90)) + "\n")
	}

	if len(telem.StreamRates) > 0 {
		buf.WriteString(tsdbWhiteBold("Active Streams Ingestion Volume:\n"))
		var sItems []string
		for _, sr := range telem.StreamRates {
			sItems = append(sItems, fmt.Sprintf("%s: %s files", tsdbDim(sr.StreamName), tsdbYellowBold(fmt.Sprintf("%d", sr.TotalFiles))))
		}
		for i := 0; i < len(sItems); i += 3 {
			end := i + 3
			if end > len(sItems) {
				end = len(sItems)
			}
			buf.WriteString("  " + strings.Join(sItems[i:end], " │ ") + "\n")
		}
		buf.WriteString(tsdbDim(strings.Repeat("─", 90)) + "\n")
	}

	if len(telem.TopCommands) > 0 {
		buf.WriteString(tsdbWhiteBold("CLI Command Velocity (Top Invocations):\n"))
		cmdColW := 36
		invColW := 8
		latColW := 12
		errColW := 8
		buf.WriteString(fmt.Sprintf("  %-*s │ %-*s │ %-*s │ %-*s\n",
			cmdColW, "COMMAND", invColW, "RUNS", latColW, "AVG LATENCY", errColW, "ERRORS"))
		buf.WriteString("  " + tsdbDim(strings.Repeat("─", cmdColW+invColW+latColW+errColW+9)) + "\n")
		for _, c := range telem.TopCommands {
			cName := c.Command
			if len(cName) > cmdColW {
				cName = cName[:cmdColW-3] + "..."
			}
			errStr := fmt.Sprintf("%d", c.Failures)
			if c.Failures > 0 {
				errStr = tsdbRedBold(errStr)
			}
			buf.WriteString(fmt.Sprintf("  %-*s │ %-*d │ %-*s │ %-*s\n",
				cmdColW, cName, invColW, c.Invocations, latColW, formatDurationMs(c.AvgLatencyMs), errColW, errStr))
		}
		buf.WriteString(tsdbDim(strings.Repeat("─", 90)) + "\n")
	}

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
