package system

const (
	truthSentinelCmdUse         = "truth-sentinel"
	truthSentinelCmdShort       = "Run the Truth Sentinel daemon"
	truthSentinelCmdLong        = "Monitor the Knowledge Kernel WAL and perform real-time QA audits."
	truthSentinelCmdDescription = "The Truth Sentinel is the autonomous guardian of ZQK's architectural integrity."
	truthSentinelHelpDesc1      = "It monitors every status transition in the WAL and performs: "
	truthSentinelHelpDesc2      = "  1. Structural AST Audits (Enforcing DRY, Abstraction, Error Handling)"
	truthSentinelHelpDesc3      = "  2. Smoke-and-Mirrors Detection (Unimplemented TODOs, logic placeholders)"
	truthSentinelHelpDesc4      = "  3. Cryptographic Sign-off (Issuing QASuccess tokens)"
	truthSentinelHelpExample    = "Run the sentinel"
	truthSentinelHelpExampleCmd = "%s system truth-sentinel"
	truthSentinelStatusMsgFmt   = "🛡️  Truth Sentinel active for project: %s\nMonitoring WAL for status transitions...\nPress Ctrl+C to stop.\n"
	truthSentinelErrProjectRoot = "project root not found"
	truthSentinelErrWalInit     = "failed to initialize lifecycle WAL: %v"
	truthSentinelErrSignerInit  = "failed to initialize auditor signer (ensure auditor key exists in keystore): %v"
)
