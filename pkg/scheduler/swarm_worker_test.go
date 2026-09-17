package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/agentfeed"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/swarm"
	"github.com/stretchr/testify/assert"
)

func TestOrphanRecoveryStatusPreservesDurableContinuation(t *testing.T) {
	t.Parallel()

	if got, recoverTask := orphanRecoveryStatus(objects.ObjectStatusInProgress); !recoverTask || got != objects.ObjectStatusApproved {
		t.Fatalf("in_progress recovery = (%q, %v), want (%q, true)", got, recoverTask, objects.ObjectStatusApproved)
	}
	if got, recoverTask := orphanRecoveryStatus(objects.ObjectStatusPendingVerification); recoverTask || got != "" {
		t.Fatalf("pending_verification recovery = (%q, %v), want unchanged", got, recoverTask)
	}
}

func TestResolveSwarmClaimant(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePeerSeatsForSwarm(t, root, map[string]agentfeed.PeerSeatRecord{
		"qwen-1": {Wake: agentfeed.WakeMembraneAgentAPI, PersonaRef: objects.ConstPersonaOrchestratorAlpha},
	})
	if got := resolveSwarmClaimant(root, map[string]any{
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
	}); got != "qwen-1" {
		t.Fatalf("persona seat = %q, want qwen-1", got)
	}
	if got := resolveSwarmClaimant(root, map[string]any{
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorBeta,
	}); got != "" {
		t.Fatalf("unseated persona must skip lease, got %q", got)
	}
	if got := resolveSwarmClaimant(root, map[string]any{}); got != swarmSchedulerClaimant {
		t.Fatalf("unassigned = %q, want %s", got, swarmSchedulerClaimant)
	}
}

func writePeerSeatsForSwarm(t *testing.T, root string, seats map[string]agentfeed.PeerSeatRecord) {
	t.Helper()
	if err := agentfeed.SavePeerSeats(root, agentfeed.PeerSeatsFile{SchemaVersion: "1", Seats: seats}); err != nil {
		t.Fatal(err)
	}
}

// MockStorage just for this test
type mockSwarmStorage struct {
	storage.ObjectStorageProvider
	mu   sync.RWMutex
	task map[string]any
}

func (m *mockSwarmStorage) List(ctx context.Context, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if filter.Kind != objects.KindAgentTask {
		return &storage.QueryResult{Objects: []map[string]any{}}, nil
	}
	if m.task != nil {
		if statusFilter, ok := filter.Filters[objects.FieldKeyStatus].(string); ok && statusFilter != "" {
			if m.task[objects.FieldKeyStatus] != statusFilter {
				return &storage.QueryResult{Objects: []map[string]any{}}, nil
			}
		}
		copied := make(map[string]any)
		for k, v := range m.task {
			copied[k] = v
		}
		return &storage.QueryResult{
			Objects: []map[string]any{copied},
		}, nil
	}
	return &storage.QueryResult{Objects: []map[string]any{}}, nil
}

func TestRecoverOrphanedTasks_UsesInProgressFilter(t *testing.T) {
	mockStore := &mockSwarmStorage{
		task: map[string]any{
			objects.FieldKeyID:        "ATK-IN-PROGRESS-1",
			objects.FieldKeyKind:      objects.KindAgentTask,
			objects.FieldKeyStatus:    objects.ObjectStatusInProgress,
			objects.FieldKeyClaimedBy: "some-agent",
		},
	}
	s := &Scheduler{
		storage: mockStore,
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
	s.recoverOrphanedTasks(context.Background())
	if mockStore.task[objects.FieldKeyStatus] != objects.ObjectStatusApproved {
		t.Fatalf("expected task status approved, got %v", mockStore.task[objects.FieldKeyStatus])
	}
	if mockStore.task[objects.FieldKeyClaimedBy] != storage.FieldUnset {
		t.Fatalf("expected claimed_by unset, got %v", mockStore.task[objects.FieldKeyClaimedBy])
	}
}

func (m *mockSwarmStorage) Update(ctx context.Context, secCtx *storage.SecurityContext, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.task != nil && m.task[objects.FieldKeyID] == id {
		for k, v := range updates {
			m.task[k] = v
		}
	}
	return nil
}

func (m *mockSwarmStorage) Read(ctx context.Context, secCtx *storage.SecurityContext, id string) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.task == nil {
		return nil, nil
	}
	copied := make(map[string]any)
	for k, v := range m.task {
		copied[k] = v
	}
	return copied, nil
}

func TestPollAndSpawnSwarmTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := map[string]any{
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyID:     "ATK-12345",
		objects.FieldKeyStatus: objects.ObjectStatusApproved,
	}

	mockStorage := &mockSwarmStorage{
		task: task,
	}

	s := &Scheduler{
		storage: mockStorage,
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}

	var hookMu sync.Mutex
	var capturedCtx context.Context
	TestSwarmWorkerHook = func(ctx context.Context) {
		hookMu.Lock()
		capturedCtx = ctx
		hookMu.Unlock()
	}
	defer func() { TestSwarmWorkerHook = nil }()
	swarmWorkerPool = nil
	swarmWorkerPoolOnce = sync.Once{}

	originalLLMClient := SwarmNewLLMClient
	SwarmNewLLMClient = func(ctx context.Context, c *llm.Config) llm.Client {
		return nil
	}
	defer func() { SwarmNewLLMClient = originalLLMClient }()

	getSwarmWorkerPool().Start(ctx)
	defer getSwarmWorkerPool().Stop()

	s.pollAndSpawnSwarmTasks(ctx)

	var updatedTask map[string]any
	deadline := time.After(2 * time.Second)
	for {
		updatedTask, _ = mockStorage.Read(ctx, nil, "ATK-12345")
		if updatedTask != nil && updatedTask[objects.FieldKeyStatus] == objects.ObjectStatusError {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for swarm task to reach error")
		case <-time.After(20 * time.Millisecond):
		}
	}
	assert.Equal(t, objects.ObjectStatusError, updatedTask[objects.FieldKeyStatus])
	assert.Equal(t, swarmSchedulerClaimant, updatedTask[objects.FieldKeyClaimedBy],
		"swarm lease must stamp claimed_by (WFL-MULTI-AGENT-WORK-CLAIM)")

	hookMu.Lock()
	capturedCtxCopy := capturedCtx
	hookMu.Unlock()

	if capturedCtxCopy == nil {
		t.Fatal("workerCtx should have been captured")
	}
	secCtx := pkgctx.GetSecurityContext(capturedCtxCopy)
	assert.NotNil(t, secCtx, "security context should be present")
	assert.Equal(t, "ACC-1785920548450214011-dabd3692", secCtx.AccountID, "should have correct account ID")
}

type mockLLMClient struct {
	mu        sync.Mutex
	responses []llm.StructuredCompletionResponse
	callCount int
	messages  []llm.Message
}

func (m *mockLLMClient) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func (m *mockLLMClient) GenerateIntent(ctx context.Context, code string) (string, error) {
	return "", nil
}

func (m *mockLLMClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}

func (m *mockLLMClient) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (llm.AnalysisResult, error) {
	return llm.AnalysisResult{}, nil
}

func (m *mockLLMClient) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	return "", nil
}

func (m *mockLLMClient) GenerateStructuredCompletion(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (llm.StructuredCompletionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = messages
	if m.callCount < len(m.responses) {
		resp := m.responses[m.callCount]
		m.callCount++
		return resp, nil
	}
	return llm.StructuredCompletionResponse{Content: "Default response"}, nil
}

func (m *mockLLMClient) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (llm.AnalysisResult, error) {
	return llm.AnalysisResult{}, nil
}
func (m *mockLLMClient) DescribeImage(ctx context.Context, image []byte) (string, error) {
	return "", nil
}
func (m *mockLLMClient) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	return "", nil
}
func (m *mockLLMClient) SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error) {
	return 0, nil
}

