package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// setupCycle31IsolatedEnv provisions hermetic storage and temp directories with zero swallowed errors.
func setupCycle31IsolatedEnv(t *testing.T) (string, storagepkg.ObjectStorageProvider) {
	tempDir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), tempDir)
	t.Setenv(zqkenv.TestRoot().Name(), tempDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		shutdownErr := sp.Shutdown(context.Background())
		require.NoError(t, shutdownErr)
	})
	return tempDir, sp
}

// mockCycle31Handler implements JobHandler for testing execution paths.
type mockCycle31Handler struct {
	executeFunc func(ctx context.Context, job *ScheduledJob) error
}

func (m *mockCycle31Handler) Execute(ctx context.Context, job *ScheduledJob) error {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, job)
	}
	return nil
}

// createCycle31JWTToken generates signed JWT tokens for adversarial authentication testing.
func createCycle31JWTToken(t *testing.T, secret, issuer, sub string, exp time.Time, perms []string, scope string) string {
	claims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   sub,
			ExpiresAt: jwt.NewNumericDate(exp),
		},
		Permissions: perms,
		Scope:       scope,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return tokenStr
}

func TestSchedulerCycle31_Adversarial_AuthHook_DenyAndFallback(t *testing.T) {
	t.Parallel()
	hook := NewDenyAuthHook("denied_type")
	req, err := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
	require.NoError(t, err)

	auth, sub, perms, authErr := hook.Authenticate(context.Background(), req)
	assert.False(t, auth)
	assert.Empty(t, sub)
	assert.Nil(t, perms)
	require.Error(t, authErr)
	assert.Contains(t, authErr.Error(), "unknown auth type \"denied_type\"")

	fallbacks := []string{"", "none", "stub", "mock", "unknown_scheme", "JWT"}
	for _, fb := range fallbacks {
		fbHook := createAuthHook(fb, nil)
		require.NotNil(t, fbHook)
		fAuth, fSub, fPerms, fErr := fbHook.Authenticate(context.Background(), req)
		assert.False(t, fAuth)
		assert.Empty(t, fSub)
		assert.Nil(t, fPerms)
		assert.Error(t, fErr)
	}
}

func TestSchedulerCycle31_Adversarial_AuthHook_JWT_MalformedHeaders(t *testing.T) {
	t.Parallel()
	hook := NewJWTAuthHook("secret123", "https://issuer.zqk.dev")

	emptyHook := NewJWTAuthHook("", "https://issuer.zqk.dev")
	req, err := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer validtoken")
	auth, sub, perms, err := emptyHook.Authenticate(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, auth)
	assert.Empty(t, sub)
	assert.Nil(t, perms)

	reqNoAuth, err := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
	require.NoError(t, err)
	auth, _, _, err = hook.Authenticate(context.Background(), reqNoAuth)
	require.NoError(t, err)
	assert.False(t, auth)

	badHeaders := []string{
		"Basic dXNlcjpwYXNz",
		"Token abcxyz",
		"Bearer",
		"Bearer    ",
		"Bearer not-a-valid-jwt-token-string",
	}
	for _, badHdr := range badHeaders {
		reqBad, reqErr := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
		require.NoError(t, reqErr)
		reqBad.Header.Set("Authorization", badHdr)
		badAuth, _, _, hookErr := hook.Authenticate(context.Background(), reqBad)
		require.NoError(t, hookErr)
		assert.False(t, badAuth, "header %q should not authenticate", badHdr)
	}
}

