package callback

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// CallbackEntry represents a single callback entry in the queue
type CallbackEntry struct {
	Payload   map[string]any
	Timestamp time.Time
	JobID     string
	Priority  int // Higher priority processed first
}

// Queue manages a queue of callback entries with sorting
type Queue struct {
	entries     []*CallbackEntry
	mu          sync.Mutex
	maxSize     int
	sorter      Sorter
	logger      logging.Logger
	processorCh chan *CallbackEntry
	stopCh      chan struct{}
	wg          sync.WaitGroup
	processing  bool
}

// Sorter defines the interface for sorting callback entries
type Sorter interface {
	Sort(entries []*CallbackEntry) []*CallbackEntry
}

// TimestampSorter sorts entries by timestamp (oldest first)
type TimestampSorter struct{}

func (s *TimestampSorter) Sort(entries []*CallbackEntry) []*CallbackEntry {
	sorted := make([]*CallbackEntry, len(entries))
	copy(sorted, entries)

	// Sort by timestamp (oldest first), then by priority (highest first), then by job ID
	sort.Slice(sorted, func(i, j int) bool {
		// Primary: timestamp (older first)
		if sorted[i].Timestamp.Before(sorted[j].Timestamp) {
			return true
		}
		if sorted[i].Timestamp.After(sorted[j].Timestamp) {
			return false
		}

		// Secondary: priority (higher first)
		if sorted[i].Priority > sorted[j].Priority {
			return true
		}
		if sorted[i].Priority < sorted[j].Priority {
			return false
		}

		// Tertiary: job ID (for consistency)
		return sorted[i].JobID < sorted[j].JobID
	})

	return sorted
}

// PrioritySorter sorts entries by priority first (highest first), then timestamp
type PrioritySorter struct{}

