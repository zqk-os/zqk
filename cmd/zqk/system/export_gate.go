package system

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/community"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// NewExportGateCmd creates a new export-gate command to audit open-core community trees.
func NewExportGateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemExportGateCommandBuilder()
	cmd.Aliases = []string{"police-export", "verify-export"}
	cmd.RunE = cli.WithProcessor(runExportGate)
	return cmd
}

func runExportGate(cmd *cobra.Command, args []string, proc *cli.Processor) error {
	var flags clipkg.FlagBag
	target := flags.String(cmd, "target")
	prohibited := flags.StringArray(cmd, "prohibited-pattern")
	if err := flags.Err(); err != nil {
		return errfmt.Newf("export-gate").Wrap(err)
	}

	if target == "" || target == "." {
		if proc != nil && proc.ProjectRoot() != "" {
			target = proc.ProjectRoot()
		} else {
			target = "."
		}
	}

	res, err := community.RunExportGate(target, prohibited)
	if err != nil {
		return errfmt.Newf("export-gate audit failed").Wrap(err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Audited %d files in %s\n", res.FilesAudited, res.TargetDirectory)
	if res.Passed {
		fmt.Fprintln(cmd.OutOrStdout(), "✅ PASS: Open-core export gate clean. Zero prohibited patterns or extensions detected.")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "❌ FAIL: Open-core export gate detected %d prohibited item(s):\n", len(res.Violations))
	for _, v := range res.Violations {
		fmt.Fprintf(cmd.OutOrStdout(), "   • [%s] %s: %s\n", v.Rule, v.Path, v.Message)
	}

	return errfmt.Errorf("open-core export gate violations detected")
}
