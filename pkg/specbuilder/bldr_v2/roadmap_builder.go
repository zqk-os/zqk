package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// RoadmapBuilder builds the roadmap spec at version v2_0_0
// File: bldr_v2/roadmap_builder.go - version is encoded in package/directory name
type RoadmapBuilder struct {
	*builders.BaseSpecBuilder
}

// NewRoadmapBuilder creates a new builder for roadmap spec version v2_0_0
func NewRoadmapBuilder() *RoadmapBuilder {
	builder := &RoadmapBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("roadmap", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("work_interval").
		SetDescription("Strategic roadmap that creates views for a given set of workstreams, their associated priority plans, and backlog items within a specific time window. Roadmaps frame work by specifying workstream_refs and timeline_start/timeline_end to generate Gantt chart visualizations. The structure comes from workstreams (organizational), priority plans (planning), and backlog items (work items) - no manual phase definitions needed.\\nLifecycle: roadmap_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("completable")

	// Add fields
	builder.addRoadmapFields()

	return builder
}

// addRoadmapFields adds the roadmap fields
func (b *RoadmapBuilder) addRoadmapFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for goal-roadmap alignment.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals this roadmap supports.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"alignment",
			}).
			Validation("Must reference existing goal IDs.").
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
		WithProfileCode("RDM-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("milestone_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for roadmap-milestone traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("milestone registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestones included in this roadmap.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"traceability",
			}).
			Validation("Must reference existing milestone IDs.").
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
		WithProfileCode("RDM-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("timeline_end", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for timeline visualization.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("timeline generation.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("End date/time for the roadmap (ISO-8601).").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"reporting",
			}).
			Validation("ISO-8601 datetime.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}:\d{2}Z)?$`).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RDM-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("timeline_start", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for timeline visualization.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("timeline generation.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Start date/time for the roadmap (ISO-8601).").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"reporting",
			}).
			Validation("ISO-8601 datetime.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}:\d{2}Z)?$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RDM-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creator/owner.").
			AutomationHooks("used for roadmap filtering and navigation.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("workstream objects.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workstreams this roadmap applies to.").
			Security("non-sensitive").
			SystemUsage([]any{
				"navigation",
				"reporting",
			}).
			Validation("must reference existing workstream objects.").
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
		WithProfileCode("RDM-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *RoadmapBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *RoadmapBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *RoadmapBuilder) GetOntology() string {
	return "roadmap"
}

func init() {
	builders.RegisterBuilder(NewRoadmapBuilder())
}
