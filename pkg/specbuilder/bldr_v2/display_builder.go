package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// DisplayBuilder builds the display spec at version v2_0_0
// File: bldr_v2/display_builder.go - version is encoded in package/directory name
type DisplayBuilder struct {
	*builders.BaseSpecBuilder
}

// NewDisplayBuilder creates a new builder for display spec version v2_0_0
func NewDisplayBuilder() *DisplayBuilder {
	builder := &DisplayBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("display", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("extensible_object").
		SetDescription("Display objects represent visualization contexts (Gantt charts, Kanban boards, dashboards, etc.). Displays are external domain objects that define how components are organized and rendered.\\nDisplays inherit all base_object traits via extensible_object: listable, readable, writable, modifiable, removable, formatable, groupable, filterable, sortable, searchable.\\nLifecycle: display_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("constrainable")

	// Add fields
	builder.addDisplayFields()

	return builder
}

// addDisplayFields adds the display fields
func (b *DisplayBuilder) addDisplayFields() {
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
			Purpose("List of constraint contexts where this display participates").
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
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("DSP-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("display_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used for constraint system").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("display_types.yaml").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("The type of display (maps to display_types.yaml)").
			Security("non-sensitive").
			SystemUsage([]any{
				"identification",
				"grouping",
				"constraints",
			}).
			Validation("Must match a display type defined in display_types.yaml. Validated dynamically at runtime via GetCachedDisplayTypes(). This ensures new display types added to display_types.yaml are automatically valid without requiring spec updates.\n").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "readable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("DSP-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("layout_config", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("user").
			AutomationHooks("used for display rendering").
			Cardinality("one").
			Criticality("association").
			Default(map[string]any{}).
			Dependencies("display_type").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Display-specific layout configuration (varies by display_type)").
			Security("non-sensitive").
			SystemUsage([]any{
				"rendering",
				"layout",
			}).
			Validation("Must be valid for display_type").
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
		WithProfileCode("DSP-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *DisplayBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *DisplayBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *DisplayBuilder) GetOntology() string {
	return "display"
}

func init() {
	builders.RegisterBuilder(NewDisplayBuilder())
}
