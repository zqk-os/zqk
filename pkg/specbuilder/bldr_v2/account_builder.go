package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// AccountBuilder builds the account spec at version v2_0_0
// File: bldr_v2/account_builder.go - version is encoded in package/directory name
type AccountBuilder struct {
	*builders.BaseSpecBuilder
}

// NewAccountBuilder creates a new builder for account spec version v2_0_0
func NewAccountBuilder() *AccountBuilder {
	builder := &AccountBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("account", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents a human or automation account that interacts with Workstream OS.\\nLifecycle: account_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addAccountFields()

	return builder
}

// addAccountFields adds the account fields
func (b *AccountBuilder) addAccountFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("display_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("account owner.").
			AutomationHooks("UI only.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("documentation.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Friendly name for UI/docs.").
			Security("PII—treat as confidential.").
			SystemUsage([]any{
				"docs",
				"status",
			}).
			Validation("<= 80 chars.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(80).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ACC-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("email", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("account owner/admin.").
			AutomationHooks("used for notifications.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("login flow.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Contact/authentication email.").
			Security("PII—must be protected.").
			SystemUsage([]any{
				"login",
				"notifications",
			}).
			Validation("RFC5322 email format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ACC-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for cross-file references.").
			Cardinality("one").
			Criticality("composition").
			Default("auto-assigned per kind sequence").
			Dependencies("linkage constraints, URN creation.").
			Lifecycle("immutable").
			Observability("logged + manifests.").
			Purpose("Stable identifier for the account. ACC-* form only (timestamp, short numeric, or subtype prefix).").
			Security("non-sensitive").
			SystemUsage([]any{
				"linking",
				"reporting",
				"URIs",
			}).
			Validation("Must match ACC-* pattern (no account:username legacy form).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^ACC-[A-Za-z0-9][A-Za-z0-9_-]*$`).
			Required(true).
			Build()).
		WithTraits("filterable", "listable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("ACC-000"))
	b.AddFieldBuilder(builders.NewFieldBuilder("persona_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("used for seat resolution.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("persona existence.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Persona reference for agent account (CRI-ACCOUNT-RBAC-READY).").
			Security("non-sensitive").
			SystemUsage([]any{
				"authentication",
				"authorization",
			}).
			Validation("valid persona ID").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ACC-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("persona_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("used for seat resolution.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("persona existence.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of persona references for agent account (CRI-ACCOUNT-RBAC-READY).").
			Security("non-sensitive").
			SystemUsage([]any{
				"authentication",
				"authorization",
			}).
			Validation("valid persona IDs").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ACC-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("profile_metadata", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("account owner.").
			AutomationHooks("apply CLI defaults at login.").
			Cardinality("one").
			Criticality("association").
			Default(map[string]any{}).
			Dependencies("CLI preferences.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Additional preferences (default profile, context cadence).").
			Security("may hold private preferences—treat accordingly.").
			SystemUsage([]any{
				"personalization",
			}).
			Validation("JSON object with documented fields.").
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
		WithProfileCode("ACC-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("roles", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("used before mutations.").
			Cardinality("many (>=1)").
			Criticality("composition").
			Default([]any{
				"observer",
			}).
			Dependencies("permission checks.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Roles assigned to this account.").
			Security("non-sensitive").
			SystemUsage([]any{
				"authorization",
			}).
			Validation("IDs must exist in role specs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ACC-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("tokens", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/auth service.").
			AutomationHooks("used to invalidate sessions.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("CLI login.").
			Lifecycle("mutable").
			Observability("yes (metadata only; never store raw secrets).").
			Purpose("References to active auth tokens/credentials (local keystore, JWT fingerprint, etc.).").
			Security("sensitive metadata ensure hashed/fingerprint only.").
			SystemUsage([]any{
				"authentication",
			}).
			Validation("each entry contains token_id, fingerprint, expiration.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ACC-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("username", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("used to tag audit events.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("login flow.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Login/display name.").
			Security("PII – treat as confidential.").
			SystemUsage([]any{
				"authentication",
				"auditing",
			}).
			Validation("Username pattern or valid email address.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[a-z0-9._-]{3,32}$|^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`).
			Required(true).
			Build()).
		WithTraits("filterable", "listable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ACC-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *AccountBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *AccountBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *AccountBuilder) GetOntology() string {
	return "account"
}

func init() {
	builders.RegisterBuilder(NewAccountBuilder())
}
