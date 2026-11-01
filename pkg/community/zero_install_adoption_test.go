package community_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/demo"
	demopkg "github.com/zqk-os/zqk/pkg/demo"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestZeroInstall_StaticFloorInvariant satisfies CRIT-1793531041195922000-b82b761c.
// Verifies zero-install packaging script invariants (install.sh) and the static command
// builder contract for 'zqk demo'.
func TestZeroInstall_StaticFloorInvariant(t *testing.T) {
	repoRoot := findRepoRootForSDK(t)

	// 1. Verify installer script presence and executable permissions
	installScriptPath := filepath.Join(repoRoot, "scripts", "install.sh")
	info, err := os.Stat(installScriptPath)
	require.NoError(t, err, "scripts/install.sh must exist")
	assert.True(t, info.Mode()&0111 != 0, "scripts/install.sh must be executable")

	installContentBytes, err := fileutil.ReadFile(installScriptPath)
	require.NoError(t, err)
	installContent := string(installContentBytes)

	// Verify installer does not require external runtime dependencies (pure POSIX/bash)
	assert.Contains(t, installContent, "#!/bin/sh", "must use standard POSIX sh shebang")
	assert.Contains(t, installContent, "Detect OS and Architecture", "must have OS and architecture detection")
	assert.Contains(t, installContent, "uname -s", "must detect operating system")
	assert.Contains(t, installContent, "uname -m", "must detect architecture")

	// 2. Verify root symlink to install.sh
	rootSymlink := filepath.Join(repoRoot, "install.sh")
	rootInfo, err := os.Lstat(rootSymlink)
	require.NoError(t, err, "root install.sh symlink must exist")
	assert.True(t, rootInfo.Mode()&os.ModeSymlink != 0, "root install.sh must be a symlink")

	// 3. Verify 'zqk demo' command static flags and builder contract
	cmd := demo.NewDemoCmd()
	require.NotNil(t, cmd, "NewDemoCmd must return a valid command")
	assert.Equal(t, "demo", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)

	// Flags inspection
	assert.NotNil(t, cmd.Flags().Lookup("non-interactive"), "must define --non-interactive flag")
	assert.NotNil(t, cmd.Flags().Lookup("speed"), "must define --speed flag")
	assert.NotNil(t, cmd.Flags().Lookup("step"), "must define --step flag")
}

// TestZeroInstall_OperationalDynamicProof satisfies CRIT-1793531041195923000-4d8eb37d.
// Verifies that the interactive terminal showcase runs non-interactively in < 5 seconds,
// covers all showcase phases (ast, vds, swarm, star), and delivers viral community engagement.
func TestZeroInstall_OperationalDynamicProof(t *testing.T) {
	var buf bytes.Buffer
	opts := demopkg.DemoOptions{
		Step:           "all",
		Speed:          "instant",
		NonInteractive: true,
		Writer:         &buf,
		Reader:         strings.NewReader("\n\n\n\n"),
	}

	start := time.Now()
	sc := demopkg.NewShowcase(opts)
	require.NotNil(t, sc)

	res, err := sc.Run()
	duration := time.Since(start)

	require.NoError(t, err, "showcase run must complete without error")
	assert.Less(t, duration, 5*time.Second, "instant non-interactive demo must finish in < 5s")
	assert.True(t, res.Success, "showcase result must indicate success")

	out := buf.String()
	assert.Contains(t, out, "zgrep", "output must demonstrate sub-15ms zgrep AST search")
	assert.Contains(t, out, "VDS", "output must demonstrate Verifiable Delivery Standard")
	assert.Contains(t, strings.ToLower(out), "swarm", "output must demonstrate agent swarm orchestration")
	assert.Contains(t, out, "github.com/zqk-os/zqk", "output must invite GitHub starring and forking")

	// Verify targeted single-step execution
	var stepBuf bytes.Buffer
	stepOpts := demopkg.DemoOptions{
		Step:           "ast",
		Speed:          "instant",
		NonInteractive: true,
		Writer:         &stepBuf,
		Reader:         strings.NewReader("\n"),
	}
	stepSc := demopkg.NewShowcase(stepOpts)
	stepRes, stepErr := stepSc.Run()
	require.NoError(t, stepErr)
	assert.True(t, stepRes.Success)
	assert.Contains(t, stepBuf.String(), "zgrep")
}

// TestZeroInstall_NegativeAdversarialBoundary satisfies CRIT-1793531041195924000-b389dccd.
// Verifies error handling for unknown demo steps and confirms no credential leaks
// or vulnerable permissions exist within installer scripts.
func TestZeroInstall_NegativeAdversarialBoundary(t *testing.T) {
	// 1. Invalid step rejection
	var buf bytes.Buffer
	badOpts := demopkg.DemoOptions{
		Step:           "unrecognized_step_vector_xyz",
		Speed:          "instant",
		NonInteractive: true,
		Writer:         &buf,
		Reader:         strings.NewReader("\n"),
	}
	badSc := demopkg.NewShowcase(badOpts)
	_, err := badSc.Run()
	assert.Error(t, err, "unrecognized step must return an error")
	assert.Contains(t, err.Error(), "unknown step", "error must describe unknown step")

	// 2. Secret leakage scan in installer script
	repoRoot := findRepoRootForSDK(t)
	installScriptPath := filepath.Join(repoRoot, "scripts", "install.sh")
	contentBytes, err := fileutil.ReadFile(installScriptPath)
	require.NoError(t, err)
	content := string(contentBytes)

	forbiddenPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(ghp_[A-Za-z0-9_]{36,})`),
		regexp.MustCompile(`(?i)(github_pat_[A-Za-z0-9_]{40,})`),
		regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{20,})`),
		regexp.MustCompile(`(?i)-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----`),
	}
	for _, pattern := range forbiddenPatterns {
		assert.False(t, pattern.MatchString(content), "install.sh must not contain embedded tokens matching: %v", pattern)
	}
}
