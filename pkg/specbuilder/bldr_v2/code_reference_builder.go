package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// CodeReferenceBuilder builds the code_reference spec at version v2_0_0
// File: bldr_v2/code_reference_builder.go - version is encoded in package/directory name
type CodeReferenceBuilder struct {
	*builders.BaseSpecBuilder
}

// NewCodeReferenceBuilder creates a new builder for code_reference spec version v2_0_0
func NewCodeReferenceBuilder() *CodeReferenceBuilder {
	builder := &CodeReferenceBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("code_reference", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents a reference to a specific code location (file, function, line range) for traceability between requirements, tests, and implementation. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addCodeReferenceFields()

	return builder
}

// addCodeReferenceFields adds the code_reference fields
func (b *CodeReferenceBuilder) addCodeReferenceFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("author", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for contributor analysis.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("Version control repository (Git, SVN, Mercurial, Perforce, etc.).").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Author of the version control commit.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"attribution",
			}).
			Validation("author name pattern.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("COD-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("author_email", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for contributor analysis.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("Version control repository (Git, SVN, Mercurial, Perforce, etc.).").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Email of the commit author.").
			Security("may be sensitive (PII).").
			SystemUsage([]any{
				"traceability",
				"attribution",
			}).
			Validation("must be valid email format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("COD-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("backlog_item_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner.").
			AutomationHooks("used for code-to-backlog traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("backlog_item registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Backlog items this code reference implements.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("Must reference existing backlog_item IDs.").
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
		WithProfileCode("COD-014"))
	b.AddFieldBuilder(builders.NewFieldBuilder("change_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for change impact analysis.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("Version control repository (Git, SVN, Mercurial, Perforce, etc.).").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Type of change to the file (added, modified, deleted, renamed, copied).").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"change analysis",
			}).
			Validation("must be one of: added, modified, deleted, renamed, copied").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"added",
				"modified",
				"deleted",
				"renamed",
				"copied",
			}).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("COD-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("commit_date", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for temporal queries.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("Version control repository (Git, SVN, Mercurial, Perforce, etc.).").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Date of the version control commit (ISO 8601 format).").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"temporal analysis",
			}).
			Validation("must be valid ISO 8601 date.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "modifiable", "readable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("COD-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("commit_hash", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for linking commits to work items.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("Version control repository (Git, SVN, Mercurial, Perforce, etc.).").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Version control system commit identifier that introduced this code reference.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"code-to-project graph",
			}).
			Validation("Must be valid VCS commit identifier. Format depends on VCS system:\n- Git: SHA-1 hash (40 chars) or short hash (7+ chars)\n- SVN: Revision number (numeric)\n- Mercurial: Hex hash (40 chars)\n- Perforce: Changelist number (numeric)\n- Other: Alphanumeric string, 1-40 characters\n").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[A-Za-z0-9_-]{1,40}$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("COD-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("file_path", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner.").
			AutomationHooks("used for code navigation, coverage reports.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("file system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Path to the source file (relative to repo root or absolute).").
			Security("may reveal file structure—non-sensitive.").
			SystemUsage([]any{
				"traceability",
				"documentation",
			}).
			Validation("must be a valid file path.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("COD-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("function_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner.").
			AutomationHooks("used for function-level traceability.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("source file parsing.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Name of the function/method being referenced.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"documentation",
			}).
			Validation("function name pattern.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("COD-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner.").
			AutomationHooks("used for code-to-goal traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals this code reference supports.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
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
		WithProfileCode("COD-016"))
	b.AddFieldBuilder(builders.NewFieldBuilder("line_end", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner.").
			AutomationHooks("used for code navigation.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null (same as line_start if not provided)").
			Dependencies("source file.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Ending line number of the code reference (1-indexed, inclusive).").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"documentation",
			}).
			Validation("must be >= line_start if provided.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithProfileCode("COD-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("line_start", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner.").
			AutomationHooks("used for code navigation.").
			Cardinality("one").
			Criticality("association").
			Default("required").
			Dependencies("source file.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Starting line number of the code reference (1-indexed).").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"documentation",
			}).
			Validation("must be positive integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithProfileCode("COD-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("lines_added", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for code metrics.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("Version control repository (Git, SVN, Mercurial, Perforce, etc.).").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Number of lines added in this file change.").
			Security("non-sensitive").
			SystemUsage([]any{
				"metrics",
				"change analysis",
			}).
			Validation("must be non-negative integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("COD-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("lines_removed", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for code metrics.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("Version control repository (Git, SVN, Mercurial, Perforce, etc.).").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Number of lines removed in this file change.").
			Security("non-sensitive").
			SystemUsage([]any{
				"metrics",
				"change analysis",
			}).
			Validation("must be non-negative integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "modifiable", "readable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("COD-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("milestone_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner.").
			AutomationHooks("used for code-to-milestone traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("milestone registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestones this code reference supports.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
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
		WithProfileCode("COD-015"))
	b.AddFieldBuilder(builders.NewFieldBuilder("requirement_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for requirement-code traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("requirement registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Requirements this code reference implements.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
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
		WithProfileCode("COD-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("test_case_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for test-code traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("test_case registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Test cases that test this code reference.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("Must reference existing test_case IDs.").
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
		WithProfileCode("COD-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner.").
			AutomationHooks("used for code-to-workstream traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("workstream registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workstreams this code reference belongs to.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
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
		WithProfileCode("COD-017"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *CodeReferenceBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *CodeReferenceBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *CodeReferenceBuilder) GetOntology() string {
	return "code_reference"
}

func init() {
	builders.RegisterBuilder(NewCodeReferenceBuilder())
}
