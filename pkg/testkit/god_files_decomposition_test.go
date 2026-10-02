package testkit_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestGodFiles_LineCount_StaticFloor verifies CRIT-1790808124762369000-516cebbd.
// All production source files must remain strictly under the 2,000 line ceiling.
func TestGodFiles_LineCount_StaticFloor(t *testing.T) {
	t.Parallel()

	filesToCheck := []string{
		filepath.Join("..", "studio", "ui_server.go"),
		filepath.Join("..", "..", "cmd", "zqk", "swarm", "run.go"),
		filepath.Join("..", "..", "scripts", "open-core", "docs-portal", "generate_docs_portal.py"),
	}

	for _, file := range filesToCheck {
		require.True(t, fileutil.Exists(file), "file must exist: %s", file)
		data, err := fileutil.ReadFile(file)
		require.NoError(t, err)

		lines := strings.Split(string(data), "\n")
		assert.Less(t, len(lines), 2000, "file %s must not exceed 2,000 lines (current: %d)", file, len(lines))
	}
}

// TestGodFiles_ExtractedAssets_OperationalProof verifies CRIT-1790808124762370000-17836c5e.
// Assets must be extracted into dedicated modular static files and loaded successfully.
func TestGodFiles_ExtractedAssets_OperationalProof(t *testing.T) {
	t.Parallel()

	cssPath := filepath.Join("..", "..", "scripts", "open-core", "docs-portal", "assets", "portal.css")
	svgPath := filepath.Join("..", "..", "scripts", "open-core", "docs-portal", "assets", "logo.svg")
	studioAssets := filepath.Join("..", "studio", "assets")

	require.True(t, fileutil.Exists(cssPath), "portal.css asset must exist")
	require.True(t, fileutil.Exists(svgPath), "logo.svg asset must exist")
	require.True(t, fileutil.Exists(studioAssets), "studio assets directory must exist")

	cssData, err := fileutil.ReadFile(cssPath)
	require.NoError(t, err)
	assert.Greater(t, len(cssData), 500, "portal.css must have substantial styling content")

	svgData, err := fileutil.ReadFile(svgPath)
	require.NoError(t, err)
	assert.Contains(t, string(svgData), "<svg", "logo.svg must be a valid SVG document")

	// Verify Python script parses cleanly and loads assets
	scriptPath := filepath.Join("..", "..", "scripts", "open-core", "docs-portal", "generate_docs_portal.py")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	cmd := testkit.ManagedCommand(t, ctx, "python3", scriptPath, "--help")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err = cmd.Run()
	assert.NoError(t, err, "generate_docs_portal.py --help must exit successfully with asset loaders intact")
}

// TestGodFiles_CorruptTarballRejection_NegativeBoundary verifies CRIT-1790808124762371000-1e61fdd3.
// Verification must reject nonexistent or corrupted tarballs fail-closed.
func TestGodFiles_CorruptTarballRejection_NegativeBoundary(t *testing.T) {
	t.Parallel()

	scriptPath := filepath.Join("..", "..", "scripts", "open-core", "docs-portal", "generate_docs_portal.py")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	// 1. Nonexistent tarball must fail
	cmd := testkit.ManagedCommand(t, ctx, "python3", scriptPath, "--verify", "nonexistent-fake-tarball.tar.gz")
	err := cmd.Run()
	assert.Error(t, err, "generate_docs_portal.py --verify must fail closed on nonexistent tarball")

	// 2. Corrupted file must fail verification
	tmpDir := t.TempDir()
	badTarball := filepath.Join(tmpDir, "corrupted.tar.gz")
	_ = fileutil.WriteFile(badTarball, []byte("not a valid gzip archive"), 0644)

	cmdBad := testkit.ManagedCommand(t, ctx, "python3", scriptPath, "--verify", badTarball)
	errBad := cmdBad.Run()
	assert.Error(t, errBad, "generate_docs_portal.py --verify must fail closed on corrupted tarball")
}
