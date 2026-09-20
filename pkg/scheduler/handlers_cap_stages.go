package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/tpm"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

// CAP review/metrics/grooming stages extracted from handlers_cap_orchestrator.go
// (BLI-CEF-R2-ARCH-GOD-SCHEDULER).
func (h *CapOrchestratorHandler) executeReviewStage(ctx context.Context, exe string) error {
	h.logger.Info("cap_stage_review_started")

	var errs []string

	// 1. Run system check (object health + compliance)
	// We run it quietly so it doesn't pollute the logs unless there's an error
	checkCmd := execwrap.CommandContext(ctx, exe, "system", "check", "--format", "json", "--quiet")
	checkCmd = h.prepareCmd(checkCmd)
	checkOut, checkErr := checkCmd.CombinedOutput()
	systemCheckOK := checkErr == nil
	publicBlockers := 0
	if parsed, perr := parseSystemCheckPublicBlockers(checkOut); perr == nil {
		publicBlockers = parsed
		// CRIT-CAPH-008: public blockers fail review even when process exit is 0.
		if publicBlockers > 0 {
			systemCheckOK = false
			errs = append(errs, fmt.Sprintf("system check public blockers: %d", publicBlockers))
			h.logger.Error("cap_review_system_check_public_blockers", fmt.Errorf("public blockers present"),
				logging.Int("public_blockers", publicBlockers))
		}
	}
	if checkErr != nil {
		h.logger.Error("cap_review_system_check_failed", checkErr,
			logging.OutputField(truncateOutput(string(checkOut), 500)))
		errs = append(errs, fmt.Sprintf("system check: %v", checkErr))
	} else if systemCheckOK {
		h.logger.Info("cap_review_system_check_passed")
	}

	// 2. Run scheduler health check
	healthCmd := execwrap.CommandContext(ctx, exe, "scheduler", "health-check")
	healthCmd = h.prepareCmd(healthCmd)
	healthOut, healthErr := healthCmd.CombinedOutput()
	if healthErr != nil {
		h.logger.Error("cap_review_scheduler_health_failed", healthErr,
			logging.OutputField(truncateOutput(string(healthOut), 500)))
		errs = append(errs, fmt.Sprintf("scheduler health: %v", healthErr))
	} else {
		h.logger.Info("cap_review_scheduler_health_passed")
	}

	// 3. Verify latest test bundle outcomes for critical packages in the background stream (health.jsonl).
	// Coverage gaps (missing jobs / no outcome yet) stay fail-closed for readiness but must not
	// fail the CAP job every 15m (desktop notification spam). TRACK: BLI-1785905541906569000-074e24d7 / MMORCH CAP ops.
	secCtx := pkgctx.GetSecurityContext(ctx)
	testsPassed, hardTestErrs, softTestErrs := h.verifyCriticalPackagesHealth(ctx, secCtx)
	if len(hardTestErrs) > 0 {
		h.logger.Error("cap_review_tests_failed", fmt.Errorf("background test bundle failures detected"),
			logging.String("failing_notes", strings.Join(hardTestErrs, "; ")))
		errs = append(errs, hardTestErrs...)
	} else if len(softTestErrs) > 0 {
		h.logger.Warn("cap_review_tests_coverage_gap",
			logging.String("notes", strings.Join(softTestErrs, "; ")))
	} else if testsPassed {
		h.logger.Info("cap_review_tests_passed")
	}
	reviewErrs := append([]string{}, errs...)
	reviewErrs = append(reviewErrs, softTestErrs...)

	// Write review results to state file for metrics stage / gate freshness checks.
	now := time.Now().UTC()
	pending, _ := h.readPendingStage()
	if h.refreshPendingBoundCVS(ctx, &pending) {
		h.writeStateFile(capStagePendingFile, pending)
	}
	cvsID := pending.CvsID
	if cvsID == "" {
		cvsID, _ = h.resolveBoundCVS(ctx, pending.PlanID)
	}
	reviewResult := map[string]any{
		"timestamp":          now.Format(time.RFC3339),
		"system_check":       systemCheckOK,
		"scheduler_health":   healthErr == nil,
		"tests_passed":       testsPassed && len(softTestErrs) == 0 && len(hardTestErrs) == 0,
		"errors":             reviewErrs,
		"coverage_gaps":      softTestErrs,
		"public_blockers":    publicBlockers,
		"cvs_id":             cvsID,
		"focus_child_cvs_id": pending.FocusChildCvsID,
		objects.FieldKeyReadyForSessionCompletion: false,
		"ready_for_parent_completion":             false,
		objects.FieldKeySource:                    "cap_orchestrator.executeReviewStage",
	}
	if checkErr != nil {
		reviewResult["system_check_detail"] = truncateOutput(string(checkOut), 400)
	}
	if healthErr != nil {
		reviewResult["scheduler_health_detail"] = truncateOutput(string(healthOut), 400)
	}
	if len(hardTestErrs) > 0 || len(softTestErrs) > 0 {
		reviewResult["tests_detail"] = reviewErrs
	}
	h.writeStateFile(capReviewResultFile, reviewResult)

	if len(errs) > 0 {
		return errfmt.Errorf("review stage found %d issues: %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}

// executeMetricsStage collects system health metrics and writes a convergence snapshot.
func (h *CapOrchestratorHandler) executeMetricsStage(ctx context.Context, exe string) error {
	h.logger.Info("cap_stage_metrics_started")

	// 1. Get object counts
	countCmd := execwrap.CommandContext(ctx, exe, "object", "count", "--format", "json")
	countCmd = h.prepareCmd(countCmd)
	countOut, countErr := countCmd.CombinedOutput()
	if countErr != nil {
		h.logger.Error("cap_metrics_count_failed", countErr)
	}

	// 2. Get whats-next payload (which includes measure_compressed, kernel_ambience, correspondence, etc.)
	whatsNextCmd := execwrap.CommandContext(ctx, exe, "workflow", "whats-next", "--format", "json", "--agent-id", "cap-orchestrator")
	whatsNextCmd = h.prepareCmd(whatsNextCmd)
	whatsNextOut, whatsNextErr := whatsNextCmd.CombinedOutput()
	if whatsNextErr != nil {
		h.logger.Error("cap_metrics_whatsnext_failed", whatsNextErr)
	}

	// 3. Get system metrics summary
	metricsSumCmd := execwrap.CommandContext(ctx, exe, "system", "metrics", "--summary", "--format", "json")
	metricsSumCmd = h.prepareCmd(metricsSumCmd)
	metricsSumOut, metricsSumErr := metricsSumCmd.CombinedOutput()
	if metricsSumErr != nil {
		h.logger.Error("cap_metrics_system_metrics_summary_failed", metricsSumErr)
	}

	// 4. Get metric outliers
	failuresCmd := execwrap.CommandContext(ctx, exe, "system", "metrics", "--filter", "failures", "--format", "json")
	failuresCmd = h.prepareCmd(failuresCmd)
	failuresOut, _ := failuresCmd.CombinedOutput()

	slowCmd := execwrap.CommandContext(ctx, exe, "system", "metrics", "--filter", "slow", "--format", "json")
	slowCmd = h.prepareCmd(slowCmd)
	slowOut, _ := slowCmd.CombinedOutput()

	timeoutsCmd := execwrap.CommandContext(ctx, exe, "system", "metrics", "--filter", "timeouts", "--format", "json")
	timeoutsCmd = h.prepareCmd(timeoutsCmd)
	timeoutsOut, _ := timeoutsCmd.CombinedOutput()

	// 5. Get scheduler health and base metrics
	baseMetricsCmd := execwrap.CommandContext(ctx, exe, "object", "list", "base_metric", "--format", "json")
	baseMetricsCmd = h.prepareCmd(baseMetricsCmd)
	baseMetricsOut, baseMetricsErr := baseMetricsCmd.CombinedOutput()
	if baseMetricsErr != nil {
		h.logger.Error("cap_metrics_base_metric_failed", baseMetricsErr)
	}

	commandMetricsCmd := execwrap.CommandContext(ctx, exe, "object", "list", "command_metric", "--format", "json")
	commandMetricsCmd = h.prepareCmd(commandMetricsCmd)
	commandMetricsOut, commandMetricsErr := commandMetricsCmd.CombinedOutput()
	if commandMetricsErr != nil {
		h.logger.Error("cap_metrics_command_metric_failed", commandMetricsErr)
	}

	schedHealthCmd := execwrap.CommandContext(ctx, exe, "object", "list", "scheduler_health_metric", "--format", "json")
	schedHealthCmd = h.prepareCmd(schedHealthCmd)
	schedHealthOut, _ := schedHealthCmd.CombinedOutput()

	// 4b. Get agent_idle_digest
	idleCmd := execwrap.CommandContext(ctx, exe, "agent", "scoreboard")
	idleCmd = h.prepareCmd(idleCmd)
	idleOut, idleErr := idleCmd.CombinedOutput()
	if idleErr != nil {
		h.logger.Error("cap_metrics_idle_scoreboard_failed", idleErr)
	}

	// 5. Read last review result
	reviewResult, _ := h.readStateFile(capReviewResultFile)

	// 6. Write metrics snapshot
	pending, _ := h.readPendingStage()
	if h.refreshPendingBoundCVS(ctx, &pending) {
		h.writeStateFile(capStagePendingFile, pending)
	}
	cvsID := pending.CvsID
	if cvsID == "" {
		cvsID, _ = h.resolveBoundCVS(ctx, pending.PlanID)
	}
	var alignPointer any
	var wnObj map[string]any
	if err := json.Unmarshal(whatsNextOut, &wnObj); err == nil {
		if pp, ok := wnObj["priority_plan"]; ok {
			alignPointer = pp
		}
	}

	var nextAdminAction any
	var sumObj map[string]any
	if err := json.Unmarshal(metricsSumOut, &sumObj); err == nil {
		if suggestions, ok := sumObj["improvement_suggestions"].([]any); ok && len(suggestions) > 0 {
			nextAdminAction = suggestions[0]
		}
	}

	metrics := map[string]any{
		"timestamp":                 time.Now().UTC().Format(time.RFC3339),
		"review":                    reviewResult,
		"cvs_id":                    cvsID,
		"focus_child_cvs_id":        pending.FocusChildCvsID,
		objects.FieldKeyObjectCount: jsonOrRaw(countOut),
		"whats_next":                jsonOrRaw(whatsNextOut),
		"system_metrics_summary":    jsonOrRaw(metricsSumOut),
		"outliers_failures":         jsonOrRaw(failuresOut),
		"outliers_slow":             jsonOrRaw(slowOut),
		"outliers_timeouts":         jsonOrRaw(timeoutsOut),
		"scheduler_health":          jsonOrRaw(schedHealthOut),
		"base_metrics":              jsonOrRaw(baseMetricsOut),
		"command_metrics":           jsonOrRaw(commandMetricsOut),
		"agent_idle_digest":         string(idleOut),
		"next_admin_action":         nextAdminAction,
		"align_pointer":             alignPointer,
		objects.FieldKeySource:      "cap_orchestrator.executeMetricsStage",
	}

	// Write to both state (for self-improvement) and reports (for history)
	h.writeStateFile("cap_metrics_latest.json", metrics)

	reportID := fmt.Sprintf("cap-metrics-%s", time.Now().UTC().Format("20060102-150405"))
	reportObj := map[string]any{
		objects.FieldKeyID:      reportID,
		objects.FieldKeyKind:    "metrics_report",
		objects.FieldKeyMetrics: metrics,
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	if err := h.storage.Create(ctx, secCtx, reportObj); err != nil {
		h.logger.Error("cap_metrics_report_write_failed", err)
	}

	// 5. Stamp agent_feed digest to TPM seat
	if metricsBytes, err := json.Marshal(metrics); err == nil {
		tpmSeat := agentfeed.CoordinatorSeatID(h.projectRoot)
		if tpmSeat == "" && len(agentfeed.DefaultPeerSeatIDs) > 0 {
			tpmSeat = agentfeed.DefaultPeerSeatIDs[0]
		}
		steerCmd := execwrap.CommandContext(ctx, exe, "feed", "steer",
			"--agent-id", "cap-orchestrator",
			"--to-agent-id", tpmSeat,
			"--await-peer-ack",
			"--message", string(metricsBytes),
		)
		steerCmd = h.prepareCmd(steerCmd)
		if steerOut, steerErr := steerCmd.CombinedOutput(); steerErr != nil {
			h.logger.Error("cap_metrics_feed_steer_failed", steerErr, logging.String("output", string(steerOut)))
		} else {
			h.logger.Info("cap_metrics_feed_steer_success", logging.String("receipt", strings.TrimSpace(string(steerOut))))
		}
	} else {
		h.logger.Error("cap_metrics_json_marshal_failed", err)
	}

	h.logger.Info("cap_stage_metrics_completed",
		logging.String("report_id", reportID))
	return nil
}

// executeSelfImprovementStage executes the improvement-report command to analyze system
// health holistically (across nodes) and generate prioritized work items.
func (h *CapOrchestratorHandler) executeSelfImprovementStage(ctx context.Context, exe string) error {
	h.logger.Info("cap_stage_self_improvement_started")

	var issues []string

	// 1. Execute system improvement-report
	reportCmd := execwrap.CommandContext(ctx, exe, "system", "improvement-report", "--format", "json")
	reportCmd = h.prepareCmd(reportCmd)
	reportOut, reportErr := reportCmd.CombinedOutput()
	if reportErr != nil {
		h.logger.Error("cap_self_improvement_report_failed", reportErr, logging.OutputField(string(reportOut)))
		issues = append(issues, fmt.Sprintf("Failed to generate improvement report: %v", reportErr))
	} else {
		// 2. Parse the report output
		var result struct {
			WorkItems []struct {
				Priority string `json:"priority"`
				Title    string `json:"title"`
				Category string `json:"category"`
			} `json:"work_items"`
		}
		if err := json.Unmarshal(reportOut, &result); err != nil {
			h.logger.Error("cap_self_improvement_report_parse_failed", err)
			issues = append(issues, fmt.Sprintf("Failed to parse improvement report JSON: %v", err))
		} else {
			for _, w := range result.WorkItems {
				issues = append(issues, fmt.Sprintf("[%s] %s (%s)", w.Priority, w.Title, w.Category))
			}
		}
	}

	// 3. Check for stuck state — are these the same issues repeating?
	tracker := h.readFailureTracker()
	stuckIssues := tracker.ConsecutiveFailures >= capAgentEscalationThreshold

	// 4. Log findings
	if len(issues) == 0 {
		h.logger.Info("cap_self_improvement_clean", logging.StatusField("no issues found"))
		return nil
	}

	h.logger.Info("cap_self_improvement_issues_found",
		logging.CountField(len(issues)),
		logging.Bool("stuck", stuckIssues))

	// 5. Create a self-improvement report state update
	report := map[string]any{
		"timestamp":            time.Now().UTC().Format(time.RFC3339),
		objects.FieldKeyStatus: objects.ObjectStatusIssuesFound,
		"issues":               issues,
		"consecutive_failures": tracker.ConsecutiveFailures,
		"stuck":                stuckIssues,
	}
	h.writeStateFile("cap_self_improvement_result.json", report)

	// 6. If stuck, escalate via the provider chain
	if stuckIssues {
		notice := EscalationNotice{
			Title:    "CAP Loop Stuck — Needs Intervention",
			Severity: EscalationSeverityCritical,
			Issues:   issues,
			Context: map[string]any{
				"consecutive_failures": tracker.ConsecutiveFailures,
				"last_stage":           tracker.LastStage,
			},
			SuggestedActions: []string{
				"zqk scheduler status",
				"cat .zqk/state/cap_review_result.json",
				"go test ./pkg/scheduler/...",
			},
		}
		if escalateErr := h.escalation.Evaluate(context.Background(), tracker.ConsecutiveFailures, notice); escalateErr != nil { // Background: request-or-shutdown derived
			h.logger.Error("cap_self_improvement_escalation_failed", escalateErr)
		}
		return errfmt.Errorf("CAP loop stuck after %d consecutive failures, escalated via provider chain",
			tracker.ConsecutiveFailures)
	}

	// 7. Not stuck yet — try to self-heal by dispatching improvement agent
	issuesSummary := strings.Join(issues, "\n- ")
	ambientCtx := fmt.Sprintf("SELF-IMPROVEMENT: The following issues were detected by the CAP review/metrics stages and need resolution:\n- %s", issuesSummary)

	agentLoop := pipeline.NewBuilder("agent_loop", h.logger).
		AddStage("self_heal", func(pCtx *pipeline.Context, input any) (any, error) {
			secCtx := pkgctx.NewSystemSecurityContext()
			inboxItem := map[string]any{
				objects.FieldKeyKind:        "agent_instruction",
				objects.FieldKeyInstruction: input.(string),
				objects.FieldKeyStatus:      objects.ObjectStatusProposed,
			}
			return nil, h.storage.Create(pCtx.Ctx, secCtx, inboxItem)
		}).Build()

	_, dispatchErr := agentLoop.Run(&pipeline.Context{Ctx: ctx}, ambientCtx)
	if dispatchErr != nil {
		h.logger.Error("cap_self_improvement_dispatch_failed", dispatchErr)
	}

	h.logger.Info("cap_stage_self_improvement_completed",
		logging.Int("issues_count", len(issues)))
	return nil
}

// executeGroomingStage provisions a Strategic Pod to convert system findings
// (like Improvement Reports) and backlog items into actionable Priority Plans.
// planID should be the live whats-next priority plan (not a hardcoded legacy PRI).
// Dispatching an AGI is not stage completion — AdvanceCAPStage waits for graph
// artifacts (priority_plan / backlog_item) or an explicit cap_stage_receipt.
func (h *CapOrchestratorHandler) executeGroomingStage(ctx context.Context, exe string, planID string, planned int, planTitle, planStatus string, counts map[string]int) error {
	h.logger.Info("cap_stage_grooming_started", logging.PlanIDField(planID))

	if pending, exhausted := h.groomingPlannedZeroExhausted(planned); exhausted {
		cue := whatsnext.PriorityPlanPackagingCue(planID, planTitle, planStatus, counts)
		if cue == "" {
			cue = fmt.Sprintf("planned=0 after %d cap_stage_grooming ticks on %s (status %s) — stop grooming no-ops; wrap this column or shape the next unlocked plan (POL-AGENT-TPM-GROOM-AHEAD-001 / POL-WORKFLOW-002)", pending.Attempts, planID, planStatus)
		}
		h.logger.Warn("cap_stage_grooming_planned_zero_exhausted",
			logging.PlanIDField(planID),
			logging.Int("attempts", pending.Attempts),
			logging.Int("max_ticks", capGroomingPlannedZeroMaxTicks),
			logging.String("packaging_cue", cue))
		h.writeStateFile(capGroomingPlannedZeroFile, map[string]any{
			capFieldPlanID:            planID,
			"attempts":                pending.Attempts,
			"state":                   "exhausted",
			"packaging_cue":           cue,
			objects.FieldKeyCreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
		return errfmt.Errorf("cap_stage_grooming planned=0 exhausted: %s", cue)
	}

	// Stay ahead of the swarm: still wake TPM even when whats-next has no plan
	// (starvation case). Stage advance remains gated on delivery evidence.
	// TRACK: BLI-1786390039711686000-e718d458 / BLI-ATK-MERGE-UP-HYGIENE-001 — kernel stage prompts.
	ambientCtx := h.capStageAGIInstruction(ctx, "cap_stage_grooming", planID)

	// Anticipatory Runway Replenishment: evaluate delta_runway <= 1 watermark and feed shovel-ready bundles to TPM
	if snap, candidates, err := tpm.EvaluateAndReplenishRunway(ctx, h.storage, h.projectRoot); err == nil && snap != nil {
		if snap.IsRunwayDepleted && len(candidates) > 0 {
			topCandidate := candidates[0]
			h.logger.Info("cap_stage_grooming_anticipatory_synthesis_triggered",
				logging.Int("runway_depth", snap.RunwayDepth),
				logging.String("domain", topCandidate.Domain),
				logging.Int("unmapped_requirements", len(topCandidate.RequirementIDs)),
			)
			ambientCtx += fmt.Sprintf("\n\n[Anticipatory Runway Replenishment]: Execution runway depth is %d (<= 1). Domain cluster '%s' has %d unmapped active requirements ready for shovel-ready staging (%s). Synthesize and stage the next priority_plan before the lead plan completes (POL-AGENT-TPM-GROOM-AHEAD-001).", snap.RunwayDepth, topCandidate.Domain, len(topCandidate.RequirementIDs), strings.Join(topCandidate.RequirementIDs, ", "))
		}
	}

	if h.hasOpenTPMGroomingInstruction(ctx, planID) {
		h.logger.Info("cap_stage_grooming_reusing_open_instruction", logging.PlanIDField(planID))
		h.wakeAgentAndScheduleHourglass(planID, "tpm")
		return nil
	}

	agentLoop := pipeline.NewBuilder("agent_loop", h.logger).
		AddStage("grooming", func(pCtx *pipeline.Context, input any) (any, error) {
			secCtx := pkgctx.NewSystemSecurityContext()
			inboxItem := map[string]any{
				objects.FieldKeyKind:        objects.KindAgentInstruction,
				"plan_id":                   planID,
				"persona_id":                "tpm",
				objects.FieldKeyInstruction: input.(string),
				objects.FieldKeyStatus:      objects.ObjectStatusProposed,
			}
			if createErr := h.storage.Create(pCtx.Ctx, secCtx, inboxItem); createErr != nil {
				return nil, createErr
			}
			// Wake a worker for the grooming instruction (AGI alone does not spawn swarm).
			h.wakeAgentAndScheduleHourglass(planID, "tpm")
			return nil, nil
		}).Build()

	_, dispatchErr := agentLoop.Run(&pipeline.Context{Ctx: ctx}, ambientCtx)
	if dispatchErr != nil {
		h.logger.Error("cap_grooming_dispatch_failed", dispatchErr)
		return fmt.Errorf("failed to dispatch strategy pod: %w", dispatchErr)
	}

	h.logger.Info("cap_stage_grooming_dispatched", logging.PlanIDField(planID))
	return nil
}

func (h *CapOrchestratorHandler) hasOpenTPMGroomingInstruction(ctx context.Context, planID string) bool {
	return h.hasOpenAgentInstruction(ctx, planID, "tpm", "cap_stage_grooming")
}

// capStageAGIInstruction expands a CapStages label into preamble+stage prompt_template body.
func (h *CapOrchestratorHandler) capStageAGIInstruction(ctx context.Context, stage, planID string) string {
	if stage == "" {
		return stage
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	body, err := agentprompt.BuildCapStageWakePrompt(ctx, h.storage, secCtx, stage, agentprompt.CapStageWakeOptions{
		PlanID: planID,
	})
	if err != nil || strings.TrimSpace(body) == "" {
		h.logger.Warn("cap_stage_prompt_fallback",
			logging.StageField(stage),
			logging.ErrorTextField(fmt.Sprintf("%v", err)))
		return stage
	}
	return body
}