func TestSchedulerCycle31_Adversarial_AuthHook_JWT_CryptographicFailures(t *testing.T) {
	t.Parallel()
	secret := "secret-cycle31"
	issuer := "https://issuer.zqk.dev"
	hook := NewJWTAuthHook(secret, issuer)

	wrongSecretToken := createCycle31JWTToken(t, "wrong-secret", issuer, "sub1", time.Now().Add(time.Hour), nil, "")
	req1, err := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
	require.NoError(t, err)
	req1.Header.Set("Authorization", "Bearer "+wrongSecretToken)
	auth1, _, _, err := hook.Authenticate(context.Background(), req1)
	require.NoError(t, err)
	assert.False(t, auth1)

	expiredToken := createCycle31JWTToken(t, secret, issuer, "sub1", time.Now().Add(-10*time.Minute), nil, "")
	req2, err := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
	require.NoError(t, err)
	req2.Header.Set("Authorization", "Bearer "+expiredToken)
	auth2, _, _, err := hook.Authenticate(context.Background(), req2)
	require.NoError(t, err)
	assert.False(t, auth2)

	wrongIssuerToken := createCycle31JWTToken(t, secret, "https://evil.zqk.dev", "sub1", time.Now().Add(time.Hour), nil, "")
	req3, err := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
	require.NoError(t, err)
	req3.Header.Set("Authorization", "Bearer "+wrongIssuerToken)
	auth3, _, _, err := hook.Authenticate(context.Background(), req3)
	require.NoError(t, err)
	assert.False(t, auth3)

	emptySubToken := createCycle31JWTToken(t, secret, issuer, "", time.Now().Add(time.Hour), nil, "")
	req4, err := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
	require.NoError(t, err)
	req4.Header.Set("Authorization", "Bearer "+emptySubToken)
	auth4, _, _, err := hook.Authenticate(context.Background(), req4)
	require.NoError(t, err)
	assert.False(t, auth4)
}

func TestSchedulerCycle31_Adversarial_AuthHook_JWT_PermissionsAndScopeParsing(t *testing.T) {
	t.Parallel()
	secret := "secret-cycle31-scope"
	issuer := "https://issuer.zqk.dev"
	hook := NewJWTAuthHook(secret, issuer)

	token1 := createCycle31JWTToken(t, secret, issuer, "sub-admin", time.Now().Add(time.Hour), []string{"job:exec", "job:cancel"}, "")
	req1, err := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
	require.NoError(t, err)
	req1.Header.Set("Authorization", "Bearer "+token1)
	auth1, sub1, perms1, err := hook.Authenticate(context.Background(), req1)
	require.NoError(t, err)
	assert.True(t, auth1)
	assert.Equal(t, "sub-admin", sub1)
	assert.Equal(t, []string{"job:exec", "job:cancel"}, perms1)

	token2 := createCycle31JWTToken(t, secret, issuer, "sub-worker", time.Now().Add(time.Hour), nil, "read write execute")
	req2, err := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
	require.NoError(t, err)
	req2.Header.Set("Authorization", "Bearer "+token2)
	auth2, sub2, perms2, err := hook.Authenticate(context.Background(), req2)
	require.NoError(t, err)
	assert.True(t, auth2)
	assert.Equal(t, "sub-worker", sub2)
	assert.Equal(t, []string{"read", "write", "execute"}, perms2)

	token3 := createCycle31JWTToken(t, secret, issuer, "sub-comma", time.Now().Add(time.Hour), nil, "read,write,admin")
	req3, err := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
	require.NoError(t, err)
	req3.Header.Set("Authorization", "Bearer "+token3)
	auth3, _, perms3, err := hook.Authenticate(context.Background(), req3)
	require.NoError(t, err)
	assert.True(t, auth3)
	assert.Equal(t, []string{"read", "write", "admin"}, perms3)

	token4 := createCycle31JWTToken(t, secret, issuer, "sub-default", time.Now().Add(time.Hour), nil, "")
	req4, err := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
	require.NoError(t, err)
	req4.Header.Set("Authorization", "Bearer "+token4)
	auth4, _, perms4, err := hook.Authenticate(context.Background(), req4)
	require.NoError(t, err)
	assert.True(t, auth4)
	assert.Equal(t, []string{"callback:write"}, perms4)
}

