// Package test is the dest-owned verification surface (dashboard first).
// Studio cmd/zqk/test is MUST_NOT on this SKU.
// TRACK: BLI-1789702449225534000-eca4a6bd
package test

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewTestCmd returns test dashboard for this SKU.
func NewTestCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewTestCommandBuilder()
	cli.RequireSession(cmd, false)
	cmd.AddCommand(newDashboardCmd())
	return cmd
}

func newDashboardCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewTestDashboardCommandBuilder()
	cli.RequireSession(cmd, false)
	cmd.RunE = runDashboard
	return cmd
}
