package workflow

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/policyinterrupt"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

const (
	statusComplete     = "complete"
	statusArchived     = "archived"
	statusRejected     = "rejected"
	statusCancelled    = "cancelled"
	statusInProcess    = "in_progress"
	statusQuestionOpen = "open"
	emptyValue         = ""

	sessionSourceEnv       = "env"
	sessionSourceStateFile = "state_file"
	sessionSourceNone      = "none"

	sourceKindPolicyInterrupt = "policy_interrupt"
	sourceFieldDedupeKey      = "dedupe_key"
)

type sourceRef struct {
	Kind  string `json:"kind" yaml:"kind"`
	ID    string `json:"id" yaml:"id"`
	Note  string `json:"note,omitempty" yaml:"note,omitempty"`
	Field string `json:"field,omitempty" yaml:"field,omitempty"`
}

// workflowStorage is the storage surface used by workflow next selection, enrichment, and signals.
// Production code passes storage.ObjectStorageProvider; tests use an in-memory implementation.
type workflowStorage interface {
	Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
	List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error)
	Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter storage.ListFilter) (int, error)
	Update(ctx context.Context, secCtx *storage.SecurityContext, id string, data map[string]any) error
}

type workflowNextResult struct {
	SessionID          string      `json:"session_id,omitempty" yaml:"session_id,omitempty"`
	SessionSource      string      `json:"session_source,omitempty" yaml:"session_source,omitempty"`
	Decision           string      `json:"decision" yaml:"decision"`
	Rationale          string      `json:"rationale" yaml:"rationale"`
	RecommendedCommand string      `json:"recommended_command" yaml:"recommended_command"`
	Precedence         []string    `json:"precedence" yaml:"precedence"`
	Sources            []sourceRef `json:"sources" yaml:"sources"`

	// Backlog branch only: loaded from storage so humans see plan/item narrative (not just IDs).
	BacklogPriorityTier string   `json:"backlog_priority_tier,omitempty" yaml:"backlog_priority_tier,omitempty"`
	PriorityPlanTitle   string   `json:"priority_plan_title,omitempty" yaml:"priority_plan_title,omitempty"`
	PriorityPlanSummary string   `json:"priority_plan_summary,omitempty" yaml:"priority_plan_summary,omitempty"`
	BacklogItemSummary  string   `json:"backlog_item_summary,omitempty" yaml:"backlog_item_summary,omitempty"`
	GoalRefs            []string `json:"goal_refs,omitempty" yaml:"goal_refs,omitempty"`
	// Optional backlog_item convergence_session_ref / profile when set on the selected item.
	ConvergenceSessionRef     string `json:"convergence_session_ref,omitempty" yaml:"convergence_session_ref,omitempty"`
	ConvergenceSessionProfile string `json:"convergence_session_profile,omitempty" yaml:"convergence_session_profile,omitempty"`
	// OpenQuestionsCount is the number of question objects with status=open (workspace signal).
	OpenQuestionsCount int `json:"open_questions_count" yaml:"open_questions_count"`
}

// NewNextCmd returns workflow next command.
func NewNextCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewWorkflowNextCommandBuilder()
	cli.BindAsyncProgress(cmd, runNext)
	cli.RequireStorage(cmd, true)
	cli.RequireSession(cmd, true)
	cli.RequireSchedulerCheck(cmd, true)
	return cmd
}

func runNext(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		projectRoot := proc.ProjectRoot()
		if projectRoot == emptyValue {
			projectRoot = cli.ResolveProjectRoot(".")
		}
		sessionID, sessionSource := resolveSession(projectRoot)

		result := workflowNextResult{
			SessionID:          sessionID,
			SessionSource:      sessionSource,
			Precedence:         []string{decisionCriticalPolicyInterrupt, decisionActiveConvergence, decisionPriorityPlanBacklog},
			Decision:           decisionNone,
			Rationale:          decisionSpecs[decisionNone].Rationale,
			RecommendedCommand: decisionSpecs[decisionNone].CommandBuilder(emptyValue),
			Sources:            []sourceRef{},
		}
		pol, _ := selectPolicyInterrupt(projectRoot)
		cv := selectActiveConvergence(proc.OperationContext(), proc.Storage())
		planID, item := selectBacklogNext(proc.OperationContext(), proc.Storage())
		applyDecision(&result, pol, cv, planID, item)
		result.OpenQuestionsCount = countOpenQuestions(proc.OperationContext(), proc.Storage())
		if result.Decision == decisionPriorityPlanBacklog && planID != emptyValue && item != nil {
			enrichPriorityPlanBacklog(proc.OperationContext(), proc.Storage(), &result, planID, item)
		}
		return writeWorkflowNextOutput(cmd, result)
	})(cmd, args)
}

