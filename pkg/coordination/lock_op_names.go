// Lock operation names for RunInLockWithLogger / RunInRLockWithLogger.
// CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A.
package coordination

const (
	LockNameCoordinatorEmitOperationalCopy     = "coordinator_emit_operational_copy"
	LockNameCoordinatorEmitOperationalSyncCopy = "coordinator_emit_operational_sync_copy"
	LockNameCoordinatorHasSubscribers          = "coordinator_has_subscribers"
	LockNameCoordinatorSubscribe               = "coordinator_subscribe"
	LockNameCoordinatorUnsubscribe             = "coordinator_unsubscribe"
)
