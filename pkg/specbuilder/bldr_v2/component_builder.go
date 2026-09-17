package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// ComponentBuilder builds the component spec at version v2_0_0
// File: bldr_v2/component_builder.go - version is encoded in package/directory name
type ComponentBuilder struct {
	*builders.BaseSpecBuilder
}

// NewComponentBuilder creates a new builder for component spec version v2_0_0
func NewComponentBuilder() *ComponentBuilder {
	builder := &ComponentBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("component", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("extensible_object").
		SetDescription("Component objects represent visual elements in Gantt charts and other visualizations. Components are external domain objects that participate in the constraint system.\\nComponents inherit all base_object traits via extensible_object: listable, readable, writable, modifiable, removable, formatable, groupable, filterable, sortable, searchable.\\nLifecycle: component_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("constrainable")

	// Add fields
	builder.addComponentFields()

	return builder
}

// addComponentFields adds the component fields
func (b *ComponentBuilder) addComponentFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("component_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used for constraint system").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("component_types.yaml").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("The type of component (maps to component_types.yaml)").
			Security("non-sensitive").
			SystemUsage([]any{
				"identification",
				"grouping",
				"constraints",
			}).
			Validation("Must match a component type defined in component_types.yaml. Validated dynamically at runtime via GetCachedComponentTypes(). This ensures new component types added to component_types.yaml are automatically valid without requiring spec updates.\n").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{}).
			Required(true).
			Build()).
		WithTraits("readable", "filterable", "groupable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("COMP-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("constraint_contexts", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used for constraint loading").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("constraint system").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of constraint contexts where this component participates").
			Security("non-sensitive").
			SystemUsage([]any{
				"constraint system",
				"validation",
			}).
			Validation("Must reference valid constraint contexts").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_read_only_group").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("COMP-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("dashboard_relationships", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used for dashboard rendering").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("display_type (dashboard)").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Dashboard-specific component relationships (widgets, panels, etc.)").
			Security("non-sensitive").
			SystemUsage([]any{
				"dashboard rendering",
				"hierarchy",
			}).
			Validation("Must be valid dashboard relationship structure").
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
		WithProfileCode("COMP-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("display_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("links component to display").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("display storage").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to the display this component belongs to").
			Security("non-sensitive").
			SystemUsage([]any{
				"rendering",
				"grouping",
				"validation",
			}).
			Validation("Must reference valid display instance if provided").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^DSP-\d+$`).
			Required(false).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("COMP-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("gantt_relationships", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used for Gantt chart rendering").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("display_type (gantt_chart)").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Gantt-specific component relationships (contains, references, etc.)").
			Security("non-sensitive").
			SystemUsage([]any{
				"gantt rendering",
				"hierarchy",
			}).
			Validation("Must be valid Gantt relationship structure").
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
		WithProfileCode("COMP-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("kanban_relationships", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used for Kanban board rendering").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("display_type (kanban_board)").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Kanban-specific component relationships (columns, lanes, etc.)").
			Security("non-sensitive").
			SystemUsage([]any{
				"kanban rendering",
				"hierarchy",
			}).
			Validation("Must be valid Kanban relationship structure").
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
		WithProfileCode("COMP-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("object_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("links component to object").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("object storage").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Reference to the underlying object (if component maps to object)").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"validation",
			}).
			Validation("Must reference valid object if provided").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[A-Z]+-\d+$`).
			Required(false).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("COMP-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("parent_component_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used for component hierarchy").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("component storage, display_type").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to parent component instances (varies by display_type)").
			Security("non-sensitive").
			SystemUsage([]any{
				"hierarchy",
				"rendering",
				"validation",
			}).
			Validation("Must reference valid component instances if provided").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^COMP-\d+$`).
			Required(false).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("COMP-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("semantic_group_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used for semantic grouping").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("semantic group system").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to semantic groups this component belongs to").
			Security("non-sensitive").
			SystemUsage([]any{
				"grouping",
				"filtering",
			}).
			Validation("Must reference valid semantic groups").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("COMP-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ComponentBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ComponentBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ComponentBuilder) GetOntology() string {
	return "component"
}

func init() {
	builders.RegisterBuilder(NewComponentBuilder())
}
