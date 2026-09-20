package validation

// semaphoreFullShouldWarn reports whether a 15s validation-slot wait should
// surface as semaphore_full. A shrinking queue is healthy backpressure during
// a full check (8k objects, 16 slots). Warn only when a prior sample exists
// and the queue did not drain — that is the stuck/non-draining signal.
func semaphoreFullShouldWarn(prevQueue, curQueue int) bool {
	if prevQueue <= 0 {
		return false
	}
	return curQueue >= prevQueue
}
