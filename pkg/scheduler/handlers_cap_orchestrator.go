package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/primaryorch"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
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

	capUnassignedWorkstreamID = "workstream:unassigned"

	// agent_task fields that bind a task to its plan and backlog item. Not FieldKeys:
	// they are agent_task spec fields, not shared ontology wire keys.
	capFieldPlanID = "plan_id"
	capFieldTaskID = "task_id"
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
	if err := fileutil.MkdirAll(filepath.Dir(eventPath), paths.DirPerm755); err == nil {
		if f, err := fileutil.OpenFile(eventPath, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644); err == nil {
			timestamp := time.Now().UTC()
			event := map[string]any{
				"timestamp": timestamp.Format(time.RFC3339),
				"sender":    "cap_orchestrator",
				"message":   fmt.Sprintf("Wake up! Task %s has been assigned to %s", taskID, persona),
			}
			if b, err := json.Marshal(event); err == nil {
				_, _ = f.Write(append(b, '\n'))
			}
			_ = f.Close()
		}
	}

	// If persona is TPM, wake the host/project primary orchestrator via pluggable
	// adapters (agent_chat / script / inbox). Do not hardcode wake-agy — that is
	// one vendor adapter selected only when primary_orchestrator.json says so.
	if strings.EqualFold(persona, "tpm") || strings.Contains(strings.ToLower(persona), "tpm") {
		planID := h.resolveWakePlanID(context.Background(), taskID) // Background: request-or-shutdown derived
		top := h.topOpenPlanBLIs(context.Background(), planID, 3)   // Background: request-or-shutdown derived
		msg := fmt.Sprintf("CAP Orchestrator wake: Task %s assigned to %s", taskID, persona)
		res, err := primaryorch.WakePrimary(context.Background(), h.projectRoot, primaryorch.WakeRequest{ // Background: request-or-shutdown derived
			TaskID:  taskID,
			Persona: persona,
			PlanID:  planID,
			TopBLIs: top,
			Message: msg,
		})
		if err != nil {
			h.logger.Error("cap_orchestrator_primary_wake_failed", err,
				logging.TaskIDField(taskID),
				logging.String("persona", persona),
			)
		} else {
			h.logger.Info("cap_orchestrator_primary_wake",
				logging.TaskIDField(taskID),
				logging.PlanIDField(planID),
				logging.AgentIDField(res.AgentID),
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
	_ = fileutil.MkdirAll(hourglassDir, paths.DirPerm755)

	filePath := filepath.Join(hourglassDir, taskID+".json")
	info := map[string]any{
		"task_id":                 taskID,
		objects.FieldKeyKind:      objects.KindAgentTask,
		objects.FieldKeyType:      "deadline",
		objects.FieldKeyExpiresAt: deadlineStr,
	}
	if data, err := json.Marshal(info); err == nil {
		_ = fileutil.WriteFile(filePath, data, paths.FilePerm644)
	}
}

// resolveWakePlanID returns taskID when it is a PRI-*, else the first active priority_plan.
func (h *CapOrchestratorHandler) resolveWakePlanID(ctx context.Context, taskID string) string {
	if strings.HasPrefix(strings.TrimSpace(taskID), "PRI-") {
		return strings.TrimSpace(taskID)
	}
	if h.storage == nil {
		return ""
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	res, err := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{
		Kind: objects.KindPriorityPlan,
		Filters: map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
		Limit:  1,
		Fields: []string{objects.FieldKeyID, objects.FieldKeyStatus},
	})
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
	res, err := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyPriorityPlanRef: planID,
			objects.FieldKeyStatus: map[string]any{
				"$nin": []string{objects.ObjectStatusComplete, "completed", objects.ObjectStatusArchived, "cancelled", "canceled"},
			},
		},
		Fields: []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyDescription, objects.FieldKeyPriorityTier, objects.FieldKeyPriorityPlanRef, objects.FieldKeyStatus},
	})
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
// Tier 2 (6 failures): Human notifications (Slack, macOS banner, bounded inbox)
func defaultEscalationChain(projectRoot string) *EscalationChain {
	agentProvider := buildAgentProvider(projectRoot)
	humanProvider := buildHumanProvider(projectRoot)

	return &EscalationChain{
		Tiers: []EscalationTier{
			{Threshold: capAgentEscalationThreshold, Provider: agentProvider, Name: "agent"},
			{Threshold: capHumanEscalationThreshold, Provider: humanProvider, Name: "human"},
		},
	}
}

