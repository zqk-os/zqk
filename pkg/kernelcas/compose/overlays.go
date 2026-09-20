package compose

import (
	"github.com/zqk-os/zqk/pkg/objects"
)

// kindOverlayRules migrates former Go customRuleValidators into declarative rules.
// TRACK: BLI-1785786996110276000-11dde761 / BLI-1785786997399161000-76ea6811
func kindOverlayRules(objectKind, intent string) []Rule {
	var rules []Rule
	switch objectKind {
	case objects.KindBacklogItem:
		rules = backlogItemOverlay()
		if intent == IntentTransition {
			rules = append(rules, backlogItemTransitionOverlay()...)
		}
	case objects.KindGoal:
		rules = goalOverlay()
	case objects.KindMilestone:
		rules = milestoneOverlay()
	case objects.KindTestCase:
		rules = testCaseOverlay()
	case objects.KindPriorityPlan:
		rules = priorityPlanOverlay()
	case objects.KindRequirement:
		rules = requirementOverlay()
		// Create + promote off conceptual: empty criteria_refs is a missing
		// traceability pipeline, not an optional field. Updates of already-originated
		// REQs stay editable so residual inventory can be repaired.
		// TRACK: POL-AGENT-TPM-TRACE-PIPELINE-001
		if intent == IntentCreate || intent == IntentTransition {
			rules = append(rules, requirementTracePipelineOverlay()...)
		}
	case objects.KindCriteria:
		rules = criteriaOverlay()
	case objects.KindWorkstream:
		rules = workstreamOverlay()
	case objects.KindWorkflow:
		rules = workflowOverlay()
	case objects.KindPipeline:
		rules = pipelineOverlay()
	case objects.KindConvergenceSession:
		rules = convergenceSessionOverlay()
	case objects.KindDisplay:
		rules = displayOverlay()
	case objects.KindDepartment:
		rules = departmentOverlay()
	case objects.KindDivision:
		rules = divisionOverlay()
	case objects.KindOrganizationalChange:
		rules = organizationalChangeOverlay()
	case objects.KindMission:
		rules = missionOverlay()
	case objects.KindOrganization:
		rules = organizationOverlay()
	case objects.KindComponent:
		rules = componentOverlay()
	case objects.KindDecision:
		rules = selfDAGRules(objects.FieldKeyDecisionRefs)
	case objects.KindGlossaryTerm:
		rules = selfDAGRules(objects.FieldKeyAliasRefs)
	case objects.KindQuestion:
		rules = selfDAGRules(objects.FieldKeyRelatedQuestionRefs)
	case objects.KindScenario:
		rules = selfDAGRules(objects.FieldKeyScenarioRefs)
	case objects.KindZqkSession:
		rules = selfDAGRules(objects.FieldKeyParentSessionRef)
	}

	if intent == IntentTransition {
		if has, _ := objects.KindHasTrait(objectKind, objects.TraitOccupiable); has {
			rules = append(rules, occupiableTransitionOverlay()...)
		}
	}
	rules = append(rules, traitOverlayRules(objectKind, intent)...)

	// All narrative planning objects crossing CAS boundary require a description unless preliminary or terminal.
	// TRACK: BLI-SPEC-CAS-DESC-GATE-001 / REQ-DATA-MODEL-CAS-DESC-001
	if objects.KindRequiresDescription(objectKind) {
		rules = append(rules, Rule{
			ID: "require_description_cas_boundary",
			Op: OpRequireField,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyDescription,
				"message":             "description must be populated (CAS boundary requirement)",
				"skip_preliminary":    true,
				"skip_terminal":       true,
			},
		})
	}
	return rules
}

// selfDAGRules refuse ID==self and 2-cycles on a same-kind ref field.
// TRACK: BLI-KERNEL-REF-GRAPH-ACYCLIC-001
func selfDAGRules(field string) []Rule {
	return []Rule{
		{
			ID: "self_id_" + field,
			Op: OpRefuseSelfRef,
			Config: map[string]any{
				objects.FieldKeyField: field,
				"message":             field + " must not contain this object's id",
			},
		},
		{
			ID: "two_cycle_" + field,
			Op: OpRefuseTwoCycle,
			Config: map[string]any{
				objects.FieldKeyField: field,
				"message":             field + " must not form a 2-cycle",
			},
		},
	}
}

