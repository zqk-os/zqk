// TRACK: orchestrate pipeline decomposition
package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentdelivery"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/authcred"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/federation/meshbroker"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/infrastructure/crypto"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/objects/koi"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/policy"
	"github.com/zqk-os/zqk/pkg/primaryorch"
	audit_event "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/audit"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

var (
// TestDelivererOverride allows integration tests to mock agent delivery.
)

// OrchestrateOptions holds the execution options for orchestrate

func runOrchestrate(cmd *cobra.Command, planArg string, opts OrchestrateOptions) error {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return err
	}

	if proc.ProjectRoot() == "" {
		return errfmt.Errorf("project root is required")
	}

	// Doers cannot orchestrate peers.
	if err := authcred.DenyOrchestrateIfDoer(proc.SecurityContext()); err != nil {
		return err
	}

	// ---> NATIVE CIRCUIT BREAKER INJECTION <---
	// To prevent autonomous runaway swarm explosions, enforce a global cap on in_progress tasks
	// before we even spin up the pipeline builder or do any LLM evaluation.
	// sp := proc.Storage()
	// if sp != nil {
	// 	capFilter := storage.ListFilter{
	// 		Kind: objects.KindAgentTask,
	// 		Filters: map[string]any{
	// 			objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	// 		},
	// 	}
	// 	capRes, err := sp.List(proc.OperationContext(), proc.SecurityContext(), pkgctx.NewStorageContext(), capFilter)
	// 	if err == nil && capRes != nil && len(capRes.Objects) >= 10 {
	// 		_ = cli.WriteOutput(cmd, []byte("\n🛑 SWARM CIRCUIT BREAKER ACTIVATED 🛑\nThere are 10 or more in_progress agent tasks active in the kernel. Halting orchestrator to prevent autonomous flood.\n\n"))
	// 		return nil
	// 	}
	// }
	// ---> END CIRCUIT BREAKER <---

	// Use a bounded orchestration context
	effectiveTimeout := resolveOrchestrationTimeout(opts.Timeout, cmd)
	ctx, ctxCancel := context.WithTimeout(proc.OperationContext(), effectiveTimeout)
	defer ctxCancel()

	b := pipeline.NewInstrumentedBuilder("cap_orchestrator", nil)

	b.AddStage("resolve_semantic_routing", func(pctx *pipeline.Context, payload any) (any, error) {
		state := payload.(*orchestratorState)
		if state.planArg == "" {
			// Query workflow whats-next logic to find active priority plan and mission
			req := &whatsnext.QueryRequest{
				PersonaID:   state.opts.PersonaID,
				SkipMeasure: true,
			}
			out, err := whatsnext.Execute(pctx.Ctx, state.sp, state.proc.ProjectRoot(), req)
			if err != nil {
				return nil, errfmt.Newf("failed to execute whats-next").Wrap(err)
			}
			if out.PriorityPlan == nil {
				return nil, errfmt.Errorf("no active priority plan or strategic plan found to orchestrate (via whats-next)")
			}
			if out.PriorityPlan.Status != objects.ObjectStatusActive && out.PriorityPlan.Status != objects.ObjectStatusInProgress {
				return nil, errfmt.Errorf("orchestrator can only operate on active or in_progress priority plans (got '%s')", out.PriorityPlan.Status)
			}
			state.planID = out.PriorityPlan.ID
			return state, nil
		}

		planID, err := state.proc.ResolveSemanticArgument(pctx.Ctx, objects.KindPriorityPlan, state.planArg)
		if err != nil {
			pipelineID, pErr := state.proc.ResolveSemanticArgument(pctx.Ctx, objects.KindPipeline, state.planArg)
			if pErr != nil {
				stratID, sErr := state.proc.ResolveSemanticArgument(pctx.Ctx, objects.KindStrategicPlan, state.planArg)
				if sErr != nil {
					return nil, errfmt.Newf("semantic routing failed for priority_plan, pipeline, and strategic_plan").Wrap(err)
				}
				state.planID = stratID
			} else {
				state.planID = pipelineID
				state.isPipeline = true
			}
		} else {
			state.planID = planID
		}
		return state, nil
	})

	b.AddStage("load_plan", func(pctx *pipeline.Context, payload any) (any, error) {
		state := payload.(*orchestratorState)
		planRaw, err := state.sp.Read(pctx.Ctx, state.secCtx, state.planID)
		if err != nil {
			return nil, errfmt.Newf("failed to load plan %s", state.planID).Wrap(err)
		}
		state.title, _ = planRaw[objects.FieldKeyTitle].(string)
		planKind, _ := planRaw[objects.FieldKeyKind].(string)
		if planKind == objects.KindPipeline {
			state.isPipeline = true
		} else if planKind == objects.KindStrategicPlan {
			state.isStrategicPlan = true
		} else if planKind == objects.KindPriorityPlan {
			st, _ := planRaw[objects.FieldKeyStatus].(string)
			if st != objects.ObjectStatusActive && st != objects.ObjectStatusInProgress {
				return nil, errfmt.Errorf("orchestrator can only operate on active or in_progress priority plans (got '%s')", st)
			}
		}
		return state, nil
	})

	b.AddStage("fetch_backlog_items", func(pctx *pipeline.Context, payload any) (any, error) {
		state := payload.(*orchestratorState)
		if state.isPipeline {
			planRaw, _ := state.sp.Read(pctx.Ctx, state.secCtx, state.planID)
			refsAny := planRaw[objects.FieldKeyAgentTaskRefs]
			var items []map[string]any
			if refs, ok := refsAny.([]any); ok {
				for _, rAny := range refs {
					if r, okStr := rAny.(string); okStr {
						if task, err := state.sp.Read(pctx.Ctx, state.secCtx, r); err == nil {
							items = append(items, task)
						}
					}
				}
			}
			state.items = items
		} else if state.isStrategicPlan {
			planRaw, _ := state.sp.Read(pctx.Ctx, state.secCtx, state.planID)
			var planPersonas []string
			if planRaw != nil {
				if pRefs, ok := planRaw[objects.FieldKeyPersonaRefs].([]any); ok {
					for _, p := range pRefs {
						if ps, okStr := p.(string); okStr && ps != "" {
							planPersonas = append(planPersonas, ps)
						}
					}
				}
			}
			if len(planPersonas) == 0 {
				planPersonas = []string{objects.ConstPersonaDefaultAgent}
			}
			state.items = []map[string]any{
				{
					objects.FieldKeyID:          fmt.Sprintf("STRAT-REV-%s-CEO", state.planID),
					objects.FieldKeyTitle:       fmt.Sprintf("CEO Strategic Review: %s", state.title),
					objects.FieldKeyDescription: "Evaluate competitor gaps, market readiness, and overall system performance.",
					"sub_agent":                 "chief-executive-officer",
					objects.FieldKeyPersonaRefs: planPersonas,
				},
				{
					objects.FieldKeyID:          fmt.Sprintf("STRAT-REV-%s-ARCH", state.planID),
					objects.FieldKeyTitle:       fmt.Sprintf("Architectural Health Review: %s", state.title),
					objects.FieldKeyDescription: "Audit code organization, pattern compliance, and performance/spaghettification concerns.",
					"sub_agent":                 "system-architect",
					objects.FieldKeyPersonaRefs: planPersonas,
				},
				{
					objects.FieldKeyID:          fmt.Sprintf("STRAT-REV-%s-SEC", state.planID),
					objects.FieldKeyTitle:       fmt.Sprintf("Security & Provenance Review: %s", state.title),
					objects.FieldKeyDescription: "Verify IP provenance, sandboxing, and zero-trust/keystore compliance.",
					"sub_agent":                 "security-reviewer",
					objects.FieldKeyPersonaRefs: planPersonas,
				},
				{
					objects.FieldKeyID:          fmt.Sprintf("STRAT-REV-%s-QA", state.planID),
					objects.FieldKeyTitle:       fmt.Sprintf("Quality & Testing Audit: %s", state.title),
					objects.FieldKeyDescription: "Audit test runner status, package test coverage, and TDD compliance.",
					"sub_agent":                 "qa-auditor",
					objects.FieldKeyPersonaRefs: planPersonas,
				},
				{
					objects.FieldKeyID:          fmt.Sprintf("STRAT-REV-%s-HATER", state.planID),
					objects.FieldKeyTitle:       fmt.Sprintf("Hater Critical Review: %s", state.title),
					objects.FieldKeyDescription: "Expose overly optimistic assumptions, call out hidden technical debt, and challenge assertions.",
					"sub_agent":                 "hater-persona",
					objects.FieldKeyPersonaRefs: planPersonas,
				},
			}
		} else {
			items, err := state.sp.List(pctx.Ctx, state.secCtx, nil, storage.ListFilter{
				Kind:    objects.KindBacklogItem,
				Filters: map[string]any{objects.FieldKeyPriorityPlanRef: state.planID},
			})
			if err != nil {
				return nil, errfmt.Newf("failed to load backlog items for plan: %s", state.planID).Wrap(err)
			}
			var active []map[string]any
			seenIDs := make(map[string]bool)
			for _, obj := range items.Objects {
				k := koi.Wrap(obj)
				st := k.Status()
				planRef := k.GetString(objects.FieldKeyPriorityPlanRef)
				id := k.ID()
				if planRef == state.planID && backlogItemEligibleForOrchestration(st) {
					if !seenIDs[id] {
						seenIDs[id] = true
						active = append(active, obj)
					}
				}
			}
			state.items = active
		}
		return state, nil
	})

	b.AddStage("filter_persona", func(pctx *pipeline.Context, payload any) (any, error) {
		state := payload.(*orchestratorState)
		state.personaRole = "coder-agent"
		if state.opts.PersonaID != "" {
			pObj, err := state.sp.Read(pctx.Ctx, state.secCtx, state.opts.PersonaID)
			if err == nil {
				if r := koi.GetString(pObj, objects.FieldKeyRole); r != "" {
					state.personaRole = r
				}
			}
			before := len(state.items)
			var filtered []map[string]any
			for _, item := range state.items {
				// Soft match: unassigned items stay; only explicit other-persona assignments drop.
				if hasPersonaMatch(item, []string{state.opts.PersonaID}) {
					filtered = append(filtered, item)
				}
			}
			state.items = filtered
			if before > 0 && len(filtered) == 0 {
				logging.FluentEvent(state.proc.Logger()).Warn("Persona filter excluded all items (explicit non-matching persona_refs)").
					PersonaRef(state.opts.PersonaID).
					Count(before).
					Log()
				_ = cli.WriteOutput(state.cmd, []byte(fmt.Sprintf(
					"⚠️  Persona filter %s excluded all %d item(s); each had non-matching persona assignment.\n",
					state.opts.PersonaID, before,
				)))
			} else if dropped := before - len(filtered); dropped > 0 {
				_ = cli.WriteOutput(state.cmd, []byte(fmt.Sprintf(
					"ℹ️  Persona filter %s kept %d/%d (unassigned kept; %d excluded by explicit assignment).\n",
					state.opts.PersonaID, len(filtered), before, dropped,
				)))
			}
		}
		return state, nil
	})

	// Skip non-actionable backlog items on dispatch.
	b.AddStage("filter_shovel_ready", func(pctx *pipeline.Context, payload any) (any, error) {
		state := payload.(*orchestratorState)
		if state.isStrategicPlan {
			return state, nil
		}
		before := len(state.items)
		var kept []map[string]any
		var skipped int
		var skippedDetails []string
		for _, item := range state.items {
			res := validation.EvaluateShovelReady(item)
			if res.Ready {
				kept = append(kept, item)
				continue
			}
			skipped++
			id := koi.ID(item)
			missingList := strings.Join(res.Missing, ", ")
			skippedDetails = append(skippedDetails, fmt.Sprintf("%s lacking [%s]", id, missingList))
			logging.FluentEvent(state.proc.Logger()).Info("orchestrate skipped non-shovel-ready BLI").
				ObjectID(id).
				String("missing", strings.Join(res.Missing, ",")).
				String("criteria_ref", validation.CriteriaIDShovelReady).
				Log()
		}
		state.items = kept
		if skipped > 0 {
			detailStr := ""
			if len(skippedDetails) > 0 {
				detailStr = fmt.Sprintf(": %s", strings.Join(skippedDetails, "; "))
			}
			_ = cli.WriteOutput(state.cmd, []byte(fmt.Sprintf(
				"ℹ️  Shovel-ready gate (%s) kept %d/%d (skipped %d lacking CRI-SHOVEL-READY fields%s).\n",
				validation.CriteriaIDShovelReady, len(kept), before, skipped, detailStr,
			)))
		}
		return state, nil
	})

	// Withhold downstream dependent tasks until upstream deliverables are verified (CRIT-QA-SHOCKWAVE-EVENT-DISPATCH).
	b.AddStage("gate_shockwave_dependencies", func(pctx *pipeline.Context, payload any) (any, error) {
		state := payload.(*orchestratorState)
		if state.isStrategicPlan {
			return state, nil
		}
		var kept []map[string]any
		var withheldCount int
		for _, item := range state.items {
			if isBlockedByUnverifiedUpstream(pctx.Ctx, state.sp, state.proc.SecurityContext(), item) {
				withheldCount++
				logging.FluentEvent(state.proc.Logger()).Info("orchestrate withheld downstream dependent task awaiting upstream shockwave delivery").
					ObjectID(koi.ID(item)).
					String("criteria_ref", "CRIT-QA-SHOCKWAVE-EVENT-DISPATCH").
					Log()
				continue
			}
			if upstreamSec := resolveVerifiedUpstreamDeliverables(pctx.Ctx, state.sp, state.proc.SecurityContext(), item); upstreamSec != "" {
				item["upstream_deliverables_section"] = upstreamSec
			}
			kept = append(kept, item)
		}
		if withheldCount > 0 {
			_ = cli.WriteOutput(state.cmd, []byte(fmt.Sprintf(
				"ℹ️  Shockwave dependency gate (CRIT-QA-SHOCKWAVE-EVENT-DISPATCH): withheld %d dependent task(s) awaiting verified upstream artifacts.\n",
				withheldCount,
			)))
		}
		state.items = kept
		return state, nil
	})

	b.AddStage("load_contexts", func(pctx *pipeline.Context, payload any) (any, error) {
		state := payload.(*orchestratorState)
		if state.opts.AmbientContext != "" {
			if content, readErr := fileutil.ReadFile(state.opts.AmbientContext); readErr == nil {
				state.ambientSection = fmt.Sprintf("\n## Ambient IDE/CLI Context\n%s\n", string(content))
			} else {
				state.ambientSection = fmt.Sprintf("\n## Ambient IDE/CLI Context\n%s\n", state.opts.AmbientContext)
			}
		}

		return state, nil
	})

	b.AddStage("dispatch_parallel", func(pctx *pipeline.Context, payload any) (any, error) {
		state := payload.(*orchestratorState)
		numItems := len(state.items)
		errs := make([]error, numItems)

		pool := goroutinelabels.NewPool(
			goroutinelabels.DefaultBudget(),
			"orchestrator",
			"dispatching concurrent tasks",
			min(numItems, nativeSwarmConcurrency),
			numItems,
		)
		pool.Start(pctx.Ctx)

		sharedMCPTransport := meshbroker.NewMCPClientTransport("")
		defer sharedMCPTransport.Close()

		for i, item := range state.items {
			i, item := i, item
			kItem := koi.Wrap(item)
			title := kItem.Title()
			state.processedItems = append(state.processedItems, title)
			capability := kItem.GetString(objects.FieldKeyCapabilityType)

			_ = pool.Submit(pctx.Ctx, func(workerCtx context.Context) error {
				clipkg.ResetTimeout()
				var agentDeliverer = TestDelivererOverride

				if capability != "" {
					computeBroker := meshbroker.NewComputeBroker(state.sp)
					pod, err := computeBroker.FindCapabilityPod(workerCtx, capability)
					if err == nil {
						endpoint := koi.GetString(pod, objects.FieldKeyEndpoint)
						agentDeliverer = &agentdelivery.TDEEnforcer{Next: agentdelivery.NewMCPDeliverer(sharedMCPTransport, endpoint, "zqk_compute_dispatch")}
					}
				}

				subAgent := state.personaRole
				if subAgentVal := kItem.GetString("sub_agent"); subAgentVal != "" {
					subAgent = subAgentVal
				}

				skillEnforcement, _ := agentprompt.LoadRelevantSkills(workerCtx, state.sp, state.secCtx, fmt.Sprintf("Process task: %s on plan %s", title, title))

				var meshSkillSection string
				if skillEnforcement == nil || len(skillEnforcement.RelevantSkills) == 0 {
					broker := meshbroker.NewSkillBroker(state.sp, nil, state.proc.ProjectRoot())
					if subAgent == "coder_agent" {
						if skillID := localSkillIDByTitle(workerCtx, state.sp, state.secCtx, "Go AST Expert"); skillID != "" {
							remoteSkill, err := broker.EnsureSkill(workerCtx, skillID)
							if err == nil {
								meshSkillSection = fmt.Sprintf("\n## Leased Mesh Capability\n- Skill: %s\n- Provider: %s\n- Token: %s\n- Status: %s\n",
									"Go AST Expert", remoteSkill.Provider, remoteSkill.Token, "Authenticated")
								if remoteSkill.Endpoint != "" {
									agentDeliverer = &agentdelivery.TDEEnforcer{Next: agentdelivery.NewMCPDeliverer(sharedMCPTransport, remoteSkill.Endpoint, "zqk_agent_process_prompt")}
								}
							}
						}
					}
				}

				executionTarget := strings.ToLower(strings.TrimSpace(kItem.GetString("execution_target")))
				if executionTarget == "" {
					executionTarget = strings.ToLower(strings.TrimSpace(kItem.GetString("runtime")))
				}
				if executionTarget == "" && (strings.HasPrefix(strings.ToLower(subAgent), "gemini") || subAgent == "subagent") {
					executionTarget = "subagent"
				}

				if agentDeliverer == nil && executionTarget == "subagent" {
					if agentdelivery.DefaultSubagentDeliverer != nil {
						agentDeliverer = agentdelivery.DefaultSubagentDeliverer
					}
				}

				itemID := kItem.ID()
				if itemID == "" {
					itemID = fmt.Sprintf("generated-%d", time.Now().UnixNano())
				}

				personaID := state.opts.PersonaID
				if personaID == "" {
					if assignee := kItem.GetString(objects.FieldKeyAssigneePersonaRef); assignee != "" {
						personaID = assignee
					} else {
						refs := kItem.GetStringSlice(objects.FieldKeyPersonaRefs)
						if len(refs) > 0 {
							personaID = refs[0]
						}
					}
				}

				if itemID != "" && !strings.HasPrefix(itemID, "generated-") {
					// Pre-flight Graph Validation (CRIT-1)
					prepared, preFlightErr := AssemblePreparedContext(workerCtx, state.secCtx, state.sp, PreparedContextInput{
						TaskID:      itemID,
						ProjectRoot: state.proc.ProjectRoot(),
						Depth:       5,
					})
					if preFlightErr == nil && !prepared.HasSemanticContext() {
						err := errfmt.Errorf("traceability_gap: Task context for '%s' (ID: %s) lacks concrete semantic instructions (description or criteria). Ensure the graph is linked.", title, itemID)
						errs[i] = err
						return err
					}
				}

				if personaID == "" {
					personaID = authcred.DefaultSwarmWorkerAccount
				}

				upstreamDeliverables, _ := kItem.Raw()["upstream_deliverables_section"].(string)
				if upstreamDeliverables == "" {
					upstreamDeliverables = resolveVerifiedUpstreamDeliverables(workerCtx, state.sp, state.secCtx, kItem.Raw())
				}

				if agentDeliverer == nil {
					titleStr := fmt.Sprintf("Execute Task: %s", title)
					envelope := buildOrchestrationTaskEnvelope(
						workerCtx,
						state,
						subAgent,
						personaID,
						capability,
						title,
						meshSkillSection,
						upstreamDeliverables,
					)
					promptMarkdown := buildOrchestrationTaskPrompt(
						workerCtx,
						state,
						subAgent,
						personaID,
						capability,
						title,
						meshSkillSection,
						upstreamDeliverables,
					)

					existingID, existingStatus, existingDisp, findErr := findExistingOrchestrationTask(workerCtx, state, title, itemID)
					if findErr != nil {
						errs[i] = findErr
						return findErr
					}
					if existingDisp == orchDispositionSkipDone {
						_ = cli.WriteOutput(state.cmd, []byte(fmt.Sprintf("ℹ️  Skipping '%s': existing ATK %s is %s.\n", titleStr, existingID, existingStatus)))
						return nil
					}
					if existingDisp == orchDispositionReplace && existingID != "" {
						_ = cli.WriteOutput(state.cmd, []byte(fmt.Sprintf("ℹ️  Replacing '%s': existing ATK %s is %s (not reused).\n", titleStr, existingID, existingStatus)))
					}

					modelTier := "tier_2_simple"
					if tVal := kItem.GetString(objects.FieldKeyModelTier); tVal != "" {
						modelTier = tVal
					}

					var taskSteps []map[string]any
					taskSteps = append(taskSteps, map[string]any{
						objects.FieldKeyTitle:       "Implementation Phase",
						objects.FieldKeyStatus:      objects.ObjectStatusPendingImplementation,
						objects.FieldKeyDescription: paths.RewriteCanonicalCLIInvocations("Resolve the task envelope refs (`zqk object get`, `zqk agent prepare-context`) and execute the dynamic task. Do not copy policy or skill bodies onto the task object."),
					})

					// Persona-Bound Task Boundary: Do not inject codebase validation for TPMs or Design personas
					lowerPersona := strings.ToLower(personaID)
					isTechnical := !strings.Contains(lowerPersona, "tpm") && !strings.Contains(lowerPersona, "design")

					if !isTechnical || strings.Contains(lowerPersona, "architect") {
						taskSteps = append(taskSteps, map[string]any{
							objects.FieldKeyTitle:       "Kernel Graph Composition Context",
							objects.FieldKeyStatus:      objects.ObjectStatusPending,
							objects.FieldKeyDescription: paths.RewriteCanonicalCLIInvocations("Kernel graph composition: use `zqk object get` for the onboarding policy, then `zqk intake` / `zqk object import`; do not script `zqk object create` loops."),
						})
					}

					if isTechnical {
						taskSteps = append(taskSteps, map[string]any{
							objects.FieldKeyTitle:       "Validation Phase",
							"verification_strategy":     "command_exit_code",
							objects.FieldKeyStatus:      objects.ObjectStatusPending,
							objects.FieldKeyDescription: "Must pass targeted validation script.",
							objects.FieldKeyCommand:     paths.RewriteCanonicalCLIInvocations("./bin/zqk agent validate"),
						})
					}
					agentTask := map[string]any{
						objects.FieldKeyKind:          objects.KindAgentTask,
						objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
						objects.FieldKeyTitle:         titleStr,
						// Persist refs, not a pasted prompt catalog.
						objects.FieldKeyDescription:        envelope.Description,
						objects.FieldKeyEstimatedEffort:    "1d",
						objects.FieldKeyAssigneePersonaRef: personaID,

						objects.FieldKeyStatus:    orchestratedTaskStatus,
						objects.FieldKeyModelTier: modelTier,
						objects.FieldKeyTaskSteps: taskSteps,
					}
					if len(envelope.PolicyRefs) > 0 {
						agentTask[objects.FieldKeyPolicyRefs] = envelope.PolicyRefs
					}
					var relatedRefs []string
					if len(envelope.SkillRefs) > 0 {
						relatedRefs = append(relatedRefs, envelope.SkillRefs...)
					}
					if itemID != "" && !strings.HasPrefix(itemID, "generated-") {
						relatedRefs = append(relatedRefs, itemID)
					}
					if len(relatedRefs) > 0 {
						agentTask[objects.FieldKeyRelatedObjectRefs] = relatedRefs
					}
					if state.isPipeline {
						agentTask[objects.FieldKeyPipelineRef] = state.planID
					}
					if !state.isPipeline && !state.isStrategicPlan {
						agentTask[objects.FieldKeyPriorityPlanRef] = state.planID
					}

					var taskID string
					if existingID != "" && existingDisp == orchDispositionReuse {
						taskID = existingID
						_ = cli.WriteOutput(state.cmd, []byte(fmt.Sprintf("♻️  Reusing ATK %s (status=%s) for '%s'\n", taskID, existingStatus, titleStr)))
					} else {
						startEvent := map[string]any{
							objects.FieldKeyKind:          objects.KindAuditEvent,
							objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
							objects.FieldKeyEventType:     string(audit_event.EventTypePromptRunStart),
							objects.FieldKeyTitle:         fmt.Sprintf("Prompt Run Start: %s", title),
							objects.FieldKeyMetadata: map[string]any{
								"run_id": state.opts.SessionID,
								"item":   title,
							},
						}
						_ = state.sp.Create(workerCtx, state.secCtx, startEvent)

						err := state.sp.Create(pkgctx.WithPromoteOnCreate(workerCtx), state.secCtx, agentTask)
						errs[i] = err

						if err != nil {
							errorEvent := map[string]any{
								objects.FieldKeyKind:          objects.KindAuditEvent,
								objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
								objects.FieldKeyEventType:     string(audit_event.EventTypePromptRunError),
								objects.FieldKeyTitle:         fmt.Sprintf("Prompt Run Error: %s", title),
								objects.FieldKeyMetadata: map[string]any{
									"run_id": state.opts.SessionID,
									"item":   title,
									"error":  err.Error(),
								},
							}
							_ = state.sp.Create(workerCtx, state.secCtx, errorEvent)
						} else {
							createdID := koi.ID(agentTask)
							if createdID == "" {
								err = errfmt.Errorf("created agent task has no id")
								errs[i] = err
								return err
							}
							taskID = createdID
						}
					}
					if taskID != "" {
						if err = waitForAgentTaskReadable(workerCtx, state.sp, state.secCtx, taskID); err != nil {
							errs[i] = err
							return err
						}

						// Mark the backlog item in_progress only if TDD posture is satisfied:
						// All criteria must be linked to a test case in a ready/active state.
						if itemID != "" && !strings.HasPrefix(itemID, "generated-") {
							if kItem.Status() != "in_progress" {
								if verifyBLITDDReady(workerCtx, state.sp, state.secCtx, item) {
									kItem.SetStatus("in_progress")
									_ = state.sp.Update(workerCtx, state.secCtx, itemID, item)
								} else {
									logging.FluentEvent(logging.GetLoggerFromContext(workerCtx)).Warn("TDD Gate: Backlog item criteria not linked to ready test cases; in_progress transition held").
										ItemID(itemID).Log()
								}
							}
						}

						completeEvent := map[string]any{
							objects.FieldKeyKind:          objects.KindAuditEvent,
							objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
							objects.FieldKeyEventType:     string(audit_event.EventTypePromptRunComplete),
							objects.FieldKeyTitle:         fmt.Sprintf("Prompt Run Complete: %s", title),
							objects.FieldKeyMetadata: map[string]any{
								"run_id":      state.opts.SessionID,
								"item":        title,
								"delivery_to": "agent_task:graph",
							},
						}
						_ = state.sp.Create(workerCtx, state.secCtx, completeEvent)

						{
							// Local/native swarm executor for simple/routine/light tiers.
							// Complex tiers wake primary (IDE/AGY) instead of starving on a
							// vendor queue labeled "Gemini". Default BLI/ATK tier is tier_2_simple.
							// align model_tier enum (tier_1_complex vs tier_1_routine) in specs.
							if !nativeSwarmEligible(modelTier) {
								_ = cli.WriteOutput(state.cmd, []byte(fmt.Sprintf("ℹ️  Queued '%s' for primary claim (skipped native swarm for %s)\n", taskID, modelTier)))
								// Wake host/project primary orchestrator so work is not stranded.
								msg := fmt.Sprintf("Orchestrate queued ATK %s (%s) — consider agent execute or claim. title=%s", taskID, modelTier, titleStr)
								if _, werr := primaryorch.WakePrimary(workerCtx, state.proc.ProjectRoot(), primaryorch.WakeRequest{
									TaskID:  taskID,
									Persona: personaID,
									Message: msg,
								}); werr != nil {
									_ = cli.WriteOutput(state.cmd, []byte(fmt.Sprintf("⚠️ primaryorch wake failed for %s: %v\n", taskID, werr)))
								}
								_, _ = agentfeed.AppendEvent(agentfeed.AppendEventInput{
									ProjectRoot:      state.proc.ProjectRoot(),
									Message:          fmt.Sprintf("Complex task %s dispatched to primary/hosted seat (%s, tier: %s)", taskID, titleStr, modelTier),
									AgentID:          "orchestration_engine",
									ToAgentID:        personaID,
									Sender:           agentfeed.FeedSenderHumanSteer,
									EventType:        agentfeed.FeedEventTypeSteering,
									SkipEnabledCheck: true,
								})
							} else {
								_, _ = agentfeed.AppendEvent(agentfeed.AppendEventInput{
									ProjectRoot:      state.proc.ProjectRoot(),
									Message:          fmt.Sprintf("Routine task %s dispatched to native/local swarm worker (%s, tier: %s)", taskID, titleStr, modelTier),
									AgentID:          "orchestration_engine",
									Sender:           agentfeed.FeedSenderHumanSteer,
									EventType:        agentfeed.FeedEventTypeMeshStatus,
									SkipEnabledCheck: true,
								})
								inTest := zqkenv.IsInTest()
								workClass := agentprompt.ClassifyWorkClass(title, promptMarkdown)
								worktreePath := state.proc.ProjectRoot()
								baseSHA := ""
								if !inTest {
									if workClass.IsDocsEval() {
										logging.FluentEvent(state.proc.Logger()).Info("native executor using studio project root for docs_eval ATK").
											String("task_id", taskID).
											String("work_class", string(workClass)).
											Log()
										var gitErr error
										baseSHA, gitErr = gitWorktreeOutput(workerCtx, state.proc.ProjectRoot(), "rev-parse", "HEAD")
										if gitErr != nil {
											baseSHA = ""
										}
									} else {
										// Isolated git worktree for the subagent. Reuse a leftover
										// checkout/branch from a prior error cycle — do not remint.
										var wtErr error
										worktreePath, wtErr = ensureOrchestrationWorktree(workerCtx, state.proc.ProjectRoot(), taskID)
										if wtErr != nil {
											_ = persistOrchestratedTaskOutcome(
												workerCtx,
												state,
												taskID,
												objects.ObjectStatusError,
												map[string]any{objects.FieldKeyType: "executor_failure", "error": wtErr.Error()},
											)
											errs[i] = wtErr
											return wtErr
										}
										if seedErr := seedAgentWorktreeRuntime(state.proc.ProjectRoot(), worktreePath); seedErr != nil {
											_ = persistOrchestratedTaskOutcome(
												workerCtx,
												state,
												taskID,
												objects.ObjectStatusError,
												map[string]any{objects.FieldKeyType: "executor_failure", "error": seedErr.Error()},
											)
											errs[i] = seedErr
											return seedErr
										}
										var gitErr error
										baseSHA, gitErr = gitWorktreeOutput(workerCtx, worktreePath, "rev-parse", "HEAD")
										if gitErr != nil {
											_ = persistOrchestratedTaskOutcome(
												workerCtx,
												state,
												taskID,
												objects.ObjectStatusError,
												map[string]any{objects.FieldKeyType: "executor_failure", "error": gitErr.Error()},
											)
											errs[i] = gitErr
											return gitErr
										}
									}

									// Must be absolute: sync-loop Dir is the worktree, so "./bin/zqk" breaks.
									zqkBin, absErr := filepath.Abs(os.Args[0])
									if absErr != nil || zqkBin == "" {
										zqkBin = filepath.Join(state.proc.ProjectRoot(), "bin", "zqk")
									}
									if _, stErr := fileutil.Stat(zqkBin); stErr != nil {
										stable := filepath.Join(state.proc.ProjectRoot(), paths.ProjectDataDir, "bin", "zqk-stable")
										if _, sErr := fileutil.Stat(stable); sErr == nil {
											zqkBin = stable
										}
									}

									spawnArgs := buildOrchestrationExecutorArgs(taskID, promptMarkdown, effectiveTimeout)
									spawnCmd := execwrap.CommandContext(
										workerCtx,
										zqkBin,
										spawnArgs...,
									)
									configureOrchestrationExecutorProcess(spawnCmd)
									// Inject seat credential; do not inherit parent/human key.
									seatAccount := authcred.ResolveSeatAccount(state.proc.ProjectRoot(), personaID)
									seatKey := authcred.APIKeyForSeat(state.proc.ProjectRoot(), seatAccount)
									spawnCmd.Env = orchestrationExecutorChildEnv(os.Environ(), state.proc.ProjectRoot(), seatKey, zqkBin)
									spawnCmd.Dir = worktreePath

									// Route logs to dedicated file so we can see why it's dying
									logDir := filepath.Join(state.proc.ProjectRoot(), paths.ProjectDataDir, paths.LogsDir, "agent")
									_ = fileutil.EnsureDir(logDir)
									logFile, logErr := fileutil.OpenFile(filepath.Join(logDir, taskID+".log"), fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_APPEND, paths.FilePerm644)
									if logErr != nil {
										executorErr := errfmt.Newf("open executor log for %s", taskID).Wrap(logErr)
										_ = persistOrchestratedTaskOutcome(
											workerCtx,
											state,
											taskID,
											objects.ObjectStatusError,
											map[string]any{objects.FieldKeyType: "executor_failure", "error": executorErr.Error()},
										)
										errs[i] = executorErr
										return executorErr
									}
									spawnCmd.Stdout = logFile
									spawnCmd.Stderr = logFile

									if startErr := persistOrchestratedTaskOutcome(
										workerCtx,
										state,
										taskID,
										objects.ObjectStatusInProgress,
										nil,
									); startErr != nil {
										errs[i] = startErr
										_ = logFile.Close()
										return startErr
									}

									cli.TouchMeaningfulActivity()

									// Monitor subagent log file growth: touch meaningful activity ONLY when the child process is actively producing output
									stopMonitor := make(chan struct{})
									goroutinelabels.NewGoroutine("subagent_exec_monitor", fmt.Sprintf("output monitor for task %s", taskID)).
										StartSimple(func() {
											ticker := time.NewTicker(10 * time.Second)
											defer ticker.Stop()
											var lastSize int64 = -1
											for {
												select {
												case <-stopMonitor:
													return
												case <-workerCtx.Done():
													return
												case <-ticker.C:
													if fi, err := logFile.Stat(); err == nil {
														if sz := fi.Size(); sz > lastSize {
															lastSize = sz
															cli.TouchMeaningfulActivity()
														}
													}
												}
											}
										})
									spawnErr := spawnCmd.Run()
									close(stopMonitor)
									cli.TouchMeaningfulActivity()
									if spawnErr != nil {
										executorErr := errfmt.Newf("native executor failed for %s", taskID).Wrap(spawnErr)
										_ = persistOrchestratedTaskOutcome(
											workerCtx,
											state,
											taskID,
											objects.ObjectStatusError,
											map[string]any{objects.FieldKeyType: "executor_failure", "error": executorErr.Error()},
										)
										errs[i] = executorErr
										_ = logFile.Close()
										return executorErr
									}
									if workClass.IsDocsEval() {
										if persistErr := persistOrchestratedTaskOutcome(
											workerCtx,
											state,
											taskID,
											objects.ObjectStatusPendingVerification,
											map[string]any{
												objects.FieldKeyType: "docs_eval_outcome",
												"task_id":            taskID,
												"work_class":         string(workClass),
												"exec_root":          state.proc.ProjectRoot(),
											},
										); persistErr != nil {
											errs[i] = persistErr
											_ = logFile.Close()
											return persistErr
										}
										_ = cli.WriteOutput(state.cmd, []byte(fmt.Sprintf("✅ Native executor completed docs_eval task: %s (tier=%s)\n", taskID, modelTier)))
										_ = logFile.Close()
									} else {
										if autoCommitErr := autoCommitWorktreeChanges(workerCtx, worktreePath, taskID, itemID); autoCommitErr != nil {
											logging.FluentEvent(state.proc.Logger()).Warn("Worktree auto-commit failed").
												String("task_id", taskID).
												WithError(autoCommitErr).
												Log()
										}
										manifest, evidenceErr := collectOrchestrationCommitManifest(
											workerCtx,
											worktreePath,
											taskID,
											baseSHA,
										)
										if evidenceErr != nil {
											_ = persistOrchestratedTaskOutcome(
												workerCtx,
												state,
												taskID,
												objects.ObjectStatusError,
												map[string]any{objects.FieldKeyType: "executor_failure", "error": evidenceErr.Error()},
											)
											errs[i] = evidenceErr
											_ = logFile.Close()
											return evidenceErr
										}
										if persistErr := persistOrchestratedTaskOutcome(
											workerCtx,
											state,
											taskID,
											objects.ObjectStatusPendingVerification,
											manifest,
										); persistErr != nil {
											errs[i] = persistErr
											_ = logFile.Close()
											return persistErr
										}
										_ = cli.WriteOutput(state.cmd, []byte(fmt.Sprintf("✅ Native executor produced commit evidence for task: %s (tier=%s)\n", taskID, modelTier)))
										_ = logFile.Close()
									}
								} else {
									_ = cli.WriteOutput(state.cmd, []byte(fmt.Sprintf("🚀 Prepared native executor task: %s (skipped execution in test)\n", taskID)))
								}
							}
						}
					}
				} else {
					startEvent := map[string]any{
						objects.FieldKeyKind:          objects.KindAuditEvent,
						objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
						objects.FieldKeyEventType:     string(audit_event.EventTypePromptRunStart),
						objects.FieldKeyTitle:         fmt.Sprintf("Prompt Run Start: %s", title),
						objects.FieldKeyMetadata: map[string]any{
							"run_id": state.opts.SessionID,
							"item":   title,
						},
					}
					_ = state.sp.Create(workerCtx, state.secCtx, startEvent)

					promptMarkdown := buildOrchestrationTaskPrompt(
						workerCtx,
						state,
						subAgent,
						personaID,
						capability,
						title,
						meshSkillSection,
						upstreamDeliverables,
					)

					signer, _ := crypto.GenerateKeypair()
					tdeEnvelope := &policy.TrustDomainEnvelope{
						ID:                fmt.Sprintf("TDE-%d", time.Now().UnixNano()),
						Scope:             "agent_orchestration",
						AllowedNamespaces: []string{"*"},
						MaxRuntimeSeconds: 300,
						Payload:           map[string]any{objects.FieldKeyIntent: title},
						MCPPermissions:    []string{"agent:orchestrate"},
						Neurological: &policy.NeurologicalGovernance{
							Confidence:     0.95,
							IsStreaming:    true,
							DecisionBranch: "main_orchestration",
						},
					}
					_ = tdeEnvelope.Sign(signer)

					prompt := agentdelivery.Prompt{
						Markdown:  []byte(promptMarkdown),
						SessionID: state.opts.SessionID,
						Format:    "agent-prompt",
						DestPath:  filepath.Join(state.proc.ProjectRoot(), paths.ProjectDataDir, paths.InboxSubdir, subAgent, fmt.Sprintf("%s.md", itemID)),
						TDE:       tdeEnvelope,
					}

					deliveryCtx, cancel := context.WithTimeout(workerCtx, 30*time.Second)
					defer cancel()
					res, err := agentDeliverer.Deliver(deliveryCtx, prompt)
					errs[i] = err

					if err != nil {
						errorEvent := map[string]any{
							objects.FieldKeyKind:          objects.KindAuditEvent,
							objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
							objects.FieldKeyEventType:     string(audit_event.EventTypePromptRunError),
							objects.FieldKeyTitle:         fmt.Sprintf("Prompt Run Error: %s", title),
							objects.FieldKeyMetadata: map[string]any{
								"run_id": state.opts.SessionID,
								"item":   title,
								"error":  err.Error(),
							},
						}
						_ = state.sp.Create(workerCtx, state.secCtx, errorEvent)
					} else {
						completeEvent := map[string]any{
							objects.FieldKeyKind:          objects.KindAuditEvent,
							objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
							objects.FieldKeyEventType:     string(audit_event.EventTypePromptRunComplete),
							objects.FieldKeyTitle:         fmt.Sprintf("Prompt Run Complete: %s", title),
							objects.FieldKeyMetadata: map[string]any{
								"run_id":      state.opts.SessionID,
								"item":        title,
								"delivery_to": res.DeliveredTo,
							},
						}
						_ = state.sp.Create(workerCtx, state.secCtx, completeEvent)

						// Phase E: Emit steering event onto agent_feed for the delegated task
						_, _ = agentfeed.AppendEvent(agentfeed.AppendEventInput{
							ProjectRoot:      state.proc.ProjectRoot(),
							Message:          fmt.Sprintf("ATTN %s: task delegated via orchestrate: %s (plan %s)", subAgent, title, state.planID),
							AgentID:          "orchestration_engine",
							ToAgentID:        subAgent,
							Sender:           agentfeed.FeedSenderHumanSteer,
							EventType:        agentfeed.FeedEventTypeSteering,
							SkipEnabledCheck: true,
						})
					}
				}
				return err
			})
		}
		pool.Stop()

		for _, err := range errs {
			if err != nil {
				return nil, errfmt.Newf("failed to deliver agent prompt to one or more sub-agents").Wrap(err)
			}
		}

		state.routingPlan = map[string]any{
			objects.KindPriorityPlan:  state.planID,
			objects.FieldKeyStatus:    string("routed"),
			"items":                   state.processedItems,
			objects.FieldKeySessionID: state.opts.SessionID,
			"dispatched_items":        len(state.processedItems),
			"candidate_items":         len(state.items),
			"delivery_result":         "success",
		}
		return state, nil
	})

	b.AddStage("emit_plan", func(pctx *pipeline.Context, payload any) (any, error) {
		state := payload.(*orchestratorState)
		format := cli.GetFormat(state.cmd)
		if format != "" {
			return nil, cli.FormatOutput(state.cmd, state.routingPlan)
		}
		return nil, cli.WriteOutput(state.cmd, []byte(fmt.Sprintf(
			"\n🚀 Initiating Orchestration for Priority Plan: %s\n   ID: %s\n   Delegating tasks: %v\n   ✓ Delivery status: %s\n\n",
			state.title, state.planID, state.routingPlan["items"], state.routingPlan["delivery_result"],
		)))
	})

	pl := b.Build()
	pctx := &pipeline.Context{Ctx: ctx}
	initialState := &orchestratorState{
		cmd:     cmd,
		proc:    proc,
		secCtx:  proc.SecurityContext(),
		sp:      proc.Storage(),
		opts:    opts,
		planArg: planArg,
	}

	_, err = pl.Run(pctx, initialState)
	return err
}

