package storage

// Promoted from export_test so dependent modules (type-aliased facades) can
// observe worker counts and the global event callback without writing unexported fields.

// ActiveWorkersForTest returns the current active worker count for operation executor tests.
func (e *OperationExecutor) ActiveWorkersForTest() int32 {
	if e == nil {
		return 0
	}
	return e.activeWorkers.Load()
}

// MaxWorkersForTest returns the configured maximum workers for operation executor tests.
func (e *OperationExecutor) MaxWorkersForTest() int {
	if e == nil {
		return 0
	}
	return e.maxWorkers
}

// GetOperationExecutorEventCallbackForTest exposes the global executor event callback for tests.
func GetOperationExecutorEventCallbackForTest() OperationExecutorEventCallback {
	return getOperationExecutorEventCallback()
}
