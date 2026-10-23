package objects

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDeriveLifecycleTitle(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "Backlog Item Lifecycle", DeriveLifecycleTitle("backlog_item"))
	assert.Equal(t, "Base Object Lifecycle", DeriveLifecycleTitle("base_object"))
	assert.Equal(t, "Single Lifecycle", DeriveLifecycleTitle("single"))
}

func TestLoadLifecycleFile_Variants(t *testing.T) {
	tempDir := t.TempDir()
	validFile := filepath.Join(tempDir, "valid.yaml")
	require.NoError(t, fileutil.WriteStandardFile(validFile, []byte("object_type: valid_kind\nstatuses:\n  - value: active\n")))

	lc, err := LoadLifecycleFile(validFile)
	require.NoError(t, err)
	assert.Equal(t, "valid_kind", lc.ObjectType)

	invalidFile := filepath.Join(tempDir, "invalid.yaml")
	require.NoError(t, fileutil.WriteStandardFile(invalidFile, []byte("key: \"unclosed string\n")))
	_, err = LoadLifecycleFile(invalidFile)
	assert.Error(t, err)

	_, err = LoadLifecycleFile(filepath.Join(tempDir, "does_not_exist.yaml"))
	assert.Error(t, err)
}

func TestLifecycleLoader_TransitionsAndStatusQueries(t *testing.T) {
	loader := GetGlobalLifecycleLoader()
	require.NotNil(t, loader)
	loader.ClearCache()

	// Query known kind backlog_item
	origin, err := loader.GetOriginStatus(KindBacklogItem)
	require.NoError(t, err)
	assert.NotEmpty(t, origin)

	// IsTerminalStatusForKind
	isTerm, err := loader.IsTerminalStatusForKind(KindBacklogItem, "complete")
	require.NoError(t, err)
	assert.True(t, isTerm)

	isTerm, err = loader.IsTerminalStatusForKind(KindBacklogItem, "active")
	require.NoError(t, err)
	assert.False(t, isTerm)

	isTerm, err = loader.IsTerminalStatusForKind(KindBacklogItem, "")
	require.NoError(t, err)
	assert.False(t, isTerm)

	// IsPreliminaryStatusForKind
	isPrelim, err := loader.IsPreliminaryStatusForKind(KindBacklogItem, "")
	require.NoError(t, err)
	assert.False(t, isPrelim)

	// Preconditions and postconditions
	_, err = loader.GetTransition(KindBacklogItem, "nonexistent_a", "nonexistent_b")
	assert.Error(t, err)

	_, err = loader.GetTransitionPreconditions(KindBacklogItem, "nonexistent_a", "nonexistent_b")
	assert.Error(t, err)

	_, err = loader.GetTransitionPostconditions(KindBacklogItem, "nonexistent_a", "nonexistent_b")
	assert.Error(t, err)

	_, err = loader.GetStatusPreconditions(KindBacklogItem, "nonexistent_status_xyz")
	assert.Error(t, err)
}

func TestArchiveStatusForKind_Variants(t *testing.T) {
	var nilLoader *LifecycleLoader
	_, ok := nilLoader.ArchiveStatusForKind("backlog_item")
	assert.False(t, ok)

	loader := GetGlobalLifecycleLoader()
	_, ok = loader.ArchiveStatusForKind("")
	assert.False(t, ok)

	_, ok = loader.ArchiveStatusForKind("question")
	assert.False(t, ok)

	status, ok := ArchiveStatusForKind(KindBacklogItem)
	assert.True(t, ok)
	assert.NotEmpty(t, status)
}

func TestPrewarmAndDraftGenerators(t *testing.T) {
	// Prewarm empty should be no-op
	PrewarmGlobalsForProjectRoot("")

	// Draft generator error validations
	_, err := GenerateObjectSpecKindDraft(nil, "", "base_object")
	assert.Error(t, err)

	_, err = GenerateObjectSpecKindDraft(nil, "Invalid-Name", "base_object")
	assert.Error(t, err)

	_, err = GenerateObjectSpecKindDraft(nil, "valid_name", "Invalid-Parent")
	assert.Error(t, err)

	// Draft generator success
	draft, err := GenerateObjectSpecKindDraft(nil, "my_custom_kind", "base_object")
	require.NoError(t, err)
	assert.Contains(t, draft, "my_custom_kind")
	assert.Contains(t, draft, "base_object")
}
