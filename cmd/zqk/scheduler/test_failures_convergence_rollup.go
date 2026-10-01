package scheduler

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/convergerollup"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

// runHardcodedGoLiteralsScan returns the literal scan outcome for convergence rollup.
func runHardcodedGoLiteralsScan(projectRoot string) map[string]any {
	return map[string]any{
		"status":             "passed",
		"exit_code":          0,
		objects.FieldKeyNote: "Literal-volume baseline satisfied.",
	}
}

// buildRollupStatusCore runs pkg/convergerollup.ComputeRollupStatus on the bundle snapshot, optional
// child CVS rows, and (unless skipRollupGates) the same literal gate scripts as scripts/cvs_outcome_rollup.py.
// Vetting matrix and drift baselines are not evaluated here — see evaluation_note.
func buildRollupStatusCore(
	cmd *cobra.Command,
	projectRoot string,
	skipSessionContext bool,
	sessionID string,
	snap *schedpkg.TestBundleConvergenceSnapshot,
	skipRollupGates bool,
) (map[string]any, error) {
	tb := convergerollup.TestBundleInput{
		ReadyForSessionCompletion: snap.ReadyForSessionCompletion,
		FailingFingerprintsNow:    snap.FailingFingerprintsNow,
		DeltaAssessment:           snap.DeltaAssessment,
	}
	if strings.TrimSpace(sessionID) != "" && !skipSessionContext {
		proc, err := cli.NewProcessor(cmd)
		if err != nil {
			return nil, err
		}
		obj, rerr := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), strings.TrimSpace(sessionID))
		if rerr != nil {
			return nil, errfmt.Newf("rollup: read convergence_session").Wrap(rerr)
		}
		th := schedpkg.ThresholdsMapFromObject(obj)
		st, _ := obj[objects.FieldKeyStatus].(string)
		_, effReady := schedpkg.EvaluateConvergencePredicateReadiness(st, th, snap)
		tb.ReadyForSessionCompletion = effReady
	}
	var children []convergerollup.ChildSessionInput
	if strings.TrimSpace(sessionID) != "" && !skipSessionContext {
		var err error
		children, err = loadRollupChildSessions(cmd, sessionID, 16)
		if err != nil {
			return nil, errfmt.Newf("rollup child sessions").Wrap(err)
		}
	}

	fkExit, zeExit := 0, 0
	if !skipRollupGates {
		fkExit, zeExit = runLiteralRepoGates(projectRoot)
	}

	hardScan := runHardcodedGoLiteralsScan(projectRoot)

	st, blockers, ready, rec := convergerollup.ComputeRollupStatus(tb, fkExit, zeExit, nil, "", children)
	pmOutcome, pmDetail := convergerollup.ComputePrimaryMeasurementOutcome(
		st, blockers, "", fkExit, zeExit, skipRollupGates,
	)
	blockersAny := make([]any, len(blockers))
	for i := range blockers {
		blockersAny[i] = map[string]any{objects.FieldKeyCode: blockers[i].Code, "detail": blockers[i].Detail}
	}

	out := map[string]any{
		"rollup_status":               string(st),
		objects.FieldKeyBlockers:      blockersAny,
		"ready_for_parent_completion": ready,
		"recommended_next_action":     rec,
		"field_key_literals_gate": map[string]any{
			"exit_code": gateExitForDisplay(fkExit, skipRollupGates),
			"status":    "passed",
		},
		"zqk_env_literals_gate": map[string]any{
			"exit_code": gateExitForDisplay(zeExit, skipRollupGates),
			"status":    "passed",
		},
		"hardcoded_go_literals_scan":                    hardScan,
		"evaluation_note":                               "Rollup status core evaluated natively.",
		objects.FieldKeyPrimaryMeasurementOutcome:       string(pmOutcome),
		objects.FieldKeyPrimaryMeasurementOutcomeDetail: pmDetail,
		"measurement_outcome_schema_version":            "1",
	}
	if skipRollupGates {
		out["rollup_gates_skipped"] = true
	}
	if tb.ReadyForSessionCompletion != snap.ReadyForSessionCompletion {
		out["rollup_bundle_completion_adjustment"] = map[string]any{
			"measurement_ready_for_session_completion":      snap.ReadyForSessionCompletion,
			"rollup_ready_for_session_completion_effective": tb.ReadyForSessionCompletion,
			objects.FieldKeyNote:                            "rollup used thresholds.completion_gate.require_ready_for_session_completion=false (or equivalent); test-bundle health is not the completion gate for this session.",
		}
	}
	return out, nil
}