type policyDecision struct {
	DedupeKey string
	Message   string
}

func applyDecision(result *workflowNextResult, pol *policyDecision, cv *convergenceInfo, planID string, item *backlogInfo) {
	if result == nil {
		return
	}
	if pol != nil {
		applyDecisionSpec(result, decisionCriticalPolicyInterrupt, pol.DedupeKey)
		result.Sources = append(result.Sources, sourceRef{
			Kind:  sourceKindPolicyInterrupt,
			ID:    pol.DedupeKey,
			Note:  pol.Message,
			Field: sourceFieldDedupeKey,
		})
		return
	}
	if cv != nil {
		applyDecisionSpec(result, decisionActiveConvergence, cv.ID)
		result.Sources = append(result.Sources, sourceRef{
			Kind:  objects.KindConvergenceSession,
			ID:    cv.ID,
			Note:  cv.Title,
			Field: objects.FieldKeyStatus,
		})
		return
	}
	if item != nil {
		applyDecisionSpec(result, decisionPriorityPlanBacklog, item.ID)
		result.Sources = append(result.Sources,
			sourceRef{Kind: objects.KindPriorityPlan, ID: planID, Field: objects.FieldKeyStatus},
			sourceRef{Kind: objects.KindBacklogItem, ID: item.ID, Note: item.Title, Field: objects.FieldKeyPriorityTier},
		)
	}
}

func selectPolicyInterrupt(projectRoot string) (*policyDecision, bool) {
	if projectRoot == emptyValue {
		return nil, false
	}
	acks, err := policyinterrupt.LoadAcksIncremental(projectRoot)
	if err != nil {
		return nil, false
	}
	latest, err := policyinterrupt.LoadLatestCriticalUnacked(projectRoot, acks)
	if err != nil || latest == nil {
		return nil, false
	}
	return &policyDecision{
		DedupeKey: latest.DedupeKey,
		Message:   latest.Message,
	}, true
}

type convergenceInfo struct {
	ID        string
	Title     string
	UpdatedAt string
}

func selectActiveConvergence(ctx context.Context, sp workflowStorage) *convergenceInfo {
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	storageCtx := pkgctx.NewStorageContext()
	result, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindConvergenceSession,
		Filters: map[string]any{
			objects.FieldKeyStatus: statusInProcess,
		},
	})
	if err != nil || len(result.Objects) == 0 {
		return nil
	}
	rows := make([]convergenceInfo, 0, len(result.Objects))
	for _, obj := range result.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		if id == emptyValue {
			continue
		}
		title, _ := obj[objects.FieldKeyTitle].(string)
		updated, _ := obj[objects.FieldKeyUpdatedAt].(string)
		rows = append(rows, convergenceInfo{ID: id, Title: title, UpdatedAt: updated})
	}
	if len(rows) == 0 {
		return nil
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UpdatedAt == rows[j].UpdatedAt {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].UpdatedAt > rows[j].UpdatedAt
	})
	return &rows[0]
}

type backlogInfo struct {
	ID           string
	Title        string
	PriorityTier string
	UpdatedAt    string
}

func selectBacklogNext(ctx context.Context, sp workflowStorage) (string, *backlogInfo) {
	planID := resolveCurrentPlanID(ctx, sp)
	if planID == emptyValue {
		return "", nil
	}
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	storageCtx := pkgctx.NewStorageContext()
	result, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyPriorityPlanRef: planID,
			objects.FieldKeyStatus: map[string]any{
				"$nin": []string{statusComplete, statusArchived, statusRejected, statusCancelled},
			},
		},
	})
	if err != nil || len(result.Objects) == 0 {
		return planID, nil
	}
	rows := make([]backlogInfo, 0, len(result.Objects))
	for _, obj := range result.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		if id == emptyValue {
			continue
		}
		title, _ := obj[objects.FieldKeyTitle].(string)
		pt, _ := obj[objects.FieldKeyPriorityTier].(string)
		updated, _ := obj[objects.FieldKeyUpdatedAt].(string)
		rows = append(rows, backlogInfo{ID: id, Title: title, PriorityTier: strings.ToUpper(pt), UpdatedAt: updated})
	}
	if len(rows) == 0 {
		return planID, nil
	}
	sort.Slice(rows, func(i, j int) bool {
		ri := priorityRank(rows[i].PriorityTier)
		rj := priorityRank(rows[j].PriorityTier)
		if ri != rj {
			return ri < rj
		}
		if rows[i].UpdatedAt != rows[j].UpdatedAt {
			return rows[i].UpdatedAt > rows[j].UpdatedAt
		}
		return rows[i].ID < rows[j].ID
	})
	return planID, &rows[0]
}

