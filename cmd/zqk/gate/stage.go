package gate

import (
	"encoding/json"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/gate"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewStageCmd creates the 'zqk gate stage' command.
func NewStageCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Stage an approval request in CAS",
		"Stages an approval request in CAS with content SHA256 digest, recording payload hash and target.",
		"",
		"Reads the payload from the specified file, calculates its SHA256 digest, and persists the staged approval request in the Knowledge Kernel CAS.",
	).
		AddExample("Stage a schema mutation", paths.CLIUsage("gate", "stage", "--type", "mutation", "--target", "system", "--payload", "./patch.json"))

	cmd := &cobra.Command{
		Use:   "stage",
		Short: "Stage an approval request in CAS",
		Long:  "Stages an approval request in CAS with content SHA256 digest, recording payload hash and target.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var flagsBag clipkg.FlagBag
			reqType := flagsBag.String(cmd, "type")
			target := flagsBag.String(cmd, "target")
			payloadPath := flagsBag.String(cmd, "payload")
			createdBy := flagsBag.String(cmd, "created-by")
			jsonOutput := flagsBag.Bool(cmd, "json")

			if flagsBag.Err() != nil {
				return flagsBag.Err()
			}
			if reqType == "" {
				return errfmt.Errorf("--type is required")
			}
			if target == "" {
				return errfmt.Errorf("--target is required")
			}
			if payloadPath == "" {
				return errfmt.Errorf("--payload file path is required")
			}

			payloadBytes, err := fileutil.ReadFile(payloadPath)
			if err != nil {
				return errfmt.Newf("failed to read payload file %s: %v", payloadPath, err)
			}
			if len(payloadBytes) == 0 {
				return errfmt.Errorf("payload file cannot be empty")
			}

			proc, err := cli.NewProcessor(cmd)
			if err != nil {
				return errfmt.Newf("failed to initialize processor: %v", err)
			}
			store := proc.Storage()
			if store == nil {
				return errfmt.Errorf("storage provider unavailable")
			}

			if createdBy == "" {
				if proc.SecurityContext() != nil && proc.SecurityContext().AccountID != "" {
					createdBy = proc.SecurityContext().AccountID
				} else {
					createdBy = "operator"
				}
			}

			req, err := gate.StageApproval(proc.OperationContext(), store, reqType, target, payloadBytes, createdBy)
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

			cmd.Println("Approval request staged successfully.")
			cmd.Printf("  ID:           %s\n", req.ID)
			cmd.Printf("  Type:         %s\n", req.Type)
			cmd.Printf("  Target:       %s\n", req.Target)
			cmd.Printf("  Payload Hash: %s\n", req.PayloadHash)
			cmd.Printf("  Status:       %s\n", req.Status)
			cmd.Printf("  Created At:   %s\n", req.CreatedAt)
			cmd.Printf("\nTo review:  %s\n", paths.CLIUsage("gate", "review", req.ID))
			cmd.Printf("To approve: %s\n", paths.CLIUsage("gate", "approve", req.ID, "--reviewer", "<name>"))
			cmd.Printf("To reject:  %s\n", paths.CLIUsage("gate", "reject", req.ID, "--reviewer", "<name>", "--reason", "<reason>"))

			return nil
		},
	}

	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("type", "", "Type of approval request (required)")
	cmd.Flags().String("target", "", "Target or recipient of the mutation (required)")
	cmd.Flags().String("payload", "", "Path to payload file (required)")
	cmd.Flags().String("created-by", "", "Optional creator identity")
	cmd.Flags().Bool("json", false, "Output result in JSON format")

	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("target")
	_ = cmd.MarkFlagRequired("payload")

	return cmd
}
