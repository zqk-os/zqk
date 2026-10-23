package main

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestRun_SuccessStdio(t *testing.T) {
	origFactory := serverFactory
	defer func() { serverFactory = origFactory }()

	mockRunner := &mockServerRunner{}
	serverFactory = func(projectRoot string) serverRunner {
		return mockRunner
	}

	err := run([]string{})
	require.NoError(t, err)
	assert.True(t, mockRunner.serveCalled)
	assert.False(t, mockRunner.serveMTLSCalled)
}

func TestRun_SuccessMTLS(t *testing.T) {
	origFactory := serverFactory
	defer func() { serverFactory = origFactory }()

	mockRunner := &mockServerRunner{}
	serverFactory = func(projectRoot string) serverRunner {
		return mockRunner
	}

	args := []string{
		"-mtls-addr", ":8443",
		"-cert", "/path/to/cert.pem",
		"-key", "/path/to/key.pem",
		"-ca", "/path/to/ca.pem",
	}
	err := run(args)
	require.NoError(t, err)
	assert.False(t, mockRunner.serveCalled)
	assert.True(t, mockRunner.serveMTLSCalled)
	assert.Equal(t, ":8443", mockRunner.mtlsAddr)
}

func TestRun_ServerError(t *testing.T) {
	origFactory := serverFactory
	defer func() { serverFactory = origFactory }()

	mockRunner := &mockServerRunner{serveErr: errors.New("simulated serve failure")}
	serverFactory = func(projectRoot string) serverRunner {
		return mockRunner
	}

	err := run([]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated serve failure")
}

func TestRun_DefaultFactory(t *testing.T) {
	tempDir := t.TempDir()
	runner := serverFactory(tempDir)
	require.NotNil(t, runner)
	_, ok := runner.(*mcp.Server)
	assert.True(t, ok)
}

func TestMain_Success(t *testing.T) {
	origFactory := serverFactory
	origArgs := os.Args
	origExit := osExit
	defer func() {
		serverFactory = origFactory
		os.Args = origArgs
		osExit = origExit
	}()

	mockRunner := &mockServerRunner{}
	serverFactory = func(projectRoot string) serverRunner {
		return mockRunner
	}

	exitCalled := false
	osExit = func(code int) {
		exitCalled = true
	}

	os.Args = []string{"mcp-simple"}
	main()

	assert.False(t, exitCalled)
	assert.True(t, mockRunner.serveCalled)
}

func TestMain_ErrorExit_RunError(t *testing.T) {
	origFactory := serverFactory
	origArgs := os.Args
	origExit := osExit
	defer func() {
		serverFactory = origFactory
		os.Args = origArgs
		osExit = origExit
	}()

	mockRunner := &mockServerRunner{serveErr: errors.New("fatal server failure")}
	serverFactory = func(projectRoot string) serverRunner {
		return mockRunner
	}

	var exitCode int
	exitCalled := false
	osExit = func(code int) {
		exitCalled = true
		exitCode = code
	}

	os.Args = []string{"mcp-simple"}
	main()

	assert.True(t, exitCalled)
	assert.Equal(t, 1, exitCode)
}

func TestMain_ErrorExit_FlagParseError(t *testing.T) {
	origArgs := os.Args
	origExit := osExit
	defer func() {
		os.Args = origArgs
		osExit = origExit
	}()

	var exitCode int
	exitCalled := false
	osExit = func(code int) {
		exitCalled = true
		exitCode = code
	}

	os.Args = []string{"mcp-simple", "-unknown-unsupported-flag"}
	main()

	assert.True(t, exitCalled)
	assert.Equal(t, 1, exitCode)
}

func TestMinimalTools_DetailedSchemaVerification(t *testing.T) {
	server := mcp.NewServer()
	registerMinimalTools(server)

	toolMap := make(map[string]mcp.Tool)
	for _, tool := range server.ListTools() {
		toolMap[tool.Name] = tool
	}

	toolNames := []string{
		"zqk_test_echo",
		"zqk_graph_traversal",
		"zqk_object_list",
		"zqk_get_current_priority_plan",
		"zqk_create_object_interactive",
	}

	for _, name := range toolNames {
		tool, exists := toolMap[name]
		require.True(t, exists, "tool %s should exist", name)
		assert.Equal(t, name, tool.Name)
		assert.NotEmpty(t, tool.Description)
		require.NotNil(t, tool.InputSchema)
		schemaMap, ok := tool.InputSchema.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "object", schemaMap[objects.FieldKeyType])
	}
}

func TestBuildServer_ExecutionToolsAndResources(t *testing.T) {
	tempRoot := t.TempDir()
	server := buildServer(tempRoot)
	require.NotNil(t, server)

	// Agent execution tools (e.g. execute_bash, read_file)
	toolMap := make(map[string]mcp.Tool)
	for _, tool := range server.ListTools() {
		toolMap[tool.Name] = tool
	}

	bashToolName := mcp.GetToolName("execute_bash")
	tool, exists := toolMap[bashToolName]
	assert.True(t, exists, "tool %s should be registered", bashToolName)
	if exists {
		assert.NotEmpty(t, tool.Description)
	}

	readFileToolName := mcp.GetToolName("read_file")
	_, exists = toolMap[readFileToolName]
	assert.True(t, exists, "tool %s should be registered", readFileToolName)

	// Onboarding prompts
	prompts := server.ListPrompts()
	assert.NotEmpty(t, prompts, "prompts should be registered")
}
