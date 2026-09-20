package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/config"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// runWrapperExecPrep holds resolved command, retry tuning, and timeout for run_wrapper execution.
type runWrapperExecPrep struct {
	Command           string
	Args              []string
	IsShellScript     bool
	CmdStr            string
	WorkingDir        string
	RetryCount        int
	RetryDelay        time.Duration
	MaxRuntimeSeconds int
}

func (h *RunWrapperHandler) prepareRunWrapperExecution(job *ScheduledJob) runWrapperExecPrep {
	command := job.Command
	args := job.CommandArgs
	if args == nil {
		args = []string{}
	}
	exeName := brand.ExecutableName()
	if exeName == emptyValue {
		exeName = "zqk"
	}
	if command == exeName || command == "zqk" {
		command = resolveSchedulerCLIBinary(h.projectRoot)
	}

	isShellScript := strings.Contains(command, "\n") ||
		strings.HasPrefix(strings.TrimSpace(command), "#") ||
		strings.Contains(command, "|") ||
		strings.Contains(command, "&&") ||
		strings.Contains(command, "||") ||
		strings.Contains(command, "$(") ||
		strings.Contains(command, "${") ||
		strings.Contains(command, "while ") ||
		strings.Contains(command, "for ") ||
		strings.Contains(command, "if ")

	if !isShellScript && len(args) == 0 && strings.Contains(command, " ") {
		parts := strings.Fields(command)
		if len(parts) > 1 {
			command = parts[0]
			args = parts[1:]
		}
	}

	if command == "zqk" {
		projectRoot := h.projectRoot
		if projectRoot == emptyValue {
			if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
				projectRoot = fileStorage.GetProjectRoot()
			}
		}
		if projectRoot == emptyValue && job.EnvironmentVariables != nil {
			if root, ok := job.EnvironmentVariables[zqkenv.ProjectRoot().Name()]; ok && root != emptyValue {
				projectRoot = root
			}
		}
		if projectRoot == emptyValue {
			if wd, err := fileutil.Getwd(); err == nil {
				projectRoot = paths.ResolveProjectRoot(wd)
			}
		}
		command = resolveSchedulerCLIBinary(projectRoot)
	}

	workingDir := job.WorkingDirectory
	retryCount := job.RetryCount
	if retryCount < 0 {
		retryCount = 0
	}
	retryDelay := time.Duration(job.RetryDelaySeconds) * time.Second
	if retryDelay < 0 {
		retryDelay = 5 * time.Second
	}

	var cmdStr string
	when.When(func() bool { return isShellScript }).Then(func() {
		cmdStr = "/bin/sh -c <shell script>"
	}).OrElseWhen(func() bool { return len(args) > 0 }).Then(func() {
		cmdStr = command + " " + strings.Join(args, " ")
	}).OrElse(func() {
		cmdStr = command
	}).Run()

	maxRuntimeSeconds := job.MaxRuntimeSeconds
	if h.isTestCommand(command, args) {
		testName, packageName := h.extractTestInfo(args)
		isBundleCommand := testName != emptyValue && (strings.Contains(testName, "|") || strings.HasPrefix(testName, "^("))
		if (testName != emptyValue || packageName != emptyValue) && !isBundleCommand {
			expectedTimeout := testkit.GetExpectedTimeout(testName, packageName)
			maxRuntimeSeconds = int(expectedTimeout.Seconds())
			RunWrapperLog(h.logger).Info(LogEventRunWrapperDynamicTimeout).
				JobID(job.ID).
				TestName(testName).
				PackageName(packageName).
				ExpectedTimeout(expectedTimeout.String()).
				Int("max_runtime_seconds", maxRuntimeSeconds).
				Log()
		} else if maxRuntimeSeconds == 0 || maxRuntimeSeconds < 3600 {
			maxRuntimeSeconds = 3600
			RunWrapperLog(h.logger).Info(LogEventRunWrapperDefaultSuiteTimeout).
				JobID(job.ID).
				Int("max_runtime_seconds", maxRuntimeSeconds).
				Log()
		}
	}

	return runWrapperExecPrep{
		Command:           command,
		Args:              args,
		IsShellScript:     isShellScript,
		CmdStr:            cmdStr,
		WorkingDir:        workingDir,
		RetryCount:        retryCount,
		RetryDelay:        retryDelay,
		MaxRuntimeSeconds: maxRuntimeSeconds,
	}
}

