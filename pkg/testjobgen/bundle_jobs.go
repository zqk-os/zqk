// Package testjobgen creates scheduler run_wrapper jobs from test bundles (scan-tests output).
// It lives outside pkg/testing so production code does not depend on the test helper module path.
package testjobgen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/process"
	schedcore "github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testscan"
	"github.com/lanceman/zqk/pkg/validation"
)

const emptyValue = ""

// Test-bundle timeout policy (scheduler job max_runtime_seconds and baseline for go test -timeout).
// Keep in sync with comments in GenerateJobs / buildTestArgs and SuggestedTimeoutForPackage in pkg/testscan.
const (
	testBundleMinTimeoutSeconds = 300  // 5m floor so short estimates still get a usable window
	testBundleMaxTimeoutSeconds = 3600 // 1h cap (matches typical job max; heavy all-kinds bundles bump up to this)
	// testBundleGoTestTimeoutLeadSeconds: go test -timeout is this many seconds below the job limit so
	// the test binary prints a useful timeout message before max_runtime_seconds kills the wrapper.
	testBundleGoTestTimeoutLeadSeconds = 60
)

// computeTestBundleTimeoutSeconds returns max_runtime_seconds for the bundle (before go-test lead subtraction).
func computeTestBundleTimeoutSeconds(bundle *testscan.TestBundle, projectRoot string) int {
	timeout := int(bundle.EstimatedDuration.Seconds() * 1.5)
	timeout = max(timeout, testBundleMinTimeoutSeconds)
	timeout = max(timeout, testscan.GetMinTimeoutSecondsForPackage(projectRoot, bundle.PackagePath))
	if hasHeavyAllKindsTests(bundle) {
		timeout = max(timeout, testBundleMaxTimeoutSeconds)
	}
	return min(timeout, testBundleMaxTimeoutSeconds)
}

// JobGenerator creates scheduler jobs for test bundles
type JobGenerator struct {
	storage storage.ObjectStorageProvider
	secCtx  *pkgctx.SecurityContext
}

// NewJobGenerator creates a new scheduler job generator
func NewJobGenerator(storage storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) *JobGenerator {
	return &JobGenerator{
		storage: storage,
		secCtx:  secCtx,
	}
}

// stableJobIDPrefix is the prefix for scan-tests run_wrapper jobs so the same bundle reuses one job.
const stableJobIDPrefix = "SCH-run-"

// sanitizeBundleIDForJobID returns a safe suffix for scheduler job IDs (alphanumeric and single dashes).
var nonAlnumDash = regexp.MustCompile(`[^a-zA-Z0-9-]+`)

func sanitizeBundleIDForJobID(bundleID string) string {
	s := nonAlnumDash.ReplaceAllString(bundleID, "-")
	s = strings.Trim(s, "-")
	if s == emptyValue {
		s = "bundle"
	}
	return s
}

// hasHeavyAllKindsTests returns true if the bundle contains tests that run many subtests (e.g. one per kind)
// and routinely exceed the default estimated duration; such bundles get the full 1-hour job timeout.
func hasHeavyAllKindsTests(bundle *testscan.TestBundle) bool {
	for _, t := range bundle.Tests {
		switch t.Name {
		case "TestAllKindsCRUD", "TestAllKindsPagination":
			return true
		}
	}
	return false
}

// priorityForTestBundleJob returns scheduler_job.priority for scan-tests–generated run_wrapper jobs.
// Bundles that carry process evidence (criteria and/or test_case refs) use high so the scheduler
// dispatches them on the priority worker pool ahead of bulk test-only bundles.
func priorityForTestBundleJob(bundle *testscan.TestBundle) string {
	if bundle == nil {
		return schedcore.JobPriorityNormal
	}
	if len(bundle.CriteriaRefs) > 0 || len(bundle.TestCaseRefs) > 0 {
		return schedcore.JobPriorityHigh
	}
	return schedcore.JobPriorityNormal
}

