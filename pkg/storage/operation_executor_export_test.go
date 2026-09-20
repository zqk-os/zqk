package storage

// ActiveWorkersForTest returns the current active worker count for operation executor tests.
func (e *OperationExecutor) ActiveWorkersForTest() int32 {
	return e.activeWorkers.Load()
}

// MaxWorkersForTest returns the configured maximum workers for operation executor tests.
func (e *OperationExecutor) MaxWorkersForTest() int {
	return e.maxWorkers
}
