package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// TdeEnvelopeBuilder builds the tde_envelope spec at version v2_0_0
// File: bldr_v2/tde_envelope_builder.go - version is encoded in package/directory name
type TdeEnvelopeBuilder struct {
	*builders.BaseSpecBuilder
}

// NewTdeEnvelopeBuilder creates a new builder for tde_envelope spec version v2_0_0
func NewTdeEnvelopeBuilder() *TdeEnvelopeBuilder {
	builder := &TdeEnvelopeBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("tde_envelope", "v2_0_0"),
	}

	builder.
		SetExtends("base_object").
		SetDescription("Envelope for time-delayed execution of agent intents (Autonomy Inbox). Occupancy uses inherited namespace_id from base_object; do not set namespace_id as spec metadata. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	builder.addTdeEnvelopeFields()

	return builder
}

func (b *TdeEnvelopeBuilder) addTdeEnvelopeFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("agent_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("submitting agent").
			AutomationHooks("set on inbox Submit").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("agent seat / account").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Mesh seat or agent that submitted this envelope.").
			Security("non-sensitive").
			SystemUsage([]any{"inbox routing", "approval provenance"}).
			Validation("Non-empty agent id").
			Build()).
		WithAccess(builders.NewAccessBuilder().Requires("access:confidential").Build()).
		WithValidation(builders.NewValidationBuilder().Required(true).Build()).
		WithTraits("readable", "filterable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("TDE-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("intent", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("submitting agent").
			AutomationHooks("set on inbox Submit").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Human-readable intent of the delayed action.").
			Security("non-sensitive").
			SystemUsage([]any{"operator review"}).
			Validation("Free-form text").
			Build()).
		WithAccess(builders.NewAccessBuilder().Requires("access:confidential").Build()).
		WithValidation(builders.NewValidationBuilder().Required(false).Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TDE-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("capability_tokens", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("submitting agent").
			AutomationHooks("set on inbox Submit").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Capability tokens this envelope may exercise (e.g. fs:delete). Not occupancy; not a typed kernel object edge.").
			Security("non-sensitive").
			SystemUsage([]any{"capability gating", "operator review"}).
			Validation("List of capability token strings").
			Build()).
		WithAccess(builders.NewAccessBuilder().Requires("access:confidential").Build()).
		WithValidation(builders.NewValidationBuilder().Required(false).Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("list").
		WithProfileCode("TDE-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("payload", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("submitting agent").
			AutomationHooks("set on inbox Submit").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none").
			Lifecycle("immutable").
			Observability("no").
			Purpose("Serialized action payload awaiting approval.").
			Security("confidential").
			SystemUsage([]any{"delayed execution"}).
			Validation("Non-empty payload string").
			Build()).
		WithAccess(builders.NewAccessBuilder().Requires("access:confidential").Build()).
		WithValidation(builders.NewValidationBuilder().Required(true).Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TDE-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("reject_reason", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator").
			AutomationHooks("set on inbox Reject").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Why this envelope was rejected.").
			Security("non-sensitive").
			SystemUsage([]any{"operator review", "audit"}).
			Validation("Free-form text when status=rejected").
			Build()).
		WithAccess(builders.NewAccessBuilder().Requires("access:confidential").Build()).
		WithValidation(builders.NewValidationBuilder().Required(false).Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TDE-005"))
}

func (b *TdeEnvelopeBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

func (b *TdeEnvelopeBuilder) GetVersion() string {
	return "v2_0_0"
}

func (b *TdeEnvelopeBuilder) GetOntology() string {
	return "tde_envelope"
}

func init() {
	builders.RegisterBuilder(NewTdeEnvelopeBuilder())
}
