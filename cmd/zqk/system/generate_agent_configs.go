package system

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/agentonboard"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewGenerateAgentConfigsCmd writes regenerable vendor directive files from the kernel boot payload.
// Prefer system agent-onboard for the full detect→seat→prime sequence (community binary).
func NewGenerateAgentConfigsCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemGenerateAgentConfigsCommandBuilder()
	cli.RequireSession(cmd, false)
	cli.RequireStorage(cmd, false)
	cmd.RunE = runGenerateAgentConfigs
	return cmd
}

func runGenerateAgentConfigs(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		root := proc.ProjectRoot()
		if root == emptyValue {
			return errfmt.Errorf("project root is required")
		}
		allVendors, _ := cmd.Flags().GetBool("all-vendors")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		force, _ := cmd.Flags().GetBool("force")

		detected := agentonboard.DetectVendors(root)
		vendors := agentonboard.VendorsToPrime(detected, allVendors)
		wr, err := agentonboard.WriteVendorConfigs(root, vendors, dryRun, force)
		if err != nil {
			return errfmt.Newf("generate-agent-configs").Wrap(err)
		}
		return cli.FormatOutput(cmd, map[string]any{
			objects.FieldKeyStatus:  "success",
			"dry_run":               dryRun,
			"force":                 force,
			"files":                 wr.Written,
			objects.FieldKeySkipped: wr.Skipped,
			"detected":              detected,
			objects.FieldKeyNote:    "Prefer " + brand.ExecutableName() + " system agent-onboard for seating + sync report",
		})
	})(cmd, nil)
}
