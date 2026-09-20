package agent

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// orchDisposition says whether an existing ATK should be minted, reused, skipped, or replaced.
type orchDisposition int

const (
	orchDispositionMint orchDisposition = iota
	orchDispositionReuse
	orchDispositionSkipDone
	// orchDispositionReplace: error/failed debris. Never reuse the dead ID
	// (2026-09-03 "executor produced no commit" remint loop). Mint a new ATK
	// when orch is explicitly asked to run; idle auto-dispatch still skips.
	orchDispositionReplace
)

func dispositionForOrchestrationStatus(status string) orchDisposition {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case objects.ObjectStatusImplemented,
		objects.ObjectStatusArchived,
		objects.ObjectStatusCompleted,
		objects.ObjectStatusComplete,
		"resolved":
		return orchDispositionSkipDone
	case objects.ObjectStatusPendingVerification:
		// Evidence already collected — do not remint or re-run the executor.
		return orchDispositionSkipDone
	case objects.ObjectStatusError, objects.ObjectStatusFailed:
		return orchDispositionReplace
	case objects.ObjectStatusApproved,
		objects.ObjectStatusInProgress,
		objects.ObjectStatusProposed,
		objects.ObjectStatusPending:
		return orchDispositionReuse
	case "":
		return orchDispositionMint
	default:
		// Fail closed: reuse the same ID rather than mint a replacement.
		return orchDispositionReuse
	}
}

func orchestrationTaskTitles(itemTitle, itemID string) []string {
	itemTitle = strings.TrimSpace(itemTitle)
	itemID = strings.TrimSpace(itemID)
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, e := range out {
			if strings.EqualFold(e, s) {
				return
			}
		}
		out = append(out, s)
	}
	add(itemTitle)
	if itemTitle != "" && !strings.HasPrefix(itemTitle, "Execute Task: ") {
		add("Execute Task: " + itemTitle)
	}
	if itemID != "" {
		add("Execute " + itemID)
		add("Execute Task: " + itemID)
	}
	return out
}

func orchestrationTaskTitleMatches(existingTitle, itemTitle, itemID string) bool {
	existingTitle = strings.TrimSpace(existingTitle)
	for _, want := range orchestrationTaskTitles(itemTitle, itemID) {
		if strings.EqualFold(existingTitle, want) {
			return true
		}
	}
	return false
}

func orchestrationTaskPlanMatches(obj map[string]any, planID string) bool {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return true
	}
	if obj == nil {
		return false
	}
	pipe, _ := obj[objects.FieldKeyPipelineRef].(string)
	pri, _ := obj[objects.FieldKeyPriorityPlanRef].(string)
	return strings.EqualFold(strings.TrimSpace(pipe), planID) || strings.EqualFold(strings.TrimSpace(pri), planID)
}

func orchestrationTaskBacklogMatches(obj map[string]any, itemID string) bool {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" || obj == nil {
		return false
	}
	ref, _ := obj[objects.FieldKeyBacklogItemRef].(string)
	return strings.EqualFold(strings.TrimSpace(ref), itemID)
}

func pickExistingOrchestrationTask(tasks []map[string]any, itemTitle, planID, itemID string) (id, status string, disp orchDisposition) {
	var bestID, bestStatus, bestUpdated string
	var bestObj map[string]any
	bestDisp := orchDispositionMint
	for _, obj := range tasks {
		if obj == nil {
			continue
		}
		title, _ := obj[objects.FieldKeyTitle].(string)
		if !orchestrationTaskTitleMatches(title, itemTitle, itemID) && !orchestrationTaskBacklogMatches(obj, itemID) {
			continue
		}
		if !orchestrationTaskPlanMatches(obj, planID) {
			continue
		}
		candidateID, _ := obj[objects.FieldKeyID].(string)
		if strings.TrimSpace(candidateID) == "" {
			continue
		}
		st, _ := obj[objects.FieldKeyStatus].(string)
		candidateDisp := dispositionForOrchestrationStatus(st)
		if candidateDisp == orchDispositionMint {
			continue
		}
		updated, _ := obj[objects.FieldKeyUpdatedAt].(string)
		if bestID == "" || betterOrchCandidate(obj, candidateDisp, updated, bestObj, bestDisp, bestUpdated) {
			bestID, bestStatus, bestDisp, bestUpdated, bestObj = candidateID, st, candidateDisp, updated, obj
		}
	}
	return bestID, bestStatus, bestDisp
}

func betterOrchCandidate(obj map[string]any, disp orchDisposition, updated string, bestObj map[string]any, bestDisp orchDisposition, bestUpdated string) bool {
	if orchDispRank(disp) != orchDispRank(bestDisp) {
		return orchDispRank(disp) > orchDispRank(bestDisp)
	}
	candRank := orchLivePickRank(obj)
	bestRank := orchLivePickRank(bestObj)
	if candRank != bestRank {
		return candRank > bestRank
	}
	return updated > bestUpdated
}

