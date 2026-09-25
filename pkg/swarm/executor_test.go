package swarm

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/llm"
)

// testExecutorImpl implements Executor for behavioral verification.
type testExecutorImpl struct {
	tools       []llm.ToolDefinition
	executeFunc func(ctx context.Context, call llm.ToolCall) (string, error)
	closed      bool
}

func (m *testExecutorImpl) GetTools(ctx context.Context) ([]llm.ToolDefinition, error) {
	if m.closed {
		return nil, errors.New("executor is closed")
	}
	return m.tools, nil
}

func (m *testExecutorImpl) ExecuteToolCall(ctx context.Context, call llm.ToolCall) (string, error) {
	if m.closed {
		return "", errors.New("executor is closed")
	}
	if m.executeFunc != nil {
		return m.executeFunc(ctx, call)
	}
	return "mock-result", nil
}

func (m *testExecutorImpl) Close() error {
	m.closed = true
	return nil
}

func TestExecutorInterface(t *testing.T) {
	ctx := context.Background()

	mockTool := llm.ToolDefinition{
		Name:        "search_code",
		Description: "Search code in repository",
	}

	exec := &testExecutorImpl{
		tools: []llm.ToolDefinition{mockTool},
		executeFunc: func(ctx context.Context, call llm.ToolCall) (string, error) {
			if call.Name == "search_code" {
				return "found 3 occurrences", nil
			}
			return "", errors.New("unknown tool")
		},
	}

	// Verify Executor interface compliance
	var _ Executor = exec

	// 1. Tool listing
	tools, err := exec.GetTools(ctx)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "search_code", tools[0].Name)

	// 2. Successful tool execution
	res, err := exec.ExecuteToolCall(ctx, llm.ToolCall{
		Name:      "search_code",
		Arguments: `{"query": "NewMeshCmd"}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "found 3 occurrences", res)

	// 3. Failed tool execution (unknown tool)
	_, err = exec.ExecuteToolCall(ctx, llm.ToolCall{
		Name: "unregistered_tool",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool")

	// 4. Close cleanup lifecycle
	err = exec.Close()
	require.NoError(t, err)

	_, err = exec.GetTools(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "executor is closed")

	_, err = exec.ExecuteToolCall(ctx, llm.ToolCall{Name: "search_code"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "executor is closed")
}
