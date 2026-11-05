package gate

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/gate"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewReviewCmd creates the 'zqk gate review' command.
func NewReviewCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Review a staged approval request",
		"Displays the staged approval request, showing id, type, target, hash, and payload preview.",
		"",
		"Inspects the staged approval request before granting or rejecting execution.",
	).
		AddExample("Review approval request", paths.CLIUsage("gate", "review", "APR-1730000000-abcd1234"))

	cmd := &cobra.Command{
		Use:   "review <id>",
		Short: "Review a staged approval request",
		Long:  "Displays the staged approval request, showing id, type, target, hash, and payload preview.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			approvalID := args[0]

			var flagsBag clipkg.FlagBag
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

			req, err := gate.GetApproval(proc.OperationContext(), store, approvalID)
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

			cmd.Println("================================================================================")
			cmd.Println("STAGED APPROVAL REQUEST REVIEW")
			cmd.Println("================================================================================")
			cmd.Printf("ID:           %s\n", req.ID)
			cmd.Printf("Type:         %s\n", req.Type)
			cmd.Printf("Target:       %s\n", req.Target)
			cmd.Printf("Hash:         %s\n", req.PayloadHash)
			cmd.Printf("Status:       %s\n", req.Status)
			cmd.Printf("Created By:   %s\n", req.CreatedBy)
			cmd.Printf("Created At:   %s\n", req.CreatedAt)

			if req.Reviewer != "" {
				cmd.Printf("Reviewer:     %s\n", req.Reviewer)
			}
			if req.ReviewedAt != "" {
				cmd.Printf("Reviewed At:  %s\n", req.ReviewedAt)
			}
			if req.Reason != "" {
				cmd.Printf("Reason:       %s\n", req.Reason)
			}

			cmd.Println("--------------------------------------------------------------------------------")
			cmd.Println("PAYLOAD PREVIEW:")
			cmd.Println("--------------------------------------------------------------------------------")

			preview := req.Payload
			if len(preview) > 1000 {
				preview = preview[:1000] + fmt.Sprintf("\n... [truncated, total length %d bytes]", len(req.Payload))
			}
			cmd.Println(preview)
			cmd.Println("================================================================================")

			if req.Status == gate.StatusStaged {
				cmd.Printf("\nTo approve: %s\n", paths.CLIUsage("gate", "approve", req.ID, "--reviewer", "<name>"))
				cmd.Printf("To reject:  %s\n", paths.CLIUsage("gate", "reject", req.ID, "--reviewer", "<name>", "--reason", "<reason>"))
			}

			return nil
		},
	}

	helpBuilder.ApplyToCommand(cmd)
	cmd.Flags().Bool("json", false, "Output result in JSON format")

	return cmd
}