// warnIfTestBundleMetadataFingerprintMismatch logs when persisted metadata does not match the command
// about to run (guards against stale in-memory jobs, manual edits, or loader drift).
func (h *RunWrapperHandler) warnIfTestBundleMetadataFingerprintMismatch(job *ScheduledJob, cmdStr string) {
	if job == nil || !IsTestBundleJob(job.ID) || job.Metadata == nil {
		return
	}
	v, ok := job.Metadata[KeyBundleCommandFingerprint]
	if !ok {
		return
	}
	metaFP, ok := v.(string)
	if !ok || metaFP == emptyValue {
		return
	}
	actual := FingerprintBundleCommand(cmdStr)
	if actual == metaFP {
		return
	}
	RunWrapperLog(h.logger).Warn(LogEventRunWrapperBundleCommandFingerprintMismatch).
		JobID(job.ID).
		String("metadata_bundle_command_fingerprint", metaFP).
		String("command_bundle_command_fingerprint", actual).
		Log()
}

func (h *RunWrapperHandler) logRunWrapperExecutionStart(job *ScheduledJob, prep runWrapperExecPrep) {
	when.When(func() bool { return job.LogLevel == "debug" }).Then(func() {
		envVarsStr := ""
		if len(job.EnvironmentVariables) > 0 {
			var parts []string
			for k, v := range job.EnvironmentVariables {
				parts = append(parts, fmt.Sprintf("%s=%s", k, v))
			}
			envVarsStr = strings.Join(parts, ", ")
		}
		RunWrapperLog(h.logger).Info(LogEventRunWrapperExecutingCommand).
			JobID(job.ID).
			Command(prep.CmdStr).
			WorkingDirectory(prep.WorkingDir).
			Int("retry_count", prep.RetryCount).
			Int("max_runtime_seconds", prep.MaxRuntimeSeconds).
			LogLevel(job.LogLevel).
			String("environment_variables", envVarsStr).
			Log()
	}).OrElse(func() {
		RunWrapperLog(h.logger).Info(LogEventRunWrapperExecutingCommand).
			JobID(job.ID).
			Command(prep.CmdStr).
			WorkingDirectory(prep.WorkingDir).
			Int("retry_count", prep.RetryCount).
			Int("max_runtime_seconds", prep.MaxRuntimeSeconds).
			Log()
	}).Run()
}

