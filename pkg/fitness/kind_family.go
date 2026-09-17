package fitness

// KindFamily groups kinds by how lifecycle error / fitness should be interpreted.
type KindFamily string

const (
	// KindFamilyWorkAttempt — jobs/tasks/sessions where error means this attempt failed.
	KindFamilyWorkAttempt KindFamily = "work_attempt"
	// KindFamilyExecutionWork — backlog/debt work items; error/halted is process halt, not missing fields.
	KindFamilyExecutionWork KindFamily = "execution_work"
	// KindFamilyIdentityGovernance — accounts, plans, roadmaps: prefer domain states; never encode completeness as error.
	KindFamilyIdentityGovernance KindFamily = "identity_governance"
	// KindFamilyStrategicContent — mission/goal/req/crit: content objects; completeness ≠ process error.
	KindFamilyStrategicContent KindFamily = "strategic_content"
	// KindFamilyTelemetry — metrics/audit streams; error may mean capture failure.
	KindFamilyTelemetry KindFamily = "telemetry"
	// KindFamilyOther — default: demote only for process_failure if kind lifecycle includes error.
	KindFamilyOther KindFamily = "other"
)

// FamilyOfKind returns the fitness kind family for a schema kind name.
func FamilyOfKind(kind string) KindFamily {
	switch kind {
	case "agent_task", "scheduler_job", "convergence_session", "mcp_session", "zqk_session",
		"agent_feed", "agent_instruction", "agent_onboarding_preparation":
		return KindFamilyWorkAttempt
	case "backlog_item", "technical_debt", "tde_envelope", "risk_blocker", "question":
		return KindFamilyExecutionWork
	case "account", "organization", "priority_plan", "roadmap", "workstream",
		"team_configuration", "role", "keystore_entry", "auth_strategy":
		return KindFamilyIdentityGovernance
	case "mission", "vision", "goal", "requirement", "criteria", "milestone",
		"strategic_plan", "strategic_context", "glossary_term", "doc_entry",
		"test_case", "workflow", "persona", "prompt_template":
		return KindFamilyStrategicContent
	case "audit_aggregation_metric", "audit_event", "code_quality_metric",
		"scheduler_health_metric", "base_metric":
		return KindFamilyTelemetry
	default:
		return KindFamilyOther
	}
}

// AutofixMayDemoteKind reports whether autofix is allowed to set status=error for this kind
// when a process_failure finding is present. Identity/governance never demote via autofix
// (even if a future lifecycle adds error); work/execution/telemetry/other may when the
// lifecycle includes error (enforced separately by applySpecFix).
func AutofixMayDemoteKind(kind string) bool {
	switch FamilyOfKind(kind) {
	case KindFamilyIdentityGovernance:
		return false
	default:
		return true
	}
}
