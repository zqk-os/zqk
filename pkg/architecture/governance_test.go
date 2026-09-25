package architecture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchitectureLayeringClassification(t *testing.T) {
	assert.Equal(t, LayerAppSurface, ClassifyLayer("cmd/zqk/system"))
	assert.Equal(t, LayerAppSurface, ClassifyLayer("pkg/mcp"))
	assert.Equal(t, LayerCoreEngine, ClassifyLayer("pkg/storage/wal"))
	assert.Equal(t, LayerCoreEngine, ClassifyLayer("pkg/scheduler"))
	assert.Equal(t, LayerFoundation, ClassifyLayer("pkg/concurrency"))
	assert.Equal(t, LayerFoundation, ClassifyLayer("pkg/objects"))
}

func TestFileComplexityBudget(t *testing.T) {
	tempDir := t.TempDir()
	normalFile := filepath.Join(tempDir, "normal.go")
	err := os.WriteFile(normalFile, []byte("package test\n\nfunc A() {}\n"), 0644)
	require.NoError(t, err)

	lines, err := ValidateFileComplexity(normalFile, 50)
	require.NoError(t, err)
	assert.Equal(t, 4, lines)

	// Exceeds limit
	_, err = ValidateFileComplexity(normalFile, 2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum line budget")
}

func TestParsePackageSummary(t *testing.T) {
	meta, err := ParsePackageSummary(".")
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.GreaterOrEqual(t, meta.FileCount, 1)
}
