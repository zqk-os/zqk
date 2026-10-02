package mesh

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewAdvertiseCmd creates the mesh advertise command
func NewAdvertiseCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewMeshAdvertiseCommandBuilder()
	cmd.RunE = runAdvertise
	return cmd
}

func runAdvertise(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		resourceType := args[0]
		amount, _ := cmd.Flags().GetFloat64("amount")
		units, _ := cmd.Flags().GetString("units")
		resourceID, _ := cmd.Flags().GetString("resource-id")

		// Resolve local identity
		kernelID, err := resolveLocalKernelID(proc.ProjectRoot())
		if err != nil {
			return err
		}

		// Create advertisement object
		obj := map[string]any{
			objects.FieldKeyKind:              objects.KindCapacityAdvertisement,
			objects.FieldKeyTitle:             fmt.Sprintf("Offer: %s (%.1f %s)", resourceType, amount, units),
			objects.FieldKeyProviderKernelRef: kernelID,
			objects.FieldKeyResourceType:      resourceType,
			objects.FieldKeyQuantity:          amount,
			objects.FieldKeyUnits:             units,
			objects.FieldKeyStatus:            objects.ObjectStatusImplemented,
		}

		if resourceID != "" {
			obj[objects.FieldKeyResourceID] = resourceID
		}

		// Use synchronous creation for immediate visibility
		syncCtx := storage.WithSyncCreateForKind(proc.OperationContext(), objects.KindCapacityAdvertisement)
		if err := proc.Storage().Create(syncCtx, proc.SecurityContext(), obj); err != nil {
			return errfmt.Newf("failed to create advertisement").Wrap(err)
		}

		if err := cli.WriteOutput(cmd, []byte(fmt.Sprintf("✅ Broadcasting %s capacity: %.1f %s\n", resourceType, amount, units))); err != nil {
			return err
		}
		return nil
	})(cmd, args)
}
