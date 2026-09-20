package scheduler

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

func rerunTestFailures(cliCtx *cli.Context, cmd *cobra.Command) error {
	// First, list failures to get the set of failing tests
	// Then schedule new test jobs for only those tests

	projectRoot := cliCtx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
		if projectRoot == emptyValue {
			return errfmt.Errorf("project root not found")
		}
	}

	packageFilter, _ := cmd.Flags().GetString("package")
	sinceStr, _ := cmd.Flags().GetString("since")

	since, err := time.ParseDuration(sinceStr)
	if err != nil {
		return errfmt.Errorf("invalid duration %q: %w", sinceStr, err)
	}

	cutoffTime := time.Now().Add(-since)

	// Find callback log files and collect failures
	failuresByPackage := make(map[string][]string)

	callbackLogsDir := filepath.Join(projectRoot, paths.ProjectDataDir, "callbacks")
	testBundlesEventsPath := filepath.Join(schedpkg.JobLogsTestBundlesDir(projectRoot), "events.jsonl")
	_, errCallbacks := fileutil.Stat(callbackLogsDir)
	_, errBundles := fileutil.Stat(testBundlesEventsPath)
	if errCallbacks != nil && errBundles != nil {
		return errfmt.Errorf("no callback logs or test-bundle events found. Run some tests first")
	}

	var walkErr error
	if errCallbacks == nil {
		walkErr = filepath.Walk(callbackLogsDir, func(path string, info fileutil.FileInfo, walkPathErr error) error {
			if walkPathErr != nil || info.ModTime().Before(cutoffTime) {
				return nil
			}

			if !strings.HasSuffix(path, ".log") && !strings.HasSuffix(path, ".jsonl") {
				return nil
			}

			data, readErr := fileutil.ReadFile(path)
			if readErr != nil {
				return nil
			}

			for line := range strings.SplitSeq(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == emptyValue {
					continue
				}

				var entry map[string]any
				if err := json.Unmarshal([]byte(line), &entry); err != nil {
					continue
				}

				eventType, _ := entry[objects.FieldKeyEventType].(string)
				if eventType != schedulerStateFailed && eventType != schedulerStateError {
					continue
				}

				testFailures, ok := entry["test_failures"].([]any)
				if !ok {
					continue
				}

				for _, failure := range testFailures {
					failureStr, ok := failure.(string)
					if !ok {
						continue
					}

					parts := strings.Split(failureStr, ".")
					if len(parts) < 2 {
						continue
					}

					pkg := strings.Join(parts[:len(parts)-1], ".")
					testName := parts[len(parts)-1]

					if packageFilter != emptyValue && !strings.Contains(pkg, packageFilter) {
						continue
					}

					pkg = strings.TrimPrefix(pkg, "github.com/zqk-os/zqk/")
					if !strings.HasPrefix(pkg, "./") {
						pkg = "./" + pkg
					}

					failuresByPackage[pkg] = append(failuresByPackage[pkg], testName)
				}
			}

			return nil
		})
	}

	if walkErr != nil {
		return errfmt.Newf("failed to scan callback logs").Wrap(walkErr)
	}

	var extraTotal int
	if errBundles == nil {
		if err := mergeTestBundleEventFailures(projectRoot, cutoffTime, packageFilter, failuresByPackage, &extraTotal); err != nil {
			return err
		}
	}

	if len(failuresByPackage) == 0 {
		msg := "✅ No failing tests found to re-run.\n"
		if err := cli.WriteOutput(cmd, []byte(msg)); err != nil {
			return errfmt.Newf("failed to write output").Wrap(err)
		}
		return nil
	}

	// Create storage and schedule test jobs
	ctx := pkgctx.NewSystemContext()
	storageFactory, err := storagepkg.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		return errfmt.Newf("failed to create storage factory").Wrap(err)
	}
	storageProvider := storageFactory.GetStorage()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()

	// Schedule test jobs for each package with failing tests
	jobIDs := []string{}
	for pkg, testNames := range failuresByPackage {
		// Remove duplicates
		uniqueTests := make(map[string]bool)
		for _, name := range testNames {
			uniqueTests[name] = true
		}

		testList := make([]string, 0, len(uniqueTests))
		for name := range uniqueTests {
			testList = append(testList, name)
		}

		// Create test pattern
		pattern := strings.Join(testList, "|")
		if len(testList) > 1 {
			pattern = "^(" + pattern + ")$"
		} else {
			pattern = "^" + pattern + "$"
		}

		// Generate job ID
		jobID := fmt.Sprintf(submitJobIDPrefix, time.Now().UnixNano())

		// Test timeout slightly under job max_runtime_seconds so go test exits with timeout message
		// before the run_wrapper kills the process (avoids orphaned zqk processes).
		const rerunJobMaxRuntimeSec = 1800
		const rerunTestTimeoutSec = rerunJobMaxRuntimeSec - 60

		// Create scheduler job
		jobData := map[string]any{
			objects.FieldKeyID:                jobID,
			objects.FieldKeyKind:              schedulerKindJob,
			objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:            submitStatusActive,
			objects.FieldKeyJobType:           schedulerJobTypeRunWrapper,
			objects.FieldKeyTriggerType:       schedulerTriggerImmediate,
			objects.FieldKeyCategory:          "testing",
			objects.FieldKeyExecutionMode:     schedulerExecutionOneTime,
			objects.FieldKeyMaxRuntimeSeconds: rerunJobMaxRuntimeSec,
			objects.FieldKeyEnabled:           true,
			objects.FieldKeyTitle:             fmt.Sprintf("Re-run failing tests: %s", pkg),
			objects.FieldKeyDescription:       fmt.Sprintf("Re-running %d failing test(s) from %s", len(testList), pkg),
			objects.FieldKeyCommand:           "go",
			objects.FieldKeyCommandArgs: []string{
				"test",
				pkg,
				"-run",
				pattern,
				"-v",
				"-count=1",
				"-timeout",
				fmt.Sprintf("%ds", rerunTestTimeoutSec),
			},
			objects.FieldKeyRetryCount:        0,
			objects.FieldKeyRetryDelaySeconds: 5,
			objects.FieldKeyCreatedAt:         zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:         pkgctx.SystemAccountID,
			objects.FieldKeyUpdatedAt:         zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:         pkgctx.SystemAccountID,
			objects.FieldKeyOriginProject:     submitOriginProject,
			objects.FieldKeyOriginSystem:      submitOriginSystem,
		}

		if err := storageProvider.Create(ctx, secCtx, jobData); err != nil {
			schedpkg.SLog(logger).Warn("Failed to create test rerun job").
				PackageName(pkg).
				WithError(err).
				Log()
			continue
		}

		jobIDs = append(jobIDs, jobID)
	}

	// Enqueue trigger requests so the daemon actually runs the jobs (same as scan-tests flow).
	if len(jobIDs) > 0 && projectRoot != emptyValue {
		triggerQueue := schedpkg.NewJobTriggerQueue(projectRoot)
		if enqErr := triggerQueue.EnqueueTriggerRequests(jobIDs, ""); enqErr != nil {
			schedpkg.SLog(logger).Warn(paths.RewriteCanonicalCLIInvocations("Failed to enqueue trigger requests for rerun jobs; trigger them manually with 'zqk scheduler trigger <job-id>'")).
				WithError(enqErr).
				Int("job_count", len(jobIDs)).
				Log()
		}
	}

	var outputBuf strings.Builder
	if len(jobIDs) > 0 {
		// Collect package names for output
		pkgList := make([]string, 0, len(failuresByPackage))
		for pkg := range failuresByPackage {
			pkgList = append(pkgList, pkg)
		}
		sort.Strings(pkgList)

		for i, pkg := range pkgList {
			if failures, ok := failuresByPackage[pkg]; ok && i < len(jobIDs) {
				fmt.Fprintf(&outputBuf, "✅ Scheduled re-run for %d test(s) in %s (job: %s)\n", len(failures), pkg, jobIDs[i])
			}
		}
		fmt.Fprintf(&outputBuf, "%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("\n📋 Scheduled %d test job(s). Use 'zqk scheduler activity' to monitor progress.\n", len(jobIDs))))
		return cli.WriteOutput(cmd, []byte(outputBuf.String()))
	}

	return nil
}
