package callback

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func safeCloseRing(t *testing.T, ring *SharedMemoryRingBuffer) {
	if ring != nil {
		closeErr := ring.Close()
		assert.NoError(t, closeErr)
	}
}

func safeCloseDispatcher(t *testing.T, sub *VectorizedBatchDispatcher) {
	if sub != nil {
		closeErr := sub.Close()
		assert.NoError(t, closeErr)
	}
}

func createTestFile(t *testing.T, path string, data []byte) {
	writeErr := fileutil.WriteFile(path, data, paths.FilePerm644)
	require.NoError(t, writeErr)
}

func createSampleEntry(jobID string, seq int64, priority int) *CallbackEntry {
	return &CallbackEntry{
		JobID:     jobID,
		Seq:       seq,
		Priority:  priority,
		Timestamp: time.Now(),
		Payload:   map[string]any{"job_id": jobID, "seq": seq},
	}
}

func TestSHM_InMemoryFIFOAndZeroCopy(t *testing.T) {
	t.Parallel()

	const capacity = 64
	const payloadSize = 256
	ring, err := NewInMemoryRingBuffer(capacity, payloadSize)
	require.NoError(t, err)
	defer safeCloseRing(t, ring)

	assert.Equal(t, capacity, int(ring.Capacity()))
	assert.Equal(t, payloadSize, int(ring.SlotPayloadSize()))

	const totalEntries = 200
	for i := 0; i < totalEntries; i++ {
		msg := fmt.Sprintf("payload-%06d", i)
		slot, commit, reserveErr := ring.Reserve(uint32(len(msg)))
		require.NoError(t, reserveErr)

		slotIdx := int(ring.Tail()-1) & int(ring.mask)
		expectedSlotStart := uintptr(unsafe.Pointer(&ring.slots[slotIdx].payload[0]))
		actualSlotStart := uintptr(unsafe.Pointer(&slot[0]))
		assert.Equal(t, expectedSlotStart, actualSlotStart)

		copy(slot, msg)
		commit()

		readSlot, release, readErr := ring.ReserveRead()
		require.NoError(t, readErr)
		assert.Equal(t, msg, string(readSlot))
		release()
	}

	assert.True(t, ring.IsEmpty())
}

func TestSHM_MmapFIFOAndPersistence(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	shmPath := filepath.Join(tmpDir, "test_shm.seg")

	const capacity = 32
	const payloadSize = 128
	ring, err := OpenMemoryMappedRingBuffer(shmPath, capacity, payloadSize)
	require.NoError(t, err)

	const count = 50
	for i := 0; i < count; i++ {
		msg := fmt.Sprintf("persisted-%04d", i)
		slot, commit, rErr := ring.Reserve(uint32(len(msg)))
		require.NoError(t, rErr)
		copy(slot, msg)
		commit()

		readSlot, release, rdErr := ring.ReserveRead()
		require.NoError(t, rdErr)
		assert.Equal(t, msg, string(readSlot))
		release()
	}

	slot1, commit1, err1 := ring.Reserve(uint32(len("item1")))
	require.NoError(t, err1)
	copy(slot1, "item1")
	commit1()

	slot2, commit2, err2 := ring.Reserve(uint32(len("item2")))
	require.NoError(t, err2)
	copy(slot2, "item2")
	commit2()

	require.NoError(t, ring.Close())

	reopened, reopenErr := OpenExistingMemoryMappedRingBuffer(shmPath)
	require.NoError(t, reopenErr)
	defer safeCloseRing(t, reopened)

	assert.Equal(t, uint64(2), reopened.Size())

	r1, rel1, r1Err := reopened.ReserveRead()
	require.NoError(t, r1Err)
	assert.Equal(t, "item1", string(r1))
	rel1()

	r2, rel2, r2Err := reopened.ReserveRead()
	require.NoError(t, r2Err)
	assert.Equal(t, "item2", string(r2))
	rel2()

	assert.True(t, reopened.IsEmpty())
}

