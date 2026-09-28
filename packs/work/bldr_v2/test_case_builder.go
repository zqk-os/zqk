package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// TestCaseBuilder builds the test_case spec at version v2_0_0
// File: bldr_v2/test_case_builder.go - version is encoded in package/directory name
type TestCaseBuilder struct {
	*builders.BaseSpecBuilder
}

// NewTestCaseBuilder creates a new builder for test_case spec version v2_0_0
func NewTestCaseBuilder() *TestCaseBuilder {
	builder := &TestCaseBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("test_case", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("work_interval").
		SetDescription("Represents an automated or manual test linked to requirements/workstreams.\\nLifecycle: test_case_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("completable")

	// Add fields
	builder.addTestCaseFields()

	return builder
}

// addTestCaseFields adds the test_case fields
func (b *TestCaseBuilder) addTestCaseFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("backlog_item_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("QA/owner.").
			AutomationHooks("used for backlog item coverage tracking.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("backlog registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Backlog items this test case validates.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"coverage",
			}).
			Validation("must reference existing backlog item IDs.").
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
		WithProfileCode("TST-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("criteria_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("QA/owner.").
			AutomationHooks("used for criteria validation.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("criteria registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Criteria this test case validates.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"validation",
			}).
			Validation("must reference existing criteria IDs.").
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
		WithProfileCode("TST-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("milestone_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("QA/owner.").
			AutomationHooks("used for milestone validation.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("milestone registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestones this test case validates.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"validation",
			}).
			Validation("must reference existing milestone IDs.").
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
		WithProfileCode("TST-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("path_or_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("QA/owner.").
			AutomationHooks("used to execute the test.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("test runner.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("File path, test identifier, or command to run the test.").
			Security("non-sensitive").
			SystemUsage([]any{
				"automation",
				"documentation",
			}).
			Validation("Free-form string.").
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
		WithProfileCode("TST-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("priority", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("QA/owner.").
			AutomationHooks("used for test execution prioritization.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("test runner.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Priority level (critical, high, medium, low) for test execution ordering.").
			Security("non-sensitive").
			SystemUsage([]any{
				"prioritization",
				"test execution ordering",
			}).
			Validation("enum (critical, high, medium, low).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"critical",
				"high",
				"medium",
				"low",
			}).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TST-000"))
	b.AddFieldBuilder(builders.NewFieldBuilder("requirement_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("QA/owner.").
			AutomationHooks("used for requirement coverage tracking.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("requirement registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Requirements this test case validates.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"coverage",
			}).
			Validation("must reference existing requirement IDs.").
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
		WithProfileCode("TST-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("scope", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("QA/owner.").
			AutomationHooks("determines runner to use.").
			Cardinality("one").
			Criticality("composition").
			Default("unit").
			Dependencies("test runner.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Test category (unit, integration, e2e, manual, scenario).").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"coverage",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"unit",
				"integration",
				"e2e",
				"manual",
				"scenario",
			}).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TST-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("verification_suites", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("QA/owner.").
			AutomationHooks("used for QA daemon verification.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of antagonistic test suites to run before cryptographic signing.").
			Security("non-sensitive").
			SystemUsage([]any{
				"verification",
			}).
			Validation("list of strings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TST-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("QA/owner.").
			AutomationHooks("used for workstream test coverage.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("workstream registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workstreams this test case belongs to.").
			Security("non-sensitive").
			SystemUsage([]any{
				"grouping",
				"reporting",
			}).
			Validation("must reference existing workstream IDs.").
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
		WithProfileCode("TST-006"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *TestCaseBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *TestCaseBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *TestCaseBuilder) GetOntology() string {
	return "test_case"
}

func init() {
	builders.RegisterBuilder(NewTestCaseBuilder())
}
