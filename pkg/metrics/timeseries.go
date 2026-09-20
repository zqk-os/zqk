package metrics

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"math"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// This package provides chunked, base+delta encoded time series storage for numeric
// metrics (object_volume, health gauges). See DATA_STORAGE_PRODUCTION_ROADMAP.md §3.
// object-count-report is wired: recordObjectVolumeMetrics writes to .zqk/metrics/object_volume/;
// pruneObjectVolumeChunks retains N days from --metrics-chunk-retention-days / METRICS_CHUNK_RETENTION_DAYS (see object_count_report.go). For new series (e.g. health), follow
// PRE_CHANGE_CHECKLIST (bounded workers, streaming I/O) and reuse this writer/reader.

// TimeSeriesPoint is a single sample in a time series.
type TimeSeriesPoint struct {
	Ts    time.Time
	Value int64
}

// TimeSeriesConfig controls how a time series is chunked and stored on disk.
type TimeSeriesConfig struct {
	// ChunkDuration controls how much wall-clock time each chunk file covers (e.g. 1h).
	ChunkDuration time.Duration
	// Dir is the base directory where chunk files will be written.
	Dir string
	// Series is an identifier for this series (e.g. "object_volume/audit_event").
	Series string
}

// TimeSeriesWriter appends points to a chunked, base+delta encoded series on disk.
type TimeSeriesWriter struct {
	cfg         TimeSeriesConfig
	curStart    time.Time // chunk start (UTC, truncated to ChunkDuration)
	baseUnix    int64     // base unix seconds for current chunk
	prevValue   int64     // previous value in this chunk (for delta encoding)
	file        *fileutil.File
	w           *bufio.Writer
	initialized bool
}

// NewTimeSeriesWriter creates a new writer for the given config. Chunks are
// created lazily on first Append.
func NewTimeSeriesWriter(cfg TimeSeriesConfig) (*TimeSeriesWriter, error) {
	if cfg.ChunkDuration <= 0 {
		return nil, errfmt.Errorf("chunkDuration must be > 0")
	}
	if cfg.Dir == emptyValue || cfg.Series == emptyValue {
		return nil, errfmt.Errorf("dir and series are required")
	}
	return &TimeSeriesWriter{cfg: cfg}, nil
}

// Append appends a single point to the series. Points should be appended in
// non-decreasing timestamp order.
func (w *TimeSeriesWriter) Append(p TimeSeriesPoint) error {
	if !w.initialized {
		if err := w.startChunk(p.Ts); err != nil {
			return err
		}
		w.initialized = true
	} else if !sameChunk(w.curStart, w.cfg.ChunkDuration, p.Ts) {
		if err := w.rotateChunk(p.Ts); err != nil {
			return err
		}
	}

	// Encode delta time (seconds from base) and delta value using varint.
	dt := p.Ts.Unix() - w.baseUnix
	if dt < 0 {
		dt = 0
	}
	// Time delta
	if err := writeVarint(w.w, dt); err != nil {
		return err
	}
	// Value delta (could be negative, so use zigzag)
	dv := p.Value - w.prevValue
	w.prevValue = p.Value
	if err := writeZigZagVarint(w.w, dv); err != nil {
		return err
	}
	return nil
}

