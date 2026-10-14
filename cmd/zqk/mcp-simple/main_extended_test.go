package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

type mockServerRunner struct {
	serveCalled     bool
	serveErr        error
	serveMTLSCalled bool
	serveMTLSErr    error
	mtlsAddr        string
	mtlsCert        string
	mtlsKey         string
	mtlsCA          string
}

func (m *mockServerRunner) Serve() error {
	m.serveCalled = true
	return m.serveErr
}

func (m *mockServerRunner) ServeMTLS(addr, cert, key, ca string) error {
	m.serveMTLSCalled = true
	m.mtlsAddr = addr
	m.mtlsCert = cert
	m.mtlsKey = key
	m.mtlsCA = ca
	return m.serveMTLSErr
}

func TestParseServerConfig_Defaults(t *testing.T) {
	cfg, err := parseServerConfig([]string{})
	require.NoError(t, err)
	assert.Empty(t, cfg.MTLSAddr)
	assert.Empty(t, cfg.CertFile)
	assert.Empty(t, cfg.KeyFile)
	assert.Empty(t, cfg.CAFile)
}

func TestParseServerConfig_CompleteMTLS(t *testing.T) {
	args := []string{
		"-mtls-addr", ":8443",
		"-cert", "/path/to/cert.pem",
		"-key", "/path/to/key.pem",
		"-ca", "/path/to/ca.pem",
	}
	cfg, err := parseServerConfig(args)
	require.NoError(t, err)
	assert.Equal(t, ":8443", cfg.MTLSAddr)
	assert.Equal(t, "/path/to/cert.pem", cfg.CertFile)
	assert.Equal(t, "/path/to/key.pem", cfg.KeyFile)
	assert.Equal(t, "/path/to/ca.pem", cfg.CAFile)
}

func TestParseServerConfig_MissingMTLSFlags(t *testing.T) {
	testCases := []struct {
		name string
		args []string
	}{
		{
			name: "addr only",
			args: []string{"-mtls-addr", ":8443"},
		},
		{
			name: "missing ca",
			args: []string{"-mtls-addr", ":8443", "-cert", "c.pem", "-key", "k.pem"},
		},
		{
			name: "missing key",
			args: []string{"-mtls-addr", ":8443", "-cert", "c.pem", "-ca", "ca.pem"},
		},
		{
			name: "missing cert",
			args: []string{"-mtls-addr", ":8443", "-key", "k.pem", "-ca", "ca.pem"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseServerConfig(tc.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "mTLS requires -cert, -key, and -ca flags")
		})
	}
}

func TestParseServerConfig_InvalidFlag(t *testing.T) {
	_, err := parseServerConfig([]string{"-nonexistent-flag-xyz"})
	require.Error(t, err)
}

func TestBuildServer_StructureAndTools(t *testing.T) {
	tempRoot := t.TempDir()
	server := buildServer(tempRoot)
	require.NotNil(t, server)

	// Verify Security Context
	secCtxRaw := server.GetSecurityContext()
	require.NotNil(t, secCtxRaw)
	secCtx, ok := secCtxRaw.(*pkgctx.SecurityContext)
	require.True(t, ok)
	assert.Equal(t, pkgctx.SystemAccountID, secCtx.AccountID)

	// Verify Cli Initialization Context
	initCtx := server.GetCliInitializationContext()
	require.NotNil(t, initCtx)
	assert.Equal(t, tempRoot, initCtx.ProjectRoot)

	// Verify minimal tools are registered
	toolMap := make(map[string]bool)
	for _, tool := range server.ListTools() {
		toolMap[tool.Name] = true
	}

	expectedMinimalTools := []string{
		"zqk_test_echo",
		"zqk_graph_traversal",
		"zqk_object_list",
		"zqk_get_current_priority_plan",
		"zqk_create_object_interactive",
	}

	for _, toolName := range expectedMinimalTools {
		assert.True(t, toolMap[toolName], "expected tool %q to be registered", toolName)
	}

	// Verify onboarding prompts are registered
	prompts := server.ListPrompts()
	assert.NotEmpty(t, prompts, "expected onboarding prompts to be registered")
}

func TestRunWithServer_Stdio(t *testing.T) {
	runner := &mockServerRunner{}
	cfg := ServerConfig{}

	err := runWithServer(cfg, runner)
	require.NoError(t, err)
	assert.True(t, runner.serveCalled)
	assert.False(t, runner.serveMTLSCalled)

	// Test error propagation
	runnerErr := &mockServerRunner{serveErr: errors.New("stdio serve failed")}
	err = runWithServer(cfg, runnerErr)
	require.Error(t, err)
	assert.Equal(t, "stdio serve failed", err.Error())
}

func TestRunWithServer_MTLS(t *testing.T) {
	runner := &mockServerRunner{}
	cfg := ServerConfig{
		MTLSAddr: ":8443",
		CertFile: "cert.pem",
		KeyFile:  "key.pem",
		CAFile:   "ca.pem",
	}

	err := runWithServer(cfg, runner)
	require.NoError(t, err)
	assert.False(t, runner.serveCalled)
	assert.True(t, runner.serveMTLSCalled)
	assert.Equal(t, ":8443", runner.mtlsAddr)
	assert.Equal(t, "cert.pem", runner.mtlsCert)
	assert.Equal(t, "key.pem", runner.mtlsKey)
	assert.Equal(t, "ca.pem", runner.mtlsCA)

	// Test error propagation
	runnerErr := &mockServerRunner{serveMTLSErr: errors.New("mtls serve failed")}
	err = runWithServer(cfg, runnerErr)
	require.Error(t, err)
	assert.Equal(t, "mtls serve failed", err.Error())
}

func TestRun_ErrorBranches(t *testing.T) {
	// Missing mTLS flags
	err := run([]string{"-mtls-addr", ":8443"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mTLS requires -cert, -key, and -ca flags")

	// Unknown flag
	err = run([]string{"-unknown-flag-12345"})
	require.Error(t, err)
}
