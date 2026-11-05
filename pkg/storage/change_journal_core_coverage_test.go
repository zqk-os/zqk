package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

func TestChangeJournalShutdownHandler_Contract(t *testing.T) {
	t.Parallel()

	handler := &ChangeJournalShutdownHandler{}

	assert.Nil(t, handler.InitiateShutdown())
	assert.Equal(t, "change_journal", handler.GetName())
	assert.True(t, handler.IsCritical())

	// When no writes are in-flight, IsDrained is true
	assert.True(t, handler.IsDrained())
	assert.GreaterOrEqual(t, handler.GetPendingCount(), int64(0))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	assert.NoError(t, handler.Drain(ctx))
}

func TestChangeJournal_DrainRootAndGlobal(t *testing.T) {
	t.Parallel()

	// Empty project root should immediately return nil
	assert.NoError(t, DrainChangeJournalForRoot(context.Background(), ""))

	// Valid project root with no pending writes returns nil
	tmpDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	assert.NoError(t, DrainChangeJournalForRoot(ctx, tmpDir))

	// Cancelled context returns error when tracker is blocked
	tracker := getChangeJournalTracker(tmpDir)
	tracker.wg.Add(1)
	tracker.pending.Add(1)

	cancelledCtx, cancelNow := context.WithCancel(context.Background())
	cancelNow()

	err := DrainChangeJournalForRoot(cancelledCtx, tmpDir)
	assert.ErrorIs(t, err, context.Canceled)

	// Clean up tracker so we do not leave pending count
	tracker.pending.Add(-1)
	tracker.wg.Done()
}

func TestChangeJournal_DrainChangeJournal_AllRoots(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	assert.NoError(t, DrainChangeJournal(ctx))
}

func TestChangeJournal_BuildDiffSummary_Variations(t *testing.T) {
	t.Parallel()

	// Empty updates
	assert.Equal(t, "No changes", buildDiffSummary(nil, nil))
	assert.Equal(t, "No changes", buildDiffSummary(map[string]any{}, map[string]any{}))

	// Only internal metadata fields
	metaOnly := map[string]any{
		"updated_at":          "2026-11-05T00:00:00Z",
		"updated_by":          "ACC-TEST",
		"expected_updated_at": "2026-11-05T00:00:00Z",
	}
	assert.Equal(t, ConstMiscMetadataUpdatedNoFieldChanges, buildDiffSummary(metaOnly, nil))

	// Added fields (oldValue == nil)
	addedUpdates := map[string]any{
		"status": "in_progress",
	}
	diffAdded := buildDiffSummary(addedUpdates, nil)
	assert.Contains(t, diffAdded, "status")
	assert.Contains(t, diffAdded, "in_progress")

	// Modified fields
	prevState := map[string]any{
		"status":   "planned",
		"priority": "P2",
	}
	modifiedUpdates := map[string]any{
		"status":   "in_progress",
		"priority": "P1",
	}
	diffModified := buildDiffSummary(modifiedUpdates, prevState)
	assert.Contains(t, diffModified, "status: planned -> in_progress")
	assert.Contains(t, diffModified, "priority: P2 -> P1")

	// Unchanged values don't produce diff entries
	sameUpdates := map[string]any{
		"status": "planned",
	}
	assert.Equal(t, ConstMiscMetadataUpdatedNoFieldChanges, buildDiffSummary(sameUpdates, prevState))
}

func TestChangeJournal_FlattenUpdatePaths_Deep(t *testing.T) {
	t.Parallel()

	// Empty
	assert.Nil(t, flattenUpdatePaths(nil))
	assert.Nil(t, flattenUpdatePaths(map[string]any{}))

	// Metadata excluded
	metaOnly := map[string]any{
		"updated_at":          "2026-11-05T00:00:00Z",
		"updated_by":          "ACC-TEST",
		"expected_updated_at": "2026-11-05T00:00:00Z",
	}
	assert.Empty(t, flattenUpdatePaths(metaOnly))

	// Nested map
	nested := map[string]any{
		"title": "New Title",
		"config": map[string]any{
			"retries": 3,
			"timeout": map[string]any{
				"duration": "10s",
			},
		},
		"tags": []string{"a", "b"},
	}

	paths := flattenUpdatePaths(nested)
	assert.Equal(t, []string{
		"config.retries",
		"config.timeout.duration",
		"tags",
		"title",
	}, paths)
}

func TestChangeJournal_CreateChangeJournalEntry_EdgeCases(t *testing.T) {
	t.Parallel()

	secCtx := pkgctx.NewSecurityContext("ACC-SYSTEM", []string{"admin"}, []string{"*"})

	// 1. Empty project root returns nil immediately
	err := createChangeJournalEntry(context.Background(), "", "OBJ-1", "backlog_item", "", OpCreate, nil, nil, secCtx, nil)
	assert.NoError(t, err)

	// 2. Kind is change_journal_entry returns nil (prevents infinite recursive journaling)
	err = createChangeJournalEntry(context.Background(), "/tmp/root", "CJE-1", objects.KindChangeJournalEntry, "", OpUpdate, nil, nil, secCtx, nil)
	assert.NoError(t, err)

	// 3. Kind is audit_event returns nil
	err = createChangeJournalEntry(context.Background(), "/tmp/root", "AUD-1", objects.KindAuditEvent, "", OpUpdate, nil, nil, secCtx, nil)
	assert.NoError(t, err)

	// 4. Kind is prompt_template triggers debug logging path without error
	err = createChangeJournalEntry(context.Background(), "", "PT-1", ConstMiscPromptTemplate, "", OpUpdate, nil, nil, secCtx, nil)
	assert.NoError(t, err)
}

func TestCASIndexInvalidationSubscriber_NegativeBoundaries(t *testing.T) {
	t.Parallel()

	// 1. Nil CAS instance returns nil safely
	nilSub := NewCASIndexInvalidationSubscriber(nil)
	assert.NoError(t, nilSub.HandleMutation(context.Background(), MutationEvent{Op: MutationOpDelete, ID: "ANY"}))

	// 2. Mock / real CAS with mismatched kind
	tmpDir := t.TempDir()
	mustEnsureProcessSpecsLayout(t, tmpDir)
	require.NoError(t, paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755))

	storage, err := NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Shutdown(context.Background()) })

	cas, err := storage.GetContentAddressableStorage("backlog_item")
	require.NoError(t, err)

	sub := NewCASIndexInvalidationSubscriber(cas)

	// Mismatched kind
	err = sub.HandleMutation(context.Background(), MutationEvent{
		Kind: "goal",
		ID:   "GOAL-1",
		Op:   MutationOpDelete,
	})
	assert.NoError(t, err)

	// Mismatched path prefix
	err = sub.HandleMutation(context.Background(), MutationEvent{
		Kind: "backlog_item",
		Path: "/completely/different/path/file.yaml",
		ID:   "BLI-PATH-MISMATCH",
		Op:   MutationOpDelete,
	})
	assert.NoError(t, err)

	// Put with invalid hash (does not match 64-char sha256 regex)
	err = sub.HandleMutation(context.Background(), MutationEvent{
		Kind:    "backlog_item",
		ID:      "BLI-INVALID-HASH",
		Op:      MutationOpPut,
		NewHash: "not-a-valid-sha256-hash",
	})
	assert.NoError(t, err)

	_, err = cas.GetHashForID("BLI-INVALID-HASH")
	assert.Error(t, err) // Should not have been indexed
}
