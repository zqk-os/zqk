package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// ConvergenceSessionBuilder builds the convergence_session spec at version v2_0_0
// File: bldr_v2/convergence_session_builder.go - version is encoded in package/directory name
type ConvergenceSessionBuilder struct {
	*builders.BaseSpecBuilder
}

// NewConvergenceSessionBuilder creates a new builder for convergence_session spec version v2_0_0
func NewConvergenceSessionBuilder() *ConvergenceSessionBuilder {
	builder := &ConvergenceSessionBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("convergence_session", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("work_interval").
		SetDescription("Persistent record of a convergence lifecycle run: hypothesis, triggers, desired end state, phased progress (C1–C6),\\npredictions and adjustable thresholds, chronological activity, before/after snapshots, delta and outcome character,\\nand next action. Supports handoffs across sessions and automation hooks that react to phase or measurement updates.\\nComplements glossary term GLS (convergence lifecycle concept); this kind is the machine-usable data structure.\\nRelated glossary terms (operational semantics): evaluation surface, observable artifact, remedy observation lifecycle,\\ndeviation response — use them to name what is measured, how signal state evolves, and how to react when observations\\ndiverge from expectations, across use cases beyond test-bundle health.jsonl.\\nLifecycle: convergence_session_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("completable")

	// Add fields
	builder.addConvergenceSessionFields()

	return builder
}

// addConvergenceSessionFields adds the convergence_session fields
func (b *ConvergenceSessionBuilder) addConvergenceSessionFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("activity_log", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation.").
			AutomationHooks("append on each act/verify step; drives chronology and audit.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("scheduler, agents, CI.").
			Lifecycle("append-mostly (mutable).").
			Observability("yes").
			Purpose("Ordered list of activity entries. Each entry is a map with at least: timestamp (RFC3339), phase (string),\naction (string), optional before_snapshot/after_snapshot (objects), delta_assessment, outcome_character,\nnotes. Enables replay and cross-session continuity.\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"convergence_tracking",
				"automation",
				"reporting",
			}).
			Validation("list of objects; shape enforced by convention and tooling.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("CVS-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("after_state_snapshot", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation.").
			AutomationHooks("compared to before_state_snapshot for delta_assessment.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("health.jsonl, metrics.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Latest measured state (e.g. failing fingerprints, health_watermark, test counts). Flexible object map.\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"measurement",
				"delta",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("automation_hooks", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("optional labels or channel names for event subscribers.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("scheduler, notifications.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional hints for tooling (e.g. event names, job categories) when this session updates.").
			Security("non-sensitive").
			SystemUsage([]any{
				"automation",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("before_state_snapshot", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation.").
			AutomationHooks("baseline for delta_assessment.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("health.jsonl.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Baseline state at session start or iteration boundary (fingerprints, metrics, watermark).").
			Security("non-sensitive").
			SystemUsage([]any{
				"measurement",
				"delta",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("current_phase", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation.").
			AutomationHooks("phase transitions may trigger automations.").
			Cardinality("one").
			Criticality("composition").
			Default("c1_scope").
			Dependencies("glossary convergence lifecycle phases.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Current convergence phase (aligned with glossary C1–C6).").
			Security("non-sensitive").
			SystemUsage([]any{
				"routing",
				"UI",
				"automation",
			}).
			Validation("enum aligned with glossary_term convergence phases.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"c1_scope",
				"c2_triage",
				"c3_order",
				"c4_act",
				"c5_verify",
				"c6_exit",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "readable", "writable", "modifiable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("debrief_notes", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation.").
			AutomationHooks("optional copy into next session iteration_process or backlog.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Post-close operator analysis: what worked, what to avoid, scheduler/queue tips, and how to run the\nnext convergence_session more efficiently. Filled at finalize; future sessions may reference this\nobject or copy excerpts into iteration_process / next_action on a new CVS.\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"handoff",
				"retrospectives",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(32000).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-021"))
	b.AddFieldBuilder(builders.NewFieldBuilder("delta_assessment", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation.").
			AutomationHooks("may drive alerts when trending_away.").
			Cardinality("one").
			Criticality("association").
			Default("unknown").
			Dependencies("before/after snapshots.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Whether the latest measurement moves toward the desired end state.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"automation",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"trending_toward",
				"trending_away",
				"neutral",
				"unknown",
			}).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "listable", "readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("desired_end_state", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used to judge completion and delta.").
			Cardinality("one").
			Criticality("composition").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Target outcome in plain language (e.g. all scoped fingerprints pass in health).").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("flow_variant", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("selects branch-specific steps or templates.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("optional workflow or policy objects.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional named branch flow (e.g. storage-heavy, scheduler-only) for specificity beyond the base C1–C6 path.").
			Security("non-sensitive").
			SystemUsage([]any{
				"routing",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "groupable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("glossary_term_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("link to operational definition of convergence lifecycle.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("").
			Dependencies("glossary_term objects.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to glossary term (typically GLS-* for convergence lifecycle).").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^$|^GLS-[0-9]+-[a-f0-9]+$`).
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("CVS-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("hypothesis", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("may seed prompts and success checks.").
			Cardinality("one").
			Criticality("composition").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Preliminary claim to validate (what we believe is wrong or what fix will work).").
			Security("non-sensitive").
			SystemUsage([]any{
				"reasoning",
				"reporting",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("iteration_process", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("optional template for repeated steps.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Proposed iteration or loop body (human-readable); may reference PRE_CHANGE_CHECKLIST or bundle commands.").
			Security("non-sensitive").
			SystemUsage([]any{
				"procedures",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("last_measurement_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("staleness checks.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("health.jsonl, jobs.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("ISO-8601 timestamp of the last measurement or verify step.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-014"))
	b.AddFieldBuilder(builders.NewFieldBuilder("next_action", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("handoff and playbooks.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("One concrete next step for the current resource (or handoff).").
			Security("non-sensitive").
			SystemUsage([]any{
				"continuity",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("outcome_character", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation.").
			AutomationHooks("surprising/catastrophic may escalate notifications.").
			Cardinality("one").
			Criticality("association").
			Default("pending").
			Dependencies("measurement vs prediction.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Whether the latest result matched expectations, was surprising, or catastrophic relative to hypothesis.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"risk",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"pending",
				"expected",
				"surprising",
				"catastrophic",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-015"))
	b.AddFieldBuilder(builders.NewFieldBuilder("predictions", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("compare to actuals for outcome_character.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Structured predictions before iterations: e.g. expected_duration, time_estimate, expected_signal,\nhypothesis_confidence. Flexible object map; tooling may standardize keys.\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"retrospectives",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-016"))
	b.AddFieldBuilder(builders.NewFieldBuilder("requirement_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("traceability to requirements.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("requirements registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Requirements this session advances or validates.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("CVS-017"))
	b.AddFieldBuilder(builders.NewFieldBuilder("start_condition", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("may trigger session creation from health predicates.").
			Cardinality("one").
			Criticality("composition").
			Default("").
			Dependencies("health.jsonl, triggers.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Starting trigger or acceptance predicate (what makes this session applicable).").
			Security("non-sensitive").
			SystemUsage([]any{
				"automation",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-018"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation.").
			AutomationHooks("lifecycle transitions; may fire events.").
			Cardinality("one").
			Criticality("composition").
			Default("draft").
			Dependencies("convergence_session_lifecycle.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Session lifecycle status.").
			Security("non-sensitive").
			SystemUsage([]any{
				"workflows",
				"reporting",
			}).
			Validation("enum from lifecycle.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"draft",
				"active",
				"paused",
				"completed",
				"abandoned",
				"archived",
				"escalated",
				"error",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-019"))
	b.AddFieldBuilder(builders.NewFieldBuilder("thresholds", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("gating reruns, stop rules, diminishing returns.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("policy, metrics.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Adjustable thresholds (max iterations, time budget, flake tolerance, min pass rate). Flexible object map.\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"automation",
				"steering",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CVS-020"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ConvergenceSessionBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ConvergenceSessionBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ConvergenceSessionBuilder) GetOntology() string {
	return "convergence_session"
}

func init() {
	builders.RegisterBuilder(NewConvergenceSessionBuilder())
}
