// Lock operation names for RunInLockWithLogger / RunInRLockWithLogger,
// and WithLockLogger / WithRLockLogger (same string contract).
// CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A.
package memgraph

const (
	LockNameMemgraphPoolCheckActive   = "memgraph_pool_check_active"
	LockNameMemgraphPoolGetFromPool   = "memgraph_pool_get_from_pool"
	LockNameMemgraphPoolGetFromWait   = "memgraph_pool_get_from_wait"
	LockNameMemgraphPoolIncActiveNew  = "memgraph_pool_inc_active_new"
	LockNameMemgraphPoolIncWait       = "memgraph_pool_inc_wait"
	LockNameMemgraphPoolReturnFull    = "memgraph_pool_return_full"
	LockNameMemgraphPoolReturnUpdate  = "memgraph_pool_return_update"
	LockNameMemgraphPoolStats         = "memgraph_pool_stats"
	LockNameMemgraphTxCommitCheck     = "memgraph_tx_commit_check"
	LockNameMemgraphTxCommitFailed    = "memgraph_tx_commit_failed"
	LockNameMemgraphTxCommitSuccess   = "memgraph_tx_commit_success"
	LockNameMemgraphTxIsCommitted     = "memgraph_tx_is_committed"
	LockNameMemgraphTxIsRolledBack    = "memgraph_tx_is_rolled_back"
	LockNameMemgraphTxRollbackCheck   = "memgraph_tx_rollback_check"
	LockNameMemgraphTxRollbackNoTx    = "memgraph_tx_rollback_no_tx"
	LockNameMemgraphTxRollbackSuccess = "memgraph_tx_rollback_success"
)
