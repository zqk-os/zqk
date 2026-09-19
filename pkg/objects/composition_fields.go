package objects

// IsCompositionFieldAllowed reports whether a field not defined directly on kind's resolved
// spec is permitted due to trait composition (e.g. occupiable, open_countable, effort_aware, completable),
// universal storage/CAS metadata (version_context, tags), or historical/runtime tracking fields.
func IsCompositionFieldAllowed(kind, field string, spec *Spec) bool {
	// 0. Universal metadata injected by storage/CAS or universal object traits
	if field == FieldKeyVersionContext || field == FieldKeyTags {
		return true
	}

	// 1. Occupancy composition fields (claimed_by, claimed_at, occupied_by)
	if field == FieldKeyClaimedBy || field == FieldKeyClaimedAt || field == "occupied_by" {
		if KindHasNamedTrait(kind, TraitOccupiable) {
			return true
		}
		if SpecHasTrait(spec, TraitOccupiable) {
			return true
		}
		if kind == KindBacklogItem {
			return true
		}
	}

	// 2. Remaining-open composition fields (remaining_open_count)
	if field == "remaining_open_count" {
		if KindHasNamedTrait(kind, TraitOpenCountable) {
			return true
		}
		if SpecHasTrait(spec, TraitOpenCountable) {
			return true
		}
	}

	// 3. Work-envelope effort composition fields (actual_effort, estimated_effort, effort_variance)
	if field == FieldKeyEffortVariance || field == FieldKeyActualEffort || field == FieldKeyEstimatedEffort {
		if KindHasNamedTrait(kind, "effort_aware") {
			return true
		}
		if SpecHasTrait(spec, "effort_aware") {
			return true
		}
	}

	// 4. Work-envelope clock & completion composition fields (percent_complete, started_at, completed_at)
	if field == FieldKeyPercentComplete || field == "started_at" || field == "completed_at" {
		if kind != KindWorkflow {
			if KindHasNamedTrait(kind, "completable") || KindHasNamedTrait(kind, "effort_aware") {
				return true
			}
			if SpecHasTrait(spec, "completable") || SpecHasTrait(spec, "effort_aware") {
				return true
			}
		}
	}

	// 5. Workflow and priority plan legacy fields
	if kind == KindWorkflow {
		switch field {
		case "name", "workflow_steps", "trigger_events":
			return true
		}
	}
	if kind == KindPriorityPlan {
		switch field {
		case "commit_hashes", "commit_refs", "notes":
			return true
		}
	}
	if kind == KindTechnicalDebt {
		switch field {
		case "impact", "priority", "remediation":
			return true
		}
	}
	if kind == KindPolicy {
		switch field {
		case "enforcement_level", "custom_field_1", "custom_field_2":
			return true
		}
	}
	if kind == KindCriteria {
		switch field {
		case "evidence",
			"acceptance_method",
			"actual_effort",
			"actual_result",
			"artifacts",
			"backlog_item_ref",
			"backlog_item_refs",
			"bundle_command_fingerprint",
			"bundle_log_path",
			"commit_refs",
			"completeness_validation",
			"conditions",
			"context",
			"criteria_type",
			"criterion_id",
			"evidence_refs",
			"job_id",
			"notes",
			"origin_project",
			"origin_system",
			"priority_tier",
			"requirement_ref",
			"success_criteria",
			"verification_method",
			"workstream_refs":
			return true
		}
	}
	if kind == KindAgentTask {
		switch field {
		case "actual_effort",
			"agent_heartbeat",
			"agent_pid",
			"backlog_item_ref",
			"branch_name",
			"branch_ref",
			"claimed_at",
			"claimed_by",
			"commit_hash",
			"git_branch",
			"commit_hashes",
			"commit_refs",
			"completed_at",
			"cvs_ref",
			"description",
			"estimated_effort",
			"execution_prompt",
			"exit_when_cvs_completed",
			"for_ref",
			"hourglass_on",
			"milestone_ref",
			"milestone_refs",
			"notes",
			"parent_ref",
			"pipeline_ref",
			"priority_plan_ref",
			"prompt",
			"started_at",
			"task_steps":
			return true
		}
	}

	// 6. Backlog item legacy and execution-tracking compatibility fields
	if kind == KindBacklogItem {
		switch field {
		case "acceptance_criteria",
			"acceptance_evidence",
			"actual_effort_hours",
			"actual_result",
			"assignee_persona_ref",
			"base_sha",
			"blocked_by_refs",
			"bundle_command_fingerprint",
			"bundle_log_path",
			"commit_hash",
			"commit_hashes",
			"commit_hashes[]",
			"commit_refs",
			"completion_notes",
			"decision_refs",
			"document_refs",
			"dependency_refs",
			"estimated_effort_hours",
			"evaluation_rating",
			"evidence_refs",
			"external_refs",
			"instructions",
			"job_id",
			"level_up_rating",
			"metadata",
			"milestone_id",
			"milestone_ref",
			"next_action",
			"note",
			"occupied_by",
			"owned_by",
			"owner",
			"path_match",
			"persona",
			"persona_ref",
			"priority_plan_id",
			"rating",
			"related_docs",
			"related_items",
			"requirement_ref",
			"requirement_refs-",
			"resolution_reason",
			"score",
			"source",
			"stakeholder_refs",
			"stakeholder_type",
			"story_points",
			"team_configuration_ref",
			"test_case_refs",
			"test_failure_reason",
			"technical_spec_refs",
			"tracker",
			"type",
			"validation_evidence",
			"verification_hash",
			"workstream_ref":
			return true
		}
	}
	if kind == KindTestCase {
		switch field {
		case "verification_hash", "verified_artifact_refs":
			return true
		}
	}
	if kind == KindRequirement {
		switch field {
		case "verification_hash":
			return true
		}
	}
	if kind == KindZqkSession {
		switch field {
		case FieldKeyAgreementMode,
			FieldKeyConsumerKernelRef,
			FieldKeyProviderKernelRef,
			FieldKeyResourceRef,
			FieldKeySessionMode,
			FieldKeyTokenID,
			FieldKeyConsumedUnits,
			FieldKeyMaxUnits,
			FieldKeyTermType,
			FieldKeyRevokedAt:
			return true
		}
	}
	return false
}

// SpecHasTrait reports whether a resolved spec contains the given trait.
func SpecHasTrait(spec *Spec, trait string) bool {
	if spec == nil {
		return false
	}
	for _, t := range spec.ResolvedTraits {
		if t == trait {
			return true
		}
	}
	for _, t := range spec.Traits {
		if t == trait {
			return true
		}
	}
	return false
}
