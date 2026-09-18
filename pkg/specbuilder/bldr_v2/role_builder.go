package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// RoleBuilder builds the role spec at version v2_0_0
// File: bldr_v2/role_builder.go - version is encoded in package/directory name
type RoleBuilder struct {
	*builders.BaseSpecBuilder
}

// NewRoleBuilder creates a new builder for role spec version v2_0_0
func NewRoleBuilder() *RoleBuilder {
	builder := &RoleBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("role", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines roles/authorities in the system (executive, owner, automation, etc.) with permissions.\\nLifecycle: role_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addRoleFields()

	return builder
}

// addRoleFields adds the role fields
func (b *RoleBuilder) addRoleFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("none.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("onboarding.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Explains responsibilities/influence.").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
			}).
			Validation("markdown.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^.+$`).
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RLE-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("influence_level", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("determines permission level.").
			Cardinality("one").
			Criticality("composition").
			Default("observer").
			Dependencies("permission system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Relative authority level (\\\\\\\"executive\\\\\\\", \\\\\\\"owner\\\\\\\", \\\\\\\"collective\\\\\\\", \\\\\\\"observer\\\\\\\", \\\\\\\"automation\\\\\\\", etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"permissions",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"executive",
				"owner",
				"collective",
				"observer",
				"automation",
			}).
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "modifiable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RLE-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("permissions", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("used for permission checks.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("permission system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of specific permissions granted to this role.").
			Security("non-sensitive").
			SystemUsage([]any{
				"authorization",
			}).
			Validation("Controlled vocabulary of permission names.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RLE-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("role_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin.").
			AutomationHooks("used during role checks.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("profile configs.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Unique identifier for the role.").
			Security("non-sensitive").
			SystemUsage([]any{
				"permissions",
				"profiles",
			}).
			Validation("Username pattern.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[a-z0-9_-]+$`).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("RLE-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *RoleBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *RoleBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *RoleBuilder) GetOntology() string {
	return "role"
}

func init() {
	builders.RegisterBuilder(NewRoleBuilder())
}
