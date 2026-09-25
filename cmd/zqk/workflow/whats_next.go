package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/agentprompt"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/interactionpolicy"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	observerpkg "github.com/zqk-os/zqk/pkg/observer"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/tpm"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

const whatsNextSchema = "zqk_whats_next_v1"

const measureSubprocessTimeout = 120 * time.Second

// whatsNextOut is the zqk_whats_next_v1 composite document.
type whatsNextOut struct {
	Schema                         string                    `json:"schema"`
	PriorityPlan                   *whatsNextPriorityPlan    `json:"priority_plan"`
	ActivePlans                    []whatsNextPriorityPlan   `json:"active_plans,omitempty"`
	BacklogCountsByStatus          map[string]int            `json:"backlog_counts_by_status"`
	ConvergenceSessionsActive      []whatsNextCVSRow         `json:"convergence_sessions_active"`
	MeasureSessionID               *string                   `json:"measure_session_id"`
	MeasureCompressed              map[string]any            `json:"measure_compressed,omitempty"`
	MeasureSkipReason              string                    `json:"measure_skip_reason,omitempty"`
	MeasureError                   string                    `json:"measure_error,omitempty"`
	AgentInstruction               string                    `json:"agent_instruction,omitempty"`
	PackagingCue                   string                    `json:"packaging_cue,omitempty"`
	ObserverTips                   []string                  `json:"observer_tips,omitempty"`
	Correspondence                 *whatsNextCorrespondence  `json:"correspondence,omitempty"`
	KernelAmbience                 *whatsnext.KernelAmbience `json:"kernel_ambience,omitempty"`
	FillItem                       *whatsnext.FillItem       `json:"fill_item,omitempty"`
	GuidingStep                    *whatsNextGuidingStep     `json:"guiding_step,omitempty"`
	StrategicReplenishmentCue      string                    `json:"strategic_replenishment_cue,omitempty"`
	RunwayDepth                    int                       `json:"runway_depth,omitempty"`
	MaterializedViewStale          bool                      `json:"materialized_view_stale,omitempty"`
	MaterializedViewRecovering     bool                      `json:"materialized_view_recovering,omitempty"`
	MaterializedViewDegradedReason string                    `json:"materialized_view_degraded_reason,omitempty"`
}

// whatsNextGuidingStep is compiled hunger from the interaction policy.
type whatsNextGuidingStep struct {
	Event       string `json:"event"`
	PolicyID    string `json:"policy_id"`
	GuidingStep string `json:"guiding_step"`
	CommandHint string `json:"command_hint,omitempty"`
	Hydrated    bool   `json:"hydrated"`
}

// whatsNextCorrespondence is persona/agent-scoped mesh receipt context.
type whatsNextCorrespondence struct {
	PersonaID             string                         `json:"persona_id,omitempty"`
	AgentID               string                         `json:"agent_id,omitempty"`
	InboxUnacked          []agentfeed.CorrespondenceItem `json:"inbox_unacked"`
	OutboxAwaitingPeerAck []agentfeed.CorrespondenceItem `json:"outbox_awaiting_peer_ack"`
	RegisteredCallbacks   []agentfeed.PeerAckAwait       `json:"registered_callbacks,omitempty"`
	ActiveAgentTask       *whatsNextActiveTask           `json:"active_agent_task,omitempty"`
	NextActionHint        string                         `json:"next_action_hint,omitempty"`
	SkipReason            string                         `json:"skip_reason,omitempty"`
}

type whatsNextActiveTask struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
}

type whatsNextPriorityPlan struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Shaped bool   `json:"shaped,omitempty"`
}

type whatsNextCVSRow struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	CurrentPhase string `json:"current_phase"`
	Status       string `json:"status"`
}

// NewWhatsNextCmd returns workflow whats-next command.
func NewWhatsNextCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewWorkflowWhatsNextCommandBuilder()
	cli.RequireSession(cmd, false)
	cmd.Flags().String("persona-id", "", "Optional persona ID to evaluate what's next for a specific persona")
	cmd.Flags().String("agent-id", "", "Swarm seat id: correspondence plus seated persona_ref for plan selection (peer_seats). Operator/TPM seats still compile the Gantt lead.")
	cmd.Flags().Bool("sync-sweep", false, "Emergency diagnostics only: perform synchronous full storage sweep instead of reading materialized lite view")
	cli.BindAsyncProgress(cmd, runWhatsNext)
	cli.RequireStorage(cmd, true)
	return cmd
}

// Cap stage selection is owned by pkg/workflow/whatsnext (peek + starve→grooming).

func runWhatsNext(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var flags clipkg.FlagBag
		priFlag := flags.String(cmd, "priority-plan")
		cvsFlag := flags.String(cmd, "session-id")
		skipMeasure := flags.Bool(cmd, "skip-measure")
		personaFlag := flags.String(cmd, "persona-id")
		agentIDFlag := flags.String(cmd, "agent-id")
		syncSweep := flags.Bool(cmd, "sync-sweep")
		if err := flags.Err(); err != nil {
			return err
		}

		projectRoot := proc.ProjectRoot()
		if projectRoot == emptyValue {
			projectRoot = cli.ResolveProjectRoot(".")
		}

		if syncSweep {
			return runWhatsNextSyncSweep(cmd, args, proc, projectRoot, priFlag, cvsFlag, personaFlag, agentIDFlag, skipMeasure)
		}

		return runWhatsNextLite(cmd, args, proc, projectRoot, priFlag, cvsFlag, personaFlag, agentIDFlag, skipMeasure)
	})(cmd, args)
}

