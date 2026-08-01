package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/primaryorch"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/workflow/whatsnext"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

const (
	// capAgentEscalationThreshold is the number of consecutive failures before
	// escalating to the AI agent via AppleScript prompt injection.
	// 3 cycles at */5 = ~15 minutes of failures before agent gets involved.
	capAgentEscalationThreshold = 3

	// capHumanEscalationThreshold is the number of consecutive failures before
	// escalating to the human inbox. Only reached if agent recovery also fails.
	// 6 cycles at */5 = ~30 minutes — agent had 3 chances and couldn't fix it.
	capHumanEscalationThreshold = 6

	// capFailureTracker is the state file that tracks consecutive failures.
	capFailureTrackerFile = "cap_failure_tracker.json"
)

type CapOrchestratorHandler struct {
	storage     storagepkg.ObjectStorageProvider
	projectRoot string
	logger      logging.Logger
	escalation  *EscalationChain
}

// wakeAgentAndScheduleHourglass triggers the agent wake via the MCP chat channel and sets an hourglass timer
func (h *CapOrchestratorHandler) wakeAgentAndScheduleHourglass(taskID, persona string) {
	// 1. Wake Agent via chat channel (simulating MCP chat_inject)
	eventPath := datacell.AgentChatChannelEventsJSONLPath(h.projectRoot)
	if err := os.MkdirAll(filepath.Dir(eventPath), 0755); err == nil {
		if f, err := os.OpenFile(eventPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			timestamp := time.Now().UTC()
			event := map[string]any{
				"timestamp": timestamp.Format(time.RFC3339),
				"sender":    "cap_orchestrator",
				"message":   fmt.Sprintf("Wake up! Task %s has been assigned to %s", taskID, persona),
			}
			if b, err := json.Marshal(event); err == nil {
				_, _ = f.Write(append(b, '\n'))
			}
			f.Close()
		}
	}

	// If persona is TPM, wake the host/project primary orchestrator via pluggable
	// adapters (agent_chat / script / inbox). Do not hardcode wake-agy — that is
	// one vendor adapter selected only when primary_orchestrator.json says so.
	if strings.EqualFold(persona, "tpm") || strings.Contains(strings.ToLower(persona), "tpm") {
		planID := h.resolveWakePlanID(context.Background(), taskID)
		top := h.topOpenPlanBLIs(context.Background(), planID, 3)
		msg := fmt.Sprintf("CAP Orchestrator wake: Task %s assigned to %s", taskID, persona)
		res, err := primaryorch.WakePrimary(context.Background(), h.projectRoot, primaryorch.WakeRequest{
			TaskID:  taskID,
			Persona: persona,
			PlanID:  planID,
			TopBLIs: top,
			Message: msg,
		})
		if err != nil {
			h.logger.Error("cap_orchestrator_primary_wake_failed", err,
				logging.String("task_id", taskID),
				logging.String("persona", persona),
			)
		} else {
			h.logger.Info("cap_orchestrator_primary_wake",
				logging.String("task_id", taskID),
				logging.String("plan_id", planID),
				logging.String("agent_id", res.AgentID),
				logging.String("adapter", res.Adapter),
				logging.String("delivered_to", res.DeliveredTo),
			)
		}
	}

	// 2. Schedule Hourglass Flip pattern
	// Set a deadline 5 minutes from now. If task remains stuck, hourglass flips.
	deadlineStr := time.Now().Add(5 * time.Minute).Format(time.RFC3339)
	schedulerRoot := paths.ResolvePathFromCacheOrConstant(h.projectRoot, "scheduler", filepath.Join(paths.ProjectDataDir, paths.SchedulerDir))
	hourglassDir := filepath.Join(schedulerRoot, "hourglass")
	_ = os.MkdirAll(hourglassDir, 0755)

	filePath := filepath.Join(hourglassDir, taskID+".json")
	info := map[string]any{
		"task_id":                 taskID,
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyType:      "deadline",
		objects.FieldKeyExpiresAt: deadlineStr,
	}
	if data, err := json.Marshal(info); err == nil {
		_ = os.WriteFile(filePath, data, 0644)
	}
}

// resolveWakePlanID returns taskID when it is a PLAN-*, else the first active priority_plan.
func (h *CapOrchestratorHandler) resolveWakePlanID(ctx context.Context, taskID string) string {
	if strings.HasPrefix(strings.TrimSpace(taskID), "PLAN-") {
		return strings.TrimSpace(taskID)
	}
	if h.storage == nil {
		return ""
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	res, err := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: objects.KindPriorityPlan})
	if err != nil || res == nil {
		return ""
	}
	for _, obj := range res.Objects {
		st, _ := obj[objects.FieldKeyStatus].(string)
		if strings.EqualFold(st, objects.ObjectStatusActive) {
			if id, _ := obj[objects.FieldKeyID].(string); id != "" {
				return id
			}
		}
	}
	return ""
}

