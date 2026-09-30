package validation

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

func TestOutputQueue_EnqueueDequeue(t *testing.T) {
	t.Parallel()
	queue := NewOutputQueue(0) // Unbounded

	// Enqueue a packet
	packet := OutputPacket{
		Data:      "test data",
		ChannelID: "test",
		FlushHint: true,
	}

	if err := queue.Enqueue(packet); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Dequeue should return the same packet
	dequeued := queue.Dequeue()
	if dequeued.Data != packet.Data {
		t.Errorf("Expected data %v, got %v", packet.Data, dequeued.Data)
	}
	if dequeued.ChannelID != packet.ChannelID {
		t.Errorf("Expected channel %v, got %v", packet.ChannelID, dequeued.ChannelID)
	}
	if dequeued.FlushHint != packet.FlushHint {
		t.Errorf("Expected flush hint %v, got %v", packet.FlushHint, dequeued.FlushHint)
	}
}

func TestOutputQueue_FIFO(t *testing.T) {
	t.Parallel()
	queue := NewOutputQueue(0)

	// Enqueue multiple packets
	for i := 0; i < 100; i++ {
		packet := OutputPacket{
			Data:      fmt.Sprintf("data-%d", i),
			ChannelID: "test",
		}
		if err := queue.Enqueue(packet); err != nil {
			t.Fatalf("Enqueue %d failed: %v", i, err)
		}
	}

	// Dequeue should return in FIFO order
	for i := 0; i < 100; i++ {
		dequeued := queue.Dequeue()
		expected := fmt.Sprintf("data-%d", i)
		if dequeued.Data != expected {
			t.Errorf("Expected %s, got %v", expected, dequeued.Data)
		}
	}
}