func TestSHM_WriteBatchAndReadBatch(t *testing.T) {
	t.Parallel()

	ring, err := NewInMemoryRingBuffer(64, 512)
	require.NoError(t, err)
	defer safeCloseRing(t, ring)

	entries := []CallbackEntry{
		{JobID: "job-1", Priority: 10, Seq: 101, Payload: map[string]any{"action": "start"}},
		{JobID: "job-2", Priority: 20, Seq: 102, Payload: map[string]any{"action": "progress"}},
		{JobID: "job-3", Priority: 30, Seq: 103, Payload: map[string]any{"action": "complete"}},
	}

	written, wErr := ring.WriteBatch(entries)
	require.NoError(t, wErr)
	assert.Equal(t, len(entries), written)

	readEntries, rErr := ring.ReadBatch(10)
	require.NoError(t, rErr)
	require.Equal(t, len(entries), len(readEntries))

	for i, expected := range entries {
		assert.Equal(t, expected.JobID, readEntries[i].JobID)
		assert.Equal(t, expected.Priority, readEntries[i].Priority)
		assert.Equal(t, expected.Seq, readEntries[i].Seq)
	}
}

func TestSHM_ZeroCopyWriteAndDrainBatch(t *testing.T) {
	t.Parallel()

	ring, err := NewSharedMemoryRingBuffer(64, 512)
	require.NoError(t, err)
	defer safeCloseRing(t, ring)

	for i := 0; i < 20; i++ {
		entry := createSampleEntry(fmt.Sprintf("shm-job-%d", i), int64(i+1), i*5)
		ok := ring.WriteEntry(entry)
		assert.True(t, ok)
	}

	assert.Equal(t, uint64(20), ring.Size())

	batch1 := ring.DrainBatch(10)
	require.Len(t, batch1, 10)
	assert.Equal(t, int64(10), ring.Watermark())

	batch2 := ring.DrainBatch(20)
	require.Len(t, batch2, 10)
	assert.Equal(t, int64(20), ring.Watermark())
	assert.True(t, ring.IsEmpty())
}

func TestSHM_SegmentRolloverAndWatermarking(t *testing.T) {
	t.Parallel()

	const capacity = 16
	ring, err := NewInMemoryRingBuffer(capacity, 256)
	require.NoError(t, err)
	defer safeCloseRing(t, ring)

	for cycle := 0; cycle < 3; cycle++ {
		for i := 0; i < capacity; i++ {
			entry := createSampleEntry(fmt.Sprintf("roll-%d-%d", cycle, i), int64(cycle*capacity+i+1), 1)
			ok := ring.WriteEntry(entry)
			assert.True(t, ok)
		}
		batch := ring.DrainBatch(capacity)
		require.Len(t, batch, capacity)
	}

	assert.True(t, ring.RolloverCount() >= 2)
	assert.True(t, ring.Watermark() >= int64(3*capacity))
}

func TestSHM_VectorizedBatch_CapacityTrigger(t *testing.T) {
	t.Parallel()

	const batchCap = 5
	var mu sync.Mutex
	var batches [][]*CallbackEntry
	batchDelivered := make(chan struct{}, 10)

	cfg := DefaultVectorizedBatchConfig()
	cfg.BatchCapacity = batchCap
	cfg.FlushInterval = 10 * time.Minute
	cfg.Handler = func(ctx context.Context, batch []*CallbackEntry) error {
		mu.Lock()
		copied := make([]*CallbackEntry, len(batch))
		copy(copied, batch)
		batches = append(batches, copied)
		mu.Unlock()
		batchDelivered <- struct{}{}
		return nil
	}

	sub, err := NewVectorizedBatchSubscriber(context.Background(), cfg)
	require.NoError(t, err)
	defer safeCloseDispatcher(t, sub)

	for i := 0; i < 4; i++ {
		entry := &CallbackEntry{JobID: fmt.Sprintf("job-%d", i), Seq: int64(i)}
		require.NoError(t, sub.Notify(context.Background(), entry))
	}

	select {
	case <-batchDelivered:
		t.Fatalf("unexpected batch delivery before capacity reached")
	case <-time.After(50 * time.Millisecond):
	}

	entry5 := &CallbackEntry{JobID: "job-4", Seq: 4}
	require.NoError(t, sub.Notify(context.Background(), entry5))

	select {
	case <-batchDelivered:
		mu.Lock()
		assert.Len(t, batches, 1)
		assert.Len(t, batches[0], batchCap)
		mu.Unlock()
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timeout waiting for batch fullness delivery")
	}
}

