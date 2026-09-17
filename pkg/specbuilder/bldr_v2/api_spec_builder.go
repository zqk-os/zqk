package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// APISpecBuilder builds the api_spec spec at version v2_0_0
type APISpecBuilder struct {
	*builders.BaseSpecBuilder
}

// NewAPISpecBuilder creates a new builder for api_spec spec version v2_0_0
func NewAPISpecBuilder() *APISpecBuilder {
	builder := &APISpecBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("api_spec", "v2_0_0"),
	}

	builder.
		SetExtends("base_object").
		SetDescription("Defines an API Specification for strict payload validation. Enforces strict structures for Autonomy Inbox messages, Mubert audio requests, and FFmpeg tool payloads.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("readable")

	builder.addAPISpecFields()

	return builder
}

// addAPISpecFields adds the api_spec fields
func (b *APISpecBuilder) addAPISpecFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("target_system", "string").
		Description("The target system for this API (e.g., mubert, ffmpeg, autonomy_inbox).").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()))

	b.AddFieldBuilder(builders.NewFieldBuilder("endpoint", "string").
		Description("The API endpoint or binary command path.").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))

	b.AddFieldBuilder(builders.NewFieldBuilder("method", "string").
		Description("The HTTP method or execution strategy (e.g., GET, POST, EXEC).").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))

	b.AddFieldBuilder(builders.NewFieldBuilder("payload_schema", "map").
		Description("Strict validation schema for the expected payload. Must be fulfilled by all Autonomy Inbox messages and tool payloads.").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()))

	b.AddFieldBuilder(builders.NewFieldBuilder("auth_strategy_ref", "string").
		Description("Reference to the auth_strategy to use for this API.").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))

	b.AddFieldBuilder(builders.NewFieldBuilder("timeout_ms", "integer").
		Description("Timeout for the API call or command execution in milliseconds.").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *APISpecBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *APISpecBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *APISpecBuilder) GetOntology() string {
	return "api_spec"
}

func init() {
	builders.RegisterBuilder(NewAPISpecBuilder())
}