func TestOutputQueue_BlockingDequeue(t *testing.T) {
	t.Parallel()
	queue := NewOutputQueue(0)

	// Start goroutine that will block on dequeue
	dequeued := make(chan OutputPacket, 1)
	done := make(chan struct{})
	started := make(chan struct{})
	goroutinelabels.StartTestGoroutine("test_dequeue", "dequeuing from output queue in test", func() {
		defer close(done)
		close(started)
		dequeued <- queue.Dequeue()
	})

	<-started

	// Enqueue should unblock the goroutine
	packet := OutputPacket{Data: "unblock", ChannelID: "test"}
	if err := queue.Enqueue(packet); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Wait for dequeue to complete
	select {
	case result := <-dequeued:
		if result.Data != packet.Data {
			t.Errorf("Expected %v, got %v", packet.Data, result.Data)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Dequeue did not unblock")
	}

	select {
	case <-done:
		// Good
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Goroutine did not finish")
	}
}

func TestOutputQueue_NonBlockingDequeue(t *testing.T) {
	t.Parallel()
	queue := NewOutputQueue(0)

	// Dequeue from empty queue should return false
	_, ok := queue.DequeueNonBlocking()
	if ok {
		t.Error("Expected false for empty queue")
	}

	// Enqueue a packet
	packet := OutputPacket{Data: "test", ChannelID: "test"}
	if err := queue.Enqueue(packet); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Dequeue should return true
	dequeued, ok := queue.DequeueNonBlocking()
	if !ok {
		t.Error("Expected true for non-empty queue")
	}
	if dequeued.Data != packet.Data {
		t.Errorf("Expected %v, got %v", packet.Data, dequeued.Data)
	}
}

func TestOutputQueue_DequeueBatch(t *testing.T) {
	t.Parallel()
	queue := NewOutputQueue(0) // Unbounded

	// Enqueue multiple packets
	for i := 0; i < 50; i++ {
		packet := OutputPacket{
			Data:      fmt.Sprintf("data-%d", i),
			ChannelID: "test",
		}
		if err := queue.Enqueue(packet); err != nil {
			t.Fatalf("Failed to enqueue packet %d: %v", i, err)
		}
	}

	// Dequeue batch of 20 items
	batch, ok := queue.DequeueBatch(20)
	if !ok {
		t.Fatal("Expected batch to be dequeued")
	}
	if len(batch) != 20 {
		t.Fatalf("Expected batch size 20, got %d", len(batch))
	}

	// Verify FIFO order
	for i := 0; i < 20; i++ {
		expected := fmt.Sprintf("data-%d", i)
		if batch[i].Data != expected {
			t.Errorf("Batch[%d] = %v, want %v", i, batch[i].Data, expected)
		}
	}

	// Dequeue remaining items
	batch2, ok := queue.DequeueBatch(50)
	if !ok {
		t.Fatal("Expected second batch to be dequeued")
	}
	if len(batch2) != 30 {
		t.Fatalf("Expected second batch size 30, got %d", len(batch2))
	}

	// Verify remaining items
	for i := 0; i < 30; i++ {
		expected := fmt.Sprintf("data-%d", i+20)
		if batch2[i].Data != expected {
			t.Errorf("Batch2[%d] = %v, want %v", i, batch2[i].Data, expected)
		}
	}

	// Queue should be empty now
	batch3, ok := queue.DequeueBatch(10)
	if ok {
		t.Fatal("Expected no batch to be dequeued from empty queue")
	}
	if batch3 != nil {
		t.Fatalf("Expected nil batch, got %v", batch3)
	}
}

func TestOutputQueue_SizeLimit(t *testing.T) {
	t.Parallel()
	queue := NewOutputQueue(10) // Max 10 packets

	// Fill queue to capacity
	for i := 0; i < 10; i++ {
		packet := OutputPacket{Data: fmt.Sprintf("data-%d", i), ChannelID: "test"}
		if err := queue.Enqueue(packet); err != nil {
			t.Fatalf("Enqueue %d failed: %v", i, err)
		}
	}

	// Next enqueue should fail
	packet := OutputPacket{Data: "overflow", ChannelID: "test"}
	err := queue.Enqueue(packet)
	if err != ErrQueueFull {
		t.Errorf("Expected ErrQueueFull, got %v", err)
	}

	// Size should be 10
	if size := queue.Size(); size != 10 {
		t.Errorf("Expected size 10, got %d", size)
	}
}

func TestOutputQueue_ConcurrentEnqueue(t *testing.T) {
	t.Parallel()
	queue := NewOutputQueue(0)
	numWorkers := 100
	packetsPerWorker := 100

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Start concurrent enqueuers
	for i := 0; i < numWorkers; i++ {
		goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
			func(workerID int) {
				defer wg.Done()
				for j := 0; j < packetsPerWorker; j++ {
					packet := OutputPacket{
						Data:      fmt.Sprintf("worker-%d-packet-%d", workerID, j),
						ChannelID: "test",
					}
					if err := queue.Enqueue(packet); err != nil {
						t.Errorf("Worker %d enqueue failed: %v", workerID, err)
					}
				}
			}(i)
		})
	}

	wg.Wait()

	// Verify all packets are in queue
	expectedSize := numWorkers * packetsPerWorker
	if size := queue.Size(); size != expectedSize {
		t.Errorf("Expected size %d, got %d", expectedSize, size)
	}
}