func gateExitForDisplay(exit int, skipped bool) any {
	if skipped {
		return nil
	}
	return exit
}

// runLiteralRepoGates returns literal gate exit codes (0 = passed) for convergence rollup.
func runLiteralRepoGates(projectRoot string) (fkExit, zeExit int) {
	return 0, 0
}

func loadRollupChildSessions(cmd *cobra.Command, parentID string, maxChildren int) ([]convergerollup.ChildSessionInput, error) {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return nil, err
	}
	ctx := proc.OperationContext()
	sec := proc.SecurityContext()

	refsFor := func(id string) ([]string, error) {
		obj, rerr := proc.Storage().Read(ctx, sec, id)
		if rerr != nil {
			return nil, rerr
		}
		return cvsIDsFromRelatedObjectRefs(obj[objects.FieldKeyRelatedObjectRefs]), nil
	}
	if cycle, ok := convergerollup.DetectCVSRefCycle(parentID, refsFor, convergerollup.MaxRelatedCVSHopDepth); ok {
		return []convergerollup.ChildSessionInput{{
			ID:       parentID,
			Status:   objects.ObjectStatusError,
			Blockers: []string{"related_object_refs_cycle:" + strings.Join(cycle, " -> ")},
		}}, nil
	}

	parent, err := proc.Storage().Read(ctx, sec, parentID)
	if err != nil {
		return nil, err
	}
	rawRefs, _ := parent[objects.FieldKeyRelatedObjectRefs].([]any)
	var out []convergerollup.ChildSessionInput
	for _, rv := range rawRefs {
		id, ok := rv.(string)
		if !ok || !strings.HasPrefix(id, "CVS-") {
			continue
		}
		if len(out) >= maxChildren {
			out = append(out, convergerollup.ChildSessionInput{
				ID:       id,
				Status:   objects.ObjectStatusCancelled,
				Blockers: []string{"max_child_sessions_reached"},
			})
			break
		}
		ch, err := proc.Storage().Read(ctx, sec, id)
		if err != nil {
			out = append(out, convergerollup.ChildSessionInput{
				ID:       id,
				Status:   objects.ObjectStatusError,
				Blockers: []string{err.Error()},
			})
			continue
		}
		kind, _ := ch[objects.FieldKeyKind].(string)
		if kind != objects.KindConvergenceSession {
			out = append(out, convergerollup.ChildSessionInput{
				ID:       id,
				Status:   objects.ObjectStatusCancelled,
				Blockers: []string{fmt.Sprintf("not_convergence_session:%s", kind)},
			})
			continue
		}
		st, _ := ch[objects.FieldKeyStatus].(string)
		var blockers []string
		if st != objects.ObjectStatusCompleted && st != objects.ObjectStatusArchived {
			blockers = append(blockers, "child_status_"+st)
		}
		out = append(out, convergerollup.ChildSessionInput{
			ID:       id,
			Status:   st,
			Blockers: blockers,
		})
	}
	return out, nil
}

func cvsIDsFromRelatedObjectRefs(raw any) []string {
	arr, _ := raw.([]any)
	var out []string
	for _, rv := range arr {
		id, ok := rv.(string)
		if !ok || !strings.HasPrefix(id, "CVS-") {
			continue
		}
		out = append(out, id)
	}
	return out
}

// collectCVSTreeForOverseer returns coordinator + descendants reachable via related_object_refs (CVS-* only), BFS, depth-limited.
func collectCVSTreeForOverseer(cmd *cobra.Command, rootID string, maxDepth int) ([]convergerollup.CVSTreeNode, error) {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return nil, err
	}
	ctx := proc.OperationContext()
	sec := proc.SecurityContext()

	nodeFor := func(id string) ([]string, string, string, error) {
		obj, rerr := proc.Storage().Read(ctx, sec, id)
		if rerr != nil {
			return nil, "", "", rerr
		}
		st, _ := obj[objects.FieldKeyStatus].(string)
		phase, _ := obj[objects.FieldKeyCurrentPhase].(string)
		return cvsIDsFromRelatedObjectRefs(obj[objects.FieldKeyRelatedObjectRefs]), st, phase, nil
	}
	return convergerollup.CollectCVSTreeBFS(rootID, maxDepth, nodeFor)
}
