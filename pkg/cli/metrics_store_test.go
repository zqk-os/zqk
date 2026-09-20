package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFileMetricsStore_Basic(t *testing.T) {
	tempDir := t.TempDir()
	metricsPath := filepath.Join(tempDir, "command_metrics.json")
	chunksDir := filepath.Join(tempDir, "command_metrics")

	store, err := NewFileMetricsStoreWithConfig(metricsPath, chunksDir, 14)
	if err != nil {
		t.Fatalf("NewFileMetricsStoreWithConfig failed: %v", err)
	}

	today := time.Now().UTC().Format("2006-01-02")
	if store.WindowDate() != today {
		t.Errorf("expected WindowDate %s, got %s", today, store.WindowDate())
	}

	// Record execution
	metric := &CommandMetric{
		Command:       "zqk",
		NormalizedCmd: "zqk system check",
		Duration:      100 * time.Millisecond,
		Timestamp:     time.Now().UTC(),
		Success:       true,
		ExitCode:      0,
	}
	if err := store.RecordCommandExecution(metric); err != nil {
		t.Fatalf("RecordCommandExecution failed: %v", err)
	}

	m, err := store.GetCommandMetrics("zqk system check")
	if err != nil {
		t.Fatalf("GetCommandMetrics failed: %v", err)
	}
	if m == nil {
		t.Fatalf("expected metrics for 'zqk system check', got nil")
	}
	if m.InvocationCount != 1 || m.SuccessCount != 1 || m.FailureCount != 0 {
		t.Errorf("unexpected counts: inv=%d, succ=%d, fail=%d", m.InvocationCount, m.SuccessCount, m.FailureCount)
	}
	if m.FastestDuration != 100*time.Millisecond || m.SlowestDuration != 100*time.Millisecond {
		t.Errorf("unexpected durations: fast=%v, slow=%v", m.FastestDuration, m.SlowestDuration)
	}

	// Record a failure
	failMetric := &CommandMetric{
		Command:       "zqk",
		NormalizedCmd: "zqk system check",
		Duration:      200 * time.Millisecond,
		Timestamp:     time.Now().UTC(),
		Success:       false,
		ExitCode:      1,
	}
	if err := store.RecordCommandExecution(failMetric); err != nil {
		t.Fatalf("RecordCommandExecution failed: %v", err)
	}

	m, err = store.GetCommandMetrics("zqk system check")
	if err != nil {
		t.Fatalf("GetCommandMetrics failed: %v", err)
	}
	if m.InvocationCount != 2 || m.SuccessCount != 1 || m.FailureCount != 1 {
		t.Errorf("unexpected counts after failure: inv=%d, succ=%d, fail=%d", m.InvocationCount, m.SuccessCount, m.FailureCount)
	}
	if m.ErrorRate != 50.0 {
		t.Errorf("expected error rate 50.0, got %f", m.ErrorRate)
	}
	store.WaitForFlushes()
}

