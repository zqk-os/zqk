package cli

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// DefaultCommandMetricsRetentionDays is the default retention period for day-rolled chunk files.
const DefaultCommandMetricsRetentionDays = 14

// FileMetricsStore implements MetricsStore using a JSON file with day-rolling and timeseries chunk integration.
type FileMetricsStore struct {
	filePath      string
	chunksDir     string
	retentionDays int
	windowDate    string       // UTC date formatted as YYYY-MM-DD for current in-memory / active file
	metrics       atomic.Value // holds map[string]*CommandMetrics
	mu            sync.Mutex
}

// cloneCommandMetricsMap returns a deep copy of the map values (struct copies); empty input yields a new empty map.
func cloneCommandMetricsMap(src map[string]*CommandMetrics) map[string]*CommandMetrics {
	if len(src) == 0 {
		return make(map[string]*CommandMetrics)
	}
	out := make(map[string]*CommandMetrics, len(src))
	for k, v := range src {
		vCopy := *v
		out[k] = &vCopy
	}
	return out
}

// NewFileMetricsStore creates a new file-based metrics store using default chunks directory and retention
func NewFileMetricsStore(filePath string) (*FileMetricsStore, error) {
	chunksDir := filepath.Join(filepath.Dir(filePath), paths.MetricsCommandMetricsSubdir)
	return NewFileMetricsStoreWithConfig(filePath, chunksDir, DefaultCommandMetricsRetentionDays)
}

// NewFileMetricsStoreWithConfig creates a new file-based metrics store with custom chunks directory and retention
func NewFileMetricsStoreWithConfig(filePath, chunksDir string, retentionDays int) (*FileMetricsStore, error) {
	if retentionDays <= 0 {
		retentionDays = DefaultCommandMetricsRetentionDays
	}
	store := &FileMetricsStore{
		filePath:      filePath,
		chunksDir:     chunksDir,
		retentionDays: retentionDays,
	}
	store.metrics.Store(make(map[string]*CommandMetrics))

	// Load existing metrics
	if err := store.Load(); err != nil {
		// If file doesn't exist, that's okay - start fresh
		if !fileutil.IsNotExist(err) {
			return nil, errfmt.Newf("failed to load metrics").Wrap(err)
		}
	}

	return store, nil
}

// WindowDate returns the current active window date (YYYY-MM-DD).
func (s *FileMetricsStore) WindowDate() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.windowDate
}