// shouldLimitGoTestParallelTests returns true for packages where t.Parallel() plus subprocess
// tests routinely flake under bundled `go test` (broken pipe, races). For those packages,
// pass `-parallel 1` so at most one parallel test runs at a time (see `go help test`).
func shouldLimitGoTestParallelTests(packagePath string) bool {
	return testscan.PackageWantsGoTestParallelOne(packagePath)
}

func commandStringFromStoredJob(obj map[string]any) string {
	if obj == nil {
		return emptyValue
	}
	cmd, _ := obj[objects.FieldKeyCommand].(string)
	var args []string
	if argsInterface, ok := obj[objects.FieldKeyCommandArgs].([]any); ok {
		for _, arg := range argsInterface {
			if argStr, ok := arg.(string); ok {
				args = append(args, argStr)
			}
		}
	}
	return schedcore.FormatRunWrapperCommandString(cmd, args)
}

// warnIfRescheduledTestBundleFingerprintChanged logs when an existing SCH-run-* job's go test -run set
// changes so operators do not confuse object get / health.jsonl rows keyed by the old fingerprint.
func (jg *JobGenerator) warnIfRescheduledTestBundleFingerprintChanged(ctx context.Context, jobID, bundleID, newFP string, logger logging.Logger) {
	if newFP == emptyValue {
		return
	}
	prev, err := jg.storage.Read(ctx, jg.secCtx, jobID)
	if err != nil || prev == nil {
		return
	}
	oldStr := commandStringFromStoredJob(prev)
	if oldStr == emptyValue {
		return
	}
	oldFP := schedcore.FingerprintBundleCommand(oldStr)
	if oldFP == emptyValue || oldFP == newFP {
		return
	}
	logging.Fluent(logger).Warn("test bundle job command fingerprint changed; prior test-bundles health and event rows for the old fingerprint do not describe this command").
		String(schedcore.KeyJobID, jobID).
		String(schedcore.KeyBundleID, bundleID).
		String(schedcore.KeyPriorBundleCommandFingerprint, oldFP).
		String(schedcore.KeyNewBundleCommandFingerprint, newFP).
		Log()
}

