package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// SamplerProfileBuilder builds the sampler_profile spec at version v2_0_0
// File: bldr_v2/sampler_profile_builder.go - version is encoded in package/directory name
type SamplerProfileBuilder struct {
	*builders.BaseSpecBuilder
}

// NewSamplerProfileBuilder creates a new builder for sampler_profile spec version v2_0_0
func NewSamplerProfileBuilder() *SamplerProfileBuilder {
	builder := &SamplerProfileBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("sampler_profile", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_sampler").
		SetDescription("Profile for metrics sampler configurations. A sampler profile defines a reusable configuration template that can be applied to multiple object kinds or scenarios. Profiles provide sensible defaults and can be customized per use case.  Profiles are used to: - Define default sampler settings for common scenarios - Create named configurations that can be referenced - Apply consistent sampling strategies across object kinds - Enable quick configuration changes by updating the profile ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addSamplerProfileFields()

	return builder
}

// addSamplerProfileFields adds the sampler_profile fields
func (b *SamplerProfileBuilder) addSamplerProfileFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("applies_to", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used to determine which object kinds this profile applies to.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("object registry").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of object kinds this profile applies to. If empty, profile is a template that must be explicitly applied.").
			Security("non-sensitive").
			SystemUsage([]any{
				"profile application",
				"configuration routing",
			}).
			Validation("Array of valid object kind identifiers.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("SMP-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("is_default", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used to select default profile when no specific profile is specified.").
			Cardinality("one").
			Criticality("association").
			Default(false).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("If true, this profile is used as the default when no specific profile is requested. Only one profile should be marked as default per metric type.").
			Security("non-sensitive").
			SystemUsage([]any{
				"default selection",
				"fallback configuration",
			}).
			Validation("Boolean value.").
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
		WithProfileCode("SMP-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("profile_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used for profile lookup and application.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Unique name for this sampler profile (e.g., \\\\\\\"high_frequency\\\\\\\", \\\\\\\"low_latency\\\\\\\", \\\\\\\"audit_events\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"profile lookup",
				"configuration application",
			}).
			Validation("Must be a valid identifier (alphanumeric, underscores, hyphens).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[a-z0-9_-]+$`).
			Required(true).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("SMP-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *SamplerProfileBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *SamplerProfileBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *SamplerProfileBuilder) GetOntology() string {
	return "sampler_profile"
}

func init() {
	builders.RegisterBuilder(NewSamplerProfileBuilder())
}
