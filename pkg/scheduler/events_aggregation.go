package scheduler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// Scheduler events and summary file names (under .zqk/scheduler/).
const (
	SchedulerEventsFileName  = "diagnostics.jsonl"
	SchedulerSummaryFileName = "scheduler-metrics-summary.json"
)

// SlowJobThresholdSec is the duration (seconds) above which a job's average run is considered "slow" in health view (e.g. P1 report).
const SlowJobThresholdSec = 300 // 5 min

// SchedulerMetricsSummary is the structure written to scheduler-metrics-summary.json.
type SchedulerMetricsSummary struct {
	WindowEndISO string                       `json:"window_end_iso"`
	LinesRead    int                          `json:"lines_read"`
	JobStats     map[string]JobExecutionStats `json:"job_stats"`
}

// JobExecutionStats holds per-job aggregated counts and duration.
type JobExecutionStats struct {
	Completed        int     `json:"completed"`
	Failed           int     `json:"failed"`
	TotalDurationSec float64 `json:"total_duration_sec"`
	LastCompletedISO string  `json:"last_completed_iso,omitempty"`
	LastFailedISO    string  `json:"last_failed_iso,omitempty"`
}

// Markers for job execution events — used to skip non-job JSONL lines without a full parse.
var (
	schedulerJobCompletedMarker = []byte(`"scheduler_job_completed"`)
	schedulerJobFailedMarker    = []byte(`"scheduler_job_failed"`)
)

// AggregateEventsFromFile reads a JSONL events file and returns a summary and line count.
// Event types considered: scheduler_job_completed, scheduler_job_failed.
// If the file does not exist, returns an empty summary and 0 lines (no error).
//
// Uses ReadBytes (not bufio.Scanner) so multi-MB JSONL lines — e.g. MCP tool errors that
// dumped full object-list payloads into diagnostics.jsonl — do not abort SCH-evag with
// "bufio.Scanner: token too long". Non-job lines are skipped without JSON unmarshal.
// TRACK: BLI-CAS-HAND-DUP-CHECK-001 — pair with logging.ErrorTextField truncation.
func AggregateEventsFromFile(eventsPath string) (*SchedulerMetricsSummary, int, error) {
	f, err := fileutil.Open(eventsPath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return &SchedulerMetricsSummary{JobStats: make(map[string]JobExecutionStats)}, 0, nil
		}
		return nil, 0, err
	}
	defer f.Close()

	jobStats := make(map[string]JobExecutionStats)
	reader := bufio.NewReaderSize(f, 256*1024)
	linesRead := 0

	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			linesRead++
			accumulateJobStatsFromDiagnosticsLine(jobStats, bytes.TrimSpace(line))
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, linesRead, readErr
		}
	}

	return &SchedulerMetricsSummary{JobStats: jobStats}, linesRead, nil
}

// accumulateJobStatsFromDiagnosticsLine updates jobStats for one diagnostics.jsonl line.
func accumulateJobStatsFromDiagnosticsLine(jobStats map[string]JobExecutionStats, line []byte) {
	if len(line) == 0 {
		return
	}
	if !bytes.Contains(line, schedulerJobCompletedMarker) && !bytes.Contains(line, schedulerJobFailedMarker) {
		return
	}
	var event map[string]interface{}
	if err := json.Unmarshal(line, &event); err != nil {
		return
	}
	eventType, _ := event[objects.FieldKeyEventType].(string)
	if eventType != "scheduler_job_completed" && eventType != "scheduler_job_failed" {
		return
	}
	jobID, _ := event["job_id"].(string)
	if jobID == emptyValue {
		return
	}
	st := jobStats[jobID]
	ts, _ := event["timestamp"].(string)
	if eventType == "scheduler_job_completed" {
		st.Completed++
		if d, ok := event[objects.FieldKeyDurationSeconds].(float64); ok {
			st.TotalDurationSec += d
		}
		if ts != emptyValue {
			st.LastCompletedISO = ts
		}
	} else {
		st.Failed++
		if ts != emptyValue {
			st.LastFailedISO = ts
		}
	}
	jobStats[jobID] = st
}

// GetGlobalTSDBProvider returns the global TSDBProvider instance (if any).
var (
	globalTSDBProvider any
	globalTSDBMutex    sync.RWMutex
)

