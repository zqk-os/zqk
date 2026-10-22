package storage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// EmbeddedTSDBProvider implements an embedded time-series database using local append-only files.
// It serves as a more structured and efficient replacement for raw JSONL logs,
// using base64-encoded [timestamp, value] pairs for compact storage as outlined in the Zenoss pattern.
type EmbeddedTSDBProvider struct {
	baseDir string
	closed  atomic.Bool
	mu      sync.RWMutex
	logFile *fileutil.File
	logger  logging.Logger
}

// NewEmbeddedTSDBProvider creates a new embedded TSDB provider.
func NewEmbeddedTSDBProvider(baseDir string) *EmbeddedTSDBProvider {
	return &EmbeddedTSDBProvider{
		baseDir: baseDir,
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Initialize prepares the embedded TSDB storage directory and current active log file.
func (p *EmbeddedTSDBProvider) Initialize(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := fileutil.EnsureDir(p.baseDir); err != nil {
		return errfmt.Errorf("failed to create tsdb directory: %w", err)
	}

	// For embedded, we append to a daily roll file for basic chunking
	today := time.Now().Format("2006-01-02")
	filePath := filepath.Join(p.baseDir, fmt.Sprintf("metrics-%s.tsdb", today))

	f, err := fileutil.OpenFile(filePath, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644)
	if err != nil {
		return errfmt.Errorf("failed to open tsdb file: %w", err)
	}
	p.logFile = f

	return nil
}

// encodePoint compacts a TSDBPoint into a base64 encoded string format.
// Format: Measurement|Tags(JSON)|Timestamp|Fields(JSON)
func encodePoint(p TSDBPoint) (string, error) {
	tagsBytes, err := json.Marshal(p.Tags)
	if err != nil {
		return "", err
	}
	fieldsBytes, err := json.Marshal(p.Fields)
	if err != nil {
		return "", err
	}

	raw := fmt.Sprintf("%s|%s|%d|%s", p.Measurement, string(tagsBytes), p.Timestamp.UnixNano(), string(fieldsBytes))
	return base64.StdEncoding.EncodeToString([]byte(raw)), nil
}

// WritePoint writes a single data point to the embedded file.
func (p *EmbeddedTSDBProvider) WritePoint(ctx context.Context, point TSDBPoint) error {
	return p.WriteBatch(ctx, []TSDBPoint{point})
}

// WriteBatch writes multiple data points efficiently.
func (p *EmbeddedTSDBProvider) WriteBatch(ctx context.Context, points []TSDBPoint) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.logFile == nil {
		return errfmt.Errorf("embedded tsdb not initialized")
	}

	for _, pt := range points {
		encoded, err := encodePoint(pt)
		if err != nil {
			logging.Fluent(p.logger).Warn("Failed to encode TSDB point").WithError(err).Log()
			continue
		}
		if _, err := p.logFile.WriteString(encoded + "\n"); err != nil {
			return errfmt.Errorf("failed to write point: %w", err)
		}
	}

	// Flush to disk
	return p.logFile.Sync()
}

func tsdbFileMatchesTimeRange(filename string, startTime, endTime time.Time) bool {
	if startTime.IsZero() && endTime.IsZero() {
		return true
	}
	base := filepath.Base(filename)
	const prefix = "metrics-"
	const suffix = ".tsdb"
	if !strings.HasPrefix(base, prefix) || !strings.HasSuffix(base, suffix) {
		return true
	}
	dateStr := base[len(prefix) : len(base)-len(suffix)]
	fileDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return true
	}
	fileStart := fileDate.UTC()
	fileEnd := fileStart.Add(24 * time.Hour)

	if !startTime.IsZero() && !startTime.UTC().Before(fileEnd) {
		return false
	}
	if !endTime.IsZero() && endTime.UTC().Before(fileStart) {
		return false
	}
	return true
}