func runWhatsNextLite(cmd *cobra.Command, args []string, proc *cli.Processor, projectRoot, priFlag, cvsFlag, personaFlag, agentIDFlag string, skipMeasure bool) error {
	ctx := proc.OperationContext()
	sp := proc.Storage()

	out := whatsNextOut{Schema: whatsNextSchema, BacklogCountsByStatus: map[string]int{}}

	// Strict default hot path: read pre-computed zero-cost projection (<5ms SLA)
	lite, _ := whatsnext.GetOrRecoverPayload(ctx, sp, projectRoot, whatsnext.DefaultStalenessTolerance)
	if lite != nil {
		out.MaterializedViewStale = lite.Stale
		out.MaterializedViewRecovering = lite.Recovering
		out.MaterializedViewDegradedReason = lite.DegradedReason
		out.RunwayDepth = lite.RunwayDepth

		for _, p := range lite.ActivePlans {
			if strings.TrimSpace(p.Title) == "" {
				continue
			}
			out.ActivePlans = append(out.ActivePlans, whatsNextPriorityPlan{
				ID:     p.ID,
				Title:  p.Title,
				Status: p.Status,
			})
		}

		explicit := strings.TrimSpace(priFlag)
		if explicit != emptyValue {
			var matched *whatsNextPriorityPlan
			if lite.LeadPlan != nil && lite.LeadPlan.ID == explicit && strings.TrimSpace(lite.LeadPlan.Title) != "" {
				matched = &whatsNextPriorityPlan{
					ID:     lite.LeadPlan.ID,
					Title:  lite.LeadPlan.Title,
					Status: lite.LeadPlan.Status,
				}
			} else {
				for _, p := range lite.ActivePlans {
					if p.ID == explicit && strings.TrimSpace(p.Title) != "" {
						matched = &whatsNextPriorityPlan{
							ID:     p.ID,
							Title:  p.Title,
							Status: p.Status,
						}
						break
					}
				}
			}
			if matched != nil {
				out.PriorityPlan = matched
			} else {
				out.PriorityPlan = &whatsNextPriorityPlan{ID: explicit}
			}
		} else if lite.LeadPlan != nil && strings.TrimSpace(lite.LeadPlan.Title) != "" {
			out.PriorityPlan = &whatsNextPriorityPlan{
				ID:     lite.LeadPlan.ID,
				Title:  lite.LeadPlan.Title,
				Status: lite.LeadPlan.Status,
			}
		}

		if out.PriorityPlan != nil {
			if counts, ok := lite.BacklogCountsByPlan[out.PriorityPlan.ID]; ok {
				out.BacklogCountsByStatus = counts
			}
		}
		if len(out.BacklogCountsByStatus) == 0 && len(lite.TotalBacklogCounts) > 0 && out.PriorityPlan == nil {
			out.BacklogCountsByStatus = lite.TotalBacklogCounts
		}
		if out.BacklogCountsByStatus == nil {
			out.BacklogCountsByStatus = map[string]int{}
		}

		if out.PriorityPlan != nil {
			if cue, ok := lite.PackagingCuesByPlan[out.PriorityPlan.ID]; ok && cue != "" {
				out.PackagingCue = cue
			} else {
				out.PackagingCue = whatsnext.PriorityPlanPackagingCue(out.PriorityPlan.ID, out.PriorityPlan.Title, out.PriorityPlan.Status, out.BacklogCountsByStatus)
			}
			if inst, ok := lite.AgentInstructionsByPlan[out.PriorityPlan.ID]; ok {
				out.AgentInstruction = inst
			}
		}
		if out.AgentInstruction == "" {
			totalWork := out.BacklogCountsByStatus["in_progress"] + out.BacklogCountsByStatus["verifying"] + out.BacklogCountsByStatus["planned"] + out.BacklogCountsByStatus["blocked"] + out.BacklogCountsByStatus["validated"] + out.BacklogCountsByStatus["exploring"]
			if totalWork == 0 && out.BacklogCountsByStatus["completed"] > 0 {
				out.AgentInstruction = "shutdown"
			} else if out.BacklogCountsByStatus["in_progress"] > 0 {
				out.AgentInstruction = "continue"
			} else if out.BacklogCountsByStatus["verifying"] > 0 {
				out.AgentInstruction = "execute_tests"
			} else if totalWork == 0 {
				out.AgentInstruction = "shutdown"
			} else {
				out.AgentInstruction = "continue"
			}
		}

		for _, row := range lite.ConvergenceSessions {
			out.ConvergenceSessionsActive = append(out.ConvergenceSessionsActive, whatsNextCVSRow{
				ID:           row.ID,
				Title:        row.Title,
				CurrentPhase: row.CurrentPhase,
				Status:       row.Status,
			})
		}

		if lite.IsRunwayDepleted {
			out.StrategicReplenishmentCue = fmt.Sprintf("Watermark delta_runway <= 1 (depth=%d).", lite.RunwayDepth)
		}
	}

	out.ObserverTips = getObserverTips()

	secCtx := pkgctx.GetSecurityContext(ctx)
	personaIDs := resolveWhatsNextPersonaIDs(ctx, nil, projectRoot, personaFlag, agentIDFlag)

	var activeTask map[string]any
	if lite != nil && len(lite.ActiveTasksByAssignee) > 0 {
		var matchedNode *whatsnext.ActiveTaskNode
		if secCtx != nil && secCtx.AccountID != "" {
			matchedNode = lite.ActiveTasksByAssignee[secCtx.AccountID]
		}
		if matchedNode == nil {
			for _, pid := range personaIDs {
				if node, ok := lite.ActiveTasksByAssignee[pid]; ok {
					matchedNode = node
					break
				}
			}
		}
		if matchedNode != nil {
			activeTask = map[string]any{
				objects.FieldKeyID:     matchedNode.ID,
				objects.FieldKeyStatus: matchedNode.Status,
				objects.FieldKeyTitle:  matchedNode.Title,
			}
		}
	}

	out.Correspondence = buildWhatsNextCorrespondence(projectRoot, strings.TrimSpace(agentIDFlag), strings.TrimSpace(personaFlag), personaIDs, activeTask)

	amb := whatsnext.LoadKernelAmbience(projectRoot)
	hint := ""
	if out.Correspondence != nil {
		hint = out.Correspondence.NextActionHint
	}
	whatsnext.ApplySeatOperatingModeIn(&amb, projectRoot, strings.TrimSpace(agentIDFlag), hint)
	out.KernelAmbience = &amb
	out.AgentInstruction = whatsnext.PrependStewardProjectionForSeat(out.AgentInstruction, amb.StewardFocus, amb.SeatMode)

	compileWhatsNextDrive(&out, ctx, nil, strings.TrimSpace(personaFlag), personaIDs)
	out.AgentInstruction = whatsnext.ApplyFillToInstruction(out.AgentInstruction, out.FillItem)

	cvsID := strings.TrimSpace(cvsFlag)
	if skipMeasure {
		out.MeasureCompressed = nil
		out.MeasureSkipReason = "--skip-measure"
	} else if cvsID != emptyValue {
		out.MeasureSessionID = &cvsID
		raw, err := runSchedulerConvergenceMeasureJSON(projectRoot, cvsID)
		if err != nil {
			out.MeasureCompressed = nil
			out.MeasureError = err.Error()
		} else {
			out.MeasureCompressed = compressWhatsNextMeasure(raw)
		}
	} else {
		out.MeasureCompressed = nil
		out.MeasureSkipReason = "zero-cost hot path (use --session-id or --sync-sweep to measure)"
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL:
		return cli.FormatOutput(cmd, out)
	case cli.FormatYAML:
		return cli.FormatOutput(cmd, out)
	default:
		return writeWhatsNextTable(cmd, out)
	}
}

