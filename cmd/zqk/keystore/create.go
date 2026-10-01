package keystore

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

const emptyValue = ""

// NewCreateCmd creates a new keystore create command
func NewCreateCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Create a new keystore entry",
		"Create a new keystore entry for storing credentials.",
		"",
		"The credential will be automatically hashed based on the key type:",
		"  - password: bcrypt hash",
		"  - api_key, oauth_token, personal_access_token: SHA256 hash",
		"",
		"The entry will be associated with your account (from security context) unless",
		"--account-id is explicitly provided (admin only).",
	).
		AddExample("Create an API key (credential will be SHA256 hashed)", "%s keystore create --key-type api_key --credential \"my-secret-key\" --title \"Production API Key\"").
		AddExample("Create a password (credential will be bcrypt hashed)", "%s keystore create --key-type password --credential \"my-password\" --title \"Database Password\"").
		AddExample("Create with expiration date", "%s keystore create --key-type api_key --credential \"key\" --title \"Temporary Key\" --expires-at \"2024-12-31T23:59:59Z\"").
		ExcludeCommonFlags()

	createCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewKeystoreCreateCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use:  "create [flags]",
		RunE: runCreate,
	})

	helpBuilder.ApplyToCommand(createCmd)

	// Add common flags
	cli.AddCommonFlags(createCmd)

	// Add create-specific flags
	createCmd.Flags().String("account-id", "", "Account ID this key belongs to (default: current user, admin only to set other accounts)")
	createCmd.Flags().String("key-type", "api_key", "Type of key: password, api_key, oauth_token, personal_access_token")
	createCmd.Flags().String("credential", "", "The credential to store (will be hashed automatically)")
	createCmd.Flags().String("title", "", "Title/description for this key (required)")
	createCmd.Flags().String("description", "", "Additional description")
	createCmd.Flags().String("expires-at", "", "Expiration date (ISO-8601 format, e.g., 2024-12-31T23:59:59Z)")

	// Mark required flags
	_ = createCmd.MarkFlagRequired("credential") //nolint:errcheck // Flag validation errors are handled by cobra
	_ = createCmd.MarkFlagRequired("title")      //nolint:errcheck // Flag validation errors are handled by cobra

	return createCmd
}

func runCreate(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		proc.Logger().LogDebug("Creating keystore entry")

		// Parse flags
		flags := parseCreateFlags(cmd)

		// Validate key type
		if err := validateKeyType(flags.KeyType); err != nil {
			return err
		}

		// Get security context
		secCtx := proc.SecurityContext()

		// Determine account ID
		accountID, err := determineAccountID(flags, secCtx)
		if err != nil {
			return err
		}

		// Hash the credential
		credentialHash, err := hashCredentialForCreate(flags.Credential, flags.KeyType)
		if err != nil {
			return err
		}

		// Build keystore entry (salt is always empty for both password and token hashing)
		entry := buildKeystoreEntry(flags, accountID, credentialHash, "")

		// Create the entry
		id, err := createKeystoreEntry(proc, entry)
		if err != nil {
			return err
		}

		// Build result
		result := buildCreateResult(id, accountID, flags.KeyType, flags.Title, flags.ExpiresAt)

		switch cli.GetFormat(cmd) {
		case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
			return cli.FormatOutput(cmd, result)
		default:
			return cli.WriteOutput(cmd, formatCreateOutputText(flags, id, accountID))
		}
	})(cmd, args)
}
