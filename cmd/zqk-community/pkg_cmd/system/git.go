package system

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

var gitCmd = clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemGitCommandBuilder(), &cobra.Command{
	Use:   "git",
	Short: "Git commit analysis and integration with work items",
})

func init() {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Git commit analysis and integration with work items",
		"Analyze Git commits and link them to work items (backlog items, milestones, goals).",
		"",
		"This command analyzes Git commits to:",
		"  - Extract work item references from commit messages (ITEM-*, MIL-*, GOAL-*)",
		"  - Create code_reference objects for changed files",
		"  - Link commits to work items",
		"  - Enable queries like \"What code changes support goal X?\"",
	).
		ExcludeCommonFlags()

	helpBuilder.ApplyToCommand(gitCmd)
}

var gitAnalyzeCmd = clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemAnalyzeCommandBuilder(), &cobra.Command{
	Use:   "analyze [commit-range]",
	Short: "Analyze Git commits and link to work items",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runGitAnalyze,
})

var gitQueryCmd = clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemQueryCommandBuilder(), &cobra.Command{
	Use:   "query [work-item-id]",
	Short: "Query code changes for a work item",
	Args:  cobra.ExactArgs(1),
	RunE:  runGitQuery,
})

var gitCmdInitOnce sync.Once

// NewGitCmd creates a new git command
func NewGitCmd() *cobra.Command {
	// sync.Once: parallel tests (e.g. TestHelpOutputFormats + TestFlagParity) may call NewGitCmd
	// concurrently; len(gitCmd.Commands())==0 is not a safe guard under race.
	gitCmdInitOnce.Do(func() {
		// Set Long descriptions with dynamic CLI command name using HelpBuilder
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
			AddExample("Find commits for backlog item", "%s system git query ITEM-625").
			AddExample("Find commits for goal", "%s system git query GOAL-6369").
			AddExample("Find commits for milestone", "%s system git query MIL-036").
			ExcludeCommonFlags()

		analyzeHelpBuilder.ApplyToCommand(gitAnalyzeCmd)
		queryHelpBuilder.ApplyToCommand(gitQueryCmd)

		gitCmd.AddCommand(gitAnalyzeCmd)
		gitCmd.AddCommand(gitQueryCmd)

		// Idempotent flag registration so parallel tests (e.g. TestHelpOutputFormats) don't panic "flag redefined"
		if gitAnalyzeCmd.Flags().Lookup("hash") == nil {
			gitAnalyzeCmd.Flags().String("hash", "", "Analyze specific commit hash")
		}
		if gitAnalyzeCmd.Flags().Lookup("recent") == nil {
			gitAnalyzeCmd.Flags().Int("recent", 0, "Analyze last N commits")
		}
		if gitAnalyzeCmd.Flags().Lookup("link") == nil {
			gitAnalyzeCmd.Flags().Bool("link", true, "Link commits to work items")
		}

		cli.AddCommonFlags(gitCmd)
		cli.AddCommonFlags(gitAnalyzeCmd)
		cli.AddCommonFlags(gitQueryCmd)
	})

	return gitCmd
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
		if refs, ok := workItem[objects.FieldKeyCommitRefs].([]any); ok {
			for _, ref := range refs {
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