func backlogItemOverlay() []Rule {
	return []Rule{
		{
			ID: "bli_hierarchical_chain",
			Op: OpRequireRefAny,
			Config: map[string]any{
				"fields":                   []any{"goal_refs", "requirement_refs"},
				"skip_terminal":            true,
				"skip_preliminary":         true,
				"skip_heal_glass":          true,
				"message":                  "backlog_item must have at least one goal_ref or requirement_ref to maintain the Hierarchical Chain of Integrity",
				"field_label":              "goal_refs/requirement_refs",
				objects.FieldKeyObjectKind: objects.KindBacklogItem,
			},
		},
		{
			ID: "bli_priority_when_ready",
			Op: OpRequireFieldWhenStatus,
			Config: map[string]any{
				objects.FieldKeyField:    objects.FieldKeyPriority,
				"fields":                 []any{objects.FieldKeyPriority, objects.FieldKeyPriorityTier},
				objects.FieldKeyStatuses: []any{objects.ObjectStatusPlanned, objects.ObjectStatusInProgress},
				"message":                "priority or priority_tier must be set when backlog_item is planned or in_progress",
			},
		},
		{
			ID: "bli_legitimate_priority_values",
			Op: OpValidatePriorityValues,
			Config: map[string]any{
				"priority_message": "priority must be one of (critical, high, medium, low)",
				"tier_message":     "priority_tier must be one of (P0, P1, P2, P3)",
			},
		},
		{
			ID: "bli_refuse_complete_plan",
			Op: OpRefusePlanStatus,
			Config: map[string]any{
				"plan_field":    objects.FieldKeyPriorityPlanRef,
				"refuse_when":   []any{objects.ObjectStatusComplete},
				"message_fmt":   "cannot assign priority_plan_ref %s because the target priority plan is in '%s' state (Scope Creep Protection)",
				"new_link_only": true,
			},
		},
		{
			// Sealed-column immutability: once a plan is execution-facing (active / in_progress),
			// refuse *new* membership. Mid-flight gaps get a new grooming (intake) column —
			// never stuff the locked set. Existing links (same priority_plan_ref) still update freely.
			// TRACK: GOAL-1786331776059716000-96be46df — sealed-plan membership refuse
			ID: "bli_refuse_new_link_execution_facing_plan",
			Op: OpRefusePlanStatus,
			Config: map[string]any{
				"plan_field":    objects.FieldKeyPriorityPlanRef,
				"refuse_when":   []any{objects.ObjectStatusActive, objects.ObjectStatusInProgress},
				"message_fmt":   "cannot assign priority_plan_ref %s: priority plan is '%s' (execution-facing / sealed); open a grooming plan for new work instead of stuffing the locked column",
				"new_link_only": true,
			},
		},
		{
			// Hold invariant: planned work cannot remain on a complete plan (same class as new-link refuse).
			ID: "bli_planned_refuses_complete_plan",
			Op: OpRefusePlanStatus,
			Config: map[string]any{
				"plan_field":         objects.FieldKeyPriorityPlanRef,
				"when_object_status": []any{objects.ObjectStatusPlanned},
				"refuse_when":        []any{objects.ObjectStatusComplete},
				"message_fmt":        "backlog_item cannot stay planned on complete priority plan %s (status %s); reopen the plan or complete/move the item",
			},
		},
		{
			ID: "bli_execution_facing_membership",
			Op: OpRefuseExecutionFacingMembership,
			Config: map[string]any{
				"plan_field":  objects.FieldKeyPriorityPlanRef,
				"message_fmt": "backlog item status %q cannot link to %s priority plan %s; demote the plan to grooming (or promote the item to planned) so execution-facing scope stays ready-only",
			},
		},
		{
			// State ceiling: archived is the history membrane of the Gantt container.
			// A child cannot occupy archived while its priority_plan is live or complete.
			// Draft-plane items with no priority_plan_ref still park. TRACK: RUL-1782235658105562000-b27c8dfc
			ID: "bli_archived_requires_archived_plan",
			Op: OpRefusePlanStatus,
			Config: map[string]any{
				"plan_field":          objects.FieldKeyPriorityPlanRef,
				"when_object_status":  []any{objects.ObjectStatusArchived},
				"require_plan_status": []any{objects.ObjectStatusArchived},
				"message_fmt":         "backlog_item cannot be archived because priority_plan %s is in '%s' status (must be 'archived'; promote the plan)",
			},
		},
		{
			ID: "bli_refuse_duplicate_refs",
			Op: OpRefuseDuplicateRefs,
		},
		{
			ID: "bli_refuse_unknown_fields",
			Op: OpRefuseUnknownFields,
		},
	}
}