// topOpenPlanBLIs returns up to n open P0/P1 backlog briefs for planID (priority order).
func (h *CapOrchestratorHandler) topOpenPlanBLIs(ctx context.Context, planID string, n int) []primaryorch.BacklogBrief {
	if n <= 0 || planID == "" || h.storage == nil {
		return nil
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	res, err := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: objects.KindBacklogItem})
	if err != nil || res == nil {
		return nil
	}
	type ranked struct {
		rank int
		b    primaryorch.BacklogBrief
	}
	var rows []ranked
	for _, obj := range res.Objects {
		ref, _ := obj[objects.FieldKeyPriorityPlanRef].(string)
		if ref != planID {
			continue
		}
		st, _ := obj[objects.FieldKeyStatus].(string)
		ls := strings.ToLower(strings.TrimSpace(st))
		switch ls {
		case objects.ObjectStatusComplete, "completed", objects.ObjectStatusArchived, "cancelled", "canceled":
			continue
		}
		tier, _ := obj[objects.FieldKeyPriorityTier].(string)
		tier = strings.ToUpper(strings.TrimSpace(tier))
		rank := 50
		switch tier {
		case "P0":
			rank = 0
		case "P1":
			rank = 1
		case "P2":
			rank = 2
		case "P3":
			rank = 3
		}
		id, _ := obj[objects.FieldKeyID].(string)
		title, _ := obj[objects.FieldKeyTitle].(string)
		if title == "" {
			if desc, _ := obj[objects.FieldKeyDescription].(string); desc != "" {
				title = desc
				if len(title) > 80 {
					title = title[:80] + "…"
				}
			}
		}
		rows = append(rows, ranked{rank: rank, b: primaryorch.BacklogBrief{ID: id, Title: title, Tier: tier}})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].rank != rows[j].rank {
			return rows[i].rank < rows[j].rank
		}
		return rows[i].b.ID < rows[j].b.ID
	})
	out := make([]primaryorch.BacklogBrief, 0, n)
	for _, r := range rows {
		if r.b.ID == "" {
			continue
		}
		out = append(out, r.b)
		if len(out) >= n {
			break
		}
	}
	return out
}

// NewCapOrchestratorHandler creates a CAP orchestrator with the default
// escalation chain: agent (via configurable command) → human (via inbox).
// The chain is vendor-neutral — swap providers without touching handler logic.
func NewCapOrchestratorHandler(storage storagepkg.ObjectStorageProvider, projectRoot string, logger logging.Logger) JobHandler {
	return &CapOrchestratorHandler{
		storage:     storage,
		projectRoot: projectRoot,
		logger:      logger,
		escalation:  defaultEscalationChain(projectRoot),
	}
}

// NewCapOrchestratorHandlerWithEscalation creates a CAP orchestrator with a
// custom escalation chain for testing or alternative delivery mechanisms.
func NewCapOrchestratorHandlerWithEscalation(storage storagepkg.ObjectStorageProvider, projectRoot string, logger logging.Logger, chain *EscalationChain) JobHandler {
	return &CapOrchestratorHandler{
		storage:     storage,
		projectRoot: projectRoot,
		logger:      logger,
		escalation:  chain,
	}
}

// defaultEscalationChain builds the standard two-tier escalation:
// Tier 1 (3 failures): Agent recovery via configurable command
// Tier 2 (6 failures): Human inbox as last resort
func defaultEscalationChain(projectRoot string) *EscalationChain {
	agentProvider := buildAgentProvider(projectRoot)
	humanProvider := &InboxEscalationProvider{
		InboxDir: filepath.Join(projectRoot, paths.ProjectDataDir, "inbox", "human"),
	}

	return &EscalationChain{
		Tiers: []EscalationTier{
			{Threshold: capAgentEscalationThreshold, Provider: agentProvider, Name: "agent"},
			{Threshold: capHumanEscalationThreshold, Provider: humanProvider, Name: "human"},
		},
	}
}

// buildAgentProvider constructs the agent escalation provider from config.
// Checks .zqk/config/escalation.json for a custom command, falls back to
// the coder_agent inbox if no command is configured.
func buildAgentProvider(projectRoot string) EscalationProvider {
	// Check for custom escalation command in project config
	configPath := filepath.Join(projectRoot, paths.ProjectDataDir, "config", "escalation.json")
	if data, err := os.ReadFile(configPath); err == nil {
		var config struct {
			AgentCommand string   `json:"agent_command"`
			AgentArgs    []string `json:"agent_args"`
		}
		if json.Unmarshal(data, &config) == nil && config.AgentCommand != "" {
			return &CommandEscalationProvider{
				Command:     config.AgentCommand,
				Args:        config.AgentArgs,
				ProjectRoot: projectRoot,
			}
		}
	}

	// Default: write to coder_agent inbox (works regardless of provider)
	return &InboxEscalationProvider{
		InboxDir: filepath.Join(projectRoot, paths.ProjectDataDir, "inbox", "coder_agent"),
	}
}

func (h *CapOrchestratorHandler) prepareCmd(cmd *exec.Cmd) *exec.Cmd {
	cmd.Dir = h.projectRoot
	// Strip then set keys so Unix first-match env semantics cannot keep a parent
	// ZQK_GRAPH_ENABLED=true (Memgraph hangs were killing review under the CAP timeout).
	// TRACK: ATK-REDACTED — CAP review freshness; prefer daemon-level
	// graph policy once overnight CAP gate is green without per-child overrides.
	graphKey := zqkenv.GraphEnabled()
	adminGraphKey := zqkenv.AdminGraphEnabled()
	const apiKey = "ZQK_API_KEY"
	env := make([]string, 0, len(os.Environ())+3)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, graphKey+"=") || strings.HasPrefix(e, adminGraphKey+"=") ||
			strings.HasPrefix(e, apiKey+"=") ||
			strings.HasPrefix(e, "ZQK_GRAPH_ENABLED=") || strings.HasPrefix(e, "ZQK_ADMIN_GRAPH_ENABLED=") {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = append(env,
		apiKey+"=account:system",
		graphKey+"=false",
		adminGraphKey+"=false",
	)
	return cmd
}

