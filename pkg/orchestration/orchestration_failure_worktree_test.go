package orchestration

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/hive/capability"
	"github.com/zqk-os/zqk/pkg/hivemind"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/rollback"
	"github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type cycle145MockPool struct {
	executeFn func(ctx context.Context, fn func(conn provider.GraphConnection) error) error
}

func (p *cycle145MockPool) Execute(ctx context.Context, fn func(conn provider.GraphConnection) error) error {
	if p.executeFn != nil {
		return p.executeFn(ctx, fn)
	}
	return nil
}

func (p *cycle145MockPool) GetConnection(_ context.Context) (provider.GraphConnection, error) {
	return nil, nil
}

func (p *cycle145MockPool) ReturnConnection(_ provider.GraphConnection) error {
	return nil
}

func (p *cycle145MockPool) Stats() provider.PoolStats {
	return provider.PoolStats{}
}

func (p *cycle145MockPool) Close() error {
	return nil
}

type cycle145MockConn struct {
	provider.GraphConnection
	queryResult   *provider.QueryResult
	queryErr      error
	createNodeErr error
	createEdgeErr error
}

func (c *cycle145MockConn) ExecuteQuery(_ context.Context, _ provider.Query) (*provider.QueryResult, error) {
	if c.queryErr != nil {
		return nil, c.queryErr
	}
	return c.queryResult, nil
}

func (c *cycle145MockConn) CreateNode(_ context.Context, _ provider.Node) error {
	if c.createNodeErr != nil {
		return c.createNodeErr
	}
	return nil
}

func (c *cycle145MockConn) CreateEdge(_ context.Context, _ provider.Edge) error {
	if c.createEdgeErr != nil {
		return c.createEdgeErr
	}
	return nil
}

type cycle145MockStorage struct {
	storage.NoopObjectStorage
	readFn   func(ctx context.Context, secCtx *storage.SecurityContext, id string) (map[string]any, error)
	createFn func(ctx context.Context, secCtx *storage.SecurityContext, obj map[string]any) error
	updateFn func(ctx context.Context, secCtx *storage.SecurityContext, id string, updates map[string]any) error
}

func (s *cycle145MockStorage) Read(ctx context.Context, secCtx *storage.SecurityContext, id string) (map[string]any, error) {
	if s.readFn != nil {
		return s.readFn(ctx, secCtx, id)
	}
	return s.NoopObjectStorage.Read(ctx, secCtx, id)
}

func (s *cycle145MockStorage) Create(ctx context.Context, secCtx *storage.SecurityContext, obj map[string]any) error {
	if s.createFn != nil {
		return s.createFn(ctx, secCtx, obj)
	}
	return s.NoopObjectStorage.Create(ctx, secCtx, obj)
}

func (s *cycle145MockStorage) Update(ctx context.Context, secCtx *storage.SecurityContext, id string, updates map[string]any) error {
	if s.updateFn != nil {
		return s.updateFn(ctx, secCtx, id, updates)
	}
	return s.NoopObjectStorage.Update(ctx, secCtx, id, updates)
}

type cycle145MockCAPLoop struct {
	mu       sync.Mutex
	alerts   []CAPAlert
	alertErr error
}

func (c *cycle145MockCAPLoop) Alert(_ context.Context, alert CAPAlert) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.alerts = append(c.alerts, alert)
	return c.alertErr
}

