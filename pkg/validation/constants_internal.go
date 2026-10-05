package validation

// Common Constants
const (
	PathAliasCache = "cache"
	pathSeparator  = "/"
	lineSeparator  = "\n"
)

// Error Messages
const (
	ErrMsgCreateCacheDir          = "failed to create cache directory"
	ErrMsgReadCacheFile           = "failed to read cache file"
	ErrMsgMarshalCache            = "failed to marshal cache"
	ErrMsgWriteCacheFile          = "failed to write cache file"
	ErrMsgRenameCacheFile         = "failed to rename cache file"
	ErrMsgRemoveCacheFile         = "failed to remove cache file"
	ErrMsgRemoveLockFile          = "failed to remove lock file: %v\n"
	ErrMsgRemoveStaleLock         = "failed to remove stale lock file: %v\n"
	ErrMsgOpenLockFile            = "failed to open lock file"
	ErrMsgUpdateLockTimestamp     = "failed to update lock file timestamp: %v\n"
	ErrMsgReleaseLock             = "failed to release lock: %v\n"
	ErrMsgLockFailedLoad          = "lock failed in Load: %v\n"
	ErrMsgLockFailedSave          = "lock failed in Save: %v\n"
	ErrMsgLockFailedGet           = "lock failed in Get: %v\n"
	ErrMsgLockFailedGetStale      = "lock failed in GetStale: %v\n"
	ErrMsgLockFailedGetAll        = "lock failed in GetAll: %v\n"
	ErrMsgLockFailedSet           = "lock failed in Set: %v\n"
	ErrMsgLockFailedInvalidate    = "lock failed in Invalidate: %v\n"
	ErrMsgLockFailedInvalidatePat = "lock failed in InvalidateByPattern: %v\n"
	ErrMsgLockFailedClear         = "lock failed in Clear: %v\n"
	ErrMsgLockFailedGetByTier     = "lock failed in GetByTier: %v\n"
	ErrMsgLockFailedCount         = "lock failed in Count: %v\n"
	ErrMsgCacheLocked             = "cache file is locked by another process"
	ErrMsgStaleLockCleanup        = "cache file is locked by another process (stale lock cleanup failed)"
	ErrMsgRecreateLockFailed      = "cache file is locked by another process (failed to recreate lock file)"
)

// More messages and logs
const (
	ErrMsgSetValFunc        = "Failed to set validation function"
	ErrMsgSetResCallback    = "Failed to set result callback"
	ErrMsgSetEvCallback     = "Failed to set event callback"
	ErrMsgSetQEmptyCallback = "Failed to set queue empty callback"
	ErrMsgSetTimeouts       = "Failed to set timeouts"
	ErrMsgLoadCache         = "Failed to load validation cache"
	LogMsgValStarted        = "Async validator started"
	ErrMsgInitStopSeq       = "Failed to initialize stop sequence"
)

// Message and fixture strings composition roots still alias.
const ()
