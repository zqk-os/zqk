package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// PromptTemplateBuilder builds the prompt_template spec at version v2_0_0
// File: bldr_v2/prompt_template_builder.go - version is encoded in package/directory name
type PromptTemplateBuilder struct {
	*builders.BaseSpecBuilder
}

// NewPromptTemplateBuilder creates a new builder for prompt_template spec version v2_0_0
func NewPromptTemplateBuilder() *PromptTemplateBuilder {
	builder := &PromptTemplateBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("prompt_template", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Prompt templates are structured, reusable prompts that guide agents or humans to produce\\nright-sized requests. They sit on a spectrum from tactical (short, explicit, low ambiguity)\\nto strategic (broad, exploratory, subjective, influenced by market/seasonal/financial/political\\nfactors). The template ensures a clear line between open-ended creative possibilities and\\nstructural concerns (dependencies, risks, timelines, delivery) so the system can provide\\nskeleton and rigidity while leaving room for high-value, creative outcomes.\\nLifecycle: prompt_template_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable")

	// Add fields
	builder.addPromptTemplateFields()

	return builder
}

// addPromptTemplateFields adds the prompt_template fields
func (b *PromptTemplateBuilder) addPromptTemplateFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("category", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for filtering and grouping (e.g. \"establish_deliverable\", \"tactical_fix\", \"exploratory_planning\").").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("controlled vocabulary recommended.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Category for grouping prompt templates (establish_deliverable, tactical, strategic, reporting, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
			}).
			Validation("string; controlled vocabulary encouraged.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("doc_entry_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("links prompt template to user-facing docs (e.g. ESTABLISH_DELIVERABLE_OBJECTS_PROMPT.md).").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("doc_entry registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Doc entries that contain or expand this template (human-readable, reporting).").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"navigation",
			}).
			Validation("list of doc_entry IDs or paths.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("expected_outcome_kinds", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used to validate or scaffold created objects (e.g. establish-deliverable prompts expect milestones, goals, requirements, etc.).").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("object kind registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Object kinds the prompt is intended to produce or touch (e.g. milestone, goal, requirement, criteria, priority_plan, backlog_item, risk_blocker, doc_entry).").
			Security("non-sensitive").
			SystemUsage([]any{
				"prompt assembly",
				"outcome validation",
				"reporting",
			}).
			Validation("list of canonical kind names.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("links template to strategic goals (e.g. \"increase delivery velocity\", \"ensure governance adherence\").").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals this prompt template supports.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"filtering",
			}).
			Validation("list of goal IDs.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("mandatory_constraints", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/policy.").
			AutomationHooks("injected into generated prompt text; enforced in instructions.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("policy and lifecycle docs.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Constraint identifiers or short statements that must be reflected in the prompt\n(e.g. \\\"use CLI only for process data\\\", \\\"backlog items start with initial status\\\",\n\\\"no direct YAML edits under docs/process/\\\"). Ensures system guardrails are present.\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"prompt assembly",
				"compliance",
			}).
			Validation("list of strings (constraint refs or statements).").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("originator_questionnaire_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used to present questions to the originator before prompt is finalized.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("question or criteria registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to question or criteria objects that the originator should answer to right-size the prompt (scope, timeline, risks, stakeholders, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"prompt development",
				"right-sizing",
			}).
			Validation("list of question or criteria IDs.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("prompt_archetype", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/product.").
			AutomationHooks("used for filtering, default questionnaire, and right-sizing guidance.").
			Cardinality("one").
			Criticality("composition").
			Default("tactical").
			Dependencies("PROMPT_DEVELOPMENT_GUIDE archetype definitions.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Archetype that drives expected length, specificity, and rigidity.\n- tactical: Short, simple, explicit instructions; clear success criteria; minimal ambiguity.\n- structured: Medium length; defined outcomes (e.g. object kinds to create); lifecycle and linkage rules explicit.\n- strategic: Broad, exploratory; subjective; influenced by market/seasonal/financial/political factors; looser expectations.\n- hybrid: Combines structured skeleton (milestones, risks, timelines) with open-ended creative scope.\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"default questionnaire selection",
				"prompt assembly",
			}).
			Validation("enum.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"tactical",
				"structured",
				"strategic",
				"hybrid",
			}).
			Required(true).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("prompt_body", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used as template body; placeholders filled from questionnaire or context.").
			Cardinality("one").
			Criticality("composition").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The main prompt text or template with placeholders (e.g. {{deliverable}}, {{intent}}, {{constraints}}). Used when archetype and specificity are known.").
			Security("may contain confidential context.").
			SystemUsage([]any{
				"prompt assembly",
				"reuse",
			}).
			Validation("text; markdown allowed.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("risk_blocker_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("documents risks if template is misused or omitted (e.g. \"lifecycle violations\", \"scattered outcomes\").").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("risk_blocker registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Risks or blockers mitigated by using this template (or risks from not using it).").
			Security("non-sensitive").
			SystemUsage([]any{
				"risk awareness",
				"mitigation",
			}).
			Validation("list of risk_blocker IDs.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("specificity_level", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used with archetype to choose questionnaire and template body.").
			Cardinality("one").
			Criticality("association").
			Default("high").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("How prescriptive the prompt should be (high = explicit constraints and steps;\nmedium = outcome-focused with guardrails; low = open-ended with minimal guardrails).\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"prompt assembly",
				"validation",
			}).
			Validation("enum.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"high",
				"medium",
				"low",
			}).
			Required(false).
			Build()))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *PromptTemplateBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *PromptTemplateBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *PromptTemplateBuilder) GetOntology() string {
	return "prompt_template"
}

func init() {
	builders.RegisterBuilder(NewPromptTemplateBuilder())
}