type mockMCPExecutor struct {
	mu       sync.Mutex
	executed []string
}

func (m *mockMCPExecutor) getExecuted() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.executed == nil {
		return nil
	}
	copied := make([]string, len(m.executed))
	copy(copied, m.executed)
	return copied
}

func (m *mockMCPExecutor) GetTools(ctx context.Context) ([]llm.ToolDefinition, error) {
	return []llm.ToolDefinition{{Name: "test_tool"}}, nil
}

func (m *mockMCPExecutor) ExecuteToolCall(ctx context.Context, call llm.ToolCall) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executed = append(m.executed, call.Name)
	return "Tool success", nil
}

func (m *mockMCPExecutor) Close() error {
	return nil
}

func TestSwarmWorkerRunLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	swarmWorkerPool = nil
	swarmWorkerPoolOnce = sync.Once{}

	task := map[string]any{
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyID:     "ATK-LOOPTEST",
		objects.FieldKeyStatus: objects.ObjectStatusApproved,
	}

	mockStorage := &mockSwarmStorage{
		task: task,
	}

	s := &Scheduler{
		storage: mockStorage,
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		secCtx:  pkgctx.NewSecurityContext("ACC-1785920548450214012-68b850c0", []string{"system"}, []string{"*"}),
	}

	llmClient := &mockLLMClient{
		responses: []llm.StructuredCompletionResponse{
			{
				Content: "Let me use a tool",
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "test_tool", Arguments: "{}"},
				},
			},
			{
				Content:   "Done!",
				ToolCalls: nil,
			},
		},
	}
	executor := &mockMCPExecutor{}

	// Override hooks
	oldLLMClient := SwarmNewLLMClient
	oldMCPExecutor := SwarmNewMCPExecutor
	defer func() {
		SwarmNewLLMClient = oldLLMClient
		SwarmNewMCPExecutor = oldMCPExecutor
	}()

	SwarmNewLLMClient = func(ctx context.Context, c *llm.Config) llm.Client {
		return llmClient
	}
	SwarmNewMCPExecutor = func(ctx context.Context, path string) (swarm.Executor, error) {
		return executor, nil
	}

	getSwarmWorkerPool().Start(ctx)
	defer getSwarmWorkerPool().Stop()

	s.pollAndSpawnSwarmTasks(ctx)

	var updatedTask map[string]any
	deadline := time.After(5 * time.Second)
	for {
		updatedTask, _ = mockStorage.Read(ctx, nil, "ATK-LOOPTEST")
		if updatedTask != nil && updatedTask[objects.FieldKeyStatus] == objects.ObjectStatusImplemented {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for swarm task to reach implemented")
		case <-time.After(20 * time.Millisecond):
		}
	}
	assert.Equal(t, objects.ObjectStatusImplemented, updatedTask[objects.FieldKeyStatus])
	assert.Equal(t, 2, llmClient.getCallCount())
	executed := executor.getExecuted()
	assert.Equal(t, 1, len(executed))
	assert.Equal(t, "test_tool", executed[0])
}

func TestApplyCodeDraftSampling(t *testing.T) {
	t.Parallel()
	coder := &llm.Config{ChatModel: "qwen2.5-coder:7b"}
	applyCodeDraftSampling(coder)
	if coder.Temperature == nil || *coder.Temperature != 0 {
		t.Fatalf("coder temp = %v, want 0", coder.Temperature)
	}
	doer := &llm.Config{ChatModel: "qwen3.6:latest"}
	applyCodeDraftSampling(doer)
	if doer.Temperature != nil {
		t.Fatalf("doer temp = %v, want unset", doer.Temperature)
	}
	preset := 0.7
	keep := &llm.Config{ChatModel: "qwen2.5-coder:7b", Temperature: &preset}
	applyCodeDraftSampling(keep)
	if keep.Temperature == nil || *keep.Temperature != 0.7 {
		t.Fatalf("preset temp overwritten: %v", keep.Temperature)
	}
}
