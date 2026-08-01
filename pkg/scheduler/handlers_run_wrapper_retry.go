
package scheduler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"github.com/lanceman/zqk/pkg/syscallutil"
	"syscall"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/gotestparse"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/testenvroot"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/lanceman/zqk/pkg/zqktime"
)

const (
	runWrapperLogLevelVerbose   = "verbose"
	runWrapperLogLevelDebug     = "debug"
	runWrapperEventStarted      = "started"
	runWrapperEventCompleted    = "completed"
	runWrapperEventFailed       = "failed"
	runWrapperEventTimeout      = "timeout"
	runWrapperTestOutcomePass   = "pass"
	runWrapperTestOutcomeFail   = "fail"
	runWrapperTestOutcomeTF     = "test_fail"
	runWrapperFieldAttempt      = "attempt"
	runWrapperFieldDuration     = "duration"
	runWrapperFieldOutput       = "output"
	runWrapperFieldFailedTests  = "failed_tests"
	runWrapperShellPath         = "/bin/sh"
	runWrapperShellFlagC        = "-c"
	runWrapperEnvKVFmt          = "%s=%s"
	runWrapperOutputTruncated   = "... (truncated)"
	runWrapperFieldMaxAttempts  = "max_attempts"
	runWrapperFieldTotalTests   = "total_tests"
	runWrapperFieldPassedTests  = "passed_tests"
	runWrapperFieldSkippedTests = "skipped_tests"
	runWrapperStatusStarted     = "started"
)

// effectiveRunWrapperTimeoutSeconds resolves the subprocess timeout used for WithTimeout,
// timeout diagnostics in [runWrapperLogFailedAttempt], and failure classification.
//
// Encoding (first non-zero wins, else default): dynamic maxRuntimeSeconds (bundle/test resolution),
// then job.MaxRuntimeSeconds, then DefaultMaxRuntimeSeconds so hung commands are always bounded.
// Prefer this helper over copy-pasting the fallback chain—it is easy to drift otherwise (duplicate
// when.Then branches, mismatched semantics between context deadline and logged max_runtime_seconds).
func effectiveRunWrapperTimeoutSeconds(maxRuntimeSeconds int, job *ScheduledJob) int {
	if maxRuntimeSeconds != 0 {
		return maxRuntimeSeconds
	}
	if job.MaxRuntimeSeconds != 0 {
		return job.MaxRuntimeSeconds
	}
	return DefaultMaxRuntimeSeconds
}

// runWrapperProcState tracks the subprocess for cleanup across retry attempts.
type runWrapperProcState struct {
	cmd            Cmd
	processGroupID int
	subprocessID   string
	reaped         bool
	progressTicker *time.Ticker
	progressDone   chan bool
}

// runWrapperRetryArgs carries state into runWrapperRetryAttempts (executeRunWrapperCore retry loop).
type runWrapperRetryArgs struct {
	cmdStr               string
	command              string
	args                 []string
	isShellScript        bool
	workingDir           string
	retryCount           int
	retryDelay           time.Duration
	maxRuntimeSeconds    int
	streamedToFiles      bool
	writeEventEntry      func(map[string]any)
	isTestBundle         bool
	stdoutStreamFile     *os.File
	stderrStreamFile     *os.File
	testRootsToClean     *[]string
	currentProcess       *runWrapperProcState
	scriptForExec        string
	terminalEntryWritten *bool
	lastErr              *error
	testFailures         *[]string
	testSummary          *map[string]any
}

