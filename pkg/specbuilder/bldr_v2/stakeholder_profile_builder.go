package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// StakeholderProfileBuilder builds the stakeholder_profile spec at version v2_0_0
// File: bldr_v2/stakeholder_profile_builder.go - version is encoded in package/directory name
type StakeholderProfileBuilder struct {
	*builders.BaseSpecBuilder
}

// NewStakeholderProfileBuilder creates a new builder for stakeholder_profile spec version v2_0_0
func NewStakeholderProfileBuilder() *StakeholderProfileBuilder {
	builder := &StakeholderProfileBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("stakeholder_profile", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Stakeholder profile captures expectations and priorities for a stakeholder (e.g. executive, product owner).\\nUsed by strategic alignment (zqk system align) and goal discovery. See project-discovery-and-strategic-alignment-v1.0.md.\\nLifecycle: stakeholder_profile_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addStakeholderProfileFields()

	return builder
}

// addStakeholderProfileFields adds the stakeholder_profile fields
func (b *StakeholderProfileBuilder) addStakeholderProfileFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("alignment_metrics", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for alignment dashboard.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Metrics and targets (e.g. metric, target) for stakeholder satisfaction.").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
				"dashboard",
			}).
			Validation("List of objects with metric, target.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("STK-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("communication_preferences", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for notification routing.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Frequency, format, and channels (e.g. frequency, format, channels).").
			Security("non-sensitive").
			SystemUsage([]any{
				"notifications",
			}).
			Validation("Object with optional frequency, format (list), channels (list).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("STK-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("expectations", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for stakeholder-work alignment.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of stakeholder expectations (e.g. quarterly reports, budget adherence).").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
				"reporting",
			}).
			Validation("List of strings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("STK-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("priorities", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for alignment scoring.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Prioritized concerns with weights (priority, concern, weight).").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
			}).
			Validation("List of objects with priority, concern, weight.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("STK-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("stakeholder_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for alignment reporting and routing.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Type of stakeholder (e.g. decision_maker, contributor, reviewer).").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
				"reporting",
			}).
			Validation("Free-form string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "filterable", "listable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("STK-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *StakeholderProfileBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *StakeholderProfileBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *StakeholderProfileBuilder) GetOntology() string {
	return "stakeholder_profile"
}

func init() {
	builders.RegisterBuilder(NewStakeholderProfileBuilder())
}
