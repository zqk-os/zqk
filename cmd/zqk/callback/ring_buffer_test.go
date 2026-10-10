package callback

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/walutil"
)

func createTestCallbackEntry(jobID string, seq int64) *CallbackEntry {
	return &CallbackEntry{
		JobID:     jobID,
		Seq:       seq,
		Timestamp: time.Now().UTC(),
		Payload: map[string]any{
			"job_id": jobID,
			"seq":    seq,
		},
	}
}

func populateRingBuffer(rb *RingBuffer, count int, startSeq int64) {
	for i := 0; i < count; i++ {
		seq := startSeq + int64(i)
		jobID := fmt.Sprintf("job-%d", seq)
		rb.Push(createTestCallbackEntry(jobID, seq))
	}
}

func TestRingBuffer_BasicPushPopDrain(t *testing.T) {
	rb := NewRingBuffer(5)
	assert.Equal(t, 5, rb.Capacity())
	assert.Equal(t, 0, rb.Size())
	assert.True(t, rb.IsEmpty())
	assert.False(t, rb.IsFull())

	// Push nil check
	evictedNil := rb.Push(nil)
	assert.False(t, evictedNil)

	// Pop empty check
	poppedNil, ok := rb.Pop()
	assert.False(t, ok)
	assert.Nil(t, poppedNil)

	// Push single entry
	entry1 := createTestCallbackEntry("job-1", 100)
	evicted1 := rb.Push(entry1)
	assert.False(t, evicted1)
	assert.Equal(t, 1, rb.Size())
	assert.False(t, rb.IsEmpty())

	// Pop single entry
	popped, ok := rb.Pop()
	require.True(t, ok)
	assert.Equal(t, int64(100), popped.Seq)
	assert.Equal(t, 0, rb.Size())
	assert.True(t, rb.IsEmpty())
	assert.Equal(t, int64(100), rb.Watermark())

	// Drain check
	populateRingBuffer(rb, 4, 201)
	assert.Equal(t, 4, rb.Size())

	drainedZero := rb.Drain(0)
	assert.Empty(t, drainedZero)

	drainedTwo := rb.Drain(2)
	require.Len(t, drainedTwo, 2)
	assert.Equal(t, int64(201), drainedTwo[0].Seq)
	assert.Equal(t, int64(202), drainedTwo[1].Seq)
	assert.Equal(t, 2, rb.Size())

	drainedRest := rb.Drain(10)
	require.Len(t, drainedRest, 2)
	assert.Equal(t, int64(203), drainedRest[0].Seq)
	assert.Equal(t, int64(204), drainedRest[1].Seq)
	assert.Equal(t, 0, rb.Size())
	assert.Equal(t, int64(204), rb.Watermark())
}

func TestRingBuffer_CircularWrapAround(t *testing.T) {
	rb := NewRingBuffer(4)

	// Fill to capacity
	populateRingBuffer(rb, 4, 1)
	assert.True(t, rb.IsFull())

	// Pop 2 items (frees slots at index 0 and 1)
	e1, ok1 := rb.Pop()
	require.True(t, ok1)
	assert.Equal(t, int64(1), e1.Seq)

	e2, ok2 := rb.Pop()
	require.True(t, ok2)
	assert.Equal(t, int64(2), e2.Seq)
	assert.Equal(t, 2, rb.Size())

	// Push 2 more items (wraps around to index 0 and 1)
	rb.Push(createTestCallbackEntry("job-5", 5))
	rb.Push(createTestCallbackEntry("job-6", 6))
	assert.True(t, rb.IsFull())

	// Drain all 4 items and verify strict FIFO ordering
	drained := rb.Drain(4)
	require.Len(t, drained, 4)
	expectedSeqs := []int64{3, 4, 5, 6}
	for i, entry := range drained {
		assert.Equal(t, expectedSeqs[i], entry.Seq)
	}
	assert.True(t, rb.IsEmpty())
}

