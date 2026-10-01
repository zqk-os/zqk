package keystore

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewRotateCmd creates a new keystore rotate command
func NewRotateCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Rotate or revoke a keystore entry",
		"Rotate or revoke a keystore entry.",
		"",
		"Rotation:",
		"  - Updates the credential_hash with a new hashed credential",
		"  - Only system can rotate (credential_hash can only be set by system)",
		"  - New credential will be hashed based on key type (bcrypt for passwords, SHA256 for tokens)",
		"",
		"Revocation:",
		"  - Marks the key as revoked",
		"  - Owner or admin can revoke keys",
		"  - Revoked keys cannot be used for authentication",
	).
		AddExample("Rotate a key with a new credential (system only)", "%s keystore rotate KEY-001 --new-credential \"new-secret-key\"").
		AddExample("Revoke a key (owner or admin)", "%s keystore rotate KEY-001 --revoke").
		AddExample("Rotate and revoke old key in one operation", "%s keystore rotate KEY-001 --new-credential \"new-key\" --revoke-old").
		ExcludeCommonFlags()

	rotateCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewKeystoreRotateCommandBuilder(), &cobra.Command{
		Use:  "rotate <key-id> [flags]",
		Args: cobra.ExactArgs(1),
		RunE: runRotate,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(rotateCmd)

	// Add common flags
	cli.AddCommonFlags(rotateCmd)

	// Add rotate-specific flags
	rotateCmd.Flags().String("new-credential", "", "New credential to set (will be hashed automatically)")
	rotateCmd.Flags().Bool("revoke", false, "Revoke the key (marks as revoked)")
	rotateCmd.Flags().Bool("revoke-old", false, "Revoke the old key when rotating (creates new entry and revokes old)")

	return rotateCmd
}

func runRotate(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		keyID := args[0]

		var err error
		_ = err

		logging.FluentEvent(proc.Logger()).Debug("Rotating keystore entry").KeyID(keyID).Log()

		// Parse flags
		flags, err := parseRotateFlags(cmd)
		if err != nil {
			return err
		}

		// Get security context
		secCtx := proc.SecurityContext()
		storageProvider := proc.Storage()
		opCtx := proc.OperationContext()

		// Read existing entry
		existing, err := storageProvider.Read(opCtx, secCtx, keyID)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to read keystore entry", err).KeyID(keyID).Log()
			return errfmt.Newf("failed to read keystore entry").Wrap(err)
		}

		// Validate entry
		if err := validateKeystoreEntry(existing, keyID); err != nil {
			return err
		}

		// Get key type and account ID
		keyType, _ := existing[objects.FieldKeyKeyType].(string)
		accountID, _ := existing[objects.FieldKeyAccountID].(string)

		// Check permissions for revocation
		if flags.Revoke || flags.RevokeOld {
			isAdmin, isOwner, err := checkPermissions(secCtx, accountID, false)
			if err != nil {
				return err
			}
			if !isAdmin && !isOwner {
				return errfmt.Errorf("permission denied: only owner, admin, or system can revoke keys")
			}
		}

		// Check permissions for rotation
		if flags.NewCredential != emptyValue {
			_, _, err := checkPermissions(secCtx, accountID, true)
			if err != nil {
				return err
			}
		}

		// Build all updates
		updates, err := buildAllUpdates(flags, keyType)
		if err != nil {
			return err
		}

		// Determine update context
		updateCtx := determineUpdateContext(secCtx, flags.NewCredential)

		// Update the entry
		if err := storageProvider.Update(opCtx, updateCtx, keyID, updates); err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to update keystore entry", err).KeyID(keyID).Log()
			return errfmt.Newf("failed to update keystore entry").Wrap(err)
		}

		// Build and format output
		result := buildRotateResult(keyID, flags)
		switch cli.GetFormat(cmd) {
		case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
			return cli.FormatOutput(cmd, result)
		default:
			return cli.WriteOutput(cmd, formatRotateOutputText(keyID, flags, result))
		}
	})(cmd, args)
}