func runWhatsNextSyncSweep(cmd *cobra.Command, args []string, proc *cli.Processor, projectRoot, priFlag, cvsFlag, personaFlag, agentIDFlag string, skipMeasure bool) error {
	ctx := proc.OperationContext()
	sp := proc.Storage()

	out := whatsNextOut{Schema: whatsNextSchema, BacklogCountsByStatus: map[string]int{}}

	personaIDs := resolveWhatsNextPersonaIDs(ctx, sp, projectRoot, personaFlag, agentIDFlag)
	columnIDs := planPersonaFilter(ctx, sp, projectRoot, agentIDFlag, personaIDs)

	planID, planSumm, activePlans := resolvePriorityPlanForWhatsNext(ctx, sp, strings.TrimSpace(priFlag), columnIDs)
	out.PriorityPlan = planSumm
	out.ActivePlans = activePlans

	// Matrix-Gated Autonomy: Removed arbitrary processVerifyingItems loop.
	// Lifecycle transitions must be governed natively by a verification_matrix.

	if planID != emptyValue {
		out.BacklogCountsByStatus = countBacklogByStatus(ctx, sp, planID, columnIDs)

		totalWork := out.BacklogCountsByStatus["in_progress"] + out.BacklogCountsByStatus["verifying"] + out.BacklogCountsByStatus["planned"] + out.BacklogCountsByStatus["blocked"] + out.BacklogCountsByStatus["validated"] + out.BacklogCountsByStatus["exploring"]
		if totalWork == 0 && out.BacklogCountsByStatus["completed"] > 0 {
			out.AgentInstruction = "shutdown"
		} else if out.BacklogCountsByStatus["in_progress"] > 0 {
			out.AgentInstruction = "continue"
		} else if out.BacklogCountsByStatus["verifying"] > 0 {
			out.AgentInstruction = "execute_tests"
		} else if totalWork == 0 {
			out.AgentInstruction = "shutdown"
		}
		if planSumm != nil {
			out.PackagingCue = whatsnext.PriorityPlanPackagingCue(planSumm.ID, planSumm.Title, planSumm.Status, out.BacklogCountsByStatus)
		}
	}

	secCtx := pkgctx.GetSecurityContext(ctx)
	rows := listActiveOrPausedConvergenceSessions(ctx, sp)
	out.ConvergenceSessionsActive = rows

	var taskPrompt string
	var activeTask map[string]any
	if secCtx != nil && (len(personaIDs) > 0 || secCtx.AccountID != "") {
		listRes, err := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), storage.DefaultQueryFactory.
			Builder(objects.KindAgentTask).
			StatusIn(objects.ObjectStatusInProgress, objects.ObjectStatusProposed).
			Build())
		if err == nil {
			var bestProposed map[string]any
			for _, obj := range listRes.Objects {
				status, _ := obj[objects.FieldKeyStatus].(string)
				if status != objects.ObjectStatusInProgress && status != objects.ObjectStatusProposed {
					continue
				}
				assignee, _ := obj[objects.FieldKeyAssigneePersonaRef].(string)
				matches := false
				if assignee == secCtx.AccountID && secCtx.AccountID != "" {
					matches = true
				} else {
					for _, pid := range personaIDs {
						if assignee == pid {
							matches = true
							break
						}
					}
				}
				if !matches {
					continue
				}
				// Prefer in_progress over proposed so READY claims cannot ignore open work.
				if status == objects.ObjectStatusInProgress {
					activeTask = obj
					break
				}
				if bestProposed == nil {
					bestProposed = obj
				}
			}
			if activeTask == nil {
				activeTask = bestProposed
			}
		}
	}

	if activeTask != nil {
		planRef, _ := activeTask[objects.FieldKeyPipelineRef].(string)
		var planTitle string
		if planRef != "" {
			if planObj, err := sp.Read(ctx, secCtx, planRef); err == nil && planObj != nil {
				planTitle, _ = planObj[objects.FieldKeyTitle].(string)
			}
		}
		var stepsStr strings.Builder
		if steps, ok := activeTask[objects.FieldKeyTaskSteps].([]any); ok { // "task_steps" has no predefined constant
			for _, stepAny := range steps {
				if step, ok := stepAny.(map[string]any); ok {
					title, _ := step[objects.FieldKeyTitle].(string)
					desc, _ := step[objects.FieldKeyDescription].(string)
					status, _ := step[objects.FieldKeyStatus].(string)
					cmd, _ := step[objects.FieldKeyCommand].(string)
					stepsStr.WriteString(fmt.Sprintf("- **%s** [%s]: %s\n", title, status, desc))
					if cmd != "" {
						stepsStr.WriteString(fmt.Sprintf("  Command: `%s`\n", cmd))
					}
				}
			}
		}

		taskTitle, _ := activeTask[objects.FieldKeyTitle].(string)
		taskDesc, _ := activeTask[objects.FieldKeyDescription].(string)
		targetAgent, _ := activeTask[objects.FieldKeyAssigneePersonaRef].(string)

		var sessionIDStr string
		if len(rows) > 0 {
			sessionIDStr = rows[0].ID
		}

		if agentprompt.IsTaskEnvelope(taskDesc) {
			taskPrompt = taskDesc
		} else {
			taskOpts := agentprompt.TaskPromptOptions{
				PlanTitle:     planTitle,
				PlanID:        planRef,
				SessionID:     sessionIDStr,
				TargetAgent:   targetAgent,
				PersonaID:     targetAgent,
				Capability:    "native_execution",
				TaskTitle:     taskTitle,
				TaskContext:   taskDesc,
				ValidationDSL: stepsStr.String(),
				IncludeTDD:    true,
			}
			if promptData, err := agentprompt.BuildTaskPrompt(ctx, sp, secCtx, projectRoot, taskOpts); err == nil {
				taskPrompt = promptData
			}
		}
	}

	if taskPrompt != "" {
		out.AgentInstruction = taskPrompt
	} else if secCtx != nil && secCtx.AccountID == objects.DefaultSystemAccountID {
		out.AgentInstruction = whatsnext.SelectCAPInstruction(projectRoot, out.BacklogCountsByStatus)
	} else if out.AgentInstruction == "continue" || out.AgentInstruction == "" {
		if promptData, err := agentprompt.BuildOnboardingPrompt(ctx, sp, 3000); err == nil {
			out.AgentInstruction = promptData
		} else {
			out.AgentInstruction = "continue" // fallback
		}
	}

	if out.AgentInstruction == "cap_stage_metrics" {
		ev := logging.FluentEvent(proc.Logger()).Info("Executing CAP stage: metrics").
			String("priority_plan", planID).
			Int("active_sessions", len(rows))
		for status, count := range out.BacklogCountsByStatus {
			ev = ev.Int("backlog_"+status, count)
		}
		ev.Log()
	}

	cvsID := strings.TrimSpace(cvsFlag)
	if cvsID == emptyValue {
		cvsID = pickDefaultMeasureSessionID(rows)
	}
	if cvsID != emptyValue {
		out.MeasureSessionID = &cvsID
	} else {
		out.MeasureSessionID = nil
	}

	if skipMeasure {
		out.MeasureCompressed = nil
		out.MeasureSkipReason = "--skip-measure"
	} else if cvsID == emptyValue {
		out.MeasureCompressed = nil
		out.MeasureSkipReason = "no --session-id and no active/paused convergence_session"
	} else {
		raw, err := runSchedulerConvergenceMeasureJSON(projectRoot, cvsID)
		if err != nil {
			out.MeasureCompressed = nil
			out.MeasureError = err.Error()
		} else {
			out.MeasureCompressed = compressWhatsNextMeasure(raw)
		}
	}

	out.ObserverTips = getObserverTips()

	out.Correspondence = buildWhatsNextCorrespondence(projectRoot, strings.TrimSpace(agentIDFlag), strings.TrimSpace(personaFlag), personaIDs, activeTask)

	amb := whatsnext.LoadKernelAmbience(projectRoot)
	var ambientPlanIDs []string
	for _, p := range activePlans {
		if p.ID != "" {
			ambientPlanIDs = append(ambientPlanIDs, p.ID)
		}
	}
	whatsnext.EnrichKernelAmbienceWithWorkflows(ctx, sp, &amb, planID, ambientPlanIDs)
	whatsnext.EnrichKernelAmbienceWithStaleTasks(ctx, sp, &amb, time.Now().UTC())
	hint := ""
	if out.Correspondence != nil {
		hint = out.Correspondence.NextActionHint
	}
	whatsnext.ApplySeatOperatingModeIn(&amb, projectRoot, strings.TrimSpace(agentIDFlag), hint)
	out.KernelAmbience = &amb
	out.AgentInstruction = whatsnext.PrependStewardProjectionForSeat(out.AgentInstruction, amb.StewardFocus, amb.SeatMode)

	compileWhatsNextDrive(&out, ctx, sp, strings.TrimSpace(personaFlag), personaIDs)
	out.AgentInstruction = whatsnext.ApplyFillToInstruction(out.AgentInstruction, out.FillItem)

	// Evaluate Strategic Readiness and Anticipatory Runway Watermark (delta_runway <= 1)
	if snap, candidates, err := tpm.EvaluateAndReplenishRunway(ctx, sp, projectRoot); err == nil && snap != nil {
		out.RunwayDepth = snap.RunwayDepth
		if snap.IsRunwayDepleted && len(candidates) > 0 {
			top := candidates[0]
			out.StrategicReplenishmentCue = fmt.Sprintf("Watermark delta_runway <= 1 (depth=%d). %d candidate bundle(s) ready for staging. Lead: %s [%s] (%d requirements)", snap.RunwayDepth, len(candidates), top.Title, top.Domain, len(top.RequirementIDs))
		}
	}

	// Materialized View: Probe zero-cost projection and trigger async background recovery if disrupted
	if lite, _ := whatsnext.GetOrRecoverPayload(ctx, sp, projectRoot, whatsnext.DefaultStalenessTolerance); lite != nil {
		out.MaterializedViewStale = lite.Stale
		out.MaterializedViewRecovering = lite.Recovering
		out.MaterializedViewDegradedReason = lite.DegradedReason
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL:
		return cli.FormatOutput(cmd, out)
	case cli.FormatYAML:
		return cli.FormatOutput(cmd, out)
	default:
		return writeWhatsNextTable(cmd, out)
	}
}

