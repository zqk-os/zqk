package whatsnext

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

	"github.com/zqk-os/zqk/pkg/agentfeed"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/convergence"
	"github.com/zqk-os/zqk/pkg/objects"
	observerpkg "github.com/zqk-os/zqk/pkg/observer"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/tpm"
)

// Default CLI alpha readiness plan (repo lane); still resolved when status is paused.
const defaultAlphaPriorityPlanID = "[REDACTED-ID]"

const WhatsNextSchema = "zqk_whats_next_v1"

const measureSubprocessTimeout = 120 * time.Second

// whatsNextOut is the zqk_whats_next_v1 composite document.
type WhatsNextOut struct {
	Schema                    string                  `json:"schema"`
	PriorityPlan              *WhatsNextPriorityPlan  `json:"priority_plan"`
	ActivePlans               []WhatsNextPriorityPlan `json:"active_plans,omitempty"`
	BacklogCountsByStatus     map[string]int          `json:"backlog_counts_by_status"`
	ConvergenceSessionsActive []WhatsNextCVSRow       `json:"convergence_sessions_active"`
	MeasureSessionID          *string                 `json:"measure_session_id"`
	MeasureCompressed         map[string]any          `json:"measure_compressed,omitempty"`
	MeasureSkipReason         string                  `json:"measure_skip_reason,omitempty"`
	MeasureError              string                  `json:"measure_error,omitempty"`
	AgentInstruction          string                  `json:"agent_instruction,omitempty"`
	// PackagingCue is a PRI≈PR wrap hint when the selected plan has no open children.
	PackagingCue string   `json:"packaging_cue,omitempty"`
	ObserverTips []string `json:"observer_tips,omitempty"`
	// KernelAmbience is the TPM thought projector (cached system check + live drafts).
	KernelAmbience *KernelAmbience `json:"kernel_ambience,omitempty"`
	// FillItem is one kernel job when the lead has no live ATK. Do not remint orch.
	FillItem *FillItem `json:"fill_item,omitempty"`
	// StrategicReplenishmentCue informs the agent/TPM when the execution runway <= 1.
	StrategicReplenishmentCue string `json:"strategic_replenishment_cue,omitempty"`
	RunwayDepth               int    `json:"runway_depth,omitempty"`
	// MaterializedViewStale indicates whether the fast projection is currently recovering from disruption.
	MaterializedViewStale bool `json:"materialized_view_stale,omitempty"`
	// MaterializedViewRecovering indicates whether an asynchronous background rebuild is in progress.
	MaterializedViewRecovering bool `json:"materialized_view_recovering,omitempty"`
	// MaterializedViewDegradedReason explains the disruption condition (e.g. staleness or cold boot).
	MaterializedViewDegradedReason string `json:"materialized_view_degraded_reason,omitempty"`
}

type WhatsNextPriorityPlan struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type WhatsNextCVSRow struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	CurrentPhase string `json:"current_phase"`
	Status       string `json:"status"`
}

// NewWhatsNextCmd returns workflow whats-next command.

func getObserverTips() []string {
	return observerpkg.ReadCachedTips(zqkenv.ProjectRoot().Get())
}