func (h *CapOrchestratorHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	exe, err := h.resolveCLIExecutable()
	if err != nil {
		return errfmt.Errorf("failed to resolve zqk executable: %w", err)
	}

	// Review runs a full system check (~1m typical; longer under load). 5m was
	// starving check under graph/memgraph contention (exit status 1 via cancel).
	timeoutCtx, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()

	// Step 1: Get current system state via whats-next natively
	req := &whatsnext.QueryRequest{
		SkipMeasure: true, // we don't need measure data for orchestration routing
	}
	result, err := whatsnext.Execute(timeoutCtx, h.storage, h.projectRoot, req)
	if err != nil {
		h.logger.Error("cap_orchestrator_whats_next_failed", err)
		return errfmt.Errorf("whats-next query failed: %w", err)
	}

	instruction := result.AgentInstruction
	h.logger.Info("cap_orchestrator_instruction", logging.String("instruction", instruction))

	var planID string
	if result.PriorityPlan != nil {
		planID = result.PriorityPlan.ID
	}

	var activePlans []string
	if len(result.ActivePlans) > 0 {
		for _, p := range result.ActivePlans {
			if p.ID != "" {
				activePlans = append(activePlans, p.ID)
			}
		}
	} else if planID != "" {
		activePlans = append(activePlans, planID)
	}

	// Step 2: Route to stage-specific handler, tracking failures for escalation
	var stageErr error
	if strings.HasPrefix(instruction, "cap_stage_") {
		h.markStageEntered(instruction, planID)
	}
	switch instruction {
	case "cap_stage_review":
		stageErr = h.executeReviewStage(timeoutCtx, exe)
	case "cap_stage_metrics":
		stageErr = h.executeMetricsStage(timeoutCtx, exe)
	case "cap_stage_self_improvement":
		stageErr = h.executeSelfImprovementStage(timeoutCtx, exe)
	case "cap_stage_grooming":
		stageErr = h.executeGroomingStage(timeoutCtx, exe, planID)
	case "cap_stage_sentinel":
		stageErr = h.executeSentinelStage(timeoutCtx, exe)
	case "", "wait", "shutdown":
		h.logger.Info("cap_orchestrator_idle", logging.String("instruction", instruction))
		return nil
	default:
		// cap_stage_planning, cap_stage_design, cap_stage_orchestrating
		// All dispatch work via agent orchestrate
		if len(activePlans) == 0 {
			h.logger.Info("cap_orchestrator_no_active_plans", logging.String("stage", instruction))
		} else {
			var errs []string
			planPool := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "plan_dispatcher", "dispatching active plans", len(activePlans), len(activePlans))
			planPool.Start(timeoutCtx)

			for _, pid := range activePlans {
				pid := pid
				_ = planPool.Submit(timeoutCtx, func(workerCtx context.Context) error {
					// Auto-recover errored tasks for this plan so the orchestrator can dispatch/resume them
					if recErr := h.autoRecoverPlanTasks(workerCtx, pid); recErr != nil {
						h.logger.Error("cap_orchestrator_auto_recover_failed", recErr, logging.String("plan_id", pid))
					}
					if err := h.executeDispatchStage(workerCtx, exe, pid, instruction); err != nil {
						// Graph/CAS ghosts in ActivePlans should not fail the whole CAP tick when
						// the primary Ambient plan can still dispatch.
						// TRACK: ATK-REDACTED — tighten whats-next ActivePlans
						// to file-backed plans only; then remove this soft-skip.
						msg := err.Error()
						if strings.Contains(msg, "failed to fetch priority plan") {
							h.logger.Warn("cap_orchestrator_plan_dispatch_skipped_missing",
								logging.String("plan_id", pid),
								logging.String("error", msg))
							return nil
						}
						errs = append(errs, fmt.Sprintf("plan %s: %v", pid, err))
						return err
					}
					return nil
				})
			}
			planPool.Stop()

			if len(errs) > 0 {
				stageErr = errfmt.Errorf("multiple plan dispatch errors: %s", strings.Join(errs, "; "))
			}
		}
	}

	// Step 3: Failures escalate; successful handler work only advances when
	// stage-specific delivery evidence exists (grooming ≠ AGI create alone).
	if stageErr != nil {
		h.recordFailure(instruction, stageErr)
		return stageErr
	}
	if advErr := h.maybeAdvanceCAPStage(timeoutCtx, instruction); advErr != nil {
		// Hold without clearing or escalating — waiting on TPM/worker artifacts.
		return nil
	}
	h.clearFailures()
	return nil
}

