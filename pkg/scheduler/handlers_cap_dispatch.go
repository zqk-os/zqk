package scheduler

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/pipeline"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

// Dispatch, ATK minting, and CAP plan payload helpers extracted from handlers_cap_orchestrator.go
// (BLI-CEF-R2-ARCH-GOD-SCHEDULER).
// executeDispatchStage handles planning, design, grooming, and orchestrating stages
// by provisioning a Sub-Kernel (Federated Team Pod). It fetches all personas and spawns
// concurrent agent orchestrate subprocesses, allowing the team to collaborate elastically.

// nestPlanWorkstreams loads workstreams/backlog for planID, nests tasks under workstreams on planObj,
// and returns a bliID→object map for task title/description resolution.
func (h *CapOrchestratorHandler) nestPlanWorkstreams(ctx context.Context, planID string, planObj map[string]any) map[string]map[string]any {
	bliMap := make(map[string]map[string]any)
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	var wsObjects []map[string]any
	allWs, wsErr := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{
		Kind: "workstream",
	})
	if wsErr == nil && allWs != nil {
		for _, ws := range allWs.Objects {
			ref, _ := ws[objects.FieldKeyPriorityPlanRef].(string)
			refs, _ := ws[objects.FieldKeyPriorityPlanRefs].(string)
			if ref == planID || refs == planID {
				wsObjects = append(wsObjects, ws)
			}
		}
	}

	var bliObjects []map[string]any
	allBli, bliErr := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{
		Kind: "backlog_item",
	})
	if bliErr == nil && allBli != nil {
		for _, bli := range allBli.Objects {
			ref, _ := bli[objects.FieldKeyPriorityPlanRef].(string)
			if ref == planID {
				bliObjects = append(bliObjects, bli)
			}
		}
	}

	wsToBli := make(map[string][]any)
	for _, bli := range bliObjects {
		bliID, _ := bli[objects.FieldKeyID].(string)
		if bliID != "" {
			bliMap[bliID] = bli
		}
		wsRef, _ := bli[objects.FieldKeyWorkstreamRef].(string)
		if wsRef == "" {
			wsRef = capUnassignedWorkstreamID
		}
		wsToBli[wsRef] = append(wsToBli[wsRef], map[string]any{
			objects.FieldKeyID:   bli[objects.FieldKeyID],
			objects.FieldKeyTags: bli[objects.FieldKeyTags],
		})
	}

	// Preserve workstreams already returned by the plan view when storage has no newer
	// workstream or backlog rows. This keeps routed plan payloads usable in isolation.
	if len(wsObjects) == 0 && len(bliObjects) == 0 {
		return bliMap
	}

	workstreamsList := make([]any, 0, len(wsObjects)+1)
	for _, ws := range wsObjects {
		wsID, _ := ws[objects.FieldKeyID].(string)
		if wsID == "" {
			continue
		}
		tasks, ok := wsToBli[wsID]
		if !ok {
			tasks = []any{}
		}
		workstreamsList = append(workstreamsList, map[string]any{
			objects.FieldKeyID: wsID,
			"tasks":            tasks,
		})
	}
	if tasks := wsToBli[capUnassignedWorkstreamID]; len(tasks) > 0 {
		workstreamsList = append(workstreamsList, map[string]any{
			objects.FieldKeyID: capUnassignedWorkstreamID,
			"tasks":            tasks,
		})
	}
	planObj["workstreams"] = workstreamsList
	return bliMap
}