// runWrapperRecordSuccessfulAttempt writes completion events, notifications, and syncs retry state when the command exits 0.
func (h *RunWrapperHandler) runWrapperRecordSuccessfulAttempt(
	ctx context.Context,
	job *ScheduledJob,
	a *runWrapperRetryArgs,
	cmdStr string,
	duration time.Duration,
	attempt int,
	stdoutWriter, stderrWriter *streamingOutputWriter,
	writeEventEntry func(map[string]any),
	streamedToFiles, isTestBundle bool,
	lastErr error,
	testFailures []string,
	testSummary map[string]any,
	testRootsToClean []string,
	cancel context.CancelFunc,
) bool {
	successEntry := map[string]any{
		KeyTimestamp: zqktime.NowRFC3339UTC(),
		KeyJobID:     job.ID,
		KeyEventType: runWrapperEventCompleted,
		KeyCommand:   cmdStr,
		KeyDuration:  duration.Seconds(),
		KeyAttempt:   attempt + 1,
		KeyExitCode:  0,
		KeySuccess:   true,
	}
	if stdoutWriter.Len() > 0 && (job.LogLevel == runWrapperLogLevelVerbose || job.LogLevel == runWrapperLogLevelDebug) {
		successEntry[KeyStdout] = stdoutWriter.Preview()
	}
	fp := FingerprintBundleCommand(cmdStr)
	successEntry[KeyBundleCommandFingerprint] = fp
	if isTestBundle {
		healthOutcome := "ok"
		testsFailed := 0
		if h.isTestCommand(job.Command, job.CommandArgs) {
			healthOutcome = runWrapperTestOutcomePass
			testOut := gatherTestOutputForParsing(streamedToFiles, h.projectRoot, job.ID, stdoutWriter, stderrWriter)
			if summary, parseErr := gotestparse.ParseGoTestOutput(testOut); parseErr == nil {
				successEntry[KeyTestSummary] = map[string]any{
					runWrapperFieldTotalTests:       summary.TotalTests,
					runWrapperFieldPassedTests:      summary.PassedCount,
					runWrapperFieldFailedTests:      summary.FailedCount,
					runWrapperFieldSkippedTests:     summary.SkippedCount,
					objects.FieldKeyDurationSeconds: summary.Duration.Seconds(),
				}
				testsFailed = summary.FailedCount
				successEntry[KeyTestOutcome] = runWrapperTestOutcomePass
				if summary.FailedCount > 0 {
					successEntry[KeyTestOutcome] = runWrapperTestOutcomeTF
					healthOutcome = runWrapperTestOutcomeTF
					names := summary.GetFailedTestNames()
					successEntry[KeyTestFailures] = names
					ts := effectiveRunWrapperTimeoutSeconds(a.maxRuntimeSeconds, job)
					if sug := BuildSuggestedGoTestRerunCommands(cmdStr, job.Command, job.CommandArgs, names, ts); len(sug) > 0 {
						successEntry[KeySuggestedRerunCommands] = sug
					}
					h.maybePersistCVSPerTestFailureLedger(ctx, job, fp, names, false)
				} else {
					h.maybePersistCVSPerTestFailureLedger(ctx, job, fp, nil, true)
				}
			} else {
				successEntry[KeyTestOutcome] = runWrapperTestOutcomePass
			}
		}
		AppendTestBundleHealthEvent(h.projectRoot, job.ID, BuildTestBundleHealthEntry(job.ID, JobLogEventCompleted, healthOutcome, testsFailed, fp, nil))
		h.emitBundleProgress(ctx, job, healthOutcome)
		MaybeAppendTestBundleCriteriaVerificationEvidence(h.projectRoot, job, healthOutcome, testsFailed, fp)
		h.maybeAutoValidateCriteriaFromSatisfiedTestBundle(ctx, job, healthOutcome, testsFailed)
		h.maybeRunConvergenceTickAfterTestBundleHealth(ctx, job)
	}
	writeEventEntry(successEntry)
	terminalEntryWritten := true
	if !streamedToFiles {
		writeSeparateJobLogsIfConfigured(h.projectRoot, job.ID, stdoutWriter.Preview(), stderrWriter.Preview())
	}

	RunWrapperLog(h.logger).Info(LogEventRunWrapperCommandSucceeded).
		JobID(job.ID).
		Command(cmdStr).
		String(runWrapperFieldDuration, duration.String()).
		Int(runWrapperFieldAttempt, attempt+1).
		Log()
	if stdoutWriter.Len() > 0 {
		preview := stdoutWriter.Preview()
		when.When(func() bool {
			return job.LogLevel == runWrapperLogLevelVerbose || job.LogLevel == runWrapperLogLevelDebug
		}).Then(func() {
			RunWrapperLog(h.logger).Info(LogEventRunWrapperCommandStdout).
				JobID(job.ID).
				String(runWrapperFieldOutput, preview).
				Log()
		}).OrElse(func() {
			RunWrapperLog(h.logger).Debug(LogEventRunWrapperCommandStdout).
				JobID(job.ID).
				String(runWrapperFieldOutput, preview).
				Log()
		}).Run()
	}

	notif := CreateJobNotification(
		job.ID,
		job.JobType,
		job.Category,
		runWrapperEventCompleted,
		PriorityMedium,
		duration,
		nil,
		map[string]any{
			KeyCommand:        cmdStr,
			KeyStdout:         stdoutWriter.Preview(),
			KeyExitCode:       0,
			KeyJobTitle:       job.Title,
			KeyJobDescription: job.Description,
		},
	)
	h.notificationContext.Notify(notif)

	if TriggerOriginFromContext(ctx) == TriggerOriginPreCommit {
		h.executeCallback(ctx, job, callbackTypeCompletion, map[string]any{
			KeyJobID:    job.ID,
			KeyCommand:  cmdStr,
			KeyDuration: duration.Seconds(),
			KeySuccess:  true,
			KeyStdout:   stdoutWriter.Preview(),
			KeyExitCode: 0,
		})
	}

	completionMsg := CreateCompletionMessage(job, duration, stdoutWriter.Preview(), "", 0)
	h.routeAsync(ctx, completionMsg)
	if cancel != nil {
		cancel()
	}
	*a.lastErr = lastErr
	*a.testFailures = testFailures
	*a.testSummary = testSummary
	*a.terminalEntryWritten = terminalEntryWritten
	*a.testRootsToClean = testRootsToClean
	return true
}

