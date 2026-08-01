package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// MetricsExchangeContractBuilder builds the metrics_exchange_contract spec at version v2_0_0
// File: bldr_v2/metrics_exchange_contract_builder.go - version is encoded in package/directory name
type MetricsExchangeContractBuilder struct {
	*builders.BaseSpecBuilder
}

// NewMetricsExchangeContractBuilder creates a new builder for metrics_exchange_contract spec version v2_0_0
func NewMetricsExchangeContractBuilder() *MetricsExchangeContractBuilder {
	builder := &MetricsExchangeContractBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("metrics_exchange_contract", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Configuration contract for facade-level metrics exchange. Defines whether metrics export is enabled, the output format/profile, and sink routing for normalized internal events.").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addMetricsExchangeContractFields()

	return builder
}

// addMetricsExchangeContractFields adds the metrics_exchange_contract fields
func (b *MetricsExchangeContractBuilder) addMetricsExchangeContractFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("enabled", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("enables/disables facade metrics emission.").
			Cardinality("one").
			Criticality("composition").
			Default(false).
			Dependencies("metrics exchange pipeline.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Global on/off switch for facade client metrics exchange.").
			Security("non-sensitive").
			SystemUsage([]any{
				"metrics routing",
			}).
			Validation("boolean.").
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
		WithProfileCode("MXC-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("field_mapping", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("maps internal normalized event keys to exchange contract keys.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(map[string]any{}).
			Dependencies("encoder profile.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional key-map and include/exclude profile for external payload shape.").
			Security("non-sensitive").
			SystemUsage([]any{
				"contract mapping",
			}).
			Validation("object/map.").
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
		WithProfileCode("MXC-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("format", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("selects encoder for outbound contract payload.").
			Cardinality("one").
			Criticality("composition").
			Default("json").
			Dependencies("metrics exporter implementation.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Outbound payload format for metrics exchange events.").
			Security("non-sensitive").
			SystemUsage([]any{
				"encoding",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"json",
				"yaml",
				"protobuf",
				"avro",
			}).
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("MXC-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("operation_scope", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("filters operations included in exchange stream.").
			Cardinality("zero_or_more").
			Criticality("association").
			Default([]any{}).
			Dependencies("facade operation names.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional operation allow-list (for example push, commit).").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
			}).
			Validation("list of operation identifiers.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("list").
		WithProfileCode("MXC-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("provider_scope", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("filters providers included in exchange stream.").
			Cardinality("zero_or_more").
			Criticality("association").
			Default([]any{}).
			Dependencies("facade provider labels.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional provider allow-list (for example git).").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
			}).
			Validation("list of provider identifiers.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("list").
		WithProfileCode("MXC-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("sample_rate", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("probabilistic emission control.").
			Cardinality("one").
			Criticality("association").
			Default(1).
			Dependencies("exporter sampler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Fraction of eligible events to emit (0.0 to 1.0).").
			Security("non-sensitive").
			SystemUsage([]any{
				"rate limiting",
			}).
			Validation("number between 0 and 1.").
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
		WithProfileCode("MXC-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("sink_kind", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("chooses sink adapter family.").
			Cardinality("one").
			Criticality("composition").
			Default("file").
			Dependencies("exporter sink adapters.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Sink adapter type for contract events.").
			Security("non-sensitive").
			SystemUsage([]any{
				"routing",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"file",
				"stream",
				"http",
			}).
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("MXC-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("sink_target", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("resolves concrete destination (path, stream key, endpoint).").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("selected sink_kind.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Target descriptor for chosen sink kind.").
			Security("non-sensitive").
			SystemUsage([]any{
				"sink configuration",
			}).
			Validation("string.").
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
		WithProfileCode("MXC-006"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *MetricsExchangeContractBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *MetricsExchangeContractBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *MetricsExchangeContractBuilder) GetOntology() string {
	return "metrics_exchange_contract"
}

func init() {
	builders.RegisterBuilder(NewMetricsExchangeContractBuilder())
}
