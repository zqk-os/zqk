package object

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestInProcess_ObjectRootFields_Positive(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	// 1. Table format with --list-kinds
	out, err := executeObjectCommand(t, tempDir, provider, "fields", "--list-kinds")
	assert.NoError(t, err, "fields --list-kinds table output: %s", out)
	assert.Contains(t, out, "Kinds (discovery_lane=")

	// 2. JSON format with --list-kinds
	out, err = executeObjectCommand(t, tempDir, provider, "fields", "--list-kinds", "--format", "json")
	assert.NoError(t, err, "fields --list-kinds json output: %s", out)
	assert.Contains(t, out, `"membrane": "persona_rbac_discovery"`)

	// 3. YAML format with --list-kinds
	out, err = executeObjectCommand(t, tempDir, provider, "fields", "--list-kinds", "--format", "yaml")
	assert.NoError(t, err, "fields --list-kinds yaml output: %s", out)
	assert.Contains(t, out, "membrane: persona_rbac_discovery")

	// 4. With --all-kinds flag
	out, err = executeObjectCommand(t, tempDir, provider, "fields", "--list-kinds", "--all-kinds")
	assert.NoError(t, err, "fields --list-kinds --all-kinds output: %s", out)
	assert.Contains(t, out, "all_kinds=true")
}

func TestInProcess_ObjectRootFields_Negative(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	// Missing --list-kinds flag
	_, err = executeObjectCommand(t, tempDir, provider, "fields")
	assert.Error(t, err)
}

func TestDiscoveryMembrane_DirectHelpers(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	addAllKindsFlag(cmd)
	assert.NotNil(t, cmd.Flags().Lookup(FlagAllKinds))

	// Re-adding should be idempotent
	addAllKindsFlag(cmd)
	assert.False(t, allKindsRequested(cmd))

	err := cmd.Flags().Set(FlagAllKinds, "true")
	require.NoError(t, err)
	assert.True(t, allKindsRequested(cmd))

	// Inherited flag coverage
	parent := &cobra.Command{Use: "parent"}
	parent.PersistentFlags().Bool(FlagAllKinds, false, "")
	child := &cobra.Command{Use: "child"}
	parent.AddCommand(child)
	err = parent.PersistentFlags().Set(FlagAllKinds, "true")
	require.NoError(t, err)
	assert.True(t, allKindsRequested(child))

	// Nil safety
	assert.False(t, allKindsRequested(nil))
	addAllKindsFlag(nil)
}

func TestInProcess_ObjectRootFields_FallbackRegistry(t *testing.T) {
	tempDir := t.TempDir()
	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	out, err := executeObjectCommand(t, tempDir, provider, "fields", "--list-kinds")
	assert.NoError(t, err, "fields fallback registry output: %s", out)
	assert.Contains(t, out, "Kinds (discovery_lane=")
}