func TestRingBuffer_OverflowAndEviction(t *testing.T) {
	rb := NewRingBuffer(3)

	// Push 3 items (exact capacity)
	populateRingBuffer(rb, 3, 10)
	assert.Equal(t, int64(0), rb.EvictedCount())
	assert.Equal(t, int64(3), rb.TotalPushed())

	// 4th push triggers eviction of oldest (seq 10)
	evicted4 := rb.Push(createTestCallbackEntry("job-13", 13))
	assert.True(t, evicted4)
	assert.Equal(t, int64(1), rb.EvictedCount())
	assert.Equal(t, 3, rb.Size())

	// 5th push triggers eviction of next oldest (seq 11)
	evicted5 := rb.Push(createTestCallbackEntry("job-14", 14))
	assert.True(t, evicted5)
	assert.Equal(t, int64(2), rb.EvictedCount())
	assert.Equal(t, 3, rb.Size())

	// Draining remaining items should yield seq 12, 13, 14
	drained := rb.Drain(5)
	require.Len(t, drained, 3)
	assert.Equal(t, int64(12), drained[0].Seq)
	assert.Equal(t, int64(13), drained[1].Seq)
	assert.Equal(t, int64(14), drained[2].Seq)

	// Reset check
	rb.Reset()
	assert.Equal(t, 0, rb.Size())
	assert.Equal(t, int64(0), rb.EvictedCount())
	assert.Equal(t, int64(0), rb.TotalPushed())
	assert.Equal(t, int64(0), rb.Watermark())
}

func TestRingBuffer_WatermarkCalculation(t *testing.T) {
	rb := NewRingBuffer(5)

	// Empty buffer initial watermark
	assert.Equal(t, int64(0), rb.Watermark())

	// Push seq 10, 11, 12: unconsumed oldest is 10, so committed watermark is 9
	rb.Push(createTestCallbackEntry("job-10", 10))
	rb.Push(createTestCallbackEntry("job-11", 11))
	rb.Push(createTestCallbackEntry("job-12", 12))
	assert.Equal(t, int64(9), rb.Watermark())

	// Pop 10: unconsumed oldest is 11, so committed watermark is 10
	p1, ok1 := rb.Pop()
	require.True(t, ok1)
	assert.Equal(t, int64(10), p1.Seq)
	assert.Equal(t, int64(10), rb.Watermark())

	// Drain remaining: buffer becomes empty, watermark is 12
	drained := rb.Drain(5)
	require.Len(t, drained, 2)
	assert.Equal(t, int64(12), rb.Watermark())

	// Explicit advancement
	advanced := rb.AdvanceWatermark(15)
	assert.True(t, advanced)
	assert.Equal(t, int64(15), rb.Watermark())

	notAdvanced := rb.AdvanceWatermark(14)
	assert.False(t, notAdvanced)
	assert.Equal(t, int64(15), rb.Watermark())
}

func TestShardedRingBuffer_HashPartitioning(t *testing.T) {
	sharded := NewShardedRingBuffer(4, 100)
	assert.Equal(t, 4, sharded.NumShards())
	assert.Equal(t, 400, sharded.TotalCapacity())
	assert.Equal(t, 0, sharded.TotalSize())

	// Same jobID always routes to identical shard
	e1 := createTestCallbackEntry("tenant-alpha-job-42", 1)
	e2 := createTestCallbackEntry("tenant-alpha-job-42", 2)
	idx1 := sharded.ShardIndexFor(e1)
	idx2 := sharded.ShardIndexFor(e2)
	assert.Equal(t, idx1, idx2)

	// Different jobIDs distribute across shards
	shardsSeen := make(map[int]bool)
	for i := 0; i < 20; i++ {
		jobID := fmt.Sprintf("partition-probe-%d", i)
		entry := createTestCallbackEntry(jobID, int64(i+1))
		idx := sharded.ShardIndexFor(entry)
		shardsSeen[idx] = true
		sharded.Push(entry)
	}
	assert.Greater(t, len(shardsSeen), 1, "hashing should distribute across multiple shards")
	assert.Equal(t, 20, sharded.TotalSize())

	// Fallback to payload object_id when jobID is empty
	payloadEntry := &CallbackEntry{
		Payload: map[string]any{"object_id": "REQ-1234"},
	}
	idxPayload := sharded.ShardIndexFor(payloadEntry)
	assert.GreaterOrEqual(t, idxPayload, 0)
	assert.Less(t, idxPayload, 4)

	// Fallback when completely empty
	emptyEntry := &CallbackEntry{}
	idxEmpty1 := sharded.ShardIndexFor(emptyEntry)
	idxEmpty2 := sharded.ShardIndexFor(emptyEntry)
	assert.GreaterOrEqual(t, idxEmpty1, 0)
	assert.GreaterOrEqual(t, idxEmpty2, 0)
}

