package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// BaseSamplerBuilder builds the base_sampler spec at version v2_0_0
// File: bldr_v2/base_sampler_builder.go - version is encoded in package/directory name
type BaseSamplerBuilder struct {
	*builders.BaseSpecBuilder
}

// NewBaseSamplerBuilder creates a new builder for base_sampler spec version v2_0_0
func NewBaseSamplerBuilder() *BaseSamplerBuilder {
	builder := &BaseSamplerBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("base_sampler", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Base specification for metrics sampler configurations. Defines how high-frequency events are batched in memory before creating metric objects. Sampler configs can be defined per object kind, field, or metric type to optimize metric collection performance.  Specialized samplers (scalar_metric_sampler, list_metric_sampler, etc.) extend this base spec to provide default configurations for specific metric types. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addBaseSamplerFields()

	return builder
}

// addBaseSamplerFields adds the base_sampler fields
func (b *BaseSamplerBuilder) addBaseSamplerFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("batch_size", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used by sampler to determine when to flush batches.").
			Cardinality("one").
			Criticality("composition").
			Default(50).
			Dependencies("metrics sampler system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Number of events to collect before creating a metric object. Must be between 1 and max_batch_size.").
			Security("non-sensitive").
			SystemUsage([]any{
				"batch management",
				"performance optimization",
			}).
			Validation("Integer between 1 and max_batch_size.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("measurement").
		WithProfileCode("SMC-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used for documentation and tooling.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Human-readable description of this sampler configuration and its purpose.").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"tooling",
			}).
			Validation("Free-form text, max 500 chars.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(500).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SMC-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("enabled", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("controls whether sampling is active for this configuration.").
			Cardinality("one").
			Criticality("composition").
			Default(true).
			Dependencies("metrics sampler system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Whether sampling is enabled for this configuration. If false, events are processed immediately without batching.").
			Security("non-sensitive").
			SystemUsage([]any{
				"sampler activation",
				"performance tuning",
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
		WithSemanticType("state").
		WithProfileCode("SMC-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("field_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used by metrics pipeline for field-specific sampling.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("object spec.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Optional field name this sampler applies to. If null, applies to all fields of the object kind.").
			Security("non-sensitive").
			SystemUsage([]any{
				"field-specific sampling",
			}).
			Validation("Must be a valid field name for the object kind.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("SMC-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("flush_interval", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used by sampler to flush partial batches after timeout.").
			Cardinality("one").
			Criticality("composition").
			Default("5m").
			Dependencies("metrics sampler system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Maximum time to wait before flushing a partial batch (e.g., \\\\\\\"5m\\\\\\\", \\\\\\\"10m\\\\\\\", \\\\\\\"1h\\\\\\\"). Prevents data loss and memory buildup.").
			Security("non-sensitive").
			SystemUsage([]any{
				"batch flushing",
				"memory management",
			}).
			Validation("Duration string (e.g., \\\\\\\"5m\\\\\\\", \\\\\\\"10m\\\\\\\", \\\\\\\"1h\\\\\\\").").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d+[smhd]|0$`).
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SMC-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("group_by_object_id", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used by sampler to determine batching strategy.").
			Cardinality("one").
			Criticality("composition").
			Default(true).
			Dependencies("metrics sampler system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("If true, batches are per-object (objectID -> batch). If false, batches are global (all objects combined).").
			Security("non-sensitive").
			SystemUsage([]any{
				"batching strategy",
				"aggregation grouping",
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
		WithSemanticType("state").
		WithProfileCode("SMC-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("max_batch_size", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used by sampler to enforce safety limits.").
			Cardinality("one").
			Criticality("composition").
			Default(1000).
			Dependencies("metrics sampler system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Maximum allowed batch size (safety limit). Prevents excessive memory usage.").
			Security("non-sensitive").
			SystemUsage([]any{
				"safety limits",
				"memory management",
			}).
			Validation("Integer >= batch_size.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("measurement").
		WithProfileCode("SMC-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metric_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used by metrics pipeline to select aggregator.").
			Cardinality("one").
			Criticality("composition").
			Default("system").
			Dependencies("metrics pipeline.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Metric type this sampler handles (e.g., \\\\\\\"system\\\\\\\", \\\\\\\"scalar_metric\\\\\\\", \\\\\\\"list_metric\\\\\\\", \\\\\\\"ordered_list_metric\\\\\\\", \\\\\\\"status_history_metric\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"aggregator selection",
				"metric routing",
			}).
			Validation("Must be a valid metric type.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"system",
				"scalar_metric",
				"list_metric",
				"ordered_list_metric",
				"status_history_metric",
			}).
			Required(true).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("SMC-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("object_kind", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used by metrics pipeline to route events to samplers.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("object registry.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Object kind this sampler configuration applies to (e.g., \\\\\\\"audit_event\\\\\\\", \\\\\\\"change_journal_entry\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"sampler routing",
				"configuration lookup",
			}).
			Validation("Must be a valid object kind.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("SMC-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *BaseSamplerBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *BaseSamplerBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *BaseSamplerBuilder) GetOntology() string {
	return "base_sampler"
}

func init() {
	builders.RegisterBuilder(NewBaseSamplerBuilder())
}

func addSamplerBatchAndIntervalFields(b *builders.BaseSpecBuilder) {
	b.AddFieldBuilder(builders.NewFieldBuilder("batch_size", "string"))
	b.AddFieldBuilder(builders.NewFieldBuilder("flush_interval", "string"))
	b.AddFieldBuilder(builders.NewFieldBuilder("group_by_object_id", "string"))
	b.AddFieldBuilder(builders.NewFieldBuilder("max_batch_size", "string"))
}
