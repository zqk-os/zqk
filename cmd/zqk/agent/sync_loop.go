package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/agentclaim"
	"github.com/zqk-os/zqk/pkg/agentidle"
	"github.com/zqk-os/zqk/pkg/audit"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/maintenance"
	"github.com/zqk-os/zqk/pkg/mutation"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/objects/koi"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/swarm"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/verification"
)

// NewSyncLoopCmd creates the agent sync-loop command
func NewSyncLoopCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentSyncLoopCommandBuilder()
	// regenerate from CLI spec; builder currently emits empty Use.
	cmd.Use = "sync-loop"
	cmd.Hidden = false
	cmd.Args = cobra.ExactArgs(1)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		taskID := ""
		if len(args) > 0 {
			taskID = strings.TrimSpace(args[0])
		}
		if taskID == "" {
			return errfmt.Errorf("task ID is required. Usage: %s <task-id>", paths.CLIUsage("agent", "sync-loop"))
		}
		return runSyncLoop(cmd, taskID)
	}
	return cmd
}

// ContextBundle represents the resolved state graph for the agent task
type ContextBundle struct {
	Task         map[string]any
	Dependencies []map[string]any
}

// QuerySubgraph resolves the bounded context graph around a task
func QuerySubgraph(ctx context.Context, secCtx *storage.SecurityContext, sp storage.ObjectStorageProvider, taskID string, depth int) ([]map[string]any, error) {
	if depth <= 0 {
		depth = 5 // enforce semantic traversal depth
	}

	hasSemanticPayload := func(obj map[string]any) bool {
		if strings.TrimSpace(koi.GetString(obj, objects.FieldKeyDescription)) != "" {
			return true
		}
		if len(koi.GetSlice(obj, objects.FieldKeyAcceptanceCriteria)) > 0 {
			return true
		}
		if strings.TrimSpace(koi.GetString(obj, "problem_statement")) != "" {
			return true
		}
		return false
	}

	visited := make(map[string]bool)
	deps := make([]map[string]any, 0)

	var traverse func(currentID string, currentDepth int)
	traverse = func(currentID string, currentDepth int) {
		if currentDepth > depth {
			return
		}
		if visited[currentID] {
			return
		}
		visited[currentID] = true

		obj, err := sp.Read(ctx, secCtx, currentID)
		if err != nil || obj == nil {
			return
		}

		if currentID != taskID {
			deps = append(deps, obj)
		}

		// Stop traversing upward if we found a concrete semantic payload, unless we're on the starting task.
		if currentID != taskID && hasSemanticPayload(obj) {
			return
		}

		var refsToExplore []string
		extractRefs := func(key string) {
			refsToExplore = append(refsToExplore, koi.GetStringSlice(obj, key)...)
		}

		// Follow pointers upward
		extractRefs("context_refs")
		extractRefs(objects.FieldKeyRelatedObjectRefs)
		extractRefs(objects.FieldKeyRequirementRefs)
		extractRefs("criteria_refs")

		for _, ref := range refsToExplore {
			traverse(ref, currentDepth+1)
		}
	}

	traverse(taskID, 0)

	for _, dep := range deps {
		stripGraphBloat(dep)
	}

	return deps, nil
}

func stripGraphBloat(obj map[string]any) {
	if obj == nil {
		return
	}
	delete(obj, objects.FieldKeyResolvedRelatedObjectRefs)
	delete(obj, objects.FieldKeyStatusHistory)
	delete(obj, objects.FieldKeyChangeLog)

	for k, v := range obj {
		if strings.HasPrefix(k, "resolved_") {
			delete(obj, k)
		} else if m, ok := v.(map[string]any); ok {
			stripGraphBloat(m)
		} else if l, ok := v.([]any); ok {
			for _, item := range l {
				if m2, ok2 := item.(map[string]any); ok2 {
					stripGraphBloat(m2)
				}
			}
		}
	}
}

func buildContextBundle(task map[string]any, deps []map[string]any) ContextBundle {
	stripGraphBloat(task)
	for _, dep := range deps {
		stripGraphBloat(dep)
	}
	return ContextBundle{
		Task:         task,
		Dependencies: deps,
	}
}