// backlogItemTransitionOverlay encodes lifecycle ready-state promote gates for TransitionStatus.
// TRACK: BLI-1785786993294193000-ebe57e46
func backlogItemTransitionOverlay() []Rule {
	return []Rule{
		{
			ID: "bli_transition_plan_ref",
			Op: OpRequireFieldWhenStatus,
			Config: map[string]any{
				objects.FieldKeyField:    objects.FieldKeyPriorityPlanRef,
				objects.FieldKeyStatuses: []any{objects.ObjectStatusPlanned, objects.ObjectStatusInProgress},
				"message":                "priority_plan_ref must be set when backlog_item is planned or in_progress",
			},
		},
		{
			ID: "bli_transition_milestone",
			Op: OpRequireFieldWhenStatus,
			Config: map[string]any{
				"fields":                 []any{objects.FieldKeyMilestoneRef, objects.FieldKeyMilestoneRefs},
				objects.FieldKeyField:    objects.FieldKeyMilestoneRef,
				objects.FieldKeyStatuses: []any{objects.ObjectStatusPlanned, objects.ObjectStatusInProgress},
				"message":                "at least one milestone_ref must be linked when backlog_item is planned or in_progress",
			},
		},
		{
			// CRI-SHOVEL-READY on promote INTO in_progress. Destination planned is
			// gated by the lifecycle token (except in_progress→planned demote).
			// TRACK: CRIT-1785885889228395000-15c56d02 — CAP dor-gap vs empty-column split.
			ID: "bli_transition_shovel_ready",
			Op: OpShovelReadyWhenStatus,
			Config: map[string]any{
				objects.FieldKeyStatuses: []any{objects.ObjectStatusInProgress},
				"message":                "CRI-SHOVEL-READY: in_progress backlog items need requirement_refs|technical_spec_refs, criteria_refs, estimated_effort|scope, and persona|stakeholders (and must not be blocked)",
			},
		},
		// Plan immutability / execution gate: in_progress BLI requires shovel-ready or locked plan.
		// TRACK: BLI-1783845980884549000-014a1c61
		{
			ID: "bli_in_progress_requires_execution_facing_plan",
			Op: OpRefusePlanStatus,
			Config: map[string]any{
				"plan_field":          objects.FieldKeyPriorityPlanRef,
				"when_object_status":  []any{objects.ObjectStatusInProgress},
				"require_plan_status": []any{objects.ObjectStatusActive, objects.ObjectStatusInProgress},
				"message_fmt":         "backlog_item cannot be in_progress because priority_plan %s is in '%s' status (must be 'active' or 'in_progress')",
			},
		},
	}
}

