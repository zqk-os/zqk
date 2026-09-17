package mcp

import (
	"bufio"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
)

// withClientsReadLock safely acquires clientsMu.RLock() with shutdown checks using TryRLock
// This prevents deadlocks by ensuring we never block on clientsMu during shutdown
// Uses TryRLock() (Go 1.18+) for non-blocking lock acquisition
// Uses shutdown context to detect shutdown without needing mutex checks
// Returns true if the lock was acquired and shutdown is not in progress, false otherwise
// The callback is only called if the lock was successfully acquired and shutdown hasn't started
// CRITICAL: The callback should check shutdown context and return early if shutdown starts during execution
func (s *Server) withClientsReadLock(callback func() bool) bool {
	// GUARD 1: Check shutdown context first (fast path, no lock needed)
	// This is the primary guard - if shutdown is ordered, we never try to acquire the lock
	select {
	case <-s.shutdownCtx.Done():
		return false // Shutdown ordered, don't acquire lock
	default:
	}

	// GUARD 2: Double-check with atomic flag (lock-free, thread-safe)
	// Sometimes shutdown context might not be cancelled yet, but shutdownFlag is set
	if s.shutdownFlag.Load() == 1 {
		return false // Shutdown in progress, don't acquire lock
	}

	// GUARD 3: Use TryRLock() for non-blocking lock acquisition
	// If shutdownSequence is holding the write lock, this returns false immediately
	// CRITICAL: TryRLock() should NEVER block - it returns false if lock is unavailable
	// If it blocks, that's a bug in Go's implementation
	if !s.clientsMu.TryRLock() {
		// Lock not available (write lock held or pending) - skip to avoid deadlock
		// This is expected behavior - we return false and the caller handles it gracefully
		return false
	}
	defer s.clientsMu.RUnlock()

	// GUARD 4: Check shutdown context after acquiring lock (defensive)
	// Shutdown may have been ordered while we were trying to acquire the lock
	// This is a final safety check before executing the callback
	select {
	case <-s.shutdownCtx.Done():
		return false // Shutdown ordered, release lock and return
	default:
	}

	// GUARD 5: Final mutex check after acquiring lock
	// Double-check that shutdown hasn't started while we were acquiring the lock
	// Use atomic load for lock-free check
	if s.shutdownFlag.Load() == 1 {
		return false // Shutdown started, release lock and return
	}

	// All guards passed - lock acquired and shutdown not in progress
	// Callback returns true if it completed successfully, false if shutdown was detected
	return callback()
}

// queueForWriter returns a MessageQueue dedicated to this TCP/stdio writer.
// Multi-client must not share clientID-scoped queues across reconnects.
func (s *Server) queueForWriter(writer *bufio.Writer, format *MessageFormat) *MessageQueue {
	if s == nil || writer == nil {
		return nil
	}
	s.writerQueuesMu.Lock()
	defer s.writerQueuesMu.Unlock()
	if s.writerQueues == nil {
		s.writerQueues = make(map[*bufio.Writer]*MessageQueue)
	}
	if q, ok := s.writerQueues[writer]; ok && q != nil && q.IsActive() {
		return q
	}
	if q, ok := s.writerQueues[writer]; ok && q != nil {
		q.Abandon()
	}
	q := NewMessageQueue(writer, format, s.getQueueConfig())
	q.SetMetricsCallback(func(depth, dropped, sent, errors int64) {
		if s.mcpMetrics != nil {
			s.mcpMetrics.RecordQueueMetrics(depth, dropped, sent, errors)
		}
	})
	s.writerQueues[writer] = q
	return q
}

// forgetWriterSubscription drops a subscription ID from writer tracking after
// an explicit events/unsubscribe (disconnect path deletes the whole writer key).
func (s *Server) forgetWriterSubscription(subscriptionID string) {
	if s == nil || subscriptionID == "" {
		return
	}
	_ = concurrency.RunInLockWithLogger(
		&s.writerSubscriptionsMu, LockNameMcpServerTrackWriterSubscription, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for w, ids := range s.writerSubscriptions {
				out := ids[:0]
				for _, id := range ids {
					if id != subscriptionID {
						out = append(out, id)
					}
				}
				if len(out) == 0 {
					delete(s.writerSubscriptions, w)
				} else {
					s.writerSubscriptions[w] = out
				}
			}
			return nil
		},
	)
}

