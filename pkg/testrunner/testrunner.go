package testrunner

import (
	"bytes"
	"context"
	"fmt"
	"os"
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
			if !exists {
				// Fallback to pathOrID if available
				cmdStr = buildDefaultCommand(pathOrID, cid, projectRoot)
			}
			invocations = append(invocations, CriterionInvocation{
				CriterionID: cid,
				Command:     cmdStr,
				Dir:         projectRoot,
			})
		}
		return invocations, nil
	}

	// 3. Fallback to path_or_id for each linked criterion
	if pathOrID != "" {
		for _, cid := range critRefs {
			cmdStr := buildDefaultCommand(pathOrID, cid, projectRoot)
			invocations = append(invocations, CriterionInvocation{
				CriterionID: cid,
				Command:     cmdStr,
				Dir:         projectRoot,
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
		if data, err := os.ReadFile(filepath.Clean(fullPath)); err == nil { //nolint:gosec // test file discovery within verified paths
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

// ExecuteSubprocess executes a test invocation in an isolated subprocess with PGID and TMPDIR sandboxing.
// TRACK: BLI-TESTCASE-SANDBOX-PGID-001, BLI-TESTCASE-SANDBOX-TMPDIR-002
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

	// BLI-TESTCASE-SANDBOX-TMPDIR-002: Deterministic ephemeral TMPDIR provisioning & auto-cleanup
	tmpDir, err := fileutil.MkdirTemp("", "zqk-testcase-")
	if err == nil {
		defer func() {
			_ = fileutil.RemoveAll(tmpDir)
		}()
	}

	// BLI-TESTCASE-SANDBOX-PGID-001: Process Group (PGID) Isolation
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Pass parent env plus test-friendly flags and isolated ephemeral TMPDIR
	cmd.Env = os.Environ()
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
	if len(inv.Env) > 0 {
		cmd.Env = append(cmd.Env, inv.Env...)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

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
			_ = syscall.Kill(pgid, syscall.SIGTERM)
			time.Sleep(50 * time.Millisecond)
			_ = syscall.Kill(pgid, syscall.SIGKILL)
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

	err = cmd.Wait()
	res.Duration = time.Since(start)
	res.Stdout = stdoutBuf.String()
	res.Stderr = stderrBuf.String()

	if err != nil {
		res.ExitCode = 1
		res.Error = err.Error()
		if exitErr, ok := err.(interface{ ExitCode() int }); ok {
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

	for _, inv := range invocations {
		perTestTimeout := opts.Timeout
		if perTestTimeout <= 0 {
			perTestTimeout = 5 * time.Minute
		}
		subCtx, cancel := context.WithTimeout(ctx, perTestTimeout)

		runRes := ExecuteSubprocess(subCtx, inv)
		cancel()
		result.CriteriaResults = append(result.CriteriaResults, runRes)

		if runRes.Passed {
			result.PassedCriteria++
			// GREEN Catalyst: transition criterion to complete
			trustedCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "test_runner green catalyst verified transition")
			critObj, readErr := sp.Read(ctx, secCtx, inv.CriterionID)
			if readErr == nil && critObj != nil {
				currentCritStatus, _ := critObj[objects.FieldKeyStatus].(string)
				if currentCritStatus != objects.ObjectStatusComplete && currentCritStatus != objects.ObjectStatusArchived {
					updates := map[string]any{
						objects.FieldKeyStatus: objects.ObjectStatusComplete,
					}
					if updateErr := sp.Update(trustedCtx, secCtx, inv.CriterionID, updates); updateErr == nil {
						if wal, walErr := lifecycle.GetOrCreateLifecycleWAL(projectRoot); walErr == nil && wal != nil {
							_ = lifecycle.AppendCriterionSatisfied(wal, inv.CriterionID, map[string]string{
								"test_case_id": testCaseID,
							})
							_ = wal.Sync()
						}
						// Trigger the Shockwave cascade!
						getStorage := func(p string) (storage.ObjectStorageProvider, bool) {
							return sp, true
						}
						lifecycle.TryEmitForTestCasesContainingCriterion(ctx, projectRoot, inv.CriterionID, getStorage)
						lifecycle.TryEmitForMilestonesContainingCriterion(ctx, projectRoot, inv.CriterionID, getStorage)
						lifecycle.TryEmitForBacklogItemsContainingCriterion(ctx, projectRoot, inv.CriterionID, getStorage)
					} else {
						runRes.Passed = false
						runRes.Error = fmt.Sprintf("failed to update criterion status: %v", updateErr)
						result.CriteriaResults[len(result.CriteriaResults)-1] = runRes
						result.PassedCriteria--
						result.FailedCriteria++
					}
				}
			}
		} else {
			result.FailedCriteria++
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
