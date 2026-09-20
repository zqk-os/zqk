package metrics

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func tsCfg(test *testing.T) (TimeSeriesConfig, time.Time) {
	test.Helper()
	dir := test.TempDir()
	base := time.Date(2024, 1, 2, 0, 30, 0, 0, time.UTC)
	return TimeSeriesConfig{
		ChunkDuration: time.Hour,
		Dir:           filepath.Join(dir, "object_volume"),
		Series:        "object_volume/test",
	}, base
}

func writeSeries(test *testing.T, w *TimeSeriesWriter, base time.Time, values ...int64) {
	test.Helper()
	for i, v := range values {
		if err := w.Append(TimeSeriesPoint{Ts: base.Add(time.Duration(i) * time.Minute), Value: v}); err != nil {
			test.Fatalf("append: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		test.Fatalf("close: %v", err)
	}
}

func TestQueryRangePoints_MatchesWriter(t *testing.T) {
	cfg, base := tsCfg(t)
	w, err := NewTimeSeriesWriter(cfg)
	if err != nil {
		t.Fatalf("writer: %v", err)
	}
	writeSeries(t, w, base, 10, 20, 30, 40)

	got := map[time.Time]int64{}
	err = QueryRangePoints(context.Background(), cfg, base, base.Add(5*time.Minute), func(p TimeSeriesPoint) error {
		got[p.Ts] = p.Value
		return nil
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d points, want 4", len(got))
	}
	if got[base.Add(2*time.Minute)] != 30 {
		t.Fatalf("point at +2m = %d, want 30", got[base.Add(2*time.Minute)])
	}
}

func TestRollup_BucketsCorrectly(t *testing.T) {
	cfg, base := tsCfg(t)
	w, err := NewTimeSeriesWriter(cfg)
	if err != nil {
		t.Fatalf("writer: %v", err)
	}
	writeSeries(t, w, base, 5, 15, 25, 35, 45, 55)

	res, err := Rollup(context.Background(), cfg, base, base.Add(5*time.Minute), 15*time.Minute)
	if err != nil {
		t.Fatalf("rollup: %v", err)
	}
	if len(res.Buckets) != 1 {
		t.Fatalf("got %d buckets, want 1", len(res.Buckets))
	}
	b := res.Buckets[0]
	if b.Count != 6 {
		t.Fatalf("count = %d, want 6", b.Count)
	}
	if b.Min != 5 || b.Max != 55 {
		t.Fatalf("min/max = %d/%d, want 5/55", b.Min, b.Max)
	}
	if b.Sum != 180 {
		t.Fatalf("sum = %d, want 180", b.Sum)
	}
	if b.Avg != 30 {
		t.Fatalf("avg = %f, want 30", b.Avg)
	}
	wantStart := base.UTC().Truncate(15 * time.Minute)
	if !b.Start.Equal(wantStart) {
		t.Fatalf("bucket start %v, want %v", b.Start, wantStart)
	}
}

func TestRollup_EmptyRangeIsZeroBucketsNotError(t *testing.T) {
	cfg, base := tsCfg(t)
	res, err := Rollup(context.Background(), cfg, base, base.Add(5*time.Minute), time.Minute)
	if err != nil {
		t.Fatalf("rollup on empty range: %v", err)
	}
	if len(res.Buckets) != 0 {
		t.Fatalf("got %d buckets on empty range, want 0", len(res.Buckets))
	}
}

func TestRollup_InvalidStep(t *testing.T) {
	cfg, base := tsCfg(t)
	if _, err := Rollup(context.Background(), cfg, base, base.Add(time.Hour), 0); err == nil {
		t.Fatal("expected error for step <= 0")
	}
	if _, err := Rollup(context.Background(), cfg, base, base.Add(time.Hour), -time.Minute); err == nil {
		t.Fatal("expected error for negative step")
	}
}

func TestListSeries_MissingDirIsEmpty(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")
	series, err := ListSeries(missing)
	if err != nil {
		t.Fatalf("list on missing dir: %v", err)
	}
	if len(series) != 0 {
		t.Fatalf("got %d series, want 0", len(series))
	}
}

func TestListSeries_RescannedAfterWrite(t *testing.T) {
	cfg, base := tsCfg(t)
	w, err := NewTimeSeriesWriter(cfg)
	if err != nil {
		t.Fatalf("writer: %v", err)
	}
	writeSeries(t, w, base, 1, 2, 3)

	series, err := ListSeries(cfg.Dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, s := range series {
		if s == "object_volume/test" {
			found = true
		}
	}
	if !found {
		t.Fatalf("series %q not found in %v", "object_volume/test", series)
	}
}

func TestAmbientFeed_BoundedTail(t *testing.T) {
	cfg, base := tsCfg(t)
	w, err := NewTimeSeriesWriter(cfg)
	if err != nil {
		t.Fatalf("writer: %v", err)
	}
	writeSeries(t, w, base, 1, 2, 3, 4, 5)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	feed, err := NewAmbientFeed(ctx, cfg, 2, 10*time.Second)
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	snap, err := feed.Next()
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if snap.Count != 2 {
		t.Fatalf("count = %d, want bounded 2", snap.Count)
	}
	// Tail must be the two most recent points.
	want := map[time.Time]int64{
		base.Add(3 * time.Minute): 4,
		base.Add(4 * time.Minute): 5,
	}
	if len(snap.Points) != 2 {
		t.Fatalf("points len = %d, want 2", len(snap.Points))
	}
	for _, p := range snap.Points {
		if want[p.Ts] != p.Value {
			t.Fatalf("unexpected point %v in tail %v", p, snap.Points)
		}
	}
	if snap.From.Equal(snap.To) {
		t.Fatal("from == to; expected a non-degenerate snapshot span")
	}
}

func TestAmbientFeed_NewFeedValidation(t *testing.T) {
	cfg, _ := tsCfg(t)
	if _, err := NewAmbientFeed(context.Background(), cfg, 0, time.Second); err == nil {
		t.Fatal("expected error for limit = 0")
	}
	if _, err := NewAmbientFeed(context.Background(), cfg, 1, 0); err == nil {
		t.Fatal("expected error for empty span")
	}
}
