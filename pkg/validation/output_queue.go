package validation

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

// OutputPacket represents a single output operation
type OutputPacket struct {
	Data      any       // The data to write
	ChannelID string    // Which channel/stream to write to ("progress", "metrics", "stdout", "stderr")
	FlushHint bool      // Should flush after this write?
	Timestamp time.Time // When was this enqueued?
}

// OutputQueue manages the FIFO queue of output packets
// Uses channel-based pattern (consistent with io_queue.go and message_queue.go)
// for native timeout support via select statements
type OutputQueue struct {
	mu      sync.Mutex
	queue   []OutputPacket
	ready   chan struct{} // Channel for signaling when queue has data (replaces sync.Cond)
	maxSize int           // Maximum queue size (0 = unbounded)
}

// NewOutputQueue creates a new output queue
func NewOutputQueue(maxSize int) *OutputQueue {
	return &OutputQueue{
		queue:   make([]OutputPacket, 0),
		ready:   make(chan struct{}, 1), // Buffered to avoid blocking on signal
		maxSize: maxSize,
	}
}

// Enqueue adds a packet to the queue (thread-safe, fast)
// Returns error if queue is full (when maxSize > 0)
func (oq *OutputQueue) Enqueue(packet OutputPacket) error {
	return concurrency.RunInLockWithLogger(
		&oq.mu,
		LockNameOutputQueueEnqueue,
		lockLoggerSystem(),
		func() error {
			// Check queue size limit
			if oq.maxSize > 0 && len(oq.queue) >= oq.maxSize {
				return ErrQueueFull
			}

			// Set timestamp if not set
			if packet.Timestamp.IsZero() {
				packet.Timestamp = time.Now()
			}

			oq.queue = append(oq.queue, packet)
			// Signal waiting dequeuers (non-blocking due to buffered channel)
			select {
			case oq.ready <- struct{}{}:
			default:
				// Channel already has signal, no need to send again
			}
			return nil
		},
	)
}

// Dequeue removes and returns the head packet (blocks if empty)
// Uses channel-based pattern with select for native timeout support
// Follows established pattern from io_queue.go and message_queue.go
func (oq *OutputQueue) Dequeue() OutputPacket {
	waitTimeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), waitTimeout)
	defer cancel()

	logger := lockLoggerSystem()

	for {
		// Check queue with lock (fast path)
		var packet OutputPacket
		var found bool
		if err := concurrency.RunInLockWithLogger(
			&oq.mu,
			LockNameOutputQueueDequeue,
			logger,
			func() error {
				if len(oq.queue) > 0 {
					packet = oq.queue[0]
					oq.queue = oq.queue[1:]
					found = true
					return nil
				}
				return nil
			},
		); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("failed to dequeue from output queue: %v\n", err).Log()
		}

		if found {
			return packet
		}

		// Queue is empty - wait for data with timeout
		// Use channel-based pattern (consistent with other queues)
		// This provides native timeout support via select
		select {
		case <-oq.ready:
			// Signal received - queue may have data, loop to check again
			// Note: We consume the signal here, which is correct
			// If multiple signals were sent, we'll process them one by one
			continue
		case <-ctx.Done():
			// Timeout occurred
			if logger != nil {
				logger.Warn("Dequeue timeout - queue empty for extended period", concurrency.LockField{Key: "operation", Value: "output_queue_dequeue"},
					concurrency.LockField{Key: "timeout", Value: waitTimeout.String()})
			}
			return OutputPacket{} // Return empty to indicate timeout
		}
	}
}

