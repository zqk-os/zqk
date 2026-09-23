package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtended_CleanupConfig_DeepCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	handler := NewCleanupConfigHandler(tmpDir, logger).(*CleanupConfigHandler)

	// 1. Parameter parsing helpers
	m := map[string]any{
		"str_key":   "hello",
		"int_key":   42,
		"slice_key": []any{"a", "b", 123},
	}
	if strParam(m, "str_key") != "hello" || strParam(m, "missing") != "" {
		t.Errorf("unexpected strParam result")
	}
	if intParam(m, "int_key") != 42 || intParam(m, "missing") != 0 {
		t.Errorf("unexpected intParam result")
	}
	sl := strSliceParam(m, "slice_key")
	if len(sl) != 2 || sl[0] != "a" || sl[1] != "b" {
		t.Errorf("unexpected strSliceParam result: %v", sl)
	}

	// parseDuration
	d, err := parseDuration("10m")
	if err != nil || d != 10*time.Minute {
		t.Errorf("unexpected parseDuration: %v, err=%v", d, err)
	}
	_, err = parseDuration("invalid")
	if err == nil {
		t.Errorf("expected error for invalid duration")
	}

	// 2. runDeleteFiles
	delFile := filepath.Join(tmpDir, "to_delete.txt")
	_ = os.WriteFile(delFile, []byte("delete me"), 0644)
	err = handler.runDeleteFiles(tmpDir, map[string]any{
		"path": "to_delete.txt",
	})
	if err != nil || fileutil.Exists(delFile) {
		t.Errorf("expected file to be deleted: %v, exists=%v", err, fileutil.Exists(delFile))
	}

	// 3. runTruncateFiles
	truncFile := filepath.Join(tmpDir, "to_truncate.txt")
	_ = os.WriteFile(truncFile, []byte("line1\nline2\nline3\nline4\n"), 0644)
	err = handler.runTruncateFiles(tmpDir, map[string]any{
		"path":            "to_truncate.txt",
		"keep_last_lines": 2,
	})
	if err != nil {
		t.Errorf("unexpected error in runTruncateFiles: %v", err)
	}

	// 4. runReapStaleLocks
	locksDir := filepath.Join(tmpDir, "locks")
	_ = fileutil.MkdirAll(locksDir, 0755)
	oldLock := filepath.Join(locksDir, "stale.lock")
	_ = os.WriteFile(oldLock, []byte("pid"), 0644)
	oldTime := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(oldLock, oldTime, oldTime)
	err = handler.runReapStaleLocks(tmpDir, map[string]any{
		"dir":       "locks",
		"stale_age": "1h",
	})
	if err != nil {
		t.Errorf("unexpected error in runReapStaleLocks: %v", err)
	}

	// 5. runReapTempFiles
	tempDir := filepath.Join(tmpDir, "temp")
	_ = fileutil.MkdirAll(tempDir, 0755)
	oldTemp := filepath.Join(tempDir, "temp.tmp")
	_ = os.WriteFile(oldTemp, []byte("temp"), 0644)
	_ = os.Chtimes(oldTemp, oldTime, oldTime)
	err = handler.runReapTempFiles(tmpDir, map[string]any{
		"dir":       "temp",
		"stale_age": "1h",
	})
	if err != nil {
		t.Errorf("unexpected error in runReapTempFiles: %v", err)
	}

	// 6. runEnforceLogRetention
	logsDir := filepath.Join(tmpDir, "logs")
	_ = fileutil.MkdirAll(logsDir, 0755)
	logFile := filepath.Join(logsDir, "test.log")
	_ = os.WriteFile(logFile, []byte("log data"), 0644)
	_ = os.Chtimes(logFile, oldTime, oldTime)
	err = handler.runEnforceLogRetention(tmpDir, map[string]any{
		"dir":     "logs",
		"max_age": "1h",
	})
	if err != nil {
		t.Errorf("unexpected error in runEnforceLogRetention: %v", err)
	}

	// 7. runCLI and runBuildTarget with invalid commands
	_ = handler.runCLI(context.Background(), tmpDir, map[string]any{"command": "nonexistent_cli_cmd_xyz"})
	_ = handler.runBuildTarget(context.Background(), tmpDir, map[string]any{"target": "clean"})
}

