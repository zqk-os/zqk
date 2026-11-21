package gate

import (
	"encoding/json"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewGateCmd creates the 'zqk gate' command group for cryptographic human-in-the-loop mutation approval gates.
func NewGateCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Cryptographic human-in-the-loop mutation approval gate",
		"Cryptographic Human-in-the-Loop Mutation Approval Gate for staging, reviewing, approving, and rejecting critical mutation requests with SHA256 digest verification and audit ledger attestation.",
		"",
		"Ensures zero unauthorized or tampered mutations occur in the Knowledge Kernel.",
		"All approvals verify cryptographic content digests against stored CAS states and record immutable audit ledger entries.",
	).
		AddExample("Stage an approval request", paths.CLIUsage("gate", "stage", "--type", "mutation", "--target", "system", "--payload", "changes.json")).
		AddExample("Review a staged approval request", paths.CLIUsage("gate", "review", "APR-1730000000-abcd1234")).
		AddExample("Approve a staged mutation", paths.CLIUsage("gate", "approve", "APR-1730000000-abcd1234", "--reviewer", "operator-alice")).
		AddExample("Reject an approval request", paths.CLIUsage("gate", "reject", "APR-1730000000-abcd1234", "--reviewer", "operator-alice", "--reason", "Security concern"))

	cmd := &cobra.Command{
		Use:   "gate",
		Short: "Cryptographic human-in-the-loop mutation approval gate",
		Long:  "Cryptographic Human-in-the-Loop Mutation Approval Gate for staging, reviewing, approving, and rejecting critical mutations.",
	}

	helpBuilder.ApplyToCommand(cmd)

	cmd.AddCommand(NewStageCmd())
	cmd.AddCommand(NewReviewCmd())
	cmd.AddCommand(NewApproveCmd())
	cmd.AddCommand(NewRejectCmd())
	cmd.AddCommand(NewListCmd())

	cli.RequireSession(cmd, false)
	cli.RequireStorage(cmd, true)

	return cmd
}

func openGateStore(cmd *cobra.Command) (*cli.Processor, storage.ObjectStorageProvider, error) {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return nil, nil, errfmt.Newf("failed to initialize processor: %v", err)
	}
	store := proc.Storage()
	if store == nil {
		return nil, nil, errfmt.Errorf("storage provider unavailable")
	}
	return proc, store, nil
}

type gateContext struct {
	proc       *cli.Processor
	store      storage.ObjectStorageProvider
	jsonOutput bool
}

func openGateContext(cmd *cobra.Command, flagsBag *clipkg.FlagBag) (gateContext, error) {
	jsonOutput := flagsBag.Bool(cmd, "json")
	if flagsBag.Err() != nil {
		return gateContext{}, flagsBag.Err()
	}
	proc, store, err := openGateStore(cmd)
	if err != nil {
		return gateContext{}, err
	}
	return gateContext{
		proc:       proc,
		store:      store,
		jsonOutput: jsonOutput,
	}, nil
}

func parseReviewerFlags(cmd *cobra.Command, flagsBag *clipkg.FlagBag) (string, bool, error) {
	reviewer := flagsBag.String(cmd, "reviewer")
	jsonOutput := flagsBag.Bool(cmd, "json")
	if flagsBag.Err() != nil {
		return "", false, flagsBag.Err()
	}
	if reviewer == "" {
		return "", false, errfmt.Errorf("--reviewer is required")
	}
	return reviewer, jsonOutput, nil
}

func printGateJSON(cmd *cobra.Command, v any) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to format json: %v", err)
	}
	cmd.Println(string(out))
	return nil
}
