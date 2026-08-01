package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// ZqkSessionBuilder builds the zqk_session spec at version v2_0_0
// File: bldr_v2/zqk_session_builder.go - version is encoded in package/directory name
type ZqkSessionBuilder struct {
	*builders.BaseSpecBuilder
}

// NewZqkSessionBuilder creates a new builder for zqk_session spec version v2_0_0
func NewZqkSessionBuilder() *ZqkSessionBuilder {
	builder := &ZqkSessionBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("zqk_session", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents a CLI or interactive session (e.g. a zqk invocation or agent session). Tracks session state and optional metadata.\\nLifecycle: zqk_session_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	// Add fields
	builder.addZqkSessionFields()

	return builder
}

// addZqkSessionFields adds the zqk_session fields
func (b *ZqkSessionBuilder) addZqkSessionFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("account_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("set from CLI security context (current user)").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("account object").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Account that owns this session (e.g. account:system or logged-in user)").
			Security("non-sensitive").
			SystemUsage([]any{
				"session attribution",
				"per-account session limits",
			}).
			Validation("Must match account ID pattern").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("ZQK-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("set by CLI or session manager").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("session lifecycle").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Stable identifier for the session (e.g. ZQK-001)").
			Security("non-sensitive").
			SystemUsage([]any{
				"session tracking",
			}).
			Validation("Must match ZQK session ID pattern").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^ZQK-\d+$`).
			Required(true).
			Build()).
		WithTraits("readable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("ZQK-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("title", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system/user").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional human-readable label for the session").
			Security("non-sensitive").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ZQK-002"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ZqkSessionBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ZqkSessionBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ZqkSessionBuilder) GetOntology() string {
	return "zqk_session"
}

func init() {
	builders.RegisterBuilder(NewZqkSessionBuilder())
}