// executeDispatchStage handles planning, design, grooming, and orchestrating stages
// by provisioning a Sub-Kernel (Federated Team Pod). It fetches all personas and spawns
// concurrent agent orchestrate subprocesses, allowing the team to collaborate elastically.
func (h *CapOrchestratorHandler) executeDispatchStage(ctx context.Context, exe, planID, instruction string) error {
	if planID == "" {
		h.logger.Info("cap_orchestrator_no_plan", logging.String("stage", instruction))
		return nil
	}

	var activePersonas []string

	// Attempt to resolve from priority_plan -> team_configuration_ref
	planCmd := exec.CommandContext(ctx, exe, "object", "show", planID, "--format", "json")
	planCmd = h.prepareCmd(planCmd)
	planOut, err := planCmd.CombinedOutput()
	if err != nil {
		h.logger.Error("cap_orchestrator_plan_fetch_failed", err, logging.String("output", string(planOut)))
		return errfmt.Errorf("failed to fetch priority plan: %w", err)
	}

	var planObj map[string]any
	if err := json.Unmarshal(planOut, &planObj); err != nil {
		h.logger.Error("cap_orchestrator_plan_parse_failed", err)
		return errfmt.Errorf("failed to parse priority plan: %w", err)
	}

	// Resolve and nest workstreams and backlog items from DB for routing
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
			refs, _ := ws["priority_plan_refs"].(string)
			if ref == planID || refs == planID {
				wsObjects = append(wsObjects, ws)
			}
		}
	}

	if len(wsObjects) > 0 {
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
			if wsRef, _ := bli[objects.FieldKeyWorkstreamRef].(string); wsRef != "" {
				wsToBli[wsRef] = append(wsToBli[wsRef], map[string]any{
					objects.FieldKeyID:   bli[objects.FieldKeyID],
					objects.FieldKeyTags: bli[objects.FieldKeyTags],
				})
			}
		}

		workstreamsList := make([]any, 0, len(wsObjects))
		for _, ws := range wsObjects {
			wsID, _ := ws[objects.FieldKeyID].(string)
			if wsID != "" {
				tasks, ok := wsToBli[wsID]
				if !ok {
					tasks = []any{}
				}
				workstreamsList = append(workstreamsList, map[string]any{
					objects.FieldKeyID: wsID,
					"tasks":            tasks,
				})
			}
		}
		planObj["workstreams"] = workstreamsList
	}

	// Use PipelineRouter to automatically split into workstreams and assign personas
	router := pipeline.NewPipelineRouter()
	if routeResult, routeErr := router.Route(planObj); routeErr == nil && len(routeResult.TaskAssignments) > 0 {
		h.logger.Info("cap_orchestrator_routed_plan", logging.Int("task_count", len(routeResult.TaskAssignments)))

		pool := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "sub_kernel", "provisioning federated team tasks", 5, 5)
		pool.Start(ctx)

		for taskID, persona := range routeResult.TaskAssignments {
			taskID := taskID
			persona := persona
			_ = pool.Submit(ctx, func(workerCtx context.Context) error {
				title := "Task: " + taskID
				desc := instruction
				if bli, ok := bliMap[taskID]; ok {
					if bliTitle, _ := bli[objects.FieldKeyTitle].(string); bliTitle != "" {
						title = bliTitle
					}
					if bliDesc, _ := bli[objects.FieldKeyDescription].(string); bliDesc != "" {
						desc = bliDesc
					}
				}

				// Deduplicate open ATKs for this plan and task to prevent task floods
				if h.hasOpenTaskForPlan(workerCtx, planID, taskID) {
					h.logger.Info("cap_orchestrator_reusing_open_task", logging.String("plan_id", planID), logging.String("task_id", taskID))
					h.wakeAgentAndScheduleHourglass(taskID, persona)
					return nil
				}

				taskObj := map[string]any{
					objects.FieldKeyKind:               objects.KindAgentTask,
					objects.FieldKeyTitle:              title,
					objects.FieldKeyDescription:        desc,
					objects.FieldKeyAssigneePersonaRef: persona,
					"plan_id":                          planID,
					"task_id":                          taskID,
					"persona_id":                       persona,
					objects.FieldKeyInstruction:        instruction,
					"integration_branch":               fmt.Sprintf("integration/%s", strings.ToLower(planID)),
					// agent_task lifecycle origin is proposed (pending/active are invalid).
					objects.FieldKeyStatus: objects.ObjectStatusProposed,
				}
				secCtx := pkgctx.NewSystemSecurityContext()
				if createErr := h.storage.Create(workerCtx, secCtx, taskObj); createErr != nil {
					h.logger.Error("cap_orchestrator_agent_task_creation_failed", createErr, logging.String("task_id", taskID))
					return createErr
				}

				// Wake Agent and Schedule Hourglass
				h.wakeAgentAndScheduleHourglass(taskID, persona)

				return nil
			})
		}
		pool.Stop()
		return nil
	}

	if teamConfigRef, _ := planObj[objects.FieldKeyTeamConfigurationRef].(string); teamConfigRef != "" {
		h.logger.Info("cap_orchestrator_resolving_team", logging.String("team_configuration_ref", teamConfigRef))
		teamCmd := exec.CommandContext(ctx, exe, "object", "show", "team_configuration", teamConfigRef, "--format", "json")
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

	// Fallback: If no team configured, list personas in-process (CLI list was flaky
	// overnight under concurrent plan dispatch — exit status 1).
	// TRACK: ATK-REDACTED — CAP review freshness / dispatch reliability.
	if len(activePersonas) == 0 {
		h.logger.Info("cap_orchestrator_fallback_all_personas")
		listed, listErr := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{
			Kind: objects.KindPersona,
		})
		if listErr != nil {
			h.logger.Warn("cap_orchestrator_persona_storage_list_failed",
				logging.String("error", listErr.Error()))
		} else if listed != nil {
			for _, p := range listed.Objects {
				if pid, _ := p[objects.FieldKeyID].(string); pid != "" {
					activePersonas = append(activePersonas, pid)
				}
			}
		}
		if len(activePersonas) == 0 {
			personaCmd := exec.CommandContext(ctx, exe, "object", "list", "persona", "--format", "json")
			personaCmd = h.prepareCmd(personaCmd)
			personaOut, err := personaCmd.CombinedOutput()
			if err != nil {
				h.logger.Warn("cap_orchestrator_persona_fetch_skipped",
					logging.String("error", err.Error()),
					logging.String("output", truncateOutput(string(personaOut), 300)))
				return nil
			}

			var pResult struct {
				Objects []map[string]any `json:"objects"`
			}
			if err := json.Unmarshal(personaOut, &pResult); err != nil {
				h.logger.Warn("cap_orchestrator_persona_parse_skipped",
					logging.String("error", err.Error()))
				return nil
			}

			for _, p := range pResult.Objects {
				if pid, _ := p[objects.FieldKeyID].(string); pid != "" {
					activePersonas = append(activePersonas, pid)
				}
			}
		}
	}

	numPersonas := len(activePersonas)
	if numPersonas == 0 {
		h.logger.Info("cap_orchestrator_no_personas")
		return nil
	}

	maxConcurrency := 5
	if numPersonas < maxConcurrency {
		maxConcurrency = numPersonas
	}

	// Provision the Sub-Kernel Pod
	pool := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "sub_kernel", "provisioning federated team", maxConcurrency, maxConcurrency)
	pool.Start(ctx)

	agentLoop := pipeline.NewBuilder("agent_loop", h.logger).
		AddStage("orchestrate", func(pCtx *pipeline.Context, input any) (any, error) {
			req := input.(map[string]any)
			pid := req["pid"].(string)
			secCtx := pkgctx.NewSystemSecurityContext()
			inboxItem := map[string]any{
				objects.FieldKeyKind:        "agent_instruction",
				"persona_id":                pid,
				"plan_id":                   req["planID"].(string),
				objects.FieldKeyInstruction: req[objects.FieldKeyInstruction].(string),
				"integration_branch":        fmt.Sprintf("integration/%s", strings.ToLower(req["planID"].(string))),
				objects.FieldKeyStatus:      objects.ObjectStatusProposed,
			}
			return nil, h.storage.Create(pCtx.Ctx, secCtx, inboxItem)
		}).Build()

	for _, pid := range activePersonas {
		pid := pid // capture loop var
		_ = pool.Submit(ctx, func(workerCtx context.Context) error {
			_, dispatchErr := agentLoop.Run(&pipeline.Context{Ctx: workerCtx}, map[string]any{
				"pid":                       pid,
				"planID":                    planID,
				objects.FieldKeyInstruction: instruction,
			})
			if dispatchErr != nil {
				h.logger.Error("sub_kernel_agent_failed", dispatchErr,
					logging.String("plan_id", planID),
					logging.String("persona", pid))
				return dispatchErr
			}

			h.logger.Info("sub_kernel_agent_completed",
				logging.String("plan_id", planID),
				logging.String("persona", pid))
			return nil
		})
	}

	pool.Stop() // Waits for the team pod to finish the cycle
	return nil
}

