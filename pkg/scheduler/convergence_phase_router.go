package scheduler

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Convergence phase values match convergence_session.current_phase enum (C1–C6).
type ConvergencePhase string

const (
	PhaseC1Scope  ConvergencePhase = "c1_scope"
	PhaseC2Triage ConvergencePhase = "c2_triage"
	PhaseC3Order  ConvergencePhase = "c3_order"
	PhaseC4Act    ConvergencePhase = "c4_act"
	PhaseC5Verify ConvergencePhase = "c5_verify"
	PhaseC6Exit   ConvergencePhase = "c6_exit"
)

var convergencePhaseOrder = []ConvergencePhase{
	PhaseC1Scope, PhaseC2Triage, PhaseC3Order, PhaseC4Act, PhaseC5Verify, PhaseC6Exit,
}

var convergencePhaseSet = func() map[ConvergencePhase]struct{} {
	m := make(map[ConvergencePhase]struct{}, len(convergencePhaseOrder))
	for _, p := range convergencePhaseOrder {
		m[p] = struct{}{}
	}
	return m
}()

// phaseRoutingProfile selects rule tables for measurement-implied phase and notes.
//
// phaseRoutingProfiles is an optional, repo-shipped hint table: it does not gate who may use
// convergence_session or which flow_variant strings are valid. Any non-empty flow_variant not listed
// here resolves to profile ID "custom" with neutral notes (same measurement rules as "default").
// Downstream teams add keys only when they want first-class agent-prompt copy without forking
// measurement math.
type phaseRoutingProfile struct {
	ID string
	// ExtraNotes are appended for operator context (e.g. fast-track hint).
	ExtraNotes func(snap *TestBundleConvergenceSnapshot) []string
}

var phaseRoutingProfiles = map[string]phaseRoutingProfile{
	"":               {ID: "default"},
	"default":        {ID: "default"},
	"scheduler_fast": {ID: "scheduler_fast", ExtraNotes: schedulerFastNotes},
	"coordinator_async": {
		ID: "coordinator_async",
		ExtraNotes: func(*TestBundleConvergenceSnapshot) []string {
			return []string{
				"Profile coordinator_async: heavy cross-checks may be queued to scheduler/coordinator workers; do not block CLI on full evaluation.",
			}
		},
	},
	// Code-quality sessions (convergence_session.flow_variant); same measurement-implied phase rules as default, distinct notes for prompts/ops.
	"code_quality_go_matrix": {
		ID: "code_quality_go_matrix",
		ExtraNotes: func(*TestBundleConvergenceSnapshot) []string {
			return []string{
				"Profile code_quality_go_matrix: matrix rows, FieldKey/ZQK-env repo gates, scan-tests bundles; use scripts/cvs_outcome_rollup.py for surfaces beyond CLI rollup.",
			}
		},
	},
	"code_quality_drift_and_standardization": {
		ID: "code_quality_drift_and_standardization",
		ExtraNotes: func(*TestBundleConvergenceSnapshot) []string {
			return []string{
				"Profile code_quality_drift_and_standardization: triage drift/hardcoded-literal baselines (map keys are not necessarily FieldKey debt); targeted scan-tests; agent-prompt + rollup; prefer AST fixers and path-scoped refactors.",
			}
		},
	},
	// Nested / matrix-only C6: CODEBASE_VETTING_MATRIX fully_vetted per VETTING_RUBRIC; bundles are secondary.
	"vetting_matrix_c6": {
		ID: "vetting_matrix_c6",
		ExtraNotes: func(*TestBundleConvergenceSnapshot) []string {
			return []string{
				"Profile vetting_matrix_c6: C6 human checklist for .go rows in CODEBASE_VETTING_MATRIX.csv; use scripts/cvs_outcome_rollup.py for pending_go_rows; test-bundle rollup alone does not prove matrix completion.",
			}
		},
	},
	// Docs + planning + scoped bundles (assessment artifacts, index, PRI/backlog linkage, archive cleanup).
	"expertise_docs_and_alpha_prep": {
		ID: "expertise_docs_and_alpha_prep",
		ExtraNotes: func(*TestBundleConvergenceSnapshot) []string {
			return []string{
				"Profile expertise_docs_and_alpha_prep: land docs under docs/architecture/ with a single index; link PRI/backlog; archive redundant reports to docs/archive/; use targeted zqk scheduler scan-tests so health.jsonl advances — full matrix only when blast radius warrants it.",
			}
		},
	},
	// Backlog-scoped delivery (tutorial/docs/hands-on); session thresholds may disable bundle-health as completion gate.
	"bli_documentation_delivery": {
		ID: "bli_documentation_delivery",
		ExtraNotes: func(*TestBundleConvergenceSnapshot) []string {
			return []string{
				"Profile bli_documentation_delivery: progress against BLI acceptance + activity_log / manual CLI verification; health.jsonl and rollup blockers are supporting signals only when thresholds.completion_gate.require_ready_for_session_completion is false.",
			}
		},
	},
	// Data cell program (registry, profile contracts, envelope, adapters, migration); REQ-DATACELL-001 / DATA_CELL_MODEL.
	"product_delivery_datacell": {
		ID: "product_delivery_datacell",
		ExtraNotes: func(*TestBundleConvergenceSnapshot) []string {
			return []string{
				"Profile product_delivery_datacell: phased BLIs for cell identity, profile contracts, operational envelope, membrane adapters, admin/discovery, migration; pkg/datacell + spec cell integration + targeted scan-tests; do not treat green bundles alone as proof of desired_end_state.",
			}
		},
	},
}

func schedulerFastNotes(snap *TestBundleConvergenceSnapshot) []string {
	if snap == nil {
		return nil
	}
	return []string{
		"Profile scheduler_fast: when remediating, prefer jumping to c4_act over lingering in c2/c3 unless ownership/order issues require those phases.",
	}
}

