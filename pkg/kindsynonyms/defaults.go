package kindsynonyms

import "github.com/lanceman/zqk/pkg/kindnames"

var (
	aliasesBacklogItem = []string{"task", "tasks", "item", "items", "backlog", "story", "stories", "work_item"}
	aliasesRequirement = []string{"req", "reqs", "spec", "specs", "specification"}
	aliasesGoal        = []string{"objective", "objectives", "aim", "target"}
	aliasesMilestone   = []string{"milestones", "checkpoint", "checkpoints", "phase"}
	aliasesComponent   = []string{"comp", "components", "module", "modules", "part"}
	aliasesCriteria    = []string{"criterion", "acceptance_criteria", "acceptance", "test_criteria"}
	aliasesTestCase    = []string{"test", "tests", "testcase", "testcases", "tc"}
	// Do not alias "roadmap" to priority_plan: roadmap is a distinct object kind (ontology roadmap).
	aliasesPriority   = []string{"plan", "priority", "priorities"}
	aliasesWorkstream = []string{"stream", "streams", "work_stream", "initiative"}
	// prompt_template: avoid "template" — conflicts with kindnames.Template (CAS template objects).
	aliasesPromptTemplate = []string{"prompt", "prompts", "prompt_tpl"}
)

// KindAliasEntry defines a canonical kind and its accepted synonyms.
// These are used as baseline defaults so CLI workflows stay consistent.
type KindAliasEntry struct {
	Kind    string
	Aliases []string
}

// DefaultKindAliasEntries returns deterministic default kind aliases.
// NOTE: Aliases intentionally exclude the canonical kind itself.
func DefaultKindAliasEntries() []KindAliasEntry {
	return []KindAliasEntry{
		{
			Kind:    kindnames.BacklogItem,
			Aliases: aliasesBacklogItem,
		},
		{
			Kind:    kindnames.Requirement,
			Aliases: aliasesRequirement,
		},
		{
			Kind:    kindnames.Goal,
			Aliases: aliasesGoal,
		},
		{
			Kind:    kindnames.Milestone,
			Aliases: aliasesMilestone,
		},
		{
			Kind:    kindnames.Component,
			Aliases: aliasesComponent,
		},
		{
			Kind:    kindnames.Criteria,
			Aliases: aliasesCriteria,
		},
		{
			Kind:    kindnames.TestCase,
			Aliases: aliasesTestCase,
		},
		{
			Kind:    kindnames.PriorityPlan,
			Aliases: aliasesPriority,
		},
		{
			Kind:    kindnames.Workstream,
			Aliases: aliasesWorkstream,
		},
		{
			Kind:    kindnames.PromptTemplate,
			Aliases: aliasesPromptTemplate,
		},
	}
}

// DefaultKindAliasesForKind returns the default aliases for a canonical kind.
// Aliases intentionally exclude the canonical kind itself.
func DefaultKindAliasesForKind(kind string) []string {
	for _, e := range DefaultKindAliasEntries() {
		if e.Kind == kind {
			return append([]string(nil), e.Aliases...)
		}
	}
	return []string{}
}