// buildHumanProvider constructs the human escalation provider.
// Supports Slack webhook, native macOS desktop notification, and a bounded inbox.
func buildHumanProvider(projectRoot string) EscalationProvider {
	var providers []EscalationProvider

	// 1. Resolve Slack webhook from env, config, or identity
	slackURL := ResolveSlackWebhookURL(projectRoot)
	if slackURL != "" {
		providers = append(providers, &WebhookEscalationProvider{
			WebhookURL: slackURL,
		})
	}

	// Custom human command from escalation.json if configured
	configPath := paths.AgentRuntimeFile(projectRoot, "escalation.json")
	if data, err := fileutil.ReadFile(configPath); err == nil {
		var config struct {
			HumanCommand string   `json:"human_command"`
			HumanArgs    []string `json:"human_args"`
		}
		if json.Unmarshal(data, &config) == nil && config.HumanCommand != "" {
			providers = append(providers, &CommandEscalationProvider{
				Command:     config.HumanCommand,
				Args:        config.HumanArgs,
				ProjectRoot: projectRoot,
			})
		}
	}

	// 2. On macOS, add native desktop banner notification with sound
	if runtime.GOOS == "darwin" {
		providers = append(providers, &MacOSNotificationProvider{})
	}

	// 3. Maintain bounded inbox directory with auto-pruning to prevent file clutter
	providers = append(providers, &InboxEscalationProvider{
		InboxDir:   filepath.Join(projectRoot, paths.ProjectDataDir, paths.InboxSubdir, "human"),
		MaxHistory: 3,
	})

	if len(providers) == 1 {
		return providers[0]
	}
	return &MultiEscalationProvider{Providers: providers}
}

// buildAgentProvider constructs the agent escalation provider from config.
// Checks .zqk/agent-runtime/escalation.json for a custom command, falls back to
// the coder_agent inbox if no command is configured.
func buildAgentProvider(projectRoot string) EscalationProvider {
	// Check for custom escalation command in project config
	configPath := paths.AgentRuntimeFile(projectRoot, "escalation.json")
	if data, err := fileutil.ReadFile(configPath); err == nil {
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
		InboxDir: filepath.Join(projectRoot, paths.ProjectDataDir, paths.InboxSubdir, "coder_agent"),
	}
}

