package scheduler

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_LifecycleCoordinationAndPID_DeepCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	// 1. ReloadConfigRequest
	if ReloadConfigRequestExists(tmpDir) {
		t.Errorf("expected reload config request not to exist initially")
	}
	err := WriteReloadConfigRequest(tmpDir)
	if err != nil {
		t.Fatalf("failed to write reload config request: %v", err)
	}
	if !ReloadConfigRequestExists(tmpDir) {
		t.Errorf("expected reload config request to exist")
	}
	ConsumeReloadConfigRequest(tmpDir)
	if ReloadConfigRequestExists(tmpDir) {
		t.Errorf("expected reload config request to be consumed")
	}

	// 2. PID File and KeepAlive helpers
	_ = RemovePIDFile(tmpDir)
	_ = removeKeepAlive(tmpDir)

	alive, _, err := IsSchedulerAlive(tmpDir)
	if alive || err != nil {
		t.Errorf("expected scheduler not alive initially: alive=%v, err=%v", alive, err)
	}

	err = WritePIDFile(tmpDir)
	if err != nil {
		t.Fatalf("failed to write pid file: %v", err)
	}
	pid, err := readPIDFile(tmpDir)
	if err != nil || pid != os.Getpid() {
		t.Errorf("unexpected pid read: %d, expected %d, err=%v", pid, os.Getpid(), err)
	}

	err = writeKeepAlive(tmpDir)
	if err != nil {
		t.Fatalf("failed to write keep alive: %v", err)
	}
	kt, err := readKeepAlive(tmpDir)
	if err != nil || kt.IsZero() {
		t.Errorf("failed to read keep alive: %v, time=%v", err, kt)
	}
	alive, _, err = IsSchedulerAlive(tmpDir)
	if !alive || err != nil {
		t.Errorf("expected scheduler alive after writeKeepAlive & WritePIDFile: alive=%v, err=%v", alive, err)
	}

	_ = RemoveKeepAlive(tmpDir)
	_ = RemovePIDFile(tmpDir)

	if !isPIDFileNotExist(os.ErrNotExist) {
		t.Errorf("expected true for os.ErrNotExist")
	}
	if isPIDFileNotExist(errors.New("other error")) {
		t.Errorf("expected false for other error")
	}

	// 3. Process checking
	if IsDaemonProcessAlive(0) {
		t.Errorf("expected false for pid 0")
	}
	if IsProcessRunning(0) || IsProcessRunning(-1) {
		t.Errorf("expected false for pid <= 0")
	}
	if !IsProcessRunning(os.Getpid()) {
		t.Errorf("expected current pid to be running")
	}

	_ = isZombieProcess(0)
	_ = isZombieProcess(os.Getpid())
	_ = isSchedulerDaemonProcess(0)
	_ = isSchedulerDaemonProcess(os.Getpid())
	_ = processBelongsToProjectRoot(os.Getpid(), tmpDir)

	// 4. Runtime thread count
	_ = GetRuntimeThreadCount()

	// 5. readFileWithTimeout
	testFile := filepath.Join(tmpDir, "read_timeout.txt")
	_ = os.WriteFile(testFile, []byte("content"), 0644)
	data, err := readFileWithTimeout(testFile, 1*time.Second)
	if err != nil || string(data) != "content" {
		t.Errorf("unexpected readFileWithTimeout: %q, err=%v", string(data), err)
	}
	_, err = readFileWithTimeout(filepath.Join(tmpDir, "nonexistent"), 10*time.Millisecond)
	if err == nil {
		t.Errorf("expected error for nonexistent file")
	}

	// 6. ResolveProjectRootFromCWD
	_ = ResolveProjectRootFromCWD()
}

