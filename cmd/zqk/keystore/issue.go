package keystore

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/authcred"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// NewIssueCmd creates keystore issue — generate a unique API key for an ACC seat.
// wire CLI spec + codegen when keystore group is migrated.
func NewIssueCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Issue a unique API key for an account seat",
		"Generate a one-time ZQK_API_KEY secret for an ACC-* account, store only its",
		"fingerprint in keystore + account.tokens, and optionally write a local seating",
		"credential for orchestrate/sync-loop injection.",
		"",
		"The plaintext secret is printed once (and written under .zqk/seating/credentials/",
		"unless --no-seating-file). It cannot be retrieved later — rotate to replace.",
	).
		AddExample("Issue a key for a swarm worker account", "%s keystore issue --account-id ACC-… --title \"swarm_worker_1\"").
		AddExample("JSON (includes credential once)", "%s keystore issue --account-id ACC-… --title seat --format json").
		ExcludeCommonFlags()

	// add .zqk/cli/specs/keystore/issue_command.yaml + codegen.
	issueCmd := &cobra.Command{
		Use:   "issue [flags]",
		Short: "Issue a unique API key for an account seat",
		RunE:  runIssue,
	}

	helpBuilder.ApplyToCommand(issueCmd)
	cli.AddCommonFlags(issueCmd)

	issueCmd.Flags().String("account-id", "", "ACC-* account to bind the key to (required)")
	issueCmd.Flags().String("title", "", "Title for the keystore entry (required)")
	issueCmd.Flags().String("description", "", "Optional description")
	issueCmd.Flags().String("expires-at", "", "Optional expiration (RFC3339)")
	issueCmd.Flags().Bool("no-seating-file", false, "Do not write .zqk/seating/credentials/<account-id>")
	_ = issueCmd.MarkFlagRequired("account-id")
	_ = issueCmd.MarkFlagRequired("title")

	return issueCmd
}

func runIssue(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		accountID, _ := cmd.Flags().GetString("account-id")
		title, _ := cmd.Flags().GetString("title")
		description, _ := cmd.Flags().GetString("description")
		expiresAt, _ := cmd.Flags().GetString("expires-at")
		noSeating, _ := cmd.Flags().GetBool("no-seating-file")

		accountID = strings.TrimSpace(accountID)
		if !strings.HasPrefix(accountID, "ACC-") {
			return errfmt.Errorf("account-id must be ACC-* form")
		}

		secCtx := proc.SecurityContext()
		if err := authorizeIssueForAccount(secCtx, accountID); err != nil {
			return err
		}

		secret, err := generateAPIKeySecret()
		if err != nil {
			return err
		}
		credentialHash := hashToken(secret)

		flags := &CreateFlags{
			AccountID:   accountID,
			KeyType:     "api_key",
			Credential:  secret,
			Title:       title,
			Description: description,
			ExpiresAt:   expiresAt,
		}
		entry := buildKeystoreEntry(flags, accountID, credentialHash, "")
		keyID, err := createKeystoreEntry(proc, entry)
		if err != nil {
			return err
		}

		if err := appendAccountTokenFingerprint(proc, accountID, keyID, credentialHash, expiresAt); err != nil {
			proc.Logger().LogWarning(fmt.Sprintf("keystore entry %s created but account.tokens update failed: %v", keyID, err))
		}

		seatingPath := ""
		if !noSeating {
			if err := authcred.WriteSeatCredential(proc.ProjectRoot(), accountID, secret); err != nil {
				return errfmt.Newf("write seating credential").Wrap(err)
			}
			seatingPath = authcred.SeatCredentialPath(proc.ProjectRoot(), accountID)
		}

		result := map[string]any{
			objects.FieldKeyID:        keyID,
			objects.FieldKeyAccountID: accountID,
			objects.FieldKeyKeyType:   "api_key",
			objects.FieldKeyTitle:     title,
			"credential":              secret,
			"fingerprint":             credentialHash,
			"seating_file":            seatingPath,
			"env_hint":                fmt.Sprintf("%s=%s", zqkenv.APIKey(), secret),
		}

		switch cli.GetFormat(cmd) {
		case "json", "yaml":
			return cli.FormatOutput(cmd, result)
		default:
			var buf strings.Builder
			buf.WriteString("✅ API key issued (plaintext shown once)\n\n")
			fmt.Fprintf(&buf, "Key ID:       %s\n", keyID)
			fmt.Fprintf(&buf, "Account:      %s\n", accountID)
			fmt.Fprintf(&buf, "Fingerprint:  %s\n", credentialHash)
			fmt.Fprintf(&buf, "Credential:   %s\n", secret)
			if seatingPath != "" {
				fmt.Fprintf(&buf, "Seating file: %s\n", seatingPath)
			}
			buf.WriteString("\nExport for this seat:\n")
			fmt.Fprintf(&buf, "  export %s=%s\n", zqkenv.APIKey(), secret)
			return cli.WriteOutput(cmd, []byte(buf.String()))
		}
	})(cmd, args)
}

func authorizeIssueForAccount(secCtx *pkgctx.SecurityContext, accountID string) error {
	if secCtx == nil {
		return errfmt.Errorf("missing security context")
	}
	if secCtx.AccountID == pkgctx.SystemAccountID || secCtx.AccountID == accountID {
		return nil
	}
	for _, role := range secCtx.Roles {
		if role == "admin" {
			return nil
		}
	}
	return errfmt.Errorf("permission denied: only admins (or the account owner) can issue keys for %s", accountID)
}

func generateAPIKeySecret() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", errfmt.Newf("generate api key").Wrap(err)
	}
	return authcred.SecretPrefix + hex.EncodeToString(b[:]), nil
}

func appendAccountTokenFingerprint(proc *cli.Processor, accountID, keyID, fingerprint, expiresAt string) error {
	systemCtx := pkgctx.NewSystemSecurityContext()
	opCtx := proc.OperationContext()
	sp := proc.Storage()
	acc, err := sp.Read(opCtx, systemCtx, accountID)
	if err != nil {
		return errfmt.Newf("read account %s", accountID).Wrap(err)
	}
	var tokens []any
	if existing, ok := acc[objects.FieldKeyTokens].([]any); ok {
		tokens = append(tokens, existing...)
	}
	tokens = append(tokens, authcred.TokenFingerprintMeta(keyID, fingerprint, expiresAt))
	acc[objects.FieldKeyTokens] = tokens
	if err := sp.Update(opCtx, systemCtx, accountID, acc); err != nil {
		return errfmt.Newf("update account tokens").Wrap(err)
	}
	return nil
}
