package monitors

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/healthcheck"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// streamVolumeMonitor checks recent stream volume for high-volume kinds (currently audit_event)
// using the compact time series written from the high-volume event cache (stream_volume/*).
// This is complementary to object_volume (CAS-backed kinds) and focuses on stream-backed data
// so we can detect accumulation that would not show up in the object ID cache.
type streamVolumeMonitor struct{}

func (m *streamVolumeMonitor) ID() string   { return "stream_volume" }
func (m *streamVolumeMonitor) Name() string { return "Stream volume and accumulation" }

func (m *streamVolumeMonitor) Run(ctx context.Context, projectRoot string) (*healthcheck.Result, error) {
	if projectRoot == emptyValue {
		return &healthcheck.Result{Status: statusOK, Summary: summaryNoProjectRoot}, nil
	}
	metricsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, paths.MetricsStreamVolumeSubdir)
	if _, err := fileutil.Stat(metricsDir); err != nil {
		if fileutil.IsNotExist(err) {
			// Uninitialized metrics must report degraded warning rather than false-green ok (TDE-F-OBS-002).
			return &healthcheck.Result{Status: statusDegraded, Summary: "uninitialized: no stream_volume metrics yet"}, nil
		}
		return &healthcheck.Result{
			Status:  statusDegraded,
			Summary: "stream_volume metrics dir error",
			Details: map[string]any{detailsErrorKey: err.Error()},
		}, nil
	}

	kind := kindAuditEvent
	series := fmt.Sprintf(streamVolumeSeriesFmt, kind)
	cfg := metrics.TimeSeriesConfig{
		ChunkDuration: objectVolumeChunkWindow,
		Dir:           metricsDir,
		Series:        series,
	}
	reader := metrics.NewTimeSeriesReader(cfg)

	now := time.Now().UTC()
	from := now.Add(-sixHourWindow)
	to := now

	var (
		lastSample  *metrics.TimeSeriesPoint
		prevSample  *metrics.TimeSeriesPoint
		sampleCount int
	)
	err := reader.Range(from, to, func(p metrics.TimeSeriesPoint) error {
		if lastSample != nil {
			prevCopy := *lastSample
			prevSample = &prevCopy
		}
		copy := p
		lastSample = &copy
		sampleCount++
		return nil
	})
	if err != nil {
		return &healthcheck.Result{
			Status:  statusDegraded,
			Summary: "failed to read stream_volume metrics",
			Details: map[string]any{detailsErrorKey: err.Error()},
		}, nil
	}

	if lastSample == nil {
		return &healthcheck.Result{Status: statusDegraded, Summary: "uninitialized: no recent stream_volume samples"}, nil
	}

	currentCount := lastSample.Value
	var ratePerHour float64
	if prevSample != nil {
		dt := lastSample.Ts.Sub(prevSample.Ts).Hours()
		if dt > 0 {
			ratePerHour = float64(lastSample.Value-prevSample.Value) / dt
		}
	}

	status := statusOK
	if currentCount > 50000 || ratePerHour > 8000 {
		status = statusFail
	} else if currentCount > 20000 || ratePerHour > 4000 {
		status = statusDegraded
	}

	summary := fmt.Sprintf("stream audit_event count=%d samples=%d rate_per_hour=%.1f", currentCount, sampleCount, ratePerHour)
	details := map[string]any{
		detailsKindKey:      kind,
		detailsCurrentCount: currentCount,
		detailsRatePerHour:  ratePerHour,
		detailsSampleCount:  sampleCount,
	}
	return &healthcheck.Result{Status: status, Summary: summary, Details: details}, nil
}

func init() {
	healthcheck.DefaultRegistry.Register(&streamVolumeMonitor{})
}