func getObserverTips() []string {
	return observerpkg.ReadCachedTips(zqkenv.ProjectRoot().Get())
}

func resolvePriorityPlanForWhatsNext(ctx context.Context, sp workflowStorage, explicit string, personaIDs []string) (planID string, summ *whatsNextPriorityPlan, activePlans []whatsNextPriorityPlan) {
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	storageCtx := pkgctx.NewStorageContext()

	// 1. Explicit plan request always wins — no work check needed.
	if explicit != emptyValue {
		obj, err := sp.Read(ctx, secCtx, explicit)
		if err == nil && obj != nil && hasPersonaMatch(obj, personaIDs) {
			pID, pSumm := summarizePriorityPlan(obj)
			return pID, pSumm, nil
		}
		res, lerr := sp.List(ctx, secCtx, storageCtx, storage.DefaultQueryFactory.
			ById(objects.KindPriorityPlan, explicit).
			Limit(1).
			IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, objects.FieldKeyActiveOrder, objects.FieldKeyPersonaRefs).
			Build())
		if lerr != nil || len(res.Objects) == 0 {
			res, lerr = sp.List(ctx, secCtx, storageCtx, storage.DefaultQueryFactory.
				ById(objects.KindStrategicPlan, explicit).
				Limit(1).
				IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, objects.FieldKeyActiveOrder, objects.FieldKeyPersonaRefs).
				Build())
		}
		if lerr == nil && len(res.Objects) > 0 {
			if hasPersonaMatch(res.Objects[0], personaIDs) {
				pID, pSumm := summarizePriorityPlan(res.Objects[0])
				return pID, pSumm, nil
			}
		}
	}

	// 2. Collect all candidate plans (execution + grooming next-columns).
	// Must stay in sync with pkg/workflow/whatsnext (PlanWhatsNextCandidateStatuses).
	var candidates []map[string]any
	for _, st := range objects.PlanWhatsNextCandidateStatuses() {
		for _, kind := range []string{objects.KindPriorityPlan, objects.KindStrategicPlan} {
			res, err := sp.List(ctx, secCtx, storageCtx, storage.DefaultQueryFactory.
				ByStatus(kind, st).
				IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, objects.FieldKeyActiveOrder, objects.FieldKeyPersonaRefs).
				Build())
			if err == nil {
				for _, obj := range res.Objects {
					if hasPersonaMatch(obj, personaIDs) {
						candidates = append(candidates, obj)
					}
				}
			}
		}
	}

	// Also try a broad list in case status filtering missed some (excluding terminal/archived)
	if len(candidates) == 0 {
		for _, kind := range []string{objects.KindPriorityPlan, objects.KindStrategicPlan} {
			res, err := sp.List(ctx, secCtx, storageCtx, storage.DefaultQueryFactory.
				Builder(kind).
				StatusNotIn(objects.ObjectStatusComplete, objects.ObjectStatusArchived, "cancelled").
				IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, objects.FieldKeyActiveOrder, objects.FieldKeyPersonaRefs).
				Build())
			if err == nil {
				for _, obj := range res.Objects {
					st, _ := obj[objects.FieldKeyStatus].(string)
					if objects.PlanStatusEligibleForWhatsNext(objects.KindPriorityPlan, st) && hasPersonaMatch(obj, personaIDs) {
						candidates = append(candidates, obj)
					}
				}
			}
		}
	}

	if len(candidates) == 0 {
		return "", nil, nil
	}

	candidates = preferSeatedPlansWithOpenWork(ctx, sp, candidates, personaIDs)

	// 4. Score: in_progress ≫ active ≫ paused; open (non-terminal) BLIs; lower active_order wins.
	var bestPlan map[string]any
	bestScore := -1

	for _, obj := range candidates {
		id, _ := obj[objects.FieldKeyID].(string)
		if id == emptyValue {
			continue
		}

		_, pSumm := summarizePriorityPlan(obj)
		if pSumm != nil {
			activePlans = append(activePlans, *pSumm)
		}

		st, _ := obj[objects.FieldKeyStatus].(string)
		score := countOpenLinkedBLIs(ctx, sp, id, personaIDs)
		score += cliPlanExecutionStatusBonus(st)
		score += objects.PlanSeatedPersonaBonus(personaIDs, obj)
		if kind, _ := obj[objects.FieldKeyKind].(string); kind == objects.KindPriorityPlan {
			score += 1_000_000
		}
		score -= cliActiveOrderPenalty(obj)
		if score > bestScore {
			bestScore = score
			bestPlan = obj
		}
	}

	// 5. If no plan has work, fall back to first candidate.
	if bestPlan == nil || bestScore <= 0 {
		pID, pSumm := summarizePriorityPlan(candidates[0])
		return pID, pSumm, activePlans
	}

	pID, pSumm := summarizePriorityPlan(bestPlan)
	return pID, pSumm, activePlans
}