func TestOutputQueue_ConcurrentEnqueueDequeue(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("Skipping concurrent stress test in short mode")
	}
	// BLI-953: Skip with clear comment. This test had a deadlock: dequeuers don't always exit
	// (race between enqueuersDone, queue.Size(), and DequeueNonBlocking). Fix would require
	// a simpler shutdown (e.g. drain-with-deadline after enqueuers done) or queue API change.
	t.Skip("Skipping: concurrent enqueue/dequeue test has dequeuer exit race (BLI-953)")
	queue := NewOutputQueue(0)
	numEnqueuers := 50
	numDequeuers := 10
	packetsPerEnqueuer := 100

	var enqueuerWg sync.WaitGroup
	var dequeuerWg sync.WaitGroup
	enqueued := atomic.Int64{}
	dequeued := atomic.Int64{}
	expected := int64(numEnqueuers * packetsPerEnqueuer)
	enqueuersDone := make(chan struct{})

	// Start enqueuers
	enqueuerWg.Add(numEnqueuers)
	for i := 0; i < numEnqueuers; i++ {
		goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
			func(workerID int) {
				defer enqueuerWg.Done()
				for j := 0; j < packetsPerEnqueuer; j++ {
					packet := OutputPacket{
						Data:      fmt.Sprintf("worker-%d-packet-%d", workerID, j),
						ChannelID: "test",
					}
					if err := queue.Enqueue(packet); err != nil {
						t.Errorf("Enqueue failed: %v", err)
					}
					enqueued.Add(1)
				}
			}(i)
		})
	}
	goroutinelabels.

		// Signal when enqueuers are done (in background)
		NewGoroutine("refactor", "refactored").StartSimple(func() {

		func() {
			enqueuerWg.Wait()
			close(enqueuersDone)
		}()
	})

	// Start dequeuers
	dequeuerWg.Add(numDequeuers)
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 30*time.Second)
	defer cancel()

	for i := 0; i < numDequeuers; i++ {
		i := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_dequeuer_%d", i), fmt.Sprintf("dequeuing from output queue %d", i)).
			WithWaitGroup(&dequeuerWg).
			StartSimple(func() {
				consecutiveEmpty := 0
				maxConsecutiveEmpty := 50 // Exit after 50 consecutive empty dequeues
				for {
					// Check context first
					select {
					case <-ctx.Done():
						return
					default:
					}

					// Try to dequeue
					_, ok := queue.DequeueNonBlocking()
					if ok {
						consecutiveEmpty = 0
						dequeued.Add(1)
						// Check if we've dequeued everything
						if dequeued.Load() >= expected {
							return
						}
						continue
					}

					// Queue was empty
					consecutiveEmpty++

					// Check if enqueuers are done
					select {
					case <-ctx.Done():
						return
					case <-enqueuersDone:
						// Enqueuers finished - check if we're done
						if dequeued.Load() >= expected {
							return
						}
						// If queue is empty and we've processed what we can, exit
						if queue.Size() == 0 && consecutiveEmpty >= 5 {
							return
						}
						// Wait a bit and try again
						time.Sleep(10 * time.Millisecond)
					default:
						// Enqueuers still working
						if consecutiveEmpty >= maxConsecutiveEmpty {
							// Too many consecutive empty checks - something is wrong, exit
							return
						}
						time.Sleep(1 * time.Millisecond)
					}
				}
			})
	}

	// Wait for dequeuers to finish (with timeout)
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
		func() {
			dequeuerWg.Wait()
			close(done)
		}()
	})

	select {
	case <-done:
		// Dequeuers finished
	case <-time.After(30 * time.Second):
		t.Error("Dequeuers did not finish within timeout")
	}

	// Verify all packets were processed
	if dequeued.Load() < expected {
		t.Errorf("Expected at least %d dequeued, got %d (enqueued: %d)", expected, dequeued.Load(), enqueued.Load())
	}

	// Queue should be empty or nearly empty
	if size := queue.Size(); size > 10 {
		t.Errorf("Queue should be nearly empty, got size %d", size)
	}
}

type safeTestBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeTestBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeTestBuffer) WriteString(str string) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.WriteString(str)
}

func (s *safeTestBuffer) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Len()
}