func TestSchedulerCycle31_Adversarial_AuthHook_APIKey_Boundaries(t *testing.T) {
	t.Parallel()
	keys := map[string]string{
		"valid-key-1":   "subject-worker-1",
		"empty-sub-key": "",
	}
	hook := NewAPIKeyAuthHook(keys)

	req1, err := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
	require.NoError(t, err)
	auth1, _, _, err := hook.Authenticate(context.Background(), req1)
	require.NoError(t, err)
	assert.False(t, auth1)

	nilHook := NewAPIKeyAuthHook(nil)
	req2, err := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
	require.NoError(t, err)
	req2.Header.Set("X-API-Key", "valid-key-1")
	auth2, _, _, err := nilHook.Authenticate(context.Background(), req2)
	require.NoError(t, err)
	assert.False(t, auth2)

	req3, err := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
	require.NoError(t, err)
	req3.Header.Set("X-API-Key", "empty-sub-key")
	auth3, _, _, err := hook.Authenticate(context.Background(), req3)
	require.NoError(t, err)
	assert.False(t, auth3)

	req4, err := http.NewRequest(http.MethodPost, "https://localhost/callback?api_key=valid-key-1", nil)
	require.NoError(t, err)
	auth4, sub4, perms4, err := hook.Authenticate(context.Background(), req4)
	require.NoError(t, err)
	assert.True(t, auth4)
	assert.Equal(t, "subject-worker-1", sub4)
	assert.Equal(t, []string{"callback:write"}, perms4)

	req5, err := http.NewRequest(http.MethodPost, "https://localhost/callback?api_key=valid-key-1", nil)
	require.NoError(t, err)
	req5.Header.Set("X-API-Key", "unknown-key")
	auth5, _, _, err := hook.Authenticate(context.Background(), req5)
	require.NoError(t, err)
	assert.False(t, auth5)
}

func TestSchedulerCycle31_Adversarial_AuthHook_X509_And_OAuth2(t *testing.T) {
	t.Parallel()
	emptyCAHook := NewX509AuthHook("")
	req1, err := http.NewRequest(http.MethodPost, "https://localhost/callback", nil)
	require.NoError(t, err)
	auth1, _, _, err := emptyCAHook.Authenticate(context.Background(), req1)
	require.NoError(t, err)
	assert.False(t, auth1)

	missingCAHook := NewX509AuthHook("/tmp/nonexistent-ca-path-cycle31.pem")
	auth2, _, _, err := missingCAHook.Authenticate(context.Background(), req1)
	require.NoError(t, err)
	assert.False(t, auth2)

	corruptCAPath := filepath.Join(t.TempDir(), "corrupt-ca.pem")
	err = fileutil.WriteFile(corruptCAPath, []byte("INVALID PEM CONTENT"), paths.FilePerm644)
	require.NoError(t, err)
	corruptCAHook := NewX509AuthHook(corruptCAPath)
	auth3, _, _, err := corruptCAHook.Authenticate(context.Background(), req1)
	require.NoError(t, err)
	assert.False(t, auth3)

	oauthHook := NewOAuth2AuthHook()
	authOAuth, subOAuth, permsOAuth, oauthErr := oauthHook.Authenticate(context.Background(), req1)
	assert.False(t, authOAuth)
	assert.Empty(t, subOAuth)
	assert.Nil(t, permsOAuth)
	require.Error(t, oauthErr)
	assert.Contains(t, oauthErr.Error(), "OAuth2 authentication not yet implemented")
}