// trackWriterSubscription records an events/subscribe ID against the connection Writer
// so disconnect can Unsubscribe and keep GetSubscriberCount honest.
func (s *Server) trackWriterSubscription(writer *bufio.Writer, subscriptionID string) {
	if s == nil || writer == nil || subscriptionID == "" {
		return
	}
	if s.writerSubscriptions == nil {
		s.writerSubscriptions = make(map[*bufio.Writer][]string)
	}
	_ = concurrency.RunInLockWithLogger(
		&s.writerSubscriptionsMu, LockNameMcpServerTrackWriterSubscription, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.writerSubscriptions[writer] = append(s.writerSubscriptions[writer], subscriptionID)
			return nil
		},
	)
}

// unsubscribeWriterSubscriptions removes EventEmitter subscribers for a dropped connection.
func (s *Server) unsubscribeWriterSubscriptions(writer *bufio.Writer) {
	if s == nil || writer == nil {
		return
	}
	var ids []string
	_ = concurrency.RunInLockWithLogger(
		&s.writerSubscriptionsMu, LockNameMcpServerUnsubscribeWriterSubscriptions, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			ids = append([]string(nil), s.writerSubscriptions[writer]...)
			delete(s.writerSubscriptions, writer)
			return nil
		},
	)
	if s.eventEmitter == nil {
		return
	}
	for _, id := range ids {
		s.eventEmitter.Unsubscribe(id)
	}
	if len(ids) > 0 {
		s.traceLogf("[MCP_DEBUG] releaseConnectionWriter: unsubscribed %d event subscriber(s)", len(ids))
	}
}

// releaseConnectionWriter clears client and transport writers that still point at a
// closed TCP connection and abandons that connection's writer queue.
// TRACK: BLI-REDACTED — remove when: per-connection Server isolates writers.
func (s *Server) releaseConnectionWriter(writer *bufio.Writer) {
	if s == nil || writer == nil {
		return
	}
	// Drop event subscriptions before clearing writers so count cannot retain ghosts.
	s.unsubscribeWriterSubscriptions(writer)

	s.writerQueuesMu.Lock()
	if q, ok := s.writerQueues[writer]; ok && q != nil {
		q.Abandon()
		delete(s.writerQueues, writer)
		s.traceLogf("[MCP_DEBUG] releaseConnectionWriter: abandoned writer queue")
	}
	s.writerQueuesMu.Unlock()

	_ = concurrency.RunInLockWithLogger(
		&s.clientsMu, LockNameMcpServerReleaseConnectionWriter, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for _, client := range s.clients {
				if client == nil || client.Writer != writer {
					continue
				}
				client.Writer = nil
				if client.Queue != nil {
					client.Queue.MarkInactive()
					client.Queue.UpdateWriter(nil, client.Format)
				}
			}
			return nil
		},
	)
	_ = concurrency.RunInLockWithLogger(
		&s.transportMu, LockNameMcpMessageProcessorUpdateTransport, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if s.transportWriter == writer {
				s.transportWriter = nil
				s.transportFormat = nil
			}
			return nil
		},
	)
}

// findClientQueue safely finds a client message queue with proper shutdown handling
// Returns the first available queue, or nil if shutdown is in progress or no queue found
func (s *Server) findClientQueue() *MessageQueue {
	var queue *MessageQueue

	success := s.withClientsReadLock(func() bool {
		// Check shutdown context during iteration (shutdown may have been ordered)
		select {
		case <-s.shutdownCtx.Done():
			return false // Shutdown ordered, release lock immediately
		default:
		}
		for _, client := range s.clients {
			// CRITICAL: Check atomic shutdown flag on EACH loop iteration
			// This ensures we exit immediately when shutdown is ordered
			// Use atomic load for lock-free, thread-safe check
			if s.shutdownFlag.Load() == 1 {
				return false // Shutdown ordered, release lock immediately
			}
			// Also check shutdown context (defensive)
			select {
			case <-s.shutdownCtx.Done():
				return false // Shutdown ordered, release lock immediately
			default:
			}
			if client.Queue != nil {
				queue = client.Queue
				return true // Found queue, success
			}
		}
		return true // No queue found, but that's OK
	})

	if !success {
		return nil // Shutdown in progress, don't return queue
	}

	return queue
}

// findClientByID safely finds a client by ID with proper shutdown handling
// Returns the client and true if found, or nil and false if not found or shutdown in progress
func (s *Server) findClientByID(clientID string) (*ClientConnection, bool) {
	if clientID == emptyValue {
		return nil, false
	}

	var client *ClientConnection
	var exists bool

	success := s.withClientsReadLock(func() bool {
		// Check shutdown context before accessing map (shutdown may have been ordered)
		select {
		case <-s.shutdownCtx.Done():
			return false // Shutdown ordered, release lock immediately
		default:
		}
		var ok bool
		client, ok = s.clients[clientID]
		exists = ok
		return true // Success
	})

	if !success {
		return nil, false // Shutdown in progress
	}

	return client, exists
}