func localSkillIDByTitle(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, title string) string {
	if sp == nil || strings.TrimSpace(title) == "" {
		return ""
	}
	listed, err := sp.List(ctx, secCtx, nil, storage.ListFilter{Kind: objects.KindAgentSkill})
	if err != nil || listed == nil {
		return ""
	}
	want := strings.ToLower(strings.TrimSpace(title))
	for _, obj := range listed.Objects {
		got := koi.Title(obj)
		if strings.ToLower(strings.TrimSpace(got)) != want {
			continue
		}
		return strings.TrimSpace(koi.ID(obj))
	}
	return ""
}

func verifyBLITDDReady(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, item map[string]any) bool {
	if item == nil {
		return false
	}
	critRefs := koi.GetStringSlice(item, objects.FieldKeyCriteriaRefs)
	tcRefs := koi.GetStringSlice(item, objects.FieldKeyTestCaseRefs)

	// If the item has direct test case refs, verify at least one is in a ready state
	hasDirectReadyTest := false
	var coveredCriteria []string
	for _, tcID := range tcRefs {
		if tcObj, err := sp.Read(ctx, secCtx, tcID); err == nil && tcObj != nil {
			st := koi.Status(tcObj)
			if isTestCaseReadyStatus(st) {
				hasDirectReadyTest = true
				cRefs := koi.GetStringSlice(tcObj, objects.FieldKeyCriteriaRefs)
				coveredCriteria = append(coveredCriteria, cRefs...)
			}
		}
	}

	// If no criteria refs, need at least direct ready test
	if len(critRefs) == 0 {
		return hasDirectReadyTest
	}

	// Every criterion must be covered by a ready test case
	for _, critID := range critRefs {
		hasTestForCrit := slices.Contains(coveredCriteria, critID)
		if !hasTestForCrit {
			critObj, err := sp.Read(ctx, secCtx, critID)
			if err == nil && critObj != nil {
				cTestRefs := koi.GetStringSlice(critObj, objects.FieldKeyTestCaseRefs)
				for _, tcID := range cTestRefs {
					if tcObj, tErr := sp.Read(ctx, secCtx, tcID); tErr == nil && tcObj != nil {
						st := koi.Status(tcObj)
						if isTestCaseReadyStatus(st) {
							hasTestForCrit = true
							break
						}
					}
				}
			}
		}
		if !hasTestForCrit {
			return false
		}
	}
	return true
}