func TestSHM_VectorizedBatch_TickerTrigger(t *testing.T) {
	t.Parallel()

	const batchCap = 100
	const flushInterval = 20 * time.Millisecond
	var mu sync.Mutex
	var batches [][]*CallbackEntry
	batchDelivered := make(chan struct{}, 10)

	cfg := DefaultVectorizedBatchConfig()
	cfg.BatchCapacity = batchCap
	cfg.FlushInterval = flushInterval
	cfg.Handler = func(ctx context.Context, batch []*CallbackEntry) error {
		mu.Lock()
		copied := make([]*CallbackEntry, len(batch))
		copy(copied, batch)
		batches = append(batches, copied)
		mu.Unlock()
		batchDelivered <- struct{}{}
		return nil
	}

	sub, err := NewVectorizedBatchSubscriber(context.Background(), cfg)
	require.NoError(t, err)
	defer safeCloseDispatcher(t, sub)

	for i := 0; i < 3; i++ {
		entry := &CallbackEntry{JobID: fmt.Sprintf("ticker-job-%d", i), Seq: int64(i)}
		require.NoError(t, sub.Notify(context.Background(), entry))
	}

	select {
	case <-batchDelivered:
		mu.Lock()
		assert.Len(t, batches, 1)
		assert.Len(t, batches[0], 3)
		mu.Unlock()
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timeout waiting for ticker flush delivery")
	}
}

func TestSHM_VectorizedBatch_RingBufferSink(t *testing.T) {
	t.Parallel()

	ring, err := NewInMemoryRingBuffer(64, 512)
	require.NoError(t, err)
	defer safeCloseRing(t, ring)

	cfg := DefaultVectorizedBatchConfig()
	cfg.BatchCapacity = 4
	cfg.FlushInterval = 20 * time.Millisecond
	cfg.RingBuffer = ring

	sub, err := NewVectorizedBatchSubscriber(context.Background(), cfg)
	require.NoError(t, err)
	defer safeCloseDispatcher(t, sub)

	for i := 0; i < 4; i++ {
		entry := &CallbackEntry{JobID: fmt.Sprintf("sink-job-%d", i), Seq: int64(i + 1)}
		require.NoError(t, sub.Notify(context.Background(), entry))
	}

	time.Sleep(50 * time.Millisecond)

	readEntries, rErr := ring.ReadBatch(10)
	require.NoError(t, rErr)
	require.Len(t, readEntries, 4)

	for i, entry := range readEntries {
		assert.Equal(t, fmt.Sprintf("sink-job-%d", i), entry.JobID)
	}
}

func spawnStressConsumers(ctx context.Context, ring *SharedMemoryRingBuffer, numConsumers, totalItems int, recMap *sync.Map, consTotal *atomic.Int64, wg *sync.WaitGroup) {
	wg.Add(numConsumers)
	for c := 0; c < numConsumers; c++ {
		goroutinelabels.NewGoroutine("test_shm_consumer", "concurrent stress consumer").
			WithWaitGroup(wg).
			StartWithContext(ctx, func(cCtx context.Context) error {
				for {
					if consTotal.Load() >= int64(totalItems) {
						return nil
					}
					slot, release, err := ring.ReserveRead()
					if err != nil {
						if errors.Is(err, ErrRingBufferEmpty) {
							select {
							case <-cCtx.Done():
								return nil
							default:
								time.Sleep(100 * time.Microsecond)
								continue
							}
						}
						if errors.Is(err, ErrRingBufferClosed) {
							return nil
						}
						return err
					}

					var val int
					parsed, scanErr := fmt.Sscanf(string(slot), "item-%d", &val)
					release()
					if scanErr == nil && parsed == 1 {
						recMap.Store(val, true)
						consTotal.Add(1)
					}
				}
			})
	}
}

