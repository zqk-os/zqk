package state

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/resourcehygiene"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TSDBTelemetry holds aggregated performance and operational metrics from the kernel TSDB.
type TSDBTelemetry struct {
	TotalPoints   int                 `json:"total_points"`
	TotalFiles    int                 `json:"total_files"`
	DiskSizeBytes int64               `json:"disk_size_bytes"`
	DiskSizeStr   string              `json:"disk_size_str"`
	Measurements  []string            `json:"measurements"`
	JobSummaries  []TSDBJobSummary    `json:"job_summaries"`
	RecentPoints  []TSDBPointView     `json:"recent_points,omitempty"`
	ChunkStats    TSDBChunkStats      `json:"chunk_stats"`
	HygieneStats  TSDBHygieneStats    `json:"hygiene_stats"`
	TopCommands   []TSDBCommandMetric `json:"top_commands,omitempty"`
	KindVolumes   []TSDBKindVolume    `json:"kind_volumes,omitempty"`
	StreamRates   []TSDBStreamRate    `json:"stream_rates,omitempty"`
}

// TSDBHygieneStats represents Layer 4 storage & resource hygiene vitals.
type TSDBHygieneStats struct {
	OpenFileDescriptors int     `json:"open_file_descriptors"`
	MaxFileDescriptors  int     `json:"max_file_descriptors"`
	FDUsagePercent      float64 `json:"fd_usage_percent"`
	TotalStorageFiles   int     `json:"total_storage_files"`
	TotalStorageBytes   int64   `json:"total_storage_bytes"`
	StorageSizeStr      string  `json:"storage_size_str"`
	StaleLocksCount     int     `json:"stale_locks_count"`
	OrphanedTempCount   int     `json:"orphaned_temp_count"`
	DraftObjectsCount   int     `json:"draft_objects_count"`
	Status              string  `json:"status"`
}

