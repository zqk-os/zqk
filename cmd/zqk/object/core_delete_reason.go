package object

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
)

const coreDeleteReasonMinRunes = 30

type deleteFlags struct {
	Cascade    bool
	UnlinkRefs bool
	DryRun     bool
}

func parseDeleteFlags(cmd *cobra.Command, customRefuseMsg string) (deleteFlags, error) {
	cascade, _ := cmd.Flags().GetBool("cascade")
	unlinkRefs, _ := cmd.Flags().GetBool("unlink-references")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if unlinkRefs && cascade {
		return deleteFlags{}, cli.Guard(cmd).Err(errors.New("--unlink-references cannot be combined with --cascade")).Return()
	}
	if !unlinkRefs && !cascade {
		return deleteFlags{}, cli.Guard(cmd).Err(errors.New(customRefuseMsg)).Return()
	}
	return deleteFlags{Cascade: cascade, UnlinkRefs: unlinkRefs, DryRun: dryRun}, nil
}

func prepareDeleteContext(cmd *cobra.Command, proc *cli.Processor, unlinkRefs bool, skipWriteBehind bool) (context.Context, error) {
	cliCtx := proc.WithCLIOperation()
	if skipWriteBehind {
		cliCtx = storage.WithSkipWriteBehind(cliCtx)
	}
	if unlinkRefs {
		cliCtx = storage.WithUnlinkReferencesBeforeDelete(cliCtx)
	}
	return withCoreDeleteReasonFromFlags(cmd, cliCtx)
}

// withCoreDeleteReasonFromFlags applies WithAllowCoreObjectDelete when --reason-code
// meets the minimum justification length (core kernel hard-delete break-glass).
//
// No elevated shortcut: privilege answers "may you", not "did you mean to". Every daemon is
// constructed elevated, so an elevation shortcut here would exempt exactly the automated callers
// the guard exists to stop.
func withCoreDeleteReasonFromFlags(cmd *cobra.Command, ctx context.Context) (context.Context, error) {
	reason, _ := cmd.Flags().GetString("reason-code")
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ctx, nil
	}
	if utf8.RuneCountInString(reason) < coreDeleteReasonMinRunes {
		return ctx, fmt.Errorf("--reason-code must be at least %d characters justifying hard-delete of a core kernel object (prefer archive + aggregate/compress with lineage)", coreDeleteReasonMinRunes)
	}
	return pkgctx.WithAllowCoreObjectDelete(ctx), nil
}
