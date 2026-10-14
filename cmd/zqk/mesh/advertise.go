package mesh

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
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
		var flags clipkg.FlagBag
		amountStr := flags.String(cmd, "amount")
		units := flags.String(cmd, "units")
		resourceID := flags.String(cmd, "resource-id")
		if err := flags.Err(); err != nil {
			return err
		}

		var amount float64
		if amountStr != "" {
			var err error
			amount, err = strconv.ParseFloat(amountStr, 64)
			if err != nil {
				return errfmt.Newf("invalid amount %q", amountStr).Wrap(err)
			}
		}

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

		return cli.WriteOutput(cmd, []byte(fmt.Sprintf("✅ Broadcasting %s capacity: %.1f %s\n", resourceType, amount, units)))
	})(cmd, args)
}
