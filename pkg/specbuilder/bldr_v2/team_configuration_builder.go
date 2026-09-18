package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// TeamConfigurationBuilder builds the team_configuration spec at version v2_0_0
// File: bldr_v2/team_configuration_builder.go - version is encoded in package/directory name
type TeamConfigurationBuilder struct {
	*builders.BaseSpecBuilder
}

// NewTeamConfigurationBuilder creates a new builder for team_configuration spec version v2_0_0
func NewTeamConfigurationBuilder() *TeamConfigurationBuilder {
	builder := &TeamConfigurationBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("team_configuration", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines a Team Configuration (or Cell archetype) to support the biological swarm architecture\\n(Neuron, Muscle, Heart, Lungs). Instead of spinning up every active persona, the CAP Loop \\nreferences a Team Configuration to spin up a tailored composition of Personas for a specific Priority Plan or Goal.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("constrainable")

	// Add fields
	builder.addTeamConfigurationFields()

	return builder
}

// addTeamConfigurationFields adds the team_configuration fields
func (b *TeamConfigurationBuilder) addTeamConfigurationFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("cell_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("Used by the orchestrator to determine the operational mode of the pod.").
			Cardinality("one").
			Criticality("metadata").
			Default("muscle").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The biological cell archetype (e.g., neuron, muscle, heart, lungs).").
			Security("non-sensitive").
			SystemUsage([]any{
				"provisioning",
				"telemetry",
			}).
			Validation("Must be one of neuron, muscle, heart, lungs").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "modifiable", "readable", "searchable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TCFG-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("focus_area", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("Used to align the team to organizational divisions.").
			Cardinality("one").
			Criticality("metadata").
			Default("engineering").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The strategic focus of the team (e.g., marketing, strategic, architecture).").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
			}).
			Validation("Must be a non-empty string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "modifiable", "readable", "searchable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TCFG-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (Generator)").
			AutomationHooks("used for cross-file references.").
			Cardinality("one").
			Criticality("composition").
			Default("auto-assigned per kind sequence").
			Dependencies("linkage constraints, URN creation.").
			Lifecycle("immutable").
			Observability("logged + manifests.").
			Purpose("Stable identifier.").
			Security("non-sensitive").
			SystemUsage([]any{
				"linking",
				"reporting",
			}).
			Validation("Must start with TCFG-").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("TCFG-000"))
	b.AddFieldBuilder(builders.NewFieldBuilder("persona_allocations", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("Used by Sub-Kernel Provisioner to spawn concurrent workers.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("persona registry").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of personas that comprise this team and their counts.").
			Security("non-sensitive").
			SystemUsage([]any{
				"provisioning",
				"concurrency mapping",
			}).
			Validation("Must map persona references to required instances").
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
		WithProfileCode("TCFG-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *TeamConfigurationBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *TeamConfigurationBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *TeamConfigurationBuilder) GetOntology() string {
	return "team_configuration"
}

func init() {
	builders.RegisterBuilder(NewTeamConfigurationBuilder())
}