func isTestCaseReadyStatus(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return s == "active" || s == "draft" || s == "metrics_captured" || s == "complete"
}

func isBlockedByUnverifiedUpstream(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, item map[string]any) bool {
	if sp == nil || item == nil {
		return false
	}
	kItem := koi.Wrap(item)

	var upstreamIDs []string
	if deps := kItem.GetStringSlice("depends_on"); len(deps) > 0 {
		upstreamIDs = append(upstreamIDs, deps...)
	}
	if upstreamTasks := kItem.GetStringSlice("upstream_task_refs"); len(upstreamTasks) > 0 {
		upstreamIDs = append(upstreamIDs, upstreamTasks...)
	}

	for _, upID := range upstreamIDs {
		upObj, err := sp.Read(ctx, secCtx, upID)
		if err != nil {
			// If upstream object is not readable or missing, withhold downstream task
			return true
		}
		kUp := koi.Wrap(upObj)
		if kUp.Status() != objects.ObjectStatusComplete && kUp.Status() != "validated" {
			return true
		}
		// If upstream item declares artifacts, verify that they are non-empty
		artifacts := kUp.GetStringSlice(objects.FieldKeyArtifacts)
		if len(artifacts) == 0 {
			return true
		}
	}

	return false
}

func resolveVerifiedUpstreamDeliverables(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, item map[string]any) string {
	if sp == nil || item == nil {
		return ""
	}
	kItem := koi.Wrap(item)

	var upstreamIDs []string
	if deps := kItem.GetStringSlice("depends_on"); len(deps) > 0 {
		upstreamIDs = append(upstreamIDs, deps...)
	}
	if upstreamTasks := kItem.GetStringSlice("upstream_task_refs"); len(upstreamTasks) > 0 {
		upstreamIDs = append(upstreamIDs, upstreamTasks...)
	}
	if len(upstreamIDs) == 0 {
		return ""
	}

	var sb strings.Builder
	var count int
	for _, upID := range upstreamIDs {
		upObj, err := sp.Read(ctx, secCtx, upID)
		if err != nil || upObj == nil {
			continue
		}
		kUp := koi.Wrap(upObj)
		artifacts := kUp.GetStringSlice(objects.FieldKeyArtifacts)
		if len(artifacts) == 0 {
			if a := kUp.GetString(objects.FieldKeyArtifacts); a != "" {
				artifacts = []string{a}
			}
		}
		summary := kUp.GetString("result_summary")
		if summary == "" {
			summary = kUp.GetString("summary")
		}

		if count == 0 {
			sb.WriteString("## Upstream Verified Deliverables\n")
			sb.WriteString("The following upstream deliverable artifacts were produced and verified by prior tasks. Use these artifact paths directly:\n")
		}
		count++
		title := kUp.Title()
		if title == "" {
			title = upID
		}
		sb.WriteString(fmt.Sprintf("- **%s** (`%s`, status: `%s`):\n", title, upID, kUp.Status()))
		if len(artifacts) > 0 {
			for _, art := range artifacts {
				sb.WriteString(fmt.Sprintf("  - Deliverable Artifact: `%s`\n", art))
			}
		}
		if summary != "" {
			sb.WriteString(fmt.Sprintf("  - Summary: %s\n", summary))
		}
	}
	if count == 0 {
		return ""
	}
	return sb.String()
}