func TestSchedulerCycle31_Adversarial_CallbackListener_UnauthorizedDenial(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	job := &ScheduledJob{ID: "SCH-test-cycle31"}

	nilHandler := &CallbackListenerHandler{authHook: nil, logger: logger}
	rec1 := httptest.NewRecorder()
	req1, err := http.NewRequest(http.MethodPost, "/callbacks/trigger", nil)
	require.NoError(t, err)
	assert.True(t, nilHandler.authenticateRequest(rec1, req1))

	denyHandler := &CallbackListenerHandler{authHook: NewDenyAuthHook("denied"), logger: logger}
	rec2 := httptest.NewRecorder()
	req2, err := http.NewRequest(http.MethodPost, "/callbacks/trigger", nil)
	require.NoError(t, err)
	assert.False(t, denyHandler.authenticateRequest(rec2, req2))
	assert.Equal(t, http.StatusInternalServerError, rec2.Code)
	denyHandler.handleCallback(rec2, req2, "trigger", job)

	apiHook := NewAPIKeyAuthHook(map[string]string{"key-1": "worker-1"})
	apiHandler := &CallbackListenerHandler{authHook: apiHook, logger: logger}
	rec3 := httptest.NewRecorder()
	req3, err := http.NewRequest(http.MethodPost, "/callbacks/trigger", nil)
	require.NoError(t, err)
	assert.False(t, apiHandler.authenticateRequest(rec3, req3))
	assert.Equal(t, http.StatusUnauthorized, rec3.Code)
	apiHandler.handleCallback(rec3, req3, "trigger", job)

	rec4 := httptest.NewRecorder()
	req4, err := http.NewRequest(http.MethodPost, "/callbacks/trigger", nil)
	require.NoError(t, err)
	req4.Header.Set("X-API-Key", "key-1")
	assert.True(t, apiHandler.authenticateRequest(rec4, req4))
	assert.Equal(t, "worker-1", req4.Context().Value(authSubjectKey{}))
	assert.Equal(t, []string{"callback:write"}, req4.Context().Value(authPermissionsKey{}))
}

func TestSchedulerCycle31_Adversarial_CASRecovery_InvalidEnvelopes(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx := context.Background()

	runCASRecoveryForKind(ctx, "", objects.KindSchedulerJob, logger, nil)
	runCASRecoveryForKind(ctx, t.TempDir(), "", logger, nil)
	runCASRecoveryForKind(ctx, "/path/does/not/exist/cycle31", objects.KindSchedulerJob, logger, nil)
	runCASRecoveryForKind(ctx, t.TempDir(), "non_existent_kind_xyz", logger, nil)

	tempDir, sp := setupCycle31IsolatedEnv(t)
	runCASRecoveryForKind(ctx, tempDir, objects.KindSchedulerJob, logger, sp)
}