func (h *CapOrchestratorHandler) prepareCmd(cmd *exec.Cmd) *exec.Cmd {
	cmd.Dir = h.projectRoot
	// Strip then set keys so Unix first-match env semantics cannot keep a parent
	// ZQK_GRAPH_ENABLED=true (Memgraph hangs were killing review under the CAP timeout).
	// graph policy once overnight CAP gate is green without per-child overrides.
	graphKey := zqkenv.RawGraphEnabled()
	adminGraphKey := zqkenv.AdminGraphEnabled()
	apiKey := zqkenv.APIKey().Name()
	env := make([]string, 0, len(os.Environ())+3)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, graphKey.Name()+"=") || strings.HasPrefix(e, adminGraphKey.Name()+"=") ||
			strings.HasPrefix(e, apiKey+"=") ||
			zqkenv.HasDefaultAssignment(e, "GRAPH_ENABLED") || zqkenv.HasDefaultAssignment(e, "ADMIN_GRAPH_ENABLED") {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = append(env,
		apiKey+"="+objects.DefaultSystemAccountID,
		graphKey.Name()+"=false",
		adminGraphKey.Name()+"=false",
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

	// CAP stage machine must not be shadowed by swarm cues from
	// deriveAgentInstructionFromBacklog ("continue" when any BLI is in_progress,
	// "shutdown", "execute_tests"). Those cues are for agents; SCH-cap-orchestrator
	// peeks/advances cap_cycle via SelectCAPInstruction (CAP_LOOP_CONTRACT).
	// keep whats-next agent_instruction vs cap_stage fields distinct.
	capStage := whatsnext.SelectCAPInstruction(h.projectRoot, result.BacklogCountsByStatus)
	if strings.HasPrefix(capStage, "cap_stage_") {
		if instruction != capStage {
			h.logger.Info("cap_orchestrator_stage_override",
				logging.String("whats_next_instruction", instruction),
				logging.String("cap_stage", capStage))
		}
		instruction = capStage
	}

	var planID string
	if result.PriorityPlan != nil {
		planID = result.PriorityPlan.ID
	}
	if err := h.resumePendingVerificationTasks(timeoutCtx, exe, planID); err != nil {
		h.recordFailure("cap_native_continuation", err)
		return err
	}
	activePlans := capDispatchPlanIDs(planID, result.ActivePlans)

	// Step 2: Route to stage-specific handler, tracking failures for escalation
	var stageErr error
	if strings.HasPrefix(instruction, "cap_stage_") {
		h.markStageEntered(instruction, planID)
		if pending, err := h.readPendingStage(); err == nil && h.capStageQuarantineActive(pending) {
			h.logger.Info("cap_stage_quarantine_hold",
				logging.StageField(instruction),
				logging.PlanIDField(planID),
				logging.Int("failure_attempts", pending.FailureAttempts))
			return nil
		}
	}
	switch instruction {
	case "cap_stage_review":
		stageErr = h.executeReviewStage(timeoutCtx, exe)
	case "cap_stage_metrics":
		stageErr = h.executeMetricsStage(timeoutCtx, exe)
	case "cap_stage_self_improvement":
		stageErr = h.executeSelfImprovementStage(timeoutCtx, exe)
	case "cap_stage_grooming":
		planned := 0
		if result.BacklogCountsByStatus != nil {
			planned = result.BacklogCountsByStatus[objects.ObjectStatusPlanned]
		}
		var planTitle, planStatus string
		if result.PriorityPlan != nil {
			planTitle = result.PriorityPlan.Title
			planStatus = result.PriorityPlan.Status
		}
		stageErr = h.executeGroomingStage(timeoutCtx, exe, planID, planned, planTitle, planStatus, result.BacklogCountsByStatus)
	case "cap_stage_sentinel":
		stageErr = h.executeSentinelStage(timeoutCtx, exe)
	case "", "wait", "shutdown":
		h.logger.Info("cap_orchestrator_idle", logging.String("instruction", instruction))
		return nil
	default:
		// cap_stage_planning, cap_stage_design, cap_stage_orchestrating
		// All dispatch work via agent orchestrate
		if len(activePlans) == 0 {
			h.logger.Info("cap_orchestrator_no_active_plans", logging.StageField(instruction))
		} else {
			var errs []string
			shared := &capDispatchShared{
				openAGI: h.buildOpenAgentInstructionIndex(timeoutCtx),
				openATK: h.buildOpenAgentTaskIndex(timeoutCtx),
			}
			planPool := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "plan_dispatcher", "dispatching active plans", len(activePlans), len(activePlans))
			planPool.Start(timeoutCtx)

			for i, pid := range activePlans {
				pid := pid
				isLead := i == 0
				_ = planPool.Submit(timeoutCtx, func(workerCtx context.Context) error {
					// Auto-recover errored tasks for this plan so the orchestrator can dispatch/resume them
					if recErr := h.autoRecoverPlanTasks(workerCtx, pid); recErr != nil {
						h.logger.Error("cap_orchestrator_auto_recover_failed", recErr, logging.PlanIDField(pid))
					}
					if err := h.executeDispatchStage(workerCtx, exe, pid, instruction, shared, isLead); err != nil {
						// Graph/CAS ghosts in ActivePlans should not fail the whole CAP tick when
						// the primary Ambient plan can still dispatch.
						// to file-backed plans only; then remove this soft-skip.
						msg := err.Error()
						if strings.Contains(msg, "failed to fetch priority plan") {
							h.logger.Warn("cap_orchestrator_plan_dispatch_skipped_missing",
								logging.PlanIDField(pid),
								logging.ErrorTextField(msg))
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

	if stageErr != nil {
		if strings.Contains(stageErr.Error(), "planned=0 exhausted") ||
			strings.Contains(stageErr.Error(), "ATTN empty-column") ||
			strings.Contains(stageErr.Error(), "ATTN dor-gap") {
			// Anticipatory grooming hold: empty-column, dor-gap, or planned=0
			// indicates the queue is waiting on TPM shaping or wrapping, not an infrastructure/daemon crash.
			// Wake TPM and hold stage without incrementing fatal failure tracker.
			h.logger.Info("cap_stage_tpm_hold",
				logging.PlanIDField(planID),
				logging.ErrorTextField(stageErr.Error()),
			)
			h.wakeAgentAndScheduleHourglass(planID, "tpm")
			return nil
		}
		h.recordFailure(instruction, stageErr)
		h.recordCAPStageFailureAttempt(instruction)
		if pending, err := h.readPendingStage(); err == nil && capStageAttemptExhausted(pending) {
			quarantineErr := errfmt.Errorf(
				"CAP stage %s quarantined after %d consecutive handler failures",
				instruction,
				pending.FailureAttempts,
			)
			h.quarantineCAPStage(pending)
			if pending.FailureAttempts == capStageMaxAttempts+1 {
				h.wakeAgentAndScheduleHourglass(planID, "tpm")
			}
			return quarantineErr
		}
		return stageErr
	}
	h.clearCAPStageFailureAttempts(instruction)
	if advErr := h.maybeAdvanceCAPStage(timeoutCtx, instruction); advErr != nil {
		// Hold without clearing or escalating — waiting on TPM/worker artifacts.
		return nil
	}
	h.clearFailures()
	return nil
}

// capDispatchShared is tick-scoped state shared across parallel plan dispatchers.
// TRACK: follow-up in kernel backlog
type capDispatchShared struct {
	openAGI *openAgentInstructionIndex
	openATK *openAgentTaskIndex
}

// openAgentInstructionIndex is a one-shot snapshot of open AGIs for CAP fan-out.
// TRACK: follow-up in kernel backlog
type openAgentInstructionIndex struct {
	mu sync.RWMutex
	// key: lower(plan)\0lower(persona) → uppercased instruction texts
	byPlanPersona map[string][]string
}

func openAGIPlanPersonaKey(planID, personaID string) string {
	return strings.ToLower(planID) + "\x00" + strings.ToLower(personaID)
}

func (idx *openAgentInstructionIndex) has(planID, personaID, instructionMatch string) bool {
	if idx == nil || instructionMatch == "" {
		return false
	}
	want := strings.ToUpper(instructionMatch)
	key := openAGIPlanPersonaKey(planID, personaID)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	for _, instr := range idx.byPlanPersona[key] {
		if strings.Contains(instr, want) {
			return true
		}
	}
	// persona substring match (legacy TPM helper: persona contains "tpm")
	wantPersona := strings.ToLower(personaID)
	if wantPersona == "" {
		return false
	}
	for k, instrs := range idx.byPlanPersona {
		parts := strings.SplitN(k, "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		if planID != "" && parts[0] != strings.ToLower(planID) {
			continue
		}
		if parts[1] != wantPersona && !strings.Contains(parts[1], wantPersona) {
			continue
		}
		for _, instr := range instrs {
			if strings.Contains(instr, want) {
				return true
			}
		}
	}
	return false
}

func (idx *openAgentInstructionIndex) remember(planID, personaID, instruction string) {
	if idx == nil {
		return
	}
	key := openAGIPlanPersonaKey(planID, personaID)
	up := strings.ToUpper(instruction)
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.byPlanPersona == nil {
		idx.byPlanPersona = make(map[string][]string)
	}
	idx.byPlanPersona[key] = append(idx.byPlanPersona[key], up)
}

func (h *CapOrchestratorHandler) buildOpenAgentInstructionIndex(ctx context.Context) *openAgentInstructionIndex {
	idx := &openAgentInstructionIndex{byPlanPersona: make(map[string][]string)}
	if h == nil || h.storage == nil {
		return idx
	}
	res, err := h.storage.List(ctx, pkgctx.NewSystemSecurityContext(), pkgctx.NewStorageContext(), storagepkg.ListFilter{
		Kind: objects.KindAgentInstruction,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				"$in": []string{objects.ObjectStatusProposed, objects.ObjectStatusInProgress},
			},
		},
		Fields: []string{objects.FieldKeyID, objects.FieldKeyStatus, "persona_id", "plan_id", objects.FieldKeyInstruction},
	})
	if err != nil || res == nil {
		return idx
	}
	for _, obj := range res.Objects {
		st, _ := obj[objects.FieldKeyStatus].(string)
		ls := strings.ToLower(st)
		if ls != objects.ObjectStatusProposed && ls != objects.ObjectStatusInProgress {
			continue
		}
		persona, _ := obj["persona_id"].(string)
		plan, _ := obj["plan_id"].(string)
		instr, _ := obj[objects.FieldKeyInstruction].(string)
		if persona == "" || instr == "" {
			continue
		}
		key := openAGIPlanPersonaKey(plan, persona)
		idx.byPlanPersona[key] = append(idx.byPlanPersona[key], strings.ToUpper(instr))
	}
	return idx
}

// hasOpenAgentInstruction reports whether an open (proposed/in_progress) agent_instruction
// already exists for the persona+plan with a matching instruction token.
// instructionMatch is matched case-insensitively as a substring so stage keys
// (cap_stage_design) and free-text grooming prompts both reuse.
// TRACK: follow-up in kernel backlog
func (h *CapOrchestratorHandler) hasOpenAgentInstruction(ctx context.Context, planID, personaID, instructionMatch string) bool {
	return h.buildOpenAgentInstructionIndex(ctx).has(planID, personaID, instructionMatch)
}

// openAgentTaskIndex is a one-shot snapshot of open agent_tasks for CAP fan-out.
//
// It spans both storage planes. CAP mints tasks at the preliminary `proposed` origin,
// which lives on the object draft plane, and List omits that plane by design (see
// pkg/storage/object_draft_plane.go). A CAS-only snapshot therefore never sees what
// CAP just created and re-mints every task on every tick.
type openAgentTaskIndex struct {
	mu sync.RWMutex
	// key: lower(plan)\0lower(task)
	planTask map[string]struct{}
}

func openATKPlanTaskKey(planID, taskID string) string {
	return strings.ToLower(planID) + "\x00" + strings.ToLower(taskID)
}

// has reports whether an open task exists. An empty planID matches any plan.
func (idx *openAgentTaskIndex) has(planID, taskID string) bool {
	if idx == nil || taskID == "" {
		return false
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if _, ok := idx.planTask[openATKPlanTaskKey(planID, taskID)]; ok {
		return true
	}
	if planID != "" {
		return false
	}
	suffix := "\x00" + strings.ToLower(taskID)
	for key := range idx.planTask {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}
	return false
}

func (idx *openAgentTaskIndex) remember(planID, taskID string) {
	if idx == nil || taskID == "" {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.planTask == nil {
		idx.planTask = make(map[string]struct{})
	}
	idx.planTask[openATKPlanTaskKey(planID, taskID)] = struct{}{}
}

// rememberIfOpen records obj when it is an agent_task still awaiting completion.
func (idx *openAgentTaskIndex) rememberIfOpen(obj map[string]any) {
	st, _ := obj[objects.FieldKeyStatus].(string)
	switch strings.ToLower(st) {
	case objects.ObjectStatusProposed, objects.ObjectStatusInProgress, objects.ObjectStatusApproved:
	default:
		return
	}
	taskID, _ := obj[capFieldTaskID].(string)
	planID, _ := obj[capFieldPlanID].(string)
	idx.remember(planID, taskID)
}

func (h *CapOrchestratorHandler) buildOpenAgentTaskIndex(ctx context.Context) *openAgentTaskIndex {
	idx := &openAgentTaskIndex{planTask: make(map[string]struct{})}
	if h == nil || h.storage == nil {
		return idx
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	if res, err := h.storage.List(ctx, secCtx, pkgctx.NewStorageContext(), storagepkg.ListFilter{
		Kind: objects.KindAgentTask,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				"$in": []string{objects.ObjectStatusProposed, objects.ObjectStatusInProgress, objects.ObjectStatusApproved},
			},
		},
		Fields: []string{objects.FieldKeyID, objects.FieldKeyStatus, capFieldTaskID, capFieldPlanID},
	}); err == nil && res != nil {
		for _, obj := range res.Objects {
			idx.rememberIfOpen(obj)
		}
	}
	draftIDs, err := storagepkg.ListObjectDraftPlaneIDs(h.projectRoot, objects.KindAgentTask)
	if err != nil {
		h.logger.Warn("cap_orchestrator_draft_plane_list_failed", logging.ErrorTextField(err.Error()))
		return idx
	}
	for _, id := range draftIDs {
		obj, readErr := h.storage.Read(ctx, secCtx, id)
		if readErr != nil || obj == nil {
			continue
		}
		idx.rememberIfOpen(obj)
	}
	return idx
}

func (h *CapOrchestratorHandler) hasOpenTaskForPlan(ctx context.Context, planID, taskID string) bool {
	return h.buildOpenAgentTaskIndex(ctx).has(planID, taskID)
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
		logging.StageField(stage))

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
			paths.CLIUsage("scheduler", "status"),
			"cat .zqk/state/cap_failure_tracker.json",
			"cat .zqk/state/cap_review_result.json",
			"go test ./pkg/scheduler/...",
		},
	}

	if tracker.ConsecutiveFailures >= capHumanEscalationThreshold {
		notice.Severity = EscalationSeverityCritical
		notice.Title = "CAP Loop Stuck — Agent Recovery Failed"

		// Rate-limit human escalation: avoid spamming identical failures on every scheduler tick.
		shouldEscalateHuman := false
		if tracker.LastHumanEscalation == "" {
			shouldEscalateHuman = true
		} else if lastTime, parseErr := time.Parse(time.RFC3339, tracker.LastHumanEscalation); parseErr == nil && time.Since(lastTime) > time.Hour {
			shouldEscalateHuman = true
		} else if tracker.ConsecutiveFailures == 25 || tracker.ConsecutiveFailures == 50 || tracker.ConsecutiveFailures == 100 || tracker.ConsecutiveFailures == 200 {
			shouldEscalateHuman = true
		}

		if shouldEscalateHuman {
			tracker.LastHumanEscalation = time.Now().UTC().Format(time.RFC3339)
			h.writeStateFile(capFailureTrackerFile, tracker)
			if escalateErr := h.escalation.Evaluate(context.Background(), tracker.ConsecutiveFailures, notice); escalateErr != nil {
				h.logger.Error("cap_escalation_failed", escalateErr)
			}
		}
		return
	}

	if escalateErr := h.escalation.Evaluate(context.Background(), tracker.ConsecutiveFailures, notice); escalateErr != nil { // Background: request-or-shutdown derived
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
	_ = json.Unmarshal(b, &tracker) //nolint:errcheck
	return tracker
}

func (h *CapOrchestratorHandler) resolveCLIExecutable() (string, error) {
	binaryPath := resolveSchedulerCLIBinary(h.projectRoot)
	if filepath.IsAbs(binaryPath) || strings.ContainsRune(binaryPath, os.PathSeparator) {
		if _, err := fileutil.Stat(binaryPath); err == nil {
			return binaryPath, nil
		}
	}
	return exec.LookPath(binaryPath)
}

// criticalPackageRoots are the CAP review health gates (CRIT-CAPH-007). Subpackages collapse to these roots.
var criticalPackageRoots = []string{"pkg/scheduler", "pkg/storage"}

func criticalRootForPackagePath(pkgPath string) string {
	pkgPath = strings.TrimPrefix(strings.TrimSpace(pkgPath), "./")
	for _, root := range criticalPackageRoots {
		if pkgPath == root || strings.HasPrefix(pkgPath, root+"/") {
			return root
		}
	}
	return ""
}

// verifyCriticalPackagesHealth checks latest test-bundle health for critical package roots.
// Returns (passed, hardErrs, softErrs). softErrs are coverage gaps (missing jobs / no outcome yet) —
// they keep review readiness red but must not fail the CAP job (notification spam). hardErrs are
// real test failures or infrastructure list errors.
func (h *CapOrchestratorHandler) verifyCriticalPackagesHealth(ctx context.Context, secCtx *pkgctx.SecurityContext) (passed bool, hardErrs, softErrs []string) {
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	storageCtx := pkgctx.NewStorageContext()

	allJobs, err := h.storage.List(ctx, secCtx, storageCtx, storagepkg.ListFilter{
		Kind: objects.KindSchedulerJob,
		Filters: map[string]any{
			objects.FieldKeyJobType: JobTypeRunWrapper,
		},
		Fields: []string{objects.FieldKeyID, objects.FieldKeyJobType, objects.FieldKeyMetadata, objects.FieldKeyLastRunAt},
	})
	if err != nil {
		return false, []string{fmt.Sprintf("failed to list scheduler jobs: %v", err)}, nil
	}

	type jobInfo struct {
		root    string
		lastRun time.Time
	}
	// A package scan creates many bundle jobs. Gate on every recently executed
	// bundle, not one lexically newest created job: old archived jobs may never
	// have run, while reused stable bundle IDs carry the current evidence.
	recentJobByID := make(map[string]jobInfo)
	recentJobCountByRoot := make(map[string]int)
	now := time.Now().UTC()
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
			root := criticalRootForPackagePath(pkgPath)
			if root == "" {
				continue
			}
			lastRunRaw, _ := jobMap[objects.FieldKeyLastRunAt].(string)
			lastRun, parseErr := time.Parse(time.RFC3339, lastRunRaw)
			if jobID == "" || parseErr != nil || now.Sub(lastRun) > capReviewMaxAge {
				continue
			}
			recentJobByID[jobID] = jobInfo{root: root, lastRun: lastRun}
			recentJobCountByRoot[root]++
		}
	}

	missingRoots := make([]string, 0, len(criticalPackageRoots))
	for _, root := range criticalPackageRoots {
		if recentJobCountByRoot[root] == 0 {
			missingRoots = append(missingRoots, root)
		}
	}
	if len(missingRoots) > 0 {
		h.logger.Warn("cap_review_no_critical_test_jobs_found",
			logging.String("missing_roots", strings.Join(missingRoots, ",")))
		for _, root := range missingRoots {
			softErrs = append(softErrs, fmt.Sprintf("no recent test_case evidence for %s (run %s)", root, paths.RewriteCanonicalCLIInvocations("zqk test run")))
		}
	}

	if len(recentJobByID) == 0 {
		// CRIT-CAPH-007: readiness stays fail-closed (passed=false); job success is soft via softErrs.
		return false, nil, softErrs
	}

	lines, err := ReadTestBundleHealthTailLines(ctx, h.projectRoot, 2000)
	if err != nil {
		if fileutil.IsNotExist(err) {
			h.logger.Info("cap_review_health_log_missing", logging.String("notes", "No test bundle health log file found yet"))
			lines = nil
		} else {
			return false, []string{fmt.Sprintf("failed to read test bundle health: %v", err)}, softErrs
		}
	}

	latestOutcomeByJob := make(map[string]string)
	for _, line := range lines {
		jobID, _ := line[KeyJobID].(string)
		job, ok := recentJobByID[jobID]
		if !ok {
			continue
		}
		outcome, _ := line[KeyTestOutcome].(string)
		if outcome == "" {
			continue
		}
		src, _ := line[objects.FieldKeySource].(string)
		notes, _ := line[objects.FieldKeyNotes].(string)
		low := strings.ToLower(src + " " + notes)
		if strings.Contains(low, "ghost") || strings.Contains(low, "health_bridge") || strings.Contains(low, "manual_bridge") {
			continue
		}
		evidenceAtRaw, _ := line[KeyTimestamp].(string)
		evidenceAt, parseErr := time.Parse(time.RFC3339, evidenceAtRaw)
		if parseErr != nil || evidenceAt.Before(job.lastRun) {
			continue
		}
		latestOutcomeByJob[jobID] = outcome
	}

	allPassed := len(missingRoots) == 0
	for jobID, job := range recentJobByID {
		outcome, ok := latestOutcomeByJob[jobID]
		if !ok {
			allPassed = false
			softErrs = append(softErrs, fmt.Sprintf("no current test outcome recorded yet for %s (job %s)", job.root, jobID))
			continue
		}
		if outcome != "pass" {
			allPassed = false
			hardErrs = append(hardErrs, fmt.Sprintf("package %s (job %s) latest outcome is %s", job.root, jobID, outcome))
		}
	}

	return allPassed && len(hardErrs) == 0 && len(softErrs) == 0, hardErrs, softErrs
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
	if sum, ok := m[objects.FieldKeySummary].(map[string]any); ok {
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
			logging.TaskIDField(id),
			logging.String("title", title),
			logging.PlanIDField(planID),
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
			h.logger.Error("cap_orchestrator_auto_recovery_update_failed", err, logging.TaskIDField(id))
			return err
		}
	}
	return nil
}
