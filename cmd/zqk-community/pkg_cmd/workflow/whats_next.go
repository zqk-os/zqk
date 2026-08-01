package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/agentprompt"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	observerpkg "github.com/lanceman/zqk/pkg/observer"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/workflow/whatsnext"
	"github.com/spf13/cobra"
)

// Default CLI alpha readiness plan (repo lane); still resolved when status is paused.
const defaultAlphaPriorityPlanID = ""

const whatsNextSchema = "zqk_whats_next_v1"

const measureSubprocessTimeout = 120 * time.Second

// whatsNextOut is the zqk_whats_next_v1 composite document.
type whatsNextOut struct {
	Schema                    string                  `json:"schema"`
	PriorityPlan              *whatsNextPriorityPlan  `json:"priority_plan"`
	ActivePlans               []whatsNextPriorityPlan `json:"active_plans,omitempty"`
	BacklogCountsByStatus     map[string]int          `json:"backlog_counts_by_status"`
	ConvergenceSessionsActive []whatsNextCVSRow       `json:"convergence_sessions_active"`
	MeasureSessionID          *string                 `json:"measure_session_id"`
	MeasureCompressed         map[string]any          `json:"measure_compressed,omitempty"`
	MeasureSkipReason         string                  `json:"measure_skip_reason,omitempty"`
	MeasureError              string                  `json:"measure_error,omitempty"`
	AgentInstruction          string                  `json:"agent_instruction,omitempty"`
	ObserverTips              []string                `json:"observer_tips,omitempty"`
}

type whatsNextPriorityPlan struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
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
	cmd.Flags().String("persona-id", "", "Optional persona ID to evaluate what's next for a specific persona")
	cli.BindAsyncProgress(cmd, runWhatsNext)
	cli.RequireStorage(cmd, true)
	return cmd
}

// Cap stage selection is owned by pkg/workflow/whatsnext (peek + starve→grooming).

func runWhatsNext(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err
		ctx := proc.OperationContext()
		sp := proc.Storage()
		projectRoot := proc.ProjectRoot()
		if projectRoot == emptyValue {
			projectRoot = cli.ResolveProjectRoot(".")
		}

		priFlag, _ := cmd.Flags().GetString("priority-plan")  //nolint:errcheck
		cvsFlag, _ := cmd.Flags().GetString("session-id")     //nolint:errcheck
		skipMeasure, _ := cmd.Flags().GetBool("skip-measure") //nolint:errcheck
		personaFlag, _ := cmd.Flags().GetString("persona-id") //nolint:errcheck

		out := whatsNextOut{Schema: whatsNextSchema, BacklogCountsByStatus: map[string]int{}}

		personaIDs := getAgentPersonaIDs(ctx, sp, personaFlag)

		planID, planSumm, activePlans := resolvePriorityPlanForWhatsNext(ctx, sp, strings.TrimSpace(priFlag), personaIDs)
		out.PriorityPlan = planSumm
		out.ActivePlans = activePlans

		// Matrix-Gated Autonomy: Removed arbitrary processVerifyingItems loop.
		// Lifecycle transitions must be governed natively by a verification_matrix.

		if planID != emptyValue {
			out.BacklogCountsByStatus = countBacklogByStatus(ctx, sp, planID, personaIDs)

			totalWork := out.BacklogCountsByStatus["in_progress"] + out.BacklogCountsByStatus["verifying"] + out.BacklogCountsByStatus["planned"] + out.BacklogCountsByStatus["blocked"] + out.BacklogCountsByStatus["validated"]
			if totalWork == 0 && out.BacklogCountsByStatus["completed"] > 0 {
				out.AgentInstruction = "shutdown"
			} else if out.BacklogCountsByStatus["in_progress"] > 0 {
				out.AgentInstruction = "continue"
			} else if out.BacklogCountsByStatus["verifying"] > 0 {
				out.AgentInstruction = "execute_tests"
			} else if totalWork == 0 {
				out.AgentInstruction = "shutdown"
			}
		}

		secCtx := pkgctx.GetSecurityContext(ctx)
		rows := listActiveOrPausedConvergenceSessions(ctx, sp)
		out.ConvergenceSessionsActive = rows

		var taskPrompt string
		var activeTask map[string]any
		if secCtx != nil && (len(personaIDs) > 0 || secCtx.AccountID != "") {
			listRes, err := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{
				Kind: objects.KindAgentTask,
			})
			if err == nil {
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
					if matches {
						activeTask = obj
						break
					}
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

			taskOpts := agentprompt.TaskPromptOptions{
				PlanTitle:     planTitle,
				PlanID:        planRef,
				SessionID:     sessionIDStr,
				TargetAgent:   targetAgent,
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

		if taskPrompt != "" {
			out.AgentInstruction = taskPrompt
		} else if secCtx != nil && secCtx.AccountID == "account:system" {
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

		switch cli.GetFormat(cmd) {
		case cli.FormatJSON, cli.FormatJSONL:
			return cli.FormatOutput(cmd, out)
		case cli.FormatYAML:
			return cli.FormatOutput(cmd, out)
		default:
			return writeWhatsNextTable(cmd, out)
		}
	})(cmd, args)
}

func getObserverTips() []string {
	return observerpkg.ReadCachedTips(zqkenv.ProjectRoot())
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
		res, lerr := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
			Kind:    objects.KindPriorityPlan,
			Filters: map[string]any{objects.FieldKeyID: explicit},
		})
		if lerr != nil || len(res.Objects) == 0 {
			res, lerr = sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
				Kind:    objects.KindStrategicPlan,
				Filters: map[string]any{objects.FieldKeyID: explicit},
			})
		}
		if lerr == nil && len(res.Objects) > 0 {
			if hasPersonaMatch(res.Objects[0], personaIDs) {
				pID, pSumm := summarizePriorityPlan(res.Objects[0])
				return pID, pSumm, nil
			}
		}
	}

	// 2. Check the hardcoded default alpha plan.
	if obj, err := sp.Read(ctx, secCtx, defaultAlphaPriorityPlanID); err == nil && obj != nil && hasPersonaMatch(obj, personaIDs) {
		if st, _ := obj[objects.FieldKeyStatus].(string); !strings.EqualFold(st, "complete") && !strings.EqualFold(st, "archived") {
			// Only use if it has work linked to it
			if planHasWork(ctx, sp, defaultAlphaPriorityPlanID) {
				pID, pSumm := summarizePriorityPlan(obj)
				return pID, pSumm, nil
			}
		}
	}

	// 3. Collect all candidate plans (in_progress, paused, active).
	var candidates []map[string]any
	for _, st := range []string{statusInProcess, "paused", "active"} {
		for _, kind := range []string{objects.KindPriorityPlan, objects.KindStrategicPlan} {
			res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
				Kind:    kind,
				Filters: map[string]any{objects.FieldKeyStatus: st},
			})
			if err == nil {
				for _, obj := range res.Objects {
					if hasPersonaMatch(obj, personaIDs) {
						candidates = append(candidates, obj)
					}
				}
			}
		}
	}

	// Also try a broad list in case status filtering missed some
	if len(candidates) == 0 {
		for _, kind := range []string{objects.KindPriorityPlan, objects.KindStrategicPlan} {
			res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: kind})
			if err == nil {
				for _, obj := range res.Objects {
					st, _ := obj[objects.FieldKeyStatus].(string)
					if (strings.EqualFold(st, statusInProcess) || strings.EqualFold(st, "paused") || strings.EqualFold(st, "active")) && hasPersonaMatch(obj, personaIDs) {
						candidates = append(candidates, obj)
					}
				}
			}
		}
	}

	if len(candidates) == 0 {
		return "", nil, nil
	}

	// 4. Score candidates: prefer plans with linked BLIs.
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

		score := countLinkedBLIs(ctx, sp, id)
		if score > bestScore {
			bestScore = score
			bestPlan = obj
		}
	}

	// 5. If no plan has work, fall back to first candidate.
	if bestPlan == nil || bestScore == 0 {
		// No plan has work; use first candidate as fallback
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

	// Try filtered query first
	res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyPriorityPlanRef: planID,
		},
	})
	if err == nil && len(res.Objects) > 0 {
		return len(res.Objects)
	}

	// Fallback: load all BLIs and filter client-side
	allRes, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindBacklogItem,
	})
	if err != nil {
		return 0
	}
	count := 0
	for _, o := range allRes.Objects {
		ref, _ := o[objects.FieldKeyPriorityPlanRef].(string)
		if ref == planID {
			count++
		}
	}
	return count
}