// planHasWork returns true if the plan has at least one linked BLI.
func planHasWork(ctx context.Context, sp workflowStorage, planID string) bool {
	return countLinkedBLIs(ctx, sp, planID) > 0
}

// countLinkedBLIs returns the number of BLIs linked to a plan.
// Falls back to client-side filtering if the storage filter returns empty.
func countLinkedBLIs(ctx context.Context, sp workflowStorage, planID string) int {
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	storageCtx := pkgctx.NewStorageContext()

	res, err := sp.List(ctx, secCtx, storageCtx, storage.DefaultQueryFactory.
		ForPlan(objects.KindBacklogItem, planID).
		StatusNot(objects.ObjectStatusArchived).
		IncludeFields(objects.FieldKeyID, objects.FieldKeyPriorityPlanRef, objects.FieldKeyStatus).
		Build())
	if err == nil {
		return len(res.Objects)
	}
	return 0
}

// preferSeatedPlansWithOpenWork drops hollow matches when the seated persona has
// fuel on another candidate (shared in_progress umbrellas must not steal a
// persona-bound column). Unfiltered TPM views keep every candidate.
func preferSeatedPlansWithOpenWork(ctx context.Context, sp workflowStorage, candidates []map[string]any, personaIDs []string) []map[string]any {
	if len(personaIDs) == 0 || len(candidates) == 0 {
		return candidates
	}
	var fueled []map[string]any
	for _, obj := range candidates {
		id, _ := obj[objects.FieldKeyID].(string)
		if id != emptyValue && countOpenLinkedBLIs(ctx, sp, id, personaIDs) > 0 {
			fueled = append(fueled, obj)
		}
	}
	if len(fueled) == 0 {
		return candidates
	}
	return fueled
}

// countOpenLinkedBLIs counts non-terminal BLIs linked to a plan (child-owned membership).
// personaIDs filters BLIs the same way plan selection does (unassigned stays eligible).
func countOpenLinkedBLIs(ctx context.Context, sp workflowStorage, planID string, personaIDs []string) int {
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	storageCtx := pkgctx.NewStorageContext()
	countOpen := func(objs []map[string]any) int {
		n := 0
		for _, o := range objs {
			ref, _ := o[objects.FieldKeyPriorityPlanRef].(string)
			if ref != planID {
				continue
			}
			st, _ := o[objects.FieldKeyStatus].(string)
			if !objects.BacklogCountsAsOpenWork(st) {
				continue
			}
			if !hasPersonaMatch(o, personaIDs) {
				continue
			}
			if len(personaIDs) > 0 {
				if !objects.BacklogCountsAsExecutionFuel(st) {
					continue
				}
			} else if !objects.BacklogCountsAsOpenWork(st) {
				continue
			}
			n++
		}
		return n
	}
	res, err := sp.List(ctx, secCtx, storageCtx, storage.DefaultQueryFactory.
		ForPlan(objects.KindBacklogItem, planID).
		StatusNotIn(objects.ObjectStatusComplete, objects.ObjectStatusArchived).
		IncludeFields(objects.FieldKeyID, objects.FieldKeyStatus, objects.FieldKeyPriorityPlanRef, objects.FieldKeyPersonaRefs).
		Build())
	if err == nil {
		return countOpen(res.Objects)
	}
	return 0
}

func cliPlanExecutionStatusBonus(st string) int {
	return objects.PlanWhatsNextStatusBonus(objects.KindPriorityPlan, st)
}

// cliActiveOrderPenalty: lower active_order ranks higher; unset is lowest for active plans.
// execution_locked (in_progress) ≡ top-of-stack (penalty 0) even when shockwave cleared active_order.
func cliActiveOrderPenalty(obj map[string]any) int {
	return objects.PlanActiveOrderPenalty(objects.KindPriorityPlan, obj)
}

func summarizePriorityPlan(obj map[string]any) (string, *whatsNextPriorityPlan) {
	id, _ := obj[objects.FieldKeyID].(string)
	if id == emptyValue {
		return "", nil
	}
	title, _ := obj[objects.FieldKeyTitle].(string)
	st, _ := obj[objects.FieldKeyStatus].(string)
	return id, &whatsNextPriorityPlan{ID: id, Title: title, Status: st, Shaped: priorityPlanLooksShaped(obj)}
}

func priorityPlanLooksShaped(obj map[string]any) bool {
	if obj == nil {
		return false
	}
	if !objects.PlanHasWrittenIdentity(obj) || !objects.PlanHasWorkstreamLane(obj) {
		return false
	}
	switch refs := obj[objects.FieldKeyPersonaRefs].(type) {
	case []any:
		return len(refs) > 0
	case []string:
		return len(refs) > 0
	default:
		return false
	}
}

func countBacklogByStatus(ctx context.Context, sp workflowStorage, planID string, personaIDs []string) map[string]int {
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	storageCtx := pkgctx.NewStorageContext()

	out := map[string]int{}
	res, err := sp.List(ctx, secCtx, storageCtx, storage.DefaultQueryFactory.
		ForPlan(objects.KindBacklogItem, planID).
		StatusNot(objects.ObjectStatusArchived).
		IncludeFields(objects.FieldKeyID, objects.FieldKeyStatus, objects.FieldKeyPriorityPlanRef, objects.FieldKeyPersonaRefs).
		Build())
	if err != nil || len(res.Objects) == 0 {
		return out
	}

	for _, o := range res.Objects {
		if !hasPersonaMatch(o, personaIDs) {
			continue
		}
		st, _ := o[objects.FieldKeyStatus].(string)
		if st == emptyValue {
			st = "unknown"
		}
		out[st]++
	}
	return out
}

