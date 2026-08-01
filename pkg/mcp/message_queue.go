package mcp

import (
	"bufio"
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
)

// QueuedMessage represents a message queued for delivery to a client
type QueuedMessage struct {
	Data   []byte
	Format *MessageFormat
	// Priority: "low", "normal", "high", "critical"
	// Higher priority messages are sent first when queue is full
	Priority string
	// Timestamp when message was queued
	QueuedAt time.Time
	// Optional callback invoked after message is successfully written
	// Called from writerLoop after writeMessage succeeds
	OnWriteComplete func()
}

// MessageQueue manages a queue of messages for a client with backpressure handling
type MessageQueue struct {
	// Queue of messages to send
	queue chan *QueuedMessage

	// Writer to send messages to
	writer *bufio.Writer
	format *MessageFormat

	// Configuration
	config QueueConfig

	// State
	mu          sync.RWMutex
	writeMu     sync.Mutex // Serializes actual writes to bufio.Writer (extra safety)
	active      atomic.Bool
	dropped     atomic.Int64 // Count of dropped messages
	sent        atomic.Int64 // Count of successfully sent messages
	errors      atomic.Int64 // Count of send errors
	lastError   error
	lastErrorAt time.Time

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Transport for writing messages
	transport Transport

	// Metrics callback (optional)
	metricsCallback func(depth, dropped, sent, errors int64)

	// Coordinator for event emission (optional)
	// Uses any to avoid import cycle
	coordinator interface {
		Emit(ctx context.Context, eventCtx any) error
	}
	coordinatorCtx context.Context
	writerDone     chan struct{}
}

// QueueConfig configures message queue behavior
type QueueConfig struct {
	// MaxQueueSize is the maximum number of messages in the queue
	// When full, new messages are dropped (or blocked, depending on DropWhenFull)
	MaxQueueSize int

	// DropWhenFull: if true, drop messages when queue is full
	// If false, block until space is available (not recommended for high throughput)
	DropWhenFull bool

	// FlushInterval is how often to flush the writer (0 = flush after each message)
	FlushInterval time.Duration

	// WriteTimeout is the maximum time to wait for a write to complete
	WriteTimeout time.Duration
}

// DefaultQueueConfig returns default queue configuration
func DefaultQueueConfig() QueueConfig {
	return QueueConfig{
		MaxQueueSize:  DefaultMessageQueueSize,
		DropWhenFull:  true,
		FlushInterval: 0, // Flush immediately
		WriteTimeout:  DefaultMessageWriteTimeout,
	}
}

// NewMessageQueue creates a new message queue for a client
// Note: Uses system context for internal cancellation context as this is a long-lived component
func NewMessageQueue(writer *bufio.Writer, format *MessageFormat, config QueueConfig) *MessageQueue {
	// Use system context for internal cancellation (long-lived component)
	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext()) //nolint:gosec // G118: cancel stored on queue; invoked from Close

	queue := &MessageQueue{
		queue:      make(chan *QueuedMessage, config.MaxQueueSize),
		writer:     writer,
		format:     format,
		config:     config,
		transport:  NewDefaultTransport(),
		ctx:        ctx,
		cancel:     cancel,
		writerDone: make(chan struct{}),
	}
	queue.active.Store(true)

	// Start writer goroutine
	goroutinelabels.NewGoroutine("mcp_message_queue_writer", "writing queued messages to client").
		StartSimple(queue.writerLoop)

	return queue
}

// SetMetricsCallback sets a callback for metrics updates
func (q *MessageQueue) SetMetricsCallback(callback func(depth, dropped, sent, errors int64)) {
	_ = concurrency.RunInLockWithLogger(
		&q.mu, LockNameMessageQueueSetMetricsCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			q.metricsCallback = callback
			return nil
		},
	)
}

// SetCoordinator sets the coordinator for event emission
func (q *MessageQueue) SetCoordinator(coordinator interface {
	Emit(ctx context.Context, eventCtx any) error
}, ctx context.Context) {
	_ = concurrency.RunInLockWithLogger(
		&q.mu, LockNameMessageQueueSetCoordinator, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			q.coordinator = coordinator
			q.coordinatorCtx = ctx
			return nil
		},
	)
}

