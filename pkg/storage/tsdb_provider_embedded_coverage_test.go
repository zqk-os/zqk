package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEmbeddedTSDB_WritePointAndUninitialized(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	provider := NewEmbeddedTSDBProvider(tmpDir)
	require.NotNil(t, provider)

	// Write before Initialize should fail cleanly
	pt := TSDBPoint{
		Measurement: "cpu",
		Tags:        map[string]string{"host": "h1"},
		Fields:      map[string]any{"load": 1.25},
		Timestamp:   time.Now(),
	}
	err := provider.WritePoint(ctx, pt)
	require.Error(t, err)

	// Now initialize and write via WritePoint
	require.NoError(t, provider.Initialize(ctx))
	t.Cleanup(func() {
		require.NoError(t, provider.Close(ctx))
	})

	require.NoError(t, provider.WritePoint(ctx, pt))
}

func TestEmbeddedTSDB_TimeRangeMatching(t *testing.T) {
	now := time.Now().UTC()
	start := now.Add(-2 * time.Hour)
	end := now.Add(2 * time.Hour)

	today := now.Format("2006-01-02")
	validFile := filepath.Join("/tmp", "metrics-"+today+".tsdb")

	// Matching time range
	require.True(t, tsdbFileMatchesTimeRange(validFile, start, end))

	// Zero times always match
	require.True(t, tsdbFileMatchesTimeRange(validFile, time.Time{}, time.Time{}))

	// Non-standard filename fallback matches
	require.True(t, tsdbFileMatchesTimeRange("/tmp/custom.log", start, end))

	// Outside time window (future start)
	futureStart := now.Add(48 * time.Hour)
	require.False(t, tsdbFileMatchesTimeRange(validFile, futureStart, time.Time{}))

	// Outside time window (past end)
	pastEnd := now.Add(-48 * time.Hour)
	require.False(t, tsdbFileMatchesTimeRange(validFile, time.Time{}, pastEnd))
}

func TestEmbeddedTSDB_ParseTSDBLine_EdgeCases(t *testing.T) {
	evaluator := NewRPNEvaluator("")

	// Corrupt base64
	_, ok := parseTSDBLine("!not-base64!", TSDBQuery{}, evaluator)
	require.False(t, ok)

	// Incomplete parts (less than 4 parts separated by |)
	encodedShort, err := encodePoint(TSDBPoint{Measurement: "m"})
	require.NoError(t, err)
	require.NotEmpty(t, encodedShort)

	// Encoded with invalid timestamp
	badLine := "bWVhc3VyZW1lbnR8e318bm90YW51bWJlcnx7fQ==" // "measurement|{}|notanumber|{}"
	_, ok = parseTSDBLine(badLine, TSDBQuery{}, evaluator)
	require.False(t, ok)

	// Mismatched measurement query
	validPt := TSDBPoint{
		Measurement: "disk",
		Tags:        map[string]string{"device": "sda"},
		Fields:      map[string]any{"used": 42.0},
		Timestamp:   time.Now().UTC(),
	}
	validEncoded, err := encodePoint(validPt)
	require.NoError(t, err)

	_, ok = parseTSDBLine(validEncoded, TSDBQuery{Measurement: "cpu"}, evaluator)
	require.False(t, ok)

	// Matching query
	parsed, ok := parseTSDBLine(validEncoded, TSDBQuery{Measurement: "disk"}, evaluator)
	require.True(t, ok)
	require.Equal(t, "disk", parsed.Measurement)
}

func TestEmbeddedTSDB_AggregationsSumAndCount(t *testing.T) {
	points := []TSDBPoint{
		{
			Measurement: "mem",
			Tags:        map[string]string{"env": "prod"},
			Fields:      map[string]any{"used_mb": 100.0, "code": 1},
			Timestamp:   time.Now().UTC(),
		},
		{
			Measurement: "mem",
			Tags:        map[string]string{"env": "prod"},
			Fields:      map[string]any{"used_mb": 200.0, "code": 2},
			Timestamp:   time.Now().UTC(),
		},
	}

	// Test SUM
	sumQuery := TSDBQuery{
		Measurement: "mem",
		Field:       "used_mb",
		Aggregation: "sum",
		GroupBy:     []string{"env"},
	}
	aggSum := aggregateQueryPoints(points, sumQuery)
	require.Len(t, aggSum, 1)
	require.Equal(t, 300.0, aggSum[0].Fields["used_mb"])

	// Test COUNT
	countQuery := TSDBQuery{
		Measurement: "mem",
		Field:       "used_mb",
		Aggregation: "count",
		GroupBy:     []string{"env"},
	}
	aggCount := aggregateQueryPoints(points, countQuery)
	require.Len(t, aggCount, 1)
	require.Equal(t, 2.0, aggCount[0].Fields["used_mb"])

	// Test integer field handling with GroupBy field
	intQuery := TSDBQuery{
		Measurement: "mem",
		Field:       "code",
		Aggregation: "sum",
		GroupBy:     []string{"code"},
	}
	aggInt := aggregateQueryPoints(points, intQuery)
	require.NotEmpty(t, aggInt)
}

func TestEmbeddedTSDB_CloseIdempotent(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	provider := NewEmbeddedTSDBProvider(tmpDir)
	require.NoError(t, provider.Initialize(ctx))
	require.False(t, provider.IsClosed())

	require.NoError(t, provider.Close(ctx))
	require.True(t, provider.IsClosed())

	// Second Close should be a no-op returning nil
	require.NoError(t, provider.Close(ctx))
}
