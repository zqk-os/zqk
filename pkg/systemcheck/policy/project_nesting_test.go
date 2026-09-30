package policy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
)

func TestProjectNestingGate(t *testing.T) {
	gate := &ProjectNestingGate{}
	assert.Equal(t, "project-nesting", gate.Name())
	assert.NotEmpty(t, gate.Description())

	t.Run("clean root passes", func(t *testing.T) {
		tempDir := t.TempDir()
		zqkDir := filepath.Join(tempDir, paths.ProjectDataDir)
		require.NoError(t, os.MkdirAll(zqkDir, paths.DirPerm755))

		res, err := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir})
		require.NoError(t, err)
		assert.True(t, res.Passed)
		assert.Empty(t, res.Violations)
	})

	t.Run("nested root fails", func(t *testing.T) {
		tempDir := t.TempDir()
		zqkDir := filepath.Join(tempDir, paths.ProjectDataDir)
		require.NoError(t, os.MkdirAll(zqkDir, paths.DirPerm755))

		// Create rogue child .zqk
		childZqk := filepath.Join(tempDir, "subpkg", "nested", paths.ProjectDataDir)
		require.NoError(t, os.MkdirAll(childZqk, paths.DirPerm755))

		res, err := gate.Run(context.Background(), RunOptions{ProjectRoot: tempDir})
		require.NoError(t, err)
		assert.False(t, res.Passed)
		assert.NotEmpty(t, res.Violations)
	})

	t.Run("run via RunGates registry", func(t *testing.T) {
		tempDir := t.TempDir()
		zqkDir := filepath.Join(tempDir, paths.ProjectDataDir)
		require.NoError(t, os.MkdirAll(zqkDir, paths.DirPerm755))

		results, passed := RunGates(context.Background(), RunOptions{ProjectRoot: tempDir}, "project-nesting")
		assert.True(t, passed)
		require.Len(t, results, 1)
		assert.True(t, results[0].Passed)
	})
}
