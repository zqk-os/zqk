package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// TemplateBuilder builds the template spec at version v2_0_0
// File: bldr_v2/template_builder.go - version is encoded in package/directory name
type TemplateBuilder struct {
	*builders.BaseSpecBuilder
}

// NewTemplateBuilder creates a new builder for template spec version v2_0_0
func NewTemplateBuilder() *TemplateBuilder {
	builder := &TemplateBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("template", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Prompt/template definitions that generate artifacts (docs, scripts, flows). ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addTemplateFields()

	return builder
}

// addTemplateFields adds the template fields
func (b *TemplateBuilder) addTemplateFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("category", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for template filtering.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("template registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Template category for organization.").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"branching",
				"commit",
				"structure",
				"bootstrap",
			}).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TPL-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("flow_order", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("determines pipeline sequencing.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("CLI flows.").
			Lifecycle("mutable (with approvals).").
			Observability("yes").
			Purpose("Position within initialization/generation pipeline.").
			Security("non-sensitive").
			SystemUsage([]any{
				"workflow orchestration",
			}).
			Validation("positive integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TPL-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("outputs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("instructs generator what to emit.").
			Cardinality("many (>=1)").
			Criticality("composition").
			Default("required").
			Dependencies("generators, doc index registration.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Files/artifacts produced by the template.").
			Security("non-sensitive").
			SystemUsage([]any{
				"generation",
				"doc index",
			}).
			Validation("paths or identifiers must be valid.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TPL-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("questions", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used to generate prompts.").
			Cardinality("many (>=1)").
			Criticality("composition").
			Default([]any{}).
			Dependencies("prompt engine.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Question set driving the template prompts.").
			Security("non-sensitive").
			SystemUsage([]any{
				"prompting",
				"AI workflows",
			}).
			Validation("list of question objects.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("TPL-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *TemplateBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *TemplateBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *TemplateBuilder) GetOntology() string {
	return "template"
}

func init() {
	builders.RegisterBuilder(NewTemplateBuilder())
}