// Close flushes and closes the current chunk file.
func (w *TimeSeriesWriter) Close() error {
	if w.w != nil {
		if err := w.w.Flush(); err != nil {
			_ = w.file.Close()
			return err
		}
	}
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

func (w *TimeSeriesWriter) startChunk(ts time.Time) error {
	start := ts.UTC().Truncate(w.cfg.ChunkDuration)
	w.curStart = start
	w.baseUnix = start.Unix()

	if err := fileutil.EnsureDir(w.cfg.Dir); err != nil {
		return err
	}
	name := fmt.Sprintf("%s_%s.chunk", sanitizeSeriesName(w.cfg.Series), start.Format("20060102T1504"))
	path := filepath.Join(w.cfg.Dir, name)
	f, err := fileutil.OpenFile(path, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_APPEND, paths.FilePerm600)
	if err != nil {
		return err
	}
	w.file = f
	w.w = bufio.NewWriter(f)
	w.prevValue = 0
	return nil
}

func (w *TimeSeriesWriter) rotateChunk(next time.Time) error {
	if err := w.Close(); err != nil {
		return err
	}
	w.file, w.w = nil, nil
	w.initialized = false
	return w.startChunk(next)
}

func sameChunk(start time.Time, d time.Duration, ts time.Time) bool {
	return ts.UTC().Truncate(d).Equal(start)
}

// SanitizeSeriesName returns a filesystem-safe series name.
func SanitizeSeriesName(s string) string {
	// Simple filesystem-safe replacement; can be extended as needed.
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

func sanitizeSeriesName(s string) string {
	return SanitizeSeriesName(s)
}

// writeVarint encodes x as unsigned varint.
func writeVarint(w *bufio.Writer, x int64) error {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutVarint(buf[:], x)
	_, err := w.Write(buf[:n])
	return err
}

// writeZigZagVarint encodes a signed integer using zigzag + varint.
func writeZigZagVarint(w *bufio.Writer, x int64) error {
	// ZigZag encode (same shape as protobuf PutUvarint after zigzag).
	u := uint64(x) //nolint:gosec // G115: intentional two's-complement bit pattern for zigzag (all int64 values)
	ux := (u << 1) ^ (u >> 63)
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], ux)
	_, err := w.Write(buf[:n])
	return err
}

// TimeSeriesReader reads points from chunked time series files.
type TimeSeriesReader struct {
	cfg TimeSeriesConfig
}

// NewTimeSeriesReader creates a new reader for the given series.
func NewTimeSeriesReader(cfg TimeSeriesConfig) *TimeSeriesReader {
	return &TimeSeriesReader{cfg: cfg}
}

// Range iterates over points in [from, to], calling fn for each point.
func (r *TimeSeriesReader) Range(from, to time.Time, fn func(TimeSeriesPoint) error) error {
	if r.cfg.ChunkDuration <= 0 {
		return errfmt.Errorf("chunkDuration must be > 0")
	}
	start := from.UTC().Truncate(r.cfg.ChunkDuration)
	end := to.UTC().Truncate(r.cfg.ChunkDuration)

	for cur := start; !cur.After(end); cur = cur.Add(r.cfg.ChunkDuration) {
		name := fmt.Sprintf("%s_%s.chunk", sanitizeSeriesName(r.cfg.Series), cur.Format("20060102T1504"))
		path := filepath.Join(r.cfg.Dir, name)
		if err := r.readChunk(path, cur, from, to, fn); err != nil {
			if fileutil.IsNotExist(err) {
				continue
			}
			return err
		}
	}
	return nil
}

func (r *TimeSeriesReader) readChunk(path string, chunkStart time.Time, from, to time.Time, fn func(TimeSeriesPoint) error) error {
	f, err := fileutil.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	baseUnix := chunkStart.Unix()
	s := bufio.NewReader(f)
	var (
		prevValue int64
	)
	for {
		dt, err := binary.ReadVarint(s)
		if err != nil {
			// EOF or other error ends the loop; caller handles EOF as success.
			break
		}
		ux, err := binary.ReadUvarint(s)
		if err != nil {
			break
		}
		// Decode zigzag (protobuf DecodeZigZag64 equivalent).
		hi := ux >> 1
		if hi > uint64(math.MaxInt64) {
			return errfmt.Errorf("timeseries: zigzag delta overflow (corrupt chunk?)")
		}
		sign := int64(0)
		if ux&1 != 0 {
			sign = -1
		}
		vDelta := int64(hi) ^ sign
		prevValue += vDelta
		ts := time.Unix(baseUnix+dt, 0).UTC()
		if ts.Before(from) || ts.After(to) {
			continue
		}
		if err := fn(TimeSeriesPoint{Ts: ts, Value: prevValue}); err != nil {
			return err
		}
	}
	return nil
}