// runWrapperLogFailedAttempt classifies failure (timeout vs exit), writes timeout events when applicable, and logs stderr preview.
func (h *RunWrapperHandler) runWrapperLogFailedAttempt(
	ctx context.Context,
	cmdCtx context.Context,
	job *ScheduledJob,
	cmdStr string,
	attempt int,
	duration time.Duration,
	stdoutWriter, stderrWriter *streamingOutputWriter,
	writeEventEntry func(map[string]any),
	streamedToFiles, isTestBundle bool,
	workingDir string,
	maxRuntimeSeconds, exitCode int,
	stderrPreviewForReason string,
	waitErr, lastErr error,
	terminalEntryWritten bool,
) (bool, string, string, error) {
	var failureKind, failureReason string
	when.When(func() bool { return cmdCtx.Err() == context.DeadlineExceeded }).Then(func() {
		timeoutSeconds := effectiveRunWrapperTimeoutSeconds(maxRuntimeSeconds, job)
		failureKind, failureReason = failureKindAndReason(0, stderrPreviewForReason, true, timeoutSeconds)

		stdoutPreview := stdoutWriter.Preview()
		stderrPreview := stderrWriter.Preview()
		if h.isTestCommand(job.Command, job.CommandArgs) {
			stdoutPreview = h.sanitizeTestOutput(stdoutPreview)
			stderrPreview = h.sanitizeTestOutput(stderrPreview)
		}

		if len(stdoutPreview) > 500 {
			stdoutPreview = stdoutPreview[:500] + runWrapperOutputTruncated
		}
		if len(stderrPreview) > 500 {
			stderrPreview = stderrPreview[:500] + runWrapperOutputTruncated
		}

		timeoutFields := []logging.Field{
			logging.String("job_id", job.ID),
			logging.String(KeyCommand, cmdStr),
			logging.Int("max_runtime_seconds", timeoutSeconds),
			logging.Int(runWrapperFieldAttempt, attempt+1),
			logging.String(runWrapperFieldDuration, duration.String()),
			logging.String("duration_seconds", fmt.Sprintf("%.2f", duration.Seconds())),
		}

		if stdoutPreview != emptyValue {
			timeoutFields = append(timeoutFields, logging.String("stdout_preview", stdoutPreview))
		}
		if stderrPreview != emptyValue {
			timeoutFields = append(timeoutFields, logging.String("stderr_preview", stderrPreview))
		}

		timeoutFields = append(timeoutFields,
			logging.String("diagnostic_note", "Command timed out - may be waiting for: file locks, network I/O, database connections, or other blocking operations"),
		)

		if workingDir != emptyValue {
			timeoutFields = append(timeoutFields, logging.String("working_directory", workingDir))
		}

		timeoutEntry := map[string]any{
			KeyTimestamp:      zqktime.NowRFC3339UTC(),
			KeyJobID:          job.ID,
			KeyEventType:      runWrapperEventTimeout,
			KeyFailureKind:    failureKind,
			KeyFailureReason:  failureReason,
			KeyCommand:        cmdStr,
			KeyDuration:       duration.Seconds(),
			KeyAttempt:        attempt + 1,
			KeyTimeoutSeconds: timeoutSeconds,
			KeySuccess:        false,
			KeyError:          failureReason,
		}
		if stdoutWriter.Len() > 0 && (job.LogLevel == runWrapperLogLevelVerbose || job.LogLevel == runWrapperLogLevelDebug) {
			timeoutEntry[KeyStdout] = stdoutWriter.Preview()
		}
		if stderrWriter.Len() > 0 {
			timeoutEntry[KeyStderr] = stderrWriter.Preview()
		}
		tfp := FingerprintBundleCommand(cmdStr)
		timeoutEntry[KeyBundleCommandFingerprint] = tfp
		timeoutEntry[KeyTestOutcome] = runWrapperEventTimeout
		if isTestBundle {
			AppendTestBundleHealthEvent(h.projectRoot, job.ID, BuildTestBundleHealthEntry(job.ID, JobLogEventTimeout, runWrapperEventTimeout, 0, tfp, nil))
			h.maybeRunConvergenceTickAfterTestBundleHealth(ctx, job)
		}
		writeEventEntry(timeoutEntry)
		terminalEntryWritten = true
		if !streamedToFiles {
			writeSeparateJobLogsIfConfigured(h.projectRoot, job.ID, stdoutWriter.Preview(), stderrWriter.Preview())
		}

		RunWrapperLog(h.logger).Warn(LogEventRunWrapperCommandTimedOut).
			WithFields(timeoutFields...).
			Log()
		lastErr = errfmt.Errorf("command timed out after %d seconds: %w", timeoutSeconds, waitErr)
	}).OrElse(func() {
		failureKind, failureReason = failureKindAndReason(exitCode, stderrPreviewForReason, false, 0)
		RunWrapperLog(h.logger).Warn(LogEventRunWrapperCommandFailed).
			WithFields(jobLogFieldsForFailedCommand(job, cmdStr, failureKind, failureReason, exitCode, attempt+1, duration, waitErr)...).
			Log()

		// Circuit Breaker: If we hit too many failures, pause the handler to protect the daemon
		h.failureCount++
		if h.failureCount > 10 {
			RunWrapperLog(h.logger).Error("Circuit breaker tripped: too many consecutive failures. Quarantining job.", errfmt.Errorf("job_id: %s", job.ID)).Log()
			if h.scheduler != nil {
				h.scheduler.DisableJobInStorage(ctx, job)
			}
		}
	}).Run()

	if stderrWriter.Len() > 0 {
		stderrOutput := stderrWriter.Preview()
		if h.isTestCommand(job.Command, job.CommandArgs) {
			stderrOutput = h.sanitizeTestOutput(stderrOutput)
		}
		if stderrOutput != emptyValue {
			when.When(func() bool {
				return job.LogLevel == runWrapperLogLevelVerbose || job.LogLevel == runWrapperLogLevelDebug
			}).Then(func() {
				RunWrapperLog(h.logger).Info(LogEventRunWrapperCommandStderr).
					JobID(job.ID).
					String("error_output", stderrOutput).
					Log()
			}).OrElse(func() {
				RunWrapperLog(h.logger).Debug(LogEventRunWrapperCommandStderr).
					JobID(job.ID).
					String("error_output", stderrOutput).
					Log()
			}).Run()
		}
	}

	return terminalEntryWritten, failureKind, failureReason, lastErr
}

