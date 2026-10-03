package monitors

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/healthcheck"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type volumeMonitorConfig struct {
	name              string
	metricsSubdir     string
	seriesFmt         string
	kind              string
	failThreshold     int64
	degradedThreshold int64
	failRate          float64
	degradedRate      float64
	summaryPrefix     string
}

func evaluateVolumeMonitor(projectRoot string, cfg volumeMonitorConfig) (*healthcheck.Result, error) {
	if projectRoot == emptyValue {
		return &healthcheck.Result{Status: statusOK, Summary: summaryNoProjectRoot}, nil
	}
	metricsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, cfg.metricsSubdir)
	if _, err := fileutil.Stat(metricsDir); err != nil {
		if fileutil.IsNotExist(err) {
			return &healthcheck.Result{Status: statusDegraded, Summary: fmt.Sprintf("uninitialized: no %s metrics yet", cfg.name)}, nil
		}
		return &healthcheck.Result{
			Status:  statusDegraded,
			Summary: fmt.Sprintf("%s metrics dir error", cfg.name),
			Details: map[string]any{detailsErrorKey: err.Error()},
		}, nil
	}

	series := fmt.Sprintf(cfg.seriesFmt, cfg.kind)
	reader := metrics.NewTimeSeriesReader(metrics.TimeSeriesConfig{
		ChunkDuration: objectVolumeChunkWindow,
		Dir:           metricsDir,
		Series:        series,
	})

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
			Summary: fmt.Sprintf("failed to read %s metrics", cfg.name),
			Details: map[string]any{detailsErrorKey: err.Error()},
		}, nil
	}

	if lastSample == nil {
		return &healthcheck.Result{Status: statusDegraded, Summary: fmt.Sprintf("uninitialized: no recent %s samples", cfg.name)}, nil
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
	if currentCount > cfg.failThreshold || ratePerHour > cfg.failRate {
		status = statusFail
	} else if currentCount > cfg.degradedThreshold || ratePerHour > cfg.degradedRate {
		status = statusDegraded
	}

	summary := fmt.Sprintf("%s%s count=%d samples=%d rate_per_hour=%.1f", cfg.summaryPrefix, cfg.kind, currentCount, sampleCount, ratePerHour)
	details := map[string]any{
		detailsKindKey:      cfg.kind,
		detailsCurrentCount: currentCount,
		detailsRatePerHour:  ratePerHour,
		detailsSampleCount:  sampleCount,
	}
	return &healthcheck.Result{Status: status, Summary: summary, Details: details}, nil
}
