package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// LifecycleBuilder builds the lifecycle spec at version v2_0_0
// File: bldr_v2/lifecycle_builder.go - version is encoded in package/directory name
type LifecycleBuilder struct {
	*builders.BaseSpecBuilder
}

// NewLifecycleBuilder creates a new builder for lifecycle spec version v2_0_0
func NewLifecycleBuilder() *LifecycleBuilder {
	builder := &LifecycleBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("lifecycle", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Lifecycle definitions define state machines for object kinds.\\nBuilt-in lifecycles are immutable defaults generated from lifecycle builders.\\nInternal lifecycle objects can override defaults for custom workflows.\\n\\nEach lifecycle object defines:\\n- Valid statuses for an object kind\\n- Valid transitions between statuses\\n- Preconditions for statuses and transitions\\n- Percent complete calculation configuration\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addLifecycleFields()

	return builder
}

// addLifecycleFields adds the lifecycle fields
func (b *LifecycleBuilder) addLifecycleFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("extends", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used for lifecycle inheritance").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("parent lifecycle definitions").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Parent lifecycle to extend (e.g., \\\"base_lifecycle\\\"). Child lifecycle inherits parent statuses and transitions, with child overrides.").
			Security("non-sensitive").
			SystemUsage([]any{
				"lifecycle inheritance",
				"lifecycle loading",
			}).
			Validation("Must reference a valid lifecycle (by naming convention or ID)").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("LIFECYCLE-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (Generator)").
			AutomationHooks("used for cross-file references.").
			Cardinality("one").
			Criticality("composition").
			Default("auto-assigned per object_type sequence").
			Dependencies("linkage constraints, URN creation.").
			Lifecycle("immutable").
			Observability("logged + manifests.").
			Purpose("Stable identifier for lifecycle objects (LIFECYCLE-{ABBR}-### format).").
			Security("non-sensitive").
			SystemUsage([]any{
				"linking",
				"reporting",
				"URIs",
			}).
			Validation("regex ^LIFECYCLE-[A-Z]+(-[A-Z]+)*-\\\\\\\\d{3,}$; uniqueness enforced.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^LIFECYCLE-[A-Z]+(-[A-Z]+)*-\d{3,}$`).
			Required(true).
			Build()).
		WithTraits("field_queryable_group").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("LIFECYCLE-000"))
	b.AddFieldBuilder(builders.NewFieldBuilder("object_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used for lifecycle resolution (which lifecycle applies to which object kind)").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("lifecycle loader").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("The object kind this lifecycle defines states for (e.g., \\\"backlog_item\\\", \\\"goal\\\", \\\"milestone\\\")").
			Security("non-sensitive").
			SystemUsage([]any{
				"lifecycle loading",
				"validation",
				"lifecycle resolution",
			}).
			Validation("Must match a valid object kind").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[a-z_]+$`).
			Required(true).
			Build()).
		WithTraits("readable", "filterable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("LIFECYCLE-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("percent_complete", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used for percent complete calculation").
			Cardinality("one").
			Criticality("association").
			Default(map[string]any{}).
			Dependencies("statuses (for default_by_status), milestone_refs (for milestone_based)").
			Lifecycle("mutable (for internal objects)").
			Observability("yes").
			Purpose("Configuration for calculating percent complete. Supports methods: \\\"status_defaults\\\", \\\"milestone_based\\\", or custom calculation.").
			Security("non-sensitive").
			SystemUsage([]any{
				"percent complete calculation",
				"progress tracking",
			}).
			Validation("Object with fields: method (string), default_by_status (map[string]any), milestone_based (map[string]any)").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("LIFECYCLE-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("source_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("determines if lifecycle is mutable (built-in vs internal)").
			Cardinality("one").
			Criticality("composition").
			Default("internal").
			Dependencies("lifecycle loader, built-in detection").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("\\\"built-in\\\" for immutable defaults (generated from builders), \\\"internal\\\" for overridable objects").
			Security("non-sensitive").
			SystemUsage([]any{
				"lifecycle mutability",
				"override detection",
				"validation",
			}).
			Validation("Must be \\\"built-in\\\" or \\\"internal\\\"").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"built-in",
				"internal",
			}).
			Required(false).
			Build()).
		WithTraits("readable", "filterable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("LIFECYCLE-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status_mapping", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("maps internal (parent) statuses to external (child) statuses during inheritance").
			Cardinality("one").
			Criticality("association").
			Default(map[string]any{}).
			Dependencies("parent lifecycle (used when extending)").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Maps internal (base) statuses to external (this lifecycle) statuses. Used when this lifecycle extends another.").
			Security("non-sensitive").
			SystemUsage([]any{
				"lifecycle inheritance",
				"status translation",
			}).
			Validation("Map of string to string (status name -> mapped status name)").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("LIFECYCLE-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("statuses", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used for status validation, initial status resolution, transition validation").
			Cardinality("many").
			Criticality("core").
			Default([]any{}).
			Dependencies("lifecycle loader").
			Lifecycle("mutable (for internal objects)").
			Observability("yes").
			Purpose("List of valid statuses for this object type. Each status defines value, display, initial/terminal/archive/system flags, and preconditions.").
			Security("non-sensitive").
			SystemUsage([]any{
				"status validation",
				"initial status resolution",
				"transition validation",
				"lifecycle state machine",
			}).
			Validation("Array of Status objects with fields: value (string, required), display (string, required), initial (bool), terminal (bool), archive (bool), system (bool), preconditions ([]string), description (string). At least one status must have initial: true.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MinCount(1).
			Required(true).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("LIFECYCLE-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("transitions", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used for transition validation, automatic status changes").
			Cardinality("many").
			Criticality("core").
			Default([]any{}).
			Dependencies("statuses (from/to must be valid statuses)").
			Lifecycle("mutable (for internal objects)").
			Observability("yes").
			Purpose("Valid state transitions. Each transition defines from/to statuses, description, manual/auto flags, and preconditions.").
			Security("non-sensitive").
			SystemUsage([]any{
				"transition validation",
				"automatic status changes",
				"lifecycle state machine",
			}).
			Validation("Array of Transition objects with fields: from (string, required), to (string, required), description (string, required), manual (bool, required), auto (bool, required), preconditions ([]string). From/to must reference valid status values.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("LIFECYCLE-006"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *LifecycleBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *LifecycleBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *LifecycleBuilder) GetOntology() string {
	return "lifecycle"
}

func init() {
	builders.RegisterBuilder(NewLifecycleBuilder())
}
