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

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSHM_SingleProducerSingleConsumer_FIFOAndZeroCopy(t *testing.T) {
	t.Parallel()

	t.Run("in_memory_fifo_and_zero_copy", func(t *testing.T) {
		const capacity = 64
		const payloadSize = 256
		ring, err := NewInMemoryRingBuffer(capacity, payloadSize)
		if err != nil {
			t.Fatalf("failed to create in-memory ring buffer: %v", err)
		}
		defer func() { _ = ring.Close() }()

		if ring.Capacity() != capacity {
			t.Fatalf("expected capacity %d, got %d", capacity, ring.Capacity())
		}
		if ring.SlotPayloadSize() != payloadSize {
			t.Fatalf("expected slot payload size %d, got %d", payloadSize, ring.SlotPayloadSize())
		}

		const totalEntries = 200 // > 3x capacity to test wraparound
		for i := 0; i < totalEntries; i++ {
			msg := fmt.Sprintf("payload-%06d", i)
			slot, commit, reserveErr := ring.Reserve(uint32(len(msg)))
			if reserveErr != nil {
				t.Fatalf("reserve error at index %d: %v", i, reserveErr)
			}

			// Verify zero-copy: address of slot[0] resides within ring.data backing buffer
			slotIdx := int(ring.Tail()-1) & int(ring.mask)
			expectedSlotStart := uintptr(unsafe.Pointer(&ring.slots[slotIdx].payload[0]))
			actualSlotStart := uintptr(unsafe.Pointer(&slot[0]))
			if expectedSlotStart != actualSlotStart {
				t.Fatalf("zero-copy violation at index %d: expected %x, got %x", i, expectedSlotStart, actualSlotStart)
			}

			copy(slot, msg)
			commit()

			// Immediately read back to keep ring bounded
			readSlot, release, readErr := ring.ReserveRead()
			if readErr != nil {
				t.Fatalf("read error at index %d: %v", i, readErr)
			}
			if string(readSlot) != msg {
				t.Fatalf("payload mismatch: expected %q, got %q", msg, string(readSlot))
			}
			release()
		}

		if !ring.IsEmpty() {
			t.Fatalf("expected ring to be empty, size is %d", ring.Size())
		}
	})

	t.Run("mmap_fifo_and_persistence", func(t *testing.T) {
		tmpDir := t.TempDir()
		shmPath := filepath.Join(tmpDir, "test_shm.seg")

		const capacity = 32
		const payloadSize = 128
		ring, err := OpenMemoryMappedRingBuffer(shmPath, capacity, payloadSize)
		if err != nil {
			t.Fatalf("failed to open memory-mapped ring buffer: %v", err)
		}

		const count = 50
		for i := 0; i < count; i++ {
			msg := fmt.Sprintf("persisted-%04d", i)
			slot, commit, rErr := ring.Reserve(uint32(len(msg)))
			if rErr != nil {
				t.Fatalf("reserve error at %d: %v", i, rErr)
			}
			copy(slot, msg)
			commit()

			readSlot, release, rdErr := ring.ReserveRead()
			if rdErr != nil {
				t.Fatalf("read error at %d: %v", i, rdErr)
			}
			if string(readSlot) != msg {
				t.Fatalf("read mismatch: expected %q, got %q", msg, string(readSlot))
			}
			release()
		}

		// Write two entries and close without reading
		slot1, commit1, _ := ring.Reserve(uint32(len("item1")))
		copy(slot1, "item1")
		commit1()

		slot2, commit2, _ := ring.Reserve(uint32(len("item2")))
		copy(slot2, "item2")
		commit2()

		if err := ring.Close(); err != nil {
			t.Fatalf("failed to close ring buffer: %v", err)
		}

		// Reopen existing segment file and read the remaining entries
		reopened, err := OpenExistingMemoryMappedRingBuffer(shmPath)
		if err != nil {
			t.Fatalf("failed to reopen existing shm segment: %v", err)
		}
		defer func() { _ = reopened.Close() }()

		if reopened.Size() != 2 {
			t.Fatalf("expected size 2 in reopened segment, got %d", reopened.Size())
		}

		r1, rel1, err1 := reopened.ReserveRead()
		if err1 != nil || string(r1) != "item1" {
			t.Fatalf("expected item1, got %q (err=%v)", string(r1), err1)
		}
		rel1()

		r2, rel2, err2 := reopened.ReserveRead()
		if err2 != nil || string(r2) != "item2" {
			t.Fatalf("expected item2, got %q (err=%v)", string(r2), err2)
		}
		rel2()

		if !reopened.IsEmpty() {
			t.Fatalf("expected reopened ring to be empty after reads")
		}
	})

	t.Run("write_batch_and_read_batch", func(t *testing.T) {
		ring, err := NewInMemoryRingBuffer(64, 512)
		if err != nil {
			t.Fatalf("failed to create ring: %v", err)
		}
		defer func() { _ = ring.Close() }()

		entries := []CallbackEntry{
			{JobID: "job-1", Priority: 10, Seq: 101, Payload: map[string]any{"action": "start"}},
			{JobID: "job-2", Priority: 20, Seq: 102, Payload: map[string]any{"action": "progress"}},
			{JobID: "job-3", Priority: 30, Seq: 103, Payload: map[string]any{"action": "complete"}},
		}

		written, err := ring.WriteBatch(entries)
		if err != nil {
			t.Fatalf("write batch failed: %v", err)
		}
		if written != len(entries) {
			t.Fatalf("expected %d written, got %d", len(entries), written)
		}

		readEntries, err := ring.ReadBatch(10)
		if err != nil {
			t.Fatalf("read batch failed: %v", err)
		}
		if len(readEntries) != len(entries) {
			t.Fatalf("expected %d entries read, got %d", len(entries), len(readEntries))
		}

		for i, expected := range entries {
			actual := readEntries[i]
			if actual.JobID != expected.JobID || actual.Priority != expected.Priority || actual.Seq != expected.Seq {
				t.Fatalf("entry %d mismatch: expected %+v, got %+v", i, expected, actual)
			}
			if actual.Payload["action"] != expected.Payload["action"] {
				t.Fatalf("payload mismatch at %d: expected %v, got %v", i, expected.Payload["action"], actual.Payload["action"])
			}
		}
	})
}

