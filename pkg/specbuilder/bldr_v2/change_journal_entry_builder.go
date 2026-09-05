package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// ChangeJournalEntryBuilder builds the change_journal_entry spec at version v2_0_0
// File: bldr_v2/change_journal_entry_builder.go - version is encoded in package/directory name
type ChangeJournalEntryBuilder struct {
	*builders.BaseSpecBuilder
}

// NewChangeJournalEntryBuilder creates a new builder for change_journal_entry spec version v2_0_0
func NewChangeJournalEntryBuilder() *ChangeJournalEntryBuilder {
	builder := &ChangeJournalEntryBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("change_journal_entry", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Internal object: logged change (create/update/delete/import/health_check) for auditing, rollbacks, and time-series (e.g. health monitor results).\\nLifecycle: change_journal_entry_lifecycle.yaml (pending → completed | failed; completed → aggregated; then archived). See docs/process/_internal/configs/change_journal_entry_config.md.\\nCollection: append on object change (create/update/delete/import) and on health_check (health_monitor:<id>). No samplers/gauges; append-only event log.\\nAggregation: change_journal_aggregation job; time window → audit_aggregation_metric; entries marked aggregated; compaction (micro-GC) .cjournal.\\nBucketing: chronological monthly (high-volume); retention in retention_tolerance.yaml; bucketing_strategy override supported.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addChangeJournalEntryFields()

	return builder
}

// addChangeJournalEntryFields adds the change_journal_entry fields
func (b *ChangeJournalEntryBuilder) addChangeJournalEntryFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("change_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("determines rollback behavior.").
			Cardinality("one").
			Criticality("composition").
			Default("update").
			Dependencies("rollback logic.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Nature of the change (create, update, delete, import, health_check).").
			Security("non-sensitive").
			SystemUsage([]any{
				"audits",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"create",
				"update",
				"delete",
				"import",
				"health_check",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("CJE-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("changed_paths", "array").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for analytics and compaction (dictionary compression).").
			Cardinality("zero_or_many").
			Criticality("association").
			Default(nil).
			Dependencies("flattenUpdatePaths from updates map.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Flattened dotted paths of changed fields (e.g. meta.tags, status) for analysis and compaction.").
			Security("non-sensitive").
			SystemUsage([]any{
				"audits",
				"compaction",
			}).
			Validation("Array of path strings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("CJE-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("diff_summary", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used in audit reports.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("diff tooling.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Summary of what changed (fields, old/new).").
			Security("non-sensitive").
			SystemUsage([]any{
				"audits",
			}).
			Validation("Free-form text describing changes.").
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
		WithProfileCode("CJE-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("object_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used to trace changes.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("rollback tooling.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Reference to the object that changed.").
			Security("non-sensitive").
			SystemUsage([]any{
				"audits",
				"rollback",
			}).
			Validation("URI/reference of the object (e.g., \\\\\\\"goal:{id}\\\\\\\").").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("CJE-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("previous_state", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for rollback operations.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("rollback tooling.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Snapshot of object state before change (for rollback).").
			Security("non-sensitive").
			SystemUsage([]any{
				"rollback",
			}).
			Validation("Object snapshot.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("CJE-004"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ChangeJournalEntryBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ChangeJournalEntryBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ChangeJournalEntryBuilder) GetOntology() string {
	return "change_journal_entry"
}

func init() {
	builders.RegisterBuilder(NewChangeJournalEntryBuilder())
}
