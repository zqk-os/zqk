// Package testjobgen creates scheduler run_wrapper jobs from test bundles (scan-tests output).
// It lives outside pkg/testing so production code does not depend on the test helper module path.
package testjobgen

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/process"
	schedcore "github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testscan"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
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

// mustRecreateTestBundleJob reports whether an existing SCH-run-* job cannot be updated
// back to active. scheduler_job lifecycle allows active/disabled/pending (and transitions
// among them); archived and error are terminal. Legacy or mis-written statuses such as
// "completed" are also not valid sources — delete+recreate under the same stable ID.
func mustRecreateTestBundleJob(ctx context.Context, store storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, jobID string) bool {
	prev, err := store.Read(ctx, secCtx, jobID)
	if err != nil || prev == nil {
		return false
	}
	status, _ := prev[objects.FieldKeyStatus].(string)
	switch status {
	case schedcore.StatusActive, "disabled", "pending", emptyValue:
		return false
	case objects.ObjectStatusArchived, objects.ObjectStatusError,
		objects.ObjectStatusCompleted, "complete":
		return true
	default:
		// Unknown / invalid source status (e.g. historical litter) — recreate.
		return true
	}
}

// GenerateJobs creates scheduler jobs for test bundles.
// Reuses an existing job when one exists for the same bundle ID (stable ID SCH-run-<bundle.ID>):
// updates that job with the current bundle definition and re-enables it so it runs again.
// Terminal statuses (archived/error) are delete+recreate under the same stable ID.
// Otherwise creates a new job. This keeps the job store from growing with redundant run_wrapper jobs.
// Uses bulk-create mode: defers CAS index flush until all jobs (and their audit events) are created,
// then flushes once to reduce I/O and wait-bound behavior.
// GenerateJobs creates/updates SCH-run-* jobs.
// studioRoot owns job objects, test-bundle logs, triggers, and concurrency-limit patches.
// sourceRoot is the go test working_directory (Local CI worktree). Empty sourceRoot means studioRoot.
func (jg *JobGenerator) GenerateJobs(ctx context.Context, bundles []*testscan.TestBundle, studioRoot, sourceRoot string, maxParallel int) ([]string, error) {
	var jobIDs []string
	now := time.Now().UTC()

	projectRoot := studioRoot
	if sourceRoot == emptyValue {
		sourceRoot = studioRoot
	}

	// Resolve to absolute so run_wrapper uses it as cmd.Dir regardless of scheduler CWD
	absSourceRoot := sourceRoot
	if absSourceRoot != emptyValue {
		if p, err := filepath.Abs(sourceRoot); err == nil {
			absSourceRoot = p
		}
	}

	// All test-bundle logs stay under studio (observability), not the CI worktree.
	logDir := schedcore.JobLogsTestBundlesDir(studioRoot)
	if err := fileutil.MkdirAll(logDir, paths.DirPerm755); err != nil {
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
		// Stable ID SCH-run-<bundle.ID> so scan-tests reuses one job per bundle (no UnixNano litter).
		// If a prior job is archived/error (no lifecycle path back to active), delete then recreate.
		jobID := fmt.Sprintf("%s%s", stableJobIDPrefix, sanitizeBundleIDForJobID(bundle.ID))

		logFileName := fmt.Sprintf("bundle-%s.log", bundle.ID)
		logFilePath := filepath.Join(logDir, logFileName)

		// Make path absolute for shell redirection
		absLogFilePath, err := filepath.Abs(logFilePath)
		if err != nil {
			return jobIDs, errfmt.Newf("failed to resolve absolute log path").Wrap(err)
		}
		logFilePath = absLogFilePath

		// Build test command arguments (timeouts/package layout from source root)
		args := jg.buildTestArgs(bundle, sourceRoot, logFilePath)
		newCmdStr := schedcore.FormatRunWrapperCommandString(schedcore.RunWrapperShellCommand, args)
		newFP := schedcore.FingerprintBundleCommand(newCmdStr)

		timeout := computeTestBundleTimeoutSeconds(bundle, sourceRoot)

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
			objects.FieldKeyWorkingDirectory:  absSourceRoot,
			objects.FieldKeyPriority:          jobPri,
			objects.FieldKeyMetadata:          metadata,
		}

		// Reuse existing job for this bundle if present (same stable ID).
		// Use Exists (O(1) CAS index lookup) instead of List (O(N) full YAML read) so the
		// N-bundle loop stays O(N) total instead of O(N²) as created files accumulate.
		jobExists, existsErr := jg.storage.Exists(ctx, jg.secCtx, jobID)
		if existsErr == nil && jobExists {
			if mustRecreateTestBundleJob(ctx, jg.storage, jg.secCtx, jobID) {
				// TRACK: open-core / scheduler_job lifecycle — archived|error|completed (invalid)
				// and other non-reactivating statuses: delete+recreate until a documented
				// reactivation transition exists for test-bundle reuse.
				// Leaf set-delete (BulkDelete) skips findDependents — needed because
				// run_wrapper jobs often retain graph/audit edges that block Delete even
				// with --unlink-references. Same stable ID is recreated below.
				delCtx := storage.WithCLIOperation(ctx)
				br, delErr := jg.storage.BulkDelete(delCtx, jg.secCtx, []string{jobID}, false)
				if delErr != nil {
					return jobIDs, errfmt.Errorf("failed to delete terminal job %s for bundle %s before recreate: %w", jobID, bundle.ID, delErr)
				}
				if br != nil && br.FailureCount > 0 && br.SuccessCount == 0 {
					msg := "bulk delete reported failure"
					if len(br.Errors) > 0 {
						msg = br.Errors[0].Message
					}
					return jobIDs, errfmt.Errorf("failed to delete terminal job %s for bundle %s before recreate: %s", jobID, bundle.ID, msg)
				}
			} else {
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
					objects.FieldKeyWorkingDirectory:  absSourceRoot,
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
		}

		// Create the job (CAS flush deferred until after loop)
		if err := jg.storage.Create(pkgctx.WithPromoteOnCreate(ctx), jg.secCtx, jobData); err != nil {
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
					objects.FieldKeyWorkingDirectory:  absSourceRoot,
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
	queue := caspkg.GetListingIndexWriteQueueForProjectRoot(projectRoot)
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

// contaminationGuardPath returns the schema-plane guard for projectRoot, or empty when
// it is missing so a tree without the script still runs its bundles.
func contaminationGuardPath(projectRoot string) string {
	if projectRoot == emptyValue {
		return emptyValue
	}
	guard := filepath.Join(projectRoot, paths.ScriptsDir, "check-test-contamination.sh")
	if info, err := fileutil.Stat(guard); err != nil || info.IsDir() {
		return emptyValue
	}
	return guard
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

	// Bound this job's own fan-out so the host-level budget means something.
	//
	// The scheduler caps how many of these jobs run at once, but that cap only bounds total load if
	// each job's thread count is known. Both go test knobs previously defaulted to GOMAXPROCS: -p ran
	// up to that many packages at once, and -parallel (unset except for a hand-picked list of
	// packages) let t.Parallel tests fan out just as wide "regardless of -p", as this code already
	// noted. One job could therefore occupy the whole machine, and several of them drove a 10-core
	// host to a load average of 15.
	jobParallelism := concurrency.GetGlobalConcurrencyConfig().SchedulerTestJobParallelism
	if jobParallelism < 1 {
		jobParallelism = 1
	}
	if bundle.IsParallel {
		testArgs = append(testArgs, "-p", strconv.Itoa(jobParallelism))
	} else {
		// Sequential bundles run one package at a time by contract, not as a load control.
		testArgs = append(testArgs, "-p", "1")
	}

	// Packages known to be parallel-unsafe stay pinned at 1; everything else gets the shared bound
	// rather than GOMAXPROCS, so worst-case threads are (concurrent jobs * jobParallelism).
	if shouldLimitGoTestParallelTests(bundle.PackagePath) {
		testArgs = append(testArgs, "-parallel", "1")
	} else {
		testArgs = append(testArgs, "-parallel", strconv.Itoa(jobParallelism))
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

	// Observe the schema plane around every bundle. A test that writes into
	// .zqk/specs replaces the ontology that every later reader validates
	// against, and no test asserts about that directory, so only the surrounding
	// run can notice. The guard exits non-zero when the plane moved, which fails the
	// bundle and names the affected files in this log.
	if guard := contaminationGuardPath(projectRoot); guard != emptyValue {
		testCmd = fmt.Sprintf("sh %s %s", guard, testCmd)
	}

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
	jobIDs, err := generator.GenerateJobs(ctx, scheduledBundles, projectRoot, projectRoot, maxParallel)
	if err != nil {
		return nil, errfmt.Newf("failed to generate jobs").Wrap(err)
	}

	return jobIDs, nil
}
