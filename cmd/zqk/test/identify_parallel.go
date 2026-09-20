package test

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	bldr "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/testrunner"
)

// NewIdentifyParallelCmd creates the `zqk test identify-parallel` command.
func NewIdentifyParallelCmd() *cobra.Command {
	cmd := bldr.NewTestIdentifyParallelCommandBuilder()
	cmd.Aliases = []string{"parallel-audit"}
	cmd.RunE = cli.WithProcessor(runIdentifyParallel)
	return cmd
}

func runIdentifyParallel(cmd *cobra.Command, args []string, proc *cli.Processor) error {
	var flags clipkg.FlagBag
	searchPath := flags.String(cmd, "path")
	safeOnly := flags.Bool(cmd, "safe-only")
	unsafeOnly := flags.Bool(cmd, "unsafe-only")
	if err := flags.Err(); err != nil {
		return errfmt.Newf("identify-parallel").Wrap(err)
	}

	if len(args) > 0 {
		searchPath = args[0]
	}

	results, err := testrunner.ScanParallelSafety(proc.ProjectRoot(), searchPath)
	if err != nil {
		return errfmt.Newf("scan parallel safety").Wrap(err)
	}

	var filtered []testrunner.TestSafetyResult
	for _, r := range results {
		if safeOnly && r.Level != testrunner.SafetyLevelSafe {
			continue
		}
		if unsafeOnly && r.Level != testrunner.SafetyLevelUnsafeSetenv {
			continue
		}
		filtered = append(filtered, r)
	}

	format, _ := cmd.Flags().GetString("format")
	if format == "json" || format == "yaml" {
		return cli.FormatOutput(cmd, filtered)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Scanned %d test(s) under %s:\n\n", len(results), searchPath)

	safeCount := 0
	unsafeCount := 0
	alreadyParallel := 0
	needsReview := 0

	for _, r := range filtered {
		switch r.Level {
		case testrunner.SafetyLevelParallelAlready:
			alreadyParallel++
			fmt.Fprintf(out, "  ⚡ [PARALLEL]   %s:%s\n", r.File, r.FuncName)
		case testrunner.SafetyLevelSafe:
			safeCount++
			fmt.Fprintf(out, "  ✅ [SAFE]       %s:%s (%s)\n", r.File, r.FuncName, strings.Join(r.Reasons, "; "))
		case testrunner.SafetyLevelUnsafeSetenv:
			unsafeCount++
			fmt.Fprintf(out, "  ❌ [UNSAFE]     %s:%s (%s)\n", r.File, r.FuncName, strings.Join(r.Reasons, "; "))
		default:
			needsReview++
			fmt.Fprintf(out, "  ⚠️  [REVIEW]     %s:%s (%s)\n", r.File, r.FuncName, strings.Join(r.Reasons, "; "))
		}
	}

	fmt.Fprintf(out, "\nSummary: %d safe candidates, %d unsafe (Setenv), %d already parallel, %d need review.\n",
		safeCount, unsafeCount, alreadyParallel, needsReview)

	return nil
}
