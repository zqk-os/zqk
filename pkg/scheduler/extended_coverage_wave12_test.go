package scheduler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

type mockAuthHook struct {
	authenticated bool
	subject       string
	permissions   []string
	err           error
}

func (m *mockAuthHook) Authenticate(ctx context.Context, r *http.Request) (bool, string, []string, error) {
	return m.authenticated, m.subject, m.permissions, m.err
}

type mockSchedulerForCallback struct {
	SchedulerInterface
	triggerErr error
}

func (m *mockSchedulerForCallback) TriggerJob(ctx context.Context, jobID string) error {
	return m.triggerErr
}

func TestExtended_CallbackListener_Comprehensive(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-cb-listener-*")
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
	notifCtx := NewNotificationContext(logger, nil)
	mockSched := &mockSchedulerForCallback{}
	h := NewCallbackListenerHandler(sp, logger, mockSched, nil, notifCtx).(*CallbackListenerHandler)

	job := &ScheduledJob{
		ID:                  "SCH-cb-test",
		ListenerPort:        0,
		ListenerPath:        "/callback",
		IdleShutdownSeconds: 1,
	}

	// 1. authenticateRequest
	// A. No auth hook configured
	req := httptest.NewRequest(http.MethodGet, "/callback/health", nil)
	rec := httptest.NewRecorder()
	if !h.authenticateRequest(rec, req) {
		t.Error("expected true when no auth hook configured")
	}

	// B. Auth hook returns error (500)
	h.authHook = &mockAuthHook{err: errors.New("auth internal failure")}
	recErr := httptest.NewRecorder()
	if h.authenticateRequest(recErr, req) {
		t.Error("expected false on auth error")
	}
	if recErr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", recErr.Code)
	}

	// C. Auth hook returns unauthenticated (401)
	h.authHook = &mockAuthHook{authenticated: false}
	recUnauth := httptest.NewRecorder()
	if h.authenticateRequest(recUnauth, req) {
		t.Error("expected false on unauthenticated")
	}
	if recUnauth.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", recUnauth.Code)
	}

	// D. Auth hook returns authenticated (200)
	h.authHook = &mockAuthHook{authenticated: true, subject: "agent-1", permissions: []string{"read", "write"}}
	recAuth := httptest.NewRecorder()
	if !h.authenticateRequest(recAuth, req) {
		t.Error("expected true on authenticated")
	}

	// 2. handleTriggerJob
	// A. Missing job_id
	emptyRec := httptest.NewRecorder()
	h.handleTriggerJob(emptyRec, req, map[string]any{}, job)
	if emptyRec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty job_id, got %d", emptyRec.Code)
	}

	// B. Trigger succeeds
	validRec := httptest.NewRecorder()
	h.handleTriggerJob(validRec, req, map[string]any{"job_id": "SCH-target-1"}, job)
	if validRec.Code != http.StatusOK {
		t.Errorf("expected 200 for valid trigger, got %d", validRec.Code)
	}

	// C. Trigger fails
	mockSched.triggerErr = errors.New("cannot trigger")
	failRec := httptest.NewRecorder()
	h.handleTriggerJob(failRec, req, map[string]any{"job_id": "SCH-target-1"}, job)
	if failRec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for failed trigger, got %d", failRec.Code)
	}

	// 3. shutdownServer
	// A. Server is nil
	h.shutdownServer()

	// B. Server exists
	srv := h.BuildHTTPServer("127.0.0.1:0", http.NewServeMux())
	h.server = srv
	h.shutdownServer()

	// 4. Execute with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = h.Execute(canceledCtx, job)
}

func TestExtended_AuditAggregationSession_EdgeBranches(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-audit-agg-*")
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

	handler := NewAuditAggregationHandler(sp).(*AuditAggregationHandler)

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	job := &ScheduledJob{
		ID: "SCH-audit-agg-edges",
		EnvironmentVariables: map[string]string{
			EnvKeyAggregationWindow: "1h",
			EnvKeyRetentionDuration: "2h",
			EnvKeyBatchSize:         "50",
		},
	}

	// Create canceled context to test cancellation paths in aggressive and proactive cleanup
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	session := &auditAggregationSession{
		handler:             handler,
		ctx:                 ctx,
		preAggCtx:           canceledCtx,
		job:                 job,
		secCtx:              secCtx,
		storageCtx:          storageCtx,
		service:             storagepkg.NewAuditAggregationService(sp),
		phaseDurations:      make(map[string]float64),
		windowDuration:      1 * time.Hour,
		deleteAfterDuration: 2 * time.Hour,
		effectiveRetention:  2 * time.Hour,
		batchSize:           50,
		windowStart:         time.Now().Add(-2 * time.Hour),
		windowEnd:           time.Now(),
		retentionMaxBatches: 2,
		catchUpMaxBatches:   2,
		archiveEnabled:      true,
		deleteEnabled:       false,
	}

	// Exercise cancellation branches
	session.runAggressiveCleanup()
	session.runProactiveCleanup()
	session.runRetentionFirstPass()

	// determineEffectiveRetention variations
	session.deleteAfterDuration = 10 * time.Minute
	session.determineEffectiveRetention()
	if session.effectiveRetention != 10*time.Minute {
		t.Errorf("expected 10m retention, got %v", session.effectiveRetention)
	}
}