// Load loads metrics from file, automatically performing day-rolling if date has changed.
func (s *FileMetricsStore) Load() error {
	today := time.Now().UTC().Format("2006-01-02")

	var data []byte
	var fileExists bool
	var readErr error
	if _, err := fileutil.Stat(s.filePath); err == nil {
		fileExists = true
		data, readErr = fileutil.ReadFile(s.filePath)
		if readErr != nil {
			if fileutil.IsNotExist(readErr) {
				fileExists = false
			} else {
				return readErr
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !fileExists || len(data) == 0 {
		s.metrics.Store(make(map[string]*CommandMetrics))
		s.windowDate = today
		return nil
	}

	var fileData struct {
		Metrics    map[string]*CommandMetrics `json:"metrics"`
		Updated    time.Time                  `json:"updated"`
		WindowDate string                     `json:"window_date,omitempty"`
	}

	if err := json.Unmarshal(data, &fileData); err != nil {
		s.metrics.Store(make(map[string]*CommandMetrics))
		s.windowDate = today
		return nil
	}

	fileDate := fileData.WindowDate
	if fileDate == "" && !fileData.Updated.IsZero() {
		fileDate = fileData.Updated.UTC().Format("2006-01-02")
	}

	// Check if existing file is from a previous day
	if fileData.Metrics != nil && len(fileData.Metrics) > 0 && fileDate != "" && fileDate != today {
		// Day rollover: archive old metrics into day-rolled chunks
		_ = s.archiveRolledMetrics(fileData.Metrics, fileDate)

		// Reset active metrics for today
		emptyMap := make(map[string]*CommandMetrics)
		s.metrics.Store(emptyMap)
		s.windowDate = today

		// Save fresh empty bounded file for today
		_ = s.saveMetrics(emptyMap, today)
		_ = s.pruneChunks()
		return nil
	}

	if fileData.Metrics == nil {
		s.metrics.Store(make(map[string]*CommandMetrics))
	} else {
		s.metrics.Store(fileData.Metrics)
	}

	if fileDate != "" {
		s.windowDate = fileDate
	} else {
		s.windowDate = today
	}

	return nil
}

// Save saves metrics to file
func (s *FileMetricsStore) Save() error {
	s.mu.Lock()
	windowDate := s.windowDate
	metricsCopy := cloneCommandMetricsMap(s.metrics.Load().(map[string]*CommandMetrics))
	s.mu.Unlock()
	return s.saveMetrics(metricsCopy, windowDate)
}

// saveMetrics saves metrics to file without holding the lock
func (s *FileMetricsStore) saveMetrics(metrics map[string]*CommandMetrics, windowDate string) error {
	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create metrics directory").Wrap(err)
	}

	if windowDate == "" {
		windowDate = time.Now().UTC().Format("2006-01-02")
	}

	fileData := struct {
		Metrics    map[string]*CommandMetrics `json:"metrics"`
		Updated    time.Time                  `json:"updated"`
		WindowDate string                     `json:"window_date"`
	}{
		Metrics:    metrics,
		Updated:    time.Now().UTC(),
		WindowDate: windowDate,
	}

	data, err := json.MarshalIndent(fileData, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal metrics").Wrap(err)
	}

	// Write to temp file first, then rename (atomic write)
	tmpFile := s.filePath + ".tmp"
	if err := fileutil.WriteFile(tmpFile, data, paths.FilePerm644); err != nil {
		_ = fileutil.Remove(tmpFile)
		return errfmt.Newf("failed to write metrics file").Wrap(err)
	}

	if err := fileutil.Rename(tmpFile, s.filePath); err != nil {
		_ = fileutil.Remove(tmpFile)
		return errfmt.Newf("failed to rename metrics file").Wrap(err)
	}

	return nil
}

// RollDay rolls the current metrics under the given dateStr into chunks and resets for the current day.
func (s *FileMetricsStore) RollDay(dateStr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.metrics.Load().(map[string]*CommandMetrics)
	if len(current) > 0 {
		targetDate := dateStr
		if targetDate == "" {
			targetDate = s.windowDate
		}
		if targetDate == "" {
			targetDate = time.Now().UTC().Format("2006-01-02")
		}
		if err := s.archiveRolledMetrics(current, targetDate); err != nil {
			return err
		}
	}

	today := time.Now().UTC().Format("2006-01-02")
	emptyMap := make(map[string]*CommandMetrics)
	s.metrics.Store(emptyMap)
	s.windowDate = today
	_ = s.saveMetrics(emptyMap, today)
	_ = s.pruneChunks()
	return nil
}

// archiveRolledMetrics writes a day's aggregated metrics to chunksDir and appends timeseries points.
func (s *FileMetricsStore) archiveRolledMetrics(metrics map[string]*CommandMetrics, dateStr string) error {
	if len(metrics) == 0 || s.chunksDir == "" {
		return nil
	}
	if err := fileutil.MkdirAll(s.chunksDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create chunks directory").Wrap(err)
	}

	compactDate := strings.ReplaceAll(dateStr, "-", "")
	chunkFileName := fmt.Sprintf("command_metrics_%s.json", compactDate)
	chunkFilePath := filepath.Join(s.chunksDir, chunkFileName)

	fileData := struct {
		Metrics    map[string]*CommandMetrics `json:"metrics"`
		Updated    time.Time                  `json:"updated"`
		WindowDate string                     `json:"window_date"`
	}{
		Metrics:    metrics,
		Updated:    time.Now().UTC(),
		WindowDate: dateStr,
	}

	data, err := json.MarshalIndent(fileData, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal rolled metrics").Wrap(err)
	}

	tmpFile := chunkFilePath + ".tmp"
	if err := fileutil.WriteFile(tmpFile, data, paths.FilePerm644); err != nil {
		_ = fileutil.Remove(tmpFile)
		return errfmt.Newf("failed to write rolled chunk file").Wrap(err)
	}

	if err := fileutil.Rename(tmpFile, chunkFilePath); err != nil {
		_ = fileutil.Remove(tmpFile)
		return errfmt.Newf("failed to rename rolled chunk file").Wrap(err)
	}

	// Write base+delta timeseries points matching timeseries architecture
	s.writeTimeseriesPoints(metrics, dateStr)
	return nil
}

// writeTimeseriesPoints appends timeseries samples for command metrics into .chunk files.
func (s *FileMetricsStore) writeTimeseriesPoints(m map[string]*CommandMetrics, dateStr string) {
	if s.chunksDir == "" || len(m) == 0 {
		return
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		t = time.Now().UTC()
	}

	var totalInvocations, totalFailures, totalTimeouts int64
	for cmdName, cmdM := range m {
		totalInvocations += int64(cmdM.InvocationCount)
		totalFailures += int64(cmdM.FailureCount)
		totalTimeouts += int64(cmdM.TimeoutCount)

		series := fmt.Sprintf("cmd_%s", sanitizeSeriesName(cmdName))
		_ = writeTimeSeriesPoint(s.chunksDir, series, t, int64(cmdM.InvocationCount))
	}

	_ = writeTimeSeriesPoint(s.chunksDir, "total_invocations", t, totalInvocations)
	_ = writeTimeSeriesPoint(s.chunksDir, "total_failures", t, totalFailures)
	_ = writeTimeSeriesPoint(s.chunksDir, "total_timeouts", t, totalTimeouts)
}

// writeTimeSeriesPoint writes a binary delta-encoded sample to .chunk file matching timeseries architecture.
func writeTimeSeriesPoint(dir, series string, pTime time.Time, value int64) error {
	if dir == "" || series == "" {
		return nil
	}
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return err
	}

	chunkDur := 24 * time.Hour
	start := pTime.UTC().Truncate(chunkDur)
	baseUnix := start.Unix()

	chunkName := fmt.Sprintf("%s_%s.chunk", sanitizeSeriesName(series), start.Format("20060102T1504"))
	chunkPath := filepath.Join(dir, chunkName)

	f, err := fileutil.OpenFile(chunkPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, paths.FilePerm644)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)

	dt := pTime.Unix() - baseUnix
	if dt < 0 {
		dt = 0
	}
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutVarint(buf[:], dt)
	if _, err := w.Write(buf[:n]); err != nil {
		return err
	}

	// ZigZag encode value
	u := uint64(value)
	ux := (u << 1) ^ (u >> 63)
	n = binary.PutUvarint(buf[:], ux)
	if _, err := w.Write(buf[:n]); err != nil {
		return err
	}

	return w.Flush()
}

