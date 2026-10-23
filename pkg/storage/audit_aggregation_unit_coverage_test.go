package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestAuditAggregation_ExpandIDRanges(t *testing.T) {
	// Single range
	singleRange := ExpandIDRange("AUD-1..AUD-3")
	require.Equal(t, []string{"AUD-1", "AUD-2", "AUD-3"}, singleRange)

	// Single ID without range
	singleID := ExpandIDRange("AUD-5")
	require.Equal(t, []string{"AUD-5"}, singleID)

	// Inverted or invalid range returns original item
	invalidRange := ExpandIDRange("AUD-5..AUD-2")
	require.Equal(t, []string{"AUD-5..AUD-2"}, invalidRange)

	// Multiple mixed ranges
	mixed := ExpandIDRanges([]string{"AUD-1..AUD-2", "AUD-10", "AUD-20..AUD-22"})
	require.Equal(t, []string{"AUD-1", "AUD-2", "AUD-10", "AUD-20", "AUD-21", "AUD-22"}, mixed)
}

func TestAuditAggregation_ParseWindowAndConfig(t *testing.T) {
	// Test days
	durDay, err := ParseWindowSize("2d")
	require.NoError(t, err)
	require.Equal(t, 48*time.Hour, durDay)

	// Test weeks
	durWeek, err := ParseWindowSize("1w")
	require.NoError(t, err)
	require.Equal(t, 7*24*time.Hour, durWeek)

	// Test months
	durMonth, err := ParseWindowSize("1m")
	require.NoError(t, err)
	require.Equal(t, 30*24*time.Hour, durMonth)

	// Test hours
	durHour, err := ParseWindowSize("3h")
	require.NoError(t, err)
	require.Equal(t, 3*time.Hour, durHour)

	// Test invalid
	_, err = ParseWindowSize("invalid")
	require.Error(t, err)
}

func TestAuditAggregation_LoadConfigAndDefaults(t *testing.T) {
	defaultCfg := DefaultAggregationConfig()
	require.NotNil(t, defaultCfg)
	require.True(t, defaultCfg.Enabled)
	require.NotEmpty(t, defaultCfg.Rules)

	tmpDir := t.TempDir()
	cfg, window := loadBufferConfigAndWindow(tmpDir)
	require.NotNil(t, cfg)
	require.True(t, window > 0)

	// Non-existent directory load
	loaded, err := LoadAggregationConfig(filepath.Join(tmpDir, "does-not-exist"))
	require.NoError(t, err)
	require.NotNil(t, loaded)
}

func TestAuditAggregation_ServiceStatsAndHelpers(t *testing.T) {
	service := NewAuditAggregationService(nil)
	require.NotNil(t, service)

	aggs, events := service.GetAuditAggregationStats()
	require.GreaterOrEqual(t, aggs, int64(0))
	require.GreaterOrEqual(t, events, int64(0))

	serviceWithBatch := NewAuditAggregationServiceWithBatchSize(nil, 50)
	require.NotNil(t, serviceWithBatch)

	// Filter out archived events on empty
	secCtx := pkgctx.NewSystemSecurityContext()
	filtered := service.filterOutArchivedEvents(context.Background(), secCtx, nil, nil)
	require.Empty(t, filtered)
}