// executeReviewStage runs system check to verify object health and compliance,
// then runs tests to verify code integrity.
func (h *CapOrchestratorHandler) executeReviewStage(ctx context.Context, exe string) error {
	h.logger.Info("cap_stage_review_started")

	var errs []string

	// 1. Run system check (object health + compliance)
	// We run it quietly so it doesn't pollute the logs unless there's an error
	checkCmd := exec.CommandContext(ctx, exe, "system", "check", "--format", "json", "--quiet")
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
			logging.String("output", truncateOutput(string(checkOut), 500)))
		errs = append(errs, fmt.Sprintf("system check: %v", checkErr))
	} else if systemCheckOK {
		h.logger.Info("cap_review_system_check_passed")
	}

	// 2. Run scheduler health check
	healthCmd := exec.CommandContext(ctx, exe, "scheduler", "health-check")
	healthCmd = h.prepareCmd(healthCmd)
	healthOut, healthErr := healthCmd.CombinedOutput()
	if healthErr != nil {
		h.logger.Error("cap_review_scheduler_health_failed", healthErr,
			logging.String("output", truncateOutput(string(healthOut), 500)))
		errs = append(errs, fmt.Sprintf("scheduler health: %v", healthErr))
	} else {
		h.logger.Info("cap_review_scheduler_health_passed")
	}

	// 3. Verify latest test bundle outcomes for critical packages in the background stream (health.jsonl)
	secCtx := pkgctx.GetSecurityContext(ctx)
	testsPassed, testErrs := h.verifyCriticalPackagesHealth(ctx, secCtx)
	if !testsPassed {
		h.logger.Error("cap_review_tests_failed", fmt.Errorf("background test bundle failures detected"),
			logging.String("failing_notes", strings.Join(testErrs, "; ")))
		errs = append(errs, testErrs...)
	} else {
		h.logger.Info("cap_review_tests_passed")
	}

	// Write review results to state file for metrics stage / gate freshness checks.
	now := time.Now().UTC()
	pending, _ := h.readPendingStage()
	cvsID := pending.CvsID
	if cvsID == "" {
		cvsID, _ = h.resolveBoundCVS(ctx, pending.PlanID)
	}
	reviewResult := map[string]any{
		"timestamp":                    now.Format(time.RFC3339),
		"system_check":                 systemCheckOK,
		"scheduler_health":             healthErr == nil,
		"tests_passed":                 testsPassed,
		"errors":                       errs,
		"public_blockers":              publicBlockers,
		"cvs_id":                       cvsID,
		"focus_child_cvs_id":           pending.FocusChildCvsID,
		"ready_for_session_completion": false,
		"ready_for_parent_completion":  false,
		objects.FieldKeySource:         "cap_orchestrator.executeReviewStage",
	}
	if checkErr != nil {
		reviewResult["system_check_detail"] = truncateOutput(string(checkOut), 400)
	}
	if healthErr != nil {
		reviewResult["scheduler_health_detail"] = truncateOutput(string(healthOut), 400)
	}
	if !testsPassed {
		reviewResult["tests_detail"] = testErrs
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
	countCmd := exec.CommandContext(ctx, exe, "object", "count", "--format", "json")
	countCmd = h.prepareCmd(countCmd)
	countOut, countErr := countCmd.CombinedOutput()
	if countErr != nil {
		h.logger.Error("cap_metrics_count_failed", countErr)
	}

	// 2. Get convergence status
	convCmd := exec.CommandContext(ctx, exe, "workflow", "convergence", "--format", "json")
	convCmd = h.prepareCmd(convCmd)
	convOut, convErr := convCmd.CombinedOutput()
	if convErr != nil {
		h.logger.Error("cap_metrics_convergence_failed", convErr)
	}

	// 3. Read last review result
	reviewResult, _ := h.readStateFile(capReviewResultFile)

	// 4. Write metrics snapshot
	pending, _ := h.readPendingStage()
	cvsID := pending.CvsID
	if cvsID == "" {
		cvsID, _ = h.resolveBoundCVS(ctx, pending.PlanID)
	}
	metrics := map[string]any{
		"timestamp":                 time.Now().UTC().Format(time.RFC3339),
		"review":                    reviewResult,
		"cvs_id":                    cvsID,
		"focus_child_cvs_id":        pending.FocusChildCvsID,
		objects.FieldKeyObjectCount: jsonOrRaw(countOut),
		"convergence":               jsonOrRaw(convOut),
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
	reportCmd := exec.CommandContext(ctx, exe, "system", "improvement-report", "--format", "json")
	reportCmd = h.prepareCmd(reportCmd)
	reportOut, reportErr := reportCmd.CombinedOutput()
	if reportErr != nil {
		h.logger.Error("cap_self_improvement_report_failed", reportErr, logging.String("output", string(reportOut)))
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
		h.logger.Info("cap_self_improvement_clean", logging.String("status", "no issues found"))
		return nil
	}

	h.logger.Info("cap_self_improvement_issues_found",
		logging.Int("count", len(issues)),
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
		if escalateErr := h.escalation.Evaluate(context.Background(), tracker.ConsecutiveFailures, notice); escalateErr != nil {
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
func (h *CapOrchestratorHandler) executeGroomingStage(ctx context.Context, exe string, planID string) error {
	h.logger.Info("cap_stage_grooming_started", logging.String("plan_id", planID))

	// Stay ahead of the swarm: still wake TPM even when whats-next has no plan
	// (starvation case). Stage advance remains gated on delivery evidence.
	ambientCtx := "GROOMING/STRATEGY (TPM — stay ahead of execution): Read .zqk/state/cap_self_improvement_result.json and recent improvement reports. Deliver concrete graph artifacts before CAP can leave grooming: create/update `priority_plan` objects and/or groom `backlog_item` rows into planned/actionable states with team_configuration assignments. Creating chat noise without plan/BLI mutations does not complete this stage. Optionally write .zqk/state/cap_stage_receipt.json with stage=cap_stage_grooming and artifact_ids."

	if h.hasOpenTPMGroomingInstruction(ctx, planID) {
		h.logger.Info("cap_stage_grooming_reusing_open_instruction", logging.String("plan_id", planID))
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

	h.logger.Info("cap_stage_grooming_dispatched", logging.String("plan_id", planID))
	return nil
}

func (h *CapOrchestratorHandler) hasOpenTPMGroomingInstruction(ctx context.Context, planID string) bool {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	res, err := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: objects.KindAgentInstruction})
	if err != nil || res == nil {
		return false
	}
	for _, obj := range res.Objects {
		st, _ := obj[objects.FieldKeyStatus].(string)
		ls := strings.ToLower(st)
		if ls != objects.ObjectStatusProposed && ls != objects.ObjectStatusInProgress {
			continue
		}
		persona, _ := obj["persona_id"].(string)
		if !strings.EqualFold(persona, "tpm") && !strings.Contains(strings.ToLower(persona), "tpm") {
			continue
		}
		if planID != "" {
			if pid, _ := obj["plan_id"].(string); pid != "" && pid != planID {
				continue
			}
		}
		instr, _ := obj[objects.FieldKeyInstruction].(string)
		if strings.Contains(strings.ToUpper(instr), "GROOMING") {
			return true
		}
	}
	return false
}

func (h *CapOrchestratorHandler) hasOpenTaskForPlan(ctx context.Context, planID, taskID string) bool {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	res, err := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{Kind: objects.KindAgentTask})
	if err != nil || res == nil {
		return false
	}
	for _, obj := range res.Objects {
		st, _ := obj[objects.FieldKeyStatus].(string)
		ls := strings.ToLower(st)
		if ls != objects.ObjectStatusProposed && ls != objects.ObjectStatusInProgress && ls != objects.ObjectStatusApproved {
			continue
		}
		pid, _ := obj["plan_id"].(string)
		tid, _ := obj["task_id"].(string)
		if (planID == "" || pid == planID) && tid == taskID {
			return true
		}
	}
	return false
}

// truncateOutput returns at most maxLen bytes of the output string.
func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}

// jsonOrRaw attempts to parse bytes as JSON; returns raw string on failure.
func jsonOrRaw(b []byte) any {
	var v any
	if err := json.Unmarshal(b, &v); err == nil {
		return v
	}
	return string(b)
}

// capFailureTracker tracks consecutive CAP cycle failures for escalation decisions.
type capFailureTracker struct {
	ConsecutiveFailures int    `json:"consecutive_failures"`
	LastFailure         string `json:"last_failure"`
	LastStage           string `json:"last_stage"`
	LastError           string `json:"last_error"`
	AgentEscalations    int    `json:"agent_escalations"`
	LastAgentEscalation string `json:"last_agent_escalation,omitempty"`
	LastHumanEscalation string `json:"last_human_escalation,omitempty"`
}

// recordFailure increments the consecutive failure counter and routes to the
// appropriate escalation tier via the provider chain.
func (h *CapOrchestratorHandler) recordFailure(stage string, err error) {
	tracker := h.readFailureTracker()
	tracker.ConsecutiveFailures++
	tracker.LastFailure = time.Now().UTC().Format(time.RFC3339)
	tracker.LastStage = stage
	tracker.LastError = truncateOutput(err.Error(), 500)
	h.writeStateFile(capFailureTrackerFile, tracker)

	h.logger.Info("cap_failure_recorded",
		logging.Int("consecutive", tracker.ConsecutiveFailures),
		logging.String("stage", stage))

	notice := EscalationNotice{
		Title:    fmt.Sprintf("CAP Loop Failure — %s", stage),
		Severity: EscalationSeverityWarning,
		Issues:   []string{fmt.Sprintf("%s failed: %s", stage, tracker.LastError)},
		Context: map[string]any{
			"consecutive_failures": tracker.ConsecutiveFailures,
			"last_stage":           tracker.LastStage,
			"last_error":           tracker.LastError,
		},
		SuggestedActions: []string{
			"zqk scheduler status",
			"cat .zqk/state/cap_failure_tracker.json",
			"cat .zqk/state/cap_review_result.json",
			"go test ./pkg/scheduler/...",
		},
	}

	if tracker.ConsecutiveFailures >= capHumanEscalationThreshold {
		notice.Severity = EscalationSeverityCritical
		notice.Title = "CAP Loop Stuck — Agent Recovery Failed"
	}

	if escalateErr := h.escalation.Evaluate(context.Background(), tracker.ConsecutiveFailures, notice); escalateErr != nil {
		h.logger.Error("cap_escalation_failed", escalateErr)
	}
}

// clearFailures resets the consecutive failure counter on success.
func (h *CapOrchestratorHandler) clearFailures() {
	tracker := h.readFailureTracker()
	if tracker.ConsecutiveFailures > 0 {
		h.logger.Info("cap_failures_cleared",
			logging.Int("was", tracker.ConsecutiveFailures))
		tracker.ConsecutiveFailures = 0
		tracker.AgentEscalations = 0
		h.writeStateFile(capFailureTrackerFile, tracker)
	}
}

// readFailureTracker loads the failure tracker from state.
func (h *CapOrchestratorHandler) readFailureTracker() capFailureTracker {
	var tracker capFailureTracker
	data, err := h.readStateFile(capFailureTrackerFile)
	if err != nil {
		return tracker
	}
	b, _ := json.Marshal(data)
	json.Unmarshal(b, &tracker) //nolint:errcheck
	return tracker
}

func (h *CapOrchestratorHandler) resolveCLIExecutable() (string, error) {
	binaryPath := resolveSchedulerCLIBinary(h.projectRoot)
	if binaryPath != "zqk" {
		if _, err := os.Stat(binaryPath); err == nil {
			return binaryPath, nil
		}
	}
	return exec.LookPath(binaryPath)
}

func (h *CapOrchestratorHandler) verifyCriticalPackagesHealth(ctx context.Context, secCtx *pkgctx.SecurityContext) (bool, []string) {
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	var errs []string
	storageCtx := pkgctx.NewStorageContext()

	// 1. List all scheduler jobs to find the ones for pkg/scheduler and pkg/storage
	allJobs, err := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{
		Kind: objects.KindSchedulerJob,
	})
	if err != nil {
		return false, []string{fmt.Sprintf("failed to list scheduler jobs: %v", err)}
	}

	type jobInfo struct {
		id        string
		createdAt string
	}
	latestJobByPkg := make(map[string]jobInfo)
	if allJobs != nil {
		for _, jobMap := range allJobs.Objects {
			jobID, _ := jobMap[objects.FieldKeyID].(string)
			jobType, _ := jobMap[objects.FieldKeyJobType].(string)
			if jobType != JobTypeRunWrapper {
				continue
			}
			metadata, _ := jobMap[objects.FieldKeyMetadata].(map[string]any)
			if metadata == nil {
				continue
			}
			pkgPath, _ := metadata[KeyTestBundleMetaPackagePath].(string)
			if pkgPath == "" {
				continue
			}
			// Match pkg/scheduler and pkg/storage (including subdirectories)
			if pkgPath == "pkg/scheduler" || strings.HasPrefix(pkgPath, "pkg/scheduler/") ||
				pkgPath == "pkg/storage" || strings.HasPrefix(pkgPath, "pkg/storage/") {
				createdAt, _ := jobMap[objects.FieldKeyCreatedAt].(string)
				current, exists := latestJobByPkg[pkgPath]
				if !exists || createdAt > current.createdAt || (createdAt == current.createdAt && jobID > current.id) {
					latestJobByPkg[pkgPath] = jobInfo{
						id:        jobID,
						createdAt: createdAt,
					}
				}
			}
		}
	}

	criticalJobIDs := make(map[string]string) // jobID -> packagePath
	for pkgPath, info := range latestJobByPkg {
		criticalJobIDs[info.id] = pkgPath
	}

	if len(criticalJobIDs) == 0 {
		// CRIT-CAPH-007: fail closed — missing coverage is not a pass.
		h.logger.Warn("cap_review_no_critical_test_jobs_found", logging.String("notes", "No test bundle jobs found for pkg/scheduler or pkg/storage"))
		return false, []string{"no critical test bundle jobs found for pkg/scheduler or pkg/storage"}
	}

	// 2. Read latest health lines from health.jsonl
	// We read the last 2000 lines to ensure we capture recent outcomes for all packages
	lines, err := ReadTestBundleHealthTailLines(ctx, h.projectRoot, 2000)
	if err != nil {
		if os.IsNotExist(err) {
			h.logger.Info("cap_review_health_log_missing", logging.String("notes", "No test bundle health log file found yet"))
			lines = nil
		} else {
			return false, []string{fmt.Sprintf("failed to read test bundle health: %v", err)}
		}
	}

	// 3. Find the latest outcome for each critical job ID
	latestOutcomeByJob := make(map[string]string)
	for _, line := range lines {
		jobID, _ := line[KeyJobID].(string)
		if _, ok := criticalJobIDs[jobID]; !ok {
			continue
		}
		outcome, _ := line[KeyTestOutcome].(string)
		if outcome != "" {
			// CRIT-CAPH-009: reject forgeable orphan/ghost health bridges.
			src, _ := line[objects.FieldKeySource].(string)
			notes, _ := line["notes"].(string)
			low := strings.ToLower(src + " " + notes)
			if strings.Contains(low, "ghost") || strings.Contains(low, "health_bridge") || strings.Contains(low, "manual_bridge") {
				continue
			}
			latestOutcomeByJob[jobID] = outcome
		}
	}

	// 4. Verify outcomes
	allPassed := true
	for jobID, pkgPath := range criticalJobIDs {
		outcome, ok := latestOutcomeByJob[jobID]
		if !ok {
			allPassed = false
			errs = append(errs, fmt.Sprintf("no test outcome recorded for package %s (job %s)", pkgPath, jobID))
			continue
		}
		if outcome != "pass" {
			allPassed = false
			errs = append(errs, fmt.Sprintf("package %s (job %s) latest outcome is %s", pkgPath, jobID, outcome))
		}
	}

	return allPassed, errs
}