// runWrapperCollectTestFailuresFromOutput parses go test output after retries are exhausted (best-effort).
func (h *RunWrapperHandler) runWrapperCollectTestFailuresFromOutput(
	job *ScheduledJob,
	streamedToFiles bool,
	stdoutWriter, stderrWriter *streamingOutputWriter,
	errorStderr string,
) ([]string, map[string]any) {
	isTestCommand := h.isTestCommand(job.Command, job.CommandArgs)
	if !isTestCommand {
		return nil, nil
	}
	var testOutput string
	if streamedToFiles && h.projectRoot != emptyValue && job.ID != emptyValue {
		stdoutPath := JobStdoutFilePath(h.projectRoot, job.ID)
		stderrPath := JobStderrFilePath(h.projectRoot, job.ID)
		const perStream = maxBytesToReadForTestParse / 2
		if b, errRead := readLastBytesFromFile(stdoutPath, perStream); errRead == nil && len(b) > 0 {
			testOutput = string(b)
		} else if errRead != nil && !os.IsNotExist(errRead) {
			RunWrapperLog(h.logger).Debug("Failed to read stdout for test parsing").WithError(errRead).Log()
		}
		if b, errRead := readLastBytesFromFile(stderrPath, perStream); errRead == nil && len(b) > 0 {
			when.When(func() bool { return testOutput != emptyValue }).Then(func() {
				testOutput += "\n" + string(b)
			}).OrElse(func() {
				testOutput = string(b)
			}).Run()
		} else if errRead != nil && !os.IsNotExist(errRead) {
			RunWrapperLog(h.logger).Debug("Failed to read stderr for test parsing").WithError(errRead).Log()
		}
	}
	if testOutput == emptyValue {
		testOutput = stdoutWriter.Preview()
		if errorStderr != emptyValue {
			when.When(func() bool { return testOutput != emptyValue }).Then(func() {
				testOutput += "\n" + errorStderr
			}).OrElse(func() {
				testOutput = errorStderr
			}).Run()
		}
	}

	if summary, err := gotestparse.ParseGoTestOutput(trimToLastSchedulerTestRunForParsing(testOutput)); err == nil {
		return summary.GetFailedTestNames(), map[string]any{
			runWrapperFieldTotalTests:       summary.TotalTests,
			runWrapperFieldPassedTests:      summary.PassedCount,
			runWrapperFieldFailedTests:      summary.FailedCount,
			runWrapperFieldSkippedTests:     summary.SkippedCount,
			objects.FieldKeyDurationSeconds: summary.Duration.Seconds(),
		}
	}
	return nil, nil
}

// runWrapperNotifyExhaustedFailure sends user notification, pre_commit error callback, and async routing after all retries fail.
func (h *RunWrapperHandler) runWrapperNotifyExhaustedFailure(
	ctx context.Context,
	job *ScheduledJob,
	cmdStr string,
	attempt, retryCount int,
	duration time.Duration,
	errorStderr string,
	lastErr error,
	failureKind, failureReason string,
	exitCode, cmdTimeoutSeconds int,
	isTestCommand bool,
	testFailures []string,
	testSummary map[string]any,
) {
	notificationMetadata := map[string]any{
		KeyCommand:        cmdStr,
		KeyFailureKind:    failureKind,
		KeyFailureReason:  failureReason,
		KeyStderr:         errorStderr,
		KeyAttempts:       retryCount + 1,
		KeyExitCode:       exitCode,
		KeyJobTitle:       job.Title,
		KeyJobDescription: job.Description,
	}

	var notificationErr error
	when.When(func() bool { return isTestCommand && exitCode == 1 }).Then(func() {
		if len(testFailures) > 0 {
			notificationMetadata[KeyTestFailures] = testFailures
			notificationMetadata[KeyTestSummary] = testSummary
		}
		notificationMetadata["is_test_failure"] = true
		notificationErr = nil
		RunWrapperLog(h.logger).Info(LogEventRunWrapperTestFailureFlagSet).
			JobID(job.ID).
			Bool("is_test_command", isTestCommand).
			Int("exit_code", exitCode).
			Int("test_failures_count", len(testFailures)).
			Log()
	}).OrElse(func() {
		notificationErr = lastErr
	}).Run()

	notif := CreateJobNotification(
		job.ID,
		job.JobType,
		job.Category,
		runWrapperEventFailed,
		PriorityHigh,
		duration,
		notificationErr,
		notificationMetadata,
	)
	h.notificationContext.Notify(notif)

	errorPayload := map[string]any{
		KeyJobID:         job.ID,
		KeyCommand:       cmdStr,
		KeyFailureKind:   failureKind,
		KeyFailureReason: failureReason,
		KeySuccess:       false,
		KeyError:         lastErr.Error(),
		KeyStderr:        errorStderr,
		KeyAttempts:      retryCount + 1,
		KeyExitCode:      exitCode,
	}

	if len(testFailures) > 0 {
		errorPayload[KeyTestFailures] = testFailures
		errorPayload[KeyTestSummary] = testSummary
	}
	errorPayload[KeyBundleCommandFingerprint] = FingerprintBundleCommand(cmdStr)
	if len(testFailures) > 0 {
		if sug := BuildSuggestedGoTestRerunCommands(cmdStr, job.Command, job.CommandArgs, testFailures, cmdTimeoutSeconds); len(sug) > 0 {
			errorPayload[KeySuggestedRerunCommands] = sug
		}
	}

	if TriggerOriginFromContext(ctx) == TriggerOriginPreCommit {
		h.executeCallback(ctx, job, callbackTypeError, errorPayload)
	}

	var errorMsgForRouting string
	when.When(func() bool { return len(testFailures) > 0 }).Then(func() {
		errorMsgForRouting = fmt.Sprintf("test bundle ran successfully but %d test(s) failed", len(testFailures))
	}).OrElse(func() {
		errorMsgForRouting = lastErr.Error()
	}).Run()
	errorMsg := CreateErrorMessage(job, duration, errorMsgForRouting, errorStderr, retryCount+1, exitCode, failureKind, failureReason)
	h.routeAsync(ctx, errorMsg)
}

func (h *RunWrapperHandler) getExecutor() CommandExecutor {
	if h.scheduler != nil {
		return h.scheduler.GetExecutor()
	}
	return &NativeExecutor{}
}

