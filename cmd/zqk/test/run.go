package test

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	bldr "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testrunner"
)

// NewRunCmd creates the `zqk test run` command.
func NewRunCmd() *cobra.Command {
	cmd := bldr.NewTestRunCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			reconnect := cli.DisconnectTimeoutMonitor()
			defer reconnect()
			clipkg.ResetTimeout()
			all, _ := cmd.Flags().GetBool("all")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			verbose, _ := cmd.Flags().GetBool("verbose")
			timeoutStr, _ := cmd.Flags().GetString("timeout")
			criteriaFilter, _ := cmd.Flags().GetStringSlice("criteria")

			var timeout time.Duration
			if timeoutStr != "" {
				var err error
				timeout, err = time.ParseDuration(timeoutStr)
				if err != nil {
					return errfmt.Newf("invalid timeout duration: %s", timeoutStr).Wrap(err)
				}
			}

			testCaseIDs := args
			sp := proc.Storage()
			ctx := cmd.Context()
			secCtx := proc.SecurityContext()

			if len(testCaseIDs) == 0 {
				if !all {
					return errfmt.Errorf("specify one or more test_case IDs or pass --all")
				}
				// List all active test cases
				listRes, err := sp.List(ctx, secCtx, nil, storage.ListFilter{
					Kind: objects.KindTestCase,
				})
				if err != nil {
					return errfmt.Newf("failed to list test cases").Wrap(err)
				}
				for _, tc := range listRes.Objects {
					id, _ := tc[objects.FieldKeyID].(string)
					st, _ := tc[objects.FieldKeyStatus].(string)
					if id != "" && (st == objects.ObjectStatusActive || st == "draft" || st == "metrics_captured" || st == "originated" || st == "proposed") {
						testCaseIDs = append(testCaseIDs, id)
					}
				}
				if len(testCaseIDs) == 0 {
					_ = cli.WriteOutput(cmd, []byte("ℹ️  No active test cases found to run.\n"))
					return nil
				}
			}

			opts := testrunner.RunOptions{
				Verbose:        verbose,
				DryRun:         dryRun,
				Timeout:        timeout,
				CriteriaFilter: criteriaFilter,
			}

			totalRun := 0
			totalPassed := 0
			totalFailed := 0

			for _, tcID := range testCaseIDs {
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "\n▶ Executing Test Case: %s\n", tcID)

				result, err := testrunner.RunTestCase(ctx, sp, proc.ProjectRoot(), tcID, opts)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "❌ Error running %s: %v\n", tcID, err)
					totalFailed++
					continue
				}

				totalRun++
				if result.Status == "passed" || opts.DryRun {
					totalPassed++
				} else {
					totalFailed++
				}

				for _, cr := range result.CriteriaResults {
					if cr.Passed {
						fmt.Fprintf(out, "  ✓ [PASS] %s: %s (%v)\n", cr.CriterionID, cr.Command, cr.Duration.Round(time.Millisecond))
					} else {
						fmt.Fprintf(out, "  ✗ [FAIL] %s: %s (exit code %d, %v)\n", cr.CriterionID, cr.Command, cr.ExitCode, cr.Duration.Round(time.Millisecond))
						if cr.Error != "" {
							fmt.Fprintf(out, "    Error: %s\n", strings.TrimSpace(cr.Error))
						}
						if cr.Stderr != "" {
							fmt.Fprintf(out, "    Stderr: %s\n", strings.TrimSpace(cr.Stderr))
						}
						if cr.Stdout != "" && verbose {
							fmt.Fprintf(out, "    Stdout: %s\n", strings.TrimSpace(cr.Stdout))
						}
					}
				}

				// Shockwave Cascade Summary
				tcStatusBadge := "PENDING"
				if result.TestCaseCompleted {
					tcStatusBadge = "COMPLETE"
				}
				fmt.Fprintf(out, "  ⚡ Status: %s | Criteria: %d/%d passed | Test Case: [%s]\n",
					strings.ToUpper(result.Status), result.PassedCriteria, result.TotalCriteria, tcStatusBadge,
				)
				if result.RequirementCompleted {
					fmt.Fprintf(out, "  ⚡ Shockwave Ripple: Requirement satisfied and transitioned to [COMPLETE]\n")
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "\nTest Execution Summary: %d test case(s) run, %d passed, %d failed\n",
				totalRun, totalPassed, totalFailed,
			)

			if totalFailed > 0 && !opts.DryRun {
				return errfmt.Errorf("%d test case(s) failed", totalFailed)
			}
			return nil
		})(cmd, args)
	}

	return cmd
}