// parseSystemCheckPublicBlockers extracts public blocker count from system check JSON (CRIT-CAPH-008).
func parseSystemCheckPublicBlockers(out []byte) (int, error) {
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		return 0, err
	}
	// Common shapes: blocking_issues.public, summary.public_blockers, blockers.public
	for _, key := range []string{"public_blockers", "public_blocking", "blocking_public"} {
		if n, ok := asInt(m[key]); ok {
			return n, nil
		}
	}
	if sum, ok := m["summary"].(map[string]any); ok {
		for _, key := range []string{"public_blockers", "blocking_public", "public"} {
			if n, ok := asInt(sum[key]); ok {
				return n, nil
			}
		}
	}
	if bi, ok := m["blocking_issues"].(map[string]any); ok {
		if n, ok := asInt(bi["public"]); ok {
			return n, nil
		}
	}
	if stats, ok := m["statistics"].(map[string]any); ok {
		if n, ok := asInt(stats["public_blockers"]); ok {
			return n, nil
		}
		if n, ok := asInt(stats["blocking_public"]); ok {
			return n, nil
		}
	}
	// Detailed Statistics narrative sometimes nests under "detailed"
	if det, ok := m["detailed"].(map[string]any); ok {
		if n, ok := asInt(det["blocking_issues_public"]); ok {
			return n, nil
		}
	}
	return 0, nil
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

