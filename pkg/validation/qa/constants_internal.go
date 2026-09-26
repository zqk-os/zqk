package qa

const (
	ErrMsgServiceReplayFailed        = "AuditorService: replay failed"
	LogFmtAuditorStart               = "🛡️ [QA-AUDITOR] Auditing %s:%s...\n"
	LogFmtAuditorReadFailed          = "❌ [QA-AUDITOR] Failed to read %s: %v\n"
	LogFmtAuditorMissingArtifacts    = "Warning: artifacts field missing or not a slice in object %s\n"
	LogFmtAuditorScanAST             = "🔍 [QA-AUDITOR] Scanning AST for %s...\n"
	LogFmtAuditorScanFailed          = "⚠️ [QA-AUDITOR] AST scan failed for %s: %v\n"
	LogFmtAuditorASTViolation        = "Structural Integrity Violation: %d AST violations detected."
	LogFmtAuditorASTDisparity        = "⚠️ [QA-AUDITOR] AST Disparity in %s: %s\n"
	LogFmtAuditorGuidance            = "💡 [GUIDANCE] %s\n"
	LogFmtAuditorEmitHITLFailed      = "❌ [QA-AUDITOR] Failed to emit HITL interrupt: %v\n"
	LogFmtAuditorMissingTitle        = "Warning: title field missing or not a string in object %s\n"
	LogFmtAuditorMissingDesc         = "Warning: description field missing or not a string in object %s\n"
	LogFmtAuditorSmokeAndMirrors     = "Detected 'smoke-and-mirrors' or unimplemented TODOs in object definition."
	LogFmtAuditorContentDisparity    = "⚠️ [QA-AUDITOR] Content Disparity in %s: %s\n"
	LogFmtAuditorSignFailed          = "❌ [QA-AUDITOR] Failed to sign report for %s: %v\n"
	LogFmtAuditorBuildSuccessFailed  = "❌ [QA-AUDITOR] Failed to build QASuccess for %s: %v\n"
	LogFmtAuditorCreateSuccessFailed = "❌ [QA-AUDITOR] Failed to create QASuccess for %s: %v\n"
	LogFmtAuditorSuccess             = "✅ [QA-AUDITOR] Audit passed and QASuccess issued for %s\n"
	ReasonMissingArtifacts           = "Missing required deliverable artifacts"
)

