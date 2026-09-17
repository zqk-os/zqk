package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// RemainingOpenBuilder builds the remaining_open mixin spec at version v2_0_0.
type RemainingOpenBuilder struct {
	*builders.BaseSpecBuilder
}

// NewRemainingOpenBuilder creates a builder for the remaining_open mixin.
func NewRemainingOpenBuilder() *RemainingOpenBuilder {
	builder := &RemainingOpenBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("remaining_open", "v2_0_0"),
	}

	builder.
		SetExtends("work_interval").
		SetDescription("Mixin for remaining-open cardinality (remaining_open_count). open_countable means the container's behavior changes when the count hits zero. Membership stays child→parent (not stored here). Not instantiable (kind_mappings skip_specs). Compose onto Gantt containers that complete when members drain (priority_plan). Parallel: occupancy houses claimed_by / claimed_at. TRACK: POL-ARCH-20260901 / BLI-CEF-CONTAINER-REMAINING-OPEN-001. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("open_countable")

	builder.addRemainingOpenFields()
	return builder
}

func (b *RemainingOpenBuilder) addRemainingOpenFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder(objects.FieldKeyRemainingOpenCount, "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system/lifecycle.").
			AutomationHooks("open_countable last-child complete; seed at execution lock; increment on reopen/override-add.").
			Cardinality("one").
			Criticality("composition").
			Default("null (unset until execution-locked; then non-negative integer)").
			Dependencies("open_countable trait; member terminal hops.").
			Lifecycle("mutable.").
			Observability("yes.").
			Purpose("Count of linked members still open (non-terminal). Behavior of this container changes when the count hits zero (complete). Not membership refs.").
			Security("non-sensitive").
			SystemUsage([]any{
				"lifecycle",
				"shockwave",
				"completeness",
			}).
			Validation("Non-negative integer when set. Unset means the interpreter has not seeded this instance.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("quantity").
		WithProfileCode("ROC-001"))
}

func (b *RemainingOpenBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

func (b *RemainingOpenBuilder) GetVersion() string {
	return "v2_0_0"
}

func (b *RemainingOpenBuilder) GetOntology() string {
	return "remaining_open"
}

func init() {
	builders.RegisterBuilder(NewRemainingOpenBuilder())
}
