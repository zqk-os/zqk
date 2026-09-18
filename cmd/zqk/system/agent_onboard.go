package system

import (
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentonboard"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/spf13/cobra"
)

// NewAgentOnboardCmd wires system agent-onboard (Vector A/B first-contact sync).
func NewAgentOnboardCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemAgentOnboardCommandBuilder()
	cli.RequireSession(cmd, false)
	cli.RequireStorage(cmd, false)
	cmd.RunE = runAgentOnboard
	return cmd
}

func runAgentOnboard(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		root := proc.ProjectRoot()
		if root == emptyValue {
			return errfmt.Errorf("project root not found")
		}
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		detectOnly, _ := cmd.Flags().GetBool("detect-only")
		skipSeat, _ := cmd.Flags().GetBool("skip-seat")
		skipPrime, _ := cmd.Flags().GetBool("skip-prime")
		allVendors, _ := cmd.Flags().GetBool("all-vendors")
		headless, _ := cmd.Flags().GetBool("headless")
		force, _ := cmd.Flags().GetBool("force")
		vendorsRaw, _ := cmd.Flags().GetString("vendors")

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		sec := proc.SecurityContext()
		sessionOK := sec != nil && sec.AccountID != ""

		res, err := agentonboard.Run(agentonboard.Options{
			ProjectRoot:  root,
			Logger:       logger,
			Seat:         SeedDefaultAgentSeatingPack,
			DryRun:       dryRun,
			DetectOnly:   detectOnly,
			SkipSeat:     skipSeat,
			SkipPrime:    skipPrime,
			AllVendors:   allVendors,
			Headless:     headless,
			Force:        force,
			VendorFilter: agentonboard.NormalizeVendorFilter(vendorsRaw),
			SessionOK:    sessionOK,
		})
		if err != nil {
			return errfmt.Newf("agent-onboard").Wrap(err)
		}
		if err := cli.FormatOutput(cmd, res); err != nil {
			return err
		}
		if res.Status == agentonboard.ResultBlocked || res.Status == agentonboard.ResultFailed {
			return errfmt.Errorf("agent-onboard %s", res.Status)
		}
		return nil
	})(cmd, nil)
}