// DequeueNonBlocking attempts to dequeue without blocking
// Returns (packet, true) if data available, (empty, false) if queue empty
func (oq *OutputQueue) DequeueNonBlocking() (OutputPacket, bool) {
	var packet OutputPacket
	var found bool
	if err := concurrency.RunInLockWithLogger(
		&oq.mu,
		LockNameOutputQueueDequeueNonBlocking,
		lockLoggerSystem(),
		func() error {
			if len(oq.queue) == 0 {
				found = false
				return nil
			}

			packet = oq.queue[0]
			oq.queue = oq.queue[1:]
			found = true
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("failed to dequeue non-blocking from output queue: %v\n", err).Log()
	}
	return packet, found
}

// DequeueBatch dequeues up to maxItems from the queue in a single lock acquisition
// Returns the batch and true if any items were dequeued, (nil, false) if queue empty
// FIXED: Outer lock acquired once, batch dequeued, lock released - inner loop processes without locks
func (oq *OutputQueue) DequeueBatch(maxItems int) ([]OutputPacket, bool) {
	var batch []OutputPacket
	var found bool
	if err := concurrency.RunInLockWithLogger(
		&oq.mu,
		LockNameOutputQueueDequeueBatch,
		lockLoggerSystem(),
		func() error {
			if len(oq.queue) == 0 {
				found = false
				return nil
			}

			// Dequeue up to maxItems (or all available if fewer)
			batchSize := maxItems
			if len(oq.queue) < batchSize {
				batchSize = len(oq.queue)
			}

			batch = make([]OutputPacket, batchSize)
			copy(batch, oq.queue[:batchSize])
			oq.queue = oq.queue[batchSize:]
			found = true
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("failed to dequeue batch from output queue: %v\n", err).Log()
	}
	return batch, found
}

// Size returns the current queue size
func (oq *OutputQueue) Size() int {
	var size int
	if err := concurrency.RunInLockWithLogger(
		&oq.mu,
		LockNameOutputQueueSize,
		lockLoggerSystem(),
		func() error {
			size = len(oq.queue)
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// OutputHandler processes data for a specific channel
			Error("failed to get output queue size: %v\n", err).Log()
	}
	return size
}

type OutputHandler interface {
	Write(data any) error
	Flush() error
	Close() error
}

// WriterOutputHandler writes to an io.Writer
type WriterOutputHandler struct {
	writer io.Writer
	buf    *syncBuffer // Optional buffering
}

// NewWriterOutputHandler creates a handler that writes to an io.Writer
func NewWriterOutputHandler(writer io.Writer, buffered bool) *WriterOutputHandler {
	h := &WriterOutputHandler{writer: writer}
	if buffered {
		// Use buffered writer for better performance
		if w, ok := writer.(*syncBuffer); ok {
			h.buf = w
		} else {
			h.buf = newSyncBuffer(writer)
		}
	}
	return h
}

// Write writes data to the writer
func (h *WriterOutputHandler) Write(data any) error {
	var err error
	switch v := data.(type) {
	case []byte:
		if h.buf != nil {
			_, err = h.buf.Write(v)
		} else {
			_, err = h.writer.Write(v)
		}
	case string:
		if h.buf != nil {
			_, err = h.buf.WriteString(v)
		} else {
			_, err = io.WriteString(h.writer, v)
		}
	default:
		// For other types, convert to string
		if h.buf != nil {
			_, err = h.buf.WriteString(stringify(v))
		} else {
			_, err = io.WriteString(h.writer, stringify(v))
		}
	}
	return err
}

// Flush flushes any buffered data
func (h *WriterOutputHandler) Flush() error {
	if h.buf != nil {
		return h.buf.Flush()
	}
	if flusher, ok := h.writer.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	return nil
}

// Close closes the writer
func (h *WriterOutputHandler) Close() error {
	if h.buf != nil {
		_ = h.buf.Flush() // Flush before closing
	}
	// CRITICAL: Do NOT close standard streams owned by the process
	if h.writer == os.Stdout || h.writer == os.Stderr || h.writer == os.Stdin {
		return nil
	}
	if h.buf != nil && (h.buf.writer == os.Stdout || h.buf.writer == os.Stderr || h.buf.writer == os.Stdin) {
		return nil
	}
	if closer, ok := h.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// syncBuffer is a thread-safe buffer wrapper
type syncBuffer struct {
	mu     sync.Mutex
	buffer []byte
	writer io.Writer
}

func newSyncBuffer(writer io.Writer) *syncBuffer {
	return &syncBuffer{
		buffer: make([]byte, 0, 4096), // 4KB initial capacity
		writer: writer,
	}
}

func (sb *syncBuffer) Write(p []byte) (int, error) {
	if err := concurrency.RunInLockWithLogger(
		&sb.mu,
		LockNameSyncBufferWrite,
		lockLoggerSystem(),
		func() error {
			sb.buffer = append(sb.buffer, p...)
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("failed to write to sync buffer: %v\n", err).Log()
		return 0, err
	}
	return len(p), nil
}

func (sb *syncBuffer) WriteString(s string) (int, error) {
	//nolint:gocritic // preferStringWriter: This IS the WriteString method implementation, so we call Write
	return sb.Write([]byte(s))
}

func (sb *syncBuffer) Flush() error {
	var bufferCopy []byte
	var writer io.Writer
	if err := concurrency.RunInLockWithLogger(
		&sb.mu,
		LockNameSyncBufferFlushCopy,
		lockLoggerSystem(),
		func() error {
			if len(sb.buffer) == 0 {
				return nil
			}
			bufferCopy = make([]byte, len(sb.buffer))
			copy(bufferCopy, sb.buffer)
			sb.buffer = sb.buffer[:0] // Clear buffer
			writer = sb.writer
			return nil
		},
	); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Perform I/O outside lock
			Error("failed to flush sync buffer: %v\n", err).Log()
		return err
	}

	if len(bufferCopy) == 0 {
		return nil
	}
	_, err := writer.Write(bufferCopy)
	return err
}

// OutputWriter handles all I/O operations
type OutputWriter struct {
	queue    *OutputQueue
	handlers map[string]OutputHandler // channel_id -> handler
	ctx      context.Context
	cancel   context.CancelFunc
	logger   logging.Logger
	wg       sync.WaitGroup
}

// NewOutputWriter creates a new output writer
func NewOutputWriter(ctx context.Context, queue *OutputQueue, logger logging.Logger) *OutputWriter {
	ctx, cancel := context.WithCancel(ctx)
	return &OutputWriter{
		queue:    queue,
		handlers: make(map[string]OutputHandler),
		ctx:      ctx,
		cancel:   cancel,
		logger:   logger,
	}
}

// RegisterHandler registers a handler for a channel
func (ow *OutputWriter) RegisterHandler(channelID string, handler OutputHandler) {
	ow.handlers[channelID] = handler
}

// Start begins the writer goroutine
func (ow *OutputWriter) Start() {
	bud := goroutinelabels.DefaultBudget()
	writerBuilder := goroutinelabels.NewGoroutine("output_writer", "writing validation output to queue").
		WithWaitGroup(&ow.wg)
	if bud != nil {
		writerBuilder = writerBuilder.WithBudget(bud)
	}
	writerBuilder.StartSimple(func() {
		ow.run()
	})
}

// run is the main writer loop
// FIXED: Uses batch dequeue - outer lock acquired once, batch processed without locks
func (ow *OutputWriter) run() {
	batchSize := 100 // Process up to 100 items per batch

	for {
		select {
		case <-ow.ctx.Done():
			ow.drainQueue(batchSize)
			ow.flushAll()
			return
		default:
			// Acquire outer lock, dequeue batch, release lock
			batch, ok := ow.queue.DequeueBatch(batchSize)
			if !ok {
				// Queue empty - check if we should exit
				select {
				case <-ow.ctx.Done():
					ow.drainQueue(batchSize)
					ow.flushAll()
					return
				default:
					// Wait a bit before checking again (avoid busy loop)
					time.Sleep(10 * time.Millisecond)
					continue
				}
			}

			ow.processBatch(batch)
		}
	}
}

func (ow *OutputWriter) drainQueue(batchSize int) {
	for {
		batch, ok := ow.queue.DequeueBatch(batchSize)
		if !ok || len(batch) == 0 {
			break
		}
		ow.processBatch(batch)
	}
}

func (ow *OutputWriter) processBatch(batch []OutputPacket) {
	for _, packet := range batch {
		// Route to appropriate handler (no lock held)
		handler := ow.handlers[packet.ChannelID]
		if handler != nil {
			if err := handler.Write(packet.Data); err != nil {
				if ow.logger != nil {
					logging.Fluent(ow.logger).Warn("Failed to write output").
						String("channel", packet.ChannelID).
						WithError(err).
						Log()
				}
			}

			if packet.FlushHint {
				if err := handler.Flush(); err != nil {
					if ow.logger != nil {
						logging.Fluent(ow.logger).Warn("Failed to flush output").
							String("channel", packet.ChannelID).
							WithError(err).
							Log()
					}
				}
			}
		} else if ow.logger != nil {
			logging.Fluent(ow.logger).Debug("No handler registered for channel").
				String("channel", packet.ChannelID).
				Log()
		}
	}
}

// flushAll flushes all registered handlers
func (ow *OutputWriter) flushAll() {
	for channelID, handler := range ow.handlers {
		if err := handler.Flush(); err != nil {
			if ow.logger != nil {
				logging.Fluent(ow.logger).Warn("Failed to flush handler on shutdown").
					String("channel", channelID).
					WithError(err).
					Log()
			}
		}
	}
}

func (ow *OutputWriter) Stop() error {
	ow.cancel()
	ow.wg.Wait()

	// Close all handlers
	for channelID, handler := range ow.handlers {
		if err := handler.Close(); err != nil {
			if ow.logger != nil {
				logging.Fluent(ow.logger).Warn("Failed to close handler").
					String("channel", channelID).
					WithError(err).
					Log()
			}
		}
	}
	return nil
}

// stringify converts any value to string
func stringify(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	}

	// For other types, use fmt.Sprint
	return fmt.Sprint(v)
}

// EnqueueProgress is a helper to enqueue progress updates
//
//nolint:gocritic // Progress passed by value for immutability
func (oq *OutputQueue) EnqueueProgress(progress ValidationProgress, flush bool) error {
	return oq.Enqueue(OutputPacket{
		Data:      progress,
		ChannelID: "progress",
		FlushHint: flush,
	})
}

// EnqueueMetrics is a helper to enqueue metrics updates
func (oq *OutputQueue) EnqueueMetrics(metrics any, flush bool) error {
	return oq.Enqueue(OutputPacket{
		Data:      metrics,
		ChannelID: "metrics",
		FlushHint: flush,
	})
}

// EnqueueStdout is a helper to enqueue stdout output
func (oq *OutputQueue) EnqueueStdout(data any, flush bool) error {
	return oq.Enqueue(OutputPacket{
		Data:      data,
		ChannelID: "stdout",
		FlushHint: flush,
	})
}

// EnqueueStderr is a helper to enqueue stderr output
func (oq *OutputQueue) EnqueueStderr(data any, flush bool) error {
	return oq.Enqueue(OutputPacket{
		Data:      data,
		ChannelID: "stderr",
		FlushHint: flush,
	})
}

// ErrQueueFull is returned when queue is full
var ErrQueueFull = &QueueFullError{}

// QueueFullError represents a queue full error
type QueueFullError struct{}

func (e *QueueFullError) Error() string {
	return "output queue is full"
}
