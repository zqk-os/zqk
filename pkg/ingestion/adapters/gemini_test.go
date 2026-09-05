package adapters

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/stretchr/testify/assert"

	"github.com/lanceman/zqk/pkg/logging"
)

func TestGeminiAdapter_Ingest(t *testing.T) {
	tempDir := t.TempDir()
	geminiDir := filepath.Join(tempDir, paths.ProjectDataDir)

	// Create test files
	assert.NoError(t, fileutil.Mkdir(geminiDir, 0755))
	assert.NoError(t, fileutil.WriteStandardFile(filepath.Join(geminiDir, "agent.json"), []byte("{}")))
	assert.NoError(t, fileutil.WriteStandardFile(filepath.Join(geminiDir, "config.yaml"), []byte("key: value")))
	assert.NoError(t, fileutil.WriteStandardFile(filepath.Join(geminiDir, "notes.txt"), []byte("some notes")))
	assert.NoError(t, fileutil.WriteStandardFile(filepath.Join(geminiDir, "ignore.bin"), []byte("binary data")))

	logger := logging.GetLoggerFromProfile("test")
	adapter := NewGeminiAdapter(logger)

	err := adapter.Ingest(context.Background(), geminiDir)
	assert.NoError(t, err)
}

func TestGeminiAdapter_Ingest_NoDir(t *testing.T) {
	tempDir := t.TempDir()
	geminiDir := filepath.Join(tempDir, paths.ProjectDataDir)

	logger := logging.GetLoggerFromProfile("test")
	adapter := NewGeminiAdapter(logger)

	err := adapter.Ingest(context.Background(), geminiDir)
	assert.NoError(t, err) // Should skip and not error
}
