package system

import (
	"context"
	"fmt"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/spf13/cobra"
)

// NewAlignCmd creates the system align command for strategic alignment validation.
// Implements BLI-780 (align), BLI-781 (alignment scoring) per
// docs/architecture/project-discovery-and-strategic-alignment-v1.0.md.
func NewAlignCmd() *cobra.Command {
	var gaps bool
	var goalID string
	var scoreOnly bool
	var dashboard bool

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Validate strategic alignment (goal-work alignment)",
		"Validates that work items (e.g. backlog items) trace to goals. Outputs alignment score (0-100).",
		"",
		"Alignment checks:",
		"  - Goal alignment: work items with goal_refs vs goals",
		"  - Alignment score: 0-100 (goal coverage; BLI-781)",
		"  - Use --dashboard for a comprehensive alignment view (BLI-782)",
		"  - Use --gaps to list only items without goal alignment",
		"  - Use --goal to focus on a specific goal",
		"  - Use --score to output only the alignment score (for scripting)",
	).
		AddExample("Check alignment", "%s system align").
		AddExample("Alignment dashboard", "%s system align --dashboard").
		AddExample("Show alignment gaps only", "%s system align --gaps").
		AddExample("Alignment for a specific goal", "%s system align --goal GOAL-001").
		AddExample("Score only (scripting)", "%s system align --score").
		ExcludeCommonFlags()

	alignCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemAlignCommandBuilder(), &cobra.Command{
		Use: "align",
	})
	cli.BindAsyncProgress(alignCmd, func(cmd *cobra.Command, args []string) error {
		return runAlign(cmd, gaps, goalID, scoreOnly, dashboard)
	})

	helpBuilder.ApplyToCommand(alignCmd)
	cli.AddCommonFlags(alignCmd)

	alignCmd.Flags().BoolVar(&gaps, "gaps", false, "Show only items without goal alignment")
	alignCmd.Flags().StringVar(&goalID, objects.KindGoal, "", "Filter by goal ID (e.g. GOAL-001); show only work aligned to this goal")
	alignCmd.Flags().BoolVar(&scoreOnly, "score", false, "Output only the alignment score (0-100) for scripting/dashboards")
	alignCmd.Flags().BoolVar(&dashboard, "dashboard", false, "Show comprehensive alignment dashboard (score, goal coverage, and metrics)")

	return alignCmd
}

func runAlign(cmd *cobra.Command, gapsOnly bool, goalID string, scoreOnly bool, dashboard bool) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}

	projectRoot := ProjectRootOrResolveDot(ctx.ProjectRoot)
	if projectRoot == emptyValue {
		return errfmt.Errorf("not a ZQK project (no project root found)")
	}

	factory, err := storage.NewStorageFactory(cmd.Context(), projectRoot)
	if err != nil {
		return errfmt.Newf("storage factory").Wrap(err)
	}
	storageProvider := factory.GetStorage()
	if storageProvider != nil {
		defer func() { _ = storageProvider.Shutdown(context.Background()) }() // Background: request-or-shutdown derived
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// List backlog items (work items with goal_refs)
	listResult, err := storageProvider.List(cmd.Context(), secCtx, storageCtx, storage.ListFilter{
		Kind: objects.KindBacklogItem,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				"$ne": objects.ObjectStatusArchived,
			},
		},
		Fields:  []string{objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus, "goal_ref", objects.FieldKeyGoalRefs},
		Limit:   10000,
		SortBy:  "id",
		SortAsc: true,
	})
	if err != nil {
		return errfmt.Newf("list backlog_item").Wrap(err)
	}

	// List goals if we need them for coverage (optional; align works without goals)
	var goalIDs map[string]bool
	if goalID != emptyValue {
		goalIDs = map[string]bool{goalID: true}
	} else {
		goalList, _ := storageProvider.List(cmd.Context(), secCtx, storageCtx, storage.ListFilter{
			Kind: objects.KindGoal,
			Filters: map[string]any{
				objects.FieldKeyStatus: map[string]any{
					"$ne": objects.ObjectStatusArchived,
				},
			},
			Fields:  []string{objects.FieldKeyID, objects.FieldKeyStatus},
			Limit:   5000,
			SortBy:  "id",
			SortAsc: true,
		})
		goalIDs = make(map[string]bool)
		for _, obj := range goalList.Objects {
			if id, ok := obj[objects.FieldKeyID].(string); ok {
				goalIDs[id] = true
			}
		}
	}

	// Classify backlog items: with goal refs vs without (gaps)
	var withGoals, withoutGoals []map[string]any
	for _, obj := range listResult.Objects {
		refs := GoalRefsFromObject(obj)
		if goalID != emptyValue {
			if refs[goalID] {
				withGoals = append(withGoals, obj)
			}
			continue
		}
		if len(refs) == 0 {
			withoutGoals = append(withoutGoals, obj)
		} else {
			withGoals = append(withGoals, obj)
		}
	}

	total := len(listResult.Objects)
	withCount := len(withGoals)
	var gapCount int
	if goalID != emptyValue {
		gapCount = total - withCount // items not aligned to this goal
	} else {
		gapCount = len(withoutGoals)
	}
	var coveragePct float64
	if total > 0 {
		coveragePct = 100 * float64(withCount) / float64(total)
	}
	// BLI-781: alignment score 0-100 (currently goal-work coverage; extensible to stakeholder/policy/context)
	alignmentScore := RoundTwo(coveragePct)

	result := map[string]any{
		"alignment": map[string]any{
			"goal_work": map[string]any{
				"total_work_items":    total,
				"items_with_goals":    withCount,
				"items_without_goals": gapCount,
				"goal_coverage_pct":   RoundTwo(coveragePct),
			},
			"alignment_score": alignmentScore,
		},
		"alignment_score": alignmentScore,
		"goals_count":     len(goalIDs),
	}

	if goalID != emptyValue {
		result["goal_filter"] = goalID
		result["aligned_items"] = withGoals
	}
	if gapsOnly {
		if goalID != emptyValue {
			result["gaps"] = nil // when filtering by goal, "gaps" are implicit (aligned_items vs all)
		} else {
			result["gaps"] = withoutGoals
		}
	} else if goalID == emptyValue {
		result["gaps_sample"] = TruncateGaps(withoutGoals, 20)
	}

	if scoreOnly {
		// Output only the numeric score for scripting/dashboards
		return cli.WriteOutput(cmd, []byte(fmt.Sprintf("%.1f\n", alignmentScore)))
	}

	if dashboard {
		return OutputAlignDashboard(cmd, result, alignmentScore, total, withCount, gapCount, len(goalIDs))
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		if err := cli.FormatOutput(cmd, result); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(ctx.Profile)).Error("align format output", err).Log()
			return err
		}
		return nil
	default:
		return OutputAlignTable(cmd, result, gapsOnly, goalID)
	}
}