func TestExtended_MeshLeaseSupervision_Wave38(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	handler := NewMeshLeaseSupervisionHandler(sp, tmpDir, logger).(*MeshLeaseSupervisionHandler)

	ctx := context.Background()

	// 1. ensureSubprocess error paths
	// Missing consumer kernel
	err = handler.ensureSubprocess(ctx, "lease-1", "KERNEL-nonexistent")
	if err == nil {
		t.Errorf("expected error for missing consumer kernel")
	}

	// Create object with invalid endpoint
	bgCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "test-reason")
	secCtx := pkgctx.NewSystemSecurityContext()
	k1 := map[string]any{
		objects.FieldKeyID:                 "ATK-invalid-ep",
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:             objects.ObjectStatusError,
		objects.FieldKeyTitle:              "Invalid Endpoint Kernel",
		objects.FieldKeyDescription:        "Invalid endpoint description",
		objects.FieldKeyAssigneePersonaRef: "software_engineer",
		objects.FieldKeyEndpoint:           "http://remote.host:8080",
	}
	if err := sp.Create(bgCtx, secCtx, k1); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}
	err = handler.ensureSubprocess(ctx, "lease-2", "ATK-invalid-ep")
	if err == nil {
		t.Errorf("expected error for non-file endpoint")
	}

	// Create object with file:// endpoint to non-existent dir
	k2 := map[string]any{
		objects.FieldKeyID:                 "ATK-missing-dir",
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:             objects.ObjectStatusError,
		objects.FieldKeyTitle:              "Missing Dir Kernel",
		objects.FieldKeyDescription:        "Missing dir description",
		objects.FieldKeyAssigneePersonaRef: "software_engineer",
		objects.FieldKeyEndpoint:           "file:///nonexistent/project/dir",
	}
	if err := sp.Create(bgCtx, secCtx, k2); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}
	err = handler.ensureSubprocess(ctx, "lease-3", "ATK-missing-dir")
	if err == nil {
		t.Errorf("expected error for missing project root dir")
	}
}

func TestExtended_RunWrapperRetry_Wave38(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	handler := NewRunWrapperHandlerWithProjectRoot(sp, logger, nil, nil, tmpDir).(*RunWrapperHandler)
	t.Cleanup(func() { handler.StopNotificationContext() })

	// 1. effectiveRunWrapperTimeoutSeconds
	job := &ScheduledJob{
		MaxRuntimeSeconds: 300,
	}
	to := effectiveRunWrapperTimeoutSeconds(150, job)
	if to != 150 {
		t.Errorf("expected 150s from param, got %d", to)
	}
	toJob := effectiveRunWrapperTimeoutSeconds(0, job)
	if toJob != 300 {
		t.Errorf("expected 300s from job, got %d", toJob)
	}
	toDef := effectiveRunWrapperTimeoutSeconds(0, &ScheduledJob{})
	if toDef != DefaultMaxRuntimeSeconds {
		t.Errorf("expected default %d, got %d", DefaultMaxRuntimeSeconds, toDef)
	}

	// 2. runWrapperCollectTestFailuresFromOutput
	nonTestJob := &ScheduledJob{Command: "echo", CommandArgs: []string{"hi"}}
	fails, metrics := handler.runWrapperCollectTestFailuresFromOutput(nonTestJob, false, nil, nil, "")
	if fails != nil || metrics != nil {
		t.Errorf("expected nil for non-test command")
	}

	testJob := &ScheduledJob{
		ID:          "SCH-test-fail",
		Command:     "go",
		CommandArgs: []string{"test", "./..."},
	}
	sw := newStreamingOutputWriter(io.Discard, 1024)
	sw.Write([]byte("--- FAIL: TestSample (0.01s)\nFAIL\n"))
	fails, metrics = handler.runWrapperCollectTestFailuresFromOutput(testJob, false, sw, sw, "")
	if len(fails) == 0 {
		t.Errorf("expected at least 1 failed test")
	}
	if metrics == nil {
		t.Errorf("expected non-nil metrics")
	}

	// 3. getExecutor
	exe := handler.getExecutor()
	if exe == nil {
		t.Errorf("expected non-nil executor")
	}

	// 4. emitBundleProgress
	ctx := context.Background()
	bundleJob := &ScheduledJob{
		ID:      "SCH-run-bundle-123",
		JobType: JobTypeRunWrapper,
	}
	handler.emitBundleProgress(ctx, bundleJob, "bundle_running")
}
