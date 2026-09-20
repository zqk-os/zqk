package audit

import "sync/atomic"

var eventCreationDepth int32

// IsCreatingEvent reports whether the current goroutine stack is already
// creating an audit event (cycle guard).
func IsCreatingEvent() bool {
	return atomic.LoadInt32(&eventCreationDepth) > 0
}

// BeginEventCreation marks the start of audit event creation.
// The returned function must be deferred.
func BeginEventCreation() func() {
	atomic.AddInt32(&eventCreationDepth, 1)
	return func() {
		atomic.AddInt32(&eventCreationDepth, -1)
	}
}
