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

// objectVolumeMonitor checks recent object volume for high-volume kinds
// using the compact object_volume time series written by system object-count-report.
//
// NOTE: This is a first pass focused on audit_event only. Before expanding it,
// align thresholds with retention_tolerance config and WAL/maintenance capacity
// (see PRI-218 backlog items).
type objectVolumeMonitor struct{}

func (m *objectVolumeMonitor) ID() string   { return "object_volume" }
func (m *objectVolumeMonitor) Name() string { return "Object volume and accumulation" }

func (m *objectVolumeMonitor) Run(ctx context.Context, projectRoot string) (*healthcheck.Result, error) {
	if projectRoot == emptyValue {
		return &healthcheck.Result{Status: statusOK, Summary: summaryNoProjectRoot}, nil
	}
	metricsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, paths.MetricsObjectVolumeSubdir)
	if _, err := fileutil.Stat(metricsDir); err != nil {
		if fileutil.IsNotExist(err) {
			// Uninitialized metrics must report degraded warning rather than false-green ok (TDE-F-OBS-002).
			return &healthcheck.Result{Status: statusDegraded, Summary: "uninitialized: no object_volume metrics yet"}, nil
		}
		// Treat unexpected filesystem errors as degraded rather than hard failure so
		// healthchk run continues and other monitors can still report.
		return &healthcheck.Result{
			Status:  statusDegraded,
			Summary: "object_volume metrics dir error",
			Details: map[string]any{detailsErrorKey: err.Error()},
		}, nil
	}

	// For now, focus on audit_event only; this is our highest-volume kind and
	// the one with strictest retention tolerance.
	kind := kindAuditEvent
	series := fmt.Sprintf(objectVolumeSeriesFmt, kind)
	cfg := metrics.TimeSeriesConfig{
		ChunkDuration: objectVolumeChunkWindow,
		Dir:           metricsDir,
		Series:        series,
	}
	reader := metrics.NewTimeSeriesReader(cfg)

	// Look at the last 6 hours of samples (if available).
	now := time.Now().UTC()
	from := now.Add(-sixHourWindow)
	to := now

	var (
		lastSample  *metrics.TimeSeriesPoint
		prevSample  *metrics.TimeSeriesPoint
		sampleCount int
	)
	err := reader.Range(from, to, func(p metrics.TimeSeriesPoint) error {
		// Keep only the last two samples in range.
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
		// Treat read errors as degraded with details rather than outright fail so
		// other monitors can still run.
		return &healthcheck.Result{
			Status:  statusDegraded,
			Summary: "failed to read object_volume metrics",
			Details: map[string]any{detailsErrorKey: err.Error()},
		}, nil
	}

	if lastSample == nil {
		return &healthcheck.Result{Status: statusDegraded, Summary: "uninitialized: no recent object_volume samples"}, nil
	}

	currentCount := lastSample.Value
	// Compute a simple accumulation rate per hour between the last two samples, if available.
	var ratePerHour float64
	if prevSample != nil {
		dt := lastSample.Ts.Sub(prevSample.Ts).Hours()
		if dt > 0 {
			ratePerHour = float64(lastSample.Value-prevSample.Value) / dt
		}
	}

	// Very simple thresholds for now; these will be tuned based on retention_tolerance
	// and maintenance capacity:
	//   - ok:     count <= 2k
	//   - degraded: 2k < count <= 5k or ratePerHour > 4000
	//   - fail:  count > 5k or ratePerHour > 8000
	status := statusOK
	if currentCount > 5000 || ratePerHour > 8000 {
		status = statusFail
	} else if currentCount > 2000 || ratePerHour > 4000 {
		status = statusDegraded
	}

	summary := fmt.Sprintf("audit_event count=%d samples=%d rate_per_hour=%.1f", currentCount, sampleCount, ratePerHour)
	details := map[string]any{
		detailsKindKey:      kind,
		detailsCurrentCount: currentCount,
		detailsRatePerHour:  ratePerHour,
		detailsSampleCount:  sampleCount,
	}
	return &healthcheck.Result{Status: status, Summary: summary, Details: details}, nil
}

func init() {
	healthcheck.DefaultRegistry.Register(&objectVolumeMonitor{})
}