//nolint:gocyclo
func runSyncLoop(cmd *cobra.Command, taskID string) (runErr error) {
	proc, err := newAgentProcessor(cmd)
	if err != nil {
		return err
	}

	// Create an execution context with an extended timeout to account for slow local CPU inference
	// Must detach from proc.OperationContext() to avoid inheriting the 30s default CLI timeout.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(proc.OperationContext()), 4*time.Hour)
	defer cancel()

	secCtx := proc.SecurityContext()
	sp := proc.Storage()

	// Verify taskID is an agent_task
	initTask, err := sp.Read(ctx, secCtx, taskID)
	if err != nil {
		return errfmt.Newf("failed to read task %s", taskID).Wrap(err)
	}
	if !koi.IsKind(initTask, objects.KindAgentTask) {
		return errfmt.Errorf("agent sync-loop can only execute agent_task objects, received: %s", koi.Kind(initTask))
	}

	// Initialize hourglass via Graph
	flipHourglass(ctx, secCtx, sp, taskID, proc.ProjectRoot())

	// Create coordination channel and publish process_started event
	cc := scheduler.NewCoordinationChannel(proc.ProjectRoot())
	if err := cc.PublishEvent(scheduler.Event{
		Type:      "process_started",
		JobID:     taskID,
		Timestamp: time.Now().UTC(),
	}); err != nil {
		logging.FluentEvent(logging.GetLogger()).Warn("Failed to publish process_started event").
			WithError(err).String("job_id", taskID).Log()
	}

	defer func() {
		removeHourglass(ctx, secCtx, sp, taskID, proc.ProjectRoot())
		if runErr != nil {
			if err := cc.PublishEvent(scheduler.Event{
				Type:      "process_errored",
				JobID:     taskID,
				Timestamp: time.Now().UTC(),
				Metadata: map[string]any{
					"error": runErr.Error(),
				},
			}); err != nil {
				logging.FluentEvent(logging.GetLogger()).Warn("Failed to publish process_errored event").
					WithError(err).String("job_id", taskID).Log()
			}
		} else {
			// Read the task to see if it is implemented
			currentTask, readErr := sp.Read(ctx, secCtx, taskID)
			if readErr == nil && koi.IsStatus(currentTask, objects.ObjectStatusImplemented) {
				if err := cc.PublishEvent(scheduler.Event{
					Type:      "process_completed",
					JobID:     taskID,
					Timestamp: time.Now().UTC(),
				}); err != nil {
					logging.FluentEvent(logging.GetLogger()).Warn("Failed to publish process_completed event").
						WithError(err).String("job_id", taskID).Log()
				}
			}
		}
	}()

	// Start background hourglass refresher
	ctxHourglass, cancelHourglass := context.WithCancel(ctx)
	defer cancelHourglass()
	goroutinelabels.NewGoroutine("agent_sync_hourglass", "refresh agent_task hourglass while sync-loop runs").StartSimple(func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctxHourglass.Done():
				return
			case <-ticker.C:
				flipHourglass(ctxHourglass, secCtx, sp, taskID, proc.ProjectRoot())
				// A live sync-loop is the strongest available liveness signal: the ticker
				// stops when the agent's process does, so renewing here means a missed
				// check-in reflects a dead or wedged seat rather than a quiet one.
				if err := agentclaim.RenewCheckin(proc.ProjectRoot(), taskID); err != nil {
					logging.FluentEvent(logging.GetLogger()).Error("checkin_renew_failed", err).
						String("object_id", taskID).Log()
				}
			}
		}
	})

	poller := time.NewTicker(2 * time.Second)
	defer poller.Stop()

	auditStream := audit.NewAuditStream(proc.ProjectRoot())
	validator := mutation.NewValidator(nil)

	var activeProfile map[string]any
	profileFilter := storage.ListFilter{Kind: objects.KindProviderProfile}
	if profiles, err := sp.List(ctx, secCtx, &storage.StorageContext{}, profileFilter); err == nil && len(profiles.Objects) > 0 {
		activeProfile = profiles.Objects[0]
	}

	var llmClient llm.Client
	llmCfg := llm.DefaultConfig(ctx)

	if activeProfile != nil {
		endpointType := koi.GetString(activeProfile, objects.FieldKeyEndpointType)
		baseURL := koi.GetString(activeProfile, objects.FieldKeyBaseURL)
		modelID := koi.GetString(activeProfile, objects.FieldKeyModelID)

		if baseURL != "" {
			llmCfg.BaseURL = baseURL
		}
		if modelID != "" {
			llmCfg.ChatModel = modelID
		}
		if endpointType != "" {
			llmCfg.Provider = endpointType
		}
	}
	llmClient = llm.NewClient(ctx, llmCfg)
	tokenTracker := swarm.NewTokenTracker(llmCfg.ContextWindowSize, 0.9)

	guardCfg := LoadLoopGuardConfig()
	stagnation := newStagnationGuard(guardCfg.MaxStagnantProgressTicks)
	loopCount := 0

	if wErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf("Starting Graph-State Sync Loop for task: %s\n", taskID))); wErr != nil {
		logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("orchestrator timeout")
		case <-poller.C:
			loopCount++
			if loopCount > guardCfg.MaxSyncLoops {
				// Transition to error state before aborting
				currentTask, rErr := sp.Read(ctx, secCtx, taskID)
				if rErr == nil {
					if aErr := applyStateMutation(ctx, secCtx, sp, taskID, koi.Kind(currentTask), validator, auditStream, objects.ObjectStatusFailed); aErr != nil {
						logging.FluentEvent(logging.GetLogger()).Error("Failed to apply failure state mutation on loop limit", aErr).
							String("task_id", taskID).Log()
					}
				}
				return fmt.Errorf("max sync loop limit reached (%d), aborting to prevent infinite cycle", guardCfg.MaxSyncLoops)
			}
			// 1. FRESH STATE READ EVERY TICK
			bypassCtx := pkgctx.WithBypassCache(ctx)
			currentTask, err := sp.Read(bypassCtx, secCtx, taskID)
			if err != nil {
				return errfmt.Newf("failed to read agent task: %s", taskID).Wrap(err)
			}

			fp := taskProgressFingerprint(currentTask)
			isStagnant := stagnation.lastFP != "" && fp == stagnation.lastFP
			if isStagnant {
				storePath := datacell.AgentIdleStorePath(proc.ProjectRoot())
				if store, err := agentidle.NewFileStore(storePath); err == nil {
					// Hardcoded 2s since the poller is 2s
					if accErr := store.Accumulate("sync-loop-agent", taskID, 2*time.Second); accErr != nil {
						logging.FluentEvent(logging.GetLogger()).Warn("Failed to accumulate agent idle time").
							WithError(accErr).String("task_id", taskID).Log()
					}
				}
			}

			if stagnation.Observe(fp) {
				if aErr := applyStateMutation(ctx, secCtx, sp, taskID, koi.Kind(currentTask), validator, auditStream, objects.ObjectStatusFailed); aErr != nil {
					logging.FluentEvent(logging.GetLogger()).Error("Failed to apply failure state mutation on stagnation", aErr).
						String("task_id", taskID).Log()
				}
				return fmt.Errorf("sync-loop stagnation: no progress for %d ticks (fingerprint unchanged), aborting", guardCfg.MaxStagnantProgressTicks)
			}

			// EXIT CONDITION
			status := koi.Status(currentTask)
			if status == objects.ObjectStatusImplemented || status == objects.ObjectStatusFailed || status == objects.ObjectStatusError {
				if wErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf("Task complete, status: %s\n", status))); wErr != nil {
					logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
				}
				return nil
			}

			// Resolve Bounded Context via QuerySubgraph
			// Reduced depth to 1 to prevent exploding context window with 2nd degree connections
			deps, err := QuerySubgraph(bypassCtx, secCtx, sp, taskID, 1)
			if err != nil {
				logging.FluentEvent(logging.GetLogger()).Error("ERROR at QuerySubgraph", err).Log()
				return errfmt.Newf("subgraph resolution failed").Wrap(err)
			}

			// Build aggregated payload of completeness_validation steps and policy validation_overlays
			var aggregatedValidationSteps []map[string]any

			// 1. Native completeness_validation steps
			cvList := koi.GetSlice(currentTask, objects.FieldKeyCompletenessValidation)
			if cvList == nil {
				// Fallback to task_steps if completeness_validation is absent, preserving existing behavior
				cvList = koi.GetSlice(currentTask, objects.FieldKeyTaskSteps)
			}
			for _, stepRaw := range cvList {
				if stepMap, ok3 := stepRaw.(map[string]any); ok3 {
					aggregatedValidationSteps = append(aggregatedValidationSteps, stepMap)
				}
			}

			// 2. Query for applicable policies and extract validation_overlays
			policyFilter := storage.ListFilter{Kind: objects.KindPolicy}
			if policies, pErr := sp.List(ctx, secCtx, &storage.StorageContext{}, policyFilter); pErr == nil && policies != nil {
				currentKind := koi.Kind(currentTask)
				for _, pol := range policies.Objects {
					applicability := koi.GetMap(pol, objects.FieldKeyApplicability)
					if applicability != nil {
						objectTypes := koi.GetStringSlice(applicability, "object_types")
						applies := false
						for _, ot := range objectTypes {
							if ot == currentKind {
								applies = true
								break
							}
						}
						if applies {
							for _, overlayRaw := range koi.GetSlice(pol, objects.FieldKeyValidationOverlays) {
								if overlayMap, ok3 := overlayRaw.(map[string]any); ok3 {
									aggregatedValidationSteps = append(aggregatedValidationSteps, overlayMap)
								}
							}
						}
					}
				}
			}

			if status == objects.ObjectStatusPendingVerification {
				// The Doer completed its implementation via zqk_agent_next and wants verification.
				// 1. Mark any 'pending_implementation' step as 'verified' (since they are now done)
				// 2. Transition the first 'pending' validation step to 'pending_verification'.
				updated := false
				for i, stepMap := range aggregatedValidationSteps {
					st := koi.Status(stepMap)
					if st == objects.ObjectStatusPendingImplementation {
						koi.SetStatus(stepMap, objects.ObjectStatusVerified)
						aggregatedValidationSteps[i] = stepMap
						updated = true
					} else if st == objects.ObjectStatusPending || st == objects.ObjectStatusRejected || st == "" {
						koi.SetStatus(stepMap, objects.ObjectStatusPendingVerification)
						aggregatedValidationSteps[i] = stepMap
						updated = true
						break
					}
				}
				if updated {
					updates := map[string]any{}
					if _, ok := currentTask[objects.FieldKeyCompletenessValidation]; ok {
						updates[objects.FieldKeyCompletenessValidation] = aggregatedValidationSteps
					} else {
						updates[objects.FieldKeyTaskSteps] = aggregatedValidationSteps
					}
					if uErr := sp.Update(ctx, secCtx, taskID, updates); uErr != nil {
						logging.FluentEvent(logging.GetLogger()).Error("Failed to update validation steps", uErr).
							String("task_id", taskID).Log()
						return errfmt.Newf("failed to update validation steps for task %s", taskID).Wrap(uErr)
					}
				}
				if err := applyStateMutation(ctx, secCtx, sp, taskID, koi.Kind(currentTask), validator, auditStream, objects.ObjectStatusInProgress); err != nil {
					if !strings.Contains(err.Error(), "already exists") {
						logging.FluentEvent(logging.GetLogger()).Error("Failed to transition task to in_progress from pending_verification", err).Log()
						return errfmt.Newf("failed to transition task %s to in_progress", taskID).Wrap(err)
					}
				}
				status = objects.ObjectStatusInProgress
			}

			if status != objects.ObjectStatusInProgress {
				koi.SetStatus(currentTask, objects.ObjectStatusInProgress)
				if err := applyStateMutation(ctx, secCtx, sp, taskID, koi.Kind(currentTask), validator, auditStream, objects.ObjectStatusInProgress); err != nil {
					if strings.Contains(err.Error(), "already exists") {
						logging.FluentEvent(logging.GetLogger()).Warn("Audit event for state mutation already exists, skipping")
					} else {
						return errfmt.Newf("failed to transition task %s to in_progress", taskID).Wrap(err)
					}
				}
			}

			// ---> SYSTEM VERIFICATION LAYER (BLIND, OBJECTIVE, PROGRAMMATIC) <---
			// The verifier does not understand or care about what was implemented. It only evaluates constraints.
			verificationTriggered := false

			// 3. Execute the aggregated payload
			var updatedSteps []any
			hasFailedSteps := false
			allStepsCompleted := true
			for i, stepMap := range aggregatedValidationSteps {
				stepStatus := koi.Status(stepMap)

				if stepStatus == objects.ObjectStatusPending || stepStatus == objects.ObjectStatusPendingImplementation || stepStatus == objects.ObjectStatusPendingVerification || stepStatus == "" {
					// We only process if it's pending OR pending_verification.
					// if pending_implementation, we should have started it above. but just in case it falls through, we skip.
					if stepStatus == objects.ObjectStatusPendingImplementation {
						continue
					}
					allStepsCompleted = false
				}

				// If any step is pending, the system intercepts and executes the dumb programmatic algorithm
				if stepStatus == objects.ObjectStatusPendingVerification {
					verificationTriggered = true

					// Track and check verification attempts
					attempts := koi.GetIntOr(stepMap, objects.FieldKeyVerificationAttempts, 0) + 1
					koi.Set(stepMap, objects.FieldKeyVerificationAttempts, attempts)

					maxAttempts := guardCfg.MaxVerificationAttempts

					if attempts > maxAttempts {
						koi.SetStatus(stepMap, objects.ObjectStatusFailed)
						koi.Set(stepMap, objects.FieldKeyVerificationFeedback, fmt.Sprintf("Max verification loop limit reached (%d/%d attempts). Aborting task.", attempts, maxAttempts))

						// Update the steps slice
						aggregatedValidationSteps[i] = stepMap
						hasFailedSteps = true

						stepFieldUpdates := map[string]any{}
						if _, ok := currentTask[objects.FieldKeyCompletenessValidation]; ok {
							stepFieldUpdates[objects.FieldKeyCompletenessValidation] = aggregatedValidationSteps
						} else {
							stepFieldUpdates[objects.FieldKeyTaskSteps] = aggregatedValidationSteps
						}
						// Persist steps + status in one mutation so a status-only write cannot
						// race and leave steps stuck at pending_verification.
						if errMut := applyStateMutationWithFields(ctx, secCtx, sp, taskID, koi.Kind(currentTask), validator, auditStream, objects.ObjectStatusError, stepFieldUpdates); errMut != nil {
							if wErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf("❌ Failed to apply state mutation: %v\n", errMut))); wErr != nil {
								logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
							}
						}

						if wErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf("❌ Max verification loop limit reached (%d/%d attempts). Transitioning task %s to error.\n", attempts, maxAttempts, taskID))); wErr != nil {
							logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
						}
						updatedSteps = append(updatedSteps, stepMap)
						continue
					}

					// Inject task context into stepMap for verification engine
					stepMap["task_id"] = taskID
					if arts := koi.GetSlice(currentTask, objects.FieldKeyArtifacts); arts != nil {
						stepMap[objects.FieldKeyArtifacts] = arts
					}

					// The system is blind to implementation details. Execute the Universal Verification DSL.
					result, err := verification.RunVerification(ctx, secCtx, sp, stepMap)

					if err != nil {
						koi.SetStatus(stepMap, objects.ObjectStatusFailed)
						koi.Set(stepMap, objects.FieldKeyVerificationFeedback, fmt.Sprintf("Internal Verification Engine Error: %v", err))
					} else if !result.Passed {
						koi.SetStatus(stepMap, objects.ObjectStatusRejected)
						koi.Set(stepMap, objects.FieldKeyVerificationFeedback, fmt.Sprintf("Attempt %d/%d: %s", attempts, maxAttempts, result.Feedback))
					} else {
						koi.SetStatus(stepMap, objects.ObjectStatusVerified)
						koi.Set(stepMap, objects.FieldKeyVerificationFeedback, result.Feedback)
					}

					stepName := koi.GetString(stepMap, objects.FieldKeyName)
					newStepStatus := koi.Status(stepMap)
					auditStream.Publish(ctx, audit.AuditRecord{
						ID:        fmt.Sprintf("overlay-update-%s-%s", taskID, stepName),
						Action:    "validation_overlay",
						Target:    taskID,
						Timestamp: time.Now().Format(time.RFC3339),
						Status:    newStepStatus,
						Overlay:   stepName,
					})

					if wErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf("⚙️ System Verification Result: %v (Feedback: %s)\n", result.Passed, result.Feedback))); wErr != nil {
						logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
					}

					currentStepStatus := koi.Status(stepMap)
					if currentStepStatus == objects.ObjectStatusFailed || currentStepStatus == objects.ObjectStatusRejected {
						hasFailedSteps = true
					}

					// Only evaluate one pending step at a time to maintain the rigid sequence
					updatedSteps = append(updatedSteps, stepMap)
					continue
				}

				currentStepStatus := koi.Status(stepMap)
				if currentStepStatus == objects.ObjectStatusFailed || currentStepStatus == objects.ObjectStatusRejected {
					hasFailedSteps = true
				}

				updatedSteps = append(updatedSteps, stepMap)
			}

			// If the system intercepted a verification step, we persist the result and SKIP the LLM Doer phase
			if verificationTriggered {
				// We update whatever field we sourced from. To be safe, update task_steps and completeness_validation
				updates := map[string]any{}
				if _, ok := currentTask[objects.FieldKeyCompletenessValidation]; ok {
					updates[objects.FieldKeyCompletenessValidation] = updatedSteps
				} else {
					updates[objects.FieldKeyTaskSteps] = updatedSteps
				}

				if uErr := sp.Update(ctx, secCtx, taskID, updates); uErr != nil {
					if wErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf("❌ System verification layer failed to persist update: %v\n", uErr))); wErr != nil {
						logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
					}
				}
				// Continue the sync loop. The Doer will only be summoned if there are no pending_verification steps.
				continue
			}

			if len(aggregatedValidationSteps) == 0 || allStepsCompleted {
				finalStatus := objects.ObjectStatusImplemented
				if hasFailedSteps {
					finalStatus = objects.ObjectStatusError
				} else if isAgentWorktree(proc.ProjectRoot()) {
					// Swarm sync-loop: build-gate then commit worktree branch before teardown.
					wtRoot := proc.ProjectRoot()
					if bErr := worktreeBuildCheck(ctx, wtRoot); bErr != nil {
						finalStatus = objects.ObjectStatusError
						if wErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf("❌ Worktree build gate failed for %s: %v\n", taskID, bErr))); wErr != nil {
							logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
						}
					} else {
						branchName := "agent/" + taskID
						addCmd := execwrap.Command("git", "add", "-A")
						addCmd.Dir = wtRoot
						if addErr := addCmd.Run(); addErr != nil {
							logging.FluentEvent(logging.GetLogger()).Warn("git add failed in worktree").WithError(addErr).Log()
						}
						commitCmd := execwrap.Command("git", "commit", "-m", "Agent implementation for "+taskID)
						commitCmd.Dir = wtRoot
						if commitErr := commitCmd.Run(); commitErr != nil {
							logging.FluentEvent(logging.GetLogger()).Warn("git commit failed in worktree").WithError(commitErr).Log()
						}

						mainRepo, rErr := agentWorktreeMainRepo(wtRoot)
						if rErr != nil {
							finalStatus = objects.ObjectStatusError
							if wErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf("❌ Worktree studio checkout unresolved for %s: %v\n", taskID, rErr))); wErr != nil {
								logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
							}
						} else {
							mergeCmd := execwrap.Command("git", "merge", branchName)
							mergeCmd.Dir = mainRepo
							if out, mErr := mergeCmd.CombinedOutput(); mErr != nil {
								finalStatus = objects.ObjectStatusError
								if wErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf("❌ Worktree merge failed for %s: %v\n%s\n", taskID, mErr, string(out)))); wErr != nil {
									logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
								}
							}
						}
					}
				}
				// Always tear down agent worktrees on terminal (success or fail).
				// previously only cleaned on success → orphan pile.
				if isAgentWorktree(proc.ProjectRoot()) {
					mainRepo, rErr := agentWorktreeMainRepo(proc.ProjectRoot())
					if rErr != nil {
						if wErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf("❌ Worktree teardown skipped for %s: %v\n", taskID, rErr))); wErr != nil {
							logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
						}
					} else {
						maintenanceService := maintenance.NewGitMaintenanceService(mainRepo)
						if cErr := maintenanceService.CleanupWorktreeAndBranchForID(ctx, taskID); cErr != nil {
							logging.FluentEvent(logging.GetLogger()).Warn("Worktree cleanup failed").WithError(cErr).Log()
						}
					}
				}
				currentKind := koi.Kind(currentTask)
				if err := applyStateMutation(ctx, secCtx, sp, taskID, currentKind, validator, auditStream, finalStatus); err != nil {
					if strings.Contains(err.Error(), "already exists") {
						logging.FluentEvent(logging.GetLogger()).Warn("Audit event for state mutation already exists, skipping")
					} else {
						return errfmt.Newf("failed to apply state mutation for %s", taskID).Wrap(err)
					}
				}
				continue
			}
			// ---> END SYSTEM VERIFICATION LAYER <---

			bundle := buildContextBundle(currentTask, deps)

			// Phase 2: Construct prompt and call LLM
			bundleBytes, _ := json.MarshalIndent(bundle, "", "  ")

			taskTitleObj := koi.Title(currentTask)
			taskDescObj := koi.GetString(currentTask, objects.FieldKeyDescription)
			displayTitle := taskTitleObj
			if displayTitle == "" {
				displayTitle = taskID
			}
			if taskDescObj != "" {
				displayTitle += "\n\nDescription:\n" + taskDescObj
			}

			if stepsRaw := koi.GetSlice(currentTask, objects.FieldKeyTaskSteps); stepsRaw != nil {
				if stepsJson, err := json.MarshalIndent(stepsRaw, "", "  "); err == nil {
					displayTitle += "\n\nSteps:\n" + string(stepsJson)
				}
			}

			// Process interjections
			var hasNewInterjections bool
			if interjections := koi.GetSlice(currentTask, objects.FieldKeyInterjections); interjections != nil {
				for i, rawInj := range interjections {
					if inj, isMap := rawInj.(map[string]any); isMap {
						if koi.IsStatus(inj, objects.ObjectStatusUnread) {
							hasNewInterjections = true
							msg := koi.GetString(inj, "message")
							displayTitle += fmt.Sprintf("\n\nCRITICAL OVERRIDE (Interjection from %s at %s):\n%s\n", inj["from"], inj["timestamp"], msg)
							koi.SetStatus(inj, objects.ObjectStatusRead)
							interjections[i] = inj
						}
					}
				}
				if hasNewInterjections {
					currentTask[objects.FieldKeyInterjections] = interjections
					if uErr := sp.Update(ctx, secCtx, taskID, currentTask); uErr != nil {
						logging.FluentEvent(logging.GetLogger()).Error("Failed to update task interjections", uErr).
							String("task_id", taskID).Log()
						return errfmt.Newf("failed to persist interjections for task %s", taskID).Wrap(uErr)
					}
				}
			}

			// 1. Construct Stateless Prompt
			prompt := fmt.Sprintf("Task ID: %s\n\nTask Graph Bundle:\n```json\n%s\n```\n\nAnalyze the context and provide a single mutation to advance the state.", taskID, string(bundleBytes))

			// 2. Invoke LLM with Strict Schema Contract
			systemPrompt := "You are the ZQK Graph-State Sync Loop LLM node. Your goal is to consume the Task Graph Bundle and produce a single state transition mutation. You must strictly output ONLY valid JSON matching the schema.\nSchema:\n" + mutation.OutputSchema()

			// Token budget gate: fit prompt before call
			fittedPrompt, estTokens, budgetOK := tokenTracker.FitUserPrompt(systemPrompt, prompt)
			logging.FluentEvent(logging.GetLogger()).Info("Sync-loop token budget check").
				WithFields(
					logging.Int("estimatedTokens", estTokens),
					logging.Int("budgetLimit", tokenTracker.BudgetLimit()),
					logging.Int("contextWindow", tokenTracker.ContextWindow),
					logging.Bool("fitted", budgetOK),
					logging.Bool("truncated", fittedPrompt != prompt),
				).
				Log()
			if !budgetOK {
				budgetErr := fmt.Errorf("sync-loop prompt exceeds token budget after truncation: %d tokens (limit %d)", estTokens, tokenTracker.BudgetLimit())
				if wErr := cli.WriteOutput(cmd, []byte(budgetErr.Error()+"\n")); wErr != nil {
					logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
				}
				if mErr := applyStateMutation(ctx, secCtx, sp, taskID, koi.Kind(currentTask), validator, auditStream, objects.ObjectStatusFailed); mErr != nil {
					logging.FluentEvent(logging.GetLogger()).Error("Failed to apply failure state mutation on token budget exceeded", mErr).
						String("task_id", taskID).Log()
				}
				return budgetErr
			}
			prompt = fittedPrompt

			resp, err := llmClient.GenerateCompletion(ctx, prompt, systemPrompt)
			if err != nil {
				if wErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf("LLM error: %v\n", err))); wErr != nil {
					logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
				}
				if mErr := applyStateMutation(ctx, secCtx, sp, taskID, koi.Kind(currentTask), validator, auditStream, objects.ObjectStatusFailed); mErr != nil {
					logging.FluentEvent(logging.GetLogger()).Error("Failed to apply failure state mutation on LLM error", mErr).
						String("task_id", taskID).Log()
				}
				return err
			}

			// 3. Validate & Map to Mutation
			var mut mutation.Mutation
			cleanResp := strings.TrimPrefix(strings.TrimSpace(resp), "```json")
			cleanResp = strings.TrimPrefix(cleanResp, "```")
			cleanResp = strings.TrimSuffix(cleanResp, "```")
			cleanResp = strings.TrimSpace(cleanResp)

			if uErr := json.Unmarshal([]byte(cleanResp), &mut); uErr != nil {
				if wErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf("Failed to parse mutation: %v\nRAW:\n%s\n", uErr, cleanResp))); wErr != nil {
					logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
				}
				if mErr := applyStateMutation(ctx, secCtx, sp, taskID, koi.Kind(currentTask), validator, auditStream, objects.ObjectStatusFailed); mErr != nil {
					logging.FluentEvent(logging.GetLogger()).Error("Failed to apply failure state mutation on unmarshal error", mErr).
						String("task_id", taskID).Log()
				}
				continue
			}

			// 4. Validation & Idempotency Key
			ik := mutation.BuildIDKey(taskID, &mut)
			safeIdempotencyKey := "AUD-" + strings.ReplaceAll(ik, "::", "-")
			if _, rErr := sp.Read(ctx, secCtx, safeIdempotencyKey); rErr == nil {
				if wErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf("Mutation %s already committed, skipping.\n", ik))); wErr != nil {
					logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
				}
				continue
			}

			// 5. Audit Hook PreFlight
			auditStream.Publish(ctx, audit.AuditRecord{
				ID:        ik,
				Action:    "pre_flight",
				Target:    taskID,
				Timestamp: time.Now().Format(time.RFC3339),
			})

			// 6. HIL Gate Safety Check
			if vErr := validator.ValidateAndRoute(ctx, taskID, &mut); vErr != nil {
				if bErr := sp.Update(ctx, secCtx, taskID, map[string]any{objects.FieldKeyStatus: objects.ObjectStatusBlocked}); bErr != nil {
					logging.FluentEvent(logging.GetLogger()).Error("Failed to set task status to blocked", bErr).Log()
				}
				if wErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf("Mutation blocked by safety gate: %v\n", vErr))); wErr != nil {
					logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
				}
				continue
			}

			// 7. Commit Mutation to Graph Kernel
			tx, txErr := sp.BeginTransaction(ctx)
			if txErr != nil {
				return errfmt.Newf("failed to begin tx").Wrap(txErr)
			}

			if auditErr := createIdempotencyAuditStamp(ctx, secCtx, tx, ik, taskID, &mut); auditErr != nil {
				if rbErr := tx.Rollback(ctx); rbErr != nil {
					return errfmt.Newf("failed to create idempotency stamp").Wrap(errors.Join(auditErr, rbErr))
				}
				return errfmt.Newf("failed to create idempotency stamp").Wrap(auditErr)
			}

			switch mut.Action {
			case mutation.ActionUpdateNode:
				if len(mut.Fields) > 0 {
					if uErr := tx.Update(ctx, secCtx, mut.TargetID, mut.Fields); uErr != nil {
						if rbErr := tx.Rollback(ctx); rbErr != nil {
							return errfmt.Newf("failed to update node %s", mut.TargetID).Wrap(errors.Join(uErr, rbErr))
						}
						return errfmt.Newf("failed to update node %s", mut.TargetID).Wrap(uErr)
					}
				}
			case mutation.ActionCreateNode:
				if cErr := tx.Create(ctx, secCtx, mut.Fields); cErr != nil {
					if rbErr := tx.Rollback(ctx); rbErr != nil {
						return errfmt.Newf("failed to create node").Wrap(errors.Join(cErr, rbErr))
					}
					return errfmt.Newf("failed to create node").Wrap(cErr)
				}
			}

			if cErr := tx.Commit(ctx); cErr != nil {
				return errfmt.Newf("failed to commit mutation").Wrap(cErr)
			}

			auditStream.Publish(ctx, audit.AuditRecord{
				ID:        ik,
				Action:    "applied",
				Target:    taskID,
				Timestamp: time.Now().Format(time.RFC3339),
				Status:    objects.ObjectStatusSuccess,
			})

			if mut.StatusTransition != "" && !koi.IsStatus(currentTask, mut.StatusTransition) {
				if sErr := sp.Update(ctx, secCtx, taskID, map[string]any{objects.FieldKeyStatus: mut.StatusTransition}); sErr != nil {
					logging.FluentEvent(logging.GetLogger()).Error("Failed to transition task status", sErr).Log()
					return errfmt.Newf("failed to transition task status to %s", mut.StatusTransition).Wrap(sErr)
				}
			}

			if wErr := cli.WriteOutput(cmd, []byte(fmt.Sprintf("Committed mutation %s\n", ik))); wErr != nil {
				logging.FluentEvent(logging.GetLogger()).Error("WriteOutput failed", wErr).Log()
			}
			continue

		}
	}
}

