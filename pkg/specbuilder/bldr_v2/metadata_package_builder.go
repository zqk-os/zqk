package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// MetadataPackageBuilder builds the metadata_package spec at version v2_0_0
// File: bldr_v2/metadata_package_builder.go - version is encoded in package/directory name
type MetadataPackageBuilder struct {
	*builders.BaseSpecBuilder
}

// NewMetadataPackageBuilder creates a new builder for metadata_package spec version v2_0_0
func NewMetadataPackageBuilder() *MetadataPackageBuilder {
	builder := &MetadataPackageBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("metadata_package", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Comprehensive metrics collection package for workstreams, milestones, or the entire project. Provides observability into codebase health, technical debt, team productivity, and project velocity. Automatically collected from various sources (source control, CI/CD, test runners, static analysis). ").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addMetadataPackageFields()

	return builder
}

// addMetadataPackageFields adds the metadata_package fields
func (b *MetadataPackageBuilder) addMetadataPackageFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("collected_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/collector.").
			AutomationHooks("used for time-series analysis.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("collector.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ISO-8601 timestamp when this metadata package was collected.").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"trending",
				"reporting",
			}).
			Validation("ISO-8601 datetime.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).
			Required(true).
			Build()).
		WithTraits("field_read_only_group").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("MDP-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metrics", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/collector.").
			AutomationHooks("used for metrics reporting and analysis.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("collector implementation.").
			Lifecycle("mutable (updated on collection).").
			Observability("yes").
			Purpose("Collected metrics data (codebase health, test coverage, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"analytics",
			}).
			Validation("Structured object with metric keys and values.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("MDP-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("scope", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/collector.").
			AutomationHooks("determines which collector methods to use.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("collector logic.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("The scope of this metadata package (workstream, milestone, or project).").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"aggregation",
				"reporting",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"workstream",
				"milestone",
				"project",
			}).
			Required(true).
			Build()).
		WithTraits("groupable", "listable", "modifiable", "readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("MDP-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("scope_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/collector.").
			AutomationHooks("links metadata to workstream/milestone.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("workstream/milestone existence.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ID of the workstream or milestone this package is for (empty for project scope).").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"linking",
				"reporting",
			}).
			Validation("Must reference an existing workstream or milestone when scope is \\\\\\\"workstream\\\\\\\" or \\\\\\\"milestone\\\\\\\".").
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
		WithProfileCode("MDP-002"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *MetadataPackageBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *MetadataPackageBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *MetadataPackageBuilder) GetOntology() string {
	return "metadata_package"
}

func init() {
	builders.RegisterBuilder(NewMetadataPackageBuilder())
}
