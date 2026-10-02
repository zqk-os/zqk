package docman

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/docman"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// NewVerifyCmd creates a new verify command for docman
func NewVerifyCmd() *cobra.Command {
	verifyCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewDocmanVerifyCommandBuilder(), &cobra.Command{
		RunE: runVerify,
	})
	return verifyCmd
}

func runVerify(cmd *cobra.Command, args []string) error {
	env, err := initDocmanEnv(cmd)
	if err != nil {
		return err
	}

	logger := env.logger

	ids, err := cmd.Flags().GetStringArray("ids")
	if err != nil {
		return errfmt.Newf("failed to get ids flag").Wrap(err)
	}
	subtrees, shippedOnly, err := resolveSubtreesAndShipped(cmd)
	if err != nil {
		return err
	}
	autoSeal, err := cmd.Flags().GetBool("auto-seal")
	if err != nil {
		return errfmt.Newf("failed to get auto-seal flag").Wrap(err)
	}
	strict, err := cmd.Flags().GetBool("strict")
	if err != nil {
		return errfmt.Newf("failed to get strict flag").Wrap(err)
	}

	verifier := docman.NewVerifier(env.storageProvider, env.projectRoot)

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

	res, err := verifier.Verify(env.ctx, env.profile, opts)
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
