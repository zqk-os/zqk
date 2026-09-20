// Lock operation names for RunInLockWithLogger / RunInRLockWithLogger.
// CONSTANTS_AND_DRY_INVENTORY_PLAN Phase A.
package git

const (
	LockNameGitIntegrationSetLogger  = "git_integration_set_logger"
	LockNameGitMetricsGet            = "git_metrics_get"
	LockNameGitMetricsRecordAnalysis = "git_metrics_record_analysis"
	LockNameGitMetricsRecordLinking  = "git_metrics_record_linking"
	LockNameGitMetricsRecordRetry    = "git_metrics_record_retry"
	LockNameGitMetricsRecordTimeout  = "git_metrics_record_timeout"
	LockNameGitMetricsReset          = "git_metrics_reset"
)
