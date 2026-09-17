// Lock operation names for RunInLockWithLogger / RunInRLockWithLogger.
// CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A.
package runtime

const (
	LockNameGoroutineManagerDetectLeaksCopy     = "goroutine_manager_detect_leaks_copy"
	LockNameGoroutineManagerGetStatsCopy        = "goroutine_manager_get_stats_copy"
	LockNameGoroutineManagerLeakCheck           = "goroutine_manager_leak_check"
	LockNameGoroutineManagerMarkLeaked          = "goroutine_manager_mark_leaked"
	LockNameGoroutineManagerPostcleanupCheck    = "goroutine_manager_postcleanup_check"
	LockNameGoroutineManagerSetLeakChannel      = "goroutine_manager_set_leak_channel"
	LockNameGoroutineManagerSetMax              = "goroutine_manager_set_max"
	LockNameGoroutineManagerSetRunning          = "goroutine_manager_set_running"
	LockNameGoroutineManagerSetShutdownTimeout  = "goroutine_manager_set_shutdown_timeout"
	LockNameGoroutineManagerShutdownTimeoutCopy = "goroutine_manager_shutdown_timeout_copy"
	LockNameGoroutineManagerStatsRead           = "goroutine_manager_stats_read"
	LockNameGoroutineManagerStopAllGetIds       = "goroutine_manager_stop_all_get_ids"
	LockNameGoroutineManagerStopCheck           = "goroutine_manager_stop_check"
	LockNameGoroutineManagerStopGet             = "goroutine_manager_stop_get"
	LockNameGoroutineManagerStopUpdate          = "goroutine_manager_stop_update"
)
