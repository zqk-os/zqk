package ambient

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TestStreamHydrator_FunctionalAcceptance verifies CRIT-1793579044171198000-9b970bc7:
// Ingest intent raindrops, pool into intent stream, and generate coherent preference summary without drift.
func TestStreamHydrator_FunctionalAcceptance(t *testing.T) {
	ctx := context.Background()
	hydrator := NewStreamHydrator(100, 0.05)

	streamID := "stream-expedite-coverage"
	drops := []IntentRaindrop{
		{
			ID:        "drop-001",
			Source:    "user_prompt",
			Kind:      "intent",
			Content:   "Expedite kernel test coverage past 85%",
			Weight:    1.0,
			Timestamp: time.Now().UTC(),
			Tags:      []string{"coverage", "testing"},
		},
		{
			ID:        "drop-002",
			Source:    "daemon_hygiene",
			Kind:      "intent",
			Content:   "Maintain repository branch hygiene",
			Weight:    0.8,
			Timestamp: time.Now().UTC(),
			Tags:      []string{"hygiene", "branch"},
		},
		{
			ID:        "drop-003",
			Source:    "human_review",
			Kind:      "preference",
			Content:   "Eliminate magic numbers in memory allocations",
			Weight:    0.9,
			Timestamp: time.Now().UTC(),
			Tags:      []string{"vet", "hygiene"},
		},
	}

	for _, drop := range drops {
		err := hydrator.PoolRaindrop(ctx, streamID, drop)
		require.NoError(t, err)
	}

	snap, ok := hydrator.GetStream(streamID)
	require.True(t, ok)
	assert.Equal(t, streamID, snap.ID)
	assert.Equal(t, StreamStatusPooling, snap.Status)
	assert.Len(t, snap.Raindrops, 3)
	assert.InDelta(t, 2.7, snap.TotalWeight, 0.001)

	merged := assertStreamMergedSuccessfully(t, hydrator, streamID)
	assert.False(t, merged.MergedAt.IsZero())
	assert.Contains(t, merged.PreferenceSummary, "Expedite kernel test coverage past 85%")
	assert.Contains(t, merged.PreferenceSummary, "Eliminate magic numbers in memory allocations")
}

// TestStreamHydrator_BoundaryAndErrorHandling verifies CRIT-1793579044171199000-564a6c05:
// Boundary validation, empty IDs, duplicate rejection, and capacity exhaustion.
func TestStreamHydrator_BoundaryAndErrorHandling(t *testing.T) {
	ctx := context.Background()
	hydrator := NewStreamHydrator(2, 0.1)

	// Sub-test 1: Empty stream ID
	err := hydrator.PoolRaindrop(ctx, "", IntentRaindrop{ID: "d1", Weight: 0.5})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "streamID cannot be empty")

	// Sub-test 2: Empty drop ID
	err = hydrator.PoolRaindrop(ctx, "s1", IntentRaindrop{ID: "", Weight: 0.5})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "raindrop ID cannot be empty")

	// Sub-test 3: Drop weight below minimum threshold
	err = hydrator.PoolRaindrop(ctx, "s1", IntentRaindrop{ID: "d-light", Weight: 0.05})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "below minimum threshold")

	// Sub-test 4: Successful ingestion
	err = hydrator.PoolRaindrop(ctx, "s1", IntentRaindrop{ID: "d1", Content: "initial", Weight: 0.5})
	require.NoError(t, err)

	// Sub-test 5: Duplicate drop ID rejection
	err = hydrator.PoolRaindrop(ctx, "s1", IntentRaindrop{ID: "d1", Content: "duplicate", Weight: 0.5})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate raindrop ID")

	// Sub-test 6: Cancelled context rejection
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err = hydrator.PoolRaindrop(cancelCtx, "s1", IntentRaindrop{ID: "d-cancelled", Weight: 0.5})
	assert.ErrorIs(t, err, context.Canceled)

	// Sub-test 7: Capacity exhaustion
	err = hydrator.PoolRaindrop(ctx, "s2", IntentRaindrop{ID: "d2", Weight: 0.5})
	require.NoError(t, err)
	err = hydrator.PoolRaindrop(ctx, "s3", IntentRaindrop{ID: "d3", Weight: 0.5})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "capacity limit reached")

	// Sub-test 8: Merging empty stream returns error
	emptyHydrator := NewStreamHydrator(10, 0.1)
	_, err = emptyHydrator.MergeStream("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestStreamHydrator_IntegrationAndConformance verifies CRIT-1793579044171200000-50c8fcf8:
// Concurrent thread safety under parallel multi-agent ingestion.
func TestStreamHydrator_IntegrationAndConformance(t *testing.T) {
	ctx := context.Background()
	hydrator := NewStreamHydrator(50, 0.01)

	streamID := "concurrent-stream"
	const numGoroutines = 20
	const dropsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		gID := g
		goroutinelabels.NewGoroutine("stream-hydrator-test", "concurrent-raindrop-ingestion").StartSimple(func() {
			defer wg.Done()
			for d := 0; d < dropsPerGoroutine; d++ {
				drop := IntentRaindrop{
					ID:        fmt.Sprintf("drop-g%d-d%d", gID, d),
					Source:    fmt.Sprintf("worker-%d", gID),
					Kind:      "stream_slice",
					Content:   fmt.Sprintf("payload from g%d item %d", gID, d),
					Weight:    0.1,
					Timestamp: time.Now().UTC(),
				}
				err := hydrator.PoolRaindrop(ctx, streamID, drop)
				assert.NoError(t, err)
			}
		})
	}

	wg.Wait()

	snap, ok := hydrator.GetStream(streamID)
	require.True(t, ok)
	assert.Equal(t, numGoroutines*dropsPerGoroutine, len(snap.Raindrops))
	expectedTotalWeight := float64(numGoroutines*dropsPerGoroutine) * 0.1
	assert.InDelta(t, expectedTotalWeight, snap.TotalWeight, 0.001)

	merged := assertStreamMergedSuccessfully(t, hydrator, streamID)
	assert.NotNil(t, merged)
}

func assertStreamMergedSuccessfully(t *testing.T, hydrator *StreamHydrator, streamID string) *IntentStream {
	t.Helper()
	merged, err := hydrator.MergeStream(streamID)
	require.NoError(t, err)
	assert.Equal(t, StreamStatusMerged, merged.Status)
	return merged
}