// TSDBCommandMetric represents metrics for a single CLI command.
type TSDBCommandMetric struct {
	Command      string  `json:"command"`
	Invocations  int     `json:"invocations"`
	Successes    int     `json:"successes"`
	Failures     int     `json:"failures"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	LastSeen     string  `json:"last_seen"`
}

// TSDBKindVolume represents object volume stats for a single kind.
type TSDBKindVolume struct {
	Kind         string `json:"kind"`
	CurrentCount int    `json:"current_count"`
}

// TSDBStreamRate represents mutation rate for an event stream.
type TSDBStreamRate struct {
	StreamName string `json:"stream_name"`
	TotalFiles int    `json:"total_files"`
}

// TSDBJobSummary represents time-series aggregates for a single scheduler job.
type TSDBJobSummary struct {
	JobID          string    `json:"job_id"`
	JobType        string    `json:"job_type"`
	Executions     int       `json:"executions"`
	Successes      int       `json:"successes"`
	Failures       int       `json:"failures"`
	SuccessRate    float64   `json:"success_rate"`
	AvgDurationMs  float64   `json:"avg_duration_ms"`
	MinDurationMs  float64   `json:"min_duration_ms"`
	MaxDurationMs  float64   `json:"max_duration_ms"`
	LastDurationMs float64   `json:"last_duration_ms"`
	LastRunAt      time.Time `json:"last_run_at"`
	Sparkline      string    `json:"sparkline"`
}

// TSDBPointView represents a decoded TSDB data point for inspection.
type TSDBPointView struct {
	Timestamp   time.Time         `json:"timestamp"`
	Measurement string            `json:"measurement"`
	Tags        map[string]string `json:"tags"`
	Fields      map[string]any    `json:"fields"`
	Summary     string            `json:"summary"`
}

// TSDBChunkStats represents statistics on chunked metrics in .zqk/metrics/.
type TSDBChunkStats struct {
	TotalChunks int      `json:"total_chunks"`
	DiskBytes   int64    `json:"disk_bytes"`
	DiskSizeStr string   `json:"disk_size_str"`
	SeriesNames []string `json:"series_names"`
}

// ReadTSDBTelemetry reads and computes full time-series telemetry from disk.
func ReadTSDBTelemetry(projectRoot string, since time.Duration, recentLimit int) *TSDBTelemetry {
	telem := &TSDBTelemetry{
		Measurements: []string{},
		JobSummaries: []TSDBJobSummary{},
		RecentPoints: []TSDBPointView{},
	}

	if projectRoot == "" {
		return telem
	}

	tsdbDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, "tsdb")
	files, err := fileutil.ReadDir(tsdbDir)
	if err != nil || len(files) == 0 {
		readChunkMetrics(projectRoot, telem)
		readHygieneStats(projectRoot, telem)
		readCommandMetricsStats(projectRoot, telem)
		readObjectVolumeStats(projectRoot, telem)
		readStreamVolumeStats(projectRoot, telem)
		return telem
	}

	var now = time.Now().UTC()
	var cutoff time.Time
	if since > 0 {
		cutoff = now.Add(-since)
	}

	measurementSet := make(map[string]bool)

	type jobAgg struct {
		jobID        string
		jobType      string
		executions   int
		successes    int
		failures     int
		durations    []float64
		lastRunAt    time.Time
		lastDuration float64
	}
	jobs := make(map[string]*jobAgg)
	var allRecentPoints []TSDBPointView

	for _, f := range files {
		if f.IsDir() || !strings.HasPrefix(f.Name(), "metrics-") || !strings.HasSuffix(f.Name(), ".tsdb") {
			continue
		}
		telem.TotalFiles++
		p := filepath.Join(tsdbDir, f.Name())
		content, rErr := fileutil.ReadFile(p)
		if rErr != nil {
			continue
		}
		telem.DiskSizeBytes += int64(len(content))

		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			decoded, dErr := base64.StdEncoding.DecodeString(line)
			if dErr != nil {
				continue
			}

			// Format: Measurement|Tags(JSON)|Timestamp(nano)|Fields(JSON)
			str := string(decoded)
			parts := strings.SplitN(str, "|", 4)
			if len(parts) < 4 {
				continue
			}

			telem.TotalPoints++
			meas := parts[0]
			measurementSet[meas] = true

			tsNano, _ := strconv.ParseInt(parts[2], 10, 64)
			ptTime := time.Unix(0, tsNano).UTC()

			if !cutoff.IsZero() && ptTime.Before(cutoff) {
				continue
			}

			var tags map[string]string
			_ = json.Unmarshal([]byte(parts[1]), &tags)

			var fields map[string]any
			_ = json.Unmarshal([]byte(parts[3]), &fields)

			if meas == "scheduler_job_execution" {
				jID := tags["job_id"]
				if jID == "" {
					jID = "unknown_job"
				}
				jType := tags["job_type"]

				durSec := 0.0
				if v, ok := fields["duration_seconds"]; ok {
					if n, ok := v.(float64); ok {
						durSec = n
					}
				}
				durMs := durSec * 1000.0

				success := true
				if v, ok := fields["success"]; ok {
					if b, ok := v.(bool); ok {
						success = b
					}
				}

				agg, exists := jobs[jID]
				if !exists {
					agg = &jobAgg{
						jobID:     jID,
						jobType:   jType,
						durations: make([]float64, 0, 16),
					}
					jobs[jID] = agg
				}
				agg.executions++
				if success {
					agg.successes++
				} else {
					agg.failures++
				}
				agg.durations = append(agg.durations, durMs)
				if ptTime.After(agg.lastRunAt) {
					agg.lastRunAt = ptTime
					agg.lastDuration = durMs
				}
			}

			// Accumulate recent points
			sum := fmt.Sprintf("%s", meas)
			if jID, ok := tags["job_id"]; ok {
				durStr := ""
				if d, ok := fields["duration_seconds"].(float64); ok {
					durStr = fmt.Sprintf(" duration=%.2fs", d)
				}
				sum = fmt.Sprintf("%s | %s%s", meas, jID, durStr)
			}
			allRecentPoints = append(allRecentPoints, TSDBPointView{
				Timestamp:   ptTime,
				Measurement: meas,
				Tags:        tags,
				Fields:      fields,
				Summary:     sum,
			})
		}
	}

	telem.DiskSizeStr = formatByteSize(telem.DiskSizeBytes)
	for m := range measurementSet {
		telem.Measurements = append(telem.Measurements, m)
	}
	sort.Strings(telem.Measurements)

	// Format job summaries
	summaries := make([]TSDBJobSummary, 0, len(jobs))
	for _, j := range jobs {
		total := j.executions
		sRate := 100.0
		if total > 0 {
			sRate = (float64(j.successes) / float64(total)) * 100.0
		}

		minD := math.MaxFloat64
		maxD := 0.0
		sumD := 0.0
		for _, d := range j.durations {
			if d < minD {
				minD = d
			}
			if d > maxD {
				maxD = d
			}
			sumD += d
		}
		avgD := 0.0
		if len(j.durations) > 0 {
			avgD = sumD / float64(len(j.durations))
		}
		if minD == math.MaxFloat64 {
			minD = 0.0
		}

		// Generate sparkline from up to last 8 durations
		sparkSlice := j.durations
		if len(sparkSlice) > 8 {
			sparkSlice = sparkSlice[len(sparkSlice)-8:]
		}
		spark := generateDurationSparkline(sparkSlice)

		summaries = append(summaries, TSDBJobSummary{
			JobID:          j.jobID,
			JobType:        j.jobType,
			Executions:     j.executions,
			Successes:      j.successes,
			Failures:       j.failures,
			SuccessRate:    sRate,
			AvgDurationMs:  avgD,
			MinDurationMs:  minD,
			MaxDurationMs:  maxD,
			LastDurationMs: j.lastDuration,
			LastRunAt:      j.lastRunAt,
			Sparkline:      spark,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].JobID < summaries[j].JobID
	})
	telem.JobSummaries = summaries

	// Sort recent points descending
	sort.Slice(allRecentPoints, func(i, j int) bool {
		return allRecentPoints[i].Timestamp.After(allRecentPoints[j].Timestamp)
	})
	if recentLimit > 0 && len(allRecentPoints) > recentLimit {
		allRecentPoints = allRecentPoints[:recentLimit]
	}
	telem.RecentPoints = allRecentPoints

	// Inspect chunk metrics, hygiene vitals, commands, object volumes, and streams
	readChunkMetrics(projectRoot, telem)
	readHygieneStats(projectRoot, telem)
	readCommandMetricsStats(projectRoot, telem)
	readObjectVolumeStats(projectRoot, telem)
	readStreamVolumeStats(projectRoot, telem)

	return telem
}

func readChunkMetrics(projectRoot string, telem *TSDBTelemetry) {
	metricsDir := filepath.Join(projectRoot, paths.ProjectDataDir, "metrics")
	entries, err := fileutil.ReadDir(metricsDir)
	if err != nil {
		return
	}

	totalChunks := 0
	var totalBytes int64
	var series []string

	for _, e := range entries {
		if e.IsDir() {
			series = append(series, e.Name())
			sDir := filepath.Join(metricsDir, e.Name())
			cFiles, cErr := fileutil.ReadDir(sDir)
			if cErr == nil {
				totalChunks += len(cFiles)
				for _, cf := range cFiles {
					if info, err := cf.Info(); err == nil {
						totalBytes += info.Size()
					}
				}
			}
		}
	}

	sort.Strings(series)
	telem.ChunkStats = TSDBChunkStats{
		TotalChunks: totalChunks,
		DiskBytes:   totalBytes,
		DiskSizeStr: formatByteSize(totalBytes),
		SeriesNames: series,
	}
}

func generateDurationSparkline(vals []float64) string {
	if len(vals) == 0 {
		return "--"
	}
	if len(vals) == 1 {
		return "• BASE"
	}

	// Compare latest run to the baseline average of previous runs
	latest := vals[len(vals)-1]
	sumPrev := 0.0
	for i := 0; i < len(vals)-1; i++ {
		sumPrev += vals[i]
	}
	avgPrev := sumPrev / float64(len(vals)-1)

	if avgPrev <= 0 {
		return "• BASE"
	}

	diffPct := ((latest - avgPrev) / avgPrev) * 100.0

	// Intelligible trend indicator
	if diffPct > 15.0 {
		return fmt.Sprintf("▲ +%.0f%%", diffPct) // Latency increased (slower)
	} else if diffPct < -15.0 {
		return fmt.Sprintf("▼ %.0f%%", diffPct)  // Latency decreased (faster)
	}
	return "→ STABLE"
}

func formatByteSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func readHygieneStats(projectRoot string, telem *TSDBTelemetry) {
	ioTel, err := resourcehygiene.InspectIOResources(context.Background(), projectRoot)
	if err != nil || ioTel == nil {
		return
	}

	fdPct := 0.0
	if ioTel.MaxFileDescriptors > 0 && ioTel.OpenFileDescriptors > 0 {
		fdPct = (float64(ioTel.OpenFileDescriptors) / float64(ioTel.MaxFileDescriptors)) * 100.0
	}

	status := "HEALTHY"
	if ioTel.StaleLocksCount > 0 || ioTel.OrphanedTempCount > 0 || fdPct > 70.0 {
		status = "ATTENTION"
	}
	if fdPct > 90.0 {
		status = "CRITICAL"
	}

	draftsCount := 0
	draftsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.ObjectDraftsDir)
	if dEntries, dErr := fileutil.ReadDir(draftsDir); dErr == nil {
		for _, de := range dEntries {
			if de.IsDir() {
				if sub, sErr := fileutil.ReadDir(filepath.Join(draftsDir, de.Name())); sErr == nil {
					draftsCount += len(sub)
				}
			}
		}
	}

	telem.HygieneStats = TSDBHygieneStats{
		OpenFileDescriptors: ioTel.OpenFileDescriptors,
		MaxFileDescriptors:  ioTel.MaxFileDescriptors,
		FDUsagePercent:      fdPct,
		TotalStorageFiles:   int(ioTel.TotalZqkFiles),
		TotalStorageBytes:   ioTel.TotalZqkBytes,
		StorageSizeStr:      formatByteSize(ioTel.TotalZqkBytes),
		StaleLocksCount:     ioTel.StaleLocksCount,
		OrphanedTempCount:   ioTel.OrphanedTempCount,
		DraftObjectsCount:   draftsCount,
		Status:              status,
	}
}

type commandMetricsPayload struct {
	Metrics map[string]struct {
		Command         string `json:"command"`
		NormalizedCmd   string `json:"normalized_cmd"`
		InvocationCount int    `json:"invocation_count"`
		SuccessCount    int    `json:"success_count"`
		FailureCount    int    `json:"failure_count"`
		AvgDuration     int64  `json:"avg_duration"` // nanoseconds
		LastSeen        string `json:"last_seen"`
	} `json:"metrics"`
}

func readCommandMetricsStats(projectRoot string, telem *TSDBTelemetry) {
	cmdMetricsFile := filepath.Join(projectRoot, paths.ProjectDataDir, "metrics", "command_metrics.json")
	data, err := fileutil.ReadFile(cmdMetricsFile)
	if err != nil {
		return
	}

	var parsed commandMetricsPayload
	if err := json.Unmarshal(data, &parsed); err != nil {
		return
	}

	cmds := make([]TSDBCommandMetric, 0, len(parsed.Metrics))
	for _, m := range parsed.Metrics {
		cmds = append(cmds, TSDBCommandMetric{
			Command:      m.NormalizedCmd,
			Invocations:  m.InvocationCount,
			Successes:    m.SuccessCount,
			Failures:     m.FailureCount,
			AvgLatencyMs: float64(m.AvgDuration) / 1e6,
			LastSeen:     m.LastSeen,
		})
	}

	sort.Slice(cmds, func(i, j int) bool {
		if cmds[i].Invocations != cmds[j].Invocations {
			return cmds[i].Invocations > cmds[j].Invocations
		}
		if cmds[i].Command != cmds[j].Command {
			return cmds[i].Command < cmds[j].Command
		}
		return cmds[i].LastSeen > cmds[j].LastSeen
	})

	if len(cmds) > 6 {
		cmds = cmds[:6]
	}
	telem.TopCommands = cmds
}

func readObjectVolumeStats(projectRoot string, telem *TSDBTelemetry) {
	processDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.ProcessDir)
	entries, err := fileutil.ReadDir(processDir)
	if err != nil {
		return
	}

	trackedKinds := map[string]bool{
		"backlog_item":         true,
		"priority_plan":        true,
		"workstream":           true,
		"requirement":          true,
		"criteria":             true,
		"test_case":            true,
		"doc_entry":            true,
		"risk_blocker":         true,
		"technical_debt":       true,
		"change_journal_entry": true,
		"audit_event":          true,
	}

	vols := make([]TSDBKindVolume, 0)
	for _, e := range entries {
		if e.IsDir() && trackedKinds[e.Name()] {
			kDir := filepath.Join(processDir, e.Name())
			files, fErr := fileutil.ReadDir(kDir)
			count := 0
			if fErr == nil {
				for _, f := range files {
					if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {
						count++
					}
				}
			}
			vols = append(vols, TSDBKindVolume{
				Kind:         e.Name(),
				CurrentCount: count,
			})
		}
	}

	sort.Slice(vols, func(i, j int) bool {
		return vols[i].CurrentCount > vols[j].CurrentCount
	})

	telem.KindVolumes = vols
}

func readStreamVolumeStats(projectRoot string, telem *TSDBTelemetry) {
	streamsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StreamsDir)
	entries, err := fileutil.ReadDir(streamsDir)
	if err != nil {
		return
	}

	rates := make([]TSDBStreamRate, 0)
	for _, e := range entries {
		if e.IsDir() {
			sDir := filepath.Join(streamsDir, e.Name())
			files, fErr := fileutil.ReadDir(sDir)
			fileCount := 0
			if fErr == nil {
				fileCount = len(files)
			}
			rates = append(rates, TSDBStreamRate{
				StreamName: e.Name(),
				TotalFiles: fileCount,
			})
		}
	}

	sort.Slice(rates, func(i, j int) bool {
		return rates[i].TotalFiles > rates[j].TotalFiles
	})

	telem.StreamRates = rates
}

