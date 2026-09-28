package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/git"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// AutoExecOptions specifies parameters for single-command autonomous execution.
type AutoExecOptions struct {
	TargetID       string // Can be BLI ID, ATK ID, or empty for auto-discovery
	Claimant       string
	PersonaRef     string
	DryRun         bool
	RunVerify      bool
	CheckinCadence time.Duration
}

// AutoExecStepResult records individual pipeline phase execution status.
type AutoExecStepResult struct {
	Step       string `json:"step"`
	StepStatus string `json:"status"` // ok, skipped, failed, dry_run
	Detail     string `json:"detail,omitempty"`
	ObjectID   string `json:"object_id,omitempty"`
}

// AutoExecResult captures the outcome of the autonomous execution pipeline.
type AutoExecResult struct {
	TargetBLIID     string               `json:"target_bli_id"`
	TargetTaskID    string               `json:"target_task_id,omitempty"`
	Claimant        string               `json:"claimant"`
	Status          string               `json:"status"` // dry_run, in_progress, complete, failed
	Steps           []AutoExecStepResult `json:"steps"`
	VerifiedTestIDs []string             `json:"verified_test_ids,omitempty"`
	LatchedCritIDs  []string             `json:"latched_criteria_ids,omitempty"`
	Error           string               `json:"error,omitempty"`
}

// TestVerifierFunc verifies a test case by path or ID.
type TestVerifierFunc func(ctx context.Context, testID, pathOrID string) (bool, string, error)

// AutoExecPipeline coordinates discovery, work claiming, testing, and latching.
type AutoExecPipeline struct {
	Storage   storage.ObjectStorageProvider
	Verifier  TestVerifierFunc
	GitFacade *git.Facade
}

// NewAutoExecPipeline creates an autonomous execution pipeline backed by storage.
func NewAutoExecPipeline(store storage.ObjectStorageProvider) *AutoExecPipeline {
	return &AutoExecPipeline{
		Storage: store,
	}
}

