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

// NewRejectCmd creates the 'zqk gate reject' command.
func NewRejectCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Reject a staged approval request",
		"Rejects the staged approval request, marking status rejected with reviewer, timestamp, and reason, recording to audit ledger.",
		"",
		"Prevents any mutation from proceeding under this approval request ID.",
	).
		AddExample("Reject staged request", paths.CLIUsage("gate", "reject", "APR-1730000000-abcd1234", "--reviewer", "operator-alice", "--reason", "Payload missing invariant checks"))

	cmd := &cobra.Command{
		Use:   "reject <id>",
		Short: "Reject a staged approval request",
		Long:  "Rejects the staged approval request, marking status rejected with reviewer, timestamp, and reason, recording to audit ledger.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			approvalID := args[0]

			var flagsBag clipkg.FlagBag
			reviewer := flagsBag.String(cmd, "reviewer")
			reason := flagsBag.String(cmd, "reason")
			jsonOutput := flagsBag.Bool(cmd, "json")

			if flagsBag.Err() != nil {
				return flagsBag.Err()
			}
			if reviewer == "" {
				return errfmt.Errorf("--reviewer is required")
			}
			if reason == "" {
				return errfmt.Errorf("--reason is required")
			}

			proc, err := cli.NewProcessor(cmd)
			if err != nil {
				return errfmt.Newf("failed to initialize processor: %v", err)
			}
			store := proc.Storage()
			if store == nil {
				return errfmt.Errorf("storage provider unavailable")
			}

			req, err := gate.Reject(proc.OperationContext(), store, proc.ProjectRoot(), approvalID, reviewer, reason)
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

			cmd.Println("Approval request rejected and recorded to audit ledger.")
			cmd.Printf("  ID:          %s\n", req.ID)
			cmd.Printf("  Status:      %s\n", req.Status)
			cmd.Printf("  Reviewer:    %s\n", req.Reviewer)
			cmd.Printf("  Reviewed At: %s\n", req.ReviewedAt)
			cmd.Printf("  Reason:      %s\n", req.Reason)

			return nil
		},
	}

	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("reviewer", "", "Reviewer identity (required)")
	cmd.Flags().String("reason", "", "Rejection rationale (required)")
	cmd.Flags().Bool("json", false, "Output result in JSON format")

	_ = cmd.MarkFlagRequired("reviewer")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}