func TestSchedulerCycle31_Adversarial_Watchdog_DefaultsAndNegativeIntervals(t *testing.T) {
	t.Parallel()
	interceptor := NewWatchdogInterceptor(nil, "", -10*time.Second)
	require.NotNil(t, interceptor)
	assert.NotNil(t, interceptor.tracker)
	assert.Equal(t, 5*time.Second, interceptor.sweepInterval)
	assert.Equal(t, 5*time.Minute, interceptor.LockSweepThreshold)

	tracker := NewHeartbeatTracker()
	err := tracker.Register("", WorkerMeta{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workerID cannot be empty")

	err = tracker.Register("worker-neg", WorkerMeta{
		Interval:       -1 * time.Second,
		StaleThreshold: -5 * time.Second,
	})
	require.NoError(t, err)
	status, ok := tracker.GetWorker("worker-neg")
	require.True(t, ok)
	assert.Equal(t, 5*time.Second, status.Meta.Interval)
	assert.Equal(t, 30*time.Second, status.Meta.StaleThreshold)
	assert.Equal(t, WorkerStateHealthy, status.State)

	err = tracker.Register("worker-neg", WorkerMeta{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestSchedulerCycle31_Adversarial_Watchdog_StaleDetectionAndRecovery(t *testing.T) {
	t.Parallel()
	tracker := NewHeartbeatTracker()

	err := tracker.Ping("unknown-worker")
	require.Error(t, err)

	err = tracker.Register("worker-live", WorkerMeta{StaleThreshold: 50 * time.Millisecond})
	require.NoError(t, err)

	tracker.MarkStalled("worker-live")
	stalledStatus, ok := tracker.GetWorker("worker-live")
	require.True(t, ok)
	assert.Equal(t, WorkerStateStalled, stalledStatus.State)
	assert.Equal(t, 1, stalledStatus.MissedCount)

	err = tracker.Ping("worker-live")
	require.NoError(t, err)
	healedStatus, ok := tracker.GetWorker("worker-live")
	require.True(t, ok)
	assert.Equal(t, WorkerStateHealthy, healedStatus.State)

	tracker.mu.Lock()
	tracker.workers["worker-completed"] = &WorkerStatus{
		ID:         "worker-completed",
		State:      WorkerStateCompleted,
		LastPingAt: time.Now().Add(-1 * time.Hour),
		Meta:       WorkerMeta{StaleThreshold: time.Second},
	}
	tracker.workers["worker-aborted"] = &WorkerStatus{
		ID:         "worker-aborted",
		State:      WorkerStateAborted,
		LastPingAt: time.Now().Add(-1 * time.Hour),
		Meta:       WorkerMeta{StaleThreshold: time.Second},
	}
	tracker.mu.Unlock()

	staleWorkers := tracker.GetStaleWorkers(time.Now())
	for _, sw := range staleWorkers {
		assert.NotEqual(t, "worker-completed", sw.ID)
		assert.NotEqual(t, "worker-aborted", sw.ID)
	}

	tracker.Unregister("worker-live")
	_, found := tracker.GetWorker("worker-live")
	assert.False(t, found)

	tracker.Reset()
	assert.Empty(t, tracker.ListWorkers())
}

func TestSchedulerCycle31_Adversarial_Watchdog_StalledWorkerCascadeAndAbort(t *testing.T) {
	t.Parallel()
	tracker := NewHeartbeatTracker()
	tempDir := t.TempDir()
	interceptor := NewWatchdogInterceptor(tracker, tempDir, 20*time.Millisecond)

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	workerID := "worker-stalled-target"
	err := tracker.Register(workerID, WorkerMeta{
		Kind:           "daemon",
		Interval:       10 * time.Millisecond,
		StaleThreshold: 20 * time.Millisecond,
		CancelFunc:     workerCancel,
	})
	require.NoError(t, err)

	var notifiedID string
	interceptor.OnStalledWorker = func(status WorkerStatus) {
		notifiedID = status.ID
	}

	tracker.mu.Lock()
	tracker.workers[workerID].LastPingAt = time.Now().Add(-10 * time.Minute)
	tracker.mu.Unlock()

	interceptor.SweepAndRecover(context.Background())

	assert.Equal(t, workerID, notifiedID)
	assert.ErrorIs(t, workerCtx.Err(), context.Canceled)

	status, ok := tracker.GetWorker(workerID)
	require.True(t, ok)
	assert.Equal(t, WorkerStateStalled, status.State)
	assert.Equal(t, 1, status.MissedCount)
}

func TestSchedulerCycle31_Adversarial_Watchdog_StaleLockEviction(t *testing.T) {
	t.Parallel()
	tracker := NewHeartbeatTracker()
	tempDir := t.TempDir()

	lockDir := filepath.Join(tempDir, paths.ProjectDataDir, "locks")
	err := fileutil.MkdirAll(lockDir, paths.DirPerm750)
	require.NoError(t, err)

	staleLockFile := filepath.Join(lockDir, "dead_worker.lock")
	err = fileutil.WriteFile(staleLockFile, []byte("pid:12345"), paths.FilePerm600)
	require.NoError(t, err)

	oldTime := time.Now().Add(-1 * time.Hour)
	err = fileutil.Chtimes(staleLockFile, oldTime, oldTime)
	require.NoError(t, err)

	interceptor := NewWatchdogInterceptor(tracker, tempDir, 50*time.Millisecond)
	interceptor.LockSweepThreshold = 5 * time.Minute

	interceptor.SweepAndRecover(context.Background())

	_, statErr := fileutil.Stat(staleLockFile)
	assert.True(t, fileutil.IsNotExist(statErr))
}

func TestSchedulerCycle31_Adversarial_Watchdog_ContextCancellation(t *testing.T) {
	t.Parallel()
	tracker := NewHeartbeatTracker()
	interceptor := NewWatchdogInterceptor(tracker, t.TempDir(), 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	goroutinelabels.NewGoroutine("test.cycle31.watchdog", "watchdog loop cancel").StartSimple(func() {
		interceptor.Start(ctx)
		close(done)
	})

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WatchdogInterceptor.Start failed to terminate promptly upon context cancellation")
	}
}

func TestSchedulerCycle31_Adversarial_QueueSaturation_UnstartedPoolAndRouting(t *testing.T) {
	t.Parallel()
	s := &Scheduler{logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))}

	err := s.submitTriggeredJob(triggeredJobWork{job: &ScheduledJob{ID: "SCH-sat-1"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scheduler triggered pool not started")

	critJob := &ScheduledJob{TriggerType: "immediate", Category: CategoryTesting, JobType: "run_wrapper", Priority: JobPriorityCritical}
	normJob := &ScheduledJob{TriggerType: "immediate", Category: CategoryTesting, JobType: "run_wrapper", Priority: JobPriorityNormal}
	assert.True(t, priorityDispatchJob(critJob))
	assert.False(t, priorityDispatchJob(normJob))
	assert.False(t, priorityDispatchJob(nil))

	assert.Equal(t, 1, s.incrementDispatchDropRetryCount("job-dropped"))
	assert.Equal(t, 2, s.incrementDispatchDropRetryCount("job-dropped"))
	assert.Equal(t, 2, s.getDispatchDropRetryCount("job-dropped"))
	s.clearDispatchDropRetryCount("job-dropped")
	assert.Equal(t, 0, s.getDispatchDropRetryCount("job-dropped"))

	assert.Equal(t, dispatchWaitStageGoroutineCeiling, dispatchWaitStageForDrop("cron", dispatchPressureReasonResourceWaitDeadlineExceeded))
	assert.Equal(t, dispatchWaitStageExecuteJobOuterCtx, dispatchWaitStageForDrop("execute_job", jobExecReasonDispatchDeadline))
	assert.Equal(t, dispatchWaitStageTriggeredPoolSubmit, dispatchWaitStageForDrop("immediate_job_submit", dispatchPressureReasonResourceWaitDeadlineExceeded))
	assert.Empty(t, dispatchWaitStageForDrop("unknown", "unknown"))

	s.recordDispatchAttemptDropped("test", "test", nil)
}

func TestSchedulerCycle31_Adversarial_JobAdmission_Predicates(t *testing.T) {
	t.Parallel()
	now := time.Now()

	assert.False(t, oneTimeImmediateShouldStartOnReload(nil, false))
	assert.False(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate, Enabled: true}, true))
	assert.False(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate, Enabled: true, LastRunAt: &now}, false))
	assert.False(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate, Enabled: false}, false))
	assert.False(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate, Enabled: true, Status: StatusDisabled}, false))
	assert.False(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate, Enabled: true, Status: objects.ObjectStatusArchived}, false))
	assert.False(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeReusable, TriggerType: TriggerTypeImmediate, Enabled: true}, false))
	assert.False(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeTimer, Enabled: true}, false))
	assert.False(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ID: "SCH-run-bundle-1", ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate, Enabled: true}, false))
	assert.False(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate, Category: CategoryTesting, Enabled: true}, false))

	assert.True(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate, Enabled: true, CallbackOnError: "echo err"}, false))
	assert.True(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate, Enabled: true, CallbackOnCompletion: "echo done"}, false))
	assert.True(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate, Enabled: true, Category: CategoryManual}, false))
	assert.True(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate, Enabled: true, Category: CategoryUser}, false))
	assert.True(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate, Enabled: true, Priority: JobPriorityHigh}, false))
	assert.True(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate, Enabled: true, Priority: JobPriorityCritical}, false))
	assert.False(t, oneTimeImmediateShouldStartOnReload(&ScheduledJob{ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate, Enabled: true, Priority: JobPriorityNormal}, false))

	assert.False(t, jobWarrantsAdmissionHourglass(nil))
	assert.False(t, jobWarrantsAdmissionHourglass(&ScheduledJob{Enabled: false}))
	assert.False(t, jobWarrantsAdmissionHourglass(&ScheduledJob{Enabled: true, Status: StatusDisabled}))
	assert.False(t, jobWarrantsAdmissionHourglass(&ScheduledJob{Enabled: true, LastRunAt: &now}))
	assert.False(t, jobWarrantsAdmissionHourglass(&ScheduledJob{Enabled: true, ExecutionMode: ExecutionModeReusable, TriggerType: TriggerTypeImmediate}))
	assert.False(t, jobWarrantsAdmissionHourglass(&ScheduledJob{Enabled: true, ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeTimer}))
	assert.True(t, jobWarrantsAdmissionHourglass(&ScheduledJob{Enabled: true, ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate, CallbackOnError: "callback"}))
}