func sanitizeSeriesName(s string) string {
	res := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			res = append(res, r)
		} else {
			res = append(res, '_')
		}
	}
	return string(res)
}

// pruneChunks removes day-rolled json chunks and .chunk files older than retentionDays.
func (s *FileMetricsStore) pruneChunks() error {
	if s.retentionDays <= 0 || s.chunksDir == "" {
		return nil
	}
	cutoff := time.Now().UTC().Add(-time.Duration(s.retentionDays) * 24 * time.Hour)
	entries, err := fileutil.ReadDir(s.chunksDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := filepath.Ext(name)
		if ext != ".chunk" && !strings.HasPrefix(name, "command_metrics_") {
			continue
		}
		path := filepath.Join(s.chunksDir, name)
		info, err := fileutil.Stat(path)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = fileutil.Remove(path)
		}
	}
	return nil
}

// PruneOldChunks explicitly triggers chunk pruning.
func (s *FileMetricsStore) PruneOldChunks() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pruneChunks()
}

// RecordCommandExecution records a command execution metric, rolling day if date has changed.
func (s *FileMetricsStore) RecordCommandExecution(metric *CommandMetric) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ts := metric.Timestamp.UTC()
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	today := ts.Format("2006-01-02")

	// Check if date has rolled over since windowDate
	if s.windowDate != "" && s.windowDate != today {
		current := s.metrics.Load().(map[string]*CommandMetrics)
		if len(current) > 0 {
			_ = s.archiveRolledMetrics(current, s.windowDate)
		}
		s.metrics.Store(make(map[string]*CommandMetrics))
		s.windowDate = today
		_ = s.pruneChunks()
	} else if s.windowDate == "" {
		s.windowDate = today
	}

	current := s.metrics.Load().(map[string]*CommandMetrics)
	newMetrics := cloneCommandMetricsMap(current)

	cmdMetrics, exists := newMetrics[metric.NormalizedCmd]
	if !exists {
		cmdMetrics = &CommandMetrics{
			Command:         metric.Command,
			NormalizedCmd:   metric.NormalizedCmd,
			FirstSeen:       metric.Timestamp,
			FastestDuration: metric.Duration,
			SlowestDuration: metric.Duration,
		}
		newMetrics[metric.NormalizedCmd] = cmdMetrics
	}

	// Update metrics
	cmdMetrics.InvocationCount++
	cmdMetrics.LastSeen = metric.Timestamp

	if metric.Success {
		cmdMetrics.SuccessCount++
	} else {
		cmdMetrics.FailureCount++
	}

	if metric.TimedOut {
		cmdMetrics.TimeoutCount++
	}

	// Update duration stats
	if metric.Duration < cmdMetrics.FastestDuration || cmdMetrics.FastestDuration == 0 {
		cmdMetrics.FastestDuration = metric.Duration
	}
	if metric.Duration > cmdMetrics.SlowestDuration {
		cmdMetrics.SlowestDuration = metric.Duration
	}

	// Calculate baseline (median or average of successful runs)
	if metric.Success && !metric.TimedOut {
		if cmdMetrics.AvgDuration == 0 {
			cmdMetrics.AvgDuration = metric.Duration
		} else {
			//nolint:gocritic // Documenting EMA formula for clarity
			cmdMetrics.AvgDuration = time.Duration(float64(cmdMetrics.AvgDuration)*0.9 + float64(metric.Duration)*0.1)
		}
		cmdMetrics.BaselineDuration = cmdMetrics.AvgDuration
	}

	// Update memory stats
	if metric.MaxMemoryBytes > cmdMetrics.MaxMemoryBytes {
		cmdMetrics.MaxMemoryBytes = metric.MaxMemoryBytes
	}
	if cmdMetrics.AvgMemoryBytes == 0 {
		cmdMetrics.AvgMemoryBytes = metric.MaxMemoryBytes
	} else if metric.MaxMemoryBytes > 0 {
		cmdMetrics.AvgMemoryBytes = uint64(float64(cmdMetrics.AvgMemoryBytes)*0.9 + float64(metric.MaxMemoryBytes)*0.1)
	}

	// Calculate rates
	if cmdMetrics.InvocationCount > 0 {
		cmdMetrics.ErrorRate = float64(cmdMetrics.FailureCount) / float64(cmdMetrics.InvocationCount) * 100
		cmdMetrics.TimeoutRate = float64(cmdMetrics.TimeoutCount) / float64(cmdMetrics.InvocationCount) * 100
	}

	s.metrics.Store(newMetrics)
	metricsCopy := cloneCommandMetricsMap(newMetrics)
	windowDate := s.windowDate

	goroutinelabels.StartNamedGoroutine("cli-metrics-flusher", "flush CLI metrics to storage", func() {
		_ = s.saveMetrics(metricsCopy, windowDate)
	})

	return nil
}

