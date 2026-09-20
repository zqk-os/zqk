package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// ResolverBuilder builds the resolver spec at version v2_0_0
// File: bldr_v2/resolver_builder.go - version is encoded in package/directory name
type ResolverBuilder struct {
	*builders.BaseSpecBuilder
}

// NewResolverBuilder creates a new builder for resolver spec version v2_0_0
func NewResolverBuilder() *ResolverBuilder {
	builder := &ResolverBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("resolver", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines how reference schemes (e.g., account:{id}, test://suite) are resolved to concrete locations/commands. Handles user-provided hints plus internal fallback logic. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addResolverFields()

	return builder
}

// addResolverFields adds the resolver fields
func (b *ResolverBuilder) addResolverFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("reference_format", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("ensures references parse correctly.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("validators.").
			Lifecycle("mutable with version bumps.").
			Observability("yes").
			Purpose("Pattern/URI describing valid references (e.g., \\\\\\\"account:{id}\\\\\\\" or \\\\\\\"test://suite/name\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation",
			}).
			Validation("regex or URI template.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RSV-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("scheme", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin/automation.").
			AutomationHooks("determines resolver selection.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("reference fields using \"scheme:{id}\" format.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Identifier for the reference scheme handled (e.g., \\\\\\\"account\\\\\\\", \\\\\\\"workstream\\\\\\\", \\\\\\\"test\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"reference resolution",
			}).
			Validation("Username pattern.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[a-z0-9_-]+$`).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("RSV-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("user_hint", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("user/admin.").
			AutomationHooks("used by resolver when primary resolution fails.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("resolver implementation.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional user-defined hint/config (path template, host, etc.).").
			Security("may contain sensitive paths.").
			SystemUsage([]any{
				"custom resolution",
			}).
			Validation("Free-form text.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("RSV-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ResolverBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ResolverBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ResolverBuilder) GetOntology() string {
	return "resolver"
}

func init() {
	builders.RegisterBuilder(NewResolverBuilder())
}
