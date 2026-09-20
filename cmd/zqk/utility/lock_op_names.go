// Lock operation names for WithLockTimeout / WithRLockWithLogger.
// CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A.
package utility

const (
	LockNameConfigWatcherCheckCopy     = "config_watcher_check_copy"
	LockNameConfigWatcherCheckUpdate   = "config_watcher_check_update"
	LockNameConfigWatcherRegisterFile  = "config_watcher_register_file"
	LockNameConfigWatcherStartCheck    = "config_watcher_start_check"
	LockNameConfigWatcherStop          = "config_watcher_stop"
	LockNameConfigWatcherUpdateDeleted = "config_watcher_update_deleted"
)
