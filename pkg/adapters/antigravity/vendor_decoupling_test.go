package antigravity_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/adapters/antigravity"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Invariant 1: Static Floor Invariant
// LookPathAgentAPI cleanly returns empty string when neither PATH nor home directory binary exists.
func TestAntigravity_StaticFloorInvariant(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("PATH", t.TempDir()) // Isolated empty PATH

	p := antigravity.LookPathAgentAPI()
	assert.Empty(t, p, "must return empty string when binary does not exist")
}

// Invariant 2: Operational Dynamic Proof
// LookPathAgentAPI successfully locates the binary placed in the canonical vendor subpath.
func TestAntigravity_OperationalDynamicProof(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("PATH", t.TempDir())

	target := filepath.Join(tempHome, antigravity.DefaultAgentAPIBinarySubpath)
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(target), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), paths.FilePerm755))

	p := antigravity.LookPathAgentAPI()
	assert.Equal(t, target, p, "must discover binary at canonical vendor adapter subpath")
}

// Invariant 3: Negative Adversarial Boundary
// Directories matching the target binary name must be rejected (not treated as executable regular file).
func TestAntigravity_NegativeAdversarialBoundary(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("PATH", t.TempDir())

	target := filepath.Join(tempHome, antigravity.DefaultAgentAPIBinarySubpath)
	require.NoError(t, fileutil.MkdirAll(target, paths.DirPerm755)) // directory instead of file

	p := antigravity.LookPathAgentAPI()
	assert.Empty(t, p, "directory matching executable subpath must be rejected as regular file")
}