func spawnStressProducers(ctx context.Context, ring *SharedMemoryRingBuffer, numProducers, itemsPerProducer int, wg *sync.WaitGroup) {
	wg.Add(numProducers)
	for p := 0; p < numProducers; p++ {
		producerID := p
		goroutinelabels.NewGoroutine("test_shm_producer", "concurrent stress producer").
			WithWaitGroup(wg).
			StartWithContext(ctx, func(pCtx context.Context) error {
				start := producerID * itemsPerProducer
				end := start + itemsPerProducer
				for i := start; i < end; i++ {
					msg := fmt.Sprintf("item-%d", i)
					for {
						slot, commit, err := ring.Reserve(uint32(len(msg)))
						if err != nil {
							if errors.Is(err, ErrRingBufferFull) {
								select {
								case <-pCtx.Done():
									return nil
								default:
									time.Sleep(50 * time.Microsecond)
									continue
								}
							}
							return err
						}
						copy(slot, msg)
						commit()
						break
					}
				}
				return nil
			})
	}
}

func runConcurrentStress(t *testing.T, ring *SharedMemoryRingBuffer) {
	const numProducers = 4
	const numConsumers = 4
	const itemsPerProducer = 250
	const totalItems = numProducers * itemsPerProducer

	var receivedMap sync.Map
	var consumedTotal atomic.Int64

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wgConsumers sync.WaitGroup
	spawnStressConsumers(ctx, ring, numConsumers, totalItems, &receivedMap, &consumedTotal, &wgConsumers)

	var wgProducers sync.WaitGroup
	spawnStressProducers(ctx, ring, numProducers, itemsPerProducer, &wgProducers)

	wgProducers.Wait()
	wgConsumers.Wait()

	assert.Equal(t, int64(totalItems), consumedTotal.Load())
}

func TestSHM_ConcurrentMultiProducerMultiConsumer_RaceDetector(t *testing.T) {
	t.Parallel()

	t.Run("concurrent_in_memory", func(t *testing.T) {
		ring, err := NewInMemoryRingBuffer(128, 64)
		require.NoError(t, err)
		defer safeCloseRing(t, ring)
		runConcurrentStress(t, ring)
	})

	t.Run("concurrent_mmap", func(t *testing.T) {
		tmpDir := t.TempDir()
		shmPath := filepath.Join(tmpDir, "stress_shm.seg")
		ring, err := OpenMemoryMappedRingBuffer(shmPath, 128, 64)
		require.NoError(t, err)
		defer safeCloseRing(t, ring)
		runConcurrentStress(t, ring)
	})
}

func TestSHM_ZeroAllocationHotPath(t *testing.T) {
	t.Run("reserve_and_release_hot_path_zero_allocs", func(t *testing.T) {
		ring, err := NewInMemoryRingBuffer(128, 64)
		require.NoError(t, err)
		defer safeCloseRing(t, ring)

		payload := []byte("hot-path-bench-bytes")
		payloadSize := uint32(len(payload))

		allocs := testing.AllocsPerRun(1000, func() {
			slot, commit, rErr := ring.Reserve(payloadSize)
			if rErr != nil {
				t.Fatalf("reserve error: %v", rErr)
			}
			copy(slot, payload)
			commit()

			readSlot, release, rdErr := ring.ReserveRead()
			if rdErr != nil {
				t.Fatalf("read error: %v", rdErr)
			}
			if len(readSlot) == 0 {
				t.Fatalf("empty payload")
			}
			release()
		})

		assert.Equal(t, 0.0, allocs)
	})

	t.Run("vectorized_subscriber_notify_hot_path_zero_allocs", func(t *testing.T) {
		cfg := DefaultVectorizedBatchConfig()
		cfg.QueueCapacity = 2000
		cfg.BatchCapacity = 500
		cfg.FlushInterval = 1 * time.Hour

		sub, err := NewVectorizedBatchSubscriber(context.Background(), cfg)
		require.NoError(t, err)
		defer safeCloseDispatcher(t, sub)

		entry := &CallbackEntry{
			JobID:    "bench-job-1",
			Priority: 10,
			Seq:      42,
		}

		ctx := context.Background()
		allocs := testing.AllocsPerRun(1000, func() {
			notifyErr := sub.Notify(ctx, entry)
			if notifyErr != nil {
				t.Fatalf("notify error: %v", notifyErr)
			}
		})

		assert.Equal(t, 0.0, allocs)
	})
}

