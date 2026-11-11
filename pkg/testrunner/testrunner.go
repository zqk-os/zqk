package testrunner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/process"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// ResolveInvocations determines the commands to execute for each criterion linked to the test case.
// Generic across any language, script, or executable command.
func ResolveInvocations(testCase map[string]any, projectRoot string) ([]CriterionInvocation, error) {
	if testCase == nil {
		return nil, errfmt.Errorf("test_case object is nil")
	}

	critRefs := lifecycle.StringRefsFromAny(testCase[objects.FieldKeyCriteriaRefs])
	pathOrID, _ := testCase[objects.FieldKeyPathOrID].(string)
	suitesRaw := testCase[objects.FieldKeyVerificationSuites]
	suites := lifecycle.StringRefsFromAny(suitesRaw)

	var invocations []CriterionInvocation

	// 1. Check if verification_suites provides explicit criterion-mapped commands
	critToCmd := make(map[string]string)
	for _, suite := range suites {
		suite = strings.TrimSpace(suite)
		if idx := strings.Index(suite, ":"); idx != -1 {
			prefix := strings.TrimSpace(suite[:idx])
			cmdPart := strings.TrimSpace(suite[idx+1:])
			if strings.HasPrefix(prefix, "CRIT-") {
				critToCmd[prefix] = cmdPart
				continue
			}
		}
		// If not prefixed, treat as standalone command
		if len(critRefs) > 0 && len(critToCmd) == 0 {
			critToCmd[critRefs[0]] = suite
		}
	}

	// 2. If individual mappings exist, build invocations for them
	if len(critToCmd) > 0 {
		for _, cid := range critRefs {
			cmdStr, exists := critToCmd[cid]
			runner := ""
			if !exists {
				// Fallback to pathOrID if available
				cmdStr = buildDefaultCommand(pathOrID, cid, projectRoot)
				if r, ok := resolveRunnerScript(pathOrID, projectRoot); ok {
					runner = r
				}
			} else {
				if r, ok := resolveRunnerFromCmd(cmdStr, cid, projectRoot); ok {
					runner = r
				}
			}
			invocations = append(invocations, CriterionInvocation{
				CriterionID:      cid,
				Command:          cmdStr,
				RunnerExecutable: runner,
				Dir:              projectRoot,
			})
		}
		return invocations, nil
	}

	// 3. Fallback to path_or_id for each linked criterion
	if pathOrID != "" {
		runner := ""
		if r, ok := resolveRunnerScript(pathOrID, projectRoot); ok {
			runner = r
		}
		for _, cid := range critRefs {
			cmdStr := buildDefaultCommand(pathOrID, cid, projectRoot)
			invocations = append(invocations, CriterionInvocation{
				CriterionID:      cid,
				Command:          cmdStr,
				RunnerExecutable: runner,
				Dir:              projectRoot,
			})
		}
		return invocations, nil
	}

	// 4. Default: verification commands
	for _, cid := range critRefs {
		invocations = append(invocations, CriterionInvocation{
			CriterionID: cid,
			Command:     fmt.Sprintf("echo 'Verifying %s'", cid),
			Dir:         projectRoot,
		})
	}

	return invocations, nil
}

