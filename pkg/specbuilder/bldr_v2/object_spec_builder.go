package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// ObjectSpecBuilder builds the object_spec spec at version v2_0_0
// File: bldr_v2/object_spec_builder.go - version is encoded in package/directory name
type ObjectSpecBuilder struct {
	*builders.BaseSpecBuilder
}

// NewObjectSpecBuilder creates a new builder for object_spec spec version v2_0_0
func NewObjectSpecBuilder() *ObjectSpecBuilder {
	builder := &ObjectSpecBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("object_spec", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Meta-kind for YAML definition files under .zqk/specs/objects.\\nList/get surface each spec file as a row (id = file stem). Specialized fields describe\\nthe spec file and the ontology it defines.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addObjectSpecFields()

	return builder
}

// addObjectSpecFields adds the object_spec fields
func (b *ObjectSpecBuilder) addObjectSpecFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("completeness_validation", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("acts as the master template for validation steps when a new object of this kind is instantiated.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("object generation pipeline.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("The canonical Validation DSL steps that must pass for any object of this kind to be considered fully complete.").
			Security("non-sensitive").
			SystemUsage([]any{
				"instantiation_templates",
				"baseline_enforcement",
			}).
			Validation("List of Validation DSL steps.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "filterable").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("OSPEC-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("file_path", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used when listing specs from disk").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("object_specs directory layout").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Absolute or project-relative path to the YAML spec file when materialized as an object row.").
			Security("non-sensitive").
			SystemUsage([]any{
				"listing",
				"navigation",
			}).
			Validation("Path string when present").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "filterable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("OSPEC-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("ontology", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("ties list rows to the kind defined by the file").
			Cardinality("one").
			Criticality("composition").
			Default("object_spec").
			Dependencies("spec YAML ontology field").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("The object kind (ontology) defined by this spec file (e.g. backlog_item, lifecycle).").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"validation",
			}).
			Validation("Lowercase identifier").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[a-z][a-z0-9_]*$`).
			Required(false).
			Build()).
		WithTraits("filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("OSPEC-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("source_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("list grouping for built-in vs internal vs public specs").
			Cardinality("one").
			Criticality("association").
			Default("internal").
			Dependencies("path and visibility in spec YAML").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Provenance bucket (built-in, internal, public) when listing from disk.").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
			}).
			Validation("enum").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"built-in",
				"internal",
				"public",
			}).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("OSPEC-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ObjectSpecBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ObjectSpecBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ObjectSpecBuilder) GetOntology() string {
	return "object_spec"
}

func init() {
	builders.RegisterBuilder(NewObjectSpecBuilder())
}