func parseTSDBLine(line string, query TSDBQuery, evaluator *RPNEvaluator) (*TSDBPoint, bool) {
	decoded, err := base64.StdEncoding.DecodeString(line)
	if err != nil {
		return nil, false
	}

	str := string(decoded)
	parts := strings.SplitN(str, "|", 4)
	if len(parts) < 4 {
		return nil, false
	}

	meas := parts[0]
	if query.Measurement != "" && meas != query.Measurement {
		return nil, false
	}

	timestampNano, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, false
	}
	ts := time.Unix(0, timestampNano)

	if !query.StartTime.IsZero() && ts.Before(query.StartTime) {
		return nil, false
	}
	if !query.EndTime.IsZero() && ts.After(query.EndTime) {
		return nil, false
	}

	var tags map[string]string
	if err := json.Unmarshal([]byte(parts[1]), &tags); err != nil {
		return nil, false
	}

	for k, v := range query.Tags {
		if tv, ok := tags[k]; !ok || tv != v {
			return nil, false
		}
	}

	var fields map[string]any
	if err := json.Unmarshal([]byte(parts[3]), &fields); err != nil {
		return nil, false
	}

	pt := TSDBPoint{
		Measurement: meas,
		Tags:        tags,
		Fields:      fields,
		Timestamp:   ts,
	}

	if query.RPNExpression != "" {
		match, err := evaluator.Evaluate(pt)
		if err != nil || !match {
			return nil, false
		}
	}

	return &pt, true
}

func (p *EmbeddedTSDBProvider) scanFilePoints(file string, query TSDBQuery, evaluator *RPNEvaluator) []TSDBPoint {
	content, err := fileutil.ReadFile(file)
	if err != nil {
		logging.Fluent(p.logger).Warn("Failed to read TSDB file").String("file", file).WithError(err).Log()
		return nil
	}

	var points []TSDBPoint
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if pt, ok := parseTSDBLine(line, query, evaluator); ok {
			points = append(points, *pt)
		}
	}
	return points
}

func aggregateQueryPoints(points []TSDBPoint, query TSDBQuery) []TSDBPoint {
	groups := make(map[string][]float64)
	for _, pt := range points {
		key := ""
		for _, g := range query.GroupBy {
			if v, ok := pt.Tags[g]; ok {
				key += v + "|"
			} else if v, ok := pt.Fields[g]; ok {
				key += fmt.Sprintf("%v|", v)
			}
		}

		var val float64
		if v, ok := pt.Fields[query.Field]; ok {
			switch vt := v.(type) {
			case float64:
				val = vt
			case int:
				val = float64(vt)
			}
		}
		groups[key] = append(groups[key], val)
	}

	aggPoints := make([]TSDBPoint, 0, len(groups))
	for key, vals := range groups {
		var aggVal float64
		switch query.Aggregation {
		case "sum":
			for _, v := range vals {
				aggVal += v
			}
		case "count":
			aggVal = float64(len(vals))
		case "mean":
			if len(vals) > 0 {
				for _, v := range vals {
					aggVal += v
				}
				aggVal /= float64(len(vals))
			}
		}

		pt := TSDBPoint{
			Measurement: query.Measurement,
			Tags:        map[string]string{objects.FieldKeyGroup: key},
			Fields:      map[string]any{query.Field: aggVal},
			Timestamp:   time.Now().UTC(),
		}
		aggPoints = append(aggPoints, pt)
	}
	return aggPoints
}

// Query executes a query against the time-series data.
// In the embedded local file provider, this scans the relevant time bucket files, decodes, and aggregates.
func (p *EmbeddedTSDBProvider) Query(ctx context.Context, query TSDBQuery) (*TSDBQueryResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := &TSDBQueryResult{
		Points: make([]TSDBPoint, 0),
	}

	files, err := filepath.Glob(filepath.Join(p.baseDir, "metrics-*.tsdb"))
	if err != nil {
		return nil, errfmt.Errorf("failed to list tsdb files: %w", err)
	}

	evaluator := NewRPNEvaluator(query.RPNExpression)
	for _, file := range files {
		if !tsdbFileMatchesTimeRange(file, query.StartTime, query.EndTime) {
			continue
		}
		pts := p.scanFilePoints(file, query, evaluator)
		if len(pts) > 0 {
			result.Points = append(result.Points, pts...)
		}
	}

	if query.Aggregation != "" && query.Field != "" {
		result.Points = aggregateQueryPoints(result.Points, query)
	}

	return result, nil
}

// IsClosed returns whether the provider has been closed.
func (p *EmbeddedTSDBProvider) IsClosed() bool {
	return p.closed.Load()
}

// Close gracefully shuts down the provider.
func (p *EmbeddedTSDBProvider) Close(ctx context.Context) error {
	if p.closed.Swap(true) {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.logFile != nil {
		err := p.logFile.Close()
		p.logFile = nil
		return err
	}
	return nil
}