func TestFileMetricsStore_DayRoll_OnLoad(t *testing.T) {
	tempDir := t.TempDir()
	metricsPath := filepath.Join(tempDir, "command_metrics.json")
	chunksDir := filepath.Join(tempDir, "command_metrics")

	// Pre-create an old command_metrics.json from 2 days ago
	oldDate := "2026-09-10"
	oldPayload := struct {
		Metrics    map[string]*CommandMetrics `json:"metrics"`
		Updated    time.Time                  `json:"updated"`
		WindowDate string                     `json:"window_date"`
	}{
		Metrics: map[string]*CommandMetrics{
			"zqk object get": {
				Command:         "zqk",
				NormalizedCmd:   "zqk object get",
				InvocationCount: 5,
				SuccessCount:    4,
				FailureCount:    1,
				FastestDuration: 50 * time.Millisecond,
				SlowestDuration: 150 * time.Millisecond,
				AvgDuration:     100 * time.Millisecond,
				ErrorRate:       20.0,
			},
		},
		Updated:    time.Now().UTC().Add(-48 * time.Hour),
		WindowDate: oldDate,
	}
	oldData, err := json.Marshal(oldPayload)
	if err != nil {
		t.Fatalf("failed to marshal old metrics: %v", err)
	}
	if err := fileutil.WriteFile(metricsPath, oldData, paths.FilePerm644); err != nil {
		t.Fatalf("failed to write fixture metrics: %v", err)
	}

	// Loading store today should trigger automatic day-roll!
	store, err := NewFileMetricsStoreWithConfig(metricsPath, chunksDir, 14)
	if err != nil {
		t.Fatalf("NewFileMetricsStoreWithConfig failed: %v", err)
	}

	today := time.Now().UTC().Format("2006-01-02")
	if store.WindowDate() != today {
		t.Errorf("expected WindowDate %s after rollover, got %s", today, store.WindowDate())
	}

	// Current active metrics should be reset to empty for today
	currentMetrics, err := store.GetAllMetrics()
	if err != nil {
		t.Fatalf("GetAllMetrics failed: %v", err)
	}
	if len(currentMetrics) != 0 {
		t.Errorf("expected 0 metrics in current bounded window after roll, got %d", len(currentMetrics))
	}

	// The rolled day should exist in chunksDir
	rolledFile := filepath.Join(chunksDir, "command_metrics_20260910.json")
	if _, err := os.Stat(rolledFile); err != nil {
		t.Fatalf("expected rolled chunk file %s to exist: %v", rolledFile, err)
	}

	// Timeseries chunks should also have been created
	rolledChunks, err := filepath.Glob(filepath.Join(chunksDir, "*.chunk"))
	if err != nil || len(rolledChunks) == 0 {
		t.Errorf("expected timeseries .chunk files in %s, got %v", chunksDir, rolledChunks)
	}

	// Query rolled day
	oldMetrics, err := store.GetMetricsForDate("2026-09-10")
	if err != nil {
		t.Fatalf("GetMetricsForDate failed: %v", err)
	}
	if len(oldMetrics) != 1 || oldMetrics["zqk object get"].InvocationCount != 5 {
		t.Errorf("unexpected old metrics content: %v", oldMetrics)
	}

	// GetAllTimeMetrics should combine historical rolled chunk and current
	allTime, err := store.GetAllTimeMetrics()
	if err != nil {
		t.Fatalf("GetAllTimeMetrics failed: %v", err)
	}
	if len(allTime) != 1 || allTime["zqk object get"].InvocationCount != 5 {
		t.Errorf("unexpected all-time metrics: %v", allTime)
	}
}

func TestFileMetricsStore_DayRoll_OnRecordExecution(t *testing.T) {
	tempDir := t.TempDir()
	metricsPath := filepath.Join(tempDir, "command_metrics.json")
	chunksDir := filepath.Join(tempDir, "command_metrics")

	store, err := NewFileMetricsStoreWithConfig(metricsPath, chunksDir, 14)
	if err != nil {
		t.Fatalf("NewFileMetricsStoreWithConfig failed: %v", err)
	}

	// Simulate recording a command for yesterday
	yesterday := time.Now().UTC().Add(-24 * time.Hour)
	store.windowDate = yesterday.Format("2006-01-02")
	metricYesterday := &CommandMetric{
		Command:       "zqk",
		NormalizedCmd: "zqk pplan list",
		Duration:      50 * time.Millisecond,
		Timestamp:     yesterday,
		Success:       true,
	}
	if err := store.RecordCommandExecution(metricYesterday); err != nil {
		t.Fatalf("RecordCommandExecution yesterday failed: %v", err)
	}

	// Now record a command for today — should trigger rollover!
	now := time.Now().UTC()
	metricToday := &CommandMetric{
		Command:       "zqk",
		NormalizedCmd: "zqk system check",
		Duration:      150 * time.Millisecond,
		Timestamp:     now,
		Success:       true,
	}
	if err := store.RecordCommandExecution(metricToday); err != nil {
		t.Fatalf("RecordCommandExecution today failed: %v", err)
	}

	// Current store should now contain ONLY today's command
	current, err := store.GetAllMetrics()
	if err != nil {
		t.Fatalf("GetAllMetrics failed: %v", err)
	}
	if len(current) != 1 {
		t.Fatalf("expected 1 command in current bounded window, got %d", len(current))
	}
	if _, ok := current["zqk system check"]; !ok {
		t.Errorf("expected 'zqk system check' in current window, got %v", current)
	}

	// Yesterday's command should be in the rolled chunk
	yesterdayDateCompact := yesterday.Format("20060102")
	yesterdayFile := filepath.Join(chunksDir, "command_metrics_"+yesterdayDateCompact+".json")
	if _, err := os.Stat(yesterdayFile); err != nil {
		t.Fatalf("expected yesterday rolled chunk file %s to exist: %v", yesterdayFile, err)
	}

	// GetAllTimeMetrics should have BOTH
	allTime, err := store.GetAllTimeMetrics()
	if err != nil {
		t.Fatalf("GetAllTimeMetrics failed: %v", err)
	}
	if len(allTime) != 2 {
		t.Errorf("expected 2 commands in all-time aggregation, got %d", len(allTime))
	}
}

func TestFileMetricsStore_Pruning(t *testing.T) {
	tempDir := t.TempDir()
	metricsPath := filepath.Join(tempDir, "command_metrics.json")
	chunksDir := filepath.Join(tempDir, "command_metrics")

	store, err := NewFileMetricsStoreWithConfig(metricsPath, chunksDir, 3) // 3-day retention
	if err != nil {
		t.Fatalf("NewFileMetricsStoreWithConfig failed: %v", err)
	}

	if err := fileutil.MkdirAll(chunksDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// Create a recent chunk (1 day old) and an expired chunk (10 days old)
	recentPath := filepath.Join(chunksDir, "command_metrics_20260916.json")
	expiredPath := filepath.Join(chunksDir, "command_metrics_20260901.json")
	recentChunk := filepath.Join(chunksDir, "total_invocations_20260916T0000.chunk")
	expiredChunk := filepath.Join(chunksDir, "total_invocations_20260901T0000.chunk")

	dummyContent, _ := json.Marshal(struct {
		Metrics map[string]*CommandMetrics `json:"metrics"`
	}{Metrics: map[string]*CommandMetrics{}})
	_ = fileutil.WriteFile(recentPath, dummyContent, paths.FilePerm644)
	_ = fileutil.WriteFile(expiredPath, dummyContent, paths.FilePerm644)
	_ = fileutil.WriteFile(recentChunk, []byte("chunk"), paths.FilePerm644)
	_ = fileutil.WriteFile(expiredChunk, []byte("chunk"), paths.FilePerm644)

	// Set mod times
	now := time.Now().UTC()
	tenDaysAgo := now.Add(-10 * 24 * time.Hour)
	oneDayAgo := now.Add(-1 * 24 * time.Hour)
	_ = os.Chtimes(recentPath, oneDayAgo, oneDayAgo)
	_ = os.Chtimes(recentChunk, oneDayAgo, oneDayAgo)
	_ = os.Chtimes(expiredPath, tenDaysAgo, tenDaysAgo)
	_ = os.Chtimes(expiredChunk, tenDaysAgo, tenDaysAgo)

	// Prune
	if err := store.PruneOldChunks(); err != nil {
		t.Fatalf("PruneOldChunks failed: %v", err)
	}

	// Expired should be removed, recent should remain
	if _, err := os.Stat(expiredPath); !os.IsNotExist(err) {
		t.Errorf("expected expired file %s to be deleted, but it exists", expiredPath)
	}
	if _, err := os.Stat(expiredChunk); !os.IsNotExist(err) {
		t.Errorf("expected expired chunk %s to be deleted, but it exists", expiredChunk)
	}
	if _, err := os.Stat(recentPath); err != nil {
		t.Errorf("expected recent file %s to exist: %v", recentPath, err)
	}
	if _, err := os.Stat(recentChunk); err != nil {
		t.Errorf("expected recent chunk %s to exist: %v", recentChunk, err)
	}
}

func TestFileMetricsStore_CorruptedFileRecovery(t *testing.T) {
	tempDir := t.TempDir()
	metricsPath := filepath.Join(tempDir, "command_metrics.json")
	chunksDir := filepath.Join(tempDir, "command_metrics")

	// Corrupted file
	if err := fileutil.WriteFile(metricsPath, []byte(`{invalid json`), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write corrupted file: %v", err)
	}

	store, err := NewFileMetricsStoreWithConfig(metricsPath, chunksDir, 14)
	if err != nil {
		t.Fatalf("expected store to recover gracefully, got err: %v", err)
	}

	metrics, err := store.GetAllMetrics()
	if err != nil {
		t.Fatalf("GetAllMetrics failed: %v", err)
	}
	if len(metrics) != 0 {
		t.Errorf("expected empty metrics on corrupted file recovery, got %d", len(metrics))
	}
}

func TestFileMetricsStore_GetMetricsSince(t *testing.T) {
	tempDir := t.TempDir()
	metricsPath := filepath.Join(tempDir, "command_metrics.json")
	chunksDir := filepath.Join(tempDir, "command_metrics")

	store, err := NewFileMetricsStoreWithConfig(metricsPath, chunksDir, 14)
	if err != nil {
		t.Fatalf("NewFileMetricsStoreWithConfig failed: %v", err)
	}
	if err := fileutil.MkdirAll(chunksDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// Create chunks for 1 day ago and 5 days ago
	now := time.Now().UTC()
	oneDayAgoStr := now.Add(-24 * time.Hour).Format("20060102")
	fiveDaysAgoStr := now.Add(-5 * 24 * time.Hour).Format("20060102")

	chunk1 := filepath.Join(chunksDir, "command_metrics_"+oneDayAgoStr+".json")
	chunk5 := filepath.Join(chunksDir, "command_metrics_"+fiveDaysAgoStr+".json")

	data1, _ := json.Marshal(struct {
		Metrics map[string]*CommandMetrics `json:"metrics"`
	}{
		Metrics: map[string]*CommandMetrics{
			"cmd_recent": {
				Command:         "zqk",
				NormalizedCmd:   "cmd_recent",
				InvocationCount: 10,
				SuccessCount:    10,
			},
		},
	})
	data5, _ := json.Marshal(struct {
		Metrics map[string]*CommandMetrics `json:"metrics"`
	}{
		Metrics: map[string]*CommandMetrics{
			"cmd_old": {
				Command:         "zqk",
				NormalizedCmd:   "cmd_old",
				InvocationCount: 50,
				SuccessCount:    50,
			},
		},
	})

	if err := fileutil.WriteFile(chunk1, data1, paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile chunk1 failed: %v", err)
	}
	if err := fileutil.WriteFile(chunk5, data5, paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile chunk5 failed: %v", err)
	}

	// Query with 48h window: should include chunk1 (1 day ago), but exclude chunk5 (5 days ago)
	windowMetrics, err := store.GetMetricsSince(48 * time.Hour)
	if err != nil {
		t.Fatalf("GetMetricsSince failed: %v", err)
	}
	if _, ok := windowMetrics["cmd_recent"]; !ok {
		t.Errorf("expected cmd_recent in 48h window, got %v", windowMetrics)
	}
	if _, ok := windowMetrics["cmd_old"]; ok {
		t.Errorf("expected cmd_old to be excluded from 48h window, got %v", windowMetrics)
	}

	// Query with 7d window: should include both
	broadMetrics, err := store.GetMetricsSince(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("GetMetricsSince failed: %v", err)
	}
	if _, ok := broadMetrics["cmd_recent"]; !ok {
		t.Errorf("expected cmd_recent in 7d window, got %v", broadMetrics)
	}
	if _, ok := broadMetrics["cmd_old"]; !ok {
		t.Errorf("expected cmd_old in 7d window, got %v", broadMetrics)
	}
}
