package asynccheck_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/systemcheck/asynccheck"
)

func TestFormatProgressSummary_Normal(t *testing.T) {
	line := asynccheck.FormatProgressSummary(50.0, 50, 100, 10, 25.0, "validating", 4, 8, 2*time.Second)
	require.Contains(t, line, "50.0%")
	require.Contains(t, line, "50/100")
	require.Contains(t, line, "queue: 10")
	require.Contains(t, line, "25/s")
	require.Contains(t, line, "workers: 4")
	require.Contains(t, line, "goroutines: 8")
	require.Contains(t, line, "2s")
}

func TestFormatProgressSummary_NaNAndZeroTotal(t *testing.T) {
	lineZero := asynccheck.FormatProgressSummary(0, 0, 0, 0, 0, "", 0, 0, 0)
	require.Contains(t, lineZero, "0.0%")
	require.NotContains(t, lineZero, "NaN")

	lineNaN := asynccheck.FormatProgressSummary(math.NaN(), 0, 10, 0, 0, "", 0, 0, 0)
	require.Contains(t, lineNaN, "0.0%")
	require.NotContains(t, lineNaN, "NaN")

	lineOver := asynccheck.FormatProgressSummary(150.0, 150, 100, 0, 0, "", 0, 0, 0)
	require.Contains(t, lineOver, "100.0%")
}

func TestCalculateRemainingTime(t *testing.T) {
	require.Equal(t, 2*time.Second, asynccheck.CalculateRemainingTime(100, 50, 25.0))
	require.Equal(t, time.Duration(0), asynccheck.CalculateRemainingTime(100, 50, 0))
	require.Equal(t, time.Duration(0), asynccheck.CalculateRemainingTime(100, 100, 25.0))
	require.Equal(t, time.Duration(0), asynccheck.CalculateRemainingTime(100, 110, 25.0))
}

func TestFormatDuration(t *testing.T) {
	require.Equal(t, "45s", asynccheck.FormatDuration(45*time.Second))
	require.Equal(t, "02:30", asynccheck.FormatDuration(150*time.Second))
	require.Equal(t, "01:15:00", asynccheck.FormatDuration(75*time.Minute))
}

func TestEmitCheckProgressEventViaCoordinator_EmptyRoot(t *testing.T) {
	// Should cleanly return without error
	asynccheck.EmitCheckProgressEventViaCoordinator(
		context.Background(),
		"",
		nil,
		"op-1",
		asynccheck.ProgressEventTypeProgress,
		10, 100, 10, 0, 90,
		"progress message",
		"system",
	)
}

func TestEmitAsyncRouterEventViaCoordinator_EmptyRoot(t *testing.T) {
	asynccheck.EmitAsyncRouterEventViaCoordinator(
		context.Background(),
		"",
		nil,
		"worker-1",
		"worker_start",
		"success",
		4, 10, 0,
		time.Second,
	)
}

func TestEmitAsyncValidatorEventViaCoordinator_EmptyRoot(t *testing.T) {
	asynccheck.EmitAsyncValidatorEventViaCoordinator(
		context.Background(),
		"",
		nil,
		"op-1",
		"validation_timeout",
		"OBJ-1",
		"timed out",
		nil,
		asynccheck.SeverityMedium,
		"system",
	)
}
