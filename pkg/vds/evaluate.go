package vds

import (
	"context"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
)

// CheckItem is one fluent checklist row agents can scan quickly.
type CheckItem struct {
	ID     string `json:"id"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// ChunkResult is evaluation for one chunk.
type ChunkResult struct {
	ChunkID      string            `json:"chunk_id"`
	Stage        string            `json:"stage"`
	Claim        string            `json:"claim"`
	Verdict      string            `json:"verdict"` // PASS | FAIL | WAIVED
	Checklist    []CheckItem       `json:"checklist"`
	Predicates   []PredicateResult `json:"predicates"`
	NextActions  []string          `json:"next_actions,omitempty"`
	WaiverRef    string            `json:"waiver_ref,omitempty"`
	RubricRef    string            `json:"rubric_ref,omitempty"`
	EvidenceRefs []string          `json:"evidence_refs,omitempty"`
}

// Report is the fluent agent-facing evaluate payload.
type Report struct {
	Schema                 string        `json:"schema"`
	PolicyID               string        `json:"policy_id"`
	GlossaryCanonicalTitle string        `json:"glossary_canonical_title"`
	GlossaryTermRef        string        `json:"glossary_term_ref,omitempty"`
	GlossaryAcronymRef     string        `json:"glossary_acronym_ref,omitempty"`
	GlossaryResolvedBy     string        `json:"glossary_resolved_by,omitempty"`
	Verdict                string        `json:"verdict"` // PASS | FAIL | WAIVED
	Tone                   string        `json:"tone"`
	Culture                []string      `json:"culture"`
	SpineProfileID         string        `json:"spine_profile_id,omitempty"`
	CustomizationID        string        `json:"customization_profile_id,omitempty"`
	TestMode               string        `json:"test_execution_mode,omitempty"`
	Chunks                 []ChunkResult `json:"chunks"`
	FailedChunkIDs         []string      `json:"failed_chunk_ids,omitempty"`
	NextActions            []string      `json:"next_actions,omitempty"`
	AgentBrief             string        `json:"agent_brief"`
	RunCommands            bool          `json:"run_commands"`
}

// Evaluate runs VDS gates over chunks.
func Evaluate(ctx context.Context, chunks []Chunk, spine *SpineProfile, cust *Customization, opt EvalOptions) *Report {
	if cust != nil {
		opt.Customization = cust
	}
	gls := ResolveGlossary(ctx, cust, opt.TitleLookup)
	rep := &Report{
		Schema:                 SchemaEvaluate,
		PolicyID:               PolicyID,
		GlossaryCanonicalTitle: GlossaryCanonicalTitle,
		GlossaryTermRef:        gls.TermRef,
		GlossaryAcronymRef:     gls.AcronymRef,
		GlossaryResolvedBy:     gls.ResolvedBy,
		Tone:                   "Done means re-runnable evidence. Narrative is never a gate.",
		Culture: []string{
			CultureGlossaryLine(gls),
			"Name stage + chunk_id before claiming progress.",
			"Every done claim needs rubric_ref, dsl_checks, evidence_refs, independent_verify.",
			"Customization changes how checks run — never whether gates exist.",
			"Waivers are human-authored objects only; agents must not self-waive.",
		},
		RunCommands: opt.RunCommands,
	}
	if spine != nil {
		rep.SpineProfileID = spine.ProfileID
	}
	if cust != nil {
		rep.CustomizationID = cust.ProfileID
		rep.TestMode = cust.TestExecution.Mode
	}

	doneVals := []string{"yes", "na"}
	if spine != nil && len(spine.Completion.DoneValues) > 0 {
		doneVals = spine.Completion.DoneValues
	}

	allPass := true
	anyWaived := false
	var globalNext []string

	for _, ch := range chunks {
		cr := evaluateChunk(ctx, ch, spine, doneVals, opt)
		rep.Chunks = append(rep.Chunks, cr)
		switch cr.Verdict {
		case "PASS":
			// ok
		case "WAIVED":
			anyWaived = true
		default:
			allPass = false
			rep.FailedChunkIDs = append(rep.FailedChunkIDs, cr.ChunkID)
			globalNext = append(globalNext, cr.NextActions...)
		}
	}

	switch {
	case allPass && anyWaived:
		rep.Verdict = "WAIVED"
	case allPass:
		rep.Verdict = "PASS"
	default:
		rep.Verdict = "FAIL"
	}
	rep.NextActions = uniqStrings(globalNext)
	rep.AgentBrief = RenderAgentBrief(rep)
	return rep
}

func evaluateChunk(ctx context.Context, ch Chunk, spine *SpineProfile, doneVals []string, opt EvalOptions) ChunkResult {
	ch.DSLChecks = NormalizeDSLChecks(ch.DSLChecks)
	cr := ChunkResult{
		ChunkID:      ch.ChunkID,
		Stage:        ch.Stage,
		Claim:        ch.Claim,
		RubricRef:    ch.RubricRef,
		EvidenceRefs: ch.EvidenceRefs,
		WaiverRef:    ch.WaiverRef,
	}

	var checks []CheckItem
	pass := true
	fail := func(id, detail string) {
		pass = false
		checks = append(checks, CheckItem{ID: id, OK: false, Detail: detail})
	}
	ok := func(id, detail string) {
		checks = append(checks, CheckItem{ID: id, OK: true, Detail: detail})
	}

	if strings.TrimSpace(ch.ChunkID) == "" {
		fail("chunk_id", "required")
	} else {
		ok("chunk_id", ch.ChunkID)
	}
	if !KnownStage(ch.Stage) {
		fail("stage", "must be one of: "+strings.Join(StageIDs, ", "))
	} else {
		ok("stage", ch.Stage)
	}
	if strings.TrimSpace(ch.Claim) == "" {
		fail("claim", "required — one independently verifiable sentence")
	} else {
		ok("claim", truncate(ch.Claim, 80))
	}
	if strings.TrimSpace(ch.RubricRef) == "" {
		fail("rubric_ref", "required — criteria id, matrix row, or rubric anchor")
	} else {
		ok("rubric_ref", ch.RubricRef)
	}
	if len(ch.DSLChecks) == 0 {
		fail("dsl_checks", "required — at least one machine predicate")
	} else {
		ok("dsl_checks", strings.Join(ch.DSLChecks, "; "))
	}
	if len(ch.EvidenceRefs) == 0 {
		fail("evidence_refs", "required — re-runnable footprints (ids, job ids, log paths)")
	} else {
		ok("evidence_refs", strings.Join(ch.EvidenceRefs, "; "))
	}

	// Stage gate cell (yes/na)
	gateOK, gateDetail := stageGateOK(ch, doneVals)
	if !gateOK {
		fail("stage_gate", gateDetail)
	} else {
		ok("stage_gate", gateDetail)
	}

	waiver := strings.TrimSpace(ch.WaiverRef) != ""
	iv := strings.ToLower(strings.TrimSpace(ch.IndependentVerify))

	// Predicates — this command is the independent verifier (fluent re-run).
	predPass := true
	for _, p := range ch.DSLChecks {
		pr := EvalPredicate(ctx, p, ch, opt)
		cr.Predicates = append(cr.Predicates, pr)
		if pr.Skipped && !pr.OK {
			if !waiver {
				predPass = false
			}
		} else if !pr.OK {
			predPass = false
		}
	}
	if len(ch.DSLChecks) > 0 {
		if predPass || waiver {
			ok("predicates", "all dsl_checks passed (or waived)")
		} else {
			fail("predicates", "one or more dsl_checks failed — see predicates[]")
		}
	}

	switch {
	case waiver:
		ok("independent_verify", "human waiver_ref="+ch.WaiverRef)
	case !predPass:
		fail("independent_verify", "cannot verify while dsl_checks fail")
	case iv == "yes":
		ok("independent_verify", "yes — confirmed by this evaluate run")
	case iv == "" || iv == "pending" || iv == "no":
		// Evaluate run itself satisfies independent verification when predicates pass.
		ok("independent_verify", "verified_by_this_run — persist independent_verify=yes on the chunk")
	default:
		fail("independent_verify", "invalid value "+iv+" (use yes|no|pending or waiver_ref)")
	}

	if !pass || (!predPass && !waiver) {
		cr.Verdict = "FAIL"
		cr.NextActions = suggestNext(ch, cr.Predicates, opt)
	} else if waiver {
		cr.Verdict = "WAIVED"
	} else {
		cr.Verdict = "PASS"
		if iv != "yes" {
			cr.NextActions = append(cr.NextActions, "Persist independent_verify=yes for chunk "+ch.ChunkID)
		}
	}
	_ = spine // reserved for future contract validation
	cr.Checklist = checks
	return cr
}

func stageGateOK(ch Chunk, doneVals []string) (bool, string) {
	var cell string
	switch ch.Stage {
	case "intent_capture":
		cell = ch.GateIntent
	case "design":
		cell = ch.GateDesign
	case "implement":
		cell = ch.GateImplement
	case "integrate_verify":
		cell = ch.GateIntegrate
	case "operate_release":
		cell = ch.GateOperate
	default:
		return false, "unknown stage"
	}
	if strings.TrimSpace(cell) == "" {
		// Allow omit if independent_verify path is used — still recommend setting gate_*
		return true, "gate_* omitted (allowed); prefer setting stage gate to yes"
	}
	if IsDoneValue(cell, doneVals) {
		return true, "gate=" + strings.TrimSpace(cell)
	}
	return false, "stage gate must be yes|na, got " + cell
}

func suggestNext(ch Chunk, preds []PredicateResult, opt EvalOptions) []string {
	var out []string
	if strings.TrimSpace(ch.ChunkID) == "" {
		out = append(out, "Set chunk_id")
	}
	if !KnownStage(ch.Stage) {
		out = append(out, "Set stage to one of "+strings.Join(StageIDs, "|"))
	}
	for _, p := range preds {
		if !p.OK {
			out = append(out, "Fix "+p.Predicate+": "+p.Detail)
		}
	}
	if opt.Customization != nil && strings.EqualFold(opt.Customization.TestExecution.Mode, "scheduler") {
		ex := opt.Customization.TestExecution.SchedulerSubmitExample
		if ex != "" {
			out = append(out, "For tests: "+ex+" then put job id + log path in evidence_refs")
		}
	}
	out = append(out, "Re-run: "+paths.CLIUsage("workflow", "vds", "evaluate", "--file", "<chunks.yaml>", "--format", "json"))
	return uniqStrings(out)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func uniqStrings(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// Passed reports whether the report is an acceptable done signal.
func (r *Report) Passed() bool {
	return r != nil && (r.Verdict == "PASS" || r.Verdict == "WAIVED")
}

// PassedChunkIDs returns chunk ids with verdict PASS or WAIVED (for --apply-verify).
func (r *Report) PassedChunkIDs() []string {
	if r == nil {
		return nil
	}
	var out []string
	for _, c := range r.Chunks {
		if c.Verdict == "PASS" || c.Verdict == "WAIVED" {
			out = append(out, c.ChunkID)
		}
	}
	return out
}
