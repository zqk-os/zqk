package quick

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// NewQuickBundleCmd creates the quick bundle command from the CLI package builder.
func NewQuickBundleCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewQuickBundleCommandBuilder()
	cmd.RunE = cli.WithProcessor(runQuickBundle)
	return cmd
}

func runQuickBundle(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
	data, err := cli.ReadRequiredFileFlag(cmd, "file")
	if err != nil {
		return err
	}

	creator := func(kind string, objData map[string]any) (string, error) {
		opCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), "", kind, "")

		// Use native kernel to create object
		if err := proc.Storage().Create(opCtx, proc.SecurityContext(), objData); err != nil {
			return "", err
		}

		id, _ := objData[objects.FieldKeyID].(string)
		return id, nil
	}

	return clipkg.ProcessQuickBundle(data, creator, cmd.OutOrStdout(), proc.Logger())
}