func TestShardedRingBuffer_CommittedWatermarkAcrossShards(t *testing.T) {
	sharded := NewShardedRingBuffer(4, 10)
	assert.Equal(t, int64(0), sharded.CommittedWatermark())

	// Shard 0 receives seq 10, 20, 30
	s0 := sharded.Shard(0)
	s0.Push(createTestCallbackEntry("j0-1", 10))
	s0.Push(createTestCallbackEntry("j0-2", 20))
	s0.Push(createTestCallbackEntry("j0-3", 30))

	// Shard 1 receives seq 15, 25
	s1 := sharded.Shard(1)
	s1.Push(createTestCallbackEntry("j1-1", 15))
	s1.Push(createTestCallbackEntry("j1-2", 25))

	// Shard 0 unconsumed oldest is 10 (wm=9), Shard 1 unconsumed oldest is 15 (wm=14)
	assert.Equal(t, int64(9), sharded.CommittedWatermark())

	// Drain 1 item from Shard 0 (seq 10 popped, next oldest is 20 -> wm=19)
	p0, ok0 := s0.Pop()
	require.True(t, ok0)
	assert.Equal(t, int64(10), p0.Seq)
	// Now Shard 0 wm=19, Shard 1 wm=14 -> min is 14
	assert.Equal(t, int64(14), sharded.CommittedWatermark())

	// Drain all from Shard 1 (empty, wm=25)
	d1 := s1.Drain(10)
	require.Len(t, d1, 2)
	// Now Shard 0 wm=19, Shard 1 wm=25 -> min is 19
	assert.Equal(t, int64(19), sharded.CommittedWatermark())

	// Drain all from Shard 0 (empty, wm=30)
	d0 := s0.Drain(10)
	require.Len(t, d0, 2)
	// Both empty: Shard 0 wm=30, Shard 1 wm=25 -> min is 25
	assert.Equal(t, int64(25), sharded.CommittedWatermark())
}

func TestShardedRingBuffer_ConcurrentPushAndDrain(t *testing.T) {
	numShards := 4
	capacityPerShard := 256
	sharded := NewShardedRingBuffer(numShards, capacityPerShard)

	producers := 4
	itemsPerProducer := 250
	var wg sync.WaitGroup

	// Launch concurrent producers
	for p := 0; p < producers; p++ {
		wg.Add(1)
		producerID := p
		goroutinelabels.NewGoroutine("test_ring_buffer_producer", "concurrent producer").StartSimple(func() {
			defer wg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				seq := int64(producerID*1000 + i + 1)
				jobID := fmt.Sprintf("tenant-%d-job-%d", producerID, i)
				entry := createTestCallbackEntry(jobID, seq)
				sharded.Push(entry)
			}
		})
	}

	// Launch concurrent shard consumers
	var consumed atomic.Int64
	stopConsumers := make(chan struct{})
	var consumerWg sync.WaitGroup

	for s := 0; s < numShards; s++ {
		consumerWg.Add(1)
		shardID := s
		goroutinelabels.NewGoroutine("test_ring_buffer_consumer", "concurrent consumer").StartSimple(func() {
			defer consumerWg.Done()
			for {
				select {
				case <-stopConsumers:
					drained := sharded.DrainShard(shardID, 100)
					consumed.Add(int64(len(drained)))
					return
				default:
					drained := sharded.DrainShard(shardID, 32)
					consumed.Add(int64(len(drained)))
					time.Sleep(50 * time.Microsecond)
				}
			}
		})
	}

	wg.Wait()
	close(stopConsumers)
	consumerWg.Wait()

	totalExpected := int64(producers * itemsPerProducer)
	totalProcessed := consumed.Load() + sharded.EvictedCount() + int64(sharded.TotalSize())
	assert.Equal(t, totalExpected, totalProcessed)
}