func resolvePriorityPlanForWhatsNext(ctx context.Context, sp storage.ObjectStorageProvider, explicit string, personaIDs []string) (planID string, summ *WhatsNextPriorityPlan, activePlans []WhatsNextPriorityPlan) {
	secCtx := pkgctx.NewSystemSecurityContext()
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

	// 3. Collect all candidate plans (operational + strategic).
	// Shared with CLI whats-next so grooming next-columns appear in ambient.
	// TRACK: follow-up in kernel backlog
	planStatuses := objects.PlanWhatsNextCandidateStatuses()
	var candidates []map[string]any
	for _, st := range planStatuses {
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
					if planStatusEligible(st) && hasPersonaMatch(obj, personaIDs) {
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

	// 4. Score candidates: prefer shovel-ready execution plans over grooming megas.
	// When any in_progress/active plan has open (non-terminal) BLIs, do not let
	// grooming/prioritizing umbrella plans win solely on historical link count.
	executionReady := false
	for _, obj := range candidates {
		id, _ := obj[objects.FieldKeyID].(string)
		if id == emptyValue {
			continue
		}
		st, _ := obj[objects.FieldKeyStatus].(string)
		if planStatusExecutionReady(st) && countOpenLinkedBLIs(ctx, sp, id, personaIDs) > 0 {
			executionReady = true
			break
		}
	}

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
		if executionReady && !planStatusExecutionReady(st) {
			continue
		}

		openCount := countOpenLinkedBLIs(ctx, sp, id, personaIDs)
		score := openCount
		score += planExecutionStatusBonus(st)
		score += objects.PlanSeatedPersonaBonus(personaIDs, obj)
		// Prefer operational priority_plan over strategic_plan when both have work —
		// strategic plans often accumulate broad links and starve the execution PRI.
		if kind, _ := obj[objects.FieldKeyKind].(string); kind == objects.KindPriorityPlan {
			score += priorityPlanScoreBonus
		}
		// Lower active_order wins (unset ⇒ lowest priority).
		score -= activeOrderPenalty(obj)
		if score > bestScore {
			bestScore = score
			bestPlan = obj
		}
	}

	// 5. If no plan has work, fall back to first candidate.
	if bestPlan == nil || bestScore <= 0 {
		// No plan has work; use first candidate as fallback
		pID, pSumm := summarizePriorityPlan(candidates[0])
		return pID, pSumm, activePlans
	}

	pID, pSumm := summarizePriorityPlan(bestPlan)
	return pID, pSumm, activePlans
}

// planHasWork returns true if the plan has at least one open (non-terminal) linked BLI.
func planHasWork(ctx context.Context, sp storage.ObjectStorageProvider, planID string) bool {
	return countOpenLinkedBLIs(ctx, sp, planID, nil) > 0
}

func planStatusExecutionReady(st string) bool {
	return objects.PlanStatusExecutionFacing(objects.KindPriorityPlan, st)
}

func planExecutionStatusBonus(st string) int {
	return objects.PlanWhatsNextStatusBonus(objects.KindPriorityPlan, st)
}

// activeOrderPenalty returns a score penalty so lower active_order ranks higher.
// Missing/invalid active_order is treated as lowest priority (large penalty) for shovel-ready
// active plans. Exception: execution_locked (in_progress) is top-of-stack (≡ active @ 0).
func activeOrderPenalty(obj map[string]any) int {
	return objects.PlanActiveOrderPenalty(objects.KindPriorityPlan, obj)
}

func bliStatusTerminal(st string) bool {
	// Terminal or halted (error) do not count as open linked work.
	return !objects.BacklogCountsAsOpenWork(st)
}

// preferSeatedPlansWithOpenWork drops hollow matches when the seated persona has
// fuel on another candidate. Unfiltered views keep every candidate.
func preferSeatedPlansWithOpenWork(ctx context.Context, sp storage.ObjectStorageProvider, candidates []map[string]any, personaIDs []string) []map[string]any {
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

// countOpenLinkedBLIs returns non-terminal BLIs linked to a plan (child priority_plan_ref).
// personaIDs filters BLIs (unassigned stays eligible). Empty filter counts all open children.
func countOpenLinkedBLIs(ctx context.Context, sp storage.ObjectStorageProvider, planID string, personaIDs []string) int {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	countOpen := func(objs []map[string]any) int {
		count := 0
		for _, o := range objs {
			ref, _ := o[objects.FieldKeyPriorityPlanRef].(string)
			if ref != planID {
				continue
			}
			st, _ := o[objects.FieldKeyStatus].(string)
			if !hasPersonaMatch(o, personaIDs) {
				continue
			}
			if len(personaIDs) > 0 {
				if !objects.BacklogCountsAsExecutionFuel(st) {
					continue
				}
			} else if bliStatusTerminal(st) {
				continue
			}
			count++
		}
		return count
	}

	// Try filtered query first
	res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyPriorityPlanRef: planID,
		},
	})
	if err == nil && len(res.Objects) > 0 {
		return countOpen(res.Objects)
	}

	// Fallback: load all BLIs and filter client-side
	allRes, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindBacklogItem,
	})
	if err != nil {
		return 0
	}
	return countOpen(allRes.Objects)
}

// countLinkedBLIs returns the number of BLIs linked to a plan (including terminal).
// Retained for callers/tests that need total membership; scoring uses countOpenLinkedBLIs.
func countLinkedBLIs(ctx context.Context, sp storage.ObjectStorageProvider, planID string) int {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	countAll := func(objs []map[string]any) int {
		count := 0
		for _, o := range objs {
			ref, _ := o[objects.FieldKeyPriorityPlanRef].(string)
			if ref == planID {
				count++
			}
		}
		return count
	}

	res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyPriorityPlanRef: planID,
		},
	})
	if err == nil && len(res.Objects) > 0 {
		return countAll(res.Objects)
	}

	allRes, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindBacklogItem,
	})
	if err != nil {
		return 0
	}
	return countAll(allRes.Objects)
}

func summarizePriorityPlan(obj map[string]any) (string, *WhatsNextPriorityPlan) {
	id, _ := obj[objects.FieldKeyID].(string)
	if id == emptyValue {
		return "", nil
	}
	title, _ := obj[objects.FieldKeyTitle].(string)
	st, _ := obj[objects.FieldKeyStatus].(string)
	return id, &WhatsNextPriorityPlan{ID: id, Title: title, Status: st}
}