func applyStateMutation(ctx context.Context, secCtx *storage.SecurityContext, sp storage.ObjectStorageProvider, taskID string, kind string, validator *mutation.Validator, auditStream *audit.AuditStream, newStatus string) error {
	return applyStateMutationWithFields(ctx, secCtx, sp, taskID, kind, validator, auditStream, newStatus, nil)
}

// applyStateMutationWithFields transitions status and optionally merges extra fields in the same Update
// so callers do not lose nested writes (e.g. task_steps) to a status-only CAS race.
func applyStateMutationWithFields(ctx context.Context, secCtx *storage.SecurityContext, sp storage.ObjectStorageProvider, taskID string, kind string, validator *mutation.Validator, auditStream *audit.AuditStream, newStatus string, extraFields map[string]any) error {
	mut := mutation.Mutation{
		Action:           mutation.ActionUpdateNode,
		TargetID:         taskID,
		TargetKind:       kind,
		StatusTransition: newStatus,
		Fields: map[string]any{
			objects.FieldKeyStatus: newStatus,
		},
		SafetyClass: mutation.SafetyWrite,
	}
	for k, v := range extraFields {
		if k == objects.FieldKeyStatus || k == objects.FieldKeyID || k == objects.FieldKeyKind {
			continue
		}
		mut.Fields[k] = v
	}

	// Auto-inject missing required assignee_persona_ref to avoid validation errors blocking transition
	taskObj, err := sp.Read(ctx, secCtx, taskID)
	if err == nil {
		if koi.GetString(taskObj, objects.FieldKeyAssigneePersonaRef) == "" {
			mut.Fields[objects.FieldKeyAssigneePersonaRef] = objects.ConstPersonaOrchestratorAlpha
		}
	}

	if err := validator.ValidateAndRoute(ctx, taskID, &mut); err != nil {
		return err
	}

	tx, err := sp.BeginTransaction(ctx)
	if err != nil {
		return err
	}

	ik := mutation.BuildIDKey(taskID, &mut)
	auditStream.Publish(ctx, audit.AuditRecord{
		ID:        ik,
		Action:    string(mut.Action),
		Target:    taskID,
		Timestamp: time.Now().Format(time.RFC3339),
	})

	if auditErr := createIdempotencyAuditStamp(ctx, secCtx, tx, ik, taskID, &mut); auditErr != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return errors.Join(auditErr, rbErr)
		}
		return auditErr
	}

	mut.Fields[objects.FieldKeyID] = taskID
	mut.Fields[objects.FieldKeyKind] = kind
	if err := tx.Update(ctx, secCtx, taskID, mut.Fields); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return errors.Join(err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	auditStream.Publish(ctx, audit.AuditRecord{
		ID:        fmt.Sprintf("status-update-%s", taskID),
		Action:    "status_transition",
		Target:    taskID,
		Timestamp: time.Now().Format(time.RFC3339),
		Status:    newStatus,
	})

	return nil
}

