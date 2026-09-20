package system

import (
	"context"
	"fmt"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/spf13/cobra"
)

// NewAuditMilestonesCmd creates the system audit-milestones command for milestone association enforcement.
func NewAuditMilestonesCmd() *cobra.Command {
	var fix bool
	var defaultMilestoneID string

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Audit milestone associations for backlog items",
		"Validates that backlog items trace to milestones. Outputs unlinked items and can optionally fix them.",
		"",
		"Audit checks:",
		"  - Detect backlog items without milestone_refs",
		"  - Detect backlog items without priority_plan_ref",
		"  - Use --fix to apply a default milestone",
		"  - Use --default-milestone to specify the milestone ID for --fix",
	).
		AddExample("Check milestone associations", "%s system audit-milestones").
		AddExample("Fix unlinked items", "%s system audit-milestones --fix --default-milestone MIL-123").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemAuditMilestonesCommandBuilder(), &cobra.Command{
		Use: "audit-milestones",
	})
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		return runAuditMilestones(cmd, fix, defaultMilestoneID)
	})

	helpBuilder.ApplyToCommand(cmd)
	cli.AddCommonFlags(cmd)

	cmd.Flags().BoolVar(&fix, "fix", false, "Fix unlinked items by associating them with the default milestone")
	cmd.Flags().StringVar(&defaultMilestoneID, "default-milestone", "", "The milestone ID to use when fixing unlinked items (e.g., MIL-triage)")

	return cmd
}

func runAuditMilestones(cmd *cobra.Command, fix bool, defaultMilestoneID string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}

	if fix && defaultMilestoneID == "" {
		return errfmt.Errorf("--default-milestone is required when --fix is specified")
	}

	projectRoot := ProjectRootOrResolveDot(ctx.ProjectRoot)
	if projectRoot == "" {
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

	// List backlog items
	listResult, err := storageProvider.List(cmd.Context(), secCtx, storageCtx, storage.ListFilter{
		Kind:    objects.KindBacklogItem,
		Limit:   10000,
		SortBy:  "id",
		SortAsc: true,
	})
	if err != nil {
		return errfmt.Newf("list backlog_item").Wrap(err)
	}

	var unlinked []map[string]any
	var linked int

	for _, obj := range listResult.Objects {
		hasMilestone := false

		if refsRaw, ok := obj[objects.FieldKeyMilestoneRefs]; ok && refsRaw != nil {
			if refs, ok := refsRaw.([]any); ok && len(refs) > 0 {
				hasMilestone = true
			} else if refsStr, ok := refsRaw.([]string); ok && len(refsStr) > 0 {
				hasMilestone = true
			}
		}

		if !hasMilestone {
			unlinked = append(unlinked, obj)
		} else {
			linked++
		}
	}

	total := len(listResult.Objects)
	unlinkedCount := len(unlinked)

	var fixedCount int
	if fix && unlinkedCount > 0 {
		for _, obj := range unlinked {
			id, ok := obj[objects.FieldKeyID].(string)
			if !ok {
				continue
			}

			// Add the default milestone
			updates := map[string]any{
				objects.FieldKeyMilestoneRefs: []string{defaultMilestoneID},
			}

			errUpdate := storageProvider.Update(cmd.Context(), secCtx, id, updates)
			if errUpdate != nil {
				logging.Fluent(logging.GetLoggerFromProfile(ctx.Profile)).Error("failed to fix milestone association", errUpdate).Log()
			} else {
				fixedCount++
			}
		}
	}

	result := map[string]any{
		"total_backlog_items": total,
		"linked_items":        linked,
		"unlinked_items":      unlinkedCount,
	}

	if fix {
		result["fixed_items"] = fixedCount
	} else if unlinkedCount > 0 {
		// Output up to 20 unlinked items
		limit := unlinkedCount
		if limit > 20 {
			limit = 20
		}
		result["unlinked_sample"] = unlinked[:limit]
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		if err := cli.FormatOutput(cmd, result); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(ctx.Profile)).Error("audit-milestones format output", err).Log()
			return err
		}
		return nil
	default:
		// Default human readable output
		_ = cli.WriteOutput(cmd, []byte("Milestone Association Audit\n"))
		_ = cli.WriteOutput(cmd, []byte("===========================\n"))
		_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Total Backlog Items: %d\n", total)))
		_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Linked Items:        %d\n", linked)))
		_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Unlinked Items:      %d\n", unlinkedCount)))
		if fix {
			_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Fixed Items:         %d\n", fixedCount)))
		} else if unlinkedCount > 0 {
			_ = cli.WriteOutput(cmd, []byte("\nUnlinked Sample:\n"))
			limit := unlinkedCount
			if limit > 20 {
				limit = 20
			}
			for i := 0; i < limit; i++ {
				id, _ := unlinked[i][objects.FieldKeyID].(string)
				title, _ := unlinked[i][objects.FieldKeyTitle].(string)
				_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("  - %s: %s\n", id, title)))
			}
			if unlinkedCount > 20 {
				_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("  ... and %d more\n", unlinkedCount-20)))
			}
		}
		return nil
	}
}