func (h *RunWrapperHandler) runWrapperRetryAttempts(ctx context.Context, job *ScheduledJob, a *runWrapperRetryArgs) bool {
	cmdStr := a.cmdStr
	command := a.command
	args := a.args
	isShellScript := a.isShellScript
	workingDir := a.workingDir
	retryCount := a.retryCount
	retryDelay := a.retryDelay
	maxRuntimeSeconds := a.maxRuntimeSeconds
	streamedToFiles := a.streamedToFiles
	writeEventEntry := a.writeEventEntry
	isTestBundle := a.isTestBundle
	stdoutStreamFile := a.stdoutStreamFile
	stderrStreamFile := a.stderrStreamFile
	testRootsToClean := *a.testRootsToClean
	currentProcess := a.currentProcess
	scriptForExec := a.scriptForExec
	var lastErr error
	var testFailures []string
	var testSummary map[string]any
	terminalEntryWritten := *a.terminalEntryWritten

	for attempt := 0; attempt <= retryCount; attempt++ {
		if attempt > 0 {
			RunWrapperLog(h.logger).Info(LogEventRunWrapperRetryingCommand).
				JobID(job.ID).
				Command(cmdStr).
				Int(runWrapperFieldAttempt, attempt+1).
				Int(runWrapperFieldMaxAttempts, retryCount+1).
				Log()
			select {
			case <-ctx.Done():
				return false
			case <-time.After(retryDelay):
			}
		}
		// Create command with timeout context (same resolution as timeout diagnostics—[effectiveRunWrapperTimeoutSeconds]).
		cmdTimeoutSeconds := effectiveRunWrapperTimeoutSeconds(maxRuntimeSeconds, job)
		cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(cmdTimeoutSeconds)*time.Second)

		// Create exec.Command
		// If command is a shell script, execute it through /bin/sh -c.
		// When streamedToFiles we may have stripped "> path 2>&1" so output goes to our writers.
		var cmd Cmd
		runScript := command
		if !isShellScript && isShellWithInlineScript(command, args) {
			if len(args) >= 2 && args[0] == runWrapperShellFlagC {
				runScript = args[1]
				if len(args) > 2 {
					runScript = strings.Join(args[1:], " ")
				}
			} else {
				runScript = args[2]
				if len(args) > 3 {
					runScript = strings.Join(args[2:], " ")
				}
			}
		}
		if scriptForExec != emptyValue {
			runScript = scriptForExec
		}
		// Tag process with job ID for visibility in ps/top/Activity Monitor (e.g. "zqk [scheduler: SCH-xxx]")
		// Strategy: Always wrap in shell that sets ZQK_JOB_ID env var so it shows in command line.
		// For shell scripts: prepend env assignment to script.
		// For direct commands: wrap in shell that sets env and exec's original command.
		jobTag := fmt.Sprintf(runWrapperEnvKVFmt, zqkenv.JobID(), job.ID)
		jobComment := fmt.Sprintf("# zqk-job: %s", job.ID)

		cmd = when.Result[Cmd]().
			When(func() bool { return isShellScript }).Then(func() Cmd {
			// Prepend env var assignment and comment to script so it shows in ps
			taggedScript := fmt.Sprintf("%s %s\n%s", jobTag, jobComment, runScript)
			return h.getExecutor().CommandContext(cmdCtx, runWrapperShellPath, runWrapperShellFlagC, taggedScript)
		}).
			OrElseWhen(func() bool { return isShellWithInlineScript(command, args) }).Then(func() Cmd {
			shellBin := command
			// 3-arg form has shell path in args[0]; 2-arg form has runWrapperShellFlagC in args[0]
			if len(args) >= 3 && strings.HasPrefix(args[0], "/") {
				shellBin = args[0] // prefer absolute path from args so we run the same binary the job specified
			}
			// Prepend env var assignment and comment to script so it shows in ps
			taggedScript := fmt.Sprintf("%s %s\n%s", jobTag, jobComment, runScript)
			return h.getExecutor().CommandContext(cmdCtx, shellBin, runWrapperShellFlagC, taggedScript)
		}).
			OrElse(func() Cmd {
				// For direct commands, wrap in shell that sets env and exec's original command.
				// This ensures ZQK_JOB_ID shows in ps/Activity Monitor command line and is safe for all commands.
				// Format: /bin/sh -c "ZQK_JOB_ID=SCH-xxx exec /path/to/command args..."
				// The exec replaces the shell process, so the command runs directly (no extra process).
				// exec resolves command via PATH or relative to working directory (set via cmd.Dir below).
				escapedArgs := make([]string, len(args))
				for i, arg := range args {
					// Escape single quotes in args for shell safety
					escapedArgs[i] = strings.ReplaceAll(arg, "'", "'\"'\"'")
				}
				// Escape command path for shell (handle spaces, special chars)
				escapedCmd := strings.ReplaceAll(command, "'", "'\"'\"'")
				execCmdStr := fmt.Sprintf("%s exec %s", jobTag, escapedCmd)
				if len(escapedArgs) > 0 {
					execCmdStr += " '" + strings.Join(escapedArgs, "' '") + "'"
				}
				return h.getExecutor().CommandContext(cmdCtx, runWrapperShellPath, runWrapperShellFlagC, execCmdStr)
			}).Run()

		// CRITICAL: Set process group to ensure child processes are killed on timeout
		// This prevents process leaks when commands spawn subprocesses
		// Setpgid: true creates a new process group, allowing us to kill the entire group
		cmd.SetSysProcAttr(&syscall.SysProcAttr{
			
		})

		// Set working directory: explicit job value, or project root so jobs run in project context
		if workingDir != emptyValue {
			cmd.SetDir(workingDir)
		} else if h.projectRoot != emptyValue {
			cmd.SetDir(h.projectRoot)
		}

		// Set environment variables (POLICY-CODE-006: isolate test runs from project data)
		env := os.Environ()
		// Ensure PATH includes 'go' for any job that might run go (tests, pre-commit-policy, pre-commit-lint)
		// so scripts like check-architecture-compliance.sh can run "go build" when daemon has minimal PATH (e.g. launchd).
		env = ensureEnvHasGoForTests(env)
		if h.isTestCommand(command, args) {
			testRoot, err := os.MkdirTemp("", "zqk-test-job-")
			if err == nil {
				testRootsToClean = append(testRootsToClean, testRoot)
				if err := testenvroot.BootstrapRoot(testRoot, h.projectRoot); err != nil {
					RunWrapperLog(h.logger).Warn(LogEventRunWrapperBootstrapTestRootFailed).
						String("test_root", testRoot).
						WithFields(logErrField(err)...).
						Log()
				}
				env = append(env, fmt.Sprintf(runWrapperEnvKVFmt, zqkenv.TestRoot(), testRoot))
			}
			// Let tests use the same zqk binary so object tests (e.g. TestUpdateFileHashValidation) skip per-test go build and avoid timeouts
			if exe, err := os.Executable(); err == nil && exe != emptyValue {
				if _, err := os.Stat(exe); err == nil {
					env = append(env, fmt.Sprintf(runWrapperEnvKVFmt, zqkenv.TestCLIBinary(), exe))
				}
			}
		}
		env = append(env, "ZQK_API_KEY=account:system")
		if len(job.EnvironmentVariables) > 0 {
			for key, value := range job.EnvironmentVariables {
				env = append(env, fmt.Sprintf(runWrapperEnvKVFmt, key, value))
			}
		}
		// Always set ZQK_JOB_ID so child processes can identify the scheduler job
		env = append(env, fmt.Sprintf(runWrapperEnvKVFmt, zqkenv.JobID(), job.ID))
		// Set ZQK_BIN for child scripts using the same resolution policy as scheduler subprocesses.
		env = append(env, fmt.Sprintf(runWrapperEnvKVFmt, zqkenv.Bin(), resolveSchedulerCLIBinary(h.projectRoot)))
		// Enforce system authentication for scheduler jobs
		env = append(env, fmt.Sprintf(runWrapperEnvKVFmt, zqkenv.APIKey(), "account:system"))
		cmd.SetEnv(env)

		// Stream stdout/stderr to files and a small ring buffer (avoids holding full output in memory)
		// Write run/retry separator so recurring jobs show multiple runs in one file
		runTs := zqktime.NowRFC3339UTC()
		if stdoutStreamFile != nil {
			if attempt == 0 {
				if _, err := fmt.Fprintf(stdoutStreamFile, "\n--- run %s ---\n", runTs); err != nil {
					RunWrapperLog(h.logger).Debug("Failed to write run header to stdout stream").WithError(err).Log()
				}
			} else {
				if _, err := fmt.Fprintf(stdoutStreamFile, "\n--- retry %d %s ---\n", attempt+1, runTs); err != nil {
					RunWrapperLog(h.logger).Debug("Failed to write retry header to stdout stream").WithError(err).Log()
				}
			}
		}
		if stderrStreamFile != nil {
			if attempt == 0 {
				if _, err := fmt.Fprintf(stderrStreamFile, "\n--- run %s ---\n", runTs); err != nil {
					RunWrapperLog(h.logger).Debug("Failed to write run header to stderr stream").WithError(err).Log()
				}
			} else {
				if _, err := fmt.Fprintf(stderrStreamFile, "\n--- retry %d %s ---\n", attempt+1, runTs); err != nil {
					RunWrapperLog(h.logger).Debug("Failed to write retry header to stderr stream").WithError(err).Log()
				}
			}
		}
		stdoutWriter := newStreamingOutputWriter(stdoutStreamFile, DefaultPreviewBytes)
		stderrWriter := newStreamingOutputWriter(stderrStreamFile, DefaultPreviewBytes)
		cmd.SetStdout(stdoutWriter)
		cmd.SetStderr(stderrWriter)

		// Execute command with status callback support
		startTime := time.Now()

		// Write execution start event to events file
		writeEventEntry(map[string]any{
			KeyTimestamp:   zqktime.FormatRFC3339UTC(startTime),
			KeyJobID:       job.ID,
			KeyEventType:   runWrapperEventStarted,
			KeyCommand:     cmdStr,
			KeyAttempt:     attempt + 1,
			KeyMaxAttempts: retryCount + 1,
		})
		if isTestBundle && attempt == 0 {
			h.emitBundleProgress(ctx, job, "started")
		}

		// If status callback is configured, call it at start (only on first attempt)
		if job.CallbackOnStatus != emptyValue && attempt == 0 {
			h.executeCallback(ctx, job, "status", map[string]any{
				KeyJobID:               job.ID,
				KeyCommand:             cmdStr,
				objects.FieldKeyStatus: runWrapperStatusStarted,
				KeyTimestamp:           zqktime.FormatRFC3339UTC(startTime),
			})

			// Non-blocking route for status
			statusMsg := CreateStatusMessage(job, runWrapperStatusStarted, nil)
			h.routeAsync(ctx, statusMsg)
		}

		// For long-running commands, log periodic progress updates
		// This helps diagnose what the command was doing when it times out
		progressTicker := time.NewTicker(30 * time.Second) // Log progress every 30 seconds
		progressDone := make(chan bool)
		goroutinelabels.NewGoroutine("job_progress_monitor", fmt.Sprintf("monitoring progress for job %s", job.ID)).
			WithCleanup(func() {
				progressTicker.Stop()
			}).
			StartWithContext(ctx, func(ctx context.Context) error {
				for {
					select {
					case <-ctx.Done():
						// Context cancelled - exit gracefully
						return ctx.Err()
					case <-progressDone:
						return nil
					case <-progressTicker.C:
						elapsed := time.Since(startTime)
						stdoutLen := stdoutWriter.Len()
						stderrLen := stderrWriter.Len()
						RunWrapperLog(h.logger).Debug(LogEventRunWrapperCommandStillRunning).
							JobID(job.ID).
							Command(cmdStr).
							String("elapsed", elapsed.String()).
							Int("stdout_preview_bytes", stdoutLen).
							Int("stderr_preview_bytes", stderrLen).
							String("note", "Command may be waiting for I/O, locks, or other blocking operations").
							Log()
					}
				}
			})

		// Start command and set up process group cleanup
		// We need to start the command first to get the PID, then set up cleanup
		if err := cmd.Start(); err != nil {
			progressTicker.Stop()
			progressDone <- true
			lastErr = err
			if cancel != nil {
				cancel()
			}
			continue // Try next attempt
		}

		// CRITICAL: Register subprocess with process group manager
		// This ensures the subprocess can be tracked and controlled during shutdown
		processPID := cmd.GetPid()
		processGroupID := -processPID // Negative PID for process group
		subprocessID := fmt.Sprintf("job-%s-attempt-%d", job.ID, attempt)

		// Track for single defer outside loop (cleanup on panic/return)
		currentProcess.cmd = cmd
		currentProcess.processGroupID = processGroupID
		currentProcess.subprocessID = subprocessID
		currentProcess.reaped = false
		currentProcess.progressTicker = progressTicker
		currentProcess.progressDone = progressDone

		killFunc := func() error {
			if processGroupID != 0 {
				// Kill entire process group using negative PID
				return syscallutil.KillProcessGroup(processGroupID)
			}
			return nil
		}

		// Register subprocess (non-critical by default, can be made critical per job config)
		critical := false // TODO: Add job config field for critical processes
		h.processGroupManager.RegisterSubprocess(
			subprocessID,
			job.ID,
			fmt.Sprintf("Command: %s (attempt %d/%d)", cmdStr, attempt+1, retryCount+1),
			processPID,
			processGroupID,
			critical,
			killFunc,
		)

		// Set up goroutine to kill process group when context is cancelled
		// This ensures child processes are killed, not just the direct child
		// The process group manager will also handle this during shutdown
		goroutinelabels.NewGoroutine("job_process_killer", fmt.Sprintf("killing process group for job %s (attempt %d)", job.ID, attempt+1)).
			WithPostCleanup(func() {
				// Unregister subprocess (cleanup)
				h.processGroupManager.UnregisterSubprocess(subprocessID)
			}).
			StartWithContext(cmdCtx, func(ctx context.Context) error {
				<-ctx.Done()
				// Context cancelled (timeout or explicit cancellation)
				// Kill the entire process group using negative PID
				// This kills the process and all its children
				if processGroupID != 0 {
					// Negative PID targets the process group
					if err := syscallutil.KillProcessGroup(processGroupID); err != nil {
						RunWrapperLog(h.logger).Debug("Failed to kill process group on cancellation").WithError(err).Log()
					}
				}
				return ctx.Err()
			})

		// Wait for command to complete
		err := cmd.Wait()
		currentProcess.reaped = true
		progressTicker.Stop()
		select {
		case progressDone <- true:
		default:
		}
		duration := time.Since(startTime)

		// Unregister subprocess after completion (cleanup)
		h.processGroupManager.UnregisterSubprocess(subprocessID)

		// Check if command succeeded
		if err == nil {
			return h.runWrapperRecordSuccessfulAttempt(ctx, job, a, cmdStr, duration, attempt, stdoutWriter, stderrWriter, writeEventEntry, streamedToFiles, isTestBundle, lastErr, testFailures, testSummary, testRootsToClean, cancel)
		}

		// Clean up timeout context if created
		if cancel != nil {
			cancel()
		}

		// Command failed
		lastErr = err
		exitCode := 1
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		}

		stderrPreviewForReason := stderrWriter.Preview()
		var failureKind, failureReason string
		terminalEntryWritten, failureKind, failureReason, lastErr = h.runWrapperLogFailedAttempt(
			ctx, cmdCtx, job, cmdStr, attempt, duration, stdoutWriter, stderrWriter,
			writeEventEntry, streamedToFiles, isTestBundle, workingDir,
			maxRuntimeSeconds, exitCode, stderrPreviewForReason, err, lastErr, terminalEntryWritten,
		)

		if attempt >= retryCount {
			errorStderr := stderrWriter.Preview()

			tf, ts := h.runWrapperCollectTestFailuresFromOutput(job, streamedToFiles, stdoutWriter, stderrWriter, errorStderr)
			if tf != nil {
				testFailures = tf
			}
			if ts != nil {
				testSummary = ts
			}
			isTestCommand := h.isTestCommand(job.Command, job.CommandArgs)

			// Write failure event to events file
			failureEntry := map[string]any{
				KeyTimestamp:     zqktime.NowRFC3339UTC(),
				KeyJobID:         job.ID,
				KeyEventType:     runWrapperEventFailed,
				KeyFailureKind:   failureKind,
				KeyFailureReason: failureReason,
				KeyCommand:       cmdStr,
				KeyDuration:      duration.Seconds(),
				KeyAttempt:       attempt + 1,
				KeyExitCode:      exitCode,
				KeySuccess:       false,
				KeyError:         lastErr.Error(),
			}
			if errorStderr != emptyValue {
				failureEntry[KeyStderr] = errorStderr
			}
			if stdoutWriter.Len() > 0 && (job.LogLevel == runWrapperLogLevelVerbose || job.LogLevel == runWrapperLogLevelDebug) {
				failureEntry[KeyStdout] = stdoutWriter.Preview()
			}
			if len(testFailures) > 0 {
				failureEntry[KeyTestFailures] = testFailures
				failureEntry[KeyTestSummary] = testSummary
			}
			failFP := FingerprintBundleCommand(cmdStr)
			failureEntry[KeyBundleCommandFingerprint] = failFP
			var suggested []string
			if len(testFailures) > 0 {
				failureEntry[KeyTestOutcome] = runWrapperTestOutcomeTF
				suggested = BuildSuggestedGoTestRerunCommands(cmdStr, job.Command, job.CommandArgs, testFailures, cmdTimeoutSeconds)
				if len(suggested) > 0 {
					failureEntry[KeySuggestedRerunCommands] = suggested
				}
			} else {
				failureEntry[KeyTestOutcome] = runWrapperTestOutcomeFail
			}
			if isTestBundle {
				if len(testFailures) > 0 {
					h.maybePersistCVSPerTestFailureLedger(ctx, job, failFP, testFailures, false)
				}
				healthFailCount := len(testFailures)
				if testSummary != nil {
					if v, ok := testSummary[runWrapperFieldFailedTests]; ok {
						switch n := v.(type) {
						case float64:
							if int(n) > healthFailCount {
								healthFailCount = int(n)
							}
						case int:
							if n > healthFailCount {
								healthFailCount = n
							}
						case int64:
							if int(n) > healthFailCount {
								healthFailCount = int(n)
							}
						}
					}
				}
				ho := runWrapperTestOutcomeFail
				if healthFailCount > 0 {
					ho = runWrapperTestOutcomeTF
				}
				AppendTestBundleHealthEvent(h.projectRoot, job.ID, BuildTestBundleHealthEntry(job.ID, JobLogEventFailed, ho, healthFailCount, failFP, suggested))
				h.emitBundleProgress(ctx, job, ho)
				MaybeAppendTestBundleCriteriaVerificationEvidence(h.projectRoot, job, ho, healthFailCount, failFP)
				h.maybeRunConvergenceTickAfterTestBundleHealth(ctx, job)
			}
			writeEventEntry(failureEntry)
			terminalEntryWritten = true
			if !streamedToFiles {
				writeSeparateJobLogsIfConfigured(h.projectRoot, job.ID, stdoutWriter.Preview(), stderrWriter.Preview())
			}

			h.runWrapperNotifyExhaustedFailure(ctx, job, cmdStr, attempt, retryCount, duration, errorStderr, lastErr, failureKind, failureReason, exitCode, cmdTimeoutSeconds, isTestCommand, testFailures, testSummary)
			break
		}
	}
	*a.lastErr = lastErr
	*a.testFailures = testFailures
	*a.testSummary = testSummary
	*a.terminalEntryWritten = terminalEntryWritten
	*a.testRootsToClean = testRootsToClean
	return false
}

