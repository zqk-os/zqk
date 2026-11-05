package gate

import (
	"encoding/json"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/gate"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewListCmd creates the 'zqk gate list' command.
func NewListCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"List approval requests",
		"Lists staged, approved, or rejected approval requests in the Knowledge Kernel CAS.",
		"",
		"Enables operators to inspect outstanding gate approval requests awaiting human review.",
	).
		AddExample("List all requests", paths.CLIUsage("gate", "list")).
		AddExample("List staged requests only", paths.CLIUsage("gate", "list", "--status", "staged"))

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List approval requests",
		Long:  "Lists staged, approved, or rejected approval requests in the Knowledge Kernel CAS.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var flagsBag clipkg.FlagBag
			statusFilter := flagsBag.String(cmd, "status")
			jsonOutput := flagsBag.Bool(cmd, "json")

			if flagsBag.Err() != nil {
				return flagsBag.Err()
			}

			proc, err := cli.NewProcessor(cmd)
			if err != nil {
				return errfmt.Newf("failed to initialize processor: %v", err)
			}
			store := proc.Storage()
			if store == nil {
				return errfmt.Errorf("storage provider unavailable")
			}

			list, err := gate.ListApprovals(proc.OperationContext(), store, statusFilter)
			if err != nil {
				return err
			}

			if jsonOutput {
				out, err := json.MarshalIndent(list, "", "  ")
				if err != nil {
					return errfmt.Newf("failed to format json: %v", err)
				}
				cmd.Println(string(out))
				return nil
			}

			if len(list) == 0 {
				cmd.Println("No approval requests found.")
				return nil
			}

			cmd.Printf("%-24s  %-12s  %-16s  %-10s  %-20s\n", "ID", "TYPE", "TARGET", "STATUS", "CREATED_AT")
			for _, item := range list {
				cmd.Printf("%-24s  %-12s  %-16s  %-10s  %-20s\n", item.ID, item.Type, item.Target, item.Status, item.CreatedAt)
			}

			return nil
		},
	}

	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("status", "", "Filter by approval status (staged, approved, rejected)")
	cmd.Flags().Bool("json", false, "Output result in JSON format")

	return cmd
}