func TestShardedRingBuffer_WALTruncationHook(t *testing.T) {
	sharded := NewShardedRingBuffer(4, 10)

	var hookWatermarks []int64
	var hookMu sync.Mutex

	sharded.SetTruncateHook(func(watermark int64) error {
		hookMu.Lock()
		defer hookMu.Unlock()
		hookWatermarks = append(hookWatermarks, watermark)
		return nil
	})

	// Truncate on 0 watermark should do nothing
	wm0, err0 := sharded.TruncateWAL()
	require.NoError(t, err0)
	assert.Equal(t, int64(0), wm0)

	// Push and drain items on shard 0 and 1
	s0 := sharded.Shard(0)
	s0.Push(createTestCallbackEntry("j0", 100))
	item0, ok0 := s0.Pop()
	require.True(t, ok0)
	assert.Equal(t, int64(100), item0.Seq)

	s1 := sharded.Shard(1)
	s1.Push(createTestCallbackEntry("j1", 80))
	item1, ok1 := s1.Pop()
	require.True(t, ok1)
	assert.Equal(t, int64(80), item1.Seq)

	// Min committed watermark is 80
	wm1, err1 := sharded.TruncateWAL()
	require.NoError(t, err1)
	assert.Equal(t, int64(80), wm1)
	assert.Equal(t, int64(80), sharded.LastTruncatedWatermark())

	// Idempotency: calling again without advance should not re-trigger hook
	wm2, err2 := sharded.TruncateWAL()
	require.NoError(t, err2)
	assert.Equal(t, int64(80), wm2)

	hookMu.Lock()
	require.Len(t, hookWatermarks, 1)
	assert.Equal(t, int64(80), hookWatermarks[0])
	hookMu.Unlock()

	// Error propagation
	sharded.SetTruncateHook(func(watermark int64) error {
		return fmt.Errorf("disk full simulation")
	})
	s1.Push(createTestCallbackEntry("j1-next", 120))
	itemNext, okNext := s1.Pop()
	require.True(t, okNext)
	assert.Equal(t, int64(120), itemNext.Seq)

	_, errErr := sharded.TruncateWAL()
	require.Error(t, errErr)
	assert.Contains(t, errErr.Error(), "disk full simulation")
}

func TestShardedRingBuffer_SubscriberInterface(t *testing.T) {
	sharded := NewShardedRingBuffer(4, 32)
	assert.Equal(t, SubscriberNameShardedRingBuffer, sharded.Name())

	dispatcher := NewMultiSubscriberDispatcher(nil)
	dispatcher.Register(sharded)

	subs := dispatcher.Subscribers()
	require.Len(t, subs, 1)

	entry := createTestCallbackEntry("dispatcher-job-1", 55)
	err := dispatcher.Dispatch(context.Background(), entry)
	require.NoError(t, err)

	assert.Equal(t, 1, sharded.TotalSize())
	drained := sharded.DrainAll(10)
	require.Len(t, drained, 1)
	assert.Equal(t, int64(55), drained[0].Seq)
}

func TestTruncateLifecycleWALFile(t *testing.T) {
	tmpDir := t.TempDir()
	walPath := filepath.Join(tmpDir, "lifecycle_events.wal")

	// Append 10 events with seq 1..10
	for i := 1; i <= 10; i++ {
		record := map[string]any{
			"seq":        int64(i),
			"event_type": "status_transition",
			"id":         fmt.Sprintf("OBJ-%d", i),
		}
		appendErr := walutil.AppendJSONLine(walPath, record)
		require.NoError(t, appendErr)
	}

	// Truncate before seq 6 (removes seq 1..6, retains 7..10)
	retained, truncated, err := TruncateLifecycleWALFile(walPath, 6)
	require.NoError(t, err)
	assert.Equal(t, 6, truncated)
	assert.Equal(t, 4, retained)

	// Verify remaining lines
	var remainingSeqs []int64
	scanErr := walutil.ScanFileLines(walPath, 64*1024, func(line []byte) {
		seq := walutil.ExtractSeqFromJSON(line)
		remainingSeqs = append(remainingSeqs, seq)
	})
	require.NoError(t, scanErr)
	assert.Equal(t, []int64{7, 8, 9, 10}, remainingSeqs)

	// Verify checkpoint file updated
	ckptPath := walPath + ".checkpoint"
	ckptData, readErr := fileutil.ReadFile(ckptPath)
	require.NoError(t, readErr)
	cursor, parseErr := walutil.ParseReplayCursorCheckpoint(ckptData)
	require.NoError(t, parseErr)
	assert.Equal(t, int64(6), cursor.Seq)

	// Test NewLifecycleWALFileTruncator
	truncator := NewLifecycleWALFileTruncator(walPath)
	truncErr := truncator.TruncateBefore(8)
	require.NoError(t, truncErr)

	var finalSeqs []int64
	scanErrFinal := walutil.ScanFileLines(walPath, 64*1024, func(line []byte) {
		finalSeqs = append(finalSeqs, walutil.ExtractSeqFromJSON(line))
	})
	require.NoError(t, scanErrFinal)
	assert.Equal(t, []int64{9, 10}, finalSeqs)
}