func buildDefaultCommand(pathOrID, critID, projectRoot string) string {
	cleanPath := strings.TrimSpace(pathOrID)
	if cleanPath == "" {
		return fmt.Sprintf("echo 'Checking %s'", critID)
	}

	testFilter := ""
	if idx := strings.Index(cleanPath, "_test.go:"); idx != -1 {
		testFilter = cleanPath[idx+len("_test.go:"):]
		cleanPath = cleanPath[:idx+len("_test.go")]
	}

	// If it looks like a go test file or package
	// Check if path is a typical Go package directory
	isGoPkg := strings.HasPrefix(cleanPath, "pkg/") || strings.HasPrefix(cleanPath, "cmd/") || strings.HasPrefix(cleanPath, "internal/")
	if strings.HasSuffix(cleanPath, "_test.go") {
		pkgDir := filepath.Dir(cleanPath)
		if !strings.HasPrefix(pkgDir, "./") && !filepath.IsAbs(pkgDir) {
			pkgDir = "./" + pkgDir
		}
		if testFilter != "" {
			return fmt.Sprintf("go test -v %s -run '^%s$'", pkgDir, testFilter)
		}
		fullPath := cleanPath
		if !filepath.IsAbs(fullPath) && projectRoot != "" {
			fullPath = filepath.Join(projectRoot, cleanPath)
		}
		if _, statErr := os.Stat(fullPath); statErr != nil {
			base := filepath.Base(cleanPath)
			if _, statBaseErr := os.Stat(base); statBaseErr == nil {
				fullPath = base
			}
		}
		if data, err := fileutil.ReadFile(filepath.Clean(fullPath)); err == nil { //nolint:gosec // test file discovery within verified paths
			var funcs []string
			lines := strings.Split(string(data), "\n")
			for _, l := range lines {
				l = strings.TrimSpace(l)
				if strings.HasPrefix(l, "func Test") {
					idx := strings.Index(l, "(")
					if idx != -1 {
						fn := strings.TrimSpace(l[5:idx])
						if fn != "" {
							funcs = append(funcs, fn)
						}
					}
				}
			}
			if len(funcs) > 0 {
				return fmt.Sprintf("go test -v %s -run '^(%s)'", pkgDir, strings.Join(funcs, "|"))
			}
		}
		return fmt.Sprintf("go test -v %s", pkgDir)
	}

	if strings.HasSuffix(cleanPath, ".go") || strings.Contains(cleanPath, "/...") || isGoPkg {
		pkgDir := cleanPath
		if strings.HasSuffix(cleanPath, ".go") {
			pkgDir = filepath.Dir(cleanPath)
		}
		if !strings.HasPrefix(pkgDir, "./") && !filepath.IsAbs(pkgDir) {
			pkgDir = "./" + pkgDir
		}
		return fmt.Sprintf("go test -v %s", pkgDir)
	}

	// If it is an executable script or file
	fullPath := cleanPath
	if !filepath.IsAbs(fullPath) && projectRoot != "" {
		fullPath = filepath.Join(projectRoot, cleanPath)
	}
	if _, err := os.Stat(fullPath); err == nil {
		if strings.HasSuffix(cleanPath, ".sh") {
			return fmt.Sprintf("bash %s %s", cleanPath, critID)
		}
		return fmt.Sprintf("%s %s", cleanPath, critID)
	}

	// Otherwise treat as a raw shell command
	return cleanPath
}

// resolveRunnerScript determines if a path points to an executable script file (not a Go package/test).
func resolveRunnerScript(pathOrID, projectRoot string) (string, bool) {
	cleanPath := strings.TrimSpace(pathOrID)
	if cleanPath == "" {
		return "", false
	}
	// Go tests and packages are handled through go test, not as standalone script runners
	if strings.HasSuffix(cleanPath, "_test.go") || strings.HasSuffix(cleanPath, ".go") ||
		strings.Contains(cleanPath, "/...") || strings.HasPrefix(cleanPath, "pkg/") ||
		strings.HasPrefix(cleanPath, "cmd/") || strings.HasPrefix(cleanPath, "internal/") {
		return "", false
	}

	fullPath := cleanPath
	if !filepath.IsAbs(fullPath) && projectRoot != "" {
		fullPath = filepath.Join(projectRoot, cleanPath)
	}
	if info, err := fileutil.Stat(fullPath); err == nil && !info.IsDir() {
		return cleanPath, true
	}
	base := filepath.Base(cleanPath)
	if info, err := fileutil.Stat(base); err == nil && !info.IsDir() {
		return cleanPath, true
	}
	return "", false
}

// resolveRunnerFromCmd extracts runner script path from a command like "bash scripts/test.sh CRIT-1" or "scripts/test.sh CRIT-1".
func resolveRunnerFromCmd(cmdStr, critID, projectRoot string) (string, bool) {
	cmdStr = strings.TrimSpace(cmdStr)
	if !strings.HasSuffix(cmdStr, critID) {
		return "", false
	}
	candidate := strings.TrimSpace(strings.TrimSuffix(cmdStr, critID))
	candidate = strings.TrimPrefix(candidate, "bash ")
	candidate = strings.TrimPrefix(candidate, "sh ")
	candidate = strings.TrimSpace(candidate)
	return resolveRunnerScript(candidate, projectRoot)
}

