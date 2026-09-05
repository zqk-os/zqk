package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/lanceman/zqk/pkg/brand"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	goroutinelabels "github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/process"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// OverrideStorage defines the minimal interface needed to create process debt objects.
type OverrideStorage interface {
	Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error
}

// OverrideConfirmationPhrase is the exact text a human must type on a TTY to proceed.
const OverrideConfirmationPhrase = "I acknowledge this bypass introduces process debt"

// EnforceOverrideFriction validates reason-code length, prompts for interactive confirmation,
// blocks in CI unless configured, blocks non-TTY (agent/pipe) callers, and records process debt.
//
// Order: CI → reason length → non-TTY deny → TTY phrase → record TDE.
// Product surface stays brand-agnostic: no studio policy/object IDs in errors or help.
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
		return fmt.Errorf("manual overrides are completely blocked in CI environments")
	}

	// 2. Justification length & word count check
	trimmed := strings.TrimSpace(reasonCode)
	words := strings.Fields(trimmed)
	if len(trimmed) < 30 || len(words) < 5 {
		return fmt.Errorf("--reason-code must be a descriptive justification of at least 30 characters (minimum 5 words) explaining why this bypass is necessary. Provided: %q (len=%d, words=%d)", reasonCode, len(trimmed), len(words))
	}

	// 3. Non-TTY hard deny (agents / pipes).
	stdinFd := int(os.Stdin.Fd())
	if !term.IsTerminal(stdinFd) {
		exe := brand.ExecutableName()
		return fmt.Errorf("lifecycle --override is blocked without an interactive TTY; use %s object promote/demote. Automated callers must not bypass the state machine", exe)
	}

	// 4. Interactive TTY confirmation
	if term.IsTerminal(stdinFd) {
		fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString("⚠️  WARNING: Bypassing lifecycle validation will introduce technical debt and requires manual remediation."))
		fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString("To proceed, you must type the following confirmation phrase exactly:"))
		fmt.Fprintf(cmd.ErrOrStderr(), "  %q\n\n", OverrideConfirmationPhrase)
		fmt.Fprintf(cmd.ErrOrStderr(), "Type the phrase: ")

		// TRACK: PRI-STABILIZE-FAILCLOSED-READS-001 — Cursor/IDE parent is not zqk; TimeoutHook
		// idle watchdog (~10s default) cancels OperationContext while the human types the phrase.
		stopPulse := pulseMeaningfulActivityWhileWaiting()
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		stopPulse()
		process.TouchMeaningfulActivity()
		if err != nil {
			return fmt.Errorf("failed to read confirmation input: %w", err)
		}
		input = strings.TrimSpace(input)
		if input != OverrideConfirmationPhrase {
			return fmt.Errorf("override aborted: confirmation phrase did not match exactly")
		}
		fmt.Fprintln(cmd.ErrOrStderr(), color.GreenString("✓ Confirmation accepted."))
	}

	// 5. Record Technical Debt directly in storage
	title := fmt.Sprintf("Snap-remedy on %s (Reason: %s)", targetID, reasonCode)
	debtObj := map[string]any{
		objects.FieldKeyKind:                 "technical_debt",
		objects.FieldKeyTitle:                title,
		objects.FieldKeyStatus:               objects.ObjectStatusIdentified,
		objects.FieldKeyDebtType:             "process",
		objects.FieldKeyImpactAssessment:     "high",
		objects.FieldKeyTargetResolutionDate: time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
		objects.FieldKeyDescription:          fmt.Sprintf("Manual status override bypassed state machine. Target: %s (%s). Reason: %s", targetID, targetKind, reasonCode),
	}
	if targetKind == "backlog_item" {
		debtObj[objects.FieldKeyBacklogRef] = targetID
	}

	// Debt create must not inherit a deadline already spent waiting on stdin.
	createBase := ctx
	if ctx != nil && ctx.Err() != nil {
		createBase = context.WithoutCancel(ctx)
	}
	// Do not arm lifecycle break_glass here. technical_debt is a critical kind;
	// storage refuses IsLifecycleBreakGlass ∧ critical ∧ !AllowCoreObjectDelete with a
	// misleading "break_glass requires --reason-code" error (KMP DECIDE). Status
	// identified is the preliminary origin (draft plane) — no lifecycle bypass needed.
	// TRACK: REDACTED
	createCtx := pkgctx.WithCacheUpdate(createBase, "", "technical_debt", "")
	if err := store.Create(createCtx, secCtx, debtObj); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), color.RedString(fmt.Sprintf("⚠️ WARNING: Failed to record technical debt in storage: %v", err)))
	} else {
		debtID, _ := debtObj[objects.FieldKeyID].(string)
		fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString(fmt.Sprintf("⚠️ WARNING: Manual status override detected. Process debt recorded: %s", debtID)))
	}

	return nil
}

// pulseMeaningfulActivityWhileWaiting keeps the CLI idle watchdog from canceling while
// EnforceOverrideFriction blocks on interactive confirmation. Returns a stop func.
func pulseMeaningfulActivityWhileWaiting() (stop func()) {
	process.TouchMeaningfulActivity()
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("override_idle_pulse", "touch meaningful activity during TTY override confirmation").
		StartSimple(func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					process.TouchMeaningfulActivity()
				}
			}
		})
	return func() { close(done) }
}