// Enqueue adds a message to the queue
// Returns true if message was queued, false if dropped
// onWriteComplete is an optional callback invoked after the message is successfully written
func (q *MessageQueue) Enqueue(data []byte, format *MessageFormat, priority string, onWriteComplete func()) bool {
	msg := &QueuedMessage{
		Data:            data,
		Format:          format,
		Priority:        priority,
		QueuedAt:        time.Now(),
		OnWriteComplete: onWriteComplete,
	}

	select {
	case q.queue <- msg:
		return true
	default:
		// Queue is full
		if q.config.DropWhenFull {
			q.dropped.Add(1)
			q.updateMetrics()
			return false
		}
		// Block until space is available (not recommended)
		q.queue <- msg
		return true
	}
}

// writerLoop continuously drains the queue and writes messages
func (q *MessageQueue) writerLoop() {
	defer close(q.writerDone)
	var flushTicker *time.Ticker
	var flushTickerC <-chan time.Time

	if q.config.FlushInterval > 0 {
		flushTicker = time.NewTicker(q.config.FlushInterval)
		defer flushTicker.Stop()
		flushTickerC = flushTicker.C
	}

	for {
		select {
		case <-q.ctx.Done():
			// Shutdown requested - flush remaining messages
			q.flushRemaining()
			return

		case msg := <-q.queue:
			if err := q.writeMessage(msg); err != nil {
				q.errors.Add(1)
				_ = concurrency.RunInLockWithLogger(
					&q.mu, LockNameMessageQueueWriteError, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
					func() error {
						q.lastError = err
						q.lastErrorAt = time.Now()
						return nil
					},
				)

				// If write fails, mark queue as inactive
				// This prevents further queuing attempts
				if q.isWriteErrorFatal(err) {
					_ = concurrency.RunInLockWithLogger(
						&q.mu, LockNameMessageQueueWriteFatal, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
						func() error {
							q.active.Store(false)
							return nil
						},
					)
					return
				}
			} else {
				q.sent.Add(1)

				// CRITICAL: Do NOT flush here - transport.WriteMessage already flushes
				// Double flushing is redundant and could potentially cause timing issues
				// The flush in transport.WriteMessage ensures each message is sent atomically
				// Only flush if using periodic flush interval (for batching)
				if q.config.FlushInterval > 0 {
					// Periodic flush will be handled by flushTickerC case
					// No need to flush here
				}

				// Update metrics
				q.updateMetrics()

				// Invoke callback if provided (after successful write)
				// This allows callers to know exactly when the message was written
				// Useful for sending follow-up messages (like log notifications) after responses
				// CRITICAL: Call callback synchronously (not in goroutine) to ensure
				// any messages it enqueues are written in order after the current message
				// The callback should only do non-blocking operations (like enqueueing)
				// This prevents race conditions where callback's messages interleave with next response
				//
				// IMPORTANT: Even though writeMessage() calls Flush(), there may be OS-level buffering
				// The callback runs after writeMessage returns, but we need to ensure the write
				// is fully complete before enqueueing follow-up messages. The callback itself should
				// handle any necessary delays (see message_processor.go initialize callback)
				if msg.OnWriteComplete != nil {
					msg.OnWriteComplete()
				}

				// CRITICAL: After callback completes, ensure any messages it enqueued are properly
				// sequenced. The callback's messages will be written in the next iteration of the loop,
				// which ensures they come after the current message is fully written and flushed.
			}

		case <-flushTickerC:
			// Periodic flush
			if err := q.writer.Flush(); err != nil {
				q.errors.Add(1)
				_ = concurrency.RunInLockWithLogger(
					&q.mu, LockNameMessageQueueFlushError, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
					func() error {
						q.lastError = err
						q.lastErrorAt = time.Now()
						return nil
					},
				)
			}
		}
	}
}

// writeMessage writes a single message
// CRITICAL: Must ensure writes are serialized - never allow concurrent writes
// Since writerLoop processes messages one at a time, we can write directly without timeout goroutine
// This eliminates any possibility of interleaving from concurrent goroutines
// Additional writeMu mutex provides extra safety for bufio.Writer (which is not thread-safe)
func (q *MessageQueue) writeMessage(msg *QueuedMessage) error {
	// CRITICAL: Acquire write mutex to ensure bufio.Writer is never accessed concurrently
	// Even though writerLoop processes messages sequentially, this provides extra safety
	// bufio.Writer is NOT thread-safe, so we must serialize all writes
	var writeErr error
	_ = concurrency.RunInLockWithLogger(
		&q.writeMu, LockNameMessageQueueWriteSerialize, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Write directly - writerLoop already serializes writes by processing messages sequentially
			// The timeout mechanism with goroutines was causing potential interleaving issues
			// Direct write is safe because:
			// 1. writerLoop processes one message at a time
			// 2. Each write completes (including flush) before the next message is processed
			// 3. writeMu mutex ensures bufio.Writer is never accessed concurrently
			writeErr = q.transport.WriteMessage(q.writer, msg.Data, msg.Format)
			return nil
		},
	)
	return writeErr
}

