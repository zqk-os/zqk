package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewRunCommandBuilder creates a new run command
func NewRunCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("run")
	builder.WithShort("Checkout committed SHA into local-ci workdir and scan-tests")
	help := clipkg.DynamicHelpBuilder("Checkout committed SHA into local-ci workdir and scan-tests")
	help.WithDescriptionLines("Runs scripts/local-ci-checkout.sh then scheduler scan-tests with the Local CI")
	help.WithDescriptionLines("source-root. Studio must be clean of tracked changes unless --allow-dirty or")
	help.WithDescriptionLines("--sha points at an already-committed revision.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Does not delete or reset in-flight SCH-run jobs unless scan-tests updates")
	help.WithDescriptionLines("matching stable IDs (prefer waiting for an idle wave before full --all).")
	help.AddExample("Full local CI on HEAD", "%s ci run --all")
	help.AddExample("Targeted package after a fix commit", "%s ci run --package ./pkg/storage")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("sha", "", "", "Commit SHA to checkout (default: HEAD)")
	builder.AddBoolFlag("allow-dirty", "", false, "Allow checkout when studio has tracked dirty files")
	builder.AddBoolFlag("no-archive", "", false, "Skip tar drop / archive-git recording")
	builder.AddStringFlag("package", "", "", "Pass-through to scan-tests --package (omit with --all)")
	builder.AddBoolFlag("all", "", false, "Pass-through to scan-tests --all (default when no --package)")
	builder.AddBoolFlag("checkout-only", "", false, "Only checkout; do not schedule tests")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