func (h *CapOrchestratorHandler) executeDispatchStage(ctx context.Context, exe, planID, instruction string, shared *capDispatchShared, isLead bool) error {
	if planID == "" {
		h.logger.Info("cap_orchestrator_no_plan", logging.StageField(instruction))
		return nil
	}
	if shared == nil {
		shared = &capDispatchShared{}
	}
	if shared.openAGI == nil {
		shared.openAGI = h.buildOpenAgentInstructionIndex(ctx)
	}
	if shared.openATK == nil {
		shared.openATK = h.buildOpenAgentTaskIndex(ctx)
	}
	openAGI := shared.openAGI
	// Stage label stays `instruction` for routing/reuse; AGI body is the kernel prompt template.
	// TRACK: BLI-1786390039711686000-e718d458 / BLI-ATK-MERGE-UP-HYGIENE-001
	agiInstruction := h.capStageAGIInstruction(ctx, instruction, planID)

	var activePersonas []string

	// Attempt to resolve from priority_plan -> team_configuration_ref
	planCmd := execwrap.CommandContext(ctx, exe, "object", "show", planID, "--format", "json")
	planCmd = h.prepareCmd(planCmd)
	planOut, err := planCmd.CombinedOutput()
	if err != nil {
		h.logger.Error("cap_orchestrator_plan_fetch_failed", err, logging.OutputField(string(planOut)))
		return errfmt.Errorf("failed to fetch priority plan: %w", err)
	}

	var planObj map[string]any
	if err := decodeCAPPlanOutput(planOut, &planObj); err != nil {
		h.logger.Error("cap_orchestrator_plan_parse_failed", err)
		return errfmt.Errorf("failed to parse priority plan: %w", err)
	}

	planStatus, _ := planObj[objects.FieldKeyStatus].(string)
	planStatus = strings.ToLower(strings.TrimSpace(planStatus))
	planComplete, _ := planObj["priority_plan_complete"].(bool)
	if planStatus == objects.ObjectStatusComplete || planStatus == objects.ObjectStatusArchived || planComplete {
		h.logger.Info("cap_orchestrator_plan_terminal_status",
			logging.PlanIDField(planID),
			logging.String("status", planStatus),
		)
		return nil
	}

	bliMap := h.nestPlanWorkstreams(ctx, planID, planObj)

	if isLead {
		// Only open (planned|in_progress) rows are CAP fuel. Completed BLIs can still
		// pass CRI-SHOVEL-READY field checks; counting them as "shovel-ready" caused
		// false confidence, and counting only EvaluateShovelReady without status
		// produced ATTN empty-column while planned>0 but DoR-incomplete (dor-gap).
		// TRACK: PRI-GROOM-EVIDENCE-REOPEN-001 — empty-column vs dor-gap diagnostic split.
		openCount := 0
		shovelReadyOpen := 0
		for _, bliObj := range bliMap {
			st, _ := bliObj[objects.FieldKeyStatus].(string)
			st = strings.ToLower(strings.TrimSpace(st))
			if st != objects.ObjectStatusPlanned && st != objects.ObjectStatusInProgress {
				continue
			}
			openCount++
			if res := validation.EvaluateShovelReady(bliObj); res.Ready {
				shovelReadyOpen++
			}
		}
		if openCount == 0 {
			h.logger.Warn("cap_orchestrator_empty_column",
				logging.PlanIDField(planID),
				logging.String("message", "Lead PRI has zero planned/in_progress BLIs. Refusing idle theater."),
			)
			h.wakeAgentAndScheduleHourglass(planID, "tpm")
			return fmt.Errorf("ATTN empty-column: lead PRI %s has zero shovel-ready BLIs", planID)
		}
		if shovelReadyOpen == 0 {
			h.logger.Warn("cap_orchestrator_dor_gap",
				logging.PlanIDField(planID),
				logging.Int("open_bli_count", openCount),
				logging.String("message", "Lead PRI has open BLIs but none pass CRI-SHOVEL-READY. Groom DoR, not empty-column."),
				logging.String("criteria_ref", validation.CriteriaIDShovelReady),
			)
			h.wakeAgentAndScheduleHourglass(planID, "tpm")
			return fmt.Errorf("ATTN dor-gap: lead PRI %s has %d open BLIs but zero CRI-SHOVEL-READY", planID, openCount)
		}
	}

	// Use PipelineRouter to automatically split into workstreams and assign personas
	router := pipeline.NewPipelineRouter()
	if routeResult, routeErr := router.Route(planObj); routeErr == nil && len(routeResult.TaskAssignments) > 0 {
		h.logger.Info("cap_orchestrator_routed_plan", logging.Int("task_count", len(routeResult.TaskAssignments)))

		pool := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "sub_kernel", "provisioning federated team tasks", 5, 5)
		pool.Start(ctx)

		var (
			dispatchErrMu sync.Mutex
			dispatchErr   error
		)
		recordDispatchErr := func(err error) {
			if err == nil {
				return
			}
			dispatchErrMu.Lock()
			defer dispatchErrMu.Unlock()
			if dispatchErr == nil {
				dispatchErr = err
			}
		}

		for taskID, persona := range routeResult.TaskAssignments {
			taskID := taskID
			persona := persona
			if submitErr := pool.Submit(ctx, func(workerCtx context.Context) error {
				_ = workerCtx
				var bliObj map[string]any
				if bli, ok := bliMap[taskID]; ok {
					bliObj = bli
				}

				// CRI-SHOVEL-READY: do not wake for non-actionable BLIs.
				if bliObj != nil {
					if res := validation.EvaluateShovelReady(bliObj); !res.Ready {
						h.logger.Info("cap_orchestrator_skip_non_shovel_ready",
							logging.PlanIDField(planID),
							logging.TaskIDField(taskID),
							logging.String("missing", strings.Join(res.Missing, ",")),
							logging.String("criteria_ref", validation.CriteriaIDShovelReady),
						)
						return nil
					}
				}

				if shared.openATK.has(planID, taskID) {
					h.logger.Info("cap_orchestrator_reusing_open_task", logging.PlanIDField(planID), logging.TaskIDField(taskID))
					h.wakeAgentAndScheduleHourglass(taskID, persona)
					return nil
				}

				// Do not mint agent_task here. REQ-CAPH-001 is CVS-bound verified
				// delivery + journal — not ATK Create. Open/proposed ATKs are
				// insufficient for dispatch (TestDispatchDeliveryComplete_OpenATKInsufficient).
				// CAP_LOOP_CONTRACT forbids treating those ATKs as delivery.
				// TPM charter: hourglass is not a license to micro-mint a conveyor.
				// Untagged router personas default to PER-DEFAULT-AGENT, which hid
				// inventory from PER-ORCH-* orchestrate. Primaries mint ATKs via
				// prepare-context + orchestrate (WFL-SUBAGENT-DISPATCH).
				h.logger.Info("cap_orchestrator_skip_atk_mint_wake_focus",
					logging.PlanIDField(planID),
					logging.TaskIDField(taskID),
					logging.String("persona", persona),
				)
				h.wakeAgentAndScheduleHourglass(planID, persona)
				return nil
			}); submitErr != nil {
				recordDispatchErr(submitErr)
			}
		}
		pool.Stop()
		return dispatchErr
	}

	if teamConfigRef, _ := planObj[objects.FieldKeyTeamConfigurationRef].(string); teamConfigRef != "" {
		h.logger.Info("cap_orchestrator_resolving_team", logging.String("team_configuration_ref", teamConfigRef))
		teamCmd := execwrap.CommandContext(ctx, exe, "object", "show", "team_configuration", teamConfigRef, "--format", "json")
		teamCmd = h.prepareCmd(teamCmd)
		if teamOut, err := teamCmd.CombinedOutput(); err == nil {
			var teamObj map[string]any
			if err := json.Unmarshal(teamOut, &teamObj); err == nil {
				if allocations, ok := teamObj[objects.FieldKeyPersonaAllocations].([]any); ok {
					for _, alloc := range allocations {
						if allocMap, ok := alloc.(map[string]any); ok {
							if personaRef, _ := allocMap[objects.FieldKeyPersonaRef].(string); personaRef != "" {
								count := 1
								if c, ok := allocMap["count"].(float64); ok {
									count = int(c)
								}
								for i := 0; i < count; i++ {
									activePersonas = append(activePersonas, personaRef)
								}
							}
						}
					}
				}
			}
		}
	}
	if len(activePersonas) == 0 {
		if refsVal, ok := planObj[objects.FieldKeyPersonaRefs]; ok {
			if refs, ok := refsVal.([]any); ok {
				for _, rAny := range refs {
					if rStr, ok := rAny.(string); ok && rStr != "" {
						activePersonas = append(activePersonas, rStr)
					}
				}
			} else if refs, ok := refsVal.([]string); ok {
				for _, rStr := range refs {
					if rStr != "" {
						activePersonas = append(activePersonas, rStr)
					}
				}
			}
		}
		if len(activePersonas) == 0 {
			if pRef, _ := planObj[objects.FieldKeyPersonaRef].(string); pRef != "" {
				activePersonas = append(activePersonas, pRef)
			} else if pID, _ := planObj["persona_id"].(string); pID != "" {
				activePersonas = append(activePersonas, pID)
			}
		}
	}

	// Fallback: If no team configured, list personas once per CAP tick (concurrent
	// List(persona) across plans hung FileObjectStorage — dump 20260805-010604).
	// Do not fan out to every persona when the plan lacks dispatch identity —
	// lifecycle now requires team_configuration_ref or persona_refs before active.
	// TRACK: BLI-1785915238591238000-619a2f9e
	if len(activePersonas) == 0 {
		h.logger.Info("cap_orchestrator_skip_no_team_or_personas",
			logging.PlanIDField(planID),
			logging.String(objects.FieldKeyInstruction, instruction))
		return nil
	}

	// CRI-PERSONA-SKILL-BOUND: skip personas without resolving ASK links (fail-closed dispatch).
	// TRACK: BLI-1785904242062561000-ec024787
	secForPersona := pkgctx.NewSystemSecurityContext()
	boundPersonas := make([]string, 0, len(activePersonas))
	for _, pid := range activePersonas {
		persona, err := h.storage.Read(ctx, secForPersona, pid)
		if err != nil || persona == nil {
			h.logger.Info("cap_orchestrator_skip_persona_unreadable",
				logging.PlanIDField(planID),
				logging.String("persona", pid))
			continue
		}
		resolve := func(askID string) (map[string]any, error) {
			return h.storage.Read(ctx, secForPersona, askID)
		}
		if !validation.IsPersonaSkillBound(persona, resolve) {
			h.logger.Info("cap_orchestrator_skip_persona_unbound_skill",
				logging.PlanIDField(planID),
				logging.String("persona", pid),
				logging.String("criteria", validation.CriteriaIDPersonaSkillBound))
			continue
		}
		boundPersonas = append(boundPersonas, pid)
	}
	if len(boundPersonas) == 0 {
		h.logger.Info("cap_orchestrator_skip_no_skill_bound_personas",
			logging.PlanIDField(planID),
			logging.Int("persona_candidates", len(activePersonas)),
			logging.String("criteria", validation.CriteriaIDPersonaSkillBound))
		return nil
	}
	activePersonas = boundPersonas

	numPersonas := len(activePersonas)
	if numPersonas == 0 {
		h.logger.Info("cap_orchestrator_no_personas")
		return nil
	}

	maxConcurrency := 5
	if numPersonas < maxConcurrency {
		maxConcurrency = numPersonas
	}

	// Shared open-AGI index from Execute (one List per CAP tick). TRACK: BLI-1785915238591238000-619a2f9e
	var claimed sync.Map // plan\0persona\0instr → struct{} for same-tick races
	var reuseCount atomic.Int64
	var mintCount atomic.Int64

	// Provision the Sub-Kernel Pod
	pool := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "sub_kernel", "provisioning federated team", maxConcurrency, maxConcurrency)
	pool.Start(ctx)

	agentLoop := pipeline.NewBuilder("agent_loop", h.logger).
		AddStage("orchestrate", func(pCtx *pipeline.Context, input any) (any, error) {
			req := input.(map[string]any)
			pid := req["pid"].(string)
			plan := req["planID"].(string)
			instr := req[objects.FieldKeyInstruction].(string)
			claimKey := strings.ToLower(plan) + "\x00" + strings.ToLower(pid) + "\x00" + strings.ToUpper(instr)
			// TRACK: BLI-1785915238591238000-619a2f9e — reuse open AGI before mint (steer fuel).
			// Reuse when an open AGI already carries this stage label (template body may differ).
			if openAGI.has(plan, pid, instruction) {
				reuseCount.Add(1)
				h.logger.Info("cap_orchestrator_reusing_open_instruction",
					logging.PlanIDField(plan),
					logging.String("persona", pid),
					logging.String(objects.FieldKeyInstruction, instruction))
				// Do not wake every persona on reuse — N plans × N personas + per-call
				// List hung the tip CAP tick. Mint is the interrupt; reuse is steady-state.
				return nil, nil
			}
			if _, loaded := claimed.LoadOrStore(claimKey, struct{}{}); loaded {
				reuseCount.Add(1)
				h.logger.Info("cap_orchestrator_reusing_in_tick_claim",
					logging.PlanIDField(plan),
					logging.String("persona", pid),
					logging.String(objects.FieldKeyInstruction, instruction))
				return nil, nil
			}
			secCtx := pkgctx.NewSystemSecurityContext()
			inboxItem := map[string]any{
				objects.FieldKeyKind:        objects.KindAgentInstruction,
				"persona_id":                pid,
				"plan_id":                   plan,
				objects.FieldKeyInstruction: instr,
				"integration_branch":        fmt.Sprintf("integration/%s", strings.ToLower(plan)),
				objects.FieldKeyStatus:      objects.ObjectStatusProposed,
			}
			if err := h.storage.Create(pCtx.Ctx, secCtx, inboxItem); err != nil {
				claimed.Delete(claimKey)
				return nil, err
			}
			mintCount.Add(1)
			openAGI.remember(plan, pid, instr)
			return nil, nil
		}).Build()

	for _, pid := range activePersonas {
		pid := pid // capture loop var
		_ = pool.Submit(ctx, func(workerCtx context.Context) error {
			_, dispatchErr := agentLoop.Run(&pipeline.Context{Ctx: workerCtx}, map[string]any{
				"pid":                       pid,
				"planID":                    planID,
				objects.FieldKeyInstruction: agiInstruction,
			})
			if dispatchErr != nil {
				h.logger.Error("sub_kernel_agent_failed", dispatchErr,
					logging.PlanIDField(planID),
					logging.String("persona", pid))
				return dispatchErr
			}

			h.logger.Info("sub_kernel_agent_completed",
				logging.PlanIDField(planID),
				logging.String("persona", pid))
			return nil
		})
	}

	pool.Stop() // Waits for the team pod to finish the cycle
	h.logger.Info("cap_orchestrator_dispatch_agi_summary",
		logging.PlanIDField(planID),
		logging.String(objects.FieldKeyInstruction, instruction),
		logging.Int("reused", int(reuseCount.Load())),
		logging.Int("minted", int(mintCount.Load())),
		logging.Int("personas", numPersonas))
	return nil
}

