package mesh

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewMarketCmd creates the mesh market command
func NewMarketCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewMeshMarketCommandBuilder()
	cmd.RunE = runMarket
	return cmd
}

func runMarket(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		resourceType, _ := cmd.Flags().GetString("type")

		// Query advertisements
		filter := storage.ListFilter{Kind: "capacity_advertisement", Limit: 0}
		if resourceType != "" {
			filter.Filters = map[string]any{objects.FieldKeyResourceType: resourceType}
		}

		result, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), proc.StorageContext(), filter)
		if err != nil {
			return errfmt.Newf("failed to list market").Wrap(err)
		}

		if len(result.Objects) == 0 {
			_ = cli.WriteOutput(cmd, []byte("No resources currently advertised in the mesh market.\n"))
			return nil
		}

		cyan := color.New(color.FgCyan).SprintFunc()
		yellow := color.New(color.FgYellow).SprintFunc()
		bold := color.New(color.Bold).SprintFunc()

		out := "\n" + bold("SOVEREIGN MESH MARKETPLACE") + "\n"
		out += "============================================================\n\n"

		for _, obj := range result.Objects {
			id, _ := obj[objects.FieldKeyID].(string)
			provider, _ := obj[objects.FieldKeyProviderKernelRef].(string)
			resType, _ := obj[objects.FieldKeyResourceType].(string)
			units, _ := obj[objects.FieldKeyUnits].(string)
			resID, _ := obj[objects.FieldKeyResourceID].(string)

			// Robustly handle quantity type (could be int or float64)
			qty := 0.0
			if val, ok := obj[objects.FieldKeyQuantity]; ok {
				switch v := val.(type) {
				case float64:
					qty = v
				case int:
					qty = float64(v)
				case int64:
					qty = float64(v)
				}
			}

			out += fmt.Sprintf("%s [%s]\n", bold(resType), cyan(id))
			out += fmt.Sprintf("  Provider: %s\n", yellow(provider))
			out += fmt.Sprintf("  Capacity: %.1f %s\n", qty, units)
			if resID != "" {
				out += fmt.Sprintf("  Resource: %s\n", resID)
			}
			out += "\n"
		}

		return cli.WriteOutput(cmd, []byte(out))
	})(cmd, args)
}