func TestSHM_VectorizedBatchDelivery_Triggers(t *testing.T) {
	t.Parallel()

	t.Run("batch_capacity_fullness_trigger", func(t *testing.T) {
		const batchCap = 5
		var mu sync.Mutex
		var batches [][]*CallbackEntry

		batchDelivered := make(chan struct{}, 10)

		cfg := DefaultVectorizedBatchConfig()
		cfg.BatchCapacity = batchCap
		cfg.FlushInterval = 10 * time.Minute // Large timeout so only capacity triggers flush
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
		if err != nil {
			t.Fatalf("failed to create subscriber: %v", err)
		}
		defer func() { _ = sub.Close() }()

		// Send 4 entries (not reaching capacity 5)
		for i := 0; i < 4; i++ {
			entry := &CallbackEntry{JobID: fmt.Sprintf("job-%d", i), Seq: int64(i)}
			if err := sub.Notify(context.Background(), entry); err != nil {
				t.Fatalf("notify error: %v", err)
			}
		}

		// Verify no batch triggered yet
		select {
		case <-batchDelivered:
			t.Fatalf("unexpected batch delivery before reaching capacity")
		case <-time.After(50 * time.Millisecond):
			// Expected: no batch yet
		}

		// Send 5th entry (reaches capacity 5)
		entry5 := &CallbackEntry{JobID: "job-4", Seq: 4}
		if err := sub.Notify(context.Background(), entry5); err != nil {
			t.Fatalf("notify error: %v", err)
		}

		// Verify immediate batch delivery
		select {
		case <-batchDelivered:
			mu.Lock()
			if len(batches) != 1 {
				mu.Unlock()
				t.Fatalf("expected 1 batch, got %d", len(batches))
			}
			if len(batches[0]) != batchCap {
				mu.Unlock()
				t.Fatalf("expected batch of %d entries, got %d", batchCap, len(batches[0]))
			}
			mu.Unlock()
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timeout waiting for batch fullness delivery")
		}
	})

	t.Run("ticker_timeout_flush_trigger", func(t *testing.T) {
		const batchCap = 100 // High capacity so ticker flushes first
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
		if err != nil {
			t.Fatalf("failed to create subscriber: %v", err)
		}
		defer func() { _ = sub.Close() }()

		// Send 3 entries
		for i := 0; i < 3; i++ {
			entry := &CallbackEntry{JobID: fmt.Sprintf("ticker-job-%d", i), Seq: int64(i)}
			if err := sub.Notify(context.Background(), entry); err != nil {
				t.Fatalf("notify error: %v", err)
			}
		}

		// Wait for ticker flush
		select {
		case <-batchDelivered:
			mu.Lock()
			if len(batches) != 1 {
				mu.Unlock()
				t.Fatalf("expected 1 batch from ticker flush, got %d", len(batches))
			}
			if len(batches[0]) != 3 {
				mu.Unlock()
				t.Fatalf("expected 3 entries in flushed batch, got %d", len(batches[0]))
			}
			mu.Unlock()
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timeout waiting for ticker flush delivery")
		}
	})

	t.Run("subscriber_with_ring_buffer_sink", func(t *testing.T) {
		ring, err := NewInMemoryRingBuffer(64, 512)
		if err != nil {
			t.Fatalf("failed to create ring: %v", err)
		}
		defer func() { _ = ring.Close() }()

		cfg := DefaultVectorizedBatchConfig()
		cfg.BatchCapacity = 4
		cfg.FlushInterval = 20 * time.Millisecond
		cfg.RingBuffer = ring

		sub, err := NewVectorizedBatchSubscriber(context.Background(), cfg)
		if err != nil {
			t.Fatalf("failed to create subscriber: %v", err)
		}
		defer func() { _ = sub.Close() }()

		for i := 0; i < 4; i++ {
			entry := &CallbackEntry{JobID: fmt.Sprintf("sink-job-%d", i), Seq: int64(i + 1)}
			if err := sub.Notify(context.Background(), entry); err != nil {
				t.Fatalf("notify error: %v", err)
			}
		}

		// Wait briefly for worker to write to ring buffer
		time.Sleep(50 * time.Millisecond)

		readEntries, err := ring.ReadBatch(10)
		if err != nil {
			t.Fatalf("read batch error: %v", err)
		}
		if len(readEntries) != 4 {
			t.Fatalf("expected 4 entries written into ring buffer, got %d", len(readEntries))
		}
		for i, entry := range readEntries {
			expectedJobID := fmt.Sprintf("sink-job-%d", i)
			if entry.JobID != expectedJobID {
				t.Fatalf("expected %q, got %q", expectedJobID, entry.JobID)
			}
		}
	})
}

