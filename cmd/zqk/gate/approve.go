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

// NewApproveCmd creates the 'zqk gate approve' command.
func NewApproveCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Approve a staged mutation request",
		"Validates existence and payload digest, marks status approved with reviewer and timestamp, records approval entry to audit ledger.",
		"",
		"Ensures the staged payload has not been tampered with since staging by checking content SHA256 digest before marking as approved.",
	).
		AddExample("Approve staged request", paths.CLIUsage("gate", "approve", "APR-1730000000-abcd1234", "--reviewer", "operator-alice"))

	cmd := &cobra.Command{
		Use:   "approve <id>",
		Short: "Approve a staged mutation request",
		Long:  "Validates existence and payload digest, marks status approved with reviewer and timestamp, records approval entry to audit ledger.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			approvalID := args[0]

			var flagsBag clipkg.FlagBag
			reviewer := flagsBag.String(cmd, "reviewer")
			jsonOutput := flagsBag.Bool(cmd, "json")

			if flagsBag.Err() != nil {
				return flagsBag.Err()
			}
			if reviewer == "" {
				return errfmt.Errorf("--reviewer is required")
			}

			proc, err := cli.NewProcessor(cmd)
			if err != nil {
				return errfmt.Newf("failed to initialize processor: %v", err)
			}
			store := proc.Storage()
			if store == nil {
				return errfmt.Errorf("storage provider unavailable")
			}

			req, err := gate.Approve(proc.OperationContext(), store, proc.ProjectRoot(), approvalID, reviewer)
			if err != nil {
				return err
			}

			if jsonOutput {
				out, err := json.MarshalIndent(req, "", "  ")
				if err != nil {
					return errfmt.Newf("failed to format json: %v", err)
				}
				cmd.Println(string(out))
				return nil
			}

			cmd.Println("Approval successfully granted and recorded to audit ledger.")
			cmd.Printf("  ID:           %s\n", req.ID)
			cmd.Printf("  Status:       %s\n", req.Status)
			cmd.Printf("  Reviewer:     %s\n", req.Reviewer)
			cmd.Printf("  Reviewed At:  %s\n", req.ReviewedAt)
			cmd.Printf("  Payload Hash: %s\n", req.PayloadHash)

			return nil
		},
	}

	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("reviewer", "", "Reviewer identity (required)")
	cmd.Flags().Bool("json", false, "Output result in JSON format")

	_ = cmd.MarkFlagRequired("reviewer")

	return cmd
}