func orchDispRank(d orchDisposition) int {
	switch d {
	case orchDispositionReuse:
		return 3
	case orchDispositionSkipDone:
		return 2
	case orchDispositionReplace:
		return 1
	default:
		return 0
	}
}

// orchLivePickRank prefers an assigned approved/claimed ATK over leftover remint debris
// that shares the same title.
func orchLivePickRank(obj map[string]any) int {
	if obj == nil {
		return 0
	}
	st, _ := obj[objects.FieldKeyStatus].(string)
	claimed, _ := obj[objects.FieldKeyClaimedBy].(string)
	st = strings.ToLower(strings.TrimSpace(st))
	claimed = strings.TrimSpace(claimed)
	switch st {
	case objects.ObjectStatusApproved:
		return 3
	case objects.ObjectStatusInProgress:
		if claimed != "" {
			return 3
		}
		return 2
	case objects.ObjectStatusError, objects.ObjectStatusFailed:
		return 1
	default:
		return 2
	}
}

// orchDispatchSkipReason is for coordinator idle auto-dispatch only.
// Empty means the idle gland may hand ORCHESTRATE_PLAN (green field, or a live ATK).
// Terminal-only (implemented / archived / error, no live ATK) is not an idle
// remint signal — that was the 2026-09-03 derailment. Explicit ORCHESTRATE_PLAN
// still runs so remaining planned BLIs can mint replacement ATKs.
func orchDispatchSkipReason(tasks []map[string]any) string {
	live, terminal := 0, 0
	for _, obj := range tasks {
		if obj == nil {
			continue
		}
		st, _ := obj[objects.FieldKeyStatus].(string)
		switch dispositionForOrchestrationStatus(st) {
		case orchDispositionReuse:
			live++
		case orchDispositionSkipDone, orchDispositionReplace:
			terminal++
		}
	}
	if live == 0 && terminal > 0 {
		return "no_live_atk"
	}
	return ""
}

func recoverAgentTaskListFilters(planID string) []storage.ListFilter {
	planID = strings.TrimSpace(planID)
	statuses := []string{objects.ObjectStatusError, objects.ObjectStatusFailed}
	keys := []string{objects.FieldKeyPriorityPlanRef, objects.FieldKeyPipelineRef}
	out := make([]storage.ListFilter, 0, len(keys)*len(statuses))
	for _, key := range keys {
		for _, st := range statuses {
			out = append(out, storage.ListFilter{
				Kind: objects.KindAgentTask,
				Filters: map[string]any{
					key:                    planID,
					objects.FieldKeyStatus: st,
				},
			})
		}
	}
	return out
}

func mergeObjectsByID(groups ...[]map[string]any) []map[string]any {
	seen := make(map[string]struct{})
	var out []map[string]any
	for _, group := range groups {
		for _, obj := range group {
			if obj == nil {
				continue
			}
			id, _ := obj[objects.FieldKeyID].(string)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, obj)
		}
	}
	return out
}

func listOrchestrationTasksForPlan(
	ctx context.Context,
	sp storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	planID string,
) ([]map[string]any, error) {
	if sp == nil {
		return nil, errfmt.Errorf("storage provider is not available")
	}
	planID = strings.TrimSpace(planID)
	var groups [][]map[string]any
	var lastErr error
	for _, key := range []string{objects.FieldKeyPriorityPlanRef, objects.FieldKeyPipelineRef} {
		res, err := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{
			Kind:    objects.KindAgentTask,
			Filters: map[string]any{key: planID},
		})
		if err != nil {
			lastErr = err
			continue
		}
		if res != nil {
			groups = append(groups, res.Objects)
		}
	}
	if len(groups) == 0 && lastErr != nil {
		return nil, errfmt.Newf("list agent tasks for plan %s", planID).Wrap(lastErr)
	}
	return mergeObjectsByID(groups...), nil
}

func findExistingOrchestrationTask(
	ctx context.Context,
	state *orchestratorState,
	itemTitle, itemID string,
) (id, status string, disp orchDisposition, err error) {
	if state == nil || state.sp == nil {
		return "", "", orchDispositionMint, errfmt.Errorf("orchestrator storage is not available")
	}
	listed, listErr := listOrchestrationTasksForPlan(ctx, state.sp, state.secCtx, state.planID)
	if listErr != nil {
		return "", "", orchDispositionMint, listErr
	}
	var drafts []map[string]any
	if state.proc != nil {
		draftIDs, dErr := storage.ListObjectDraftPlaneIDs(state.proc.ProjectRoot(), objects.KindAgentTask)
		if dErr == nil {
			for _, dID := range draftIDs {
				dObj, readErr := state.sp.Read(ctx, state.secCtx, dID)
				if readErr != nil || dObj == nil {
					continue
				}
				drafts = append(drafts, dObj)
			}
		}
	}
	id, status, disp = pickExistingOrchestrationTask(mergeObjectsByID(listed, drafts), itemTitle, state.planID, itemID)
	return id, status, disp, nil
}