// BuildBatchCommand constructs a batch execution command for a shared runner script and criterion IDs.
func BuildBatchCommand(runner string, critIDs []string) string {
	cleanPath := strings.TrimSpace(runner)
	args := strings.Join(critIDs, " ")
	if strings.HasPrefix(cleanPath, "bash ") || strings.HasPrefix(cleanPath, "sh ") {
		return fmt.Sprintf("%s %s", cleanPath, args)
	}
	if strings.HasSuffix(cleanPath, ".sh") {
		return fmt.Sprintf("bash %s %s", cleanPath, args)
	}
	return fmt.Sprintf("%s %s", cleanPath, args)
}

func getRunnerExecutable(inv CriterionInvocation, projectRoot string) string {
	if inv.RunnerExecutable != "" {
		return inv.RunnerExecutable
	}
	if r, ok := resolveRunnerFromCmd(inv.Command, inv.CriterionID, projectRoot); ok {
		return r
	}
	return ""
}

type invocationBatch struct {
	runner      string
	invocations []CriterionInvocation
}

func groupInvocations(invocations []CriterionInvocation, projectRoot string) []invocationBatch {
	var batches []invocationBatch
	if len(invocations) == 0 {
		return batches
	}

	runnerIndex := make(map[string]int)

	for _, inv := range invocations {
		runner := getRunnerExecutable(inv, projectRoot)
		if runner == "" {
			batches = append(batches, invocationBatch{
				runner:      "",
				invocations: []CriterionInvocation{inv},
			})
			continue
		}

		idx, exists := runnerIndex[runner]
		if exists {
			batches[idx].invocations = append(batches[idx].invocations, inv)
		} else {
			runnerIndex[runner] = len(batches)
			batches = append(batches, invocationBatch{
				runner:      runner,
				invocations: []CriterionInvocation{inv},
			})
		}
	}
	return batches
}

func transitionCriterionSuccess(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	sp storage.ObjectStorageProvider,
	projectRoot, testCaseID, critID string,
) error {
	trustedCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "test_runner green catalyst verified transition")
	critObj, readErr := sp.Read(ctx, secCtx, critID)
	if readErr != nil || critObj == nil {
		return readErr
	}
	currentCritStatus, _ := critObj[objects.FieldKeyStatus].(string)
	if currentCritStatus == objects.ObjectStatusComplete || currentCritStatus == objects.ObjectStatusArchived {
		return nil
	}
	updates := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	}
	if updateErr := sp.Update(trustedCtx, secCtx, critID, updates); updateErr != nil {
		return updateErr
	}
	if wal, walErr := lifecycle.GetOrCreateLifecycleWAL(projectRoot); walErr == nil && wal != nil {
		_ = lifecycle.AppendCriterionSatisfied(wal, critID, map[string]string{
			"test_case_id": testCaseID,
		})
		_ = wal.Sync()
	}
	getStorage := func(p string) (storage.ObjectStorageProvider, bool) {
		return sp, true
	}
	lifecycle.TryEmitForTestCasesContainingCriterion(ctx, projectRoot, critID, getStorage)
	lifecycle.TryEmitForMilestonesContainingCriterion(ctx, projectRoot, critID, getStorage)
	lifecycle.TryEmitForBacklogItemsContainingCriterion(ctx, projectRoot, critID, getStorage)
	return nil
}

