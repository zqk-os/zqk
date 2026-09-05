package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// KeystoreEntryBuilder builds the keystore_entry spec at version v2_0_0
// File: bldr_v2/keystore_entry_builder.go - version is encoded in package/directory name
type KeystoreEntryBuilder struct {
	*builders.BaseSpecBuilder
}

// NewKeystoreEntryBuilder creates a new builder for keystore_entry spec version v2_0_0
func NewKeystoreEntryBuilder() *KeystoreEntryBuilder {
	builder := &KeystoreEntryBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("keystore_entry", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents a secure credential entry in the local keystore/vault. Stores hashed credentials (passwords, tokens) with access restrictions. Admins can see and edit all entries; users can only see/edit their own.\\nLifecycle: keystore_entry_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	// Add fields
	builder.addKeystoreEntryFields()

	return builder
}

// addKeystoreEntryFields adds the keystore_entry fields
func (b *KeystoreEntryBuilder) addKeystoreEntryFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("account_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin or account owner").
			AutomationHooks("set during key creation").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("account object").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Account ID this key belongs to (e.g., \\\\\\\"account:developer\\\\\\\")").
			Security("non-sensitive").
			SystemUsage([]any{
				"authentication",
				"access control",
			}).
			Validation("Must match account ID pattern").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^ACC-[A-Za-z0-9][A-Za-z0-9_-]*$`).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("KEY-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("credential_hash", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/auth service only").
			AutomationHooks("set during key creation, never exposed to users").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("hashing algorithm").
			Lifecycle("mutable (can be rotated)").
			Observability("no (never logged or exposed)").
			Purpose("Hashed credential (bcrypt for passwords, SHA256 for tokens). Never store raw credentials. Only system can read this field.").
			Security("highly sensitive - must be hashed, system-only access").
			SystemUsage([]any{
				"authentication",
			}).
			Validation("Must be a valid hash string").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("role:admin", "ACC-1785920548450214012-68b850c0").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("KEY-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin or account owner").
			AutomationHooks("set during key creation").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("key management").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Human-readable description of this key (e.g., \\\\\\\"MCP authentication key\\\\\\\", \\\\\\\"API access token\\\\\\\")").
			Security("non-sensitive").
			SystemUsage([]any{
				"key management",
				"documentation",
			}).
			Validation("Free-form string").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("KEY-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("expires_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin or account owner").
			AutomationHooks("set during key creation").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("key rotation policy").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("ISO-8601 timestamp when this key expires (null = never expires)").
			Security("non-sensitive").
			SystemUsage([]any{
				"key rotation",
				"expiration checks",
			}).
			Validation("ISO-8601 datetime").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("timestamp").
		WithProfileCode("KEY-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("key_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin or account owner").
			AutomationHooks("set during key creation").
			Cardinality("one").
			Criticality("composition").
			Default("password").
			Dependencies("authentication flow").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Type of credential (password, oauth_token, personal_access_token, api_key)").
			Security("non-sensitive").
			SystemUsage([]any{
				"authentication",
				"key management",
			}).
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"password",
				"oauth_token",
				"personal_access_token",
				"api_key",
			}).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable", "groupable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("KEY-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("last_used_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/auth service").
			AutomationHooks("updated on each successful authentication").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("authentication flow").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("ISO-8601 timestamp of last successful authentication using this key").
			Security("non-sensitive").
			SystemUsage([]any{
				"audit",
				"key rotation",
			}).
			Validation("ISO-8601 datetime").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).
			Required(false).
			Build()).
		WithTraits("readable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("timestamp").
		WithProfileCode("KEY-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("revoked", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin or account owner").
			AutomationHooks("set when key is revoked").
			Cardinality("one").
			Criticality("composition").
			Default(false).
			Dependencies("key management").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Whether this key has been revoked (revoked keys cannot be used for authentication)").
			Security("non-sensitive").
			SystemUsage([]any{
				"key management",
				"authentication",
			}).
			Validation("Boolean").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("state").
		WithProfileCode("KEY-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("revoked_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin or account owner").
			AutomationHooks("set when key is revoked").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("key revocation").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("ISO-8601 timestamp when this key was revoked").
			Security("non-sensitive").
			SystemUsage([]any{
				"audit",
				"key management",
			}).
			Validation("ISO-8601 datetime").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("timestamp").
		WithProfileCode("KEY-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("salt", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/auth service only").
			AutomationHooks("generated during key creation").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("hashing algorithm").
			Lifecycle("immutable").
			Observability("no").
			Purpose("Salt for password hashing (if not using bcrypt which has built-in salt). Only system can read this field.").
			Security("sensitive, system-only access").
			SystemUsage([]any{
				"authentication",
			}).
			Validation("Must be a valid salt string").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("role:admin", "ACC-1785920548450214012-68b850c0").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("KEY-004"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *KeystoreEntryBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *KeystoreEntryBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *KeystoreEntryBuilder) GetOntology() string {
	return "keystore_entry"
}

func init() {
	builders.RegisterBuilder(NewKeystoreEntryBuilder())
}
