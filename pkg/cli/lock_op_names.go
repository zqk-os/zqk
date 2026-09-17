// Lock operation names for RunInLockWithLogger / RunInRLockWithLogger.
// CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A.
package cli

const (
	LockNameCliLoaderCacheProfile        = "cli_loader_cache_profile"
	LockNameCliLoaderCheckCache          = "cli_loader_check_cache"
	LockNameCliLoaderClearCache          = "cli_loader_clear_cache"
	LockNameCommandTrackerRecordCreated  = "command_tracker_record_created"
	LockNameCommandTrackerRecordDeleted  = "command_tracker_record_deleted"
	LockNameCommandTrackerRecordUpdated  = "command_tracker_record_updated"
	LockNameCommandTrackerSetActor       = "command_tracker_set_actor"
	LockNameCommandTrackerSetCommand     = "command_tracker_set_command"
	LockNameCommandTrackerSetContext     = "command_tracker_set_context"
	LockNameCommandTrackerSetFlags       = "command_tracker_set_flags"
	LockNameCommandTrackerSetNormalized  = "command_tracker_set_normalized"
	LockNameCommandTrackerSetOutcome     = "command_tracker_set_outcome"
	LockNameCommandTrackerToMetric       = "command_tracker_to_metric"
	LockNameMetricsStoreGetAll           = "metrics_store_get_all"
	LockNameMetricsStoreGetCommand       = "metrics_store_get_command"
	LockNameMetricsStoreLoad             = "metrics_store_load"
	LockNameMetricsStoreRecordUpdate     = "metrics_store_record_update"
	LockNameTimeoutHookBaselineDuration  = "timeout_hook_baseline_duration"
	LockNameTimeoutHookGetResetFunc      = "timeout_hook_get_reset_func"
	LockNameTimeoutHookGetServerContext  = "timeout_hook_get_server_context"
	LockNameTimeoutHookGetTimeoutCopy    = "timeout_hook_get_timeout_copy"
	LockNameTimeoutHookSetCommandTimeout = "timeout_hook_set_command_timeout"
	LockNameTimeoutHookSetEnabled        = "timeout_hook_set_enabled"
	LockNameTimeoutHookSetMaxTimeout     = "timeout_hook_set_max_timeout"
	LockNameTimeoutHookSetMetricsStore   = "timeout_hook_set_metrics_store"
	LockNameTimeoutHookSetResetFunc      = "timeout_hook_set_reset_func"
	LockNameTimeoutHookSetServerContext  = "timeout_hook_set_server_context"
)
