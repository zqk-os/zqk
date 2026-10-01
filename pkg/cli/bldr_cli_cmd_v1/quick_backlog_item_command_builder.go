package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewQuickBacklogItemCommandBuilder creates a new quick_backlog_item command
func NewQuickBacklogItemCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("backlog-item")
	builder.WithShort("Create a backlog item from a file or content")
	help := clipkg.DynamicHelpBuilder("Create a backlog item from a file or content")
	help.WithDescriptionLines("Reads .md/.txt (or --content). First # line or first line = title; rest = description.")
	help.WithDescriptionLines("Creates via object create. Use --file, --content, or --title.")
	help.AddExample("Create from file", "%s quick backlog-item --file=notes.md")
	help.AddExample("Create from inline content", "%s quick backlog-item --content=\"Add Redis cache\n\nWe need a shared cache.\"")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("file", "", "", "Path to .md or .txt file")
	builder.AddStringFlag("content", "", "", "Inline content (first line or # = title, rest = description)")
	builder.AddStringFlag("title", "", "", "Title (overrides first line when set)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