// GetCommandMetrics gets metrics for a specific command from the current active window.
func (s *FileMetricsStore) GetCommandMetrics(command string) (*CommandMetrics, error) {
	c := s.metrics.Load().(map[string]*CommandMetrics)
	metrics, exists := c[command]
	if !exists {
		return nil, nil
	}

	// Return a copy
	result := *metrics
	return &result, nil
}

// GetAllMetrics gets all command metrics for the current active window (today).
func (s *FileMetricsStore) GetAllMetrics() (map[string]*CommandMetrics, error) {
	c := s.metrics.Load().(map[string]*CommandMetrics)
	return cloneCommandMetricsMap(c), nil
}

// GetMetricsForDate gets command metrics for a specific date (YYYY-MM-DD or YYYYMMDD).
func (s *FileMetricsStore) GetMetricsForDate(dateStr string) (map[string]*CommandMetrics, error) {
	normalizedDate := strings.ReplaceAll(dateStr, "-", "")
	todayCompact := strings.ReplaceAll(s.WindowDate(), "-", "")

	if normalizedDate == todayCompact {
		return s.GetAllMetrics()
	}

	if s.chunksDir == "" {
		return make(map[string]*CommandMetrics), nil
	}

	chunkFile := filepath.Join(s.chunksDir, fmt.Sprintf("command_metrics_%s.json", normalizedDate))
	data, err := fileutil.ReadFile(chunkFile)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return make(map[string]*CommandMetrics), nil
		}
		return nil, errfmt.Newf("failed to read rolled metrics for %s", dateStr).Wrap(err)
	}

	var fileData struct {
		Metrics map[string]*CommandMetrics `json:"metrics"`
	}
	if err := json.Unmarshal(data, &fileData); err != nil {
		return nil, errfmt.Newf("failed to parse rolled metrics for %s", dateStr).Wrap(err)
	}
	if fileData.Metrics == nil {
		return make(map[string]*CommandMetrics), nil
	}
	return fileData.Metrics, nil
}

