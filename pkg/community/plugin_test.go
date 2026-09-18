package community_test

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/community"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CRIT-1789723094574004000-4bd89527: Functional Acceptance
// Verifies plugin manifest creation, serialization, deserialization, and registry directory scanning.
func TestPluginManager_FunctionalAcceptance(t *testing.T) {
	tmpDir := t.TempDir()

	manifest := community.PluginManifest{
		Name:             "stats-exporter",
		Version:          "1.2.0",
		Description:      "Exports telemetry stats to local file",
		Author:           "Community Contributor",
		Entrypoint:       "bin/stats-exporter",
		MinKernelVersion: "2.0.0",
		Capabilities:     []string{"read_metrics", "export"},
	}

	manifestPath := filepath.Join(tmpDir, "stats-exporter.json")
	err := community.SaveManifestFile(manifestPath, manifest)
	require.NoError(t, err)

	loaded, err := community.LoadManifestFile(manifestPath)
	require.NoError(t, err)
	assert.Equal(t, "stats-exporter", loaded.Name)
	assert.Equal(t, "1.2.0", loaded.Version)
	assert.Equal(t, "bin/stats-exporter", loaded.Entrypoint)
	assert.Contains(t, loaded.Capabilities, "export")

	pm := community.NewPluginManager(tmpDir)
	count, err := pm.ScanPluginsDirectory(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	found, ok := pm.GetPlugin("stats-exporter")
	assert.True(t, ok)
	assert.Equal(t, "stats-exporter", found.Name)

	list := pm.ListPlugins()
	assert.Len(t, list, 1)
}

// CRIT-1789723094574005000-921cbcdc: Boundary & Error Handling
// Verifies validation rules: empty name, special chars in name, path traversal in entrypoint.
func TestPluginManager_BoundaryAndErrorHandling(t *testing.T) {
	t.Run("empty_name", func(t *testing.T) {
		err := community.ValidateManifest(community.PluginManifest{
			Name:       "   ",
			Version:    "1.0.0",
			Entrypoint: "bin/run",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "plugin name cannot be empty")
	})

	t.Run("invalid_chars_in_name", func(t *testing.T) {
		err := community.ValidateManifest(community.PluginManifest{
			Name:       "my plugin!@#",
			Version:    "1.0.0",
			Entrypoint: "bin/run",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must contain only letters, numbers, hyphens, and underscores")
	})

	t.Run("empty_version", func(t *testing.T) {
		err := community.ValidateManifest(community.PluginManifest{
			Name:       "good-name",
			Version:    "",
			Entrypoint: "bin/run",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "plugin version cannot be empty")
	})

	t.Run("empty_entrypoint", func(t *testing.T) {
		err := community.ValidateManifest(community.PluginManifest{
			Name:       "good-name",
			Version:    "1.0.0",
			Entrypoint: "",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "plugin entrypoint cannot be empty")
	})

	t.Run("path_traversal_entrypoint", func(t *testing.T) {
		err := community.ValidateManifest(community.PluginManifest{
			Name:       "exploit-plugin",
			Version:    "1.0.0",
			Entrypoint: "../../../bin/sh",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "path traversal")
	})

	t.Run("absolute_path_entrypoint", func(t *testing.T) {
		err := community.ValidateManifest(community.PluginManifest{
			Name:       "exploit-plugin",
			Version:    "1.0.0",
			Entrypoint: "/usr/local/bin/run",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "absolute path not allowed")
	})

	t.Run("missing_directory_scan", func(t *testing.T) {
		pm := community.NewPluginManager("/non/existent/path")
		_, err := pm.ScanPluginsDirectory("/non/existent/path")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})
}

// CRIT-1789723094574006000-8d14d8a0: Integration & Conformance
// Verifies SHA256 checksum verification and file integrity validation.
func TestPluginManager_IntegrationAndConformance(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "plugin_binary")
	binContent := []byte("binary executable mock content")

	require.NoError(t, fileutil.WriteStandardFile(binPath, binContent))

	sum := sha256.Sum256(binContent)
	expectedSHA := hex.EncodeToString(sum[:])

	// Match passes
	err := community.VerifyPluginChecksum(binPath, expectedSHA)
	require.NoError(t, err)

	// Mismatch fails
	err = community.VerifyPluginChecksum(binPath, "deadbeef1234567890")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")

	// Empty sha fails
	err = community.VerifyPluginChecksum(binPath, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}