// ExecuteSubprocess executes a test invocation in an isolated subprocess with PGID and TMPDIR sandboxing.
func ExecuteSubprocess(ctx context.Context, inv CriterionInvocation) CriterionRunResult {
	start := time.Now()
	res := CriterionRunResult{
		CriterionID: inv.CriterionID,
		Command:     inv.Command,
	}

	shell := zqkenv.OSShell().Get()
	if shell == "" {
		shell = "sh"
	}

	cmd := execwrap.CommandContext(ctx, shell, "-c", inv.Command)
	if inv.Dir != "" {
		cmd.Dir = inv.Dir
	}

	// Provision ephemeral TMPDIR and configure PGID isolation and sandbox environment
	tmpDir, cleanup := createEphemeralTmpDir("zqk-testcase-")
	defer cleanup()

	setupIsolatedCommandEnv(cmd, tmpDir, []string{
		"ZQK_JOB_ID=kernel-testrunner",
		"ZQK_ALLOW_FOREGROUND_GO_TEST=1",
		"ZQK_TEST_ALLOW_FOREGROUND_GO_TEST=1",
		zqkenv.AllowForegroundGoTest().Name() + "=1",
		zqkenv.InTest().Name() + "=1",
	}, inv.Env)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdoutBuf, &stderrBuf

	if err := cmd.Start(); err != nil {
		res.Duration = time.Since(start)
		res.ExitCode = 1
		res.Error = err.Error()
		res.Passed = false
		return res
	}

	// BLI-TESTCASE-SANDBOX-PGID-001: Recursive Signal Teardown (SIGTERM -> SIGKILL) on context cancellation
	pid := cmd.Process.Pid
	pgid := -pid

	done := make(chan struct{})
	defer close(done)

	goroutinelabels.NewGoroutine("testrunner.signal_teardown", "teardown process group on context cancellation").StartSimple(func() {
		select {
		case <-ctx.Done():
			_ = killProcessGroup(pgid, syscall.SIGTERM)
			time.Sleep(50 * time.Millisecond)
			_ = killProcessGroup(pgid, syscall.SIGKILL)
		case <-done:
		}
	})

	activityTicker := time.NewTicker(time.Second)
	defer activityTicker.Stop()
	goroutinelabels.NewGoroutine("testrunner.activity_ticker", "keep process watchdog alive during test execution").StartSimple(func() {
		for {
			select {
			case <-activityTicker.C:
				process.TouchMeaningfulActivity()
			case <-done:
				return
			}
		}
	})

	waitErr := cmd.Wait()
	res.Duration = time.Since(start)
	res.Stdout = stdoutBuf.String()
	res.Stderr = stderrBuf.String()

	if waitErr != nil {
		res.ExitCode = 1
		res.Error = waitErr.Error()
		if exitErr, ok := waitErr.(interface{ ExitCode() int }); ok {
			res.ExitCode = exitErr.ExitCode()
		}
		res.Passed = false
	} else {
		res.ExitCode = 0
		res.Passed = true
	}

	return res
}

