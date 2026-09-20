package keystore

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewKeystoreCmd creates a new keystore command group
func NewKeystoreCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Keystore operations (create, issue, list, rotate keys)",
		"Keystore operations for managing authentication keys in the local vault.",
		"",
		"The keystore stores secure credential entries (passwords, tokens, API keys) with",
		"access restrictions. Only system can set credential hashes; users can create entries",
		"for their own account but must use the create/issue commands to properly hash credentials.",
		"Prefer keystore issue for agent seating (unique secret + fingerprint + seating file).",
	).
		AddExample("Issue a unique seat API key", "%s keystore issue --account-id ACC-… --title swarm_worker_1").
		AddExample("Create a new API key for your account", "%s keystore create --account-id ACC-… --key-type api_key --credential \"my-secret-key\"").
		AddExample("List your keystore entries", "%s keystore list").
		AddExample("Rotate an existing key", "%s keystore rotate KEY-001 --new-credential \"new-secret-key\"")

	keystoreCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewKeystoreCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "keystore",
	})

	helpBuilder.ApplyToCommand(keystoreCmd)

	keystoreCmd.AddCommand(NewCreateCmd())
	keystoreCmd.AddCommand(NewIssueCmd())
	keystoreCmd.AddCommand(NewListCmd())
	keystoreCmd.AddCommand(NewRotateCmd())

	return keystoreCmd
}