func ensureOrchestrationWorktree(ctx context.Context, projectRoot, taskID string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return "", errfmt.Errorf("empty task id for orchestration worktree")
	}
	worktreePath := paths.AgentWorktreeDir(projectRoot, taskID)
	baseRef := orchestrationWorktreeBaseRef(ctx, projectRoot)
	if isGitWorktreeCheckout(worktreePath) {
		// Leftover agent/ATK-* branches sit on pre-merge tips. Reset to the
		// plan/main tip, then drop untracked junk (except .zqk/process CAS).
		if err := resetOrchestrationWorktree(ctx, worktreePath, baseRef); err != nil {
			return "", err
		}
		_ = paths.BootstrapWorktreeSettings(worktreePath, projectRoot)
		assumeUnchangedWorktreeConfig(ctx, worktreePath)
		return worktreePath, nil
	}
	if _, err := fileutil.Stat(worktreePath); err == nil {
		if rmErr := fileutil.RemoveAll(worktreePath); rmErr != nil {
			return "", errfmt.Newf("replace broken orchestration worktree %s", worktreePath).Wrap(rmErr)
		}
	}
	prune := execwrap.CommandContext(ctx, "git", "worktree", "prune")
	prune.Dir = projectRoot
	_ = prune.Run()

	branchName := "agent/" + taskID
	// -B retargets a leftover agent/ATK branch onto the current base instead
	// of checking out the stale tip (live ALPHA was still on #1702).
	cmd := execwrap.CommandContext(ctx, "git", "worktree", "add", "-B", branchName, worktreePath, baseRef)
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		if isGitWorktreeCheckout(worktreePath) {
			_ = paths.BootstrapWorktreeSettings(worktreePath, projectRoot)
			assumeUnchangedWorktreeConfig(ctx, worktreePath)
			return worktreePath, nil
		}
		return "", errfmt.Newf("create isolated worktree for %s: %s", taskID, strings.TrimSpace(string(out))).Wrap(err)
	}
	_ = paths.BootstrapWorktreeSettings(worktreePath, projectRoot)
	assumeUnchangedWorktreeConfig(ctx, worktreePath)
	return worktreePath, nil
}

func assumeUnchangedWorktreeConfig(_ context.Context, worktreePath string) {
	paths.AssumeUnchangedWorktreeConfig(worktreePath)
}

func orchestrationWorktreeBaseRef(ctx context.Context, projectRoot string) string {
	cmd := execwrap.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = projectRoot
	if out, err := cmd.Output(); err == nil {
		if sha := strings.TrimSpace(string(out)); sha != "" {
			return sha
		}
	}
	return "HEAD"
}

func gitRevExists(ctx context.Context, projectRoot, rev string) bool {
	cmd := execwrap.CommandContext(ctx, "git", "rev-parse", "--verify", rev)
	cmd.Dir = projectRoot
	return cmd.Run() == nil
}

func resetOrchestrationWorktree(ctx context.Context, worktreePath, baseRef string) error {
	baseRef = strings.TrimSpace(baseRef)
	if baseRef == "" {
		baseRef = "HEAD"
	}
	reset := execwrap.CommandContext(ctx, "git", "reset", "--hard", baseRef)
	reset.Dir = worktreePath
	if out, err := reset.CombinedOutput(); err != nil {
		return errfmt.Newf("reset orchestration worktree %s to %s: %s", worktreePath, baseRef, strings.TrimSpace(string(out))).Wrap(err)
	}
	// Exclude process directories and .zqk: untracked hash YAML is kernel state.
	clean := execwrap.CommandContext(ctx, "git", "clean", "-fd", "-e", paths.ProcessDir, "-e", paths.ProcessDir+"/", "-e", ".zqk", "-e", ".zqk/")
	clean.Dir = worktreePath
	if out, err := clean.CombinedOutput(); err != nil {
		return errfmt.Newf("clean orchestration worktree %s: %s", worktreePath, strings.TrimSpace(string(out))).Wrap(err)
	}
	return nil
}

func isGitWorktreeCheckout(path string) bool {
	st, err := fileutil.Lstat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return st.Mode().IsRegular() || st.IsDir()
}

func gitRefExists(ctx context.Context, projectRoot, ref string) bool {
	cmd := execwrap.CommandContext(ctx, "git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = projectRoot
	return cmd.Run() == nil
}
