package agentclaim

import (
	"context"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// Result is the outcome of a claim or release attempt.
type Result struct {
	Claimed   bool
	Released  bool
	ClaimedBy string
	ClaimedAt string
	Reason    string
}

// ClaimOptions holds extra data that can be associated with a claim.
type ClaimOptions struct {
	ForRef               string
	CVSRef               string
	ExitWhenCVSCompleted bool
	HourglassOn          bool

	// ProjectRoot enables the cadence check-in timer. Empty leaves the claim untimed,
	// which is the pre-existing behavior for callers that have no root to hand in.
	ProjectRoot string

	// CheckinCadence overrides DefaultCheckinCadence for this claim.
	CheckinCadence time.Duration
}

// TryClaim sets claimed_by/claimed_at when the task is free or already held by claimant.
// Contended claims (different claimed_by) fail closed.
//
// Occupancy is not a sidecar of the lifecycle: agent_task_lifecycle.yaml says proposed is
// preliminary realign ("not yet approved for dispatch") and approved → in_progress is
// "Begin work on task". Claim must refuse origin/preliminary and hop shovel-ready to
// execution_locked. TRACK: BLI-COMPLETE-HOP-ATK-PARENT-SHOCKWAVE-001
func TryClaim(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, taskID, claimant string, opts ...ClaimOptions) (Result, error) {
	claimant = strings.TrimSpace(claimant)
	if claimant == "" {
		return Result{Reason: "empty_claimant"}, errfmt.Errorf("claimed_by claimant is required")
	}
	if sp == nil {
		return Result{}, errfmt.Errorf("storage unavailable")
	}
	task, err := sp.Read(ctx, sec, taskID)
	if err != nil {
		return Result{}, errfmt.Newf("read task %s", taskID).Wrap(err)
	}
	kind, _ := task[objects.FieldKeyKind].(string)
	if kind == "" {
		kind = validation.GetIDValidator().InferKindFromID(taskID)
	}
	hasTrait, _ := objects.KindHasTrait(kind, objects.TraitOccupiable)
	if !hasTrait {
		return Result{Reason: "not_occupiable"}, errfmt.Errorf("%s (kind %s) is not occupiable", taskID, kind)
	}

	status := strings.TrimSpace(objects.StringField(task, objects.FieldKeyStatus))
	checker := objects.GetGlobalStatusChecker()
	if status != "" && checker.IsPreliminary(kind, status) {
		return Result{Reason: "not_dispatched", ClaimedBy: objects.StringField(task, objects.FieldKeyClaimedBy)},
			errfmt.Errorf("task %s is %s (preliminary realign); approve for dispatch before claim", taskID, status)
	}
	if status != "" && checker.IsTerminal(kind, status) {
		return Result{Reason: "task_terminal"},
			errfmt.Errorf("task %s is terminal (%s); cannot claim", taskID, status)
	}
	if err := refuseClaimIfParentBacklogTerminal(ctx, sp, sec, task, taskID); err != nil {
		return Result{Reason: "parent_terminal"}, err
	}

	holder := strings.TrimSpace(objects.StringField(task, objects.FieldKeyClaimedBy))
	if holder != "" && !strings.EqualFold(holder, claimant) {
		return Result{
			Claimed:   false,
			ClaimedBy: holder,
			ClaimedAt: objects.StringField(task, objects.FieldKeyClaimedAt),
			Reason:    "already_claimed",
		}, errfmt.Errorf("task %s already claimed by %s", taskID, holder)
	}
	now := time.Now().UTC().Format(time.RFC3339)

	updates := map[string]any{
		objects.FieldKeyClaimedBy: claimant,
		objects.FieldKeyClaimedAt: now,
	}
	if status == objects.ObjectStatusApproved {
		updates[objects.FieldKeyStatus] = objects.ObjectStatusInProgress
	}
	if len(opts) > 0 {
		opt := opts[0]
		if opt.ForRef != "" {
			updates["for_ref"] = opt.ForRef
		}
		if opt.CVSRef != "" {
			updates["cvs_ref"] = opt.CVSRef
		}
		if opt.ExitWhenCVSCompleted {
			updates["exit_when_cvs_completed"] = opt.ExitWhenCVSCompleted
		}
		if opt.HourglassOn {
			updates["hourglass_on"] = opt.HourglassOn
		}
	}

	kindName, _ := task[objects.FieldKeyKind].(string)

	if holder != "" && strings.EqualFold(holder, claimant) {
		// Idempotent re-claim: refresh timestamp and any extra fields.
		_ = sp.Update(ctx, sec, taskID, updates)
		stampClaimProvenance(ctx, sp, sec, task, opts)
		armCheckin(opts, taskID, claimant, kindName)
		return Result{Claimed: true, ClaimedBy: claimant, ClaimedAt: now, Reason: "already_held"}, nil
	}

	if len(opts) > 0 && opts[0].ProjectRoot != "" {
		if err := enforceTrunkTipFreshness(ctx, opts[0].ProjectRoot); err != nil {
			return Result{Reason: "stale_branch"}, err
		}
		branchRef, err := gitOutput(opts[0].ProjectRoot, "rev-parse", "--abbrev-ref", "HEAD")
		if err == nil && branchRef != "" {
			updates[objects.FieldKeyBranchName] = strings.TrimSpace(branchRef)
		}
	}

	if err := sp.Update(ctx, sec, taskID, updates); err != nil {
		return Result{}, errfmt.Newf("claim update %s", taskID).Wrap(err)
	}
	// Re-read to detect lost races (best-effort; CAS update is single-writer per process).
	after, err := sp.Read(ctx, sec, taskID)
	if err == nil {
		got := strings.TrimSpace(objects.StringField(after, objects.FieldKeyClaimedBy))
		if got != "" && !strings.EqualFold(got, claimant) {
			return Result{Claimed: false, ClaimedBy: got, Reason: "lost_race"},
				errfmt.Errorf("claim race lost on %s (held by %s)", taskID, got)
		}
	}
	stampClaimProvenance(ctx, sp, sec, task, opts)
	armCheckin(opts, taskID, claimant, kindName)
	return Result{Claimed: true, ClaimedBy: claimant, ClaimedAt: now, Reason: "claimed"}, nil
}

func refuseClaimIfParentBacklogTerminal(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, task map[string]any, taskID string) error {
	parentID := strings.TrimSpace(objects.StringField(task, objects.FieldKeyBacklogItemRef))
	if parentID == "" {
		return nil
	}
	parent, err := sp.Read(ctx, sec, parentID)
	if err != nil || parent == nil {
		return errfmt.Errorf("task %s parent backlog_item %s is unreadable; refuse claim", taskID, parentID)
	}
	st := strings.TrimSpace(objects.StringField(parent, objects.FieldKeyStatus))
	if objects.GetGlobalStatusChecker().Role(objects.KindBacklogItem, st) == objects.LifecycleRoleTerminal {
		return errfmt.Errorf("task %s parent %s is terminal (%s); cannot claim", taskID, parentID, st)
	}
	return nil
}

func enforceTrunkTipFreshness(ctx context.Context, projectRoot string) error {
	if zqkenv.TestBypassGitevidence().Get() == "1" {
		return nil
	}
	mergeBaseOut, err := gitOutput(projectRoot, "merge-base", "HEAD", "main")
	if err != nil {
		return errfmt.Newf("git merge-base failed: branch unlinked or no main branch? fail-closed gate").Wrap(err)
	}
	mainRevOut, err := gitOutput(projectRoot, "rev-parse", "main")
	if err != nil {
		return errfmt.Newf("git rev-parse main failed: no local main? fail-closed gate").Wrap(err)
	}
	headRevOut, err := gitOutput(projectRoot, "rev-parse", "HEAD")
	if err != nil {
		return errfmt.Newf("git rev-parse HEAD failed: fail-closed gate").Wrap(err)
	}

	mergeBase := strings.TrimSpace(mergeBaseOut)
	mainRev := strings.TrimSpace(mainRevOut)
	headRev := strings.TrimSpace(headRevOut)

	if mergeBase != mainRev && mergeBase != headRev {
		return errfmt.Errorf("claim requires branching from current trunk tip (main); please rebase")
	}
	return nil
}

// armCheckin opens the cadence window for a successful claim.
//
// A timer failure must not fail the claim: the claim is already recorded in the kernel,
// and refusing it here would leave the task owned but reported unclaimed. The cost of a
// missing timer is a claim nobody watches, which is the pre-existing state.
func armCheckin(opts []ClaimOptions, taskID, claimant, kind string) {
	if len(opts) == 0 || opts[0].ProjectRoot == "" {
		return
	}
	_ = ArmCheckin(opts[0].ProjectRoot, taskID, claimant, kind, opts[0].CheckinCadence)
}

// Release clears claimed_by/claimed_at when held by claimant (or force).
//
// projectRoot is variadic so existing callers compile unchanged; when supplied, the
// cadence check-in timer is removed with the claim. A timer outliving its claim would
// wake the orchestrator about a task nobody holds.
// TRACK: BLI-REDACTED
func Release(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, taskID, claimant string, force bool, projectRoot ...string) (Result, error) {
	defer func() {
		if len(projectRoot) > 0 && projectRoot[0] != "" {
			_ = ClearCheckin(projectRoot[0], taskID)
		}
	}()
	if sp == nil {
		return Result{}, errfmt.Errorf("storage unavailable")
	}
	task, err := sp.Read(ctx, sec, taskID)
	if err != nil {
		return Result{}, errfmt.Newf("read task %s", taskID).Wrap(err)
	}
	kind, _ := task[objects.FieldKeyKind].(string)
	if kind == "" {
		kind = validation.GetIDValidator().InferKindFromID(taskID)
	}
	hasTrait, _ := objects.KindHasTrait(kind, objects.TraitOccupiable)
	if !hasTrait {
		return Result{Reason: "not_occupiable"}, errfmt.Errorf("%s (kind %s) is not occupiable", taskID, kind)
	}
	holder := strings.TrimSpace(objects.StringField(task, objects.FieldKeyClaimedBy))
	if holder == "" {
		return Result{Released: true, Reason: "not_claimed"}, nil
	}
	claimant = strings.TrimSpace(claimant)
	if !force && claimant != "" && !strings.EqualFold(holder, claimant) {
		return Result{ClaimedBy: holder, Reason: "held_by_other"},
			errfmt.Errorf("task %s claimed by %s; cannot release as %s", taskID, holder, claimant)
	}
	updates := map[string]any{
		objects.FieldKeyClaimedBy: storage.FieldUnset,
		objects.FieldKeyClaimedAt: storage.FieldUnset,
	}
	if err := sp.Update(ctx, sec, taskID, updates); err != nil {
		return Result{}, errfmt.Newf("release update %s", taskID).Wrap(err)
	}
	return Result{Released: true, Reason: "released"}, nil
}
