package metrics

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// chunkFileInfo records a chunk's file path and time boundary.
type chunkFileInfo struct {
	path  string
	start time.Time
	end   time.Time
}

// findSeriesChunks scans dir and returns all existing chunks for the given series, sorted chronologically.
func findSeriesChunks(cfg TimeSeriesConfig) ([]chunkFileInfo, error) {
	if _, err := os.Stat(cfg.Dir); err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	entries, err := os.ReadDir(cfg.Dir)
	if err != nil {
		return nil, err
	}

	prefix := sanitizeSeriesName(cfg.Series) + "_"
	suffix := ".chunk"
	var chunks []chunkFileInfo

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
			continue
		}
		timeStr := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
		start, err := time.Parse("20060102T1504", timeStr)
		if err != nil {
			continue
		}
		start = start.UTC()
		chunkDur := cfg.ChunkDuration
		if chunkDur <= 0 {
			chunkDur = time.Hour
		}
		chunks = append(chunks, chunkFileInfo{
			path:  filepath.Join(cfg.Dir, name),
			start: start,
			end:   start.Add(chunkDur),
		})
	}

	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].start.Before(chunks[j].start)
	})

	return chunks, nil
}

// QueryRangePoints iterates over points in [from, to] respecting context cancellation.
func QueryRangePoints(ctx context.Context, cfg TimeSeriesConfig, from, to time.Time, fn func(TimeSeriesPoint) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	effectiveFrom := from
	if effectiveFrom.IsZero() {
		effectiveFrom = time.Unix(0, 0).UTC()
	}
	effectiveTo := to
	if effectiveTo.IsZero() {
		effectiveTo = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	}

	chunks, err := findSeriesChunks(cfg)
	if err != nil {
		return err
	}

	r := NewTimeSeriesReader(cfg)
	for _, ch := range chunks {
		if err := ctx.Err(); err != nil {
			return err
		}
		if ch.end.Before(effectiveFrom) {
			continue
		}
		if ch.start.After(effectiveTo) {
			continue
		}
		if err := r.readChunk(ch.path, ch.start, effectiveFrom, effectiveTo, fn); err != nil {
			if fileutil.IsNotExist(err) {
				continue
			}
			return err
		}
	}
	return nil
}

// RollupBucket represents aggregated points within a single fixed-duration bucket.
type RollupBucket struct {
	Start time.Time
	Count int
	Min   int64
	Max   int64
	Sum   int64
	Avg   float64
}

// RollupResult holds the list of buckets for a Rollup query.
type RollupResult struct {
	Buckets []RollupBucket
}

// Rollup aggregates points within [from, to] into fixed-duration step buckets.
func Rollup(ctx context.Context, cfg TimeSeriesConfig, from, to time.Time, step time.Duration) (RollupResult, error) {
	if step <= 0 {
		return RollupResult{}, errfmt.Errorf("step must be > 0")
	}
	if err := ctx.Err(); err != nil {
		return RollupResult{}, err
	}

	type accum struct {
		start time.Time
		count int
		min   int64
		max   int64
		sum   int64
	}

	buckets := make(map[time.Time]*accum)
	var bucketKeys []time.Time

	err := QueryRangePoints(ctx, cfg, from, to, func(p TimeSeriesPoint) error {
		bStart := p.Ts.UTC().Truncate(step)
		ac, exists := buckets[bStart]
		if !exists {
			ac = &accum{
				start: bStart,
				count: 1,
				min:   p.Value,
				max:   p.Value,
				sum:   p.Value,
			}
			buckets[bStart] = ac
			bucketKeys = append(bucketKeys, bStart)
		} else {
			ac.count++
			if p.Value < ac.min {
				ac.min = p.Value
			}
			if p.Value > ac.max {
				ac.max = p.Value
			}
			ac.sum += p.Value
		}
		return nil
	})
	if err != nil {
		return RollupResult{}, err
	}

	sort.Slice(bucketKeys, func(i, j int) bool {
		return bucketKeys[i].Before(bucketKeys[j])
	})

	res := RollupResult{
		Buckets: make([]RollupBucket, 0, len(bucketKeys)),
	}
	for _, k := range bucketKeys {
		ac := buckets[k]
		avg := float64(ac.sum) / float64(ac.count)
		res.Buckets = append(res.Buckets, RollupBucket{
			Start: ac.start,
			Count: ac.count,
			Min:   ac.min,
			Max:   ac.max,
			Sum:   ac.sum,
			Avg:   avg,
		})
	}
	return res, nil
}

// ListSeries discovers series names available in the specified directory.
func ListSeries(dir string) ([]string, error) {
	if _, err := os.Stat(dir); err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var seriesList []string

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".chunk") {
			continue
		}
		base := strings.TrimSuffix(name, ".chunk")
		lastIdx := strings.LastIndex(base, "_")
		if lastIdx == -1 {
			continue
		}
		sanitized := base[:lastIdx]
		var original string
		if strings.HasPrefix(sanitized, "object_volume_") {
			original = "object_volume/" + strings.TrimPrefix(sanitized, "object_volume_")
		} else {
			original = sanitized
		}
		if _, ok := seen[original]; !ok {
			seen[original] = struct{}{}
			seriesList = append(seriesList, original)
		}
	}
	sort.Strings(seriesList)
	return seriesList, nil
}

// AmbientSnapshot represents a recent bounded window of points for ambient feed consumers.
type AmbientSnapshot struct {
	Points []TimeSeriesPoint
	Count  int
	From   time.Time
	To     time.Time
}

// AmbientFeed produces recent bounded snapshots of time series points.
type AmbientFeed struct {
	ctx   context.Context
	cfg   TimeSeriesConfig
	limit int
	span  time.Duration
}

// NewAmbientFeed constructs a new AmbientFeed.
func NewAmbientFeed(ctx context.Context, cfg TimeSeriesConfig, limit int, span time.Duration) (*AmbientFeed, error) {
	if limit <= 0 {
		return nil, errfmt.Errorf("limit must be > 0")
	}
	if span <= 0 {
		return nil, errfmt.Errorf("span must be > 0")
	}
	return &AmbientFeed{
		ctx:   ctx,
		cfg:   cfg,
		limit: limit,
		span:  span,
	}, nil
}

// Next reads the most recent span up to limit points.
func (f *AmbientFeed) Next() (*AmbientSnapshot, error) {
	var all []TimeSeriesPoint
	err := QueryRangePoints(f.ctx, f.cfg, time.Time{}, time.Time{}, func(p TimeSeriesPoint) error {
		all = append(all, p)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(all) == 0 {
		now := time.Now().UTC()
		return &AmbientSnapshot{
			Points: nil,
			Count:  0,
			From:   now.Add(-f.span),
			To:     now,
		}, nil
	}

	startIdx := 0
	if len(all) > f.limit {
		startIdx = len(all) - f.limit
	}
	tail := all[startIdx:]

	snapFrom := tail[0].Ts
	snapTo := tail[len(tail)-1].Ts
	if snapFrom.Equal(snapTo) {
		snapFrom = snapFrom.Add(-time.Second)
	}

	return &AmbientSnapshot{
		Points: tail,
		Count:  len(tail),
		From:   snapFrom,
		To:     snapTo,
	}, nil
}