package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// VerificationMatrixBuilder builds the verification_matrix spec at version v2_0_0
// File: bldr_v2/verification_matrix_builder.go - version is encoded in package/directory name
type VerificationMatrixBuilder struct {
	*builders.BaseSpecBuilder
}

// NewVerificationMatrixBuilder creates a new builder for verification_matrix spec version v2_0_0
func NewVerificationMatrixBuilder() *VerificationMatrixBuilder {
	builder := &VerificationMatrixBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("verification_matrix", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("A registered verification or traceability matrix: links CSV inventories and profile YAML to\\nconvergence sessions, goals, milestones, and roadmaps. Use for codebase vetting rows, test-bundle\\nhealth, goal-progress rollups, and future transition gates (e.g. roadmap complete only when\\nlinked milestones and criteria satisfy a configured matrix). Agents and automation resolve the\\nmatrix by id; CLI uses docs/quality/matrix_registry.yaml for default paths unless overrides are set.\\nLifecycle: verification_matrix_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable")

	// Add fields
	builder.addVerificationMatrixFields()

	return builder
}

// addVerificationMatrixFields adds the verification_matrix fields
func (b *VerificationMatrixBuilder) addVerificationMatrixFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("csv_path_override", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("matrix report, future get/update.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("filesystem.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional repo-relative path overriding registry CSV for this object instance.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("VMX-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("gate_policy_notes", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("future evaluator (JSON or rubric text).").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Human- and machine-readable notes for gate rules (which rows must be yes/na, criteria links).\nMay reference CRIT- ids or backlog items.\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"gating",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("VMX-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("gated_object_kind", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("future transition gate evaluator.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("object kinds registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("When matrix_role is transition_gate, the object kind whose transitions are gated\n(e.g. roadmap, milestone). Empty when not used as a gate.\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"gating",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("VMX-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("gated_transition_to", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("future gate engine.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("target kind lifecycle.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Target status value that requires matrix completion (e.g. complete).").
			Security("non-sensitive").
			SystemUsage([]any{
				"gating",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("VMX-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("linked_goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("goal progress matrices, reporting.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals whose achievement or progress this matrix tracks or gates.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
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
		WithProfileCode("VMX-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("linked_milestone_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("milestone completion vs matrix rows.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("milestone registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestones tied to this verification surface (e.g. all complete before roadmap closes).").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
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
		WithProfileCode("VMX-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("linked_roadmap_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("roadmap completion gates.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("roadmap registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Roadmaps whose completion may depend on matrix fullness.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
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
		WithProfileCode("VMX-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("matrix_role", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("routing to evaluators, report commands, future gate engine.").
			Cardinality("one").
			Criticality("composition").
			Default("custom").
			Dependencies("matrix_registry.yaml, profile YAML.").
			Lifecycle("mutable (with care).").
			Observability("yes").
			Purpose("Kind of matrix: codebase file vetting, test bundles, goal progress rollup, transition_gate\n(block object status until rows complete), or agent_coordination (cross-agent punch list).\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"gating",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"custom",
				"codebase_vetting",
				"test_bundle",
				"goal_progress",
				"transition_gate",
				"agent_coordination",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("VMX-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("planning_notes", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("agent handoff, viability review.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Forward-looking context: unknowns, complexity, cross-cutting constraints, agent specialization\nslices. Complements gate_policy_notes with qualitative planning.\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("VMX-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("primary_convergence_session_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("activity_log, convergence reporting.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("convergence_session objects.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Primary CVS id (CVS-…) for traceability when updating matrix rows or notes.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_reference_group", "writable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("VMX-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("profile_path_override", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("matrix report column semantics.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("profile YAML.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional repo-relative path overriding registry profile.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("VMX-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("registry_alias", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks(paths.CLIInvocation("matrix report --name <alias>.")).
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("docs/quality/matrix_registry.yaml.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Key in matrix_registry.yaml when csv/profile overrides are not used.").
			Security("non-sensitive").
			SystemUsage([]any{
				"CLI resolution",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("VMX-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("lifecycle.").
			AutomationHooks("lifecycle transitions.").
			Cardinality("one").
			Criticality("composition").
			Default("draft").
			Dependencies("verification_matrix_lifecycle.yaml.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Lifecycle status (draft, active, archived).").
			Security("non-sensitive").
			SystemUsage([]any{
				"lifecycle enforcement",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"draft",
				"active",
				"archived",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("VMX-002"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *VerificationMatrixBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *VerificationMatrixBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *VerificationMatrixBuilder) GetOntology() string {
	return "verification_matrix"
}

func init() {
	builders.RegisterBuilder(NewVerificationMatrixBuilder())
}
