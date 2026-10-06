package keystore

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/authcred"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
	"golang.org/x/crypto/bcrypt"
)

// CreateFlags contains parsed create command flags
type CreateFlags struct {
	AccountID   string
	KeyType     string
	KeyData     string
	Title       string
	Description string
	ExpiresAt   string
}

// parseCreateFlags parses all create command flags
func parseCreateFlags(cmd *cobra.Command) *CreateFlags {
	flags := &CreateFlags{}

	accountID, _ := cmd.Flags().GetString("account-id") //nolint:errcheck // Flag getters don't fail in cobra
	flags.AccountID = accountID

	keyType, _ := cmd.Flags().GetString("key-type") //nolint:errcheck // Flag getters don't fail in cobra
	flags.KeyType = keyType

	credential, _ := cmd.Flags().GetString("credential") //nolint:errcheck // Flag getters don't fail in cobra
	flags.KeyData = credential

	title, _ := cmd.Flags().GetString("title") //nolint:errcheck // Flag getters don't fail in cobra
	flags.Title = title

	description, _ := cmd.Flags().GetString("description") //nolint:errcheck // Flag getters don't fail in cobra
	flags.Description = description

	expiresAt, _ := cmd.Flags().GetString("expires-at") //nolint:errcheck // Flag getters don't fail in cobra
	flags.ExpiresAt = expiresAt

	return flags
}

// validateKeyType validates the key type
func validateKeyType(keyType string) error {
	validKeyTypes := map[string]bool{
		"password":              true,
		"api_key":               true,
		"oauth_token":           true,
		"personal_access_token": true,
	}
	if !validKeyTypes[keyType] {
		return errfmt.Errorf("invalid key-type: %s (must be one of: password, api_key, oauth_token, personal_access_token)", keyType)
	}
	return nil
}

// determineAccountID determines the account ID with permission checks
func determineAccountID(flags *CreateFlags, secCtx *pkgctx.SecurityContext) (string, error) {
	if flags.AccountID == emptyValue {
		// Use current user's account ID
		accountID := secCtx.AccountID
		if accountID == emptyValue || accountID == pkgctx.SystemAccountID {
			return "", errfmt.Errorf("account-id is required (cannot use system account for keystore entries)")
		}
		return accountID, nil
	}

	if secCtx.AccountID != pkgctx.SystemAccountID {
		// Admin can set other accounts, but regular users can only set their own
		isAdmin := false
		for _, role := range secCtx.Roles {
			if role == "admin" {
				isAdmin = true
				break
			}
		}
		if !isAdmin && flags.AccountID != secCtx.AccountID {
			return "", errfmt.Errorf("permission denied: only admins can create keys for other accounts")
		}
	}

	return flags.AccountID, nil
}

// hashPassword hashes a user password using bcrypt with built-in salt.
func hashPassword(password string) (string, error) {
	hashBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", errfmt.Newf("failed to hash password").Wrap(err)
	}
	return string(hashBytes), nil
}

// hashToken hashes an API key or token using SHA-256 digest.
func hashToken(token string) string {
	return authcred.HashAPIKey(token)
}

// buildKeystoreEntry builds the keystore entry object
func buildKeystoreEntry(flags *CreateFlags, accountID, credentialHash, salt string) map[string]any {
	entry := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          flags.Title,
		objects.FieldKeyAccountID:      accountID,
		objects.FieldKeyKeyType:        flags.KeyType,
		objects.FieldKeyCredentialHash: credentialHash,
		objects.FieldKeySalt:           salt,
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	if flags.Description != emptyValue {
		entry[objects.FieldKeyDescription] = flags.Description
	}

	if flags.ExpiresAt != emptyValue {
		entry[objects.FieldKeyExpiresAt] = flags.ExpiresAt
	}

	return entry
}

// createKeystoreEntry creates the keystore entry using system context
func createKeystoreEntry(proc *cli.Processor, entry map[string]any) (string, error) {
	// Create the entry using system context (only system can set credential_hash)
	// This is a special case - we need to use system context to create the entry
	// with the hashed credential, but the entry will be associated with the user's account
	systemCtx := pkgctx.NewSystemSecurityContext()
	storageProvider := proc.Storage()

	// Create the entry
	opCtx := proc.OperationContext()
	if err := storageProvider.Create(opCtx, systemCtx, entry); err != nil {
		proc.Logger().LogError("Failed to create keystore entry", err)
		return "", errfmt.Newf("failed to create keystore entry").Wrap(err)
	}

	// Get the generated ID
	id, ok := entry[objects.FieldKeyID].(string)
	if !ok || id == emptyValue {
		return "", errfmt.Errorf("failed to get created keystore entry ID")
	}

	return id, nil
}

// buildCreateResult builds the result map for output
func buildCreateResult(id, accountID, keyType, title, expiresAt string) map[string]any {
	result := map[string]any{
		objects.FieldKeyID:        id,
		objects.FieldKeyAccountID: accountID,
		objects.FieldKeyKeyType:   keyType,
		objects.FieldKeyTitle:     title,
		objects.FieldKeyStatus:    objects.ObjectStatusCreated,
	}
	if expiresAt != emptyValue {
		result[objects.FieldKeyExpiresAt] = expiresAt
	}
	return result
}

// formatCreateOutputText returns human-readable create success output (default format).
func formatCreateOutputText(flags *CreateFlags, id, accountID string) []byte {
	var buf strings.Builder
	buf.WriteString("✅ Keystore entry created successfully\n\n")
	fmt.Fprintf(&buf, "ID:         %s\n", id)
	fmt.Fprintf(&buf, "Account:    %s\n", accountID)
	fmt.Fprintf(&buf, "Key Type:   %s\n", flags.KeyType)
	fmt.Fprintf(&buf, "Title:      %s\n", flags.Title)
	if flags.Description != emptyValue {
		fmt.Fprintf(&buf, "Description: %s\n", flags.Description)
	}
	if flags.ExpiresAt != emptyValue {
		fmt.Fprintf(&buf, "Expires:    %s\n", flags.ExpiresAt)
	}
	buf.WriteString("\n⚠️  Keep your credential secure - it cannot be retrieved after creation\n")
	return []byte(buf.String())
}