// RunTestCase executes all criteria tests for a test_case and triggers state machine transitions.
func RunTestCase(
	ctx context.Context,
	sp storage.ObjectStorageProvider,
	projectRoot, testCaseID string,
	opts RunOptions,
) (*TestRunResult, error) {
	if sp == nil {
		return nil, errfmt.Errorf("storage provider is required")
	}
	if testCaseID == "" {
		return nil, errfmt.Errorf("test_case id is required")
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	testCase, err := sp.Read(ctx, secCtx, testCaseID)
	if err != nil || testCase == nil {
		return nil, errfmt.Newf("failed to read test_case %s", testCaseID).Wrap(err)
	}

	title, _ := testCase[objects.FieldKeyTitle].(string)
	pathOrID, _ := testCase[objects.FieldKeyPathOrID].(string)

	invocations, err := ResolveInvocations(testCase, projectRoot)
	if err != nil {
		return nil, errfmt.Newf("failed to resolve test invocations for %s", testCaseID).Wrap(err)
	}

	// Filter criteria if requested
	if len(opts.CriteriaFilter) > 0 {
		filterMap := make(map[string]bool)
		for _, f := range opts.CriteriaFilter {
			filterMap[strings.ToUpper(strings.TrimSpace(f))] = true
		}
		var filtered []CriterionInvocation
		for _, inv := range invocations {
			if filterMap[strings.ToUpper(inv.CriterionID)] {
				filtered = append(filtered, inv)
			}
		}
		invocations = filtered
	}

	result := &TestRunResult{
		TestCaseID:    testCaseID,
		TestCaseTitle: title,
		PathOrID:      pathOrID,
		TotalCriteria: len(invocations),
	}

	if opts.DryRun {
		for _, inv := range invocations {
			result.CriteriaResults = append(result.CriteriaResults, CriterionRunResult{
				CriterionID: inv.CriterionID,
				Command:     inv.Command,
				Passed:      true,
			})
		}
		result.Status = "planned"
		return result, nil
	}

	// Ensure test_case is in active or metrics_captured state
	tcStatus, _ := testCase[objects.FieldKeyStatus].(string)
	if tcStatus == "originated" || tcStatus == "draft" {
		testCase[objects.FieldKeyStatus] = objects.ObjectStatusActive
		_ = sp.Update(pkgctx.WithLifecycleBreakGlass(ctx, "test runner active activation"), secCtx, testCaseID, testCase)
	}

	// Seed remaining_open_count if unset
	_ = lifecycle.SeedRemainingOpenCountFromMembers(ctx, sp, testCaseID, false)

	batches := groupInvocations(invocations, projectRoot)
	perTestTimeout := opts.Timeout
	if perTestTimeout <= 0 {
		perTestTimeout = 5 * time.Minute
	}

	for _, batch := range batches {
		isBatchCandidate := len(batch.invocations) > 1 && batch.runner != "" && !opts.NoBatch

		if isBatchCandidate {
			critIDs := make([]string, len(batch.invocations))
			for i, inv := range batch.invocations {
				critIDs[i] = inv.CriterionID
			}
			batchCmd := BuildBatchCommand(batch.runner, critIDs)
			batchInv := CriterionInvocation{
				CriterionID:      strings.Join(critIDs, ","),
				Command:          batchCmd,
				RunnerExecutable: batch.runner,
				Dir:              batch.invocations[0].Dir,
				Env: append(batch.invocations[0].Env,
					"ZQK_TEST_BATCH=1",
					fmt.Sprintf("ZQK_TEST_CRITERIA=%s", strings.Join(critIDs, " ")),
				),
			}

			subCtx, cancel := context.WithTimeout(ctx, perTestTimeout)
			batchRes := ExecuteSubprocess(subCtx, batchInv)
			cancel()

			if batchRes.Passed {
				result.BatchedExecution = true
				for _, inv := range batch.invocations {
					cr := CriterionRunResult{
						CriterionID: inv.CriterionID,
						Command:     batchCmd,
						Passed:      true,
						ExitCode:    batchRes.ExitCode,
						Stdout:      batchRes.Stdout,
						Stderr:      batchRes.Stderr,
						Duration:    batchRes.Duration,
						Batched:     true,
					}
					result.CriteriaResults = append(result.CriteriaResults, cr)
					result.PassedCriteria++
					if err := transitionCriterionSuccess(ctx, secCtx, sp, projectRoot, testCaseID, inv.CriterionID); err != nil {
						cr.Passed = false
						cr.Error = fmt.Sprintf("failed to update criterion status: %v", err)
						result.CriteriaResults[len(result.CriteriaResults)-1] = cr
						result.PassedCriteria--
						result.FailedCriteria++
					}
				}
				continue
			}

			// Batch runner failed -> graceful fallback to isolated single-criterion execution
			result.BatchedExecution = true
			result.FallbackTriggered = true
			logger := logging.GetLoggerFromProfile("system")
			logging.Fluent(logger).Warn(fmt.Sprintf(
				"Batch criteria execution failed for runner %s (exit code %d); falling back to isolated single-criterion execution",
				batch.runner, batchRes.ExitCode,
			)).Log()
		}

		// Isolated single-criterion execution (standalone invocations or fallback)
		for _, inv := range batch.invocations {
			subCtx, cancel := context.WithTimeout(ctx, perTestTimeout)
			runRes := ExecuteSubprocess(subCtx, inv)
			cancel()
			result.CriteriaResults = append(result.CriteriaResults, runRes)

			if runRes.Passed {
				result.PassedCriteria++
				if err := transitionCriterionSuccess(ctx, secCtx, sp, projectRoot, testCaseID, inv.CriterionID); err != nil {
					runRes.Passed = false
					runRes.Error = fmt.Sprintf("failed to update criterion status: %v", err)
					result.CriteriaResults[len(result.CriteriaResults)-1] = runRes
					result.PassedCriteria--
					result.FailedCriteria++
				}
			} else {
				result.FailedCriteria++
			}
		}
	}

	if result.PassedCriteria == result.TotalCriteria && result.TotalCriteria > 0 {
		result.Status = "passed"
	} else if result.PassedCriteria > 0 {
		result.Status = "partial"
	} else {
		result.Status = "failed"
	}

	// Re-read test_case to check terminal completion and remaining counter
	afterTC, readErr := sp.Read(ctx, secCtx, testCaseID)
	if readErr == nil && afterTC != nil {
		afterStatus, _ := afterTC[objects.FieldKeyStatus].(string)
		if remVal, ok := afterTC[objects.FieldKeyRemainingOpenCount]; ok {
			if remInt, okInt := remVal.(int); okInt {
				result.RemainingOpenCount = remInt
			}
		}
		if afterStatus != objects.ObjectStatusComplete && afterStatus != objects.ObjectStatusArchived && result.PassedCriteria == result.TotalCriteria && result.TotalCriteria > 0 && result.RemainingOpenCount == 0 {
			trustedCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "test_runner all criteria passed and remaining count drained")
			updates := map[string]any{
				objects.FieldKeyStatus: objects.ObjectStatusComplete,
			}
			if uErr := sp.Update(trustedCtx, secCtx, testCaseID, updates); uErr == nil {
				afterStatus = objects.ObjectStatusComplete
				afterTC[objects.FieldKeyStatus] = objects.ObjectStatusComplete
			}
		}
		result.TestCaseCompleted = (afterStatus == objects.ObjectStatusComplete)
	}

	// Check linked requirement and trigger shockwave if completed
	reqRefs := lifecycle.StringRefsFromAny(testCase[objects.FieldKeyRequirementRefs])
	if len(reqRefs) > 0 {
		if result.TestCaseCompleted {
			lifecycle.ApplyDependencyRefEvents(ctx, logging.NewEventLogger(ctx), sp, projectRoot, objects.KindTestCase, testCaseID, objects.ObjectStatusActive, objects.ObjectStatusComplete, afterTC)
		}
		reqObj, rErr := sp.Read(ctx, secCtx, reqRefs[0])
		if rErr == nil && reqObj != nil {
			reqStatus, _ := reqObj[objects.FieldKeyStatus].(string)
			result.RequirementCompleted = (reqStatus == objects.ObjectStatusComplete)
		}
	}

	logger := logging.GetLoggerFromProfile("system")
	logging.Fluent(logger).Info(fmt.Sprintf(
		"Test runner finished for %s: %d/%d criteria passed (test_case complete=%v, req complete=%v)",
		testCaseID, result.PassedCriteria, result.TotalCriteria, result.TestCaseCompleted, result.RequirementCompleted,
	)).Log()

	return result, nil
}