// SetGlobalTSDBProvider sets the global TSDBProvider instance.
func SetGlobalTSDBProvider(tsdb any) {
	globalTSDBMutex.Lock()
	defer globalTSDBMutex.Unlock()
	globalTSDBProvider = tsdb
}

// GetGlobalTSDBProvider returns the global TSDBProvider instance.
func GetGlobalTSDBProvider() any {
	globalTSDBMutex.RLock()
	defer globalTSDBMutex.RUnlock()
	return globalTSDBProvider
}

// AggregateEventsFromTSDB reads events from the TSDB and returns a summary and line count.
func AggregateEventsFromTSDB(provider any) (*SchedulerMetricsSummary, int, error) {
	tsdb, ok := provider.(storage.TSDBProvider)
	if !ok {
		return nil, 0, errfmt.Errorf("invalid TSDB provider")
	}

	// Query scheduler_job_execution
	res, err := tsdb.Query(pkgctx.NewSystemContext(), storage.TSDBQuery{
		Measurement: "scheduler_job_execution",
	})
	if err != nil {
		return nil, 0, err
	}

	jobStats := make(map[string]JobExecutionStats)
	for _, pt := range res.Points {
		jobID, ok := pt.Tags["job_id"]
		if !ok || jobID == "" {
			continue
		}
		status := pt.Tags[objects.FieldKeyStatus]
		st := jobStats[jobID]
		ts := pt.Timestamp.Format(time.RFC3339)

		if status == "success" {
			st.Completed++
			if d, ok := pt.Fields[objects.FieldKeyDurationSeconds].(float64); ok {
				st.TotalDurationSec += d
			}
			st.LastCompletedISO = ts
		} else if status == "failed" {
			st.Failed++
			st.LastFailedISO = ts
		}
		jobStats[jobID] = st
	}

	return &SchedulerMetricsSummary{JobStats: jobStats}, len(res.Points), nil
}

// WriteSummary writes the summary JSON to summaryPath, creating the directory if needed.
func WriteSummary(summary *SchedulerMetricsSummary, summaryPath string) error {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(summaryPath)
	if err := fileutil.EnsureDir(dir); err != nil {
		return err
	}
	return fileutil.WriteSecureFile(summaryPath, data)
}

// EventsPath returns the path to diagnostics.jsonl under projectRoot.
func EventsPath(projectRoot string) string {
	newPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, SchedulerEventsFileName)
	legacyPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, "scheduler-events.json")
	if err := migrateLegacyPath(legacyPath, newPath); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to migrate legacy events path").WithError(err).Log()
	}
	return newPath
}

// SummaryPath returns the path to scheduler-metrics-summary.json under projectRoot.
func SummaryPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, SchedulerSummaryFileName)
}

// RunAggregation reads events from eventsPath, aggregates, and writes summary to summaryPath.
// Sets WindowEndISO and LinesRead on the summary. projectRoot is used only for logging context if needed.
func RunAggregation(projectRoot, eventsPath, summaryPath string) (*SchedulerMetricsSummary, int, error) {
	// First, attempt to use TSDBProvider if one is available
	if provider := GetGlobalTSDBProvider(); provider != nil {
		summary, linesRead, err := AggregateEventsFromTSDB(provider)
		if err == nil {
			summary.WindowEndISO = zqktime.NowRFC3339UTC()
			summary.LinesRead = linesRead
			if writeErr := WriteSummary(summary, summaryPath); writeErr != nil {
				return nil, linesRead, writeErr
			}
			return summary, linesRead, nil
		}
		// If TSDB fails, fallback to JSONL file
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Warn("Failed to aggregate events from TSDB, falling back to JSONL file").WithError(err).Log()
	}

	summary, linesRead, err := AggregateEventsFromFile(eventsPath)
	if err != nil {
		return nil, 0, err
	}
	_ = projectRoot // reserved for logging if handler needs it
	summary.WindowEndISO = zqktime.NowRFC3339UTC()
	summary.LinesRead = linesRead
	if err := WriteSummary(summary, summaryPath); err != nil {
		return nil, linesRead, err
	}
	return summary, linesRead, nil
}
