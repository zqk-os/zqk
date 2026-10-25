// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package kindsynonyms

import "github.com/zqk-os/zqk/pkg/kindnames"

var (
	aliasesBacklogItem = []string{"task", "tasks", "item", "items", "backlog", "story", "stories", "work_item", "bli", "blis"}
	aliasesRequirement = []string{"req", "reqs", "spec", "specs", "specification"}
	aliasesGoal        = []string{"objective", "objectives", "aim", "target", "goals"}
	aliasesMilestone   = []string{"milestones", "checkpoint", "checkpoints", "phase", "mil", "mils"}
	aliasesComponent   = []string{"comp", "components", "module", "modules", "part"}
	aliasesCriteria    = []string{"criterion", "acceptance_criteria", "acceptance", "test_criteria", "crit", "crits"}
	// ADRs are the same ontology as decision (ADR- vs DEC- id prefixes), not extra kinds.
	aliasesDecision = []string{"adr", "arch_decision_record", "architectural_decision"}
	aliasesTestCase = []string{"test", "tests", "testcase", "testcases", "tc", "tcs", "tst", "tsts"}
	// Do not alias "roadmap" to priority_plan: roadmap is a distinct object kind (ontology roadmap).
	aliasesPriority   = []string{"plan", "priority", "priorities", "pplan", "pplans", "pri", "pris"}
	aliasesWorkstream = []string{"stream", "streams", "work_stream", "initiative"}
	// prompt_template: avoid "template" — conflicts with kindnames.Template (CAS template objects).
	aliasesPromptTemplate = []string{"prompt", "prompts", "prompt_tpl"}
	// Object CLI shortcut groups (object splan/pplan/…) must also resolve as list/count kinds.
	// TRACK: follow-up in kernel backlog
	aliasesStrategicPlan        = []string{"splan", "strat_plan", "strategic"}
	aliasesWorkstreamTransition = []string{"wstrans", "ws_trans"}
	aliasesEvolutionManagement  = []string{"evoman", "evo_man"}
	aliasesEpic                 = []string{"epics"}
	aliasesChangeJournalEntry   = []string{"journal", "change_journal", "journal_entry"}
	aliasesPolicy               = []string{"policies"}
	aliasesVocabularyScheme     = []string{"vocab", "vocabulary"}
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
			Kind:    kindnames.Decision,
			Aliases: aliasesDecision,
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
		{
			Kind:    kindnames.StrategicPlan,
			Aliases: aliasesStrategicPlan,
		},
		{
			Kind:    kindnames.WorkstreamTransition,
			Aliases: aliasesWorkstreamTransition,
		},
		{
			Kind:    kindnames.EvolutionManagement,
			Aliases: aliasesEvolutionManagement,
		},
		{
			Kind:    kindnames.Epic,
			Aliases: aliasesEpic,
		},
		{
			Kind:    kindnames.ChangeJournalEntry,
			Aliases: aliasesChangeJournalEntry,
		},
		{
			Kind:    kindnames.Policy,
			Aliases: aliasesPolicy,
		},
		{
			Kind:    kindnames.VocabularyScheme,
			Aliases: aliasesVocabularyScheme,
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