func resolvePhaseRoutingProfile(flowVariant string) phaseRoutingProfile {
	key := strings.TrimSpace(flowVariant)
	if key == emptyValue {
		key = "default"
	}
	if p, ok := phaseRoutingProfiles[key]; ok {
		return p
	}
	// Arbitrary flow_variant values are valid; built-in map entries are optional prompt hints only.
	return phaseRoutingProfile{
		ID: "custom",
		ExtraNotes: func(*TestBundleConvergenceSnapshot) []string {
			fv := strings.TrimSpace(flowVariant)
			return []string{fmt.Sprintf(
				"Custom flow_variant %q: no built-in prompt-hint registry entry in this binary; measurement-implied phase uses the same rules as the default profile. Define the workflow on the CVS (hypothesis, desired_end_state, context, iteration_process, thresholds, glossary_term_ref).",
				fv,
			)}
		},
	}
}

// PhaseRouterResult is structured output for convergence_session updates and CLI suggested fields.
// It is derived from TestBundleConvergenceSnapshot plus optional session context (current phase, flow variant).
type PhaseRouterResult struct {
	RoutingProfileID string `json:"routing_profile"`

	MeasurementImpliedPhase ConvergencePhase `json:"measurement_implied_phase"`
	// SuggestedCurrentPhase is what automation may write to current_phase (measurement authority).
	SuggestedCurrentPhase ConvergencePhase `json:"suggested_current_phase"`

	// CurrentPhaseEcho is set when the caller supplied a phase for alignment analysis.
	CurrentPhaseEcho string `json:"current_phase,omitempty"`

	// PhaseAlignment compares echoed current phase to measurement-implied: aligned | session_behind | session_ahead | unknown
	PhaseAlignment string `json:"phase_alignment,omitempty"`

	AllowedNextPhases []ConvergencePhase `json:"allowed_next_phases,omitempty"`
	Notes             []string           `json:"notes,omitempty"`

	// RequestAsyncEvaluation is true if the selected routing profile mandates that heavy cross-checks
	// or aggregations be enqueued to a coordinator-backed worker.
	RequestAsyncEvaluation bool `json:"request_async_evaluation,omitempty"`
}

// RouteConvergencePhase computes phase routing hints from a test-bundle health snapshot.
// currentPhase may be empty (measurement-only); flowVariant selects routing profile (convergence_session.flow_variant).
func RouteConvergencePhase(snap *TestBundleConvergenceSnapshot, currentPhase string, flowVariant string) *PhaseRouterResult {
	if snap == nil {
		return &PhaseRouterResult{
			RoutingProfileID:        resolvePhaseRoutingProfile(flowVariant).ID,
			MeasurementImpliedPhase: PhaseC1Scope,
			SuggestedCurrentPhase:   PhaseC1Scope,
			Notes:                   []string{"nil snapshot"},
		}
	}
	prof := resolvePhaseRoutingProfile(flowVariant)
	implied := measurementImpliedPhase(snap, prof)

	out := &PhaseRouterResult{
		RoutingProfileID:        prof.ID,
		MeasurementImpliedPhase: implied,
		SuggestedCurrentPhase:   implied,
		AllowedNextPhases:       allowedNextPhases(implied),
		RequestAsyncEvaluation:  prof.ID == "coordinator_async",
	}
	if prof.ExtraNotes != nil {
		out.Notes = append(out.Notes, prof.ExtraNotes(snap)...)
	}

	cp := strings.TrimSpace(currentPhase)
	if cp == emptyValue {
		return out
	}

	ph := ConvergencePhase(cp)
	if _, ok := convergencePhaseSet[ph]; !ok {
		out.CurrentPhaseEcho = cp
		out.PhaseAlignment = "unknown"
		out.Notes = append(out.Notes, fmt.Sprintf("current_phase %q is not a known convergence phase enum value", cp))
		return out
	}
	out.CurrentPhaseEcho = cp
	out.PhaseAlignment = phaseAlignment(ph, implied)
	return out
}

func measurementImpliedPhase(snap *TestBundleConvergenceSnapshot, _ phaseRoutingProfile) ConvergencePhase {
	switch snap.DeltaAssessment {
	case "unknown":
		return PhaseC1Scope
	case "trending_away":
		return PhaseC4Act
	case "trending_toward":
		return PhaseC5Verify
	case "neutral":
		if snap.ReadyForSessionCompletion {
			return PhaseC6Exit
		}
		return PhaseC5Verify
	default:
		return PhaseC1Scope
	}
}

func phaseAlignment(current, implied ConvergencePhase) string {
	ci := phaseRank(current)
	ii := phaseRank(implied)
	switch {
	case ci < 0 || ii < 0:
		return "unknown"
	case ci == ii:
		return "aligned"
	case ii > ci:
		return "session_behind"
	default:
		return "session_ahead"
	}
}

func phaseRank(p ConvergencePhase) int {
	for i, x := range convergencePhaseOrder {
		if x == p {
			return i
		}
	}
	return -1
}

// allowedNextPhases returns the single forward transition from the measurement-implied phase (bounded graph).
func allowedNextPhases(implied ConvergencePhase) []ConvergencePhase {
	i := phaseRank(implied)
	if i < 0 || implied == PhaseC6Exit {
		return nil
	}
	if i+1 >= len(convergencePhaseOrder) {
		return nil
	}
	return []ConvergencePhase{convergencePhaseOrder[i+1]}
}

// PhaseRouterResultAsMap encodes PhaseRouterResult as a generic map for embedding in object update payloads.
func PhaseRouterResultAsMap(r *PhaseRouterResult) (map[string]any, error) {
	if r == nil {
		return nil, nil
	}
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}
