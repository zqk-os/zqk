package object

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/spf13/cobra"
)

const coreDeleteReasonMinRunes = 30

// withCoreDeleteReasonFromFlags applies WithAllowCoreObjectDelete when --reason-code
// meets the minimum justification length (core kernel hard-delete break-glass).
//
// No elevated shortcut: privilege answers "may you", not "did you mean to". Every daemon is
// constructed elevated, so an elevation shortcut here would exempt exactly the automated callers
// the guard exists to stop. TRACK: BLI-1785723654802038000-b14064bc.
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
