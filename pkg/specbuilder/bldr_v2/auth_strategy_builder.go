package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// AuthStrategyBuilder builds the auth_strategy spec at version v2_0_0
// File: bldr_v2/auth_strategy_builder.go - version is encoded in package/directory name
type AuthStrategyBuilder struct {
	*builders.BaseSpecBuilder
}

// NewAuthStrategyBuilder creates a new builder for auth_strategy spec version v2_0_0
func NewAuthStrategyBuilder() *AuthStrategyBuilder {
	builder := &AuthStrategyBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("auth_strategy", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents an authentication strategy available in the system. Defines how users can authenticate (keystore, OAuth, username/password, etc.). Strategies are configured at the system level and can be enabled/disabled.\\nLifecycle: auth_strategy_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	// Add fields
	builder.addAuthStrategyFields()

	return builder
}

// addAuthStrategyFields adds the auth_strategy fields
func (b *AuthStrategyBuilder) addAuthStrategyFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("configuration", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin").
			AutomationHooks("used by authentication implementation").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(map[string]any{}).
			Dependencies("strategy implementation").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Strategy-specific configuration (e.g., OAuth provider URL, keystore path)").
			Security("may contain sensitive configuration").
			SystemUsage([]any{
				"authentication",
				"configuration",
			}).
			Validation("JSON object with strategy-specific fields").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("AUTH-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin").
			AutomationHooks("used in documentation").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("documentation").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Human-readable description of this authentication strategy").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"configuration",
			}).
			Validation("Free-form string").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("AUTH-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("enabled", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin").
			AutomationHooks("checked during authentication flow").
			Cardinality("one").
			Criticality("composition").
			Default(false).
			Dependencies("configuration").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Whether this authentication strategy is enabled").
			Security("non-sensitive").
			SystemUsage([]any{
				"authentication",
				"configuration",
			}).
			Validation("Boolean").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("state").
		WithProfileCode("AUTH-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("priority", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin").
			AutomationHooks("used to determine order of strategy checks").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("authentication flow").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Priority order for this strategy (lower numbers checked first)").
			Security("non-sensitive").
			SystemUsage([]any{
				"authentication",
			}).
			Validation("Integer >= 0").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("ordering").
		WithProfileCode("AUTH-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("strategy_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin").
			AutomationHooks("used by authentication flow").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("authentication implementation").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Type of authentication strategy (keystore, oauth, username_password, personal_access_token, api_key)").
			Security("non-sensitive").
			SystemUsage([]any{
				"authentication",
				"configuration",
			}).
			Validation("Must be one of: keystore, oauth, username_password, personal_access_token, api_key").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"keystore",
				"oauth",
				"username_password",
				"personal_access_token",
				"api_key",
			}).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable", "groupable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("AUTH-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *AuthStrategyBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *AuthStrategyBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *AuthStrategyBuilder) GetOntology() string {
	return "auth_strategy"
}

func init() {
	builders.RegisterBuilder(NewAuthStrategyBuilder())
}