type mockSchedulerForWave39 struct {
	SchedulerInterface
}

func (m *mockSchedulerForWave39) TriggerJob(ctx context.Context, jobID string) error {
	return nil
}

func (m *mockSchedulerForWave39) TriggerJobByEvent(ctx context.Context, eventType, eventKind string, eventData map[string]any) error {
	return nil
}

func TestExtended_CallbackListener_DeepCoverage(t *testing.T) {
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
	mockSched := &mockSchedulerForWave39{}
	handlerIface := NewCallbackListenerHandler(sp, logger, mockSched, nil, nil)
	handler := handlerIface.(*CallbackListenerHandler)
	defer handler.StopNotificationContext()

	job := &ScheduledJob{ID: "SCH-cb-test"}

	// 1. BuildHTTPServer
	srv := handler.BuildHTTPServer("127.0.0.1:0", http.NotFoundHandler())
	if srv == nil {
		t.Errorf("expected non-nil http server")
	}

	// 2. authenticateRequest
	reqAuth := httptest.NewRequest("GET", "/health", nil)
	wAuth := httptest.NewRecorder()
	if !handler.authenticateRequest(wAuth, reqAuth) {
		t.Errorf("expected authentication to succeed when no authHook configured")
	}

	// 3. handleHealthCheck
	wHealth := httptest.NewRecorder()
	reqHealth := httptest.NewRequest("GET", "/health", nil)
	handler.handleHealthCheck(wHealth, reqHealth)
	if wHealth.Code != http.StatusOK {
		t.Errorf("expected 200 OK from health check, got %d", wHealth.Code)
	}

	// 4. handleJobComplete
	wComplete := httptest.NewRecorder()
	handler.handleJobComplete(wComplete, nil, map[string]any{"job_id": "SCH-1", "result": "ok"}, job)
	if wComplete.Code != http.StatusOK {
		t.Errorf("expected 200 OK from handleJobComplete, got %d", wComplete.Code)
	}

	// 5. handleJobError
	wError := httptest.NewRecorder()
	handler.handleJobError(wError, nil, map[string]any{"job_id": "SCH-1", "error": "failed"}, job)
	if wError.Code != http.StatusOK {
		t.Errorf("expected 200 OK from handleJobError, got %d", wError.Code)
	}

	// 6. handleJobStatus
	wStatus := httptest.NewRecorder()
	handler.handleJobStatus(wStatus, nil, map[string]any{"job_id": "SCH-1", "status": "running"}, job)
	if wStatus.Code != http.StatusOK {
		t.Errorf("expected 200 OK from handleJobStatus, got %d", wStatus.Code)
	}

	// 7. handleTriggerJob
	wTrigger := httptest.NewRecorder()
	reqTrigger := httptest.NewRequest("POST", "/trigger", nil)
	handler.handleTriggerJob(wTrigger, reqTrigger, map[string]any{"job_id": "SCH-1"}, job)
	if wTrigger.Code != http.StatusOK {
		t.Errorf("expected 200 OK from handleTriggerJob, got %d", wTrigger.Code)
	}

	// 8. handleEmitEvent
	wEvent := httptest.NewRecorder()
	reqEvent := httptest.NewRequest("POST", "/event", nil)
	handler.handleEmitEvent(wEvent, reqEvent, map[string]any{"event_type": "test_event"}, job)
	if wEvent.Code != http.StatusOK {
		t.Errorf("expected 200 OK from handleEmitEvent, got %d", wEvent.Code)
	}

	// 9. handleCallback generic with various types
	for _, cbType := range []string{"complete", "error", "status", "trigger", "event", "unknown"} {
		payload := map[string]any{"job_id": "SCH-1"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/callback/"+cbType, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.handleCallback(w, req, cbType, job)
	}

	// 10. registerRoutes
	mux := http.NewServeMux()
	handler.registerRoutes(mux, "/api/v1", job)

	// Test registered routes via mux
	wRoute := httptest.NewRecorder()
	rRoute := httptest.NewRequest("GET", "/api/v1/health", nil)
	mux.ServeHTTP(wRoute, rRoute)
	if wRoute.Code != http.StatusOK {
		t.Errorf("expected 200 OK from registered /health, got %d", wRoute.Code)
	}
}
