package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// OverrideStorage defines the minimal interface needed to create process debt objects.
type OverrideStorage interface {
	Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error
}

// EnforceOverrideFriction validates reason-code length, prompts for interactive confirmation,
// blocks in CI unless configured, and records the process debt directly to storage.
func EnforceOverrideFriction(
	cmd *cobra.Command,
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	store OverrideStorage,
	targetID string,
	targetKind string,
	reasonCode string,
) error {
	// 1. CI Block check
	if os.Getenv("CI") != emptyValue {
		if os.Getenv(zqkenv.AllowCIOverrides()) != "1" {
			return fmt.Errorf("manual overrides are completely blocked in CI environments. Set %s=1 if this is intentional", zqkenv.AllowCIOverrides())
		}
	}

	// 2. Justification length & word count check
	trimmed := strings.TrimSpace(reasonCode)
	words := strings.Fields(trimmed)
	if len(trimmed) < 30 || len(words) < 5 {
		return fmt.Errorf("--reason-code must be a descriptive justification of at least 30 characters (minimum 5 words) explaining why this bypass is necessary. Provided: %q (len=%d, words=%d)", reasonCode, len(trimmed), len(words))
	}

	// 3. Interactive TTY confirmation
	stdinFd := int(os.Stdin.Fd())
	if term.IsTerminal(stdinFd) {
		fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString("⚠️  WARNING: Bypassing lifecycle validation will introduce technical debt and requires manual remediation."))
		fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString("To proceed, you must type the following confirmation phrase exactly:"))
		expectedPhrase := "I acknowledge this bypass introduces process debt"
		fmt.Fprintf(cmd.ErrOrStderr(), "  %q\n\n", expectedPhrase)
		fmt.Fprintf(cmd.ErrOrStderr(), "Type the phrase: ")

		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read confirmation input: %w", err)
		}
		input = strings.TrimSpace(input)
		if input != expectedPhrase {
			return fmt.Errorf("override aborted: confirmation phrase did not match exactly")
		}
		fmt.Fprintln(cmd.ErrOrStderr(), color.GreenString("✓ Confirmation accepted."))
	}

	// 4. Record Technical Debt directly in storage
	title := fmt.Sprintf("Snap-remedy on %s (Reason: %s)", targetID, reasonCode)
	if len(title) > 120 {
		title = title[:117] + "..."
	}
	debtObj := map[string]any{
		objects.FieldKeyKind:                 "technical_debt",
		objects.FieldKeyTitle:                title,
		objects.FieldKeyStatus:               "identified",
		objects.FieldKeyDebtType:             "process",
		objects.FieldKeyImpactAssessment:     "high",
		objects.FieldKeyTargetResolutionDate: time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
		objects.FieldKeyDescription:          fmt.Sprintf("Manual status override bypassed state machine. Target: %s (%s). Reason: %s", targetID, targetKind, reasonCode),
	}
	if targetKind == "backlog_item" {
		debtObj[objects.FieldKeyBacklogRef] = targetID
	}

	// Use relaxed mode when creating the debt object so we don't trigger validation warnings/blocks during the override itself
	createCtx := pkgctx.WithCacheUpdate(ctx, "", "technical_debt", "")
	createCtx = pkgctx.WithForceLifecycleOverride(createCtx)
	if err := store.Create(createCtx, secCtx, debtObj); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), color.RedString(fmt.Sprintf("⚠️ WARNING: Failed to record technical debt in storage: %v", err)))
	} else {
		debtID, _ := debtObj[objects.FieldKeyID].(string)
		fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString(fmt.Sprintf("⚠️ WARNING: Manual status override detected. Process debt recorded: %s", debtID)))
	}

	return nil
}