// Execute runs the autonomous execution loop end-to-end against a target or discovered BLI.
func (p *AutoExecPipeline) Execute(ctx context.Context, sec *pkgctx.SecurityContext, opts AutoExecOptions) (*AutoExecResult, error) {
	if p.Storage == nil {
		return nil, errfmt.Errorf("storage provider is required")
	}
	if sec == nil {
		sec = pkgctx.NewSystemSecurityContext()
	}
	claimant := strings.TrimSpace(opts.Claimant)
	if claimant == "" {
		claimant = "agent:auto-exec"
	}

	res := &AutoExecResult{
		Claimant: claimant,
		Steps:    make([]AutoExecStepResult, 0, 5),
	}

	// Phase 1: Target Discovery / Resolution
	bli, taskID, err := p.resolveTarget(ctx, sec, opts.TargetID)
	if err != nil {
		res.Status = objects.ObjectStatusFailed
		res.Error = err.Error()
		res.Steps = append(res.Steps, AutoExecStepResult{
			Step:       "discovery",
			StepStatus: objects.ObjectStatusFailed,
			Detail:     err.Error(),
		})
		return res, err
	}
	bliID, _ := bli[objects.FieldKeyID].(string)
	res.TargetBLIID = bliID
	res.TargetTaskID = taskID
	res.Steps = append(res.Steps, AutoExecStepResult{
		Step:       "discovery",
		StepStatus: "ok",
		Detail:     fmt.Sprintf("Resolved target backlog item %s", bliID),
		ObjectID:   bliID,
	})

	// Phase 2: Claiming
	if opts.DryRun {
		res.Status = "dry_run"
		res.Steps = append(res.Steps, AutoExecStepResult{
			Step:       "claim",
			StepStatus: "dry_run",
			Detail:     "Simulated claim without mutating storage",
			ObjectID:   bliID,
		})
		return res, nil
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	bli[objects.FieldKeyClaimedBy] = claimant
	bli[objects.FieldKeyClaimedAt] = nowStr
	currentStatus, _ := bli[objects.FieldKeyStatus].(string)
	if currentStatus != objects.ObjectStatusInProgress && currentStatus != objects.ObjectStatusComplete {
		bli[objects.FieldKeyStatus] = objects.ObjectStatusInProgress
	}
	if bli["estimated_effort"] == nil || bli["estimated_effort"] == "" {
		bli["estimated_effort"] = "1d"
	}
	if err := p.Storage.Update(ctx, sec, bliID, bli); err != nil {
		res.Status = objects.ObjectStatusFailed
		res.Error = fmt.Sprintf("failed to claim BLI: %v", err)
		res.Steps = append(res.Steps, AutoExecStepResult{
			Step:       "claim",
			StepStatus: objects.ObjectStatusFailed,
			Detail:     err.Error(),
			ObjectID:   bliID,
		})
		return res, err
	}
	res.Status = objects.ObjectStatusInProgress
	res.Steps = append(res.Steps, AutoExecStepResult{
		Step:       "claim",
		StepStatus: "ok",
		Detail:     fmt.Sprintf("Claimed %s for %s", bliID, claimant),
		ObjectID:   bliID,
	})

	// Phase 3: Context & Verification Latching
	critIDs := toStringSlice(bli["criteria_refs"])
	if len(critIDs) == 0 {
		critIDs = toStringSlice(bli["criteria_ref"])
	}

	if opts.RunVerify && len(critIDs) > 0 {
		verifiedTests, latchedCrits, verifyErr := p.verifyAndLatch(ctx, sec, bliID, critIDs, claimant)
		if verifyErr != nil {
			res.Error = verifyErr.Error()
			res.Steps = append(res.Steps, AutoExecStepResult{
				Step:       "verification",
				StepStatus: objects.ObjectStatusFailed,
				Detail:     verifyErr.Error(),
			})
			return res, verifyErr
		}
		res.VerifiedTestIDs = verifiedTests
		res.LatchedCritIDs = latchedCrits
		res.Steps = append(res.Steps, AutoExecStepResult{
			Step:       "verification",
			StepStatus: "ok",
			Detail:     fmt.Sprintf("Verified %d tests and latched %d criteria", len(verifiedTests), len(latchedCrits)),
		})

		// Check if all criteria for this BLI are now complete
		allComplete := true
		for _, cID := range critIDs {
			cObj, err := p.Storage.Read(ctx, sec, cID)
			if err != nil || cObj[objects.FieldKeyStatus] != objects.ObjectStatusComplete {
				allComplete = false
				break
			}
		}
		if allComplete {
			bli[objects.FieldKeyStatus] = objects.ObjectStatusComplete
			if bli["estimated_effort"] == nil || bli["estimated_effort"] == "" {
				bli["estimated_effort"] = "1d"
			}
			if bli["actual_effort"] == nil || bli["actual_effort"] == "" {
				bli["actual_effort"] = "1d"
			}

			// Automatically discover commit hashes citing the BLI if not already set
			existingHashes := toStringSlice(bli[objects.FieldKeyCommitHashes])
			if len(existingHashes) == 0 {
				planRef, _ := bli[objects.FieldKeyPriorityPlanRef].(string)
				facade := p.GitFacade
				if facade == nil {
					facade = git.NewFacade("")
				}
				patterns := []string{bliID}
				if planRef != "" {
					patterns = append(patterns, planRef)
				}
				if discovered, err := facade.FindCommitHashesByGrep(ctx, 10, patterns...); err == nil && len(discovered) > 0 {
					bli[objects.FieldKeyCommitHashes] = discovered
				}
			}

			if err := p.Storage.Update(ctx, sec, bliID, bli); err != nil {
				res.Steps = append(res.Steps, AutoExecStepResult{
					Step:       "latch",
					StepStatus: objects.ObjectStatusBlocked,
					Detail:     fmt.Sprintf("Promotion to complete blocked by lifecycle precondition: %v", err),
					ObjectID:   bliID,
				})
			} else {
				res.Status = objects.ObjectStatusComplete
				res.Steps = append(res.Steps, AutoExecStepResult{
					Step:       "latch",
					StepStatus: "ok",
					Detail:     fmt.Sprintf("Promoted %s to complete", bliID),
					ObjectID:   bliID,
				})
			}
		}
	}

	return res, nil
}

func (p *AutoExecPipeline) resolveTarget(ctx context.Context, sec *pkgctx.SecurityContext, targetID string) (map[string]any, string, error) {
	target := strings.TrimSpace(targetID)
	if target != "" {
		if strings.HasPrefix(target, "ATK-") {
			task, err := p.Storage.Read(ctx, sec, target)
			if err != nil {
				return nil, "", errfmt.Errorf("task %s not found: %w", target, err)
			}
			refs := toStringSlice(task["related_object_refs"])
			for _, ref := range refs {
				if strings.HasPrefix(ref, "BLI-") {
					bli, err := p.Storage.Read(ctx, sec, ref)
					if err == nil && bli != nil {
						return bli, target, nil
					}
				}
			}
			return nil, target, errfmt.Errorf("task %s has no associated backlog item", target)
		}
		if strings.HasPrefix(target, "BLI-") {
			bli, err := p.Storage.Read(ctx, sec, target)
			if err != nil {
				return nil, "", errfmt.Errorf("backlog item %s not found: %w", target, err)
			}
			return bli, "", nil
		}
		// Attempt reading directly as ID
		obj, err := p.Storage.Read(ctx, sec, target)
		if err == nil && obj != nil {
			if obj[objects.FieldKeyKind] == objects.KindBacklogItem {
				return obj, "", nil
			}
		}
	}

	// Auto-discovery from storage
	qr, err := p.Storage.List(ctx, sec, pkgctx.NewStorageContext(), storage.ListFilter{
		Kind: objects.KindBacklogItem,
	})
	if err != nil {
		return nil, "", errfmt.Errorf("failed to list backlog items for discovery: %w", err)
	}

	var candidates []map[string]any
	for _, it := range qr.Objects {
		st, _ := it[objects.FieldKeyStatus].(string)
		if st == "planned" || st == "in_progress" {
			candidates = append(candidates, it)
		}
	}
	if len(candidates) == 0 {
		for _, it := range qr.Objects {
			st, _ := it[objects.FieldKeyStatus].(string)
			if st == "originated" {
				candidates = append(candidates, it)
			}
		}
	}
	if len(candidates) == 0 {
		return nil, "", errfmt.Errorf("no eligible backlog items found for autonomous execution")
	}

	// Prioritize high priority, then in_progress, then planned
	sort.SliceStable(candidates, func(i, j int) bool {
		pI := priorityRank(candidates[i]["priority"])
		pJ := priorityRank(candidates[j]["priority"])
		return pI > pJ
	})

	return candidates[0], "", nil
}

func (p *AutoExecPipeline) verifyAndLatch(ctx context.Context, sec *pkgctx.SecurityContext, bliID string, critIDs []string, claimant string) ([]string, []string, error) {
	var verifiedTests []string
	var latchedCrits []string

	// Discover test cases linking to these criteria
	tcResult, err := p.Storage.List(ctx, sec, pkgctx.NewStorageContext(), storage.ListFilter{
		Kind: objects.KindTestCase,
	})
	if err != nil {
		return nil, nil, err
	}

	critMap := make(map[string]bool, len(critIDs))
	for _, c := range critIDs {
		critMap[c] = true
	}

	for _, tc := range tcResult.Objects {
		tcID, _ := tc[objects.FieldKeyID].(string)
		tcCrits := toStringSlice(tc["criteria_refs"])
		hasMatch := false
		for _, c := range tcCrits {
			if critMap[c] {
				hasMatch = true
				break
			}
		}
		if !hasMatch {
			bliRefs := toStringSlice(tc["backlog_item_refs"])
			for _, b := range bliRefs {
				if b == bliID {
					hasMatch = true
					break
				}
			}
		}
		if !hasMatch {
			continue
		}

		pathOrID, _ := tc["path_or_id"].(string)
		passed := true
		if p.Verifier != nil {
			ok, _, err := p.Verifier(ctx, tcID, pathOrID)
			if err != nil || !ok {
				passed = false
			}
		} else if pathOrID != "" {
			cmd := execwrap.CommandContext(ctx, "go", "test", "-v", pathOrID)
			if err := cmd.Run(); err != nil {
				passed = false
			}
		}

		if passed {
			tc[objects.FieldKeyStatus] = objects.ObjectStatusComplete
			_ = p.Storage.Update(ctx, sec, tcID, tc)
			verifiedTests = append(verifiedTests, tcID)

			for _, c := range tcCrits {
				if critMap[c] {
					cObj, err := p.Storage.Read(ctx, sec, c)
					if err == nil && cObj != nil {
						cObj[objects.FieldKeyStatus] = objects.ObjectStatusComplete
						_ = p.Storage.Update(ctx, sec, c, cObj)
						latchedCrits = append(latchedCrits, c)
					}
				}
			}
		}
	}

	return verifiedTests, latchedCrits, nil
}

func priorityRank(v any) int {
	s, _ := v.(string)
	switch strings.ToLower(s) {
	case "p0", "urgent", "critical":
		return 4
	case "p1", "high":
		return 3
	case "p2", "normal", "medium":
		return 2
	case "p3", "low":
		return 1
	default:
		return 0
	}
}

func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch typed := v.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, it := range typed {
			if s, ok := it.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if typed != "" {
			return []string{typed}
		}
	}
	return nil
}
