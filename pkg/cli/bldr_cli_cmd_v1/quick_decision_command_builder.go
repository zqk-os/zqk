package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewQuickDecisionCommandBuilder creates a new quick_decision command
func NewQuickDecisionCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("decision")
	builder.WithShort("Create a decision (or ADR) from a file or content")
	help := clipkg.DynamicHelpBuilder("Create a decision (or ADR) from a file or content")
	help.WithDescriptionLines("Reads .md/.txt (or --content). First # line or first line = title; rest = context.")
	help.WithDescriptionLines("Creates via object create. Use --file, --content, or --title.")
	help.AddExample("Create from file", "%s quick decision --file=adr-001.md")
	help.AddExample("Create from content", "%s quick decision --content=\"Use Redis for cache.\n\nRationale: ...\"")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("file", "", "", "Path to .md or .txt file")
	builder.AddStringFlag("content", "", "", "Inline content (first line or # = title, rest = context)")
	builder.AddStringFlag("title", "", "", "Title (overrides first line when set)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
