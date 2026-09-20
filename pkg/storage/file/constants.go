package file

const (
	ErrMsgSwallowedError                              = "swallowed error"
	ConstMiscEarlyBailoutLockContentionDetectedAfterV = `early bailout: lock contention detected after %v threshold=%v`
	ConstMiscFailedToAcquireFileLock                  = `failed to acquire file lock`
	ConstMiscFailedToCreateLockDirectory              = `failed to create lock directory`
	ConstMiscFailedToOpenLockFile                     = `failed to open lock file`
	ConstMiscFailedToReleaseFileLock                  = `failed to release file lock`
	ConstMiscFileLockAlreadyHeld                      = `file lock already held`
	ConstMiscFileLockNotHeld                          = `file lock not held`
	ConstMiscTimeoutWaitingForFileLockAfterV          = `timeout waiting for file lock after %v`
	ConstMiscFailedToAcquireLock                      = `failed to acquire lock`
	ConstMiscFailedToCloseFileLock                    = `failed to close file lock`
	ConstMiscFailedToCreateFileLock                   = `failed to create file lock`
	ConstMiscFailedToRemoveStaleLockFile              = `failed to remove stale lock file`
	ConstMiscFailedToStatLockFile                     = `failed to stat lock file`
	ConstMiscFailedToBuildFileLockMetricInstance      = `failed to build file lock metric instance`
	ConstMiscFailedToCreateFileLockMetric             = `failed to create file lock metric`
	ConstMiscFailedToGetLatestSchemaVersionForFileLoc = `failed to get latest schema version for file_lock_metric`
	ConstMiscFileLockMetricsDAcquisitionsFromSToS     = `File Lock Metrics: %d acquisitions from %s to %s`
	ConstMiscMetricIdNotSetAfterCreation              = `metric ID not set after creation`
	ConstMiscFileLockMetricsAsyncWorker               = `file_lock_metrics_async_worker`
	ConstMiscProcessingAsyncFileLockMetricsCollection = `processing async file lock metrics collection`
)