func (h *RunWrapperHandler) emitBundleProgress(ctx context.Context, job *ScheduledJob, eventType string) {
	if !IsTestBundleJob(job.ID) {
		return
	}

	// 1. Resolve all test bundle jobs
	var bundleJobs []*ScheduledJob
	if h.scheduler != nil {
		for _, j := range h.scheduler.ListJobs() {
			if strings.HasPrefix(j.ID, "SCH-run-bundle-") {
				bundleJobs = append(bundleJobs, j)
			}
		}
	}

	totalCount := len(bundleJobs)
	if totalCount == 0 {
		totalCount = 8 // Default fallback
	}

	// 2. Read latest state of each bundle
	runningIDs := map[string]bool{}
	if h.scheduler != nil {
		for _, id := range h.scheduler.GetRunningJobIDs() {
			runningIDs[id] = true
		}
	}

	completedCount := 0
	passCount := 0
	failCount := 0

	lines, err := ReadTestBundleHealthTailLines(ctx, h.projectRoot, 200)
	latestOutcome := map[string]string{}
	if err == nil {
		for _, m := range lines {
			jID, _ := m[KeyJobID].(string)
			if jID == "" {
				continue
			}
			o, _ := m[KeyTestOutcome].(string)
			latestOutcome[jID] = o
		}
	}

	// Override/inject the current transition
	if eventType == "started" {
		runningIDs[job.ID] = true
		delete(latestOutcome, job.ID)
	} else if eventType == "pass" || eventType == "passed" || eventType == "ok" {
		delete(runningIDs, job.ID)
		latestOutcome[job.ID] = "pass"
	} else if eventType == "skipped" {
		delete(runningIDs, job.ID)
		latestOutcome[job.ID] = "skipped"
	} else {
		delete(runningIDs, job.ID)
		latestOutcome[job.ID] = "fail"
	}

	// Iterate over the known bundle jobs to count stats
	if len(bundleJobs) > 0 {
		for _, j := range bundleJobs {
			outcome := latestOutcome[j.ID]
			if outcome != "" {
				completedCount++
				if outcome == "pass" || outcome == "ok" {
					passCount++
				} else if outcome == "skipped" {
					// skipped is completed but not pass or fail
				} else {
					failCount++
				}
			}
		}
	} else {
		// If we don't have list of jobs in memory, build it from latest outcomes + running
		for jID, outcome := range latestOutcome {
			if strings.HasPrefix(jID, "SCH-run-bundle-") {
				completedCount++
				if outcome == "pass" || outcome == "ok" {
					passCount++
				} else if outcome == "skipped" {
					// skipped is completed but not pass or fail
				} else {
					failCount++
				}
			}
		}
		totalCount = completedCount
		for rID := range runningIDs {
			if strings.HasPrefix(rID, "SCH-run-bundle-") {
				totalCount++
			}
		}
	}

	fp := ""
	if job.Metadata != nil {
		if v, ok := job.Metadata[KeyBundleCommandFingerprint].(string); ok {
			fp = v
		}
	}
	if fp == "" {
		cmdStr := job.Command
		if len(job.CommandArgs) > 0 {
			cmdStr += " " + strings.Join(job.CommandArgs, " ")
		}
		fp = FingerprintBundleCommand(cmdStr)
	}

	progressEntry := map[string]any{
		"timestamp":                  zqktime.NowRFC3339UTC(),
		"job_id":                     job.ID,
		objects.FieldKeyEventType:    eventType,
		"completed_count":            completedCount,
		"total_count":                totalCount,
		"pass_count":                 passCount,
		"fail_count":                 failCount,
		"bundle_command_fingerprint": fp,
	}

	AppendTestBundleProgressEvent(h.projectRoot, job.ID, progressEntry)
}
