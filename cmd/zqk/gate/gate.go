package gate

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/paths"
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
