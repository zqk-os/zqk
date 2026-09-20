package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// VitalityReportBuilder builds the vitality_report spec at version v2_0_0
// File: bldr_v2/vitality_report_builder.go - version is encoded in package/directory name
type VitalityReportBuilder struct {
	*builders.BaseSpecBuilder
}

// NewVitalityReportBuilder creates a new builder for vitality_report spec version v2_0_0
func NewVitalityReportBuilder() *VitalityReportBuilder {
	builder := &VitalityReportBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("vitality_report", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents the real-time operational health and project confidence score (PCS) \\nof the Knowledge Kernel. Calculated by Heart nodes.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addVitalityReportFields()

	return builder
}

// addVitalityReportFields adds the vitality_report fields
func (b *VitalityReportBuilder) addVitalityReportFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("drift_frequency", "float").
		WithTraits("readable", "writable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("last_calculation_at", "timestamp").
		WithTraits("readable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("project_confidence_score", "integer").
		WithTraits("readable", "writable", "groupable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("success_rate", "float").
		WithTraits("readable", "writable"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *VitalityReportBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *VitalityReportBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *VitalityReportBuilder) GetOntology() string {
	return "vitality_report"
}

func init() {
	builders.RegisterBuilder(NewVitalityReportBuilder())
}
