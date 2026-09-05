package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// EvolutionManagementBuilder builds the evolution_management spec at version v2_0_0
// File: bldr_v2/evolution_management_builder.go - version is encoded in package/directory name
type EvolutionManagementBuilder struct {
	*builders.BaseSpecBuilder
}

// NewEvolutionManagementBuilder creates a new builder for evolution_management spec version v2_0_0
func NewEvolutionManagementBuilder() *EvolutionManagementBuilder {
	builder := &EvolutionManagementBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("evolution_management", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Manages incremental, chaotic system evolution by tracking chaos indicators, adaptive adjustments, and evolution patterns. Ensures system stability during rapid growth.\\nLifecycle: evolution_management_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addEvolutionManagementFields()

	return builder
}

// addEvolutionManagementFields adds the evolution_management fields
func (b *EvolutionManagementBuilder) addEvolutionManagementFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("adaptive_adjustments", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for adjustment tracking").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of adaptive adjustments made based on chaos indicators.").
			Security("non-sensitive").
			SystemUsage([]any{
				"evolution_management",
				"adjustment_tracking",
			}).
			Validation("List of adjustment objects").
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
		WithProfileCode("EVOL-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("chaos_indicators", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for health monitoring").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of chaos indicators being monitored (graph growth rate, agent onboarding velocity, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"evolution_monitoring",
				"health_tracking",
			}).
			Validation("List of indicator objects").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("EVOL-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("period", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for period validation").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Time period covered by this evolution management (e.g., \\\\\\\"2026-01-01 to 2026-03-31\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"evolution_tracking",
				"reporting",
			}).
			Validation("Date range format").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "readable", "searchable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("EVOL-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("strategy", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("determines management approach").
			Cardinality("one").
			Criticality("composition").
			Default("adaptive").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Strategy for managing evolution (adaptive, conservative, aggressive, balanced).").
			Security("non-sensitive").
			SystemUsage([]any{
				"evolution_management",
				"strategy_application",
			}).
			Validation("enum").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"adaptive",
				"conservative",
				"aggressive",
				"balanced",
			}).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("EVOL-002"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *EvolutionManagementBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *EvolutionManagementBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *EvolutionManagementBuilder) GetOntology() string {
	return "evolution_management"
}

func init() {
	builders.RegisterBuilder(NewEvolutionManagementBuilder())
}