// GenerateJobs creates scheduler jobs for test bundles.
// Reuses an existing job when one exists for the same bundle ID (stable ID SCH-run-<bundle.ID>):
// updates that job with the current bundle definition and re-enables it so it runs again.
// Otherwise creates a new job. This keeps the job store from growing with redundant run_wrapper jobs.
// Uses bulk-create mode: defers CAS index flush until all jobs (and their audit events) are created,
// then flushes once to reduce I/O and wait-bound behavior.
func (jg *JobGenerator) GenerateJobs(ctx context.Context, bundles []*testscan.TestBundle, projectRoot string, maxParallel int) ([]string, error) {
	var jobIDs []string
	now := time.Now().UTC()

	// Resolve to absolute so run_wrapper uses it as cmd.Dir regardless of scheduler CWD
	absProjectRoot := projectRoot
	if absProjectRoot != emptyValue {
		if p, err := filepath.Abs(projectRoot); err == nil {
			absProjectRoot = p
		}
	}

	// All test-bundle logs in one folder to reduce directories and align with scheduler job logs (JobLogDir for SCH-run-*).
	logDir := schedcore.JobLogsTestBundlesDir(projectRoot)
	if err := os.MkdirAll(logDir, paths.DirPerm755); err != nil {
		return nil, errfmt.Errorf("failed to create log directory %s: %w", logDir, err)
	}

	ctx = storage.WithBulkCreateDeferFlush(ctx)
	// Use sync (non-WAL) writes so all YAML files are on disk before FlushKind runs the CAS
	// index rebuild. WAL write-behind would queue 327 files and block FlushKind for minutes.
	ctx = storage.WithSyncCreateForSchedulerJob(ctx)
	// Defer audit events so they are batched into a single CAS flush instead of one WAL entry
	// per create (327 individual WAL writes would slow down subsequent runs via WAL replay).
	ctx = storage.WithDeferAuditEvents(ctx)

	concurrencyByPackage := testscan.ComputePackageConcurrencyLimitsFromBundles(bundles, maxParallel)
	jgLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	for i, bundle := range bundles {
		// Touch meaningful activity every 25 items so the idle watchdog (10s inactivity limit)
		// does not cancel the context mid-loop when processing large sets (327+ bundles).
		if i%25 == 0 {
			process.TouchMeaningfulActivity()
		}
		// Use UnixNano to generate a unique job ID for each run. This prevents
		// lifecycle transition errors (e.g., from 'archived' to 'active') when reusing
		// an old job ID. Stale jobs are cleaned up separately.
		timestamp := time.Now().UnixNano()
		jobID := fmt.Sprintf("%s%s-%d", stableJobIDPrefix, sanitizeBundleIDForJobID(bundle.ID), timestamp)

		logFileName := fmt.Sprintf("bundle-%s-%d.log", bundle.ID, timestamp)
		logFilePath := filepath.Join(logDir, logFileName)

		// Make path absolute for shell redirection
		absLogFilePath, err := filepath.Abs(logFilePath)
		if err != nil {
			return jobIDs, errfmt.Newf("failed to resolve absolute log path").Wrap(err)
		}
		logFilePath = absLogFilePath

		// Build test command arguments
		args := jg.buildTestArgs(bundle, projectRoot, logFilePath)
		newCmdStr := schedcore.FormatRunWrapperCommandString(schedcore.RunWrapperShellCommand, args)
		newFP := schedcore.FingerprintBundleCommand(newCmdStr)

		timeout := computeTestBundleTimeoutSeconds(bundle, projectRoot)

		// Persisted map keys: use objects.FieldKey* (spec-union field_keys.go), not per-kind bldr_v2 aliases.
		// Add metadata about the bundle
		ppKey := strings.TrimPrefix(strings.TrimSpace(bundle.PackagePath), "./")
		maxConc := concurrencyByPackage[ppKey]
		if maxConc <= 0 {
			maxConc = 1
		}
		metadata := map[string]any{
			schedcore.KeyTestBundleMetaBundleID:                 bundle.ID,
			schedcore.KeyTestBundleMetaPackagePath:              bundle.PackagePath,
			schedcore.KeyTestBundleMetaIsParallel:               bundle.IsParallel,
			schedcore.KeyTestBundleMetaTestCount:                len(bundle.Tests),
			schedcore.KeyTestBundleMetaEstimatedDurationSec:     int(bundle.EstimatedDuration.Seconds()),
			schedcore.KeyTestBundleMetaLogFile:                  logFilePath,
			schedcore.KeyTestBundleMetaMaxConcurrentSamePackage: maxConc,
			schedcore.KeyBundleCommandFingerprint:               newFP,
		}
		if len(bundle.CriteriaRefs) > 0 {
			metadata[schedcore.KeyTestBundleMetaCriteriaRefs] = append([]string(nil), bundle.CriteriaRefs...)
		}
		if len(bundle.TestCaseRefs) > 0 {
			metadata[schedcore.KeyTestBundleMetaTestCaseRefs] = append([]string(nil), bundle.TestCaseRefs...)
		}

		jobPri := priorityForTestBundleJob(bundle)

		jobData := map[string]any{
			objects.FieldKeyID:                jobID,
			objects.FieldKeyKind:              objects.KindSchedulerJob,
			objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:            schedcore.StatusActive,
			objects.FieldKeyJobType:           schedcore.JobTypeRunWrapper,
			objects.FieldKeyTriggerType:       schedcore.TriggerTypeImmediate,
			objects.FieldKeyCategory:          schedcore.CategoryTesting,
			objects.FieldKeyExecutionMode:     schedcore.ExecutionModeOneTime,
			objects.FieldKeyMaxRuntimeSeconds: timeout,
			objects.FieldKeyEnabled:           true,
			objects.FieldKeyTitle:             fmt.Sprintf("Test Bundle: %s (%d tests)", bundle.PackagePath, len(bundle.Tests)),
			objects.FieldKeyDescription:       jg.buildDescription(bundle, logFilePath),
			objects.FieldKeyCommand:           schedcore.RunWrapperShellCommand,
			objects.FieldKeyCommandArgs:       args,
			objects.FieldKeyRetryCount:        schedcore.DefaultRetryCount,
			objects.FieldKeyRetryDelaySeconds: schedcore.DefaultRetryDelaySeconds,
			objects.FieldKeyCreatedAt:         now.Format(time.RFC3339),
			objects.FieldKeyCreatedBy:         pkgctx.SystemAccountID,
			objects.FieldKeyUpdatedAt:         now.Format(time.RFC3339),
			objects.FieldKeyUpdatedBy:         pkgctx.SystemAccountID,
			objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
			objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
			objects.FieldKeyWorkingDirectory:  absProjectRoot,
			objects.FieldKeyPriority:          jobPri,
			objects.FieldKeyMetadata:          metadata,
		}

		// Reuse existing job for this bundle if present (same stable ID).
		// Use Exists (O(1) CAS index lookup) instead of List (O(N) full YAML read) so the
		// N-bundle loop stays O(N) total instead of O(N²) as created files accumulate.
		jobExists, existsErr := jg.storage.Exists(ctx, jg.secCtx, jobID)
		if existsErr == nil && jobExists {
			jg.warnIfRescheduledTestBundleFingerprintChanged(ctx, jobID, bundle.ID, newFP, jgLogger)
			// Update and re-enable so the scheduler runs it again (one_time jobs are skipped when last_run_at is set)
			updates := map[string]any{
				objects.FieldKeyEnabled:           true,
				objects.FieldKeyLastRunAt:         "", // clear so loader doesn't skip
				objects.FieldKeyStatus:            schedcore.StatusActive,
				objects.FieldKeyTitle:             jobData[objects.FieldKeyTitle],
				objects.FieldKeyDescription:       jobData[objects.FieldKeyDescription],
				objects.FieldKeyCommand:           jobData[objects.FieldKeyCommand],
				objects.FieldKeyCommandArgs:       jobData[objects.FieldKeyCommandArgs],
				objects.FieldKeyMaxRuntimeSeconds: jobData[objects.FieldKeyMaxRuntimeSeconds],
				objects.FieldKeyWorkingDirectory:  absProjectRoot,
				objects.FieldKeyPriority:          jobPri,
				objects.FieldKeyMetadata:          metadata,
				objects.FieldKeyUpdatedAt:         now.Format(time.RFC3339),
				objects.FieldKeyUpdatedBy:         pkgctx.SystemAccountID,
			}
			if err := jg.storage.Update(ctx, jg.secCtx, jobID, updates); err != nil {
				return jobIDs, errfmt.Errorf("failed to update job %s for bundle %s: %w", jobID, bundle.ID, err)
			}
			jobIDs = append(jobIDs, jobID)
			continue
		}

		// Create the job (CAS flush deferred until after loop)
		if err := jg.storage.Create(ctx, jg.secCtx, jobData); err != nil {
			// List may not have found the job (e.g. CAS index lag); if Create says already exists, update and re-enable
			if err == storage.ErrObjectExists || strings.Contains(err.Error(), "already exists") {
				jg.warnIfRescheduledTestBundleFingerprintChanged(ctx, jobID, bundle.ID, newFP, jgLogger)
				updates := map[string]any{
					objects.FieldKeyEnabled:           true,
					objects.FieldKeyLastRunAt:         "",
					objects.FieldKeyStatus:            schedcore.StatusActive,
					objects.FieldKeyTitle:             jobData[objects.FieldKeyTitle],
					objects.FieldKeyDescription:       jobData[objects.FieldKeyDescription],
					objects.FieldKeyCommand:           jobData[objects.FieldKeyCommand],
					objects.FieldKeyCommandArgs:       jobData[objects.FieldKeyCommandArgs],
					objects.FieldKeyMaxRuntimeSeconds: jobData[objects.FieldKeyMaxRuntimeSeconds],
					objects.FieldKeyWorkingDirectory:  absProjectRoot,
					objects.FieldKeyPriority:          jobPri,
					objects.FieldKeyMetadata:          metadata,
					objects.FieldKeyUpdatedAt:         now.Format(time.RFC3339),
					objects.FieldKeyUpdatedBy:         pkgctx.SystemAccountID,
				}
				if updateErr := jg.storage.Update(ctx, jg.secCtx, jobID, updates); updateErr != nil {
					return jobIDs, errfmt.Errorf("failed to update existing job %s for bundle %s: %w", jobID, bundle.ID, updateErr)
				}
				jobIDs = append(jobIDs, jobID)
				continue
			}
			return jobIDs, errfmt.Errorf("failed to create job for bundle %s: %w", bundle.ID, err)
		}

		jobIDs = append(jobIDs, jobID)
	}

	// Flush deferred audit events (best effort; batch-create so they share one CAS flush)
	storage.FlushPendingAuditEvents(ctx, projectRoot, jg.storage, jg.secCtx)

	const casFlushTimeout = 30 * time.Second
	queue := storage.GetListingIndexWriteQueueForProjectRoot(projectRoot)
	if err := queue.FlushKind(objects.KindSchedulerJob, casFlushTimeout); err != nil {
		return jobIDs, errfmt.Newf("flush scheduler_job CAS index after bulk create").Wrap(err)
	}
	if err := queue.FlushKind(objects.KindAuditEvent, casFlushTimeout); err != nil {
		return jobIDs, errfmt.Newf("flush audit_event CAS index after bulk create").Wrap(err)
	}

	// Enqueue all trigger requests in one batch (single lock) so the daemon runs them via the trigger queue.
	// Without this, when the daemon is already running it only reloads (reload=true) and
	// does not submit immediate one_time jobs, so newly created test bundles would never run.
	if projectRoot != emptyValue && len(jobIDs) > 0 {
		triggerQueue := schedcore.NewJobTriggerQueue(projectRoot)
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		if enqErr := triggerQueue.EnqueueTriggerRequests(jobIDs, ""); enqErr != nil {
			// Best-effort: log but do not fail the whole operation; jobs exist and may be picked up on next daemon load
			logging.Fluent(logger).Warn("Failed to enqueue trigger requests for test bundle jobs; daemon may not run these until next reload").
				String("error", enqErr.Error()).
				Int("job_count", len(jobIDs)).
				Log()
		}
	}

	return jobIDs, nil
}