func TestSHM_ErrorBoundaries_TruncatedFiles(t *testing.T) {
	t.Parallel()

	t.Run("truncated_file_below_header_size", func(t *testing.T) {
		tmpDir := t.TempDir()
		shmPath := filepath.Join(tmpDir, "short_header.seg")
		createTestFile(t, shmPath, make([]byte, 30))

		_, err := OpenExistingMemoryMappedRingBuffer(shmPath)
		assert.True(t, errors.Is(err, ErrTruncatedFile))
	})

	t.Run("truncated_file_below_total_payload_size", func(t *testing.T) {
		tmpDir := t.TempDir()
		shmPath := filepath.Join(tmpDir, "truncated_body.seg")

		headerBuf := make([]byte, 100)
		var h SHMHeader
		copy(h.Magic[:], SHMMagic)
		h.Version = SHMVersion
		h.HeaderSize = SHMHeaderSize
		h.SlotCapacity = 64
		h.SlotPayloadSize = 128
		h.SlotSize = calculateAlignedSlotSize(128)
		writeHeader(headerBuf, &h)

		createTestFile(t, shmPath, headerBuf)

		_, err := OpenExistingMemoryMappedRingBuffer(shmPath)
		assert.True(t, errors.Is(err, ErrTruncatedFile))
	})
}

func TestSHM_ErrorBoundaries_MagicAndChecksum(t *testing.T) {
	t.Parallel()

	t.Run("invalid_magic_bytes", func(t *testing.T) {
		tmpDir := t.TempDir()
		shmPath := filepath.Join(tmpDir, "bad_magic.seg")

		buf := make([]byte, 2048)
		copy(buf[0:8], "BADMAGIC")
		binary.LittleEndian.PutUint32(buf[8:12], SHMVersion)
		binary.LittleEndian.PutUint32(buf[12:16], SHMHeaderSize)
		binary.LittleEndian.PutUint32(buf[16:20], 8)
		binary.LittleEndian.PutUint32(buf[20:24], 64)
		binary.LittleEndian.PutUint32(buf[24:28], calculateAlignedSlotSize(64))
		binary.LittleEndian.PutUint32(buf[32:36], crc32.ChecksumIEEE(buf[:32]))

		createTestFile(t, shmPath, buf)

		_, err := OpenExistingMemoryMappedRingBuffer(shmPath)
		assert.True(t, errors.Is(err, ErrInvalidMagic))
		assert.True(t, errors.Is(err, ErrInvalidSHMMagic))
	})

	t.Run("corrupted_header_checksum", func(t *testing.T) {
		tmpDir := t.TempDir()
		shmPath := filepath.Join(tmpDir, "bad_checksum.seg")

		buf := make([]byte, 2048)
		var h SHMHeader
		copy(h.Magic[:], SHMMagic)
		h.Version = SHMVersion
		h.HeaderSize = SHMHeaderSize
		h.SlotCapacity = 8
		h.SlotPayloadSize = 64
		h.SlotSize = calculateAlignedSlotSize(64)
		writeHeader(buf, &h)

		buf[10] ^= 0xFF
		createTestFile(t, shmPath, buf)

		_, err := OpenExistingMemoryMappedRingBuffer(shmPath)
		assert.True(t, errors.Is(err, ErrCorruptedHeader))
	})
}