// resolveWhatsNextPersonaIDs prefers --persona-id, then peer_seats[agent-id], then
// the process security-context personas. Seat binding must win over the caller
// identity so `whats-next --agent-id <peer>` is that peer's column.
func resolveWhatsNextPersonaIDs(ctx context.Context, sp workflowStorage, projectRoot, personaFlag, agentID string) []string {
	if strings.TrimSpace(personaFlag) != "" {
		return []string{strings.TrimSpace(personaFlag)}
	}
	if ref := agentfeed.SeatPersonaRef(projectRoot, agentID); ref != "" {
		return []string{ref}
	}
	return getAgentPersonaIDs(ctx, sp, "")
}

// planPersonaFilter keeps seated persona filters for worker seats. Operator/TPM
// seats (or no --agent-id) still drop the filter so hunger compiles the Gantt lead.
func planPersonaFilter(ctx context.Context, sp workflowStorage, projectRoot, agentID string, personaIDs []string) []string {
	if strings.TrimSpace(agentID) != "" && !whatsnext.IsTPMProcessAdminSeatIn(projectRoot, agentID) {
		return personaIDs
	}
	return leadColumnPersonaIDs(ctx, sp, personaIDs)
}

func getAgentPersonaIDs(ctx context.Context, sp workflowStorage, explicitPersonaID string) []string {
	if explicitPersonaID != "" {
		return []string{explicitPersonaID}
	}
	if sp == nil {
		return nil
	}

	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil || len(secCtx.Roles) == 0 {
		return nil
	}

	sysSecCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	res, err := sp.List(ctx, sysSecCtx, storageCtx, storage.DefaultQueryFactory.
		NotArchived(objects.KindPersona).
		IncludeFields(objects.FieldKeyID, objects.FieldKeyRole, objects.FieldKeyStatus).
		Build())
	if err != nil || len(res.Objects) == 0 {
		return nil
	}

	var personaIDs []string
	for _, obj := range res.Objects {
		role, _ := obj[objects.FieldKeyRole].(string)
		if role == "" {
			continue
		}
		for _, secRole := range secCtx.Roles {
			if strings.EqualFold(role, secRole) {
				id, _ := obj[objects.FieldKeyID].(string)
				personaIDs = append(personaIDs, id)
				break
			}
		}
	}
	return personaIDs
}

// leadColumnPersonaIDs drops plan/BLI persona filters for operator/TPM seats so
// hunger compiles from the lead Gantt. Evaluate still uses the original IDs.
func leadColumnPersonaIDs(ctx context.Context, sp workflowStorage, personaIDs []string) []string {
	if len(personaIDs) == 0 || sp == nil {
		return personaIDs
	}
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	for _, id := range personaIDs {
		obj, err := sp.Read(ctx, secCtx, id)
		if err != nil || obj == nil {
			continue
		}
		role, _ := obj[objects.FieldKeyRole].(string)
		if objects.PersonaSeesLeadGantt(role) {
			return nil
		}
	}
	return personaIDs
}

func hasPersonaMatch(obj map[string]any, personaIDs []string) bool {
	if len(personaIDs) == 0 {
		return true // no filtering
	}
	refsAny := obj[objects.FieldKeyPersonaRefs]
	if refsAny == nil {
		// Unassigned stays eligible (same contract as pkg/workflow/whatsnext).
		return true
	}
	if refs, ok := refsAny.([]any); ok {
		if len(refs) == 0 {
			return true
		}
		for _, rAny := range refs {
			if r, okStr := rAny.(string); okStr {
				for _, pid := range personaIDs {
					if strings.EqualFold(r, pid) {
						return true
					}
				}
			}
		}
	} else if refStrs, ok := refsAny.([]string); ok {
		if len(refStrs) == 0 {
			return true
		}
		for _, r := range refStrs {
			for _, pid := range personaIDs {
				if strings.EqualFold(r, pid) {
					return true
				}
			}
		}
	}
	return false
}

func listActiveOrPausedConvergenceSessions(ctx context.Context, sp workflowStorage) []whatsNextCVSRow {
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	storageCtx := pkgctx.NewStorageContext()
	res, err := sp.List(ctx, secCtx, storageCtx, storage.DefaultQueryFactory.
		Builder(objects.KindConvergenceSession).
		StatusIn("active", "paused").
		IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyCurrentPhase, objects.FieldKeyStatus).
		Build())
	if err != nil {
		return nil
	}
	var rows []whatsNextCVSRow
	for _, obj := range res.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		if id == emptyValue {
			continue
		}
		st, _ := obj[objects.FieldKeyStatus].(string)
		title, _ := obj[objects.FieldKeyTitle].(string)
		phase, _ := obj[objects.FieldKeyCurrentPhase].(string)
		rows = append(rows, whatsNextCVSRow{
			ID: id, Title: title, CurrentPhase: phase, Status: st,
		})
	}
	return rows
}

func pickDefaultMeasureSessionID(rows []whatsNextCVSRow) string {
	for _, row := range rows {
		t := strings.ToLower(row.Title)
		if strings.Contains(t, "data cell") || strings.Contains(t, "datacell") {
			return row.ID
		}
	}
	if len(rows) > 0 {
		return rows[0].ID
	}
	return ""
}

func runSchedulerConvergenceMeasureJSON(projectRoot, sessionID string) (map[string]any, error) {
	exe, err := resolveZQKCLIExecutable()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), measureSubprocessTimeout) // Background: request-or-shutdown derived
	defer cancel()
	c := execwrap.CommandContext(ctx, exe, "scheduler", "convergence", "measure", "--format", "json", "--session-id", sessionID)
	c.Dir = projectRoot
	out, err := c.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, errfmt.Errorf("%w: %s", err, string(ee.Stderr))
		}
		return nil, err
	}
	var m map[string]any
	if jerr := json.Unmarshal(out, &m); jerr != nil {
		return nil, jerr
	}
	return m, nil
}

// resolveZQKCLIExecutable returns the path to the running zqk binary (for subprocess measure).
var resolveZQKCLIExecutable = func() (string, error) {
	if p, err := fileutil.Executable(); err == nil && strings.TrimSpace(p) != "" {
		return p, nil
	}
	if len(os.Args) > 0 {
		return exec.LookPath(os.Args[0])
	}
	return "", errfmt.Errorf("cannot resolve CLI executable")
}

