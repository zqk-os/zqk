// Package kindpack installs a verified spec pack into the kernel record.
package kindpack

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/packrecord"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewKindPackCmd installs an uploaded spec pack so its kinds load as objects.
func NewKindPackCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewKindPackCommandBuilder()
	cmd.AddCommand(newInstallCmd())
	return cmd
}

func newInstallCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewKindPackInstallCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		root := paths.ResolveProjectRoot(".")
		recorded, err := packrecord.Install(args[0], root)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "recorded %s (%s)\n", recorded.Name, joinKinds(recorded.Kinds))
		return err
	}
	return cmd
}

func joinKinds(kinds []string) string {
	out := ""
	for i, kind := range kinds {
		if i > 0 {
			out += ","
		}
		out += kind
	}
	return out
}
