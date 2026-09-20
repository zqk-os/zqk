package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewQuickQuestionCommandBuilder creates a new quick_question command
func NewQuickQuestionCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("question")
	builder.WithShort("Create a question from a file or content (optionally with answer)")
	help := clipkg.DynamicHelpBuilder("Create a question from a file or content (optionally with answer)")
	help.WithDescriptionLines("Reads .md/.txt (or --content). First # or first line = title; rest = context.")
	help.WithDescriptionLines("Use --answer to capture an answer (e.g. Q&A). Creates via object create.")
	help.AddExample("Create question with answer", "%s quick question --content=\"How do we deploy?\" --answer=\"Via CI pipeline.\"")
	help.AddExample("Create from file", "%s quick question --file=open-questions.txt")
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
	builder.AddStringFlag("answer", "", "", "Answer (for Q&A capture)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
