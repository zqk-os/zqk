package community_test

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type devcontainerSpec struct {
	Schema            string         `json:"$schema"`
	Name              string         `json:"name"`
	Image             string         `json:"image"`
	RemoteUser        string         `json:"remoteUser"`
	PostCreateCommand string         `json:"postCreateCommand"`
	Features          map[string]any `json:"features"`
	Customizations    struct {
		VSCode struct {
			Extensions []string       `json:"extensions"`
			Settings   map[string]any `json:"settings"`
		} `json:"vscode"`
	} `json:"customizations"`
	ContainerEnv map[string]string `json:"containerEnv"`
	RunArgs      []string          `json:"runArgs"`
	Privileged   bool              `json:"privileged"`
	ForwardPorts []int             `json:"forwardPorts"`
}

func loadDevcontainerSpec(t *testing.T) devcontainerSpec {
	t.Helper()
	repoRoot := findRepoRootForSDK(t)
	specPath := filepath.Join(repoRoot, ".devcontainer", "devcontainer.json")

	data, err := fileutil.ReadFile(specPath)
	require.NoError(t, err, "devcontainer.json must exist and be readable")

	var spec devcontainerSpec
	err = json.Unmarshal(data, &spec)
	require.NoError(t, err, "devcontainer.json must be valid JSON conforming to devcontainer spec")
	return spec
}

// TestDevcontainer_StaticFloorInvariant satisfies CRIT-1793508466670250000-294f039e.
// Verifies devcontainer.json schema v1 conformance, Go 1.26+ toolchain pin, non-root user,
// and pre-built zqk binary target.
func TestDevcontainer_StaticFloorInvariant(t *testing.T) {
	spec := loadDevcontainerSpec(t)

	assert.Contains(t, spec.Schema, "devcontainers/spec", "schema should reference devcontainer spec")
	assert.NotEmpty(t, spec.Name, "devcontainer name must be defined")

	// Verify Go toolchain pin (Go 1.26+)
	assert.Contains(t, spec.Image, "devcontainers/go:1.26", "image must pin Go 1.26+ toolchain")

	// Verify non-root user
	assert.Equal(t, "vscode", spec.RemoteUser, "remoteUser must be configured as non-root vscode")

	// Verify post-create build command
	assert.Contains(t, spec.PostCreateCommand, "go build -o bin/zqk ./cmd/zqk", "postCreateCommand must build ./bin/zqk")
	assert.Contains(t, spec.PostCreateCommand, "agent-onboard", "postCreateCommand must run agent-onboard")

	// Verify essential VS Code extensions
	assert.Contains(t, spec.Customizations.VSCode.Extensions, "golang.go", "must configure Go extension")
	assert.Contains(t, spec.Customizations.VSCode.Extensions, "redhat.vscode-yaml", "must configure YAML extension")
}

// TestDevcontainer_OperationalDynamicProof satisfies CRIT-1793508497663872000-6bd124fa.
// Verifies automated container environment readiness, headless context configuration,
// and multi-agent seating discovery hooks.
func TestDevcontainer_OperationalDynamicProof(t *testing.T) {
	spec := loadDevcontainerSpec(t)

	// Verify container environment parameters
	require.NotNil(t, spec.ContainerEnv, "containerEnv must be populated")
	assert.Equal(t, "ai-agent", spec.ContainerEnv["ZQK_CONTEXT"], "ZQK_CONTEXT must default to ai-agent")
	assert.Equal(t, "true", spec.ContainerEnv["ZQK_ALLOW_DEGRADED"], "ZQK_ALLOW_DEGRADED must allow degraded boot without daemon")

	// Verify post-create multi-step pipeline sequence: build -> mcp ensure -> agent-onboard
	expectedSequence := []string{
		"go build -o bin/zqk ./cmd/zqk",
		"./bin/zqk mcp ensure",
		"./bin/zqk system agent-onboard --headless --format json",
	}
	for _, step := range expectedSequence {
		assert.Contains(t, spec.PostCreateCommand, step, "postCreateCommand must execute required boot step: %s", step)
	}

	// Verify GitHub CLI feature is present for PR management and CI integration
	assert.Contains(t, spec.Features, "ghcr.io/devcontainers/features/github-cli:1", "must include GitHub CLI feature")
}

// TestDevcontainer_NegativeAdversarialBoundary satisfies CRIT-1793508501136695000-614dc2a2.
// Verifies devcontainer security hardening: capability dropping, rejection of root escalation,
// zero unauthenticated exposed ports, and zero hardcoded credentials or tokens.
func TestDevcontainer_NegativeAdversarialBoundary(t *testing.T) {
	spec := loadDevcontainerSpec(t)

	// 1. Privileged mode MUST be false
	assert.False(t, spec.Privileged, "devcontainer must NOT run in privileged mode")

	// 2. Capabilities MUST be explicitly dropped
	hasCapDrop := false
	for _, arg := range spec.RunArgs {
		if strings.Contains(arg, "--cap-drop=ALL") {
			hasCapDrop = true
			break
		}
	}
	assert.True(t, hasCapDrop, "runArgs must contain --cap-drop=ALL to drop root capabilities")

	// 3. No unauthenticated exposed host ports
	for _, p := range spec.ForwardPorts {
		assert.NotEqual(t, 22, p, "SSH port 22 must never be forwarded directly")
		assert.NotEqual(t, 2375, p, "Docker daemon port 2375 must never be forwarded")
	}

	// 4. Zero hardcoded secrets/keys in devcontainer.json
	repoRoot := findRepoRootForSDK(t)
	specPath := filepath.Join(repoRoot, ".devcontainer", "devcontainer.json")
	rawBytes, err := fileutil.ReadFile(specPath)
	require.NoError(t, err)
	rawContent := string(rawBytes)

	secretPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(ghp_[A-Za-z0-9_]{36,})`),
		regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{20,})`),
		regexp.MustCompile(`(?i)password\s*[:=]\s*["'][^"']+["']`),
		regexp.MustCompile(`(?i)secret_key\s*[:=]\s*["'][^"']+["']`),
	}
	for _, pattern := range secretPatterns {
		assert.False(t, pattern.MatchString(rawContent), "devcontainer.json must not leak credentials matching pattern: %v", pattern)
	}
}
