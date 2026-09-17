package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// DocEntryBuilder builds the doc_entry spec at version v2_0_0
// File: bldr_v2/doc_entry_builder.go - version is encoded in package/directory name
type DocEntryBuilder struct {
	*builders.BaseSpecBuilder
}

// NewDocEntryBuilder creates a new builder for doc_entry spec version v2_0_0
func NewDocEntryBuilder() *DocEntryBuilder {
	builder := &DocEntryBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("doc_entry", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("First-class representation of a document entry in the documentation index. Documents are core artifacts that link to goals, workstreams, and other objects. This enables traceability and discoverability via the docman CLI.\\nLifecycle: doc_entry_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addDocEntryFields()

	return builder
}

// addDocEntryFields adds the doc_entry fields
func (b *DocEntryBuilder) addDocEntryFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("category", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for advanced filtering and search queries.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("doc index organization.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Fine-grained category for document classification (e.g., \\\\\\\"validation\\\\\\\", \\\\\\\"graph-backend\\\\\\\", \\\\\\\"semantic-types\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"search",
				"organization",
			}).
			Validation("Lowercase alphanumeric with hyphens/underscores.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[a-z][a-z0-9_-]*$`).
			Required(false).
			Build()).
		WithTraits("filterable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("DOC-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("content_hash", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation.").
			AutomationHooks("used by system check and docman to verify cryptographic integrity and detect drift.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("content hasher.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Cryptographic SHA-256 leash of the document target file.").
			Security("non-sensitive").
			SystemUsage([]any{
				"verification",
				"drift-detection",
			}).
			Validation("SHA-256 hexadecimal string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[a-f0-9]{64}$`).
			Required(false).
			Build()).
		WithTraits("filterable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("DOC-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("content_searchable", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation.").
			AutomationHooks("used by search indexer to determine if content should be indexed.").
			Cardinality("one").
			Criticality("association").
			Default(true).
			Dependencies("search index, content extractor.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Flag indicating if document content should be indexed for full-text search.").
			Security("non-sensitive").
			SystemUsage([]any{
				"search",
				"indexing",
			}).
			Validation("boolean.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("flag").
		WithProfileCode("DOC-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("content_size", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation.").
			AutomationHooks("used by system check to verify file size integrity.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("content hasher.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Byte size of the document target file.").
			Security("non-sensitive").
			SystemUsage([]any{
				"verification",
				"metrics",
			}).
			Validation("integer >= 0.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "listable", "modifiable", "readable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("quantity").
		WithProfileCode("DOC-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for goal-document traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals this document relates to.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"navigation",
			}).
			Validation("Must reference existing goal IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("DOC-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("group", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for doc index organization.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("doc index organization.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Category/group the document belongs to (project_goals, project_specific, tooling, architecture, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"organization",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"project_goals",
				"project_specific",
				"tooling",
				"process",
				"onboarding",
				"design",
				"architecture",
				"other",
			}).
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "modifiable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("DOC-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("milestone_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for milestone-document traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("milestone registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestones this document relates to.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"navigation",
			}).
			Validation("Must reference existing milestone IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("DOC-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("path", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation.").
			AutomationHooks("used by docman view command, auto-registration.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("doc index, resolver.").
			Lifecycle("mutable (if document moves).").
			Observability("yes").
			Purpose("File system path or URL to the document.").
			Security("non-sensitive (unless path reveals sensitive locations).").
			SystemUsage([]any{
				"navigation",
				"resolution",
			}).
			Validation("Path must exist or be resolvable via resolver.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "modifiable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("DOC-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("requirement_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for requirement-document traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("requirement registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Requirements this document relates to.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"navigation",
			}).
			Validation("Must reference existing requirement IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("DOC-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("summary", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used in docman list output.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("doc index display.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Brief description of the document's content and purpose.").
			Security("non-sensitive").
			SystemUsage([]any{
				"search",
				"filtering",
				"previews",
			}).
			Validation("<= 200 chars recommended.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(200).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("DOC-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for workstream-document traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("workstream registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workstreams this document relates to.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"navigation",
			}).
			Validation("Must reference existing workstream IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("DOC-005"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *DocEntryBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *DocEntryBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *DocEntryBuilder) GetOntology() string {
	return "doc_entry"
}

func init() {
	builders.RegisterBuilder(NewDocEntryBuilder())
}
