package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/multimodal"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/swarm"
	"github.com/stretchr/testify/assert"
)

// MockStorage just for this test
type mockSwarmStorage struct {
	storage.ObjectStorageProvider
	mu   sync.RWMutex
	task map[string]any
}

func (m *mockSwarmStorage) List(ctx context.Context, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.task != nil && m.task[objects.FieldKeyStatus] == "proposed" {
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
		objects.FieldKeyStatus: objects.ObjectStatusProposed,
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
	SwarmNewLLMClient = func(c *llm.Config) llm.Client {
		return nil
	}
	defer func() { SwarmNewLLMClient = originalLLMClient }()

	getSwarmWorkerPool().Start(ctx)
	defer getSwarmWorkerPool().Stop()

	s.pollAndSpawnSwarmTasks(ctx)

	time.Sleep(100 * time.Millisecond)

	updatedTask, _ := mockStorage.Read(ctx, nil, "ATK-12345")
	assert.Equal(t, "error", updatedTask[objects.FieldKeyStatus])

	hookMu.Lock()
	capturedCtxCopy := capturedCtx
	hookMu.Unlock()

	if capturedCtxCopy == nil {
		t.Fatal("workerCtx should have been captured")
	}
	secCtx := pkgctx.GetSecurityContext(capturedCtxCopy)
	assert.NotNil(t, secCtx, "security context should be present")
	assert.Equal(t, "account:swarm_worker", secCtx.AccountID, "should have correct account ID")
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

func (m *mockLLMClient) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (multimodal.AnalysisResult, error) {
	return multimodal.AnalysisResult{}, nil
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

func (m *mockLLMClient) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (multimodal.AnalysisResult, error) {
	return multimodal.AnalysisResult{}, nil
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
		objects.FieldKeyStatus: objects.ObjectStatusProposed,
	}

	mockStorage := &mockSwarmStorage{
		task: task,
	}

	s := &Scheduler{
		storage: mockStorage,
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		secCtx:  pkgctx.NewSecurityContext("account:system", []string{"system"}, []string{"*"}),
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

	SwarmNewLLMClient = func(c *llm.Config) llm.Client {
		return llmClient
	}
	SwarmNewMCPExecutor = func(ctx context.Context, path string) (swarm.Executor, error) {
		return executor, nil
	}

	getSwarmWorkerPool().Start(ctx)
	defer getSwarmWorkerPool().Stop()

	s.pollAndSpawnSwarmTasks(ctx)

	// Wait for goroutine to finish
	time.Sleep(500 * time.Millisecond)

	updatedTask, _ := mockStorage.Read(ctx, nil, "ATK-LOOPTEST")
	assert.Equal(t, "implemented", updatedTask[objects.FieldKeyStatus])
	assert.Equal(t, 2, llmClient.getCallCount())
	executed := executor.getExecuted()
	assert.Equal(t, 1, len(executed))
	assert.Equal(t, "test_tool", executed[0])
}
