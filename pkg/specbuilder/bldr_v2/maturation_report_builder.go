package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// MaturationReportBuilder builds the maturation_report spec at version v2_0_0
// File: bldr_v2/maturation_report_builder.go - version is encoded in package/directory name
type MaturationReportBuilder struct {
	*builders.BaseSpecBuilder
}

// NewMaturationReportBuilder creates a new builder for maturation_report spec version v2_0_0
func NewMaturationReportBuilder() *MaturationReportBuilder {
	builder := &MaturationReportBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("maturation_report", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Persistent record of a component's fitness and maturation progress.\\nEvaluates whether shadow actions lead to better outcomes than mainline.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	// Add fields
	builder.addMaturationReportFields()

	return builder
}

// addMaturationReportFields adds the maturation_report fields
func (b *MaturationReportBuilder) addMaturationReportFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("component_id", "string").
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("reference"))
	b.AddFieldBuilder(builders.NewFieldBuilder("fitness_score", "float").
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("expression"))
	b.AddFieldBuilder(builders.NewFieldBuilder("graduation_status", "enum").
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"maturation",
				"ready",
				"promoted",
			}).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("observation_duration", "string").
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *MaturationReportBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *MaturationReportBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *MaturationReportBuilder) GetOntology() string {
	return "maturation_report"
}

func init() {
	builders.RegisterBuilder(NewMaturationReportBuilder())
}
