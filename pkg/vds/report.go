package vds

import (
	"fmt"
	"strings"
)

// ChecklistReport is the culture/orientation payload (no chunk evaluation).
type ChecklistReport struct {
	Schema                 string              `json:"schema"`
	PolicyID               string              `json:"policy_id"`
	GlossaryCanonicalTitle string              `json:"glossary_canonical_title"`
	GlossaryTermRef        string              `json:"glossary_term_ref,omitempty"`
	GlossaryAcronymRef     string              `json:"glossary_acronym_ref,omitempty"`
	GlossaryResolvedBy     string              `json:"glossary_resolved_by,omitempty"`
	Tone                   string              `json:"tone"`
	Culture                []string            `json:"culture"`
	Stages                 []StageChecklistRow `json:"stages"`
	ChunkContract          []string            `json:"chunk_contract"`
	HowToEvaluate          []string            `json:"how_to_evaluate"`
	Customization          map[string]any      `json:"customization_summary,omitempty"`
	AgentBrief             string              `json:"agent_brief"`
}

// StageChecklistRow is one stage in the orientation view.
type StageChecklistRow struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Purpose string `json:"purpose"`
	Ask     string `json:"ask"`
}

// BuildChecklist builds the orientation report.
// Pass glossary refs from ResolveGlossary (title lookup and/or customization pins).
func BuildChecklist(spine *SpineProfile, cust *Customization, gls GlossaryRefs) *ChecklistReport {
	rep := &ChecklistReport{
		Schema:                 SchemaChecklist,
		PolicyID:               PolicyID,
		GlossaryCanonicalTitle: GlossaryCanonicalTitle,
		GlossaryTermRef:        gls.TermRef,
		GlossaryAcronymRef:     gls.AcronymRef,
		GlossaryResolvedBy:     gls.ResolvedBy,
		Tone:                   "Gates first. Speed comes from small, honest chunks — not from skipping evidence.",
		Culture: []string{
			CultureGlossaryLine(gls),
			PolicyID + " applies to every project that uses this spine.",
			"Chat cannot mark work done.",
			"Prefer zqk workflow vds evaluate before claiming stage advance.",
		},
		ChunkContract: []string{
			"chunk_id", "stage", "claim", "rubric_ref", "dsl_checks", "evidence_refs", "independent_verify",
		},
		HowToEvaluate: []string{
			"zqk workflow vds init                    # scaffold chunks file if missing",
			"zqk workflow vds checklist --format json # stages + culture (this command)",
			"zqk workflow vds evaluate --file docs/quality/vds_chunks.yaml --format json",
			"zqk workflow vds evaluate --format agent-prompt   # pasteable brief",
			"zqk workflow vds project --provider <id> --write|--check  # kernel → vendor export (config)",
			"zqk workflow vds project --list   # configured vendor_providers",
			"Add --run-commands only when you intend to execute lint/scan from customization",
		},
	}
	asks := map[string]string{
		"intent_capture":   "Is vision/mission/goal captured as durable objects with evidence?",
		"design":           "Do requirements/criteria/tests/backlog chunks each have a rubric?",
		"implement":        "Does each change have lint/test evidence per project customization?",
		"integrate_verify": "Is there smoke/integration/CI footprint — not just unit green?",
		"operate_release":  "Are security/perf/publish gates considered or explicitly na?",
	}
	if spine != nil {
		for _, s := range spine.Stages {
			rep.Stages = append(rep.Stages, StageChecklistRow{
				ID: s.ID, Label: s.Label, Purpose: s.Purpose, Ask: asks[s.ID],
			})
		}
	} else {
		for _, id := range StageIDs {
			rep.Stages = append(rep.Stages, StageChecklistRow{ID: id, Ask: asks[id]})
		}
	}
	if cust != nil {
		rep.Customization = map[string]any{
			"project_id":            cust.ProjectID,
			"test_execution_mode":   cust.TestExecution.Mode,
			"process_data_mutation": cust.ProcessData.Mutation,
			"lint_commands":         cust.CodeStyle.LintCommands,
		}
	}
	rep.AgentBrief = RenderChecklistBrief(rep)
	return rep
}

// RenderAgentBrief is a short markdown agents can respect in one glance.
func RenderAgentBrief(r *Report) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# VDS %s (%s)\n\n", r.Verdict, r.PolicyID)
	fmt.Fprintf(&b, "%s\n\n", r.Tone)
	for _, c := range r.Culture {
		fmt.Fprintf(&b, "- %s\n", c)
	}
	b.WriteString("\n## Chunks\n")
	for _, ch := range r.Chunks {
		fmt.Fprintf(&b, "\n### `%s` — %s — **%s**\n", ch.ChunkID, ch.Stage, ch.Verdict)
		if ch.Claim != "" {
			fmt.Fprintf(&b, "%s\n", ch.Claim)
		}
		b.WriteString("\n| check | ok | detail |\n|---|---|---|\n")
		for _, item := range ch.Checklist {
			mark := "FAIL"
			if item.OK {
				mark = "ok"
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n", item.ID, mark, escapeCell(item.Detail))
		}
		if len(ch.NextActions) > 0 {
			b.WriteString("\nNext:\n")
			for _, a := range ch.NextActions {
				fmt.Fprintf(&b, "- %s\n", a)
			}
		}
	}
	if len(r.NextActions) > 0 && r.Verdict == "FAIL" {
		b.WriteString("\n## Global next\n")
		for _, a := range r.NextActions {
			fmt.Fprintf(&b, "- %s\n", a)
		}
	}
	b.WriteString("\n```\nzqk workflow vds evaluate --format json\n```\n")
	return b.String()
}

// RenderChecklistBrief renders orientation markdown.
func RenderChecklistBrief(r *ChecklistReport) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# VDS checklist (%s)\n\n%s\n\n", r.PolicyID, r.Tone)
	for _, c := range r.Culture {
		fmt.Fprintf(&b, "- %s\n", c)
	}
	b.WriteString("\n## Stages\n")
	for i, s := range r.Stages {
		label := s.Label
		if label == "" {
			label = s.ID
		}
		fmt.Fprintf(&b, "%d. **%s** (`%s`) — %s\n   - Ask: %s\n", i+1, label, s.ID, s.Purpose, s.Ask)
	}
	b.WriteString("\n## Chunk contract\n")
	fmt.Fprintf(&b, "`%s`\n\n", strings.Join(r.ChunkContract, "`, `"))
	b.WriteString("## Commands\n")
	for _, h := range r.HowToEvaluate {
		fmt.Fprintf(&b, "- `%s`\n", h)
	}
	return b.String()
}

func escapeCell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "/"), "\n", " ")
}
