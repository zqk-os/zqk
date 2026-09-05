package metrics

import (
	"path/filepath"
	"testing"
	"time"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestTimeSeries_WriteAndReadRange(t *testing.T) {
	dir := t.TempDir()
	cfg := TimeSeriesConfig{
		ChunkDuration: time.Hour,
		Dir:           dir,
		Series:        "object_volume_audit_event",
	}

	w, err := NewTimeSeriesWriter(cfg)
	if err != nil {
		t.Fatalf("NewTimeSeriesWriter error: %v", err)
	}
	start := time.Unix(1_700_000_000, 0).UTC()
	points := []TimeSeriesPoint{
		{Ts: start, Value: 100},
		{Ts: start.Add(10 * time.Minute), Value: 150},
		{Ts: start.Add(20 * time.Minute), Value: 180},
		{Ts: start.Add(70 * time.Minute), Value: 200}, // next chunk
	}
	for _, p := range points {
		if err := w.Append(p); err != nil {
			t.Fatalf("Append error: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	// sanity: chunk files exist
	files, err := fileutil.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected at least one chunk file, got 0")
	}

	r := NewTimeSeriesReader(cfg)
	var got []TimeSeriesPoint
	from := start.Add(-time.Minute)
	to := start.Add(2 * time.Hour)
	if err := r.Range(from, to, func(p TimeSeriesPoint) error {
		got = append(got, p)
		return nil
	}); err != nil {
		t.Fatalf("Range error: %v", err)
	}
	if len(got) != len(points) {
		t.Fatalf("expected %d points, got %d", len(points), len(got))
	}
	for i := range points {
		if !points[i].Ts.Equal(got[i].Ts) || points[i].Value != got[i].Value {
			t.Fatalf("point %d mismatch: want %+v got %+v", i, points[i], got[i])
		}
	}
}

func TestTimeSeries_SanitizeSeriesName(t *testing.T) {
	name := sanitizeSeriesName("object/volume:audit-event")
	if name != "object_volume_audit_event" {
		t.Fatalf("unexpected sanitized name: %q", name)
	}
	if filepath.Base(name) != name {
		t.Fatalf("sanitizeSeriesName should produce basename-safe string: %q", name)
	}
}
