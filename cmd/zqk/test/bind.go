package test

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// CandidateMatch represents a scored candidate requirement or goal for lineage binding.
type CandidateMatch struct {
	ID    string
	Title string
	Score int
}

// NewBindCmd creates the `zqk test bind` command.
func NewBindCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewTestBindCommandBuilder()
	cmd.Aliases = []string{"trace-bind", "link"}
	cmd.RunE = cli.WithProcessor(runBind)
	return cmd
}

func runBind(cmd *cobra.Command, args []string, proc *cli.Processor) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var flags clipkg.FlagBag
	reqFlag := flags.String(cmd, "req")
	goalFlag := flags.String(cmd, "goal")
	bliFlag := flags.String(cmd, "bli")
	autoFlag := flags.Bool(cmd, "auto")
	dryRun := flags.Bool(cmd, "dry-run")
	if err := flags.Err(); err != nil {
		return err
	}

	sp := proc.Storage()
	secCtx := proc.SecurityContext()

	state := NewDashboardState()
	if err := state.LoadFromStorage(ctx, proc, false, "", ""); err != nil {
		return fmt.Errorf("failed to load test assets from storage: %w", err)
	}

	var targetTCID string
	if len(args) > 0 {
		targetTCID = strings.TrimSpace(args[0])
	}

	if targetTCID != "" && (reqFlag != "" || goalFlag != "" || bliFlag != "") {
		return executeExplicitBinding(ctx, proc, targetTCID, reqFlag, goalFlag, bliFlag, dryRun, cmd)
	}
	if autoFlag {
		return executeAutoBinding(ctx, proc, state, dryRun, cmd)
	}
	return displayBindingAssistant(ctx, sp, secCtx, state, targetTCID, cmd)
}