func goalOverlay() []Rule {
	return []Rule{
		{
			ID: "goal_desc_min",
			Op: OpMinStringLen,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyDescription,
				"min_len":             20,
				"message":             "goal description must be at least 20 characters to ensure semantic depth",
			},
		},
		{
			ID: "goal_metric_when_active",
			Op: OpRequireFieldWhenActive,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyMetric,
				"message":             "active goal must specify a metric to evaluate launch value",
				objects.FieldKeyKind:  objects.KindGoal,
			},
		},
		{
			ID: "goal_target_when_active",
			Op: OpRequireFieldWhenActive,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyTarget,
				"message":             "active goal must specify a target threshold to evaluate launch value",
				objects.FieldKeyKind:  objects.KindGoal,
			},
		},
		{
			ID: "goal_plan_active_gate",
			Op: OpRefusePlanStatus,
			Config: map[string]any{
				"plan_field":          objects.FieldKeyPriorityPlanRef,
				"when_object_status":  []any{objects.ObjectStatusActive},
				"require_plan_status": []any{objects.ObjectStatusActive, objects.ObjectStatusInProgress},
				"message_fmt":         "goal status cannot be set to 'active' because the linked priority_plan %s is in '%s' status (must be 'active' or 'in_progress')",
			},
		},
		{
			// Occupancy is child-owned. TRACK: BLI-KERNEL-REF-GRAPH-ACYCLIC-001
			ID: "goal_no_backlog_item_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyBacklogItemRefs,
				"message":             "goal must not store backlog_item_refs; occupancy is child-owned via backlog_item.goal_refs",
			},
		},
		{
			ID: "goal_no_requirement_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyRequirementRefs,
				"message":             "goal must not store requirement_refs; occupancy is child-owned via requirement.goal_refs",
			},
		},
		{
			ID: "goal_no_milestone_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyMilestoneRefs,
				"message":             "goal must not store milestone_refs; occupancy is child-owned via milestone.goal_refs",
			},
		},
	}
}

func milestoneOverlay() []Rule {
	rules := []Rule{
		{
			// Occupancy is child-owned. TRACK: BLI-KERNEL-REF-GRAPH-ACYCLIC-001
			ID: "mil_no_backlog_item_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyBacklogItemRefs,
				"message":             "milestone must not store backlog_item_refs; occupancy is child-owned via backlog_item.milestone_refs",
			},
		},
		{
			ID: "mil_no_requirement_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyRequirementRefs,
				"message":             "milestone must not store requirement_refs; occupancy is child-owned via requirement.milestone_refs",
			},
		},
		{
			ID: "mil_active_criteria_refs",
			Op: OpRequireFieldWhenStatus,
			Config: map[string]any{
				objects.FieldKeyField:    objects.FieldKeyCriteriaRefs,
				objects.FieldKeyStatuses: []any{objects.ObjectStatusInProgress},
				"message":                "criteria_refs must be linked when milestone is active",
			},
		},
	}
	out := append([]Rule{}, rules...)
	out = append(out, selfDAGRules(objects.FieldKeyPrerequisiteRefs)...)
	out = append(out, selfDAGRules(objects.FieldKeyBlockedByRefs)...)
	return out
}

func workstreamOverlay() []Rule {
	rules := []Rule{
		{
			// Occupancy is child-owned. TRACK: BLI-KERNEL-REF-GRAPH-ACYCLIC-001
			ID: "ws_no_requirement_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyRequirementRefs,
				"message":             "workstream must not store requirement_refs; occupancy is child-owned via requirement.workstream_refs",
			},
		},
		{
			ID: "ws_no_milestone_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyMilestoneRefs,
				"message":             "workstream must not store milestone_refs; occupancy is child-owned via milestone.workstream_refs",
			},
		},
	}
	return append(rules, selfDAGRules(objects.FieldKeyWorkstreamRefs)...)
}

func workflowOverlay() []Rule {
	return []Rule{
		{
			ID: "workflow_refuse_percent_complete",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyPercentComplete,
				"message":             "percent_complete is a lifecycle document field, not a workflow field (TDE-CEF-CAS-SPEC-FIELD-DIFF-001)",
			},
		},
		{
			ID: "workflow_refuse_unknown_fields",
			Op: OpRefuseUnknownFields,
		},
		{
			ID: "workflow_refuse_duplicate_refs",
			Op: OpRefuseDuplicateRefs,
		},
	}
}

func testCaseOverlay() []Rule {
	return []Rule{
		{
			ID: "tc_path_when_active",
			Op: OpRequireFieldWhenActive,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyPathOrID,
				"message":             "test_case must specify a valid path_or_id to be executable",
				objects.FieldKeyKind:  objects.KindTestCase,
				"include_terminal":    true,
			},
		},
	}
}