func TestSchedulerCycle31_Adversarial_JobAdmission_OccupancyAndStorage(t *testing.T) {
	t.Parallel()
	s := &Scheduler{logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))}

	assert.False(t, s.isJobOccupied(nil))

	job := &ScheduledJob{ID: "SCH-occ-test", ExecutionMode: ExecutionModeOneTime, TriggerType: TriggerTypeImmediate}
	assert.False(t, s.isJobOccupied(job))

	s.markOneTimeImmediatePending(job)
	assert.True(t, s.isImmediateDispatchPending(job.ID))
	assert.True(t, s.isJobOccupied(job))

	s.clearImmediateDispatchPending(job.ID)
	assert.False(t, s.isImmediateDispatchPending(job.ID))
	assert.False(t, s.isJobOccupied(job))

	assert.False(t, s.admissionAlreadyFailed("SCH-occ-test"))
	assert.True(t, s.markAdmissionFailed("SCH-occ-test"))
	assert.False(t, s.markAdmissionFailed("SCH-occ-test"))
	assert.True(t, s.admissionAlreadyFailed("SCH-occ-test"))

	var nilSched *Scheduler
	assert.False(t, nilSched.isJobOccupied(job))
	assert.False(t, nilSched.markAdmissionFailed("SCH-x"))
	assert.False(t, nilSched.admissionAlreadyFailed("SCH-x"))
	assert.False(t, s.markAdmissionFailed(""))
	assert.False(t, s.admissionAlreadyFailed(""))
	s.markOneTimeImmediatePending(nil)
	s.clearImmediateDispatchPending("")
}

