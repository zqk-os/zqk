// Lock operation names for RunInLockWithLogger / RunInRLockWithLogger.
// CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A.
package rollback

const (
	LockNameRollbackStoreAppend = "rollback_store_append"
	LockNameRollbackStoreGet    = "rollback_store_get"
	LockNameRollbackStoreList   = "rollback_store_list"
	LockNameRollbackStoreRetain = "rollback_store_retain"
)
