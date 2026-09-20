package system

const (
	truthSentinelFlagOnce        = "once"
	truthSentinelFlagIDs         = "ids"
	truthSentinelErrOnceNeedsIDs = "--once requires --ids"
	truthSentinelOnceDoneFmt     = "truth-sentinel: audited %d id(s)\n"
	truthSentinelStatusMsgFmt    = "🛡️  Truth Sentinel active for project: %s\nMonitoring WAL for status transitions...\nPress Ctrl+C to stop.\n"
	truthSentinelErrProjectRoot  = "project root not found"
	truthSentinelErrWalInit      = "failed to initialize lifecycle WAL: %v"
	truthSentinelErrSignerInit   = "failed to initialize auditor signer (ensure auditor key exists in keystore): %v"
)
