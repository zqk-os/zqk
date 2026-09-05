package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// RollbackReportBuilder builds the rollback_report spec at version v2_0_0
// File: bldr_v2/rollback_report_builder.go - version is encoded in package/directory name
type RollbackReportBuilder struct {
	*builders.BaseSpecBuilder
}

// NewRollbackReportBuilder creates a new builder for rollback_report spec version v2_0_0
func NewRollbackReportBuilder() *RollbackReportBuilder {
	builder := &RollbackReportBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("rollback_report", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Summary of a rollback/reset operation describing which objects/docs were affected. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addRollbackReportFields()

	return builder
}

// addRollbackReportFields adds the rollback_report fields
func (b *RollbackReportBuilder) addRollbackReportFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("affected_objects", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used to prompt re-entry/manual review.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("change journals.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("References to objects/documents that were rolled back/removed.").
			Security("may reveal sensitive docs—treat accordingly.").
			SystemUsage([]any{
				"audits",
				"notifications",
			}).
			Validation("list of references or paths.").
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
		WithProfileCode("RBR-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("git_reference", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("links to git history.").
			Cardinality("one").
			Criticality("association").
			Default("required").
			Dependencies("rollback tooling.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Git hash/tag involved in the rollback.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("git ref format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("RBR-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("summary", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("used in rollback notifications.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Human-readable explanation of the rollback (why, what next).").
			Security("non-sensitive").
			SystemUsage([]any{
				"communication",
			}).
			Validation("markdown allowed.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RBR-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *RollbackReportBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *RollbackReportBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *RollbackReportBuilder) GetOntology() string {
	return "rollback_report"
}

func init() {
	builders.RegisterBuilder(NewRollbackReportBuilder())
}
