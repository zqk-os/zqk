package quick

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// NewQuickCmd creates the quick command group for one-click object creation from text or files.
func NewQuickCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Quick create system objects from text or files",
		"Create backlog items, decisions, questions, and other objects from markdown, text, or inline content.",
		"",
		"Use --file to read from .md/.txt, or --content/--title for inline input.",
		"Title is taken from the first # line or first line; the rest becomes description/context.",
	).
		AddExample("Backlog item from file", "%s quick backlog-item --file=notes.md").
		AddExample("Decision from content", "%s quick decision --content=\"Use Redis for cache.\n\nRationale: ...\"").
		AddExample("Question with answer", "%s quick question --content=\"How do we deploy?\" --answer=\"Via CI pipeline.\"")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewQuickCommandBuilder(), &cobra.Command{
		Use: "quick",
	})
	helpBuilder.ApplyToCommand(cmd)

	cmd.AddCommand(NewQuickBacklogItemCmd())
	cmd.AddCommand(NewQuickDecisionCmd())
	cmd.AddCommand(NewQuickQuestionCmd())
	cmd.AddCommand(NewQuickBundleCmd())

	return cmd
}
