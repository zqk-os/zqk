package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// ApplicationRecordBuilder builds the application_record spec at version v2_0_0
// File: bldr_v2/application_record_builder.go - version is encoded in package/directory name
type ApplicationRecordBuilder struct {
	*builders.BaseSpecBuilder
}

// NewApplicationRecordBuilder creates a new builder for application_record spec version v2_0_0
func NewApplicationRecordBuilder() *ApplicationRecordBuilder {
	builder := &ApplicationRecordBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("application_record", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("A record of an application generated or submitted for a specific job listing.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable")

	// Add fields
	builder.addApplicationRecordFields()

	return builder
}

// addApplicationRecordFields adds the application_record fields
func (b *ApplicationRecordBuilder) addApplicationRecordFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("cover_letter_content", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Purpose("The generated cover letter.").
			Build()).
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("APP-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("job_listing_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Purpose("The ID of the target job listing.").
			Build()).
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("APP-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ApplicationRecordBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ApplicationRecordBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ApplicationRecordBuilder) GetOntology() string {
	return "application_record"
}

func init() {
	builders.RegisterBuilder(NewApplicationRecordBuilder())
}
