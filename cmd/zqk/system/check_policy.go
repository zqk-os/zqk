package system

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/systemcheck/policy"
)

// NewCheckPolicyCmd creates a new check-policy command to run repo governance gates.
func NewCheckPolicyCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemCheckPolicyCommandBuilder()
	cmd.Aliases = []string{"policy-check", "check-repo-policy"}
	cmd.RunE = cli.WithProcessor(runCheckPolicy)
	return cmd
}

func runCheckPolicy(cmd *cobra.Command, args []string, proc *cli.Processor) error {
	var flags clipkg.FlagBag
	all := flags.Bool(cmd, "all")
	files := flags.StringArray(cmd, "files")
	scope := flags.String(cmd, "scope")
	if err := flags.Err(); err != nil {
		return errfmt.Newf("check-policy").Wrap(err)
	}

	gates := args
	if all || len(gates) == 0 {
		gates = []string{"doc-links", "secrets", "storage-boundaries", "goroutines", "field-keys", "project-nesting"}
	}

	opts := policy.RunOptions{
		ProjectRoot: proc.ProjectRoot(),
		Files:       files,
		Scope:       scope,
	}

	results, passed := policy.RunGates(cmd.Context(), opts, gates...)

	for _, res := range results {
		if res.Passed {
			fmt.Fprintf(cmd.OutOrStdout(), "✅ PASS: %s - %s\n", res.GateName, res.Message)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "❌ FAIL: %s - %s\n", res.GateName, res.Message)
			for _, v := range res.Violations {
				fmt.Fprintf(cmd.OutOrStdout(), "   • %s\n", v)
			}
		}
	}

	if !passed {
		return errfmt.Errorf("one or more policy gates failed")
	}
	return nil
}