func (s *PrioritySorter) Sort(entries []*CallbackEntry) []*CallbackEntry {
	sorted := make([]*CallbackEntry, len(entries))
	copy(sorted, entries)

	// Sort by priority (highest first), then by timestamp (oldest first)
	sort.Slice(sorted, func(i, j int) bool {
		// Primary: priority (higher first)
		if sorted[i].Priority > sorted[j].Priority {
			return true
		}
		if sorted[i].Priority < sorted[j].Priority {
			return false
		}

		// Secondary: timestamp (older first)
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	return sorted
}

// NewQueue creates a new callback queue
func NewQueue(maxSize int, sorter Sorter, logger logging.Logger) *Queue {
	if sorter == nil {
		sorter = &TimestampSorter{} // Default to timestamp sorting
	}
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}

	return &Queue{
		entries:     make([]*CallbackEntry, 0),
		maxSize:     maxSize,
		sorter:      sorter,
		logger:      logger,
		processorCh: make(chan *CallbackEntry, maxSize),
		stopCh:      make(chan struct{}),
	}
}

// Enqueue adds a callback entry to the queue
func (q *Queue) Enqueue(entry *CallbackEntry) error {
	return concurrency.WithLockTimeout(
		&q.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(q.logger),
		LockNameCallbackQueueEnqueue,
		func() error {
			// Check if queue is full
			if q.maxSize > 0 && len(q.entries) >= q.maxSize {
				return ErrQueueFull
			}

			// Set timestamp if not set
			if entry.Timestamp.IsZero() {
				entry.Timestamp = time.Now().UTC()
			}

			// Extract job ID from payload if not set
			if entry.JobID == emptyValue {
				if jobID, ok := entry.Payload["job_id"].(string); ok {
					entry.JobID = jobID
				}
			}

			// Determine priority from payload if not set
			if entry.Priority == 0 {
				entry.Priority = q.determinePriority(entry.Payload)
			}

			q.entries = append(q.entries, entry)

			return nil
		},
	)
}

// determinePriority determines priority from payload
// Completion/error callbacks get higher priority than status callbacks
func (q *Queue) determinePriority(payload map[string]any) int {
	callbackType, _ := payload[objects.FieldKeyCallbackType].(string)
	switch callbackType {
	case callbackTypeCompletion, callbackTypeError:
		return 10 // Higher priority
	case callbackTypeStatus:
		return 5 // Lower priority
	default:
		return 5 // Default priority
	}
}

// DequeueSorted removes and returns entries in sorted order
func (q *Queue) DequeueSorted(count int) []*CallbackEntry {
	var result []*CallbackEntry
	_ = concurrency.WithLockTimeout(
		&q.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(q.logger),
		LockNameCallbackQueueDequeue,
		func() error {

			if len(q.entries) == 0 {
				result = nil
				return nil
			}

			// Sort entries
			sorted := q.sorter.Sort(q.entries)

			// Take up to count entries
			takeCount := count
			if takeCount <= 0 || takeCount > len(sorted) {
				takeCount = len(sorted)
			}

			result = sorted[:takeCount]

			// Remove dequeued entries from queue
			// Create a map of entries to remove for efficient lookup
			toRemove := make(map[*CallbackEntry]bool)
			for _, entry := range result {
				toRemove[entry] = true
			}

			// Rebuild entries list without removed items
			newEntries := make([]*CallbackEntry, 0, len(q.entries)-takeCount)
			for _, entry := range q.entries {
				if !toRemove[entry] {
					newEntries = append(newEntries, entry)
				}
			}

			q.entries = newEntries
			return nil
		},
	)
	return result
}

// Size returns the current queue size
func (q *Queue) Size() int {
	var size int
	_ = concurrency.WithLockTimeout(
		&q.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(q.logger),
		LockNameCallbackQueueSize,
		func() error {
			size = len(q.entries)
			return nil
		},
	)
	return size
}

// StartProcessing starts the background processor
func (q *Queue) StartProcessing(ctx context.Context, processor func(*CallbackEntry) error) {
	var shouldStart bool
	_ = concurrency.WithLockTimeout(
		&q.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(q.logger),
		LockNameCallbackQueueStartProcessing,
		func() error {
			if q.processing {
				shouldStart = false
				return nil
			}
			q.processing = true
			shouldStart = true
			return nil
		},
	)

	if !shouldStart {
		return
	}

	goroutinelabels.NewGoroutine("callback_queue_processor", "processing callback queue entries").
		WithWaitGroup(&q.wg).
		StartSimple(func() {
			q.processLoop(ctx, processor)
		})
}

// StopProcessing stops the background processor
func (q *Queue) StopProcessing() {
	var shouldStop bool
	_ = concurrency.WithLockTimeout(
		&q.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(q.logger),
		LockNameCallbackQueueStopProcessing,
		func() error {
			if !q.processing {
				shouldStop = false
				return nil
			}
			q.processing = false
			shouldStop = true
			return nil
		},
	)

	if !shouldStop {
		return
	}

	// Close stop channel to signal processor to stop
	select {
	case <-q.stopCh:
		// Already closed
	default:
		close(q.stopCh)
	}
	q.wg.Wait()

	// Recreate stopCh for potential restart
	_ = concurrency.WithLockTimeout(
		&q.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(q.logger),
		LockNameCallbackQueueRecreateStopch,
		func() error {
			q.stopCh = make(chan struct{})
			return nil
		},
	)
}

// processLoop processes entries from the queue.
// The goroutine that runs processLoop is already tracked by WithWaitGroup(&q.wg) in StartProcessing,
// so we must not call q.wg.Done() here (would double-count and cause WaitGroup reuse panic).
func (q *Queue) processLoop(ctx context.Context, processor func(*CallbackEntry) error) {
	ticker := time.NewTicker(100 * time.Millisecond) // Process every 100ms
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-q.stopCh:
			return
		case <-ticker.C:
			// Process up to 10 entries at a time
			entries := q.DequeueSorted(10)
			for _, entry := range entries {
				if err := processor(entry); err != nil {
					logging.Fluent(q.logger).Warn("Failed to process callback entry").
						JobID(entry.JobID).
						WithError(err).
						Log()
					// Re-enqueue on error? Or skip? For now, skip.
				}
			}
		}
	}
}

// ErrQueueFull is returned when the queue is full
var ErrQueueFull = &QueueError{Message: "queue is full"}

// QueueError represents a queue-related error
type QueueError struct {
	Message string
}

func (e *QueueError) Error() string {
	return e.Message
}
