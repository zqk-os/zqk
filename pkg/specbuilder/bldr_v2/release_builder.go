package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// ReleaseBuilder builds the release spec at version v2_0_0
// File: bldr_v2/release_builder.go - version is encoded in package/directory name
type ReleaseBuilder struct {
	*builders.BaseSpecBuilder
}

// NewReleaseBuilder creates a new builder for release spec version v2_0_0
func NewReleaseBuilder() *ReleaseBuilder {
	builder := &ReleaseBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("release", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents a software release with version, release criteria, and release notes. Enables structured release planning, tracking, and documentation. ").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addReleaseFields()

	return builder
}

// addReleaseFields adds the release fields
func (b *ReleaseBuilder) addReleaseFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("backlog_item_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/release_manager.").
			AutomationHooks("used for release content tracking.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("backlog registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Backlog items included in this release.").
			Security("non-sensitive").
			SystemUsage([]any{
				"release planning",
				"traceability",
			}).
			Validation("must reference existing backlog item IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("REL-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("criteria_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/release_manager.").
			AutomationHooks("used for release gate validation.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("criteria registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Release criteria that must be met for this release.").
			Security("non-sensitive").
			SystemUsage([]any{
				"release validation",
				"gating",
			}).
			Validation("must reference existing criteria IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("REL-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("milestone_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/release_manager.").
			AutomationHooks("used for release content tracking.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("milestone registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestones included in this release.").
			Security("non-sensitive").
			SystemUsage([]any{
				"release planning",
				"traceability",
			}).
			Validation("must reference existing milestone IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("REL-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("release_date", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/release_manager.").
			AutomationHooks("used for release scheduling, release notes generation.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("release scheduling, release notes.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("ISO-8601 date when this release is scheduled or was released.").
			Security("non-sensitive").
			SystemUsage([]any{
				"release planning",
				"release notes",
				"scheduling",
			}).
			Validation("ISO-8601 date format (YYYY-MM-DD).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("REL-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("release_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/release_manager.").
			AutomationHooks("used for version number calculation and release notes categorization.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("versioning strategy.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Type of release (e.g., \\\\\\\"major\\\\\\\", \\\\\\\"minor\\\\\\\", \\\\\\\"patch\\\\\\\", \\\\\\\"hotfix\\\\\\\", \\\\\\\"beta\\\\\\\", \\\\\\\"alpha\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"release planning",
				"versioning",
				"release notes",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"major",
				"minor",
				"patch",
				"hotfix",
				"beta",
				"alpha",
			}).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("REL-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("requirement_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/release_manager.").
			AutomationHooks("used for release content tracking.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("requirement registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Requirements fulfilled in this release.").
			Security("non-sensitive").
			SystemUsage([]any{
				"release planning",
				"traceability",
			}).
			Validation("must reference existing requirement IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("REL-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("version", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/release_manager.").
			AutomationHooks("used for release notes generation, version tagging.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("version tagging, release notes.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Version identifier for this release (e.g., \\\\\\\"v1.0.0\\\\\\\", \\\\\\\"2025.12.06\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"versioning",
				"release tracking",
				"release notes",
			}).
			Validation("Semantic versioning format recommended (e.g., \\\\\\\"v1.0.0\\\\\\\") or date-based (e.g., \\\\\\\"2025.12.06\\\\\\\").").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("REL-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ReleaseBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ReleaseBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ReleaseBuilder) GetOntology() string {
	return "release"
}

func init() {
	builders.RegisterBuilder(NewReleaseBuilder())
}