// Execute runs the external command with timeout and retry logic
//
//nolint:gocyclo
func (h *RunWrapperHandler) executeRunWrapperCore(ctx context.Context, job *ScheduledJob) (err error) {
	// Defensive: ensure panics become actionable errors with stack traces.
	// This prevents a single job from taking down the scheduler loop and makes root cause obvious.
	defer func() {
		if e := h.runWrapperHandlePanic(job, recover(), debug.Stack()); e != nil {
			err = e
		}
	}()

	if job == nil {
		return errfmt.Errorf("job is nil")
	}
	if job.Command == emptyValue {
		return errfmt.Errorf("command is required for run_wrapper job type")
	}

	prep := h.prepareRunWrapperExecution(job)
	command := prep.Command
	args := prep.Args
	isShellScript := prep.IsShellScript
	cmdStr := prep.CmdStr
	workingDir := prep.WorkingDir
	retryCount := prep.RetryCount
	retryDelay := prep.RetryDelay
	maxRuntimeSeconds := prep.MaxRuntimeSeconds

	h.warnIfTestBundleMetadataFingerprintMismatch(job, cmdStr)

	h.logRunWrapperExecutionStart(job, prep)

	// Write job execution events: test-bundle jobs (SCH-run-*) use single shared test-bundles/events.jsonl;
	// churn-style one-off jobs use shared churn-runs/ with per-job stems; others use per-job dirs.
	// Stdout/stderr: JobStdoutFilePath / JobStderrFilePath (stem may be hashed when job ID is very long).
	var eventsWriter *jobLogWriter
	var stdoutStreamFile, stderrStreamFile *fileutil.File
	streamedToFiles := false
	terminalEntryWritten := false
	isTestBundle := IsTestBundleJob(job.ID)
	if h.projectRoot != emptyValue && job.ID != emptyValue {
		logDir := JobLogDir(h.projectRoot, job.ID)
		if err := fileutil.MkdirAll(logDir, paths.DirPerm755); err == nil {
			if !isTestBundle {
				if w, err := GetOrCreateJobLogWriter(h.projectRoot, job.ID); err == nil && w != nil {
					eventsWriter = w
					defer func() {
						if !terminalEntryWritten && eventsWriter != nil {
							entry := map[string]any{
								KeyTimestamp: zqktime.NowRFC3339UTC(),
								KeyJobID:     job.ID,
								KeyEventType: "exited_without_final_status",
								KeyCommand:   cmdStr,
								KeySuccess:   false,
								KeyError:     "handler returned without writing completed/failed/timeout; scheduler may have been interrupted or context cancelled",
							}
							if jsonData, marshalErr := json.Marshal(entry); marshalErr == nil {
								if errWrite := eventsWriter.WriteLine(jsonData); errWrite != nil {
									RunWrapperLog(h.logger).Debug("Failed to write final status to job log").WithError(errWrite).Log()
								}
							}
						}
					}()
				}
			} else {
				defer func() {
					if !terminalEntryWritten {
						AppendTestBundleEvent(h.projectRoot, job.ID, map[string]any{
							KeyTimestamp: zqktime.NowRFC3339UTC(),
							KeyJobID:     job.ID,
							KeyEventType: "exited_without_final_status",
							KeyCommand:   cmdStr,
							KeySuccess:   false,
							KeyError:     "handler returned without writing completed/failed/timeout; scheduler may have been interrupted or context cancelled",
						})
					}
				}()
			}
			// Open stdout/stderr files for streaming (append so recurring jobs accumulate output across runs)
			if so, err := fileutil.OpenFile(JobStdoutFilePath(h.projectRoot, job.ID), fileutil.O_WRONLY|fileutil.O_CREATE|fileutil.O_APPEND, paths.FilePerm600); err == nil {
				stdoutStreamFile = so
				defer stdoutStreamFile.Close()
			}
			if se, err := fileutil.OpenFile(JobStderrFilePath(h.projectRoot, job.ID), fileutil.O_WRONLY|fileutil.O_CREATE|fileutil.O_APPEND, paths.FilePerm600); err == nil {
				stderrStreamFile = se
				defer stderrStreamFile.Close()
			}
			if stdoutStreamFile != nil && stderrStreamFile != nil {
				streamedToFiles = true
			}
		}
	}
	writeEventEntry := func(entry map[string]any) {
		if isTestBundle {
			AppendTestBundleEvent(h.projectRoot, job.ID, entry)
			et, _ := entry[KeyEventType].(string)
			TrackAndAppendTestBundleProgress(h.projectRoot, job.ID, et)
			return
		}
		if eventsWriter == nil {
			return
		}
		if data, err := json.Marshal(entry); err == nil {
			if errWrite := eventsWriter.WriteLine(data); errWrite != nil {
				RunWrapperLog(h.logger).Debug("Failed to write event entry to job log").WithError(errWrite).Log()
			}
		}
	}

	var lastErr error
	// Track test failures across retries (declared outside loop for access at return)
	var testFailures []string
	var testSummary map[string]any

	// Collect test temp dirs and clean once on return (avoids defer in loop / resource leak)
	var testRootsToClean []string
	defer func() {
		for _, d := range testRootsToClean {
			if err := fileutil.RemoveAll(d); err != nil && !fileutil.IsNotExist(err) {
				RunWrapperLog(h.logger).Debug("Failed to cleanup test root").WithError(err).Path(d).Log()
			}
		}
	}()

	// Single defer outside loop to avoid deferInLoop: clean up any started-but-not-reaped process on panic/return
	var currentProcess runWrapperProcState
	defer func() {
		if currentProcess.progressTicker != nil {
			currentProcess.progressTicker.Stop()
		}
		if currentProcess.progressDone != nil {
			select {
			case currentProcess.progressDone <- true:
			default:
			}
		}
		if currentProcess.reaped || currentProcess.cmd == nil || currentProcess.cmd.GetPid() == 0 {
			return
		}
		if currentProcess.processGroupID != 0 {
			if err := syscall.Kill(currentProcess.processGroupID, syscall.SIGKILL); err != nil {
				RunWrapperLog(h.logger).Debug("Failed to kill process group on exit").WithError(err).Log()
			}
		}
		if err := currentProcess.cmd.Wait(); err != nil {
			RunWrapperLog(h.logger).Debug("Failed to reap subprocess on exit").WithError(err).Log()
		}
		h.processGroupManager.UnregisterSubprocess(currentProcess.subprocessID)
	}()

	// When streaming to job stdout/stderr files, strip shell redirect from the script so the
	// command output goes to our writers (and we copy to the redirect path afterward).
	var redirectTargetPath string
	scriptForExec := ""
	if streamedToFiles && (isShellScript || isShellWithInlineScript(command, args)) {
		var script string
		if isShellScript {
			script = command
		} else {
			// Inline script: either args=["-c", script] or args=[shellPath, "-c", script]
			if len(args) >= 2 && args[0] == "-c" {
				script = args[1]
				if len(args) > 2 {
					script = strings.Join(args[1:], " ")
				}
			} else {
				script = args[2]
				if len(args) > 3 {
					script = strings.Join(args[2:], " ")
				}
			}
		}
		stripped, target := stripShellOutputRedirect(script)
		if target != emptyValue {
			redirectTargetPath = target
			if !filepath.IsAbs(redirectTargetPath) && workingDir != emptyValue {
				redirectTargetPath = filepath.Join(workingDir, redirectTargetPath)
			} else if !filepath.IsAbs(redirectTargetPath) && h.projectRoot != emptyValue {
				redirectTargetPath = filepath.Join(h.projectRoot, redirectTargetPath)
			}
			scriptForExec = stripped
		}
	}

	// Fail fast: validate shell script syntax before running so we don't tie up resources.
	// See Lesson 7 in docs/architecture/LESSONS_LEARNED.md.
	if !isShellScript && !isShellWithInlineScript(command, args) && isShellScriptPath(command) {
		baseDir := workingDir
		if baseDir == emptyValue {
			baseDir = h.projectRoot
		}
		if absPath, resolveErr := resolveScriptPath(command, baseDir, h.projectRoot); resolveErr == nil {
			syntaxStderr, syntaxErr := validateShellScriptSyntax(ctx, h.getExecutor(), absPath, baseDir)
			if syntaxErr != nil {
				failureReason := firstLine(syntaxStderr, 300)
				if failureReason == emptyValue {
					failureReason = syntaxErr.Error()
				}
				RunWrapperLog(h.logger).Warn(LogEventRunWrapperShellSyntaxFailed).
					JobID(job.ID).
					Command(cmdStr).
					ScriptPath(absPath).
					StderrSnippet(syntaxStderr).
					Log()
				if eventsWriter != nil {
					failureEntry := map[string]any{
						KeyTimestamp:     zqktime.NowRFC3339UTC(),
						KeyJobID:         job.ID,
						KeyEventType:     "failed",
						KeyFailureKind:   "script_syntax",
						KeyFailureReason: "shell script syntax error (sh -n). " + failureReason,
						KeyCommand:       cmdStr,
						KeyDuration:      0,
						KeyAttempt:       1,
						KeySuccess:       false,
						KeyError:         syntaxErr.Error(),
					}
					if syntaxStderr != emptyValue {
						failureEntry[KeyStderr] = syntaxStderr
					}
					writeEventEntry(failureEntry)
					terminalEntryWritten = true
				}
				return errfmt.Newf("script syntax check failed (sh -n)").Wrap(syntaxErr)
			}
		}
	}

	if h.runWrapperRetryAttempts(
		ctx,
		job,
		&runWrapperRetryArgs{
			cmdStr:               cmdStr,
			command:              command,
			args:                 args,
			isShellScript:        isShellScript,
			workingDir:           workingDir,
			retryCount:           retryCount,
			retryDelay:           retryDelay,
			maxRuntimeSeconds:    maxRuntimeSeconds,
			streamedToFiles:      streamedToFiles,
			writeEventEntry:      writeEventEntry,
			isTestBundle:         isTestBundle,
			stdoutStreamFile:     stdoutStreamFile,
			stderrStreamFile:     stderrStreamFile,
			testRootsToClean:     &testRootsToClean,
			currentProcess:       &currentProcess,
			scriptForExec:        scriptForExec,
			terminalEntryWritten: &terminalEntryWritten,
			lastErr:              &lastErr,
			testFailures:         &testFailures,
			testSummary:          &testSummary,
		},
	) {
		return nil
	}

	// When we stripped shell redirect so output went to our stream files, copy them to the
	// expected redirect path (e.g. .zqk/logs/tests/bundle-xxx.log) so callbacks and tools see the file.
	if redirectTargetPath != emptyValue && streamedToFiles && h.projectRoot != emptyValue && job.ID != emptyValue {
		logDir := JobLogDir(h.projectRoot, job.ID)
		copyStreamFilesToRedirectTarget(h.projectRoot, logDir, job.ID, redirectTargetPath)
	}

	// All retries exhausted
	// For test failures, create a clearer error message that distinguishes test failures from job execution failures
	if len(testFailures) > 0 {
		return errfmt.Errorf("test bundle ran successfully but %d test(s) failed", len(testFailures))
	}
	return errfmt.Errorf("command failed after %d attempts: %w", retryCount+1, lastErr)
}