func priorityPlanOverlay() []Rule {
	return []Rule{
		{
			ID: "pri_active_order",
			Op: OpRequireFieldWhenStatus,
			Config: map[string]any{
				objects.FieldKeyField:    objects.FieldKeyActiveOrder,
				objects.FieldKeyStatuses: []any{objects.ObjectStatusActive},
				"message":                "active_order must be set when priority_plan is active",
			},
		},
		{
			// Execution lock and terminal statuses leave the roadmap queue.
			// Numeric active_order is only for shovel-ready active plans (unique among actives).
			// TRACK: BLI-1785439367722386000-7bd43e71 / BLI-1785439369431933000-f0cccd6c
			ID: "pri_active_order_cleared_when_locked",
			Op: OpRefuseFieldWhenStatus,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyActiveOrder,
				objects.FieldKeyStatuses: []any{
					objects.ObjectStatusInProgress,
					objects.ObjectStatusComplete,
					objects.ObjectStatusArchived,
					objects.ObjectStatusCancelled,
				},
				"message": "active_order must be unset once priority_plan is in_progress, complete, archived, or cancelled",
			},
		},
		{
			// Spec removed parent→child membership; instances must not carry the key.
			// TRACK: BLI-1785439365092316000-2c09c364 — child-owned priority_plan membership.
			ID: "pri_no_backlog_item_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyBacklogItemRefs,
				"message":             "priority_plan must not store backlog_item_refs; membership is child-owned via backlog_item.priority_plan_ref (use zqk pplan add/remove)",
			},
		},
		{
			ID: "pri_no_backlog_related_refs",
			Op: OpRefuseRefPrefix,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyRelatedObjectRefs,
				"prefix":              "BLI-",
				"message":             "priority_plan.related_object_refs must not contain backlog items; membership is child-owned via backlog_item.priority_plan_ref",
			},
		},
	}
}

func requirementOverlay() []Rule {
	return []Rule{
		{
			ID: "require_description",
			Op: OpRequireField,
			Config: map[string]any{
				objects.FieldKeyField: "description",
				"message":             "description must be populated (CAS completeness barrier)",
				"skip_preliminary":    true,
				"skip_terminal":       true,
			},
		},
		{
			ID: "req_refuse_complete_with_unverified_criteria",
			Op: OpRefuseChildStatus,
			Config: map[string]any{
				"when_parent_status": []any{objects.ObjectStatusComplete},
				"child_field":        objects.FieldKeyCriteriaRefs,
				// These are the criteria lifecycle's own unverified statuses. The rule previously
				// named "unverified" and "not_started", neither of which criteria has ever
				// defined, so it matched nothing and the barrier did not exist. archived and
				// rejected stay out on purpose: neither is pending evidence, so neither is an
				// obstacle to completing the parent — same treatment they get in
				// validation.refStatusRules for the linked-criteria constraint.
				"refuse_child_status": []any{
					objects.ObjectStatusAwaitingVerification,
					objects.ObjectStatusInProgress,
					objects.ObjectStatusBlocked,
				},
				"message_fmt": "cannot complete requirement because criteria %s is in '%s' status (must be validated or complete, or not linked)",
			},
		},
		{
			// Occupancy is child-owned. TRACK: BLI-KERNEL-REF-GRAPH-ACYCLIC-001
			ID: "req_no_backlog_item_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyBacklogItemRefs,
				"message":             "requirement must not store backlog_item_refs; occupancy is child-owned via backlog_item.requirement_refs",
			},
		},
		{
			ID: "req_no_crit_related_refs",
			Op: OpRefuseRefPrefix,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyRelatedObjectRefs,
				"prefix":              "CRIT-",
				"message":             "requirement.related_object_refs must not contain criteria; composition is parent-owned via requirement.criteria_refs",
			},
		},
		{
			ID: "req_no_test_case_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyTestCaseRefs,
				"message":             "requirement must not store test_case_refs; occupancy is child-owned via test_case.requirement_refs",
			},
		},
		{
			ID: "req_no_technical_spec_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyTechnicalSpecRefs,
				"message":             "requirement must not store technical_spec_refs; occupancy is child-owned via technical_spec.requirement_refs",
			},
		},
	}
}

