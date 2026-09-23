package scheduler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type mockSchedulerForRunWrapper struct {
	SchedulerInterface
	disabledJob *ScheduledJob
}

func (m *mockSchedulerForRunWrapper) DisableJobInStorage(ctx context.Context, job *ScheduledJob) {
	m.disabledJob = job
}

func TestExtended_RunWrapperRetry_CircuitBreakerAndFailures(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-rw-retry-*")
	if err != nil {
		t.Fatalf("temp dir failed: %v", err)
	}
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("storage failed: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	mockSched := &mockSchedulerForRunWrapper{}
	notifCtx := NewNotificationContext(logger, nil)

	h := &RunWrapperHandler{
		storage:             sp,
		logger:              logger,
		projectRoot:         tmpDir,
		failureCount:        10, // already at 10 to trip circuit breaker on next failure
		scheduler:           mockSched,
		notificationContext: notifCtx,
	}

	job := &ScheduledJob{
		ID:          "SCH-rw-cb-trip",
		Command:     "go",
		CommandArgs: []string{"test", "./..."},
		LogLevel:    runWrapperLogLevelVerbose,
	}

	stdoutWriter := newStreamingOutputWriter(nil, 1024)
	stdoutWriter.Write([]byte("test stdout line\n"))
	stderrWriter := newStreamingOutputWriter(nil, 1024)
	stderrWriter.Write([]byte("test stderr error line\n"))

	eventsWritten := 0
	writeEvent := func(ev map[string]any) {
		eventsWritten++
	}

	// 1. Test timeout path
	timeoutCtx, cancelTimeout := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelTimeout()

	termWritten, kind, reason, lastErr := h.runWrapperLogFailedAttempt(
		context.Background(),
		timeoutCtx,
		job,
		"go test ./...",
		0,
		5*time.Second,
		stdoutWriter,
		stderrWriter,
		writeEvent,
		false, // streamedToFiles
		true,  // isTestBundle
		tmpDir,
		5,
		0,
		"timeout reason",
		context.DeadlineExceeded,
		nil,
		false,
	)

	if !termWritten || kind == "" || reason == "" || lastErr == nil {
		t.Errorf("expected timeout failure handled: term=%v, kind=%s, reason=%s, err=%v", termWritten, kind, reason, lastErr)
	}

	// 2. Test exit failure path with circuit breaker trip (failureCount 10 -> 11)
	cmdCtx := context.Background()
	_, kindExit, reasonExit, _ := h.runWrapperLogFailedAttempt(
		context.Background(),
		cmdCtx,
		job,
		"go test ./...",
		1,
		1*time.Second,
		stdoutWriter,
		stderrWriter,
		writeEvent,
		false,
		false,
		tmpDir,
		5,
		1, // exitCode = 1
		"exit error",
		errors.New("exit status 1"),
		nil,
		false,
	)

	if kindExit == "" || reasonExit == "" {
		t.Errorf("expected exit failure kind and reason: %s, %s", kindExit, reasonExit)
	}
	if mockSched.disabledJob == nil || mockSched.disabledJob.ID != job.ID {
		t.Errorf("expected circuit breaker to disable job, got: %v", mockSched.disabledJob)
	}

	// 3. runWrapperCollectTestFailuresFromOutput with streamed files
	stdoutFile := JobStdoutFilePath(tmpDir, job.ID)
	stderrFile := JobStderrFilePath(tmpDir, job.ID)
	_ = fileutil.MkdirAll(filepath.Dir(stdoutFile), 0755)
	testFailedOutput := "=== RUN   TestFailureScenario\n--- FAIL: TestFailureScenario (0.02s)\nFAIL\n"
	_ = fileutil.WriteFile(stdoutFile, []byte(testFailedOutput), 0644)
	_ = fileutil.WriteFile(stderrFile, []byte("FAIL exit\n"), 0644)

	failedNames, summaryMap := h.runWrapperCollectTestFailuresFromOutput(
		job,
		true,
		stdoutWriter,
		stderrWriter,
		"FAIL exit",
	)

	if len(failedNames) == 0 {
		t.Errorf("expected failed test name to be parsed from file, got %v", failedNames)
	}
	if summaryMap == nil {
		t.Errorf("expected summary map")
	}

	// 4. runWrapperNotifyExhaustedFailure with pre_commit origin
	preCommitCtx := ContextWithTriggerOrigin(context.Background(), TriggerOriginPreCommit)
	h.runWrapperNotifyExhaustedFailure(
		preCommitCtx,
		job,
		"go test ./...",
		2,
		2,
		3*time.Second,
		"FAIL error",
		errors.New("tests failed"),
		"exit_status",
		"exit code 1",
		1,
		30,
		true,
		failedNames,
		summaryMap,
	)
}