// runWrapperHandlePanic converts a recovered panic into a returned error and performs best-effort logging/notifications.
func (h *RunWrapperHandler) runWrapperHandlePanic(job *ScheduledJob, r interface{}, stack []byte) error {
	if r == nil {
		return nil
	}
	jobID := ""
	jobType := ""
	category := ""
	command := ""
	if job != nil {
		jobID = job.ID
		jobType = job.JobType
		category = job.Category
		command = job.Command
	}
	panicErr := errfmt.Errorf("job panicked: %v", r)
	RunWrapperLog(h.logger).Error(LogEventRunWrapperPanicked, errfmt.Errorf("panic: %v", r)).
		WithFields(jobLogFieldsForPanic(jobID, jobType, category, command, stack)...).
		Log()

	// Best-effort: append a panic event to the per-job events file (if configured).
	if h.projectRoot != emptyValue && jobID != emptyValue {
		logDir := JobLogDir(h.projectRoot, jobID)
		eventsFilePath := JobEventsFilePath(h.projectRoot, jobID)
		if mkErr := fileutil.MkdirAll(logDir, paths.DirPerm755); mkErr == nil {
			if f, openErr := fileutil.OpenFile(eventsFilePath, fileutil.O_WRONLY|fileutil.O_CREATE|fileutil.O_APPEND, paths.FilePerm600); openErr == nil {
				entry := map[string]any{
					KeyTimestamp: zqktime.NowRFC3339UTC(),
					KeyJobID:     jobID,
					KeyEventType: "panic",
					KeyCommand:   command,
					KeyError:     fmt.Sprintf("panic: %v", r),
				}
				stackStr := string(stack)
				if len(stackStr) > 8192 {
					stackStr = stackStr[:8192] + "... (truncated)"
				}
				entry["stack"] = stackStr
				if b, mErr := json.Marshal(entry); mErr == nil {
					if _, errWrite := f.Write(b); errWrite != nil {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("failed to write panic event: %v\n", errWrite).Log()
					}
					if _, errWriteStr := f.WriteString("\n"); errWriteStr != nil {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("failed to write panic event newline: %v\n", errWriteStr).Log()
					}
				}
				if errClose := f.Close(); errClose != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("failed to close panic event log: %v\n", errClose).Log()
				}
			}
		}
	}

	// Best-effort: notify the user (high priority).
	if h.notificationContext != nil && jobID != emptyValue {
		notif := CreateJobNotification(
			jobID,
			jobType,
			category,
			"failed",
			PriorityHigh,
			0,
			panicErr,
			map[string]any{
				KeyCommand: command,
			},
		)
		h.notificationContext.Notify(notif)
	}
	return panicErr
}