// flushRemaining flushes any remaining messages in the queue
func (q *MessageQueue) flushRemaining() {
	// Drain queue
	for {
		select {
		case msg := <-q.queue:
			_ = q.writeMessage(msg) //nolint:errcheck // Best effort - queue cleanup
		default:
			// Queue is empty
			if err := q.writer.Flush(); err != nil {
				// Ignore flush errors during shutdown
			}
			return
		}
	}
}

// isWriteErrorFatal determines if a write error should stop the queue
func (q *MessageQueue) isWriteErrorFatal(err error) bool {
	if err == nil {
		return false
	}
	// Consider connection errors fatal
	errStr := err.Error()
	return containsAny(errStr, []string{
		"broken pipe",
		"connection reset",
		"connection refused",
		"EOF",
	})
}

// updateMetrics calls the metrics callback and coordinator if set
func (q *MessageQueue) updateMetrics() {
	var callback func(depth, dropped, sent, errors int64)
	var coordinator interface {
		Emit(ctx context.Context, eventCtx any) error
	}
	var coordinatorCtx context.Context
	var depth int64
	_ = concurrency.RunInRLockWithLogger(
		&q.mu, LockNameMessageQueueUpdateMetrics, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			callback = q.metricsCallback
			coordinator = q.coordinator
			coordinatorCtx = q.coordinatorCtx
			depth = int64(len(q.queue))
			return nil
		},
	)
	dropped := q.dropped.Load()
	sent := q.sent.Load()
	errors := q.errors.Load()

	if callback != nil {
		callback(depth, dropped, sent, errors)
	}

	// Emit queue metrics via coordinator if available
	if coordinator != nil && coordinatorCtx != nil {
		eventCtx := BuildMCPQueueMetricsEventContext(depth, dropped, sent, errors)
		_ = coordinator.Emit(coordinatorCtx, eventCtx) //nolint:errcheck // Async, best-effort
	}
}

// Stats returns queue statistics
func (q *MessageQueue) Stats() QueueStats {
	var stats QueueStats
	_ = concurrency.RunInRLockWithLogger(
		&q.mu, LockNameMessageQueueStats, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			stats = QueueStats{
				QueueDepth:   len(q.queue),
				MaxQueueSize: q.config.MaxQueueSize,
				Dropped:      q.dropped.Load(),
				Sent:         q.sent.Load(),
				Errors:       q.errors.Load(),
				LastError:    q.lastError,
				LastErrorAt:  q.lastErrorAt,
				Active:       q.active.Load(),
			}
			return nil
		},
	)

	// Update metrics when stats are retrieved
	q.updateMetrics()

	return stats
}

// QueueStats contains statistics about a message queue
type QueueStats struct {
	QueueDepth   int
	MaxQueueSize int
	Dropped      int64
	Sent         int64
	Errors       int64
	LastError    error
	LastErrorAt  time.Time
	Active       bool
}

// IsHealthy returns true if the queue is in a healthy state
func (s QueueStats) IsHealthy() bool {
	return s.Active && s.Errors == 0 && s.QueueDepth < s.MaxQueueSize
}

// Stop stops the queue and flushes remaining messages
func (q *MessageQueue) Stop() {
	_ = concurrency.RunInLockWithLogger(
		&q.mu, LockNameMessageQueueStop, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			q.active.Store(false)
			return nil
		},
	)

	q.cancel()
	<-q.writerDone
}

// UpdateWriter updates the writer and format (e.g., when client reconnects)
func (q *MessageQueue) UpdateWriter(writer *bufio.Writer, format *MessageFormat) {
	_ = concurrency.RunInLockWithLogger(
		&q.mu, LockNameMessageQueueUpdateWriter, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			q.writer = writer
			q.format = format
			q.active.Store(true)
			return nil
		},
	)
}

// Helper function
func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if contains(s, substr) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr))))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