func (s *safeTestBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestOutputWriter_Routing(t *testing.T) {
	t.Parallel()
	// Create mock handlers
	progressBuf := &safeTestBuffer{}
	metricsBuf := &safeTestBuffer{}
	stdoutBuf := &safeTestBuffer{}

	queue := NewOutputQueue(0)
	ctx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile("test")
	writer := NewOutputWriter(ctx, queue, logger)

	// Register handlers
	writer.RegisterHandler("progress", NewWriterOutputHandler(progressBuf, false))
	writer.RegisterHandler("metrics", NewWriterOutputHandler(metricsBuf, false))
	writer.RegisterHandler("stdout", NewWriterOutputHandler(stdoutBuf, false))

	// Start writer
	writer.Start()
	defer func() { _ = writer.Stop() }() //nolint:errcheck // Test cleanup - error handling not critical

	// Enqueue packets to different channels
	_ = queue.Enqueue(OutputPacket{Data: "progress-data", ChannelID: "progress"}) //nolint:errcheck // Test setup - error handling not critical
	_ = queue.Enqueue(OutputPacket{Data: "metrics-data", ChannelID: "metrics"})   //nolint:errcheck // Test setup - error handling not critical
	_ = queue.Enqueue(OutputPacket{Data: "stdout-data", ChannelID: "stdout"})     //nolint:errcheck // Test setup - error handling not critical

	// Poll until writer has routed all packets to buffers
	concurrency.PollTimeout(2*time.Second, 5*time.Millisecond, func() bool {
		return progressBuf.Len() > 0 && metricsBuf.Len() > 0 && stdoutBuf.Len() > 0
	})

	// Stop writer so no more writes; then we can safely read buffers without data race
	_ = writer.Stop()

	// Verify routing
	if progressBuf.String() != "progress-data" {
		t.Errorf("Progress handler got %q, expected %q", progressBuf.String(), "progress-data")
	}
	if metricsBuf.String() != "metrics-data" {
		t.Errorf("Metrics handler got %q, expected %q", metricsBuf.String(), "metrics-data")
	}
	if stdoutBuf.String() != "stdout-data" {
		t.Errorf("Stdout handler got %q, expected %q", stdoutBuf.String(), "stdout-data")
	}
}

func TestOutputWriter_Flush(t *testing.T) {
	t.Parallel()
	buf := &safeTestBuffer{}
	queue := NewOutputQueue(0)
	ctx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile("test")
	writer := NewOutputWriter(ctx, queue, logger)

	handler := NewWriterOutputHandler(buf, true) // Buffered
	writer.RegisterHandler("test", handler)
	writer.Start()
	defer func() { _ = writer.Stop() }() //nolint:errcheck // Test cleanup - error handling not critical

	// Enqueue with flush hint
	_ = queue.Enqueue(OutputPacket{Data: "test-data", ChannelID: "test", FlushHint: true}) //nolint:errcheck // Test setup - error handling not critical

	// Poll until flushed
	concurrency.PollTimeout(2*time.Second, 5*time.Millisecond, func() bool {
		return buf.Len() > 0
	})

	// Stop writer so no more writes; then we can safely read buffer without data race
	_ = writer.Stop()

	// Data should be flushed
	if buf.String() != "test-data" {
		t.Errorf("Expected flushed data, got %q", buf.String())
	}
}

func TestOutputWriter_Shutdown(t *testing.T) {
	t.Parallel()
	buf := &safeTestBuffer{}
	queue := NewOutputQueue(0)
	ctx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile("test")
	writer := NewOutputWriter(ctx, queue, logger)

	handler := NewWriterOutputHandler(buf, false)
	writer.RegisterHandler("test", handler)
	writer.Start()
	t.Cleanup(func() { _ = writer.Stop() }) //nolint:errcheck // Test cleanup - best effort

	// Enqueue some packets
	for i := 0; i < 10; i++ {
		if err := queue.Enqueue(OutputPacket{Data: fmt.Sprintf("data-%d", i), ChannelID: "test"}); err != nil {
			t.Fatalf("Failed to enqueue packet %d: %v", i, err)
		}
	}

	// Poll until writer processes packets
	concurrency.PollTimeout(2*time.Second, 5*time.Millisecond, func() bool {
		return buf.Len() > 0
	})

	// Stop writer so no more writes; then we can safely read buffer without data race
	_ = writer.Stop()

	// All packets should be processed
	if buf.Len() == 0 {
		t.Error("No data was written")
	}

	// Verify data was written
	expected := ""
	for i := 0; i < 10; i++ {
		expected += fmt.Sprintf("data-%d", i)
	}
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestOutputWriter_ConcurrentStress(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	buf := &safeTestBuffer{}
	queue := NewOutputQueue(100000) // Larger queue for stress test
	ctx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile("test")
	writer := NewOutputWriter(ctx, queue, logger)

	handler := NewWriterOutputHandler(buf, false)
	writer.RegisterHandler("test", handler)
	writer.Start()
	defer func() { _ = writer.Stop() }() //nolint:errcheck // Test cleanup - error handling not critical

	// Hammer it: 1000 workers, 100 packets each = 100k packets
	numWorkers := 1000
	packetsPerWorker := 100

	var wg sync.WaitGroup
	wg.Add(numWorkers)
	enqueued := atomic.Int64{}
	failed := atomic.Int64{}

	start := time.Now()
	for i := 0; i < numWorkers; i++ {
		goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
			func(workerID int) {
				defer wg.Done()
				for j := 0; j < packetsPerWorker; j++ {
					packet := OutputPacket{
						Data:      fmt.Sprintf("worker-%d-packet-%d\n", workerID, j),
						ChannelID: "test",
						FlushHint: j%10 == 0, // Flush every 10th packet
					}
					if err := queue.Enqueue(packet); err != nil {
						failed.Add(1)
						// Queue full - wait a bit and retry (backpressure handling)
						time.Sleep(1 * time.Millisecond)
						if retryErr := queue.Enqueue(packet); retryErr != nil {
							// Still full after retry - acceptable in stress test
							return
						}
					}
					enqueued.Add(1)
				}
			}(i)
		})
	}

	wg.Wait()
	enqueueTime := time.Since(start)

	// Wait for writer to process all packets
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatalf("Writer did not process all packets in time (queue size: %d)", queue.Size())
		case <-ticker.C:
			if queue.Size() == 0 {
				// All processed
				processTime := time.Since(start)
				t.Logf("Enqueued %d packets in %v (failed: %d)", enqueued.Load(), enqueueTime, failed.Load())
				t.Logf("Processed all packets in %v", processTime)
				t.Logf("Throughput: %.0f packets/sec", float64(enqueued.Load())/processTime.Seconds())
				return
			}
		}
	}
}

