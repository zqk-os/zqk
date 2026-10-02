package scheduler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewSchedulerBundleProgressCmd wires the bundle-progress command.
func NewSchedulerBundleProgressCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSchedulerBundleProgressCommandBuilder()
	cli.BindAsyncProgress(cmd, runBundleProgress)
	return cmd
}

type bundleProgressRow struct {
	JobID       string
	RunState    string // pending | running | pass | fail
	StartTime   time.Time
	EndTime     time.Time
	ElapsedTime time.Duration
}

func runBundleProgress(cmd *cobra.Command, args []string) error {
	projectRoot, err := resolveSchedulerProjectRoot(cmd)
	if err != nil {
		return err
	}

	rows, err := readBundleProgress(projectRoot)
	if err != nil {
		return err
	}

	// Render table
	var b strings.Builder
	b.WriteString("Bundle Progress Matrix:\n")
	b.WriteString(strings.Repeat("-", 75) + "\n")
	fmt.Fprintf(&b, "%-20s | %-8s | %-25s | %-12s\n", "Job ID", "Status", "Started", "Elapsed")
	b.WriteString(strings.Repeat("-", 75) + "\n")

	total := len(rows)
	completed := 0
	passed := 0
	failed := 0
	running := 0
	pending := 0

	for _, r := range rows {
		statusStr := r.RunState
		startStr := "-"
		elapsedStr := "-"

		if !r.StartTime.IsZero() {
			startStr = r.StartTime.Format(time.RFC3339)
			if r.ElapsedTime > 0 {
				elapsedStr = r.ElapsedTime.Round(time.Second).String()
			}
		}

		fmt.Fprintf(&b, "%-20s | %-8s | %-25s | %-12s\n", r.JobID, statusStr, startStr, elapsedStr)

		switch statusStr {
		case "pass":
			completed++
			passed++
		case "fail":
			completed++
			failed++
		case "running":
			running++
		case "pending":
			pending++
		}
	}

	b.WriteString(strings.Repeat("-", 75) + "\n")
	fmt.Fprintf(&b, "Summary: %d total, %d completed, %d passed, %d failed, %d running, %d pending\n",
		total, completed, passed, failed, running, pending)

	return cli.WriteOutput(cmd, []byte(b.String()))
}

func readBundleProgress(projectRoot string) ([]bundleProgressRow, error) {
	path := scheduler.TestBundlesProgressFilePath(projectRoot)
	f, err := fileutil.Open(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return buildDefaultPendingRows(projectRoot), nil
		}
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	eventsByJob := map[string][]map[string]any{}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err == nil {
			jobID, _ := m["job_id"].(string)
			if jobID != "" {
				eventsByJob[jobID] = append(eventsByJob[jobID], m)
			}
		}
	}

	knownIDs := readKnownBundleJobIDs(projectRoot)

	for jID := range eventsByJob {
		found := false
		for _, kID := range knownIDs {
			if kID == jID {
				found = true
				break
			}
		}
		if !found {
			knownIDs = append(knownIDs, jID)
		}
	}
	sort.Strings(knownIDs)

	var rows []bundleProgressRow
	for _, jID := range knownIDs {
		events := eventsByJob[jID]
		row := bundleProgressRow{
			JobID:    jID,
			RunState: "pending",
		}
		var startTs, endTs time.Time
		for _, ev := range events {
			et, _ := ev[objects.FieldKeyEventType].(string)
			tsStr, _ := ev["timestamp"].(string)
			ts, err := time.Parse(time.RFC3339, tsStr)
			if err != nil {
				continue
			}
			if et == "started" {
				startTs = ts
				row.RunState = "running"
			} else if et == "passed" || et == "pass" || et == "completed" {
				endTs = ts
				row.RunState = "pass"
			} else if et == "failed" || et == "fail" || et == "test_fail" {
				endTs = ts
				row.RunState = "fail"
			}
		}
		if !startTs.IsZero() {
			row.StartTime = startTs
			if !endTs.IsZero() && endTs.After(startTs) {
				row.EndTime = endTs
				row.ElapsedTime = endTs.Sub(startTs)
			} else if row.RunState == "running" {
				row.ElapsedTime = time.Since(startTs)
			}
		}
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ElapsedTime == rows[j].ElapsedTime {
			return rows[i].JobID < rows[j].JobID
		}
		return rows[i].ElapsedTime > rows[j].ElapsedTime
	})

	return rows, nil
}

func readKnownBundleJobIDs(projectRoot string) []string {
	var knownIDs []string
	bundleStorageDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.TestBundlesDir)
	if entries, err := fileutil.ReadDir(bundleStorageDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
				name := entry.Name()[:len(entry.Name())-5] // Remove .json extension
				knownIDs = append(knownIDs, "SCH-run-bundle-"+name)
			}
		}
	}
	return knownIDs
}

func buildDefaultPendingRows(projectRoot string) []bundleProgressRow {
	knownIDs := readKnownBundleJobIDs(projectRoot)
	sort.Strings(knownIDs)

	var rows []bundleProgressRow
	for _, jID := range knownIDs {
		rows = append(rows, bundleProgressRow{
			JobID:    jID,
			RunState: "pending",
		})
	}
	return rows
}