// requirementTracePipelineOverlay refuses a non-preliminary requirement without
// criteria_refs. Conceptual draft is the only status that may exist without a
// pipeline; leaving it requires `zqk workflow gen-trace-pipeline <REQ>`.
// TRACK: POL-AGENT-TPM-TRACE-PIPELINE-001
func requirementTracePipelineOverlay() []Rule {
	return []Rule{
		{
			ID: "req_trace_pipeline_criteria",
			Op: OpRequireField,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyCriteriaRefs,
				"message":             "requirement.criteria_refs must be populated before leaving conceptual (CAS traceability barrier). Run: zqk workflow gen-trace-pipeline <REQ-id>",
				"skip_preliminary":    true,
				"skip_terminal":       true,
			},
		},
	}
}

func pipelineOverlay() []Rule {
	return []Rule{
		{
			// Occupancy is child-owned. TRACK: BLI-KERNEL-REF-GRAPH-ACYCLIC-001
			ID: "pipeline_no_agent_task_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyAgentTaskRefs,
				"message":             "pipeline must not store agent_task_refs; occupancy is child-owned via agent_task.pipeline_ref",
			},
		},
	}
}

func convergenceSessionOverlay() []Rule {
	return []Rule{
		{
			// Occupancy is child-owned. TRACK: BLI-KERNEL-REF-GRAPH-ACYCLIC-001
			ID: "cvs_no_backlog_item_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyBacklogItemRefs,
				"message":             "convergence_session must not store backlog_item_refs; occupancy is child-owned via backlog_item.convergence_session_ref",
			},
		},
	}
}

func displayOverlay() []Rule {
	return []Rule{
		{
			// Occupancy is child-owned. TRACK: BLI-KERNEL-REF-GRAPH-ACYCLIC-001
			ID: "display_no_component_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyComponentRefs,
				"message":             "display must not store component_refs; occupancy is child-owned via component.display_ref",
			},
		},
	}
}

func departmentOverlay() []Rule {
	return []Rule{
		{
			ID: "dept_no_team_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyTeamRefs,
				"message":             "department must not store team_refs; occupancy is child-owned via team.department_ref",
			},
		},
	}
}

func divisionOverlay() []Rule {
	rules := []Rule{
		{
			ID: "div_no_team_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyTeamRefs,
				"message":             "division must not store team_refs; occupancy is child-owned via team.division_ref",
			},
		},
		{
			ID: "div_no_child_division_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyChildDivisionRefs,
				"message":             "division must not store child_division_refs; tree occupancy is child-owned via parent_division_ref",
			},
		},
	}
	return append(rules, selfDAGRules(objects.FieldKeyParentDivisionRef)...)
}

func componentOverlay() []Rule {
	rules := []Rule{
		{
			ID: "comp_no_child_component_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyChildComponentRefs,
				"message":             "component must not store child_component_refs; tree occupancy is child-owned via parent_component_refs",
			},
		},
	}
	return append(rules, selfDAGRules(objects.FieldKeyParentComponentRefs)...)
}

func organizationalChangeOverlay() []Rule {
	return []Rule{
		{
			ID: "orgchg_no_impact_analysis_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyImpactAnalysisRefs,
				"message":             "organizational_change must not store impact_analysis_refs; occupancy is child-owned via impact_analysis.change_ref",
			},
		},
	}
}

func missionOverlay() []Rule {
	return []Rule{
		{
			// Occupancy is child-owned. TRACK: BLI-KERNEL-REF-GRAPH-ACYCLIC-001
			ID: "mission_no_persona_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyPersonaRefs,
				"message":             "mission must not store persona_refs; occupancy is child-owned via persona.mission_refs",
			},
		},
	}
}

func organizationOverlay() []Rule {
	return []Rule{
		{
			ID: "org_no_partnership_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyPartnershipRefs,
				"message":             "organization must not store partnership_refs; membership of a partnership is parent-owned via partnership.organization_refs",
			},
		},
	}
}