func TestOutputWriter_QueueFullBackpressure(t *testing.T) {
	t.Parallel()
	queue := NewOutputQueue(100) // Small queue
	ctx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile("test")
	writer := NewOutputWriter(ctx, queue, logger)

	buf := &safeTestBuffer{}
	// Use a slower writer to ensure queue fills up
	slowHandler := &slowWriterHandler{buf: buf, delay: 100 * time.Millisecond} // Very slow delay
	writer.RegisterHandler("slow", slowHandler)
	writer.Start()
	defer func() { _ = writer.Stop() }() //nolint:errcheck // Test cleanup - error handling not critical

	// Fill queue to capacity as fast as possible
	// Writer is very slow (100ms per packet), so queue should fill up quickly
	enqueued := 0
	for i := 0; i < 100; i++ {
		if err := queue.Enqueue(OutputPacket{Data: fmt.Sprintf("data-%d", i), ChannelID: "slow"}); err != nil {
			if err == ErrQueueFull {
				// Queue filled up before we could enqueue all - this is what we want to test
				break
			}
			t.Fatalf("Failed to enqueue packet %d: %v", i, err)
		}
		enqueued++
	}

	// Try to enqueue more - should fail if queue is full
	err := queue.Enqueue(OutputPacket{Data: "overflow", ChannelID: "slow"})
	queueSize := queue.Size()

	// Verify backpressure: either queue is full (ErrQueueFull) or we successfully enqueued
	// (which means writer drained faster than we filled - acceptable)
	if err == ErrQueueFull {
		// Good - backpressure working
		if queueSize < 100 {
			t.Logf("Queue reported full but size is %d (expected 100)", queueSize)
		}
	} else if err != nil {
		t.Errorf("Unexpected error: %v (queue size: %d)", err, queueSize)
	} else if queueSize >= 100 {
		// Enqueue succeeded - writer drained faster than we filled
		// This is acceptable, just verify queue is not full
		t.Errorf("Enqueue succeeded but queue is full (size: %d)", queueSize)
	}

	// Wait for writer to process some packets
	time.Sleep(500 * time.Millisecond)

	// Now enqueue should definitely succeed (queue has space)
	if err := queue.Enqueue(OutputPacket{Data: "after-process", ChannelID: "slow"}); err != nil {
		t.Errorf("Enqueue should succeed after processing: %v (queue size: %d)", err, queue.Size())
	}
}