// compressWhatsNextMeasure reduces scheduler convergence measure JSON to a small agent-facing map.
func compressWhatsNextMeasure(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	rsc, _ := m["rollup_status_core"].(map[string]any)
	if rsc == nil {
		rsc = map[string]any{}
	}
	sug, _ := m["suggested_convergence_session_fields"].(map[string]any)
	if sug == nil {
		sug = map[string]any{}
	}
	pr, _ := sug["phase_router"].(map[string]any)
	if pr == nil {
		pr = map[string]any{}
	}
	out := map[string]any{
		"convergence_session_id":                        m["convergence_session_id"],
		objects.FieldKeyPrimaryMeasurementOutcome:       m[objects.FieldKeyPrimaryMeasurementOutcome],
		objects.FieldKeyPrimaryMeasurementOutcomeDetail: m[objects.FieldKeyPrimaryMeasurementOutcomeDetail],
		objects.FieldKeyDeltaAssessment:                 m[objects.FieldKeyDeltaAssessment],
		"health_watermark_rfc3339":                      m["health_watermark_rfc3339"],
		"had_failure_in_window":                         m["had_failure_in_window"],
		"failing_fingerprints_now":                      m["failing_fingerprints_now"],
		objects.FieldKeyNextActionHint:                  m[objects.FieldKeyNextActionHint],
		objects.FieldKeyReadyForSessionCompletion:       m[objects.FieldKeyReadyForSessionCompletion],
		"phase_router": map[string]any{
			"phase_alignment":           pr["phase_alignment"],
			"suggested_current_phase":   pr["suggested_current_phase"],
			"routing_profile":           pr["routing_profile"],
			"measurement_implied_phase": pr["measurement_implied_phase"],
		},
		"rollup_status":           rsc["rollup_status"],
		"recommended_next_action": rsc["recommended_next_action"],
	}
	if na, ok := sug[objects.FieldKeyNextAction].(string); ok {
		out["next_action_suggested"] = na
	} else {
		out["next_action_suggested"] = nil
	}
	return out
}

func writeWhatsNextTable(cmd *cobra.Command, out whatsNextOut) error {
	var b strings.Builder
	b.WriteString("Workflow whats-next (composite)\n")
	b.WriteString("=================================\n")
	b.WriteString("Schema: " + out.Schema + "\n")
	if out.PriorityPlan != nil {
		fmt.Fprintf(&b, "Priority plan: %s (%s) [%s]\n", out.PriorityPlan.Title, out.PriorityPlan.ID, out.PriorityPlan.Status)
	} else {
		b.WriteString("Priority plan: (none resolved)\n")
	}
	if out.PackagingCue != emptyValue {
		fmt.Fprintf(&b, "Packaging cue: %s\n", out.PackagingCue)
	}
	if out.StrategicReplenishmentCue != emptyValue {
		fmt.Fprintf(&b, "Strategic replenishment (anticipatory runway <= 1):\n  %s\n", out.StrategicReplenishmentCue)
	}
	if out.MaterializedViewRecovering || out.MaterializedViewStale {
		fmt.Fprintf(&b, "Materialized view: recovering (%s)\n", out.MaterializedViewDegradedReason)
	}
	if len(out.BacklogCountsByStatus) > 0 {
		b.WriteString("Backlog counts by status:\n")
		for st, n := range out.BacklogCountsByStatus {
			fmt.Fprintf(&b, "  %s: %d\n", st, n)
		}
	}
	b.WriteString(fmt.Sprintf("Convergence sessions (active/paused): %d\n", len(out.ConvergenceSessionsActive)))
	for _, row := range out.ConvergenceSessionsActive {
		fmt.Fprintf(&b, "  - %s :: %s [%s] %s\n", row.ID, row.Title, row.Status, row.CurrentPhase)
	}
	if out.MeasureSessionID != nil {
		b.WriteString("Measure session: " + *out.MeasureSessionID + "\n")
	}
	if out.MeasureSkipReason != emptyValue {
		b.WriteString("Measure: skipped (" + out.MeasureSkipReason + ")\n")
	} else if out.MeasureError != emptyValue {
		b.WriteString("Measure error: " + out.MeasureError + "\n")
	} else if out.MeasureCompressed != nil {
		b.WriteString("Measure: compressed JSON present (use --format json)\n")
	}
	if len(out.ObserverTips) > 0 {
		b.WriteString("\nObserver Recommendations:\n")
		for _, tip := range out.ObserverTips {
			b.WriteString(fmt.Sprintf("  * %s\n", tip))
		}
	}
	if out.KernelAmbience != nil && out.KernelAmbience.StewardFocus != "" {
		b.WriteString("\nKernel steward (ambience):\n  ")
		b.WriteString(out.KernelAmbience.StewardFocus)
		b.WriteString("\n")
	}
	if out.FillItem != nil && out.FillItem.CommandHint != "" {
		b.WriteString("\nKernel fill (no live ATK):\n  ")
		b.WriteString(out.FillItem.Kind)
		b.WriteString(": ")
		b.WriteString(out.FillItem.Reason)
		b.WriteString("\n  ")
		b.WriteString(out.FillItem.CommandHint)
		b.WriteString("\n")
	}
	if out.GuidingStep != nil && out.GuidingStep.GuidingStep != "" {
		b.WriteString("\nGuiding step (hunger):\n  ")
		b.WriteString(out.GuidingStep.PolicyID)
		b.WriteString(": ")
		b.WriteString(out.GuidingStep.GuidingStep)
		b.WriteString("\n")
	}
	if out.Correspondence != nil {
		b.WriteString("\nCorrespondence (seat):\n")
		if out.Correspondence.SkipReason != "" {
			fmt.Fprintf(&b, "  skip: %s\n", out.Correspondence.SkipReason)
		} else {
			fmt.Fprintf(&b, "  agent_id=%s persona_id=%s hint=%s\n", out.Correspondence.AgentID, out.Correspondence.PersonaID, out.Correspondence.NextActionHint)
			fmt.Fprintf(&b, "  inbox_unacked: %d  outbox_awaiting_peer_ack: %d  registered_callbacks: %d\n", len(out.Correspondence.InboxUnacked), len(out.Correspondence.OutboxAwaitingPeerAck), len(out.Correspondence.RegisteredCallbacks))
			if out.Correspondence.ActiveAgentTask != nil {
				fmt.Fprintf(&b, "  active_agent_task: %s [%s] %s\n", out.Correspondence.ActiveAgentTask.ID, out.Correspondence.ActiveAgentTask.Status, out.Correspondence.ActiveAgentTask.Title)
			}
			for _, item := range out.Correspondence.InboxUnacked {
				fmt.Fprintf(&b, "  inbox: %s %s\n", item.EventID, item.Summary)
			}
			for _, a := range out.Correspondence.RegisteredCallbacks {
				fmt.Fprintf(&b, "  await: %s event=%s action=%s\n", a.ID, a.EventID, a.Action)
			}
		}
	}
	return cli.WriteOutput(cmd, []byte(b.String()))
}

