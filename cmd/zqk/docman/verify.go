package docman

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/docman"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewVerifyCmd creates a new verify command for docman
func NewVerifyCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Cryptographically verify doc_entry objects against on-disk files and detect drift",
		"Verify cryptographic integrity (SHA-256 and byte size) of registered doc_entry objects against on-disk documentation files.",
		"",
		"This command:",
		"  - Scans doc_entry objects across specified subtrees or IDs",
		"  - Computes cryptographic SHA-256 hashes of the target markdown files",
		"  - Validates content_hash and content_size matching against recorded leases",
		"  - Fails closed if drift or unsealed published docs are detected",
		"  - Supports --auto-seal to automatically record/update cryptographic hashes",
	).
		AddExample("Verify all shipped documentation entries", "%s docman verify --shipped-only").
		AddExample("Verify specific doc_entry IDs", "%s docman verify --ids DOC-001,DOC-002").
		AddExample("Verify and auto-seal drifted or unsealed documents", "%s docman verify --shipped-only --auto-seal").
		ExcludeCommonFlags()

	verifyCmd := &cobra.Command{
		Use:   "verify",
		Short: "Cryptographically verify doc_entry objects against on-disk files and detect drift",
		RunE:  runVerify,
	}

	helpBuilder.ApplyToCommand(verifyCmd)

	verifyCmd.Flags().StringSlice("ids", nil, "Specific doc_entry IDs to verify")
	verifyCmd.Flags().StringSlice("subtrees", nil, "Specific subtrees to verify (defaults to all doc_entries)")
	verifyCmd.Flags().Bool("shipped-only", false, "Only verify shipped documentation subtrees (architecture, best-practices, onboarding)")
	verifyCmd.Flags().Bool("auto-seal", false, "Compute and update cryptographic content_hash and content_size in storage")
	verifyCmd.Flags().Bool("strict", true, "Fail closed (exit with non-zero code) if any verification violations occur")

	cli.AddCommonFlags(verifyCmd)

	return verifyCmd
}

func runVerify(cmd *cobra.Command, args []string) error {
	cmdCtx := cmd.Context()
	if cmdCtx == nil {
		cmdCtx = pkgctx.NewSystemContext()
	}

	profile := string(pkgctx.ProfileHuman)
	if f := cmd.Flags().Lookup("context"); f != nil {
		if val, err := cmd.Flags().GetString("context"); err == nil && val != emptyValue {
			profile = val
		}
	}

	logger := logging.GetLoggerFromProfile(profile)

	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	ids, err := cmd.Flags().GetStringSlice("ids")
	if err != nil {
		return errfmt.Newf("failed to get ids flag").Wrap(err)
	}
	subtrees, err := cmd.Flags().GetStringSlice("subtrees")
	if err != nil {
		return errfmt.Newf("failed to get subtrees flag").Wrap(err)
	}
	shippedOnly, err := cmd.Flags().GetBool("shipped-only")
	if err != nil {
		return errfmt.Newf("failed to get shipped-only flag").Wrap(err)
	}
	autoSeal, err := cmd.Flags().GetBool("auto-seal")
	if err != nil {
		return errfmt.Newf("failed to get auto-seal flag").Wrap(err)
	}
	strict, err := cmd.Flags().GetBool("strict")
	if err != nil {
		return errfmt.Newf("failed to get strict flag").Wrap(err)
	}

	factory, err := storage.NewStorageFactory(cmdCtx, projectRoot)
	if err != nil {
		return errfmt.Newf("failed to initialize storage factory").Wrap(err)
	}
	storageProvider := factory.GetStorageForKind("doc_entry")

	verifier := docman.NewVerifier(storageProvider, projectRoot)

	opts := docman.VerifyOptions{
		IDs:         ids,
		Subtrees:    subtrees,
		ShippedOnly: shippedOnly,
		AutoSeal:    autoSeal,
		Strict:      strict,
	}

	logging.Fluent(logger).Info("Running doc_entry cryptographic verification").
		Int("ids_count", len(ids)).
		Int("subtrees_count", len(subtrees)).
		Bool("shipped_only", shippedOnly).
		Bool("auto_seal", autoSeal).
		Log()

	res, err := verifier.Verify(cmdCtx, profile, opts)
	if err != nil {
		return errfmt.Newf("verification failed").Wrap(err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Scanned %d doc_entry object(s):\n", res.TotalChecked))
	sb.WriteString(fmt.Sprintf("  - Passed: %d\n", res.Passed))
	sb.WriteString(fmt.Sprintf("  - Drifted: %d\n", res.Drifted))
	sb.WriteString(fmt.Sprintf("  - Unsealed: %d\n", res.Unsealed))
	sb.WriteString(fmt.Sprintf("  - Missing target: %d\n", res.Missing))
	if autoSeal {
		sb.WriteString(fmt.Sprintf("  - Auto-sealed: %d\n", res.AutoSealed))
	}

	if len(res.Violations) > 0 {
		sb.WriteString("\nViolations:\n")
		for _, v := range res.Violations {
			sb.WriteString(fmt.Sprintf("  [%s] %s (%s): %s\n", v.Severity, v.ObjectID, v.Path, v.Message))
		}
	}

	_ = cli.WriteOutput(cmd, []byte(sb.String()))

	if strict && len(res.Violations) > 0 {
		return errfmt.Errorf("doc_entry verification failed: %d violation(s) detected", len(res.Violations))
	}

	if len(res.Violations) == 0 {
		_ = cli.WriteOutput(cmd, []byte("\n✓ All checked doc_entry objects cryptographically verified with 0 drift violations.\n"))
	}

	return nil
}