// buildTestArgs builds the command arguments for running a test bundle
// The command will be wrapped in a shell to redirect output to the log file
func (jg *JobGenerator) buildTestArgs(bundle *testscan.TestBundle, projectRoot string, logFilePath string) []string {
	// Build the base go test command (must be "go test", not "test")
	testArgs := []string{"go", "test"}

	// Add package path
	if bundle.PackagePath != emptyValue {
		testArgs = append(testArgs, fmt.Sprintf("./%s", bundle.PackagePath))
	} else {
		testArgs = append(testArgs, "./...")
	}

	// Add -run flag with test names
	if len(bundle.Tests) > 0 {
		var testNames []string
		for _, test := range bundle.Tests {
			testNames = append(testNames, test.Name)
		}
		// Use regex pattern to match any of the test names
		pattern := strings.Join(testNames, "|")
		testArgs = append(testArgs, "-run", fmt.Sprintf("^(%s)$", pattern))
	}

	// Add verbose flag
	testArgs = append(testArgs, "-v")

	// Add count=1 to avoid caching
	testArgs = append(testArgs, "-count=1")

	// Add -timeout so go test exits with a clear "test timed out" and the running test name
	// before the job's max_runtime_seconds kills the process (same ceiling as job, minus lead).
	testTimeoutSec := computeTestBundleTimeoutSeconds(bundle, projectRoot)
	if testTimeoutSec > testBundleGoTestTimeoutLeadSeconds {
		testTimeoutSec -= testBundleGoTestTimeoutLeadSeconds
	}
	testArgs = append(testArgs, "-timeout", fmt.Sprintf("%ds", testTimeoutSec))

	// Set parallelism flag based on bundle type
	// Note: -p flag controls package-level parallelism in go test
	// Tests with t.Parallel() will run concurrently within the package regardless of -p
	if bundle.IsParallel {
		// For parallel-safe bundles, use GOMAXPROCS (typically 8+ on modern machines)
		// This allows multiple packages to be tested concurrently if needed
		maxProcs := runtime.GOMAXPROCS(0)
		if maxProcs < 1 {
			maxProcs = 8 // Fallback to 8 if GOMAXPROCS is unset
		}
		testArgs = append(testArgs, "-p", strconv.Itoa(maxProcs))
	} else {
		// For sequential bundles, force -p 1 to ensure tests run one at a time
		testArgs = append(testArgs, "-p", "1")
	}

	if shouldLimitGoTestParallelTests(bundle.PackagePath) {
		testArgs = append(testArgs, "-parallel", "1")
	}

	// Build the full command with output redirection
	// Use shell to redirect both stdout and stderr to the log file
	// Format: sh -c "go test ... > logfile.log 2>&1"
	// Important: Quote the -run regex pattern to prevent shell interpretation
	// The regex pattern contains ^, (, |, ), $ which are shell metacharacters
	var quotedArgs []string
	skipNext := false
	for i, arg := range testArgs {
		if skipNext {
			skipNext = false
			continue
		}
		// Quote the -run flag's value (the next argument) to protect regex patterns
		if arg == "-run" && i+1 < len(testArgs) {
			quotedArgs = append(quotedArgs, arg)
			// Quote the regex pattern value to prevent shell interpretation
			pattern := testArgs[i+1]
			// Use single quotes to prevent any shell interpretation
			// Escape any single quotes in the pattern by replacing ' with '\''
			quotedArgs = append(quotedArgs, fmt.Sprintf("'%s'", strings.ReplaceAll(pattern, "'", "'\"'\"'")))
			skipNext = true // Skip the next arg since we've already processed it
		} else {
			quotedArgs = append(quotedArgs, arg)
		}
	}
	testCmd := strings.Join(quotedArgs, " ")
	shellCmd := fmt.Sprintf("%s > %s 2>&1", testCmd, logFilePath)

	// Return as shell command
	return []string{"-c", shellCmd}
}

