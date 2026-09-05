package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// QuestionBuilder builds the question spec at version v2_0_0
// File: bldr_v2/question_builder.go - version is encoded in package/directory name
type QuestionBuilder struct {
	*builders.BaseSpecBuilder
}

// NewQuestionBuilder creates a new builder for question spec version v2_0_0
func NewQuestionBuilder() *QuestionBuilder {
	builder := &QuestionBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("question", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Questions represent unknowns, decisions that need to be made, or clarifications required before work can proceed. Questions have a distinct lifecycle focused on getting answers and tracking resolution, separate from feature work tracked in backlog items. Questions can block milestones, goals, and other work items.\\nLifecycle: question_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addQuestionFields()

	return builder
}

// addQuestionFields adds the question fields
func (b *QuestionBuilder) addQuestionFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("answer", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/answerer.").
			AutomationHooks("used for question resolution tracking, knowledge base.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable (can be updated as understanding evolves).").
			Observability("yes").
			Purpose("The answer to the question, once resolved.").
			Security("may contain sensitive information.").
			SystemUsage([]any{
				"display",
				"search",
				"documentation",
			}).
			Validation("Markdown allowed. Should be set when question is answered.").
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
		WithProfileCode("QST-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("answer_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/answerer.").
			AutomationHooks("used for traceability, linking answers to questions.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("object registry.").
			Lifecycle("mutable (set when question is answered).").
			Observability("yes").
			Purpose("Reference to the object that answers this question (e.g., decision ID, backlog item ID, document path).").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"linking",
			}).
			Validation("Should reference existing object ID or document path.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("QST-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("blocking_goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/product.").
			AutomationHooks("used for blocking analysis, goal dependency tracking.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals that are blocked by this unanswered question.").
			Security("non-sensitive").
			SystemUsage([]any{
				"blocking analysis",
				"dependency tracking",
			}).
			Validation("Must reference existing goal object IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("QST-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("blocking_milestone_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/product.").
			AutomationHooks("used for blocking analysis, milestone dependency tracking.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("milestone registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestones that are blocked by this unanswered question.").
			Security("non-sensitive").
			SystemUsage([]any{
				"blocking analysis",
				"dependency tracking",
			}).
			Validation("Must reference existing milestone object IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("QST-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("question_text", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creator/owner.").
			AutomationHooks("used for question search, filtering, reporting.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("none.").
			Lifecycle("mutable (can be refined/clarified).").
			Observability("yes").
			Purpose("The actual question being asked. Should be clear and specific.").
			Security("non-sensitive").
			SystemUsage([]any{
				"display",
				"search",
				"filtering",
			}).
			Validation("Required field. Should be a clear question statement.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("QST-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("related_question_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for question grouping, traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("question registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to related questions (e.g., follow-up questions, related unknowns).").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"question grouping",
			}).
			Validation("Must reference existing question object IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("QST-009"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *QuestionBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *QuestionBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *QuestionBuilder) GetOntology() string {
	return "question"
}

func init() {
	builders.RegisterBuilder(NewQuestionBuilder())
}