// GetAllTimeMetrics aggregates metrics across all historical day-rolled chunks and the current window.
func (s *FileMetricsStore) GetAllTimeMetrics() (map[string]*CommandMetrics, error) {
	result := cloneCommandMetricsMap(s.metrics.Load().(map[string]*CommandMetrics))

	if s.chunksDir == "" {
		return result, nil
	}

	entries, err := os.ReadDir(s.chunksDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "command_metrics_") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		chunkPath := filepath.Join(s.chunksDir, e.Name())
		data, err := fileutil.ReadFile(chunkPath)
		if err != nil {
			continue
		}
		var fileData struct {
			Metrics map[string]*CommandMetrics `json:"metrics"`
		}
		if err := json.Unmarshal(data, &fileData); err != nil || fileData.Metrics == nil {
			continue
		}
		mergeCommandMetrics(result, fileData.Metrics)
	}

	return result, nil
}

// GetMetricsSince aggregates command metrics from the past duration d up to now.
func (s *FileMetricsStore) GetMetricsSince(d time.Duration) (map[string]*CommandMetrics, error) {
	result := cloneCommandMetricsMap(s.metrics.Load().(map[string]*CommandMetrics))
	if s.chunksDir == "" {
		return result, nil
	}

	cutoff := time.Now().UTC().Add(-d)
	cutoffDateStr := cutoff.Format("20060102")

	entries, err := os.ReadDir(s.chunksDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "command_metrics_") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		datePart := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "command_metrics_"), ".json")
		if datePart < cutoffDateStr {
			continue
		}

		chunkPath := filepath.Join(s.chunksDir, e.Name())
		data, err := fileutil.ReadFile(chunkPath)
		if err != nil {
			continue
		}
		var fileData struct {
			Metrics map[string]*CommandMetrics `json:"metrics"`
		}
		if err := json.Unmarshal(data, &fileData); err != nil || fileData.Metrics == nil {
			continue
		}
		mergeCommandMetrics(result, fileData.Metrics)
	}

	return result, nil
}

// ListRolledDates returns a sorted list of dates (YYYY-MM-DD) for which day-rolled chunks exist.
func (s *FileMetricsStore) ListRolledDates() ([]string, error) {
	if s.chunksDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(s.chunksDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var dates []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "command_metrics_") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		datePart := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "command_metrics_"), ".json")
		if t, err := time.Parse("20060102", datePart); err == nil {
			dates = append(dates, t.Format("2006-01-02"))
		}
	}
	sort.Strings(dates)
	return dates, nil
}

// mergeCommandMetrics merges src metrics into dst map.
func mergeCommandMetrics(dst, src map[string]*CommandMetrics) {
	for k, v := range src {
		existing, ok := dst[k]
		if !ok {
			vCopy := *v
			dst[k] = &vCopy
			continue
		}

		oldCount := existing.InvocationCount
		newCount := oldCount + v.InvocationCount
		existing.InvocationCount = newCount
		existing.SuccessCount += v.SuccessCount
		existing.FailureCount += v.FailureCount
		existing.TimeoutCount += v.TimeoutCount

		if v.FastestDuration > 0 && (v.FastestDuration < existing.FastestDuration || existing.FastestDuration == 0) {
			existing.FastestDuration = v.FastestDuration
		}
		if v.SlowestDuration > existing.SlowestDuration {
			existing.SlowestDuration = v.SlowestDuration
		}

		if newCount > 0 {
			combinedDuration := int64(existing.AvgDuration)*int64(oldCount) + int64(v.AvgDuration)*int64(v.InvocationCount)
			existing.AvgDuration = time.Duration(combinedDuration / int64(newCount))
			existing.BaselineDuration = existing.AvgDuration

			existing.ErrorRate = float64(existing.FailureCount) / float64(newCount) * 100
			existing.TimeoutRate = float64(existing.TimeoutCount) / float64(newCount) * 100
		}

		if v.MaxMemoryBytes > existing.MaxMemoryBytes {
			existing.MaxMemoryBytes = v.MaxMemoryBytes
		}
		if newCount > 0 && (existing.AvgMemoryBytes > 0 || v.AvgMemoryBytes > 0) {
			combinedMem := existing.AvgMemoryBytes*uint64(oldCount) + v.AvgMemoryBytes*uint64(v.InvocationCount)
			existing.AvgMemoryBytes = combinedMem / uint64(newCount)
		}

		if !v.FirstSeen.IsZero() && (existing.FirstSeen.IsZero() || v.FirstSeen.Before(existing.FirstSeen)) {
			existing.FirstSeen = v.FirstSeen
		}
		if v.LastSeen.After(existing.LastSeen) {
			existing.LastSeen = v.LastSeen
		}
	}
}