func createIdempotencyAuditStamp(ctx context.Context, secCtx *storage.SecurityContext, tx storage.ObjectTransaction, ik string, taskID string, mut *mutation.Mutation) error {
	safeIdempotencyKey := "AUD-" + strings.ReplaceAll(ik, "::", "-")
	return tx.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyKind:          objects.KindAuditEvent,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyID:            safeIdempotencyKey,
		objects.FieldKeyTitle:         fmt.Sprintf("Mutation Stamp: %s", string(mut.Action)),
		objects.FieldKeyEventType:     "integrity_recovery",
		objects.FieldKeyMetadata: map[string]any{
			"task_id":              taskID,
			"action":               string(mut.Action),
			"safety_class":         string(mut.SafetyClass),
			objects.FieldKeyStatus: objects.ObjectStatusApplied,
			"timestamp":            time.Now().Format(time.RFC3339),
		},
	})
}

func transitionToError(ctx context.Context, secCtx *storage.SecurityContext, sp storage.ObjectStorageProvider, taskID string, currentTask map[string]any, validator *mutation.Validator, auditStream *audit.AuditStream) {
	if err := applyStateMutation(ctx, secCtx, sp, taskID, koi.Kind(currentTask), validator, auditStream, objects.ObjectStatusError); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error("Failed to transition to error", err).Log()
	}
}