func TestSchedulerCycle31_Adversarial_JobAdmission_FailAdmissionAndStorage(t *testing.T) {
	_, sp := setupCycle31IsolatedEnv(t)
	s := NewScheduler(sp, objects.GetGlobalSpecLoader(), objects.GetGlobalLifecycleLoader()).(*Scheduler)

	s.FailJobAdmission(context.Background(), "", "empty_job_id")
	s.FailJobAdmission(context.Background(), "SCH-nonexistent-cycle31", "missing_job")

	assert.False(t, s.AdmitJobFromStorage(context.Background(), ""))
	assert.False(t, s.AdmitJobFromStorage(context.Background(), "SCH-nonexistent-cycle31"))

	secCtx := pkgctx.NewSystemSecurityContext()
	jobData := map[string]any{
		objects.FieldKeyID:          "SCH-disabled-cycle31",
		objects.FieldKeyKind:        objects.KindSchedulerJob,
		objects.FieldKeyJobType:     "run_wrapper",
		objects.FieldKeyEnabled:     false,
		objects.FieldKeyStatus:      StatusDisabled,
		objects.FieldKeyTriggerType: "immediate",
	}
	createErr := sp.Create(pkgctx.WithPromoteOnCreate(context.Background()), secCtx, jobData)
	require.NoError(t, createErr)

	assert.False(t, s.AdmitJobFromStorage(context.Background(), "SCH-disabled-cycle31"))
}

