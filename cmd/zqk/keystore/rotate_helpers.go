package keystore

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/authcred"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// RotateFlags contains parsed rotate command flags
type RotateFlags struct {
	NewKeyData string
	Revoke     bool
	RevokeOld  bool
}

// parseRotateFlags parses all rotate command flags
func parseRotateFlags(cmd *cobra.Command) (*RotateFlags, error) {
	flags := &RotateFlags{}

	newKey, _ := cmd.Flags().GetString("new-credential") //nolint:errcheck // Flag getters don't fail in cobra
	flags.NewKeyData = newKey

	revoke, _ := cmd.Flags().GetBool("revoke") //nolint:errcheck // Flag getters don't fail in cobra
	flags.Revoke = revoke

	revokeOld, _ := cmd.Flags().GetBool("revoke-old") //nolint:errcheck // Flag getters don't fail in cobra
	flags.RevokeOld = revokeOld

	// Validate flags
	if flags.NewKeyData == emptyValue && !flags.Revoke && !flags.RevokeOld {
		return nil, errfmt.Errorf("must specify either --new-credential, --revoke, or --revoke-old")
	}

	return flags, nil
}

// validateKeystoreEntry validates that the entry is a keystore_entry
func validateKeystoreEntry(existing map[string]any, keyID string) error {
	kind, _ := existing[objects.FieldKeyKind].(string)
	if kind != objects.KindKeystoreEntry {
		return errfmt.Errorf("object %s is not a keystore_entry (kind: %s)", keyID, kind)
	}
	return nil
}

// checkPermissions checks if the user has permission to perform the operation
func checkPermissions(secCtx *pkgctx.SecurityContext, accountID string, requireSystem bool) (isAdmin, hasPermission bool, err error) {
	isSystem := secCtx.AccountID == pkgctx.SystemAccountID
	isAdmin = false
	for _, role := range secCtx.Roles {
		if role == "admin" {
			isAdmin = true
			break
		}
	}
	hasPermission = accountID == secCtx.AccountID

	if requireSystem && !isSystem {
		err = errfmt.Errorf("permission denied: only system can rotate keys (update credential_hash)")
		return
	}

	return
}

// buildRevocationUpdates builds updates for revocation
func buildRevocationUpdates(revoke, revokeOld bool) map[string]any {
	if !revoke && !revokeOld {
		return nil
	}
	return map[string]any{
		objects.FieldKeyRevoked:   true,
		objects.FieldKeyRevokedAt: zqktime.NowRFC3339UTC(),
	}
}

// buildRotationUpdates builds updates for rotation
func buildRotationUpdates(newVal, keyType string) (map[string]any, error) {
	if newVal == emptyValue {
		return nil, nil
	}

	var hashVal string
	var err error
	if keyType == "password" {
		hashVal, err = hashUserKey(newVal)
		if err != nil {
			return nil, err
		}
	} else {
		hashVal = hashToken(newVal)
	}

	updates := map[string]any{
		objects.FieldKeyCredentialHash: authcred.SanitizeDigest(hashVal),
		objects.FieldKeyLastUsedAt:     "", // Reset last_used_at on rotation
	}
	// Note: salt is always empty (bcrypt has built-in salt, SHA256 doesn't use salt)

	return updates, nil
}

// buildAllUpdates builds all updates for rotation and revocation
func buildAllUpdates(flags *RotateFlags, keyType string) (map[string]any, error) {
	updates := make(map[string]any)

	// Handle revocation
	if revokeUpdates := buildRevocationUpdates(flags.Revoke, flags.RevokeOld); revokeUpdates != nil {
		for k, v := range revokeUpdates {
			updates[k] = v
		}
	}

	// Handle rotation (new credential)
	rotationUpdates, err := buildRotationUpdates(flags.NewKeyData, keyType)
	if err != nil {
		return nil, err
	}
	for k, v := range rotationUpdates {
		updates[k] = v
	}

	// If no updates, return error
	if len(updates) == 0 {
		return nil, errfmt.Errorf("no updates specified")
	}

	return updates, nil
}

// determineUpdateContext determines which security context to use for updates
func determineUpdateContext(secCtx *pkgctx.SecurityContext, newKeyData string) *pkgctx.SecurityContext {
	if newKeyData != emptyValue {
		// Must use system context for credential_hash updates
		return pkgctx.NewSystemSecurityContext()
	}
	// Can use user context for revocation or default case
	return secCtx
}

// buildRotateResult builds the result map for output
func buildRotateResult(keyID string, flags *RotateFlags) map[string]any {
	result := map[string]any{
		objects.FieldKeyID:     keyID,
		objects.FieldKeyStatus: objects.ObjectStatusUpdated,
		"actions":              []string{},
	}

	if flags.NewKeyData != emptyValue {
		result["actions"] = append(result["actions"].([]string), "rotated")
	}
	if flags.Revoke || flags.RevokeOld {
		result["actions"] = append(result["actions"].([]string), "revoked")
	}

	return result
}

// formatRotateOutputText returns human-readable rotate success output (default format).
func formatRotateOutputText(keyID string, flags *RotateFlags, result map[string]any) []byte {
	var buf strings.Builder
	buf.WriteString("✅ Keystore entry updated successfully\n\n")
	fmt.Fprintf(&buf, "ID: %s\n", keyID)

	actions := result["actions"].([]string)
	if len(actions) > 0 {
		fmt.Fprintf(&buf, "Actions: %s\n", strings.Join(actions, ", "))
	}

	if flags.NewKeyData != emptyValue {
		buf.WriteString("⚠️  Key has been rotated - old credential is no longer valid\n")
	}
	if flags.Revoke || flags.RevokeOld {
		buf.WriteString("⚠️  Key has been revoked - it cannot be used for authentication\n")
	}

	return []byte(buf.String())
}
