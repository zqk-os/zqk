package bldr_cli_cmd_v1

import (
	"os"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewRulesCommandBuilder creates the parent 'rules' command group matching .zqk/cli/specs/rules_command.yaml.
func NewRulesCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("rules")
	builder.WithShort("Multi-host IDE directives generator for AI developer harnesses")
	help := clipkg.DynamicHelpBuilder("Multi-host IDE directives generator for AI developer harnesses")
	help.WithDescriptionLines("Generate persistent memory directives and rules files for Cursor (.cursorrules),")
	help.WithDescriptionLines("Claude Code (CLAUDE.md), Windsurf (.windsurfrules), and headless agent runtimes (.agents/AGENTS.md).")
	builder.WithHelpBuilder(help)
	cmd := builder.Build()
	cmd.AddCommand(NewRulesGenerateCommandBuilder())
	return cmd
}

// NewRulesCommand creates the parent 'rules' command group.
func NewRulesCommand() *cobra.Command {
	return NewRulesCommandBuilder()
}

// NewRulesGenerateCommandBuilder creates the 'rules generate' command.
func NewRulesGenerateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("generate")
	builder.WithShort("Generate multi-host IDE directive and rules files")
	help := clipkg.DynamicHelpBuilder("Generate multi-host IDE directive and rules files")
	help.WithDescriptionLines("Generates syntax-valid, token-conserving IDE instructions for Cursor (.cursorrules),")
	help.WithDescriptionLines("Claude Code (CLAUDE.md), Windsurf (.windsurfrules), and Antigravity/universal agents (.agents/AGENTS.md)")
	help.WithDescriptionLines("embedding sub-15ms zgrep AST search queries and persistent Knowledge Kernel rules.")
	help.AddExample("Generate rules for all supported hosts", "%s rules generate")
	help.AddExample("Generate rules for Cursor and Claude only", "%s rules generate --host cursor,claude")
	help.AddExample("Dry-run preview of rules generation", "%s rules generate --dry-run")
	help.AddExample("Force overwrite of existing rules", "%s rules generate --force")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.NoArgs)
	builder.AddStringFlag("host", "", "all", "Target host IDE or agents (cursor, claude, windsurf, agents, all)")
	builder.AddBoolFlag("dry-run", "", false, "Preview generated files without writing to disk")
	builder.AddBoolFlag("force", "", false, "Overwrite existing directive files")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"columns"})

	cmd := builder.Build()
	cli.RequireSession(cmd, false)
	cli.RequireStorage(cmd, false)

	cmd.RunE = RunRulesGenerate
	return cmd
}

// RunRulesGenerate executes the rules generate command.
func RunRulesGenerate(cmd *cobra.Command, _ []string) error {
	root, _ := cli.ResolveCommandProjectRoot(cmd)
	if root == "" {
		root = paths.FindNearestProjectRoot(".")
	}
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	hostFlag, _ := cmd.Flags().GetString("host")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")

	opts := RulesGenerateOptions{
		Hosts:  []string{hostFlag},
		DryRun: dryRun,
		Force:  force,
	}

	res, err := GenerateRules(root, opts)
	if err != nil {
		return err
	}

	if dryRun {
		cmd.Printf("DRY-RUN: Previewing multi-host rules generation in %s\n", root)
		for _, file := range res.Written {
			cmd.Printf("  [WOULD WRITE] %s\n", file)
		}
		for _, file := range res.Skipped {
			cmd.Printf("  [WOULD SKIP]  %s (already exists, use --force to overwrite)\n", file)
		}
	} else {
		cmd.Printf("Generated multi-host IDE rules in %s\n", root)
		for _, file := range res.Written {
			cmd.Printf("  ✓ Wrote %s\n", file)
		}
		for _, file := range res.Skipped {
			cmd.Printf("  ⚠️ Skipped %s (already exists, use --force to overwrite)\n", file)
		}
	}

	return cli.FormatOutput(cmd, map[string]any{
		objects.FieldKeyStatus: "success",
		"dry_run":              dryRun,
		"force":                force,
		"written":              res.Written,
		"skipped":              res.Skipped,
		"previews":             res.Previews,
	})
}
