package whatsnext

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/zqkenv"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	observerpkg "github.com/lanceman/zqk/pkg/observer"
	"github.com/lanceman/zqk/pkg/storage"
)

// Default CLI alpha readiness plan (repo lane); still resolved when status is paused.
const defaultAlphaPriorityPlanID = "PLAN-EXAMPLE"

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
	ObserverTips              []string                `json:"observer_tips,omitempty"`
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
	return observerpkg.ReadCachedTips(zqkenv.ProjectRoot())
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
	// Include prioritizing — common mid-lifecycle for priority_plan before active.
	planStatuses := []string{statusInProcess, "paused", "active", "grooming", "prioritizing"}
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
		if planStatusExecutionReady(st) && countOpenLinkedBLIs(ctx, sp, id) > 0 {
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

		openCount := countOpenLinkedBLIs(ctx, sp, id)
		score := openCount
		score += planExecutionStatusBonus(st)
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
	return countOpenLinkedBLIs(ctx, sp, planID) > 0
}

func planStatusExecutionReady(st string) bool {
	switch strings.ToLower(strings.TrimSpace(st)) {
	case statusInProcess, "active", "paused":
		return true
	default:
		return false
	}
}

func planExecutionStatusBonus(st string) int {
	switch strings.ToLower(strings.TrimSpace(st)) {
	case statusInProcess:
		return 500_000
	case "active":
		return 400_000
	case "paused":
		return 200_000
	case "prioritizing":
		return 50_000
	case "grooming":
		return 0
	default:
		return 0
	}
}

// activeOrderPenalty returns a score penalty so lower active_order ranks higher.
// Missing/invalid active_order is treated as lowest priority (large penalty).
func activeOrderPenalty(obj map[string]any) int {
	const unsetPenalty = 1_000_000
	v, ok := obj[objects.FieldKeyActiveOrder]
	if !ok || v == nil {
		return unsetPenalty
	}
	switch n := v.(type) {
	case int:
		if n < 0 {
			return unsetPenalty
		}
		return n
	case int64:
		if n < 0 {
			return unsetPenalty
		}
		return int(n)
	case float64:
		if n < 0 {
			return unsetPenalty
		}
		return int(n)
	default:
		return unsetPenalty
	}
}

func bliStatusTerminal(st string) bool {
	switch strings.ToLower(strings.TrimSpace(st)) {
	case objects.ObjectStatusComplete, objects.ObjectStatusArchived, objects.ObjectStatusCancelled,
		objects.ObjectStatusImplemented, objects.ObjectStatusRejected:
		return true
	default:
		return false
	}
}

// countOpenLinkedBLIs returns non-terminal BLIs linked to a plan (child priority_plan_ref).
// Falls back to client-side filtering if the storage filter returns empty.
func countOpenLinkedBLIs(ctx context.Context, sp storage.ObjectStorageProvider, planID string) int {
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
			if bliStatusTerminal(st) {
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
	return counts["in_progress"] + counts["verifying"] + counts["planned"] + counts["blocked"] + counts["validated"]
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
	if counts["verifying"] > 0 {
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
	switch strings.ToLower(strings.TrimSpace(st)) {
	case statusInProcess, "paused", "active", "grooming", "prioritizing":
		return true
	default:
		return false
	}
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
	c := exec.CommandContext(ctx, exe, "scheduler", "convergence", "measure", "--format", "json", "--session-id", sessionID)
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
	if p, err := os.Executable(); err == nil && strings.TrimSpace(p) != "" {
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
}

func Execute(ctx context.Context, sp storage.ObjectStorageProvider, projectRoot string, req *QueryRequest) (*WhatsNextOut, error) {
	out := WhatsNextOut{Schema: WhatsNextSchema, BacklogCountsByStatus: map[string]int{}}

	personaIDs := getAgentPersonaIDs(ctx, sp, req.PersonaID)

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

	return &out, nil
}