func TestSHM_ConcurrentMultiProducerMultiConsumer_RaceDetector(t *testing.T) {
	t.Parallel()

	runConcurrentStress := func(t *testing.T, ring *SharedMemoryRingBuffer) {
		const numProducers = 4
		const numConsumers = 4
		const itemsPerProducer = 250
		const totalItems = numProducers * itemsPerProducer

		var receivedMap sync.Map
		var consumedTotal atomic.Int64

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var wgConsumers sync.WaitGroup
		for c := 0; c < numConsumers; c++ {
			goroutinelabels.NewGoroutine("test_shm_consumer", "concurrent stress consumer").
				WithWaitGroup(&wgConsumers).
				StartWithContext(ctx, func(cCtx context.Context) error {
					for {
						if consumedTotal.Load() >= totalItems {
							return nil
						}
						slot, release, err := ring.ReserveRead()
						if err != nil {
							if errors.Is(err, ErrRingBufferEmpty) {
								if consumedTotal.Load() >= totalItems {
									return nil
								}
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
						_, scanErr := fmt.Sscanf(string(slot), "item-%d", &val)
						release()
						if scanErr == nil {
							if _, loaded := receivedMap.LoadOrStore(val, true); loaded {
								t.Errorf("duplicate item %d detected", val)
							}
							consumedTotal.Add(1)
						}
					}
				})
		}

		var wgProducers sync.WaitGroup
		for p := 0; p < numProducers; p++ {
			producerID := p
			goroutinelabels.NewGoroutine("test_shm_producer", "concurrent stress producer").
				WithWaitGroup(&wgProducers).
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

		wgProducers.Wait()
		wgConsumers.Wait()

		if consumedTotal.Load() != totalItems {
			t.Fatalf("expected %d total consumed items, got %d", totalItems, consumedTotal.Load())
		}
	}

	t.Run("concurrent_in_memory", func(t *testing.T) {
		ring, err := NewInMemoryRingBuffer(128, 64)
		if err != nil {
			t.Fatalf("failed to create in-memory ring: %v", err)
		}
		defer func() { _ = ring.Close() }()
		runConcurrentStress(t, ring)
	})

	t.Run("concurrent_mmap", func(t *testing.T) {
		tmpDir := t.TempDir()
		shmPath := filepath.Join(tmpDir, "stress_shm.seg")
		ring, err := OpenMemoryMappedRingBuffer(shmPath, 128, 64)
		if err != nil {
			t.Fatalf("failed to create mmap ring: %v", err)
		}
		defer func() { _ = ring.Close() }()
		runConcurrentStress(t, ring)
	})
}

func TestSHM_ZeroAllocationHotPath(t *testing.T) {
	t.Run("reserve_and_release_hot_path_zero_allocs", func(t *testing.T) {
		ring, err := NewInMemoryRingBuffer(128, 64)
		if err != nil {
			t.Fatalf("failed to create ring: %v", err)
		}
		defer func() { _ = ring.Close() }()

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
			_ = readSlot
			release()
		})

		if allocs != 0.0 {
			t.Fatalf("expected 0.0 allocs/op in ring buffer hot path, got %.2f", allocs)
		}
	})

	t.Run("vectorized_subscriber_notify_hot_path_zero_allocs", func(t *testing.T) {
		cfg := DefaultVectorizedBatchConfig()
		cfg.QueueCapacity = 2000
		cfg.BatchCapacity = 500
		cfg.FlushInterval = 1 * time.Hour // Do not trigger ticker during micro-alloc check

		sub, err := NewVectorizedBatchSubscriber(context.Background(), cfg)
		if err != nil {
			t.Fatalf("failed to create subscriber: %v", err)
		}
		defer func() { _ = sub.Close() }()

		entry := &CallbackEntry{
			JobID:    "bench-job-1",
			Priority: 10,
			Seq:      42,
		}

		ctx := context.Background()
		allocs := testing.AllocsPerRun(1000, func() {
			if notifyErr := sub.Notify(ctx, entry); notifyErr != nil {
				t.Fatalf("notify error: %v", notifyErr)
			}
		})

		if allocs != 0.0 {
			t.Fatalf("expected 0.0 allocs/op in subscriber Notify hot path, got %.2f", allocs)
		}
	})
}

func TestSHM_ErrorBoundaries_HeaderAndCorruption(t *testing.T) {
	t.Parallel()

	t.Run("truncated_file_below_header_size", func(t *testing.T) {
		tmpDir := t.TempDir()
		shmPath := filepath.Join(tmpDir, "short_header.seg")
		if err := fileutil.WriteFile(shmPath, make([]byte, 30), 0644); err != nil {
			t.Fatalf("failed to write truncated file: %v", err)
		}

		_, err := OpenExistingMemoryMappedRingBuffer(shmPath)
		if !errors.Is(err, ErrTruncatedFile) {
			t.Fatalf("expected ErrTruncatedFile, got: %v", err)
		}
	})

	t.Run("truncated_file_below_total_payload_size", func(t *testing.T) {
		tmpDir := t.TempDir()
		shmPath := filepath.Join(tmpDir, "truncated_body.seg")

		// Prepare valid 64-byte header
		headerBuf := make([]byte, 100)
		var h SHMHeader
		copy(h.Magic[:], SHMMagic)
		h.Version = SHMVersion
		h.HeaderSize = SHMHeaderSize
		h.SlotCapacity = 64
		h.SlotPayloadSize = 128
		h.SlotSize = calculateAlignedSlotSize(128)
		writeHeader(headerBuf, &h)

		if err := fileutil.WriteFile(shmPath, headerBuf, 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		_, err := OpenExistingMemoryMappedRingBuffer(shmPath)
		if !errors.Is(err, ErrTruncatedFile) {
			t.Fatalf("expected ErrTruncatedFile, got: %v", err)
		}
	})

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

		if err := fileutil.WriteFile(shmPath, buf, 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		_, err := OpenExistingMemoryMappedRingBuffer(shmPath)
		if !errors.Is(err, ErrInvalidMagic) {
			t.Fatalf("expected ErrInvalidMagic, got: %v", err)
		}
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

		// Corrupt one byte in the header
		buf[10] ^= 0xFF

		if err := fileutil.WriteFile(shmPath, buf, 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		_, err := OpenExistingMemoryMappedRingBuffer(shmPath)
		if !errors.Is(err, ErrCorruptedHeader) {
			t.Fatalf("expected ErrCorruptedHeader, got: %v", err)
		}
	})

	t.Run("invalid_capacity_non_power_of_two", func(t *testing.T) {
		_, err := NewInMemoryRingBuffer(100, 64)
		if !errors.Is(err, ErrInvalidCapacity) {
			t.Fatalf("expected ErrInvalidCapacity, got: %v", err)
		}
	})

	t.Run("payload_exceeds_slot_size", func(t *testing.T) {
		ring, err := NewInMemoryRingBuffer(16, 64)
		if err != nil {
			t.Fatalf("failed to create ring: %v", err)
		}
		defer func() { _ = ring.Close() }()

		_, _, rErr := ring.Reserve(100)
		if !errors.Is(rErr, ErrPayloadTooLarge) {
			t.Fatalf("expected ErrPayloadTooLarge, got: %v", rErr)
		}
	})

	t.Run("ring_buffer_saturation_full", func(t *testing.T) {
		ring, err := NewInMemoryRingBuffer(4, 32)
		if err != nil {
			t.Fatalf("failed to create ring: %v", err)
		}
		defer func() { _ = ring.Close() }()

		for i := 0; i < 4; i++ {
			slot, commit, rErr := ring.Reserve(10)
			if rErr != nil {
				t.Fatalf("unexpected reserve error at %d: %v", i, rErr)
			}
			copy(slot, "item")
			commit()
		}

		if !ring.IsFull() {
			t.Fatalf("expected ring buffer to be full")
		}

		// 5th reservation must fail fail-closed
		_, _, fullErr := ring.Reserve(10)
		if !errors.Is(fullErr, ErrRingBufferFull) {
			t.Fatalf("expected ErrRingBufferFull, got: %v", fullErr)
		}
	})

	t.Run("empty_ring_buffer_read", func(t *testing.T) {
		ring, err := NewInMemoryRingBuffer(4, 32)
		if err != nil {
			t.Fatalf("failed to create ring: %v", err)
		}
		defer func() { _ = ring.Close() }()

		_, _, emptyErr := ring.ReserveRead()
		if !errors.Is(emptyErr, ErrRingBufferEmpty) {
			t.Fatalf("expected ErrRingBufferEmpty, got: %v", emptyErr)
		}
	})

	t.Run("nil_callback_entry_boundary", func(t *testing.T) {
		cfg := DefaultVectorizedBatchConfig()
		sub, err := NewVectorizedBatchSubscriber(context.Background(), cfg)
		if err != nil {
			t.Fatalf("failed to create subscriber: %v", err)
		}
		defer func() { _ = sub.Close() }()

		nilErr := sub.Notify(context.Background(), nil)
		if !errors.Is(nilErr, ErrNilCallbackEntry) {
			t.Fatalf("expected ErrNilCallbackEntry, got: %v", nilErr)
		}
	})

	t.Run("closed_ring_and_subscriber_boundaries", func(t *testing.T) {
		ring, err := NewInMemoryRingBuffer(16, 64)
		if err != nil {
			t.Fatalf("failed to create ring: %v", err)
		}
		if err := ring.Close(); err != nil {
			t.Fatalf("failed to close ring: %v", err)
		}
		_, _, closedErr := ring.Reserve(10)
		if !errors.Is(closedErr, ErrRingBufferClosed) {
			t.Fatalf("expected ErrRingBufferClosed from Reserve, got: %v", closedErr)
		}
		_, _, closedReadErr := ring.ReserveRead()
		if !errors.Is(closedReadErr, ErrRingBufferClosed) {
			t.Fatalf("expected ErrRingBufferClosed from ReserveRead, got: %v", closedReadErr)
		}

		cfg := DefaultVectorizedBatchConfig()
		sub, err := NewVectorizedBatchSubscriber(context.Background(), cfg)
		if err != nil {
			t.Fatalf("failed to create subscriber: %v", err)
		}
		if err := sub.Close(); err != nil {
			t.Fatalf("failed to close subscriber: %v", err)
		}
		subErr := sub.Notify(context.Background(), &CallbackEntry{JobID: "test"})
		if !errors.Is(subErr, ErrSubscriberClosed) {
			t.Fatalf("expected ErrSubscriberClosed from Notify, got: %v", subErr)
		}
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
		if err != nil {
			t.Fatalf("iteration %d: failed to create subscriber: %v", iteration, err)
		}

		const total = 50
		for i := 0; i < total; i++ {
			entry := &CallbackEntry{JobID: fmt.Sprintf("leak-test-%d", i), Seq: int64(i)}
			if err := sub.Notify(context.Background(), entry); err != nil {
				t.Fatalf("iteration %d: notify error: %v", iteration, err)
			}
		}

		if err := sub.Close(); err != nil {
			t.Fatalf("iteration %d: close error: %v", iteration, err)
		}

		if received.Load() != total {
			t.Fatalf("iteration %d: expected %d received, got %d", iteration, total, received.Load())
		}
	}
}