func countBacklogByStatus(ctx context.Context, sp storage.ObjectStorageProvider, primaryPlanID string, activePlanIDs []string, personaIDs []string) map[string]int {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	planIDSet := make(map[string]bool)
	if primaryPlanID != emptyValue {
		planIDSet[primaryPlanID] = true
	}
	for _, pid := range activePlanIDs {
		if pid != emptyValue {
			planIDSet[pid] = true
		}
	}

	out := map[string]int{}

	allRes, allErr := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindBacklogItem,
	})
	if allErr != nil {
		return out
	}

	_ = personaIDs // backlog inventory is plan-scoped; persona filtering applies to plans, not BLIs
	for _, o := range allRes.Objects {
		ref, _ := o[objects.FieldKeyPriorityPlanRef].(string)
		if len(planIDSet) > 0 && !planIDSet[ref] {
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

// countActiveWorkItems returns the total number of items requiring active agent work.
func countActiveWorkItems(counts map[string]int) int {
	return counts["in_progress"] + counts["verifying"] + counts["testing"] + counts["planned"] + counts["blocked"] + counts["validated"] + counts["exploring"]
}

// deriveAgentInstructionFromBacklog evaluates backlog status counts and returns the appropriate agent instruction.
func deriveAgentInstructionFromBacklog(counts map[string]int) string {
	activeWork := countActiveWorkItems(counts)
	if activeWork == 0 && counts["completed"] > 0 {
		return "shutdown"
	}
	if counts["in_progress"] > 0 {
		return "continue"
	}
	if counts["verifying"] > 0 || counts["testing"] > 0 {
		return "execute_tests"
	}
	if activeWork == 0 {
		return "shutdown"
	}
	return ""
}

// priorityPlanScoreBonus biases selection toward execution priority_plan objects.
const priorityPlanScoreBonus = 1_000_000

func planStatusEligible(st string) bool {
	return objects.PlanStatusEligibleForWhatsNext(objects.KindPriorityPlan, st)
}

func resolvePersonaIDs(ctx context.Context, sp storage.ObjectStorageProvider, projectRoot, personaID, agentID string) []string {
	if strings.TrimSpace(personaID) != "" {
		return []string{strings.TrimSpace(personaID)}
	}
	if ref := agentfeed.SeatPersonaRef(projectRoot, agentID); ref != "" {
		return []string{ref}
	}
	return getAgentPersonaIDs(ctx, sp, "")
}

func getAgentPersonaIDs(ctx context.Context, sp storage.ObjectStorageProvider, explicitPersonaID string) []string {
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
		// No persona_refs ⇒ not restricted; available to any agent persona.
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

func listActiveOrPausedConvergenceSessions(ctx context.Context, sp storage.ObjectStorageProvider) []WhatsNextCVSRow {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	res, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: objects.KindConvergenceSession})
	if err != nil {
		return nil
	}
	var rows []WhatsNextCVSRow
	for _, obj := range res.Objects {
		st, _ := obj[objects.FieldKeyStatus].(string)
		if !convergence.SessionStatusListedForWhatsNextMeasure(st) {
			continue
		}
		id, _ := obj[objects.FieldKeyID].(string)
		if id == emptyValue {
			continue
		}
		title, _ := obj[objects.FieldKeyTitle].(string)
		phase, _ := obj[objects.FieldKeyCurrentPhase].(string)
		rows = append(rows, WhatsNextCVSRow{
			ID: id, Title: title, CurrentPhase: phase, Status: st,
		})
	}
	return rows
}

func pickDefaultMeasureSessionID(rows []WhatsNextCVSRow) string {
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
	c := execwrap.CommandContext(ctx, exe, "scheduler", "convergence", "measure", "--format", "json", "--session-id", sessionID)
	zqkenv.WireExecForIsolatedProject(c, projectRoot)
	out, err := c.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%w: %s", err, string(ee.Stderr))
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
	return "", fmt.Errorf("cannot resolve CLI executable")
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

const emptyValue = ""
const statusInProcess = "in_progress"

type QueryRequest struct {
	PriorityPlanID string
	SessionID      string
	SkipMeasure    bool
	PersonaID      string
	// AgentID is the swarm seat. When PersonaID is empty, plan selection uses
	// peer_seats[AgentID].persona_ref so Execute matches CLI --agent-id.
	AgentID string
	// SkipPersonaFilter is coordinator duty: Gantt lead across all columns,
	// not the OPERATOR-scoped view. Do not pass a hardcoded orch persona list.
	SkipPersonaFilter bool
}

func Execute(ctx context.Context, sp storage.ObjectStorageProvider, projectRoot string, req *QueryRequest) (*WhatsNextOut, error) {
	out := WhatsNextOut{Schema: WhatsNextSchema, BacklogCountsByStatus: map[string]int{}}

	personaIDs := resolvePersonaIDs(ctx, sp, projectRoot, req.PersonaID, req.AgentID)
	if req.SkipPersonaFilter {
		personaIDs = nil
	}

	planID, planSumm, activePlans := resolvePriorityPlanForWhatsNext(ctx, sp, strings.TrimSpace(req.PriorityPlanID), personaIDs)
	out.PriorityPlan = planSumm
	out.ActivePlans = activePlans

	if planID != emptyValue {
		var activePlanIDs []string
		for _, p := range activePlans {
			if p.ID != "" {
				activePlanIDs = append(activePlanIDs, p.ID)
			}
		}
		out.BacklogCountsByStatus = countBacklogByStatus(ctx, sp, planID, activePlanIDs, personaIDs)
		out.AgentInstruction = deriveAgentInstructionFromBacklog(out.BacklogCountsByStatus)
		// Cue uses the selected plan's children only (not the multi-plan aggregate).
		primaryCounts := countBacklogByStatus(ctx, sp, planID, nil, personaIDs)
		if planSumm != nil {
			out.PackagingCue = PriorityPlanPackagingCue(planSumm.ID, planSumm.Title, planSumm.Status, primaryCounts)
		}
	}

	rows := listActiveOrPausedConvergenceSessions(ctx, sp)
	out.ConvergenceSessionsActive = rows

	if !req.SkipMeasure {
		if req.SessionID != emptyValue {
			out.MeasureSessionID = &req.SessionID
		} else {
			def := pickDefaultMeasureSessionID(rows)
			if def != emptyValue {
				out.MeasureSessionID = &def
			}
		}

		if out.MeasureSessionID != nil && *out.MeasureSessionID != emptyValue {
			measure, err := runSchedulerConvergenceMeasureJSON(projectRoot, *out.MeasureSessionID)
			if err != nil {
				out.MeasureError = err.Error()
			} else {
				out.MeasureCompressed = compressWhatsNextMeasure(measure)
			}
		} else {
			out.MeasureSkipReason = "no --session-id and no active/paused convergence_session"
		}
	} else {
		out.MeasureSkipReason = "skipped via --skip-measure"
	}

	out.ObserverTips = getObserverTips()
	if out.AgentInstruction == "" {
		// Peek only — AdvanceCAPStage runs after stage delivery evidence.
		out.AgentInstruction = SelectCAPInstruction(projectRoot, out.BacklogCountsByStatus)
	}

	amb := LoadKernelAmbience(projectRoot)
	var ambientPlanIDs []string
	for _, p := range activePlans {
		if p.ID != "" {
			ambientPlanIDs = append(ambientPlanIDs, p.ID)
		}
	}
	EnrichKernelAmbienceWithWorkflows(ctx, sp, &amb, planID, ambientPlanIDs)
	ApplySeatOperatingModeIn(&amb, projectRoot, req.AgentID, "")
	out.KernelAmbience = &amb
	out.FillItem = CompileFillItem(&amb)
	out.AgentInstruction = ApplyFillToInstruction(out.AgentInstruction, out.FillItem)
	out.AgentInstruction = PrependStewardProjectionForSeat(out.AgentInstruction, amb.StewardFocus, amb.SeatMode)

	// Evaluate Strategic Readiness and Anticipatory Runway Watermark (delta_runway <= 1)
	if snap, candidates, err := tpm.EvaluateAndReplenishRunway(ctx, sp, projectRoot); err == nil && snap != nil {
		out.RunwayDepth = snap.RunwayDepth
		if snap.IsRunwayDepleted && len(candidates) > 0 {
			top := candidates[0]
			out.StrategicReplenishmentCue = fmt.Sprintf("Watermark delta_runway <= 1 (runway_depth=%d). %d candidate bundle(s) ready for shovel-ready staging. Lead: %s [%s] (%d requirements)", snap.RunwayDepth, len(candidates), top.Title, top.Domain, len(top.RequirementIDs))
		}
	}

	// Materialized View: Probe zero-cost projection and trigger async background recovery if disrupted
	if lite, _ := GetOrRecoverPayload(ctx, sp, projectRoot, DefaultStalenessTolerance); lite != nil {
		out.MaterializedViewStale = lite.Stale
		out.MaterializedViewRecovering = lite.Recovering
		out.MaterializedViewDegradedReason = lite.DegradedReason
	}

	return &out, nil
}
