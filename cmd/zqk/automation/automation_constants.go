package automation

const (
	automationProfileHuman   = "human"
	automationProfileAIAgent = "ai-agent"
)

const (
	automationEventTypeLintBypass    = "lint_bypass"
	automationAuditCodeQualityBypass = "code_quality_bypass"
	automationTargetKindLint         = "lint"
	automationSeverityMedium         = "medium"
	automationStatusComplete         = "complete"
	automationSourceGitHook          = "git_hook"
)

const (
	automationAuditKeyEventType   = "event_type"
	automationAuditKeyOperation   = "operation"
	automationAuditKeySeverity    = "severity"
	automationAuditKeyTargetKind  = "target_kind"
	automationAuditKeyTargetID    = "target_id"
	automationAuditKeyTargetPath  = "target_path"
	automationAuditKeySource      = "source"
	automationAuditKeyProjectRoot = "project_root"
)