func TestSHM_ErrorBoundaries_CapacityAndBufferLimits(t *testing.T) {
	t.Parallel()

	t.Run("invalid_capacity_non_power_of_two", func(t *testing.T) {
		_, err := NewInMemoryRingBuffer(100, 64)
		assert.True(t, errors.Is(err, ErrInvalidCapacity))
	})

	t.Run("payload_exceeds_slot_size", func(t *testing.T) {
		ring, err := NewInMemoryRingBuffer(16, 64)
		require.NoError(t, err)
		defer safeCloseRing(t, ring)

		_, _, rErr := ring.Reserve(100)
		assert.True(t, errors.Is(rErr, ErrPayloadTooLarge))
	})

	t.Run("ring_buffer_saturation_full", func(t *testing.T) {
		ring, err := NewInMemoryRingBuffer(4, 32)
		require.NoError(t, err)
		defer safeCloseRing(t, ring)

		for i := 0; i < 4; i++ {
			slot, commit, rErr := ring.Reserve(10)
			require.NoError(t, rErr)
			copy(slot, "item")
			commit()
		}

		assert.True(t, ring.IsFull())

		_, _, fullErr := ring.Reserve(10)
		assert.True(t, errors.Is(fullErr, ErrRingBufferFull))
	})

	t.Run("empty_ring_buffer_read", func(t *testing.T) {
		ring, err := NewInMemoryRingBuffer(4, 32)
		require.NoError(t, err)
		defer safeCloseRing(t, ring)

		_, _, emptyErr := ring.ReserveRead()
		assert.True(t, errors.Is(emptyErr, ErrRingBufferEmpty))
	})
}

func TestSHM_ErrorBoundaries_ClosedAndNil(t *testing.T) {
	t.Parallel()

	t.Run("nil_callback_entry_boundary", func(t *testing.T) {
		cfg := DefaultVectorizedBatchConfig()
		sub, err := NewVectorizedBatchSubscriber(context.Background(), cfg)
		require.NoError(t, err)
		defer safeCloseDispatcher(t, sub)

		nilErr := sub.Notify(context.Background(), nil)
		assert.True(t, errors.Is(nilErr, ErrNilCallbackEntry))
	})

	t.Run("closed_ring_and_subscriber_boundaries", func(t *testing.T) {
		ring, err := NewInMemoryRingBuffer(16, 64)
		require.NoError(t, err)
		require.NoError(t, ring.Close())

		_, _, closedErr := ring.Reserve(10)
		assert.True(t, errors.Is(closedErr, ErrRingBufferClosed))

		_, _, closedReadErr := ring.ReserveRead()
		assert.True(t, errors.Is(closedReadErr, ErrRingBufferClosed))

		cfg := DefaultVectorizedBatchConfig()
		sub, err := NewVectorizedBatchSubscriber(context.Background(), cfg)
		require.NoError(t, err)
		require.NoError(t, sub.Close())

		subErr := sub.Notify(context.Background(), &CallbackEntry{JobID: "test"})
		assert.True(t, errors.Is(subErr, ErrSubscriberClosed))
	})
}

func TestSHM_CleanShutdownAndZeroLeaks(t *testing.T) {
	t.Parallel()

	for iteration := 0; iteration < 5; iteration++ {
		var received atomic.Int64
		cfg := DefaultVectorizedBatchConfig()
		cfg.BatchCapacity = 16
		cfg.FlushInterval = 5 * time.Millisecond
		cfg.Handler = func(ctx context.Context, batch []*CallbackEntry) error {
			received.Add(int64(len(batch)))
			return nil
		}

		sub, err := NewVectorizedBatchSubscriber(context.Background(), cfg)
		require.NoError(t, err)

		const total = 50
		for i := 0; i < total; i++ {
			entry := &CallbackEntry{JobID: fmt.Sprintf("leak-test-%d", i), Seq: int64(i)}
			require.NoError(t, sub.Notify(context.Background(), entry))
		}

		require.NoError(t, sub.Close())
		assert.Equal(t, int64(total), received.Load())
	}
}

func TestSHM_SegmentRecovery(t *testing.T) {
	t.Parallel()

	seg := NewSharedMemorySegment(0, 32, 128)
	assert.False(t, seg.IsCorrupted())

	seg.MarkCorrupted()
	assert.True(t, seg.IsCorrupted())

	recErr := seg.Recover()
	assert.NoError(t, recErr)
	assert.False(t, seg.IsCorrupted())
}
