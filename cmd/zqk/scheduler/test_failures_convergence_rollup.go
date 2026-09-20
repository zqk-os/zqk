package scheduler

import (
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/convergerollup"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

const (
	rollupGateFieldKeyScriptRel = "scripts/check-field-key-literals-repo.sh"
	rollupGateZQKEnvScriptRel   = "scripts/check-zqk-env-literals-repo.sh"
	rollupGateRunTimeout        = 10 * time.Minute
	hardcodedGoLiteralsScript   = "scripts/scan-hardcoded-go-literals.sh"
	hardcodedGoLiteralsTimeout  = 10 * time.Minute
)

// goGrepHitLine matches git grep lines like pkg/foo.go:12:…
var goGrepHitLine = regexp.MustCompile(`\.go:\d+:`)

func countGoPathGrepHitLines(stdout []byte) int {
	if len(stdout) == 0 {
		return 0
	}
	n := 0
	for _, line := range bytes.Split(stdout, []byte{'\n'}) {
		if goGrepHitLine.Match(line) {
			n++
		}
	}
	return n
}

// runHardcodedGoLiteralsScan runs the same multi-pattern scan as scripts/cvs_outcome_rollup.py
// (GIT_DRIFT_SEARCH_PATTERNS §8). Always records full stdout under .zqk/logs/drift for triage.
func runHardcodedGoLiteralsScan(projectRoot string) map[string]any {
	rel := hardcodedGoLiteralsScript
	path := filepath.Join(projectRoot, filepath.FromSlash(rel))
	if _, err := fileutil.Stat(path); err != nil {
		return map[string]any{
			"script":             rel,
			"status":             "skipped",
			objects.FieldKeyNote: "scan script not present in target repository",
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), hardcodedGoLiteralsTimeout) // Background: request-or-shutdown derived
	defer cancel()
	// Script is bash (see shebang); POSIX sh cannot parse process substitution / [[.
	cmd := execwrap.CommandContext(ctx, "bash", path, "--no-tests")
	zqkenv.WireExecForIsolatedProject(cmd, projectRoot)
	stdout, err := cmd.Output()
	exit := 0
	var stderrTail string
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exit = ee.ExitCode()
			t := string(ee.Stderr)
			if len(t) > 800 {
				t = t[len(t)-800:]
			}
			stderrTail = t
		} else if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return map[string]any{objects.ObjectStatusError: "scan_timeout", "script": rel}
		} else {
			exit = -1
		}
	}
	hits := countGoPathGrepHitLines(stdout)
	latestRel := filepath.Join(paths.ProjectDataDir, paths.LogsDir, "drift", "hardcoded-go-literals-scan-latest.txt")
	latest := filepath.Join(projectRoot, filepath.FromSlash(latestRel))
	_ = fileutil.EnsureDir(filepath.Dir(latest))
	_ = fileutil.WriteSecureFile(latest, stdout)
	out := map[string]any{
		"script":                     rel,
		"argv":                       []string{"--no-tests"},
		"exit_code":                  exit,
		"git_grep_hit_lines":         hits,
		"stdout_line_count":          bytes.Count(stdout, []byte{'\n'}),
		"captured_log_repo_relative": filepath.ToSlash(latestRel),
		objects.FieldKeyNote: "Primary grep-baseline for string/legacy literal triage (not pass/fail). " +
			"Pattern-8 drift-search-baseline file sizes are intentionally not used in rollup_status_core.",
	}
	if stderrTail != "" {
		out["stderr_tail"] = stderrTail
	}
	return out
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
			"script":    rollupGateFieldKeyScriptRel,
		},
		"zqk_env_literals_gate": map[string]any{
			"exit_code": gateExitForDisplay(zeExit, skipRollupGates),
			"script":    rollupGateZQKEnvScriptRel,
		},
		"hardcoded_go_literals_scan": hardScan,
		"evaluation_note": "Vetting matrix row counts: scripts/cvs_outcome_rollup.py. " +
			"Literal-volume measurement for drift work: hardcoded_go_literals_scan " +
			"(scripts/scan-hardcoded-go-literals.sh), same as Python rollup — not pattern-8 search-baseline file bytes.",
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

// runLiteralRepoGates runs literal gate scripts when present in the target repository.
// Missing script files yield exit code 0 (skipped) so binary-only projects are not blocked.
func runLiteralRepoGates(projectRoot string) (fkExit, zeExit int) {
	fkExit = runRepoShellScript(projectRoot, rollupGateFieldKeyScriptRel)
	zeExit = runRepoShellScript(projectRoot, rollupGateZQKEnvScriptRel)
	return fkExit, zeExit
}

func runRepoShellScript(projectRoot, rel string) int {
	path := filepath.Join(projectRoot, filepath.FromSlash(rel))
	if _, err := fileutil.Stat(path); err != nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), rollupGateRunTimeout) // Background: request-or-shutdown derived
	defer cancel()
	cmd := execwrap.CommandContext(ctx, "sh", path)
	zqkenv.WireExecForIsolatedProject(cmd, projectRoot)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if err == nil {
		return 0
	}
	if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return -2
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
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
