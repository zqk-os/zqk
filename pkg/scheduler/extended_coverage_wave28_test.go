package scheduler

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_RunWrapperRetry_DeepCoverage(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "test-runwrapper-retry-deep-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	handlerIface := NewRunWrapperHandlerWithProjectRoot(sp, logger, nil, nil, tmpDir)
	h := handlerIface.(*RunWrapperHandler)
	t.Cleanup(func() { h.StopNotificationContext() })

	// 1. effectiveRunWrapperTimeoutSeconds
	jobEmpty := &ScheduledJob{}
	if eff := effectiveRunWrapperTimeoutSeconds(10, jobEmpty); eff != 10 {
		t.Errorf("expected 10, got %d", eff)
	}
	jobWithMax := &ScheduledJob{MaxRuntimeSeconds: 30}
	if eff := effectiveRunWrapperTimeoutSeconds(0, jobWithMax); eff != 30 {
		t.Errorf("expected 30, got %d", eff)
	}
	if eff := effectiveRunWrapperTimeoutSeconds(0, jobEmpty); eff != DefaultMaxRuntimeSeconds {
		t.Errorf("expected default %d, got %d", DefaultMaxRuntimeSeconds, eff)
	}

	// 2. getExecutor
	exec := h.getExecutor()
	if exec == nil {
		t.Errorf("expected non-nil executor")
	}

	// 3. emitBundleProgress
	bundleJob := &ScheduledJob{
		ID:          "SCH-bundle-job",
		Command:     "go",
		CommandArgs: []string{"test", "./pkg/..."},
		Metadata: map[string]any{
			KeyBundleCommandFingerprint: "fp123",
		},
	}
	h.emitBundleProgress(ctx, bundleJob, "pass")

	bundleJobNoFp := &ScheduledJob{
		ID:          "SCH-bundle-job-nofp",
		Command:     "echo",
		CommandArgs: []string{"hello"},
	}
	h.emitBundleProgress(ctx, bundleJobNoFp, "completed")

	// 4. runWrapperCollectTestFailuresFromOutput
	nonTestJob := &ScheduledJob{
		Command:     "echo",
		CommandArgs: []string{"hi"},
	}
	outWriter := newStreamingOutputWriter(nil, 1024)
	errWriter := newStreamingOutputWriter(nil, 1024)
	fails, summary := h.runWrapperCollectTestFailuresFromOutput(nonTestJob, false, outWriter, errWriter, "")
	if fails != nil || summary != nil {
		t.Errorf("expected nil for non-test job failures")
	}

	testJob := &ScheduledJob{
		ID:          "SCH-test-collect",
		Command:     "go",
		CommandArgs: []string{"test", "./..."},
	}
	testErrWriter := newStreamingOutputWriter(nil, 1024)
	testOutWriter := newStreamingOutputWriter(nil, 1024)
	simulatedTestOutput := "=== RUN   TestFail1\n--- FAIL: TestFail1 (0.01s)\n=== RUN   TestPass\n--- PASS: TestPass (0.01s)\nFAIL\n"
	_, _ = testOutWriter.Write([]byte(simulatedTestOutput))

	fails, summary = h.runWrapperCollectTestFailuresFromOutput(testJob, false, testOutWriter, testErrWriter, "")
	if len(fails) != 1 || fails[0] != "TestFail1" {
		t.Errorf("expected [TestFail1], got %v", fails)
	}
	if summary == nil || summary[runWrapperFieldFailedTests] != 1 {
		t.Errorf("expected summary with 1 failed test, got %v", summary)
	}

	// 5. runWrapperLogFailedAttempt
	cmdCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
	defer cancel()

	var eventsWritten []map[string]any
	writeEvent := func(e map[string]any) {
		eventsWritten = append(eventsWritten, e)
	}

	// Timeout case (cmdCtx.Err() == context.DeadlineExceeded)
	term, fKind, fReason, lastErr := h.runWrapperLogFailedAttempt(
		ctx,
		cmdCtx,
		testJob,
		"go test ./...",
		0,
		2*time.Second,
		testOutWriter,
		testErrWriter,
		writeEvent,
		false,
		true,
		tmpDir,
		5,
		1,
		"timeout reason",
		errors.New("timed out"),
		nil,
		false,
	)
	if !term {
		t.Errorf("expected terminalEntryWritten true on timeout")
	}
	_ = fKind
	_ = fReason
	_ = lastErr

	// Normal exit failure case
	normCtx := context.Background()
	term2, fKind2, fReason2, lastErr2 := h.runWrapperLogFailedAttempt(
		ctx,
		normCtx,
		testJob,
		"go test ./...",
		1,
		1*time.Second,
		testOutWriter,
		testErrWriter,
		writeEvent,
		false,
		false,
		tmpDir,
		0,
		1,
		"some error",
		errors.New("exit status 1"),
		nil,
		false,
	)
	_ = term2
	_ = fKind2
	_ = fReason2
	_ = lastErr2

	// 6. runWrapperNotifyExhaustedFailure
	h.runWrapperNotifyExhaustedFailure(
		ctx,
		testJob,
		"go test ./...",
		2,
		2,
		1*time.Second,
		"stderr error",
		errors.New("failed after retries"),
		"exit_error",
		"failed with exit code 1",
		1,
		10,
		true,
		nil,
		nil,
	)
}