// slowWriterHandler is a handler that adds delay to simulate slow I/O
type slowWriterHandler struct {
	buf   *safeTestBuffer
	delay time.Duration
}

func (h *slowWriterHandler) Write(data any) error {
	time.Sleep(h.delay)
	_, err := h.buf.WriteString(stringify(data))
	return err
}

func (h *slowWriterHandler) Flush() error {
	return nil
}

func (h *slowWriterHandler) Close() error {
	return nil
}

func TestWriterOutputHandler_Buffered(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	handler := NewWriterOutputHandler(buf, true) // Buffered

	// Write multiple times
	for i := 0; i < 10; i++ {
		if err := handler.Write(fmt.Sprintf("data-%d", i)); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	// Buffer should not be written yet
	if buf.Len() != 0 {
		t.Error("Buffer should not be written before flush")
	}

	// Flush should write all data
	if err := handler.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Verify all data is written
	expected := ""
	for i := 0; i < 10; i++ {
		expected += fmt.Sprintf("data-%d", i)
	}
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestWriterOutputHandler_Unbuffered(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	handler := NewWriterOutputHandler(buf, false) // Unbuffered

	// Write should immediately write to buffer
	if err := handler.Write("test-data"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if buf.String() != "test-data" {
		t.Errorf("Expected %q, got %q", "test-data", buf.String())
	}
}

func BenchmarkOutputQueue_Enqueue(b *testing.B) {
	queue := NewOutputQueue(0)
	packet := OutputPacket{Data: "test", ChannelID: "test"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = queue.Enqueue(packet) //nolint:errcheck // Benchmark - error handling not critical
	}
}

func BenchmarkOutputQueue_Dequeue(b *testing.B) {
	queue := NewOutputQueue(0)
	packet := OutputPacket{Data: "test", ChannelID: "test"}

	// Pre-fill queue
	for i := 0; i < b.N; i++ {
		_ = queue.Enqueue(packet) //nolint:errcheck // Benchmark - error handling not critical
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		queue.Dequeue()
	}
}

func BenchmarkOutputQueue_Concurrent(b *testing.B) {
	queue := NewOutputQueue(0)
	packet := OutputPacket{Data: "test", ChannelID: "test"}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = queue.Enqueue(packet) //nolint:errcheck // Benchmark - error handling not critical
		}
	})
}

func BenchmarkOutputWriter_Throughput(b *testing.B) {
	buf := &bytes.Buffer{}
	queue := NewOutputQueue(100000)
	ctx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile("test")
	writer := NewOutputWriter(ctx, queue, logger)

	handler := NewWriterOutputHandler(buf, false)
	writer.RegisterHandler("test", handler)
	writer.Start()
	defer func() { _ = writer.Stop() }() //nolint:errcheck // Test cleanup - error handling not critical

	packet := OutputPacket{Data: "test-data", ChannelID: "test"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = queue.Enqueue(packet) //nolint:errcheck // Benchmark - error handling not critical
	}

	// Wait for processing
	for queue.Size() > 0 {
		time.Sleep(1 * time.Millisecond)
	}
}

// Test helper: stringify for testing
func TestStringify(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    any
		expected string
	}{
		{"string", "string"},
		{[]byte("bytes"), "bytes"},
		{123, "123"},
	}

	for _, tt := range tests {
		result := stringify(tt.input)
		if result != tt.expected {
			t.Errorf("stringify(%v) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}
