package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// JobListingBuilder builds the job_listing spec at version v2_0_0
// File: bldr_v2/job_listing_builder.go - version is encoded in package/directory name
type JobListingBuilder struct {
	*builders.BaseSpecBuilder
}

// NewJobListingBuilder creates a new builder for job_listing spec version v2_0_0
func NewJobListingBuilder() *JobListingBuilder {
	builder := &JobListingBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("job_listing", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents an individual job listing discovered through automated search or manual curation.\\nUsed to track the state of a job opportunity from discovery through application.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable")

	// Add fields
	builder.addJobListingFields()

	return builder
}

// addJobListingFields adds the job_listing fields
func (b *JobListingBuilder) addJobListingFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("company_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Purpose("The name of the company offering the job.").
			Build()).
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("JOB-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("curation_status", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Purpose("Status of the job listing (e.g., discovered, reviewed, interested, applied, rejected).").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("JOB-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Purpose("The full description of the job.").
			Build()).
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("JOB-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("job_title", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Purpose("The title of the job listing.").
			Build()).
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("JOB-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("job_url", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Purpose("The URL to the original job posting.").
			Build()).
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("JOB-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("match_score", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Purpose("Automated match score against user profile.").
			Build()).
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("JOB-006"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *JobListingBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *JobListingBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *JobListingBuilder) GetOntology() string {
	return "job_listing"
}

func init() {
	builders.RegisterBuilder(NewJobListingBuilder())
}