func executeExplicitBinding(
	ctx context.Context,
	proc *cli.Processor,
	tcID, reqID, goalID, bliID string,
	dryRun bool,
	cmd *cobra.Command,
) error {
	sp := proc.Storage()
	secCtx := proc.SecurityContext()
	tcObj, err := sp.Read(ctx, secCtx, tcID)
	if err != nil || tcObj == nil {
		return fmt.Errorf("test case %s not found: %w", tcID, err)
	}

	ui := cli.StandardUIPalette()
	bold, cyan, green := ui.Bold, ui.Cyan, ui.Green

	// 1. Bind requirement or backlog item to test case
	if reqID != "" {
		curReqs := lifecycle.StringRefsFromAny(tcObj[objects.FieldKeyRequirementRefs])
		if !containsString(curReqs, reqID) {
			curReqs = append(curReqs, reqID)
			tcObj[objects.FieldKeyRequirementRefs] = curReqs
			if !dryRun {
				cleanLegacyFields(tcObj)
				if err := sp.Update(ctx, secCtx, tcID, tcObj); err != nil {
					return fmt.Errorf("failed to update test case %s: %w", tcID, err)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Bound %s ➔ %s\n", bold(tcID), cyan(reqID))
		}
	}

	if bliID != "" {
		curBLIs := lifecycle.StringRefsFromAny(tcObj[objects.FieldKeyBacklogItemRefs])
		if !containsString(curBLIs, bliID) {
			curBLIs = append(curBLIs, bliID)
			tcObj[objects.FieldKeyBacklogItemRefs] = curBLIs
			if !dryRun {
				cleanLegacyFields(tcObj)
				if err := sp.Update(ctx, secCtx, tcID, tcObj); err != nil {
					return fmt.Errorf("failed to update test case %s with backlog item: %w", tcID, err)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Bound %s ➔ %s\n", bold(tcID), cyan(bliID))
		}
	}

	// 2. Bind goal to requirement if provided
	if goalID != "" && reqID != "" {
		reqObj, err := sp.Read(ctx, secCtx, reqID)
		if err == nil && reqObj != nil {
			curGoals := lifecycle.StringRefsFromAny(reqObj[objects.FieldKeyGoalRefs])
			if !containsString(curGoals, goalID) {
				curGoals = append(curGoals, goalID)
				reqObj[objects.FieldKeyGoalRefs] = curGoals
				if !dryRun {
					cleanLegacyFields(reqObj)
					if err := sp.Update(ctx, secCtx, reqID, reqObj); err != nil {
						return fmt.Errorf("failed to update requirement %s with goal: %w", reqID, err)
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Bound %s ➔ %s\n", cyan(reqID), green(goalID))
			}
		}
	}

	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "\n[DRY RUN] No changes were written to storage.\n")
		return nil
	}

	// Refresh dashboard state and save lite-file
	state := NewDashboardState()
	_ = state.LoadFromStorage(ctx, proc, false, "", "")
	_ = state.SaveToLiteFile(proc.ProjectRoot())

	fmt.Fprintf(cmd.OutOrStdout(), "%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("\n%s Lineage updated successfully! Run 'zqk test dashboard' to inspect.\n", green("✓"))))
	return nil
}

func displayBindingAssistant(
	ctx context.Context,
	sp storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	state *DashboardState,
	filterTCID string,
	cmd *cobra.Command,
) error {
	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	// Load available requirements and goals for matching (filter out completed/archived, project needed fields)
	reqList, goalList := listActiveRequirementsAndGoals(ctx, sp, secCtx)

	var brokenTCs []*TestCaseModel
	for _, tcID := range state.TestCaseOrder {
		if filterTCID != "" && !strings.EqualFold(tcID, filterTCID) {
			continue
		}
		tc := state.TestCases[tcID]
		if tc == nil || tc.Status == objects.ObjectStatusComplete || tc.Status == objects.ObjectStatusArchived {
			continue
		}
		if tc.Lineage == nil || !tc.Lineage.IsIntact {
			brokenTCs = append(brokenTCs, tc)
		}
	}

	if len(brokenTCs) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\n%s %s\n\n", green("✓"), bold("TPM Definition of Done SATISFIED: All active test case lineages are intact up to root!"))
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", bold("🛠️  TPM LINEAGE BINDING ASSISTANT | [DEFINITION OF DONE]"))
	fmt.Fprintf(cmd.OutOrStdout(), "══════════════════════════════════════════════════════════════════════════════\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Found %s test case(s) with broken or incomplete lineage chains.\n\n", yellow(fmt.Sprintf("%d", len(brokenTCs))))

	for idx, tc := range brokenTCs {
		if idx >= 10 {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", dim(fmt.Sprintf("... and %d more test cases (use --auto to repair all)", len(brokenTCs)-idx)))
			break
		}

		fmt.Fprintf(cmd.OutOrStdout(), "▶ %s  %s\n", bold(tc.ID), tc.Title)
		fmt.Fprintf(cmd.OutOrStdout(), "    Current: %s\n", formatLineageStrip(tc.Lineage, tc.ID, tc.Status))

		// Check what is missing
		if len(tc.RequirementRefs) == 0 && len(tc.BacklogItemRefs) == 0 {
			// Needs requirement binding
			candidates := rankCandidates(tc.Title, reqList.Objects)
			fmt.Fprintf(cmd.OutOrStdout(), "    Issue  : Missing parent requirement or backlog item reference.\n")
			fmt.Fprintf(cmd.OutOrStdout(), "    Options:\n")
			for optIdx, c := range candidates {
				if optIdx >= 2 {
					break
				}
				prefix := "(Recommended)"
				if optIdx > 0 {
					prefix = "(Alternative)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "      %s %s %s %s\n", yellow(fmt.Sprintf("[%d]", optIdx+1)), cyan(prefix), bold(c.ID), dim(c.Title))
				fmt.Fprintf(cmd.OutOrStdout(), "%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("          zqk test bind %s --req %s\n", tc.ID, c.ID)))
			}
		} else if tc.Lineage != nil && tc.Lineage.RootObject == nil {
			// Has requirement, but requirement lacks root goal
			reqID := ""
			if len(tc.Lineage.Requirements) > 0 {
				reqID = tc.Lineage.Requirements[0].ID
			} else if len(tc.RequirementRefs) > 0 {
				reqID = tc.RequirementRefs[0]
			}
			candidates := rankCandidates(tc.Title, goalList.Objects)
			fmt.Fprintf(cmd.OutOrStdout(), "    Issue  : Requirement %s has no root Goal or Priority Plan bound.\n", cyan(reqID))
			fmt.Fprintf(cmd.OutOrStdout(), "    Options:\n")
			for optIdx, c := range candidates {
				if optIdx >= 2 {
					break
				}
				prefix := "(Recommended)"
				if optIdx > 0 {
					prefix = "(Alternative)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "      %s %s %s %s\n", yellow(fmt.Sprintf("[%d]", optIdx+1)), cyan(prefix), bold(c.ID), dim(c.Title))
				fmt.Fprintf(cmd.OutOrStdout(), "%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("          zqk test bind %s --req %s --goal %s\n", tc.ID, reqID, c.ID)))
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\n")
	}

	fmt.Fprintf(cmd.OutOrStdout(), "──────────────────────────────────────────────────────────────────────────────\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Quick Command to automatically bind all incomplete chains using best matches:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  %s\n\n", bold(paths.CLIUsage("test", "bind", "--auto")))
	return nil
}

func executeAutoBinding(
	ctx context.Context,
	proc *cli.Processor,
	state *DashboardState,
	dryRun bool,
	cmd *cobra.Command,
) error {
	sp, secCtx, projectRoot := proc.Storage(), proc.SecurityContext(), proc.ProjectRoot()
	reqList, goalList := listActiveRequirementsAndGoals(ctx, sp, secCtx)

	ui := cli.StandardUIPalette()
	bold, cyan, green := ui.Bold, ui.Cyan, ui.Green

	boundCount := 0

	for _, tcID := range state.TestCaseOrder {
		tc := state.TestCases[tcID]
		if tc == nil || tc.Status == objects.ObjectStatusComplete || tc.Status == objects.ObjectStatusArchived {
			continue
		}
		if tc.Lineage != nil && tc.Lineage.IsIntact {
			continue
		}

		tcObj, err := sp.Read(ctx, secCtx, tcID)
		if err != nil || tcObj == nil {
			continue
		}

		reqRefs := lifecycle.StringRefsFromAny(tcObj[objects.FieldKeyRequirementRefs])
		if len(reqRefs) == 0 {
			candidates := rankCandidates(tc.Title, reqList.Objects)
			if len(candidates) > 0 {
				topReq := candidates[0].ID
				tcObj[objects.FieldKeyRequirementRefs] = []string{topReq}
				if !dryRun {
					cleanLegacyFields(tcObj)
					if err := sp.Update(ctx, secCtx, tcID, tcObj); err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "  ⚠️ Failed to update test case %s: %v\n", tcID, err)
						continue
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Bound %s ➔ %s\n", bold(tcID), cyan(topReq))
				boundCount++
				reqRefs = []string{topReq}
			}
		}

		// Ensure requirement has a root goal bound
		if len(reqRefs) > 0 {
			reqID := reqRefs[0]
			reqObj, err := sp.Read(ctx, secCtx, reqID)
			if err == nil && reqObj != nil {
				goalRefs := lifecycle.StringRefsFromAny(reqObj[objects.FieldKeyGoalRefs])
				if len(goalRefs) == 0 {
					candidates := rankCandidates(tc.Title, goalList.Objects)
					if len(candidates) > 0 {
						topGoal := candidates[0].ID
						reqObj[objects.FieldKeyGoalRefs] = []string{topGoal}
						if !dryRun {
							cleanLegacyFields(reqObj)
							if err := sp.Update(ctx, secCtx, reqID, reqObj); err != nil {
								fmt.Fprintf(cmd.ErrOrStderr(), "  ⚠️ Failed to update requirement %s: %v\n", reqID, err)
								continue
							}
						}
						fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Bound %s ➔ %s\n", cyan(reqID), green(topGoal))
						boundCount++
					}
				}
			}
		}
	}

	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "\n[DRY RUN] %d binding(s) proposed.\n", boundCount)
		return nil
	}

	// Refresh dashboard lite-file
	stateRefresh := NewDashboardState()
	_ = stateRefresh.LoadFromStorage(ctx, proc, false, "", "")
	_ = stateRefresh.SaveToLiteFile(projectRoot)

	fmt.Fprintf(cmd.OutOrStdout(), "\n%s Successfully applied %d lineage binding(s)! Working set is now closed.\n", green("✓"), boundCount)
	return nil
}

func rankCandidates(targetTitle string, candidates []map[string]any) []CandidateMatch {
	targetWords := extractKeywords(targetTitle)
	var matches []CandidateMatch

	for _, cand := range candidates {
		id, _ := cand[objects.FieldKeyID].(string)
		title, _ := cand[objects.FieldKeyTitle].(string)
		if id == "" {
			continue
		}

		candWords := extractKeywords(title + " " + id)
		score := calculateWordOverlap(targetWords, candWords)

		// Base match bonus for common domain roots
		if strings.Contains(strings.ToLower(id), "core") || strings.Contains(strings.ToLower(id), "launch") {
			score += 1
		}

		matches = append(matches, CandidateMatch{
			ID:    id,
			Title: title,
			Score: score,
		})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].ID < matches[j].ID
	})

	return matches
}

func extractKeywords(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	var clean []string
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "for": true, "to": true, "in": true, "of": true, "test": true, "suite": true, "from": true, "case": true, "with": true, "on": true,
	}
	for _, w := range words {
		w = strings.Trim(w, ":,.-_()[]!?;\"'")
		if len(w) > 2 && !stopWords[w] {
			clean = append(clean, w)
		}
	}
	return clean
}

func calculateWordOverlap(target, candidate []string) int {
	candSet := make(map[string]bool, len(candidate))
	for _, c := range candidate {
		candSet[c] = true
	}
	score := 0
	for _, t := range target {
		if candSet[t] {
			score += 10
		}
	}
	return score
}

func containsString(list []string, item string) bool {
	for _, s := range list {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}

func cleanLegacyFields(obj map[string]any) {
	legacyFields := []string{
		"category", "phase", "date_captured", "group", "origin_project", "origin_system",
		"acceptance_criteria", "spec_adherence", "context", "date", "spec_refs",
		"collection_count", "cron_restarts", "first_seen", "health_check_duration_ms",
		"health_checks", "last_seen", "measurement_window_end", "measurement_window_start",
		"metric_type", "missed_triggers", "recovered_jobs", "estimated_effort",
		"event_type", "operation", "benefits", "components", "considerations",
		"priority_plan_ref",
	}
	for _, f := range legacyFields {
		obj[f] = storage.FieldUnset
	}
	if a, ok := obj["archived_at"]; ok {
		if _, isStr := a.(string); !isStr {
			obj["archived_at"] = storage.FieldUnset
		}
	}
	if a, ok := obj["archived_by"]; ok {
		if _, isStr := a.(string); !isStr {
			obj["archived_by"] = storage.FieldUnset
		}
	}
}

func listActiveRequirementsAndGoals(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) (*storage.QueryResult, *storage.QueryResult) {
	activeStatusFilter := map[string]any{
		objects.FieldKeyStatus: map[string]any{"$nin": []any{objects.ObjectStatusComplete, objects.ObjectStatusArchived}},
	}
	reqFields := []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, objects.FieldKeyGoalRefs}
	goalFields := []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus}
	reqList, _ := sp.List(ctx, secCtx, nil, storage.ListFilter{
		Kind:    objects.KindRequirement,
		Filters: activeStatusFilter,
		Fields:  reqFields,
	})
	goalList, _ := sp.List(ctx, secCtx, nil, storage.ListFilter{
		Kind:    objects.KindGoal,
		Filters: activeStatusFilter,
		Fields:  goalFields,
	})
	return reqList, goalList
}