func summarizePriorityPlan(obj map[string]any) (string, *whatsNextPriorityPlan) {
	id, _ := obj[objects.FieldKeyID].(string)
	if id == emptyValue {
		return "", nil
	}
	title, _ := obj[objects.FieldKeyTitle].(string)
	st, _ := obj[objects.FieldKeyStatus].(string)
	return id, &whatsNextPriorityPlan{ID: id, Title: title, Status: st}
}

func countBacklogByStatus(ctx context.Context, sp workflowStorage, planID string, personaIDs []string) map[string]int {
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	storageCtx := pkgctx.NewStorageContext()

	// Try filtered query first
	res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyPriorityPlanRef: planID,
		},
	})

	out := map[string]int{}

	// If filter returned nothing, fall back to full list + client-side match
	if err != nil || len(res.Objects) == 0 {
		allRes, allErr := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
			Kind: objects.KindBacklogItem,
		})
		if allErr != nil {
			return out
		}
		// Filter client-side
		var filtered []map[string]any
		for _, o := range allRes.Objects {
			ref, _ := o[objects.FieldKeyPriorityPlanRef].(string)
			if ref == planID {
				filtered = append(filtered, o)
			}
		}
		res = &storage.QueryResult{Objects: filtered}
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

func getAgentPersonaIDs(ctx context.Context, sp workflowStorage, explicitPersonaID string) []string {
	if explicitPersonaID != "" {
		return []string{explicitPersonaID}
	}

	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil || len(secCtx.Roles) == 0 {
		return nil
	}

	sysSecCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	res, err := sp.List(ctx, sysSecCtx, storageCtx, storage.ListFilter{Kind: objects.KindPersona})
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
				if id, _ := obj[objects.FieldKeyID].(string); id != "" {
					personaIDs = append(personaIDs, id)
				}
				break
			}
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
		return false // strictly filter
	}
	if refs, ok := refsAny.([]any); ok {
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
	res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindConvergenceSession})
	if err != nil {
		return nil
	}
	var rows []whatsNextCVSRow
	for _, obj := range res.Objects {
		st, _ := obj[objects.FieldKeyStatus].(string)
		ls := strings.ToLower(strings.TrimSpace(st))
		if ls != "active" && ls != "paused" {
			continue
		}
		id, _ := obj[objects.FieldKeyID].(string)
		if id == emptyValue {
			continue
		}
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
	ctx, cancel := context.WithTimeout(context.Background(), measureSubprocessTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, exe, "scheduler", "convergence", "measure", "--format", "json", "--session-id", sessionID)
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
	if p, err := os.Executable(); err == nil && strings.TrimSpace(p) != "" {
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
	return cli.WriteOutput(cmd, []byte(b.String()))
}