// buildDescription builds a description for the test bundle job
func (jg *JobGenerator) buildDescription(bundle *testscan.TestBundle, logFilePath string) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("Test bundle containing %d test(s) from package %s", len(bundle.Tests), bundle.PackagePath))

	if bundle.IsParallel {
		parts = append(parts, "All tests are parallel-safe")
	} else {
		parts = append(parts, "Tests must run sequentially")
	}

	parts = append(parts, fmt.Sprintf("Estimated duration: %v", bundle.EstimatedDuration.Round(time.Second)))

	if len(bundle.Tests) > 0 {
		var testNames []string
		for _, test := range bundle.Tests {
			testNames = append(testNames, test.Name)
		}
		parts = append(parts, fmt.Sprintf("Tests: %s", strings.Join(testNames, ", ")))
	}

	// Add log file information
	parts = append(parts, fmt.Sprintf("Log file: %s", logFilePath))

	return strings.Join(parts, ". ")
}

// ScanAndSchedule performs a full scan, bundle, and schedule operation
func ScanAndSchedule(ctx context.Context, projectRoot string, storage storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, maxBundleSize, maxParallel int) ([]string, error) {
	// Create scanner
	scanner := testscan.NewScanner(projectRoot)

	// Scan for tests
	tests, err := scanner.Scan()
	if err != nil {
		return nil, errfmt.Newf("failed to scan tests").Wrap(err)
	}

	if len(tests) == 0 {
		return nil, errfmt.Errorf("no tests found")
	}

	// Bundle tests
	bundles := scanner.BundleTests(tests, maxBundleSize)

	// Schedule bundles
	scheduledBundles := scanner.ScheduleBundles(bundles, maxParallel)

	if err := testscan.WritePackageConcurrencyLimitsPatch(projectRoot, scheduledBundles, maxParallel); err != nil {
		return nil, errfmt.Newf("write package concurrency limits").Wrap(err)
	}

	// Generate jobs
	generator := NewJobGenerator(storage, secCtx)
	jobIDs, err := generator.GenerateJobs(ctx, scheduledBundles, projectRoot, maxParallel)
	if err != nil {
		return nil, errfmt.Newf("failed to generate jobs").Wrap(err)
	}

	return jobIDs, nil
}