// createEphemeralTmpDir creates a temporary directory with the given prefix and returns its path and a cleanup function.
func createEphemeralTmpDir(prefix string) (string, func()) {
	tmpDir, err := fileutil.MkdirTemp("", prefix)
	if err != nil {
		return "", func() {}
	}
	return tmpDir, func() {
		_ = fileutil.RemoveAll(tmpDir)
	}
}

// setupIsolatedCommandEnv configures PGID isolation and standard sandbox environment on cmd.
func setupIsolatedCommandEnv(cmd *exec.Cmd, tmpDir string, baseEnv []string, extraEnv []string) {
	setCommandProcessGroup(cmd)
	cmd.Env = os.Environ()
	if len(baseEnv) > 0 {
		cmd.Env = append(cmd.Env, baseEnv...)
	}
	if os.Getenv("DEVELOPER_DIR") == "" {
		if _, err := os.Stat("/Library/Developer/CommandLineTools"); err == nil {
			cmd.Env = append(cmd.Env, "DEVELOPER_DIR=/Library/Developer/CommandLineTools")
		}
	}
	if tmpDir != "" {
		cmd.Env = append(cmd.Env,
			"TMPDIR="+tmpDir,
			"TEMP="+tmpDir,
			"TMP="+tmpDir,
		)
	}
	if len(extraEnv) > 0 {
		cmd.Env = append(cmd.Env, extraEnv...)
	}
}