func resolveCurrentPlanID(ctx context.Context, sp workflowStorage) string {
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	storageCtx := pkgctx.NewStorageContext()
	result, err := sp.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindPriorityPlan,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				"$in": []string{statusInProcess, "active"},
			},
		},
	})
	if err != nil || len(result.Objects) == 0 {
		return ""
	}
	type row struct {
		ID          string
		ActiveOrder int
		HasOrder    bool
		UpdatedAt   string
	}
	rows := make([]row, 0, len(result.Objects))
	for _, obj := range result.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		if id == emptyValue {
			continue
		}
		r := row{ID: id}
		switch v := obj[objects.FieldKeyActiveOrder].(type) {
		case int:
			r.ActiveOrder = v
			r.HasOrder = true
		case float64:
			r.ActiveOrder = int(v)
			r.HasOrder = true
		}
		r.UpdatedAt, _ = obj[objects.FieldKeyUpdatedAt].(string)
		rows = append(rows, r)
	}
	if len(rows) == 0 {
		return ""
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].HasOrder != rows[j].HasOrder {
			return rows[i].HasOrder
		}
		if rows[i].HasOrder && rows[i].ActiveOrder != rows[j].ActiveOrder {
			return rows[i].ActiveOrder < rows[j].ActiveOrder
		}
		if rows[i].UpdatedAt != rows[j].UpdatedAt {
			return rows[i].UpdatedAt > rows[j].UpdatedAt
		}
		return rows[i].ID < rows[j].ID
	})
	return rows[0].ID
}

func priorityRank(tier string) int {
	switch tier {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P2":
		return 2
	case "P3":
		return 3
	default:
		return 9
	}
}

func resolveSession(projectRoot string) (string, string) {
	if id := strings.TrimSpace(zqkenv.SessionID().Get()); id != emptyValue {
		return id, sessionSourceEnv
	}
	if projectRoot != emptyValue {
		statePath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "session")
		if b, err := fileutil.ReadFile(statePath); err == nil {
			if id := strings.TrimSpace(string(b)); id != emptyValue {
				return id, sessionSourceStateFile
			}
		}
	}
	return "", sessionSourceNone
}

const (
	workflowPlanSummaryMaxRunes  = 360
	workflowItemSummaryMaxRunes  = 520
	workflowGoalRefsDisplayLimit = 12
)

func enrichPriorityPlanBacklog(ctx context.Context, sp workflowStorage, result *workflowNextResult, planID string, item *backlogInfo) {
	if result == nil || item == nil || planID == emptyValue || sp == nil {
		return
	}
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	result.BacklogPriorityTier = item.PriorityTier

	planObj, err := sp.Read(ctx, secCtx, planID)
	if err == nil && planObj != nil {
		if t, _ := planObj[objects.FieldKeyTitle].(string); strings.TrimSpace(t) != emptyValue {
			result.PriorityPlanTitle = strings.TrimSpace(t)
		}
		if d, _ := planObj[objects.FieldKeyDescription].(string); strings.TrimSpace(d) != emptyValue {
			result.PriorityPlanSummary = truncateWorkflowText(strings.TrimSpace(d), workflowPlanSummaryMaxRunes)
		}
	}

	itemObj, err := sp.Read(ctx, secCtx, item.ID)
	if err != nil || itemObj == nil {
		return
	}
	if d, _ := itemObj[objects.FieldKeyDescription].(string); strings.TrimSpace(d) != emptyValue {
		result.BacklogItemSummary = truncateWorkflowText(strings.TrimSpace(d), workflowItemSummaryMaxRunes)
	}
	result.GoalRefs = stringSliceFromAnyField(itemObj[objects.FieldKeyGoalRefs])
	if len(result.GoalRefs) > workflowGoalRefsDisplayLimit {
		result.GoalRefs = result.GoalRefs[:workflowGoalRefsDisplayLimit]
	}
	if ref, _ := itemObj[objects.FieldKeyConvergenceSessionRef].(string); strings.TrimSpace(ref) != emptyValue {
		result.ConvergenceSessionRef = strings.TrimSpace(ref)
	}
	if prof, _ := itemObj[objects.FieldKeyConvergenceSessionProfile].(string); strings.TrimSpace(prof) != emptyValue {
		result.ConvergenceSessionProfile = strings.TrimSpace(prof)
	}
}

