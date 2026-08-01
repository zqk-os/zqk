package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
)

// FileMetricsStore implements MetricsStore using a JSON file
type FileMetricsStore struct {
	filePath string
	metrics  atomic.Value // holds map[string]*CommandMetrics
	mu       sync.Mutex
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

// NewFileMetricsStore creates a new file-based metrics store
func NewFileMetricsStore(filePath string) (*FileMetricsStore, error) {
	store := &FileMetricsStore{
		filePath: filePath,
	}
	store.metrics.Store(make(map[string]*CommandMetrics))

	// Load existing metrics
	if err := store.Load(); err != nil {
		// If file doesn't exist, that's okay - start fresh
		if !os.IsNotExist(err) {
			return nil, errfmt.Newf("failed to load metrics").Wrap(err)
		}
	}

	return store, nil
}

// Load loads metrics from file
func (s *FileMetricsStore) Load() error {
	// Check if file exists and read it (outside lock for I/O)
	var data []byte
	var fileExists bool
	var readErr error
	if _, err := os.Stat(s.filePath); err == nil {
		fileExists = true
		data, readErr = os.ReadFile(s.filePath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
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
		return nil
	}

	var fileData struct {
		Metrics map[string]*CommandMetrics `json:"metrics"`
	}

	if err := json.Unmarshal(data, &fileData); err != nil {
		s.metrics.Store(make(map[string]*CommandMetrics))
		return nil
	}

	if fileData.Metrics == nil {
		s.metrics.Store(make(map[string]*CommandMetrics))
	} else {
		s.metrics.Store(fileData.Metrics)
	}
	return nil
}

// Save saves metrics to file
func (s *FileMetricsStore) Save() error {
	metricsCopy := cloneCommandMetricsMap(s.metrics.Load().(map[string]*CommandMetrics))
	return s.saveMetrics(metricsCopy)
}

// saveMetrics saves metrics to file without holding the lock
func (s *FileMetricsStore) saveMetrics(metrics map[string]*CommandMetrics) error {
	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create metrics directory").Wrap(err)
	}

	fileData := struct {
		Metrics map[string]*CommandMetrics `json:"metrics"`
		Updated time.Time                  `json:"updated"`
	}{
		Metrics: metrics,
		Updated: time.Now(),
	}

	data, err := json.MarshalIndent(fileData, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal metrics").Wrap(err)
	}

	// Write to temp file first, then rename (atomic write)
	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, paths.FilePerm644); err != nil {
		// Clean up temp file on error
		_ = os.Remove(tmpFile)
		return errfmt.Newf("failed to write metrics file").Wrap(err)
	}

	if err := os.Rename(tmpFile, s.filePath); err != nil {
		// Clean up temp file on error
		_ = os.Remove(tmpFile)
		return errfmt.Newf("failed to rename metrics file").Wrap(err)
	}

	return nil
}

// RecordCommandExecution records a command execution metric
func (s *FileMetricsStore) RecordCommandExecution(metric *CommandMetric) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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
	// CRITICAL: Only use successful, non-timed-out runs for baseline calculation
	// Timed-out runs should NOT affect the baseline, as they represent failures, not normal execution time
	if metric.Success && !metric.TimedOut {
		// Recalculate average (simplified - in production, might want to track sum)
		// For now, use exponential moving average
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

	// Save to disk (outside lock, but since we are holding s.mu.Lock(), it's safe to spawn a goroutine or just save synchronously.
	// We'll save synchronously to match the original behavior, but wait, the original didn't hold the lock during saveMetrics)
	// Actually we should defer Unlock or just unlock before saving.
	// But it's easier to unlock using a func
	go func() {
		_ = s.saveMetrics(metricsCopy)
	}()

	return nil
}

// GetCommandMetrics gets metrics for a specific command
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

// GetAllMetrics gets all command metrics
func (s *FileMetricsStore) GetAllMetrics() (map[string]*CommandMetrics, error) {
	c := s.metrics.Load().(map[string]*CommandMetrics)
	return cloneCommandMetricsMap(c), nil
}