func compileWhatsNextDrive(out *whatsNextOut, ctx context.Context, sp workflowStorage, personaFlag string, personaIDs []string) {
	if out == nil {
		return
	}
	out.FillItem = whatsnext.CompileFillItem(out.KernelAmbience)
	planned, inProgress, exploring, validated := 0, 0, 0, 0
	if out.BacklogCountsByStatus != nil {
		planned = out.BacklogCountsByStatus["planned"]
		inProgress = out.BacklogCountsByStatus["in_progress"]
		exploring = out.BacklogCountsByStatus["exploring"]
		validated = out.BacklogCountsByStatus["validated"]
	}
	inbox, outbox := 0, 0
	if out.Correspondence != nil {
		inbox = len(out.Correspondence.InboxUnacked)
		outbox = len(out.Correspondence.OutboxAwaitingPeerAck)
	}
	planStatus := ""
	if out.PriorityPlan != nil {
		planStatus = out.PriorityPlan.Status
	}
	branchAhead := 0
	if out.KernelAmbience != nil {
		branchAhead = out.KernelAmbience.BranchAhead
	}
	event := interactionpolicy.ClassifyAmbience(interactionpolicy.Ambience{
		Planned:        planned,
		InProgress:     inProgress,
		Exploring:      exploring,
		Validated:      validated,
		PlanStatus:     planStatus,
		InboxUnacked:   inbox,
		OutboxAwaiting: outbox,
		BranchAhead:    branchAhead,
	})
	if event == "" {
		return
	}
	personaRef := personaFlag
	if personaRef == "" && len(personaIDs) > 0 {
		personaRef = personaIDs[0]
	}
	if personaRef == "" {
		personaRef = strings.TrimSpace(zqkenv.Persona().Get())
	}
	var persona map[string]any
	sec := pkgctx.GetSecurityContext(ctx)
	if sec == nil {
		sec = pkgctx.NewSystemSecurityContext()
	}
	if personaRef != "" && sp != nil {
		if obj, err := sp.Read(ctx, sec, personaRef); err == nil {
			persona = obj
		}
	}
	res := interactionpolicy.Evaluate(persona, event)
	if !res.Matched || res.Step == nil {
		return
	}
	hydrated := false
	// Idle catalog text must not be replaced by a GROOM-AHEAD / process-admin
	// policy body (those overlay as "column is empty").
	if !interactionpolicy.SkipCASOverlay(event) && sp != nil && res.Step.PolicyID != "" {
		if pol, err := sp.Read(ctx, sec, res.Step.PolicyID); err == nil && interactionpolicy.OverlayFromPolicy(res.Step, pol) {
			hydrated = true
		}
	}
	drive := res.Step.GuidingStep
	hint := res.Step.CommandHint
	fill := out.FillItem
	if event == interactionpolicy.EventIdle {
		if outbox > 0 {
			drive = strings.TrimSpace(drive + " Outbox awaiting peer_ack: do not remint the same wake; keep working other kernel items.")
		}
		hint = ""
		if fill != nil && fill.CommandHint != "" {
			hint = fill.CommandHint
			drive = strings.TrimSpace(drive + " Do this fill instead of shutdown or reminting ORCHESTRATE_PLAN: " + fill.Reason + ".")
		}
	}
	if event == interactionpolicy.EventShovelReadyEmpty || event == interactionpolicy.EventStratplanAhead {
		amb := stratplanAmbientFromWhatsNext(out)
		drive = interactionpolicy.AppendStratplanAmbient(drive, amb)
		if event == interactionpolicy.EventStratplanAhead {
			hint = interactionpolicy.CompileStratplanCommandHint(amb)
		}
	}
	out.GuidingStep = &whatsNextGuidingStep{
		Event:       res.Event,
		PolicyID:    res.Step.PolicyID,
		GuidingStep: drive,
		CommandHint: hint,
		Hydrated:    hydrated,
	}
}

func stratplanAmbientFromWhatsNext(out *whatsNextOut) interactionpolicy.StratplanAmbient {
	a := interactionpolicy.StratplanAmbient{Counts: out.BacklogCountsByStatus}
	if out.PriorityPlan != nil {
		a.LeadID = out.PriorityPlan.ID
		a.LeadTitle = out.PriorityPlan.Title
		a.LeadStatus = out.PriorityPlan.Status
	}
	if out.KernelAmbience != nil {
		a.DraftPlaneTotal = out.KernelAmbience.DraftPlaneTotal
		if al := out.KernelAmbience.StrategicAlignment; al != nil && al.Available {
			a.AlignOK = true
			a.AlignScore = al.AlignmentScore
			a.GoalGaps = al.ItemsWithoutGoals
			a.AlignMeasuredAt = al.MeasuredAt
		}
	}
	lead := ""
	if out.PriorityPlan != nil {
		lead = out.PriorityPlan.ID
	}
	refs := make([]interactionpolicy.AmbientPlanRef, 0, len(out.ActivePlans))
	for _, p := range out.ActivePlans {
		refs = append(refs, interactionpolicy.AmbientPlanRef{ID: p.ID, Status: p.Status, Shaped: p.Shaped})
	}
	a.NextPlans = interactionpolicy.RankNextPlanLabels(lead, refs)
	interactionpolicy.FinalizeAlignFreshness(&a, time.Now().UTC())
	return a
}

func buildWhatsNextCorrespondence(projectRoot, agentID, personaFlag string, personaIDs []string, activeTask map[string]any) *whatsNextCorrespondence {
	corr := &whatsNextCorrespondence{
		InboxUnacked:          []agentfeed.CorrespondenceItem{},
		OutboxAwaitingPeerAck: []agentfeed.CorrespondenceItem{},
	}
	if agentID == "" {
		corr.SkipReason = "agent_id_required"
		return corr
	}
	personaRef := personaFlag
	if personaRef == "" && len(personaIDs) > 0 {
		personaRef = personaIDs[0]
	}
	corr.AgentID = agentID
	corr.PersonaID = personaRef
	if activeTask != nil {
		id, _ := activeTask[objects.FieldKeyID].(string)
		st, _ := activeTask[objects.FieldKeyStatus].(string)
		title, _ := activeTask[objects.FieldKeyTitle].(string)
		corr.ActiveAgentTask = &whatsNextActiveTask{ID: id, Status: st, Title: title}
	}
	snap, err := agentfeed.LoadCorrespondence(projectRoot, agentfeed.Seat{
		AgentID:    agentID,
		PersonaRef: personaRef,
	}, 20)
	if err != nil {
		corr.SkipReason = err.Error()
		return corr
	}
	if snap.SkipReason != "" {
		corr.SkipReason = snap.SkipReason
		return corr
	}
	corr.InboxUnacked = snap.InboxUnacked
	if corr.InboxUnacked == nil {
		corr.InboxUnacked = []agentfeed.CorrespondenceItem{}
	}
	corr.OutboxAwaitingPeerAck = snap.OutboxAwaitingPeerAck
	if corr.OutboxAwaitingPeerAck == nil {
		corr.OutboxAwaitingPeerAck = []agentfeed.CorrespondenceItem{}
	}
	corr.RegisteredCallbacks = snap.RegisteredCallbacks
	corr.NextActionHint = snap.NextActionHint
	if corr.ActiveAgentTask != nil && corr.NextActionHint == agentfeed.HintContinue {
		corr.NextActionHint = agentfeed.HintStandbyForbidden
	}
	if len(corr.InboxUnacked) > 0 {
		corr.NextActionHint = agentfeed.HintAckThenContinue
	}
	return corr
}