func countOpenQuestions(ctx context.Context, sp workflowStorage) int {
	if sp == nil {
		return 0
	}
	secCtx := pkgctx.GetSecurityContext(ctx)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}
	n, err := sp.Count(ctx, secCtx, storage.ListFilter{
		Kind: objects.KindQuestion,
		Filters: map[string]any{
			objects.FieldKeyStatus: statusQuestionOpen,
		},
	})
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func truncateWorkflowText(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}

func stringSliceFromAnyField(v any) []string {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []string:
		out := make([]string, 0, len(x))
		for _, s := range x {
			s = strings.TrimSpace(s)
			if s != emptyValue {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				s = strings.TrimSpace(s)
				if s != emptyValue {
					out = append(out, s)
				}
			}
		}
		return out
	default:
		return nil
	}
}

func writeWorkflowNextOutput(cmd *cobra.Command, result workflowNextResult) error {
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, result)
	case cli.FormatTable:
		var buf bytes.Buffer
		buf.WriteString("Workflow next recommendation\n")
		buf.WriteString("============================\n")
		buf.WriteString("Decision: " + result.Decision + "\n")
		buf.WriteString("Recommended command: " + result.RecommendedCommand + "\n")
		buf.WriteString("Rationale: " + result.Rationale + "\n")
		if result.SessionID != emptyValue {
			buf.WriteString("Session: " + result.SessionID + " (" + result.SessionSource + ")\n")
		}
		if result.OpenQuestionsCount > 0 {
			buf.WriteString(fmt.Sprintf("Open questions (status=%s): %d\n", statusQuestionOpen, result.OpenQuestionsCount))
		}
		if result.Decision == decisionPriorityPlanBacklog && (result.PriorityPlanTitle != emptyValue || result.PriorityPlanSummary != emptyValue || result.BacklogItemSummary != emptyValue || result.BacklogPriorityTier != emptyValue || result.ConvergenceSessionRef != emptyValue || result.ConvergenceSessionProfile != emptyValue || len(result.GoalRefs) > 0) {
			buf.WriteString("\nWork context (from priority plan + backlog item):\n")
			if result.BacklogPriorityTier != emptyValue {
				buf.WriteString("  Tier: " + result.BacklogPriorityTier + " (P0 is highest; workflow next picks the best eligible item on the current in-progress plan)\n")
			}
			if result.PriorityPlanTitle != emptyValue {
				buf.WriteString("  Plan: " + result.PriorityPlanTitle + "\n")
			}
			if result.PriorityPlanSummary != emptyValue {
				buf.WriteString("  Plan summary: " + result.PriorityPlanSummary + "\n")
			}
			if result.BacklogItemSummary != emptyValue {
				buf.WriteString("  Item summary: " + result.BacklogItemSummary + "\n")
			}
			if len(result.GoalRefs) > 0 {
				buf.WriteString("  Goal refs: " + strings.Join(result.GoalRefs, ", ") + "\n")
			}
			if result.ConvergenceSessionRef != emptyValue {
				line := "  Convergence session: " + result.ConvergenceSessionRef
				if result.ConvergenceSessionProfile != emptyValue {
					line += " (" + result.ConvergenceSessionProfile + ")"
				}
				buf.WriteString(line + "\n")
			}
		}
		if len(result.Sources) > 0 {
			buf.WriteString("Sources:\n")
			for _, s := range result.Sources {
				line := fmt.Sprintf("- %s %s", s.Kind, s.ID)
				if s.Note != emptyValue {
					line += " :: " + s.Note
				}
				buf.WriteString(line + "\n")
			}
		}
		buf.WriteString("Precedence: " + strings.Join(result.Precedence, " > ") + "\n")
		if result.Decision == decisionPriorityPlanBacklog {
			buf.WriteString("\nTip: To focus on a theme (e.g. data-cell architecture), raise its backlog tier to P0, complete or archive competing P0 items, or link items to goals via goal_refs so reporting stays aligned.\n")
		}
		return cli.WriteOutput(cmd, buf.Bytes())
	default:
		return cli.FormatOutput(cmd, result)
	}
}