// resumePendingVerificationTasks consumes durable executor evidence after a restart
// or scheduler tick. Integration remains an explicit plan-branch action; once the
// commit is contained by that branch, CAP tears down the clean worktree and hands
// the ATK to sync-loop for graph-state reconciliation and verification.
func (h *CapOrchestratorHandler) resumePendingVerificationTasks(ctx context.Context, exe, planID string) error {
	if planID == "" {
		return nil
	}
	store := h.capEvidenceStorage()
	if store == nil {
		return errfmt.Errorf("CAP continuation storage is unavailable")
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	res, err := store.List(ctx, secCtx, pkgctx.NewStorageContext(), storagepkg.ListFilter{
		Kind: objects.KindAgentTask,
		Filters: map[string]any{
			objects.FieldKeyStatus:          objects.ObjectStatusPendingVerification,
			objects.FieldKeyPriorityPlanRef: planID,
		},
	})
	if err != nil {
		return errfmt.Newf("list pending CAP continuations").Wrap(err)
	}
	if res == nil {
		return nil
	}

	for _, task := range res.Objects {
		taskID, _ := task[objects.FieldKeyID].(string)
		commitHashes := relatedStringRefs(task[objects.FieldKeyCommitHashes])
		if len(commitHashes) == 0 {
			return errfmt.Errorf("pending verification task %s has no durable commit evidence", taskID)
		}

		integrationBranch := "integration/" + strings.ToLower(planID)
		allMerged := true
		for _, commitHash := range commitHashes {
			containsCmd := execwrap.CommandContext(ctx, "git", "merge-base", "--is-ancestor", commitHash, integrationBranch)
			containsCmd.Dir = h.projectRoot
			if err := containsCmd.Run(); err != nil {
				allMerged = false
				h.logger.Info(
					"cap_continuation_awaiting_integration",
					logging.TaskIDField(taskID),
					logging.PlanIDField(planID),
					logging.String("commit_hash", commitHash),
					logging.String("integration_branch", integrationBranch),
				)
			}
		}
		if !allMerged {
			h.wakeAgentAndScheduleHourglass(taskID, "tpm")
			continue
		}

		worktreePath := paths.AgentWorktreeDir(h.projectRoot, taskID)
		if _, statErr := fileutil.Stat(worktreePath); statErr == nil {
			statusCmd := execwrap.CommandContext(ctx, "git", "status", "--porcelain")
			statusCmd.Dir = worktreePath
			dirty, statusErr := statusCmd.CombinedOutput()
			if statusErr != nil {
				return errfmt.Newf("inspect continuation worktree for %s", taskID).Wrap(statusErr)
			}
			if strings.TrimSpace(string(dirty)) != "" {
				return errfmt.Errorf("pending verification worktree for %s is dirty", taskID)
			}
			removeCmd := execwrap.CommandContext(ctx, "git", "worktree", "remove", "--force", worktreePath)
			removeCmd.Dir = h.projectRoot
			if output, removeErr := removeCmd.CombinedOutput(); removeErr != nil {
				return errfmt.Newf("remove integrated worktree for %s: %s", taskID, strings.TrimSpace(string(output))).Wrap(removeErr)
			}
		} else if !fileutil.IsNotExist(statErr) {
			return errfmt.Newf("inspect continuation worktree for %s", taskID).Wrap(statErr)
		}

		reconcileCmd := execwrap.CommandContext(ctx, exe, "agent", "sync-loop", taskID)
		reconcileCmd = h.prepareCmd(reconcileCmd)
		if output, reconcileErr := reconcileCmd.CombinedOutput(); reconcileErr != nil {
			return errfmt.Newf("reconcile pending verification task %s: %s", taskID, strings.TrimSpace(string(output))).Wrap(reconcileErr)
		}
		h.logger.Info("cap_continuation_reconciled", logging.TaskIDField(taskID), logging.PlanIDField(planID))
	}
	return nil
}

func decodeCAPPlanOutput(output []byte, dst *map[string]any) error {
	return json.NewDecoder(bytes.NewReader(output)).Decode(dst)
}

// mintAgentTaskID creates a storage-shaped ATK id for builder-backed creates.
// Build() requires id before Create can assign one.
func mintAgentTaskID() string {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return fmt.Sprintf("ATK-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("ATK-%d-%x", time.Now().UnixNano(), suffix)
}

func capDispatchPlanIDs(primaryPlanID string, plans []whatsnext.WhatsNextPriorityPlan) []string {
	ids := make([]string, 0, len(plans)+1)
	seen := make(map[string]struct{}, len(plans)+1)
	add := func(id string) {
		if id == "" {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	add(primaryPlanID)
	for _, plan := range plans {
		add(plan.ID)
	}
	return ids
}