func flipHourglass(ctx context.Context, secCtx *storage.SecurityContext, sp storage.ObjectStorageProvider, taskID string, projectRoot string) {
	if err := sp.Update(ctx, secCtx, taskID, map[string]any{
		"agent_heartbeat": time.Now().Format(time.RFC3339),
		"agent_pid":       os.Getpid(),
	}); err != nil {
		logging.FluentEvent(logging.GetLogger()).Warn("flipHourglass update failed").WithError(err).
			String("task_id", taskID).Log()
	}

	if projectRoot != "" {
		expiresAt := time.Now().Add(5 * time.Minute)
		schedulerRoot := paths.ResolvePathFromCacheOrConstant(projectRoot, "scheduler", filepath.Join(paths.ProjectDataDir, paths.SchedulerDir))
		hourglassDir := filepath.Join(schedulerRoot, "hourglass")
		if err := fileutil.EnsureDir(hourglassDir); err != nil {
			logging.FluentEvent(logging.GetLogger()).Warn("flipHourglass ensure dir failed").WithError(err).Log()
		}
		filePath := filepath.Join(hourglassDir, taskID+".json")
		data := map[string]any{
			"task_id":                 taskID,
			"pid":                     os.Getpid(),
			objects.FieldKeyExpiresAt: expiresAt.Format(time.RFC3339),
		}
		if b, err := json.Marshal(data); err == nil {
			if wErr := fileutil.WriteStandardFile(filePath, b); wErr != nil {
				logging.FluentEvent(logging.GetLogger()).Warn("flipHourglass write file failed").WithError(wErr).Log()
			}
		}
	}
}

func removeHourglass(ctx context.Context, secCtx *storage.SecurityContext, sp storage.ObjectStorageProvider, taskID string, projectRoot string) {
	// If sync-loop context is canceled (timeout or interrupt), sp.Update will fail. Use a detached context.
	cleanupCtx := context.WithoutCancel(ctx)
	if err := sp.Update(cleanupCtx, secCtx, taskID, map[string]any{
		"agent_pid": nil,
	}); err != nil {
		logging.FluentEvent(logging.GetLogger()).Warn("removeHourglass update failed").WithError(err).
			String("task_id", taskID).Log()
	}
	if projectRoot != "" {
		schedulerRoot := paths.ResolvePathFromCacheOrConstant(projectRoot, "scheduler", filepath.Join(paths.ProjectDataDir, paths.SchedulerDir))
		hourglassDir := filepath.Join(schedulerRoot, "hourglass")
		filePath := filepath.Join(hourglassDir, taskID+".json")
		if err := fileutil.Remove(filePath); err != nil && !os.IsNotExist(err) {
			logging.FluentEvent(logging.GetLogger()).Warn("removeHourglass remove file failed").WithError(err).Log()
		}
	}
}