// stripShellOutputRedirect removes trailing "> path 2>&1" or ">> path 2>&1" from a shell script
// so that when we set cmd.Stdout/cmd.Stderr to our writers, the child output goes to them instead of the redirect.
// Returns (scriptWithoutRedirect, redirectTargetPath). If no redirect is found, returns (script, "").
// redirectTargetPath is trimmed; callers should make it absolute if needed.
func stripShellOutputRedirect(script string) (string, string) {
	s := script
	if !strings.HasSuffix(s, " 2>&1") {
		return script, ""
	}
	s = s[:len(s)-len(" 2>&1")]
	idx := strings.LastIndex(s, " > ")
	if idx < 0 {
		idx = strings.LastIndex(s, " >> ")
		if idx < 0 {
			return script, ""
		}
		// " >> path"
		return strings.TrimRight(s[:idx], " \t"), strings.TrimSpace(s[idx+4:])
	}
	// " > path"
	return strings.TrimRight(s[:idx], " \t"), strings.TrimSpace(s[idx+3:])
}

// copyStreamFilesToRedirectTarget writes the contents of job stdout/stderr stream files to targetPath
// (combined: stdout then stderr, matching shell "> file 2>&1"). Best-effort; errors are not surfaced.
func copyStreamFilesToRedirectTarget(projectRoot, logDir, jobID, targetPath string) {
	if logDir == emptyValue || jobID == emptyValue || targetPath == emptyValue {
		return
	}
	if IsTestBundleJob(jobID) && projectRoot != emptyValue {
		targetPath = NormalizeTestBundleRedirectLogPath(projectRoot, targetPath)
	}
	stem := JobLogFileStem(jobID)
	stdoutPath := filepath.Join(logDir, stem+".stdout")
	stderrPath := filepath.Join(logDir, stem+".stderr")

	if errMkdir := fileutil.MkdirAll(filepath.Dir(targetPath), paths.DirPerm755); errMkdir != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("failed to create redirect target dir: %v\n", errMkdir).Log()
	}

	outF, err := fileutil.OpenFile(targetPath, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_TRUNC, paths.FilePerm600)
	if err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("failed to open redirect target: %v\n", err).Log()
		return
	}
	defer outF.Close()

	hasStdout := false
	if inF, err := fileutil.Open(stdoutPath); err == nil {
		if stat, err := inF.Stat(); err == nil && stat.Size() > 0 {
			hasStdout = true
			_, _ = io.Copy(outF, inF)
		}
		_ = inF.Close()
	}

	if inF, err := fileutil.Open(stderrPath); err == nil {
		if stat, err := inF.Stat(); err == nil && stat.Size() > 0 {
			if hasStdout {
				_, _ = outF.Write([]byte("\n"))
			}
			_, _ = io.Copy(outF, inF)
		}
		_ = inF.Close()
	}
}

// writeSeparateJobLogsIfConfigured writes stdout and stderr to jobID.stdout and jobID.stderr in the job log dir
// when project config logging.error_log_output is "separate". Best-effort; errors are not surfaced.
func writeSeparateJobLogsIfConfigured(projectRoot, jobID, stdoutStr, stderrStr string) {
	if projectRoot == emptyValue || jobID == emptyValue || config.GetErrorLogOutput(projectRoot) != "separate" {
		return
	}
	logDir := JobLogDir(projectRoot, jobID)
	if err := fileutil.MkdirAll(logDir, paths.DirPerm755); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("failed to create job log directory: %v\n", err).Log()
	}
	if err := fileutil.WriteFile(JobStdoutFilePath(projectRoot, jobID), []byte(stdoutStr), paths.FilePerm600); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("failed to write separate job stdout log: %v\n", err).Log()
	}
	if err := fileutil.WriteFile(JobStderrFilePath(projectRoot, jobID), []byte(stderrStr), paths.FilePerm600); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("failed to write separate job stderr log: %v\n", err).Log()
	}
}