// autoRecoverPlanTasks automatically finds errored agent_task objects in the plan
// and resets them to approved (clearing failed steps to pending_verification)
// so the orchestrator can re-dispatch/resume them.
func (h *CapOrchestratorHandler) autoRecoverPlanTasks(ctx context.Context, planID string) error {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	filter := storagepkg.ListFilter{
		Kind: objects.KindAgentTask,
		Filters: map[string]any{
			objects.FieldKeyPipelineRef: planID,
			objects.FieldKeyStatus:      objects.ObjectStatusError,
		},
	}

	res, err := h.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return err
	}

	for _, obj := range res.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		title, _ := obj[objects.FieldKeyTitle].(string)

		h.logger.Info("cap_orchestrator_auto_recovering_task",
			logging.String("task_id", id),
			logging.String("title", title),
			logging.String("plan_id", planID),
		)

		// Reset step status to pending_verification so it can be retried
		if stepsAny, ok := obj[objects.FieldKeyTaskSteps]; ok {
			if steps, ok := stepsAny.([]any); ok {
				for _, stepAny := range steps {
					if stepMap, ok := stepAny.(map[string]any); ok {
						status, _ := stepMap[objects.FieldKeyStatus].(string)
						if status == "error" || status == "rejected" {
							stepMap[objects.FieldKeyStatus] = "pending_verification"
							stepMap[objects.FieldKeyVerificationFeedback] = ""
						}
					}
				}
			}
		}

		// Transition status
		obj[objects.FieldKeyStatus] = objects.ObjectStatusApproved

		err = h.storage.Update(ctx, secCtx, id, obj)
		if err != nil {
			h.logger.Error("cap_orchestrator_auto_recovery_update_failed", err, logging.String("task_id", id))
			return err
		}
	}
	return nil
}
