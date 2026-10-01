package congruence

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/operational"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// RecordObjectVolumeMetrics appends a single time-series sample per kind using the
// compact metrics timeseries writer. Best-effort only; failures are logged.
// After writing, prunes chunk files older than retentionDays.
func RecordObjectVolumeMetrics(projectRoot string, report *operational.CongruenceReport, logger logging.Logger, retentionDays int) {
	if report == nil || projectRoot == "" {
		return
	}
	metricsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, paths.MetricsObjectVolumeSubdir)
	ts := report.GeneratedAt

	for kind, count := range report.ObjectCountByKind {
		cfg := metrics.TimeSeriesConfig{
			ChunkDuration: time.Hour,
			Dir:           metricsDir,
			Series:        fmt.Sprintf("object_volume/%s", kind),
		}
		w, err := metrics.NewTimeSeriesWriter(cfg)
		if err != nil {
			logging.Fluent(logger).Debug("Object volume metrics: writer init failed").
				Kind(kind).
				WithError(err).
				Log()
			continue
		}
		if err := w.Append(metrics.TimeSeriesPoint{
			Ts:    ts,
			Value: int64(count),
		}); err != nil {
			logging.Fluent(logger).Debug("Object volume metrics: append failed").
				Kind(kind).
				WithError(err).
				Log()
			if closeErr := w.Close(); closeErr != nil {
				logging.Fluent(logger).Debug("Object volume metrics: close after append failure failed").WithError(closeErr).Log()
			}
			continue
		}
		if err := w.Close(); err != nil {
			logging.Fluent(logger).Debug("Object volume metrics: close failed").
				Kind(kind).
				WithError(err).
				Log()
		}
	}

	PruneObjectVolumeChunks(metricsDir, retentionDays, logger)
}

// RecordStreamVolumeMetrics appends a time-series sample per high-volume stream kind
// using the high-volume event cache as the source of truth.
func RecordStreamVolumeMetrics(projectRoot string, logger logging.Logger, retentionDays int) {
	if projectRoot == "" {
		return
	}
	cache := storage.GetGlobalHighVolumeEventCache()
	if cache == nil {
		return
	}
	metricsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, paths.MetricsStreamVolumeSubdir)
	ts := time.Now().UTC()

	streamKinds := storage.StreamStorageEnabledKindsList()
	for _, kind := range streamKinds {
		count := cache.CountByKind(kind)
		cfg := metrics.TimeSeriesConfig{
			ChunkDuration: time.Hour,
			Dir:           metricsDir,
			Series:        fmt.Sprintf("stream_volume/%s", kind),
		}
		w, err := metrics.NewTimeSeriesWriter(cfg)
		if err != nil {
			logging.Fluent(logger).Debug("Stream volume metrics: writer init failed").
				Kind(kind).
				WithError(err).
				Log()
			continue
		}
		if err := w.Append(metrics.TimeSeriesPoint{
			Ts:    ts,
			Value: int64(count),
		}); err != nil {
			logging.Fluent(logger).Debug("Stream volume metrics: append failed").
				Kind(kind).
				WithError(err).
				Log()
			if closeErr := w.Close(); closeErr != nil {
				logging.Fluent(logger).Debug("Stream volume metrics: close after append failure failed").WithError(closeErr).Log()
			}
			continue
		}
		if err := w.Close(); err != nil {
			logging.Fluent(logger).Debug("Stream volume metrics: close failed").
				Kind(kind).
				WithError(err).
				Log()
		}
	}
	PruneObjectVolumeChunks(metricsDir, retentionDays, logger)
}

// RecordFilesystemSnapshotMetrics appends time-series samples for filesystem project snapshot metrics.
func RecordFilesystemSnapshotMetrics(projectRoot string, snap *operational.FilesystemProjectSnapshot, logger logging.Logger, retentionDays int) {
	if snap == nil || projectRoot == "" {
		return
	}
	metricsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, paths.MetricsFilesystemSnapshotSubdir)
	ts, err := time.Parse(time.RFC3339Nano, snap.GeneratedAt)
	if err != nil {
		ts = time.Now().UTC()
	}

	appendFSPoint := func(series string, val int64) {
		cfg := metrics.TimeSeriesConfig{
			ChunkDuration: time.Hour,
			Dir:           metricsDir,
			Series:        series,
		}
		w, werr := metrics.NewTimeSeriesWriter(cfg)
		if werr != nil {
			logging.Fluent(logger).Debug("Filesystem snapshot metrics: writer init failed").
				String("series", series).
				WithError(werr).
				Log()
			return
		}
		if err := w.Append(metrics.TimeSeriesPoint{Ts: ts, Value: val}); err != nil {
			logging.Fluent(logger).Debug("Filesystem snapshot metrics: append failed").
				String("series", series).
				WithError(err).
				Log()
			if closeErr := w.Close(); closeErr != nil {
				logging.Fluent(logger).Debug("Filesystem snapshot metrics: close after append failure failed").WithError(closeErr).Log()
			}
			return
		}
		if err := w.Close(); err != nil {
			logging.Fluent(logger).Debug("Filesystem snapshot metrics: close failed").
				String("series", series).
				WithError(err).
				Log()
		}
	}

	appendFSPoint("filesystem_snapshot/total_files", snap.TotalFiles)
	appendFSPoint("filesystem_snapshot/total_bytes", snap.TotalBytes)
	if snap.Volume != nil {
		appendFSPoint("filesystem_snapshot/volume_total_bytes", int64OrZero(snap.Volume.TotalBytes))
		appendFSPoint("filesystem_snapshot/volume_available_bytes", int64OrZero(snap.Volume.AvailableBytes))
	}
	for child, st := range snap.ZqkByChild {
		if st == nil {
			continue
		}
		series := fmt.Sprintf("filesystem_snapshot/zqk_%s/files", sanitizeFSSeriesPart(child))
		appendFSPoint(series, st.Files)
	}

	PruneObjectVolumeChunks(metricsDir, retentionDays, logger)
}

// PruneObjectVolumeChunks removes chunk files in dir older than retentionDays (by ModTime).
func PruneObjectVolumeChunks(dir string, retentionDays int, logger logging.Logger) {
	if retentionDays <= 0 {
		return
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return
		}
		logging.Fluent(logger).Debug("Object volume prune: read dir failed").Dir(dir).WithError(err).Log()
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".chunk" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := fileutil.Stat(path)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := fileutil.Remove(path); err != nil {
				logging.Fluent(logger).Debug("Object volume prune: remove failed").Path(path).WithError(err).Log()
			}
		}
	}
}

func int64OrZero(u uint64) int64 {
	const maxInt64 = 1<<63 - 1
	if u > uint64(maxInt64) {
		return maxInt64
	}
	return int64(u)
}

func sanitizeFSSeriesPart(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "unknown"
	}
	return out
}