func TestCycle145_IntentPredictorHeuristics(t *testing.T) {
	ctx := context.Background()
	conn := &cycle145MockConn{
		queryResult: &provider.QueryResult{
			Rows: []map[string]any{
				{
					objects.FieldKeyKind: "cli_command",
					"props": map[string]any{
						"payload_command": "zqk object create goal --title test",
					},
				},
				{
					objects.FieldKeyKind: "cli_command",
					"props": map[string]any{
						"payload_command": "make build",
					},
				},
				{
					objects.FieldKeyKind: "cli_command",
					"props":              "not-a-map",
				},
				{
					objects.FieldKeyKind: "cli_command",
					"props": map[string]any{
						"payload_command": "git status",
					},
				},
			},
		},
	}
	pool := &cycle145MockPool{
		executeFn: func(_ context.Context, fn func(conn provider.GraphConnection) error) error {
			return fn(conn)
		},
	}

	engine := NewIntentPredictionHeuristicsEngine(pool)
	require.NotNil(t, engine)

	predictions, err := engine.PredictFromRecentMessages(ctx, time.Now().Add(-10*time.Minute))
	require.NoError(t, err)
	require.Len(t, predictions, 2)
	assert.Equal(t, "create_requirement", predictions[0].TargetAction)
	assert.Equal(t, "requirement", predictions[0].TargetObject)
	assert.Equal(t, "run_tests", predictions[1].TargetAction)
	assert.Equal(t, "test_suite", predictions[1].TargetObject)

	conn.queryErr = errors.New("simulated cypher failure")
	_, err = engine.PredictFromRecentMessages(ctx, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query recent messages for intent prediction")
}

func TestCycle145_SubagentRunner_TruthSentinelAndRetries(t *testing.T) {
	tempRoot := t.TempDir()
	t.Cleanup(func() {
		cleanupErr := fileutil.RemoveAll(tempRoot)
		assert.NoError(t, cleanupErr)
	})

	cfg := SentinelConfig{
		TruthSentinel:           true,
		EnterpriseTruthSentinel: true,
		MaxRetries:              1,
	}
	store := &cycle145MockStorage{
		updateFn: func(_ context.Context, _ *storage.SecurityContext, _ string, _ map[string]any) error {
			return nil
		},
	}
	capLoop := &cycle145MockCAPLoop{}
	runner := NewSubagentRunner(cfg, tempRoot, store, capLoop)
	require.NotNil(t, runner)

	states := []rollback.ObjectState{
		{ID: "OBJ-1", Kind: "req", State: map[string]any{"id": "OBJ-1", "kind": "req"}},
	}
	getStates := func() ([]rollback.ObjectState, error) {
		return states, nil
	}

	taskRuns := 0
	task := func(_ context.Context) error {
		taskRuns++
		return nil
	}

	ctx := context.Background()
	err := runner.RunSubagentTask(ctx, "sub-1", task, getStates)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rolled back after 1 retries")
	assert.Equal(t, 2, taskRuns)
	assert.Len(t, capLoop.alerts, 1)
	assert.Equal(t, "sub-1", capLoop.alerts[0].SubagentID)
}

func TestCycle145_SubagentRunner_IdleWatchdogAndRollbackFailure(t *testing.T) {
	tempRoot := t.TempDir()
	t.Cleanup(func() {
		cleanupErr := fileutil.RemoveAll(tempRoot)
		assert.NoError(t, cleanupErr)
	})

	cfg := SentinelConfig{
		IdleWatchdogEnabled: true,
		IdleWatchdogTimeout: 50 * time.Millisecond,
		IdleWatchdogTick:    10 * time.Millisecond,
		MaxRetries:          1,
	}
	store := &cycle145MockStorage{
		updateFn: func(_ context.Context, _ *storage.SecurityContext, _ string, _ map[string]any) error {
			return errors.New("simulated storage update failure on rollback")
		},
	}
	capLoop := &cycle145MockCAPLoop{}
	runner := NewSubagentRunner(cfg, tempRoot, store, capLoop)

	states := []rollback.ObjectState{
		{ID: "OBJ-RB-FAIL", Kind: "req", State: map[string]any{"id": "OBJ-RB-FAIL", "kind": "req"}},
	}
	getStates := func() ([]rollback.ObjectState, error) {
		return states, nil
	}

	err := runner.RunSubagentTask(context.Background(), "sub-watchdog", func(_ context.Context) error {
		return errors.New("underlying worker crashed")
	}, getStates)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rollback failed during retry")
}

func TestCycle145_SubagentRunner_CaptureFailureAndAlertFailure(t *testing.T) {
	tempRoot := t.TempDir()
	t.Cleanup(func() {
		cleanupErr := fileutil.RemoveAll(tempRoot)
		assert.NoError(t, cleanupErr)
	})

	cfg := SentinelConfig{MaxRetries: 0}
	capLoop := &cycle145MockCAPLoop{
		alertErr: errors.New("cap alert channel blocked"),
	}
	store := &cycle145MockStorage{
		updateFn: func(_ context.Context, _ *storage.SecurityContext, _ string, _ map[string]any) error {
			return nil
		},
	}
	runner := NewSubagentRunner(cfg, tempRoot, store, capLoop)

	captureErrGetStates := func() ([]rollback.ObjectState, error) {
		return nil, errors.New("state collector exploded")
	}

	err := runner.RunSubagentTask(context.Background(), "sub-fail-capture", func(_ context.Context) error {
		return nil
	}, captureErrGetStates)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to capture rollback point")

	err = runner.RunSubagentTask(context.Background(), "sub-alert-err", func(_ context.Context) error {
		return errors.New("fatal worker execution error")
	}, func() ([]rollback.ObjectState, error) {
		return nil, nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rolled back after 0 retries")
}

func TestCycle145_WorktreePool_CapacityAndClosed(t *testing.T) {
	baseDir := t.TempDir()
	t.Cleanup(func() {
		cleanupErr := fileutil.RemoveAll(baseDir)
		assert.NoError(t, cleanupErr)
	})

	ctx := context.Background()
	pool := NewWorktreePool(baseDir, 1)
	require.NotNil(t, pool)

	h1, err := pool.Acquire(ctx, "agent-primary", 5*time.Minute)
	require.NoError(t, err)
	require.NotNil(t, h1)
	assert.Equal(t, 1, pool.ActiveCount())

	_, err = pool.Acquire(ctx, "agent-overflow", 5*time.Minute)
	require.Error(t, err)
	assert.Equal(t, "worktree pool capacity exceeded", err.Error())

	err = pool.Close()
	require.NoError(t, err)

	_, err = pool.Acquire(ctx, "agent-after-close", 5*time.Minute)
	require.Error(t, err)
	assert.Equal(t, "worktree pool is closed", err.Error())
}

func TestCycle145_WorktreePool_MkdirAndReleaseErrors(t *testing.T) {
	baseDir := t.TempDir()
	t.Cleanup(func() {
		cleanupErr := fileutil.RemoveAll(baseDir)
		assert.NoError(t, cleanupErr)
	})

	ctx := context.Background()
	pool := NewWorktreePool(baseDir, 2)

	err := pool.Release(ctx, "non-existent-handle")
	require.Error(t, err)
	assert.Equal(t, "handle not found in pool", err.Error())

	filePath := filepath.Join(baseDir, "blocker-file")
	writeErr := fileutil.WriteFile(filePath, []byte("blocker"), 0600)
	require.NoError(t, writeErr)

	blockedPool := NewWorktreePool(filePath, 2)
	_, err = blockedPool.Acquire(ctx, "agent-blocked", time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create worktree directory")

	closeErr := blockedPool.Close()
	require.NoError(t, closeErr)
	closeErr = pool.Close()
	require.NoError(t, closeErr)
}

func TestCycle145_WorktreeSweeper_ExpirationAndContext(t *testing.T) {
	baseDir := t.TempDir()
	t.Cleanup(func() {
		cleanupErr := fileutil.RemoveAll(baseDir)
		assert.NoError(t, cleanupErr)
	})

	ctx := context.Background()
	pool := NewWorktreePool(baseDir, 5)
	sweeper := NewWorktreeSweeper(pool)
	require.NotNil(t, sweeper)

	hExpired, err := pool.Acquire(ctx, "agent-expired", -1*time.Hour)
	require.NoError(t, err)
	require.NotNil(t, hExpired)

	hActive, err := pool.Acquire(ctx, "agent-active", 1*time.Hour)
	require.NoError(t, err)
	require.NotNil(t, hActive)

	assert.Equal(t, 2, pool.ActiveCount())

	pruned, err := sweeper.SweepContext(ctx, time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, pruned)
	assert.Equal(t, 1, pool.ActiveCount())

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	prunedCanceled, err := sweeper.SweepContext(canceledCtx, time.Now())
	require.Error(t, err)
	assert.Equal(t, 0, prunedCanceled)
	assert.Equal(t, context.Canceled, err)

	closeErr := pool.Close()
	require.NoError(t, closeErr)
}

func TestCycle145_AutonomyInbox_CreateNodeAndEdgeErrors(t *testing.T) {
	ctx := context.Background()
	msg := Message{
		ID:          "msg-cycle145-1",
		Kind:        "backlog_item",
		Destination: "coord-target",
		Payload:     map[string]any{"bli_id": "BLI-1"},
		Timestamp:   time.Now(),
	}

	conn := &cycle145MockConn{
		createNodeErr: errors.New("node creation rejected by db"),
	}
	pool := &cycle145MockPool{
		executeFn: func(_ context.Context, fn func(conn provider.GraphConnection) error) error {
			return fn(conn)
		},
	}
	inbox := NewAutonomyInbox(pool)
	require.NotNil(t, inbox)

	err := inbox.RouteMessage(ctx, msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create message node")

	conn.createNodeErr = nil
	conn.createEdgeErr = errors.New("edge creation timeout")
	err = inbox.RouteMessage(ctx, msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to route message to destination")

	conn.createEdgeErr = nil
	err = inbox.RouteMessage(ctx, msg)
	require.NoError(t, err)
}

type cycle145TestManager struct {
	lastIntent RawIntent
}

func (m *cycle145TestManager) ProcessIntent(_ context.Context, intent RawIntent) (CapabilityRef, error) {
	m.lastIntent = intent
	return CapabilityRef{ID: "CAP-CYCLE145", Kind: "capability"}, nil
}

func TestCycle145_OrchestratorRegistry_NilAndRoundtrip(t *testing.T) {
	reg := &OrchestratorRegistry{}
	mgr := reg.GetManager()
	assert.Nil(t, mgr)

	testMgr := &cycle145TestManager{}
	reg.RegisterManager(testMgr)

	retrieved := reg.GetManager()
	require.NotNil(t, retrieved)

	ref, err := retrieved.ProcessIntent(context.Background(), RawIntent{Signature: "sig-test"})
	require.NoError(t, err)
	assert.Equal(t, "CAP-CYCLE145", ref.ID)
	assert.Equal(t, "sig-test", testMgr.lastIntent.Signature)
}

func TestCycle145_Session_GatesAndFailureTransitions(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	ctx := context.Background()

	storeReadErr := &cycle145MockStorage{
		readFn: func(_ context.Context, _ *storage.SecurityContext, id string) (map[string]any, error) {
			return nil, fmt.Errorf("policy lookup error for %s", id)
		},
	}
	sessWithStoreErr := NewSession(logger, storeReadErr)
	addErr := sessWithStoreErr.AddAgent(ctx, "agent-1")
	require.NoError(t, addErr)

	err := sessWithStoreErr.Start(ctx, "topic-fail", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch Truth Sentinel policy")
	assert.Equal(t, objects.ObjectStatusFailed, sessWithStoreErr.Status())

	storePolicyViolation := &cycle145MockStorage{
		readFn: func(_ context.Context, _ *storage.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{objects.FieldKeyID: id}, nil
		},
	}
	sessViolation := NewSession(logger, storePolicyViolation)
	err = sessViolation.Start(ctx, "topic-violation", map[string]any{
		"local_merge_to_main": true,
		"pr_link":             "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local merge to main attempted without a valid PR link")
	assert.Equal(t, objects.ObjectStatusFailed, sessViolation.Status())

	sessSuccess := NewSession(logger, storePolicyViolation)
	err = sessSuccess.Start(ctx, "topic-success", map[string]any{
		"local_merge_to_main": true,
		"pr_link":             "https://github.com/zqk-os/zqk/pull/145",
	})
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusCompleted, sessSuccess.Status())
}

type cycle145MockNegotiator struct {
	result *scheduler.NegotiationResult
	err    error
}

func (n *cycle145MockNegotiator) Propose(_ context.Context, _ scheduler.Proposal) (*scheduler.NegotiationResult, error) {
	return n.result, n.err
}

type cycle145MockMemory struct {
	err error
}

func (m *cycle145MockMemory) RetrieveSemantically(_ context.Context, _ string, _ int) ([]hivemind.MemoryResult, error) {
	return nil, m.err
}
func (cycle145MockMemory) RetrieveGraphContext(_ context.Context, _ string, _ int) (*hivemind.GraphSubgraph, error) {
	return nil, nil
}
func (cycle145MockMemory) QueryHybrid(_ context.Context, _ string, _ int, _ hivemind.HybridConstraints) ([]hivemind.MemoryResult, error) {
	return nil, nil
}
func (cycle145MockMemory) FindObjectsMissingVectors(_ context.Context, _ int) ([]string, error) {
	return nil, nil
}
func (cycle145MockMemory) LinkVectorID(_ context.Context, _, _ string) error {
	return nil
}

func TestCycle145_Manager_SynthesisAndCommitFailures(t *testing.T) {
	ctx := context.Background()
	neg := &cycle145MockNegotiator{
		result: &scheduler.NegotiationResult{Accepted: true},
	}
	mem := &cycle145MockMemory{}
	store := &cycle145MockStorage{
		createFn: func(_ context.Context, _ *storage.SecurityContext, _ map[string]any) error {
			return errors.New("cas storage write disk full")
		},
	}

	mgr := NewManager(neg, mem, store)
	require.NotNil(t, mgr)

	intent := RawIntent{
		Signature: "feature-alpha",
		Payload: map[string]any{
			"test_cases": []any{"tst-1", "tst-2"},
		},
	}
	_, err := mgr.ProcessIntent(ctx, intent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrMsgCommitmentFailed)

	dyn := &dynamicCapability{id: "dyn-id", title: "dyn-title"}
	assert.Equal(t, "dyn-id", dyn.Name())
	assert.Equal(t, "dyn-title", dyn.Description())
	evalOk, evalErr := dyn.EvaluatePolicy(ctx, capability.EvalContext{})
	require.NoError(t, evalErr)
	assert.True(t, evalOk)
}

func TestCycle145_PredictiveSpawner_HeuristicsAndErrors(t *testing.T) {
	ctx := context.Background()
	conn := &cycle145MockConn{
		queryErr: errors.New("db execution error"),
	}
	pool := &cycle145MockPool{
		executeFn: func(_ context.Context, fn func(conn provider.GraphConnection) error) error {
			return fn(conn)
		},
	}
	spawner := NewPredictiveSpawner(pool)
	require.NotNil(t, spawner)

	err := spawner.IngestEvent(ctx, AmbientEvent{
		Type:    "FILE_CREATED",
		Payload: map[string]any{"filepath": "pkg/orchestration/test_stub.go"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to schedule test generation task")

	conn.queryErr = nil
	err = spawner.IngestEvent(ctx, AmbientEvent{
		Type:    "FILE_CREATED",
		Payload: map[string]any{"filepath": "pkg/orchestration/test_stub.go"},
	})
	require.NoError(t, err)

	err = spawner.IngestEvent(ctx, AmbientEvent{
		Type:    "FILE_CREATED",
		Payload: map[string]any{"filepath": "pkg/orchestration/test_stub_test.go"},
	})
	require.NoError(t, err)

	err = spawner.IngestEvent(ctx, AmbientEvent{
		Type:    "USER_LOGGED_IN",
		Payload: map[string]any{"user": "admin"},
	})
	require.NoError(t, err)
}

func TestCycle145_IntentPrewarmer_HeuristicsAndErrors(t *testing.T) {
	ctx := context.Background()
	conn := &cycle145MockConn{
		queryErr: errors.New("prewarm cypher timeout"),
	}
	pool := &cycle145MockPool{
		executeFn: func(_ context.Context, fn func(conn provider.GraphConnection) error) error {
			return fn(conn)
		},
	}
	prewarmer := NewIntentPrewarmer(pool)
	require.NotNil(t, prewarmer)

	err := prewarmer.Prewarm(ctx, IntentPrediction{})
	require.NoError(t, err)

	err = prewarmer.Prewarm(ctx, IntentPrediction{TargetObject: "requirement"})
	require.Error(t, err)
	assert.Equal(t, "prewarm cypher timeout", err.Error())

	conn.queryErr = nil
	err = prewarmer.Prewarm(ctx, IntentPrediction{TargetObject: "test_suite"})
	require.NoError(t, err)
}

func TestCycle145_PolicyNegotiator_DriftAndReconcile(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	ctx := context.Background()
	negotiator := NewPolicyNegotiator(logger)
	require.NotNil(t, negotiator)

	driftProposal := scheduler.Proposal{
		Metadata: map[string]any{
			"raw": map[string]any{
				ConstVersionContext: "legacy-v1",
			},
		},
	}
	res, err := negotiator.Propose(ctx, driftProposal)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.Accepted)
	assert.Equal(t, ConstSchemaReconciled, res.Reason)

	standardProposal := scheduler.Proposal{
		Metadata: map[string]any{
			"raw": map[string]any{
				ConstVersionContext: "current",
			},
		},
	}
	resStd, err := negotiator.Propose(ctx, standardProposal)
	require.NoError(t, err)
	require.NotNil(t, resStd)
	assert.True(t, resStd.Accepted)
	assert.Equal(t, ConstStandardProposalAccepted, resStd.Reason)
}

func TestCycle145_ErrorsAndConstantsCoverage(t *testing.T) {
	assert.Equal(t, "policy negotiation failed", ErrMsgPolicyNegotiationFailed)
	assert.Equal(t, "memory retrieval failed", ErrMsgMemoryRetrievalFailed)
	assert.Equal(t, "commitment failed", ErrMsgCommitmentFailed)
	assert.Equal(t, "synthesis error", ErrMsgSynthesisError)
	assert.Equal(t, "synthesis replay failed", ErrMsgSynthesisReplayFailed)
	assert.Equal(t, "hive_activity.log", LogKeyHiveActivity)
	assert.Equal(t, "intent_submission", LogKeyIntentSubmission)
	assert.Equal(t, "capability_synthesis", OpNameCapabilitySynthesis)
}

func TestCycle145_WorktreePool_ConcurrentAcquireWithGoroutineLabels(t *testing.T) {
	baseDir := t.TempDir()
	t.Cleanup(func() {
		cleanupErr := fileutil.RemoveAll(baseDir)
		assert.NoError(t, cleanupErr)
	})

	pool := NewWorktreePool(baseDir, 10)
	var wg sync.WaitGroup
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		wg.Add(1)
		workerID := fmt.Sprintf("worker-%d", i)
		goroutinelabels.NewGoroutine("test", workerID).StartWithContext(ctx, func(gCtx context.Context) error {
			defer wg.Done()
			h, err := pool.Acquire(gCtx, workerID, time.Minute)
			assert.NoError(t, err)
			if h != nil {
				relErr := pool.Release(gCtx, h.ID)
				assert.NoError(t, relErr)
			}
			return nil
		})
	}
	wg.Wait()
	closeErr := pool.Close()
	require.NoError(t, closeErr)
}
