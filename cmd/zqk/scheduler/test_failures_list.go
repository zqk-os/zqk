package scheduler

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/gotestparse"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func listTestFailures(cliCtx *cli.Context, cmd *cobra.Command) error {
	cfg, err := parseTestFailuresFilterConfig(cliCtx, cmd)
	if err != nil {
		return err
	}

	limit, _ := cmd.Flags().GetInt("limit")

	// Find callback log files AND scheduler execution logs
	callbackLogsDir := filepath.Join(cfg.ProjectRoot, paths.ProjectDataDir, "callbacks")
	schedulerLogsDir := filepath.Join(cfg.ProjectRoot, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir)
	testBundlesEventsPath := filepath.Join(schedpkg.JobLogsTestBundlesDir(cfg.ProjectRoot), "health.jsonl")

	hasCallbackLogs := false
	hasSchedulerLogs := false
	hasTestBundlesEvents := false

	if _, err := fileutil.Stat(callbackLogsDir); err == nil {
		hasCallbackLogs = true
	}
	if _, err := fileutil.Stat(schedulerLogsDir); err == nil {
		hasSchedulerLogs = true
	}
	if _, err := fileutil.Stat(testBundlesEventsPath); err == nil {
		hasTestBundlesEvents = true
	}

	if !hasCallbackLogs && !hasSchedulerLogs && !hasTestBundlesEvents {
		msg := "No test logs found. Run some tests first.\n"
		if err := cli.WriteOutput(cmd, []byte(msg)); err != nil {
			return errfmt.Newf("failed to write output").Wrap(err)
		}
		return nil
	}

	// Collect all failures
	failuresByPackage := make(map[string][]string)
	totalFailures := 0

	// Process callback logs (contains parsed test_failures from callbacks)
	if hasCallbackLogs {
		err = walkLogFileLines(callbackLogsDir, cfg.CutoffTime, []string{".log", ".jsonl"}, func(line string) {
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				return
			}

			parseCallbackTestFailures(entry, cfg.PackageFilter, func(pkg, testName string) {
				fullTestName := fmt.Sprintf("%s.%s", pkg, testName)
				failuresByPackage[pkg] = append(failuresByPackage[pkg], fullTestName)
				totalFailures++
			})
		})
		if err != nil {
			return errfmt.Newf("failed to scan callback logs").Wrap(err)
		}
	}

	// Process scheduler execution logs (.zqk/logs/scheduler/SCH-XXX/SCH-XXX.log)
	// These contain execution history with stdout/stderr that may have test failure info
	if hasSchedulerLogs {
		err = walkLogFileLines(schedulerLogsDir, cfg.CutoffTime, []string{".log"}, func(line string) {
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				return
			}

			// Check if this is a failed test execution
			eventType, _ := entry[objects.FieldKeyEventType].(string)
			if eventType != schedulerStateFailed {
				return
			}

			// Extract command to check if it's a test command
			command, _ := entry[objects.FieldKeyCommand].(string)
			if !strings.Contains(command, "go test") {
				return
			}

			// Extract stderr which may contain test failure details
			stderr, _ := entry["stderr"].(string)
			if stderr != emptyValue {
				// Try to parse test failures from stderr using test parser
				if summary, err := gotestparse.ParseGoTestOutput(stderr); err == nil && summary.FailedCount > 0 {
					for _, test := range summary.FailedTestList {
						pkg := test.PackagePath
						testName := test.TestName

						// Apply package filter if specified
						if cfg.PackageFilter != emptyValue && !strings.Contains(pkg, cfg.PackageFilter) {
							continue
						}

						// Normalize package path
						pkg = strings.TrimPrefix(pkg, "github.com/zqk-os/zqk/")
						if !strings.HasPrefix(pkg, "./") {
							pkg = "./" + pkg
						}

						fullTestName := fmt.Sprintf("%s.%s", pkg, testName)
						failuresByPackage[pkg] = append(failuresByPackage[pkg], fullTestName)
						totalFailures++
					}
				}
			}
		})
		if err != nil {
			return errfmt.Newf("failed to scan scheduler logs").Wrap(err)
		}
	}

	if hasTestBundlesEvents {
		if err := mergeTestBundleEventFailures(cfg.ProjectRoot, cfg.CutoffTime, cfg.PackageFilter, failuresByPackage, &totalFailures); err != nil {
			return err
		}
	}

	// Display results
	var buf strings.Builder
	if totalFailures == 0 {
		buf.WriteString("✅ No test failures found in recent runs.\n")
		return cli.WriteOutput(cmd, []byte(buf.String()))
	}

	fmt.Fprintf(&buf, "Found %d failing test(s) in %d package(s):\n\n", totalFailures, len(failuresByPackage))

	// Sort packages for consistent output
	packages := make([]string, 0, len(failuresByPackage))
	for pkg := range failuresByPackage {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)

	displayed := 0
	for _, pkg := range packages {
		failures := failuresByPackage[pkg]
		fmt.Fprintf(&buf, "📦 %s (%d failure(s))\n", pkg, len(failures))

		for _, failure := range failures {
			if displayed >= limit {
				fmt.Fprintf(&buf, "\n... and %d more (use --limit to see more)\n", totalFailures-displayed)
				return cli.WriteOutput(cmd, []byte(buf.String()))
			}
			fmt.Fprintf(&buf, "  ❌ %s\n", failure)
			displayed++
		}
		buf.WriteString("\n")
	}

	return cli.WriteOutput(cmd, []byte(buf.String()))
}