func criteriaOverlay() []Rule {
	return []Rule{
		{
			ID: "require_description",
			Op: OpRequireField,
			Config: map[string]any{
				objects.FieldKeyField: "description",
				"message":             "description must be populated (CAS completeness barrier)",
				"skip_preliminary":    true,
				"skip_terminal":       true,
			},
		},
		{
			// Hash CAS must not persist criteria without category. Create parks
			// incomplete objects on the draft plane; this overlay refuses promote
			// / update materialize. TRACK: BLI-KERNEL-CRIT-CATEGORY-MINT-001
			ID: "crit_require_category",
			Op: OpRequireField,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyCategory,
				"message":             "criteria requires category (functional|non-functional|acceptance|test|performance|security|compliance)",
			},
		},
		{
			// Parent-owned composition. TRACK: BLI-KERNEL-REF-GRAPH-ACYCLIC-001
			ID: "crit_no_requirement_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyRequirementRefs,
				"message":             "criteria must not store requirement_refs; composition is parent-owned via requirement.criteria_refs",
			},
		},
		{
			ID: "crit_no_milestone_refs",
			Op: OpRefuseFieldPresent,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyMilestoneRefs,
				"message":             "criteria must not store milestone_refs; composition is parent-owned via milestone.criteria_refs",
			},
		},
		{
			ID: "crit_no_req_related_refs",
			Op: OpRefuseRefPrefix,
			Config: map[string]any{
				objects.FieldKeyField: objects.FieldKeyRelatedObjectRefs,
				"prefix":              "REQ-",
				"message":             "criteria.related_object_refs must not contain requirements; composition is parent-owned via requirement.criteria_refs",
			},
		},
	}
}

// occupiableTransitionOverlay requires claimed_by when transitioning into in_progress on occupiable kinds.
// TRACK: TDE-CEF-IN-PROGRESS-REQUIRES-CLAIM-001
func occupiableTransitionOverlay() []Rule {
	return []Rule{
		{
			ID: "occupiable_transition_claimed_by",
			Op: OpRequireFieldWhenStatus,
			Config: map[string]any{
				objects.FieldKeyField:    objects.FieldKeyClaimedBy,
				objects.FieldKeyStatuses: []any{objects.ObjectStatusInProgress},
				"message":                "claimed_by must be set when transitioning into in_progress on occupiable objects (exclusive claim required)",
			},
		},
	}
}

func traitOverlayRules(objectKind, intent string) []Rule {
	var rules []Rule
	if intent != IntentTransition {
		return rules
	}

	loader := objects.NewSpecLoader("")
	spec, err := loader.LoadSpecWithInheritance(objectKind + ".yaml")
	if err != nil || spec == nil {
		return rules
	}
	lcLoader := objects.NewLifecycleLoader("")
	lc, err := lcLoader.LoadLifecycle(objectKind)
	if err != nil || lc == nil {
		return rules
	}

	hasTrait := func(t string) bool {
		for _, rt := range spec.ResolvedTraits {
			if rt == t {
				return true
			}
		}
		return false
	}

	var effortStatuses []any
	var priorityStatuses []any

	for _, st := range lc.Statuses {
		role := st.Role
		if role == "" {
			switch st.Value {
			case objects.ObjectStatusInProgress, "testing":
				role = "execution_locked"
			case objects.ObjectStatusPlanned:
				role = "shovel_ready"
			}
		}
		if role == "execution_locked" || st.WorkDone {
			effortStatuses = append(effortStatuses, st.Value)
		}
		if role == "shovel_ready" || role == "execution_locked" {
			priorityStatuses = append(priorityStatuses, st.Value)
		}
	}

	if hasTrait("effort_aware") && len(effortStatuses) > 0 {
		rules = append(rules, Rule{
			ID: "trait_transition_estimated_effort",
			Op: OpRequireFieldWhenStatus,
			Config: map[string]any{
				objects.FieldKeyField:    objects.FieldKeyEstimatedEffort,
				objects.FieldKeyStatuses: effortStatuses,
				"message":                "estimated_effort must be set when effort_aware object is in_progress or complete",
			},
		})
	}

	if hasTrait("priority_aware") && len(priorityStatuses) > 0 {
		rules = append(rules, Rule{
			ID: "trait_transition_priority_tier",
			Op: OpRequireFieldWhenStatus,
			Config: map[string]any{
				objects.FieldKeyField:    objects.FieldKeyPriorityTier,
				objects.FieldKeyStatuses: priorityStatuses,
				"message":                "priority_tier must be set when priority_aware object is planned or in_progress",
			},
		})
	}

	return rules
}