func TestSchedulerCycle31_Adversarial_JobLoader_MalformedDefinitions(t *testing.T) {
	_, sp := setupCycle31IsolatedEnv(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	loader := NewJobLoader(sp, secCtx, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)), nil)

	_, err := loader.HydrateJob(map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "job missing id field")

	_, err = loader.HydrateJob(map[string]any{objects.FieldKeyID: "j1", objects.FieldKeyTriggerType: "timer"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timer trigger requires schedule_expression")

	_, err = loader.HydrateJob(map[string]any{objects.FieldKeyID: "j2", objects.FieldKeyTriggerType: "workflow"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow trigger requires workflow_ref")

	_, err = loader.HydrateJob(map[string]any{objects.FieldKeyID: "j3", objects.FieldKeyTriggerType: "event"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "event trigger requires event_filter")

	_, err = loader.HydrateJob(map[string]any{objects.FieldKeyID: "j4", objects.FieldKeyTriggerType: "lifecycle"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lifecycle trigger requires lifecycle_filter")

	_, err = loader.HydrateJob(map[string]any{objects.FieldKeyID: "j5", objects.FieldKeyTriggerType: "unknown_trigger_mode"})
	require.Error(t, err)

	job, err := loader.HydrateJob(map[string]any{
		objects.FieldKeyID:                "j6",
		objects.FieldKeyTriggerType:       "manual",
		objects.FieldKeyLogLevel:          "invalid_log_level",
		objects.FieldKeyPriority:          "invalid_priority",
		objects.FieldKeyMaxRuntimeSeconds: -99,
	})
	require.NoError(t, err)
	assert.Equal(t, "default", job.LogLevel)
	assert.Equal(t, JobPriorityNormal, job.Priority)
	assert.Equal(t, 3600, job.MaxRuntimeSeconds)

	now := time.Now()
	jobSkipped, err := loader.HydrateJob(map[string]any{
		objects.FieldKeyID:            "j7",
		objects.FieldKeyTriggerType:   "manual",
		objects.FieldKeyExecutionMode: "one_time",
		objects.FieldKeyLastRunAt:     now,
	})
	require.NoError(t, err)
	assert.Nil(t, jobSkipped)

	assert.Equal(t, "timer", ParseTriggerType("", nil, "j8"))
	assert.Equal(t, "timer", ParseTriggerType("unrecognized", nil, "j8"))
	assert.Equal(t, "manual", ParseTriggerType("manual", nil, "j8"))
	assert.Equal(t, JobPriorityNormal, rawJobPriority(map[string]any{}))
	assert.Equal(t, JobPriorityCritical, rawJobPriority(map[string]any{objects.FieldKeyPriority: JobPriorityCritical}))
	assert.Equal(t, JobPriorityNormal, rawJobPriority(map[string]any{objects.FieldKeyPriority: "invalid"}))
}

func TestSchedulerCycle31_Adversarial_JobExecution_ContextCancellationAndPanics(t *testing.T) {
	_, sp := setupCycle31IsolatedEnv(t)
	s := NewScheduler(sp, objects.GetGlobalSpecLoader(), objects.GetGlobalLifecycleLoader()).(*Scheduler)
	job := &ScheduledJob{
		ID:                "SCH-exec-adversarial",
		JobType:           "test_job",
		Category:          "testing",
		MaxRuntimeSeconds: 10,
	}

	panicHandler := &mockCycle31Handler{
		executeFunc: func(ctx context.Context, job *ScheduledJob) error {
			panic("simulated craftsman fault in job handler")
		},
	}
	panicErr := s.runSchedulerJobHandlerWithRetry(context.Background(), context.Background(), job, panicHandler, nil, time.Now())
	require.Error(t, panicErr)
	assert.Contains(t, panicErr.Error(), "panic in job execution: simulated craftsman fault in job handler")

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelHandler := &mockCycle31Handler{
		executeFunc: func(ctx context.Context, job *ScheduledJob) error {
			return cancelCtx.Err()
		},
	}
	cancelErr := s.runSchedulerJobHandlerWithRetry(context.Background(), cancelCtx, job, cancelHandler, nil, time.Now())
	require.Error(t, cancelErr)
	assert.ErrorIs(t, cancelErr, context.Canceled)

	deadlineCtx, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Minute))
	defer cancelDeadline()
	timeoutHandler := &mockCycle31Handler{
		executeFunc: func(ctx context.Context, job *ScheduledJob) error {
			return deadlineCtx.Err()
		},
	}
	timeoutErr := s.runSchedulerJobHandlerWithRetry(context.Background(), deadlineCtx, job, timeoutHandler, nil, time.Now().Add(-15*time.Second))
	require.Error(t, timeoutErr)
	assert.Contains(t, timeoutErr.Error(), "job exceeded max_runtime_seconds")
}
