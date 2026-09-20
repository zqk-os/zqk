package system

import (
	"bytes"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewGitCmd creates a new git command for system operations.
func NewGitCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemGitCommandBuilder(), &cobra.Command{
		Use:   "git",
		Short: "Git commit analysis and integration with work items",
	})

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Git commit analysis and integration with work items",
		"Analyze Git commits and link them to work items (backlog items, milestones, goals).",
		"",
		"This command analyzes Git commits to:",
		"  - Extract work item references from commit messages (BLI-*, MIL-*, GOAL-*)",
		"  - Create code_reference objects for changed files",
		"  - Link commits to work items",
		"  - Enable queries like \"What code changes support goal X?\"",
	).
		ExcludeCommonFlags()

	helpBuilder.ApplyToCommand(cmd)

	analyzeCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemAnalyzeCommandBuilder(), &cobra.Command{
		Use:   "analyze [commit-range]",
		Short: "Analyze Git commits and link to work items",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runGitAnalyze,
	})

	queryCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemQueryCommandBuilder(), &cobra.Command{
		Use:   "query [work-item-id]",
		Short: "Query code changes for a work item",
		Args:  cobra.ExactArgs(1),
		RunE:  runGitQuery,
	})

	analyzeHelpBuilder := clipkg.DynamicHelpBuilder(
		"Analyze Git commits and link to work items",
		"Analyze Git commits in the specified range and link them to work items.",
	).
		AddExample("Analyze last 10 commits", "%s system git analyze HEAD~10..HEAD").
		AddExample("Analyze commits between branches", "%s system git analyze main..feature").
		AddExample("Analyze last 20 commits", "%s system git analyze --recent 20").
		AddExample("Analyze specific commit", "%s system git analyze --hash abc123").
		ExcludeCommonFlags()

	queryHelpBuilder := clipkg.DynamicHelpBuilder(
		"Query code changes for a work item",
		"Query code changes (commits and files) linked to a work item.",
	).
		AddExample("Find commits for backlog item", "%s system git query BLI-625").
		AddExample("Find commits for goal", "%s system git query GOAL-6369").
		AddExample("Find commits for milestone", "%s system git query MIL-036").
		ExcludeCommonFlags()

	analyzeHelpBuilder.ApplyToCommand(analyzeCmd)
	queryHelpBuilder.ApplyToCommand(queryCmd)

	analyzeCmd.Flags().String("hash", "", "Analyze specific commit hash")
	analyzeCmd.Flags().Int("recent", 0, "Analyze last N commits")
	analyzeCmd.Flags().Bool("link", true, "Link commits to work items")

	cli.AddCommonFlags(cmd)
	cli.AddCommonFlags(analyzeCmd)
	cli.AddCommonFlags(queryCmd)

	cmd.AddCommand(analyzeCmd)
	cmd.AddCommand(queryCmd)

	return cmd
}

func runGitAnalyze(cmd *cobra.Command, args []string) error {
	analyzeCtx, err := initializeGitAnalyzeContext(cmd)
	if err != nil {
		return err
	}

	startTime := time.Now()
	commits, err := analyzeCommits(analyzeCtx, args)
	if err != nil {
		return err
	}

	duration := time.Since(startTime)
	logging.FluentEvent(analyzeCtx.Logger).Info("Analyzed commits").
		Int("count", len(commits)).
		String("duration", duration.String()).
		Log()

	if analyzeCtx.Link {
		if err := linkCommitsToWorkItems(analyzeCtx, commits); err != nil {
			return err
		}
	}

	logGitMetrics(analyzeCtx.Logger)

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return outputCommitsStructured(cmd, commits)
	default:
		return outputCommitsTable(cmd, commits)
	}
}

func runGitQuery(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		workItemID := args[0]

		// Read work item
		workItem, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), workItemID)
		if err != nil {
			return errfmt.Newf("failed to read work item %s", workItemID).Wrap(err)
		}

		// Get commit references
		commitRefs := []string{}
		refsRaw, ok := workItem[objects.FieldKeyCommitHashes].([]any)
		if ok {
			for _, ref := range refsRaw {
				if refStr, ok := ref.(string); ok {
					commitRefs = append(commitRefs, refStr)
				}
			}
		}

		// Query code references linked to this work item
		storageCtx := proc.StorageContext()
		filter := storage.ListFilter{
			Kind: objects.KindCodeReference,
			Filters: map[string]any{
				objects.FieldKeyBacklogItemRefs: workItemID,
			},
		}

		// Try different reference fields based on work item kind
		kind, _ := workItem[objects.FieldKeyKind].(string)
		switch kind {
		case objects.KindBacklogItem:
			filter.Filters[objects.FieldKeyBacklogItemRefs] = workItemID
		case objects.KindMilestone:
			filter.Filters[objects.FieldKeyMilestoneRefs] = workItemID
		case objects.KindGoal:
			filter.Filters[objects.FieldKeyGoalRefs] = workItemID
		}

		result, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), storageCtx, filter)
		if err != nil {
			return errfmt.Errorf("failed to query code references: %v", err)
		}

		switch proc.Format() {
		case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
			output := map[string]any{
				"work_item_id":    workItemID,
				"commits":         commitRefs,
				"code_references": result.Objects,
			}
			return cli.FormatOutput(cmd, output)
		default:
			var buf bytes.Buffer
			fmt.Fprintf(&buf, "Code changes for %s:\n\n", workItemID)
			fmt.Fprintf(&buf, "Commits: %d\n", len(commitRefs))
			for _, ref := range commitRefs {
				fmt.Fprintf(&buf, "  - %s\n", ref)
			}
			fmt.Fprintf(&buf, "\nCode References: %d\n", len(result.Objects))
			for _, ref := range result.Objects {
				filePath, _ := ref[objects.FieldKeyFilePath].(string)
				commitHash, _ := ref[objects.FieldKeyCommitHash].(string)
				fmt.Fprintf(&buf, "  - %s (commit: %s)\n", filePath, commitHash)
			}
			return cli.WriteOutput(cmd, buf.Bytes())
		}
	})(cmd, args)
}
