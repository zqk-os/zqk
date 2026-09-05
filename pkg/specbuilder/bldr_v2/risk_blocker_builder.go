package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// RiskBlockerBuilder builds the risk_blocker spec at version v2_0_0
// File: bldr_v2/risk_blocker_builder.go - version is encoded in package/directory name
type RiskBlockerBuilder struct {
	*builders.BaseSpecBuilder
}

// NewRiskBlockerBuilder creates a new builder for risk_blocker spec version v2_0_0
func NewRiskBlockerBuilder() *RiskBlockerBuilder {
	builder := &RiskBlockerBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("risk_blocker", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("First-class risk and blocker tracking object. Distinguishes between risks (potential future issues) and blockers (current impediments), enabling impact analysis, mitigation planning, and resolution tracking. ").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addRiskBlockerFields()

	return builder
}

// addRiskBlockerFields adds the risk_blocker fields
func (b *RiskBlockerBuilder) addRiskBlockerFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("affected_items", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("updates affected item status, triggers notifications.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("work item status tracking.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Work items (milestones, goals, backlog items) affected by this risk/blocker.").
			Security("non-sensitive").
			SystemUsage([]any{
				"impact analysis",
				"traceability",
				"reporting",
			}).
			Validation("Must reference existing work items.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("RB-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("backlog_item_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for backlog item impact tracking.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("backlog registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Backlog items affected by this risk/blocker.").
			Security("non-sensitive").
			SystemUsage([]any{
				"impact analysis",
				"traceability",
			}).
			Validation("Must reference existing backlog item IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("RB-015"))
	b.AddFieldBuilder(builders.NewFieldBuilder("detected_by", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/creator.").
			AutomationHooks("used in detection method analytics.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("manual").
			Dependencies("detection analytics.").
			Lifecycle("immutable (set at creation).").
			Observability("yes").
			Purpose("How this risk/blocker was detected (system, manual, D&B detection, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"analytics",
				"process improvement",
			}).
			Validation("Free text or system identifier.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("RB-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for goal impact tracking.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals affected by this risk/blocker.").
			Security("non-sensitive").
			SystemUsage([]any{
				"impact analysis",
				"traceability",
			}).
			Validation("Must reference existing goal IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("RB-014"))
	b.AddFieldBuilder(builders.NewFieldBuilder("impact", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used in impact analysis, reporting.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("impact analysis reports.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Description of the impact if this risk materializes or blocker persists.").
			Security("may contain sensitive information.").
			SystemUsage([]any{
				"impact analysis",
				"reporting",
			}).
			Validation("markdown allowed.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RB-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("milestone_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for milestone impact tracking.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("milestone registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestones affected by this risk/blocker.").
			Security("non-sensitive").
			SystemUsage([]any{
				"impact analysis",
				"traceability",
			}).
			Validation("Must reference existing milestone IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("RB-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("mitigation_plan", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used in resolution tracking, reporting.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("resolution tracking.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Plan to mitigate the risk or resolve the blocker.").
			Security("may contain sensitive information.").
			SystemUsage([]any{
				"planning",
				"tracking",
			}).
			Validation("markdown allowed.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RB-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("probability", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/risk_manager.").
			AutomationHooks("used in risk score calculation.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null (not applicable to blockers)").
			Dependencies("risk scoring.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Probability of risk materializing (for risks only). Not applicable to blockers (they're already materialized).").
			Security("non-sensitive").
			SystemUsage([]any{
				"risk scoring",
				"prioritization",
			}).
			Validation("enum. Only applicable when risk_type is risk.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"high",
				"medium",
				"low",
			}).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RB-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("related_risks", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used in risk dependency analysis.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("risk_blocker registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Related risk/blocker objects (e.g., cascading risks, related blockers).").
			Security("non-sensitive").
			SystemUsage([]any{
				"risk analysis",
				"dependency graphs",
			}).
			Validation("Must reference existing risk_blocker objects.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("RB-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("resolution_status", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("triggers status updates, affects affected items.").
			Cardinality("one").
			Criticality("composition").
			Default("open").
			Dependencies("status transitions, reporting.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Current resolution status (open, in_progress, resolved, mitigated, accepted).").
			Security("non-sensitive").
			SystemUsage([]any{
				"status tracking",
				"reporting",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"open",
				"in_progress",
				"resolved",
				"mitigated",
				"accepted",
			}).
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "modifiable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RB-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("resolved_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner.").
			AutomationHooks("used in resolution time analytics.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("resolution_status.").
			Lifecycle("mutable (set when resolution_status changes to resolved/mitigated).").
			Observability("yes").
			Purpose("When the risk was mitigated or blocker was resolved.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"analytics",
			}).
			Validation("ISO-8601 datetime.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}:\d{2}Z)?$`).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RB-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("resolved_by", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner.").
			AutomationHooks("used in accountability reporting.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("resolution_status.").
			Lifecycle("mutable (set when resolution_status changes to resolved/mitigated).").
			Observability("yes").
			Purpose("Actor who resolved the risk/blocker.").
			Security("non-sensitive").
			SystemUsage([]any{
				"accountability",
				"reporting",
			}).
			Validation("Must reference existing account or system identifier.").
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
		WithProfileCode("RB-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("risk_score", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for automatic prioritization, sorting.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("severity, probability.").
			Lifecycle("mutable (recalculated when severity or probability changes).").
			Observability("yes").
			Purpose("Calculated risk score (0-100) based on severity and probability. Higher score = higher priority.").
			Security("non-sensitive").
			SystemUsage([]any{
				"prioritization",
				"sorting",
				"filtering",
			}).
			Validation("Number between 0 and 100.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RB-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("risk_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("triggers different workflows for risks vs blockers.").
			Cardinality("one").
			Criticality("composition").
			Default("risk").
			Dependencies("status transitions, reporting.").
			Lifecycle("mutable (can convert risk to blocker when it materializes).").
			Observability("yes").
			Purpose("Distinguish between risk (potential future issue) and blocker (current impediment).").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"reporting",
				"workflow",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"risk",
				"blocker",
			}).
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "modifiable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RB-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("severity", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/risk_manager.").
			AutomationHooks("triggers alerts, affects priority calculations.").
			Cardinality("one").
			Criticality("composition").
			Default("medium").
			Dependencies("prioritization, alerts.").
			Lifecycle("mutable (can escalate or de-escalate).").
			Observability("yes").
			Purpose("Severity level (critical, high, medium, low) for prioritization and impact analysis.").
			Security("non-sensitive").
			SystemUsage([]any{
				"prioritization",
				"reporting",
				"filtering",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"critical",
				"high",
				"medium",
				"low",
			}).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RB-002"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *RiskBlockerBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *RiskBlockerBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *RiskBlockerBuilder) GetOntology() string {
	return "risk_blocker"
}

func init() {
	builders.RegisterBuilder(NewRiskBlockerBuilder())
}
