package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// JobSearchProfileBuilder builds the job_search_profile spec at version v2_0_0
// File: bldr_v2/job_search_profile_builder.go - version is encoded in package/directory name
type JobSearchProfileBuilder struct {
	*builders.BaseSpecBuilder
}

// NewJobSearchProfileBuilder creates a new builder for job_search_profile spec version v2_0_0
func NewJobSearchProfileBuilder() *JobSearchProfileBuilder {
	builder := &JobSearchProfileBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("job_search_profile", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Profile defining the criteria for automated job discovery, including keywords, locations, and thresholds.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable")

	// Add fields
	builder.addJobSearchProfileFields()

	return builder
}

// addJobSearchProfileFields adds the job_search_profile fields
func (b *JobSearchProfileBuilder) addJobSearchProfileFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("enabled", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Purpose("Whether this search profile is actively running.").
			Build()).
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("JSP-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("excluded_companies", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Purpose("Companies to exclude from search results.").
			Build()).
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("JSP-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("keywords", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Purpose("Keywords to search for in job titles or descriptions.").
			Build()).
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("JSP-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("locations", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Purpose("Target locations for the job search (e.g., 'Remote', 'San Francisco, CA').").
			Build()).
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("JSP-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("match_threshold", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Purpose("Minimum match score (0-100) required to automatically save a job listing.").
			Build()).
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("JSP-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("minimum_salary", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Purpose("Minimum acceptable salary.").
			Build()).
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("JSP-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *JobSearchProfileBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *JobSearchProfileBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *JobSearchProfileBuilder) GetOntology() string {
	return "job_search_profile"
}

func init() {
	builders.RegisterBuilder(NewJobSearchProfileBuilder())
}
