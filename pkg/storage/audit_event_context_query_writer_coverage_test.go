package storage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/audit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestStorageExtended_AuditEventContext(t *testing.T) {
	assert.False(t, IsCreatingAuditEvent())

	done := BeginAuditEventCreation()
	assert.True(t, IsCreatingAuditEvent())

	done()
	assert.False(t, IsCreatingAuditEvent())
}

func TestStorageExtended_AuditObjectQuery(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	t.Run("auditStore_List", func(t *testing.T) {
		mock := &mockStorageWave8CustomErr{NoopObjectStorage: NoopObjectStorage{}}
		store := auditStore{p: mock}

		q := audit.ListQuery{
			Kind:  "audit_event",
			Limit: 10,
		}

		// Success
		objs, err := store.List(ctx, secCtx, nil, q)
		require.NoError(t, err)
		assert.Empty(t, objs)

		// Provider returns error
		mockErr := &mockStorageWave8CustomErr{
			NoopObjectStorage: NoopObjectStorage{},
			listErr:           errors.New("list failed"),
		}
		storeErr := auditStore{p: mockErr}
		_, err = storeErr.List(ctx, secCtx, nil, q)
		require.Error(t, err)
	})

	t.Run("auditStore_Create_and_Update", func(t *testing.T) {
		mock := &mockStorageWave12{}
		store := auditStore{p: mock}

		err := store.Create(ctx, secCtx, map[string]any{"id": "EVT-1"})
		require.NoError(t, err)
		assert.Len(t, mock.createCalls, 1)

		err = store.Update(ctx, secCtx, "EVT-1", map[string]any{"status": "archived"})
		require.NoError(t, err)
		assert.Len(t, mock.updateCalls, 1)
	})

	t.Run("auditStore_UpdateStatus", func(t *testing.T) {
		mock := &mockStorageWave8CustomErr{
			NoopObjectStorage: NoopObjectStorage{},
			bulkUpdateResult: &BulkResult{
				SuccessCount: 2,
				TotalCount:   2,
			},
		}
		store := auditStore{p: mock}

		count, err := store.UpdateStatus(ctx, secCtx, []string{"EVT-1", "EVT-2"}, "processed")
		require.NoError(t, err)
		assert.Equal(t, 2, count)

		// Provider error
		mock.bulkUpdateErr = errors.New("bulk update error")
		_, err = store.UpdateStatus(ctx, secCtx, []string{"EVT-1"}, "processed")
		require.Error(t, err)

		// Provider returns nil result
		mock.bulkUpdateErr = nil
		mock.bulkUpdateResult = nil
		count, err = store.UpdateStatus(ctx, secCtx, []string{"EVT-1"}, "processed")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("auditStore_DeleteIDs", func(t *testing.T) {
		// Generic ObjectStorageProvider
		mock := &mockStorageWave8CustomErr{
			NoopObjectStorage: NoopObjectStorage{},
			bulkDeleteResult: &BulkResult{
				SuccessCount: 3,
				FailureCount: 0,
			},
		}
		store := auditStore{p: mock}

		outcome, err := store.DeleteIDs(ctx, secCtx, []string{"E1", "E2", "E3"})
		require.NoError(t, err)
		assert.Equal(t, 3, outcome.SuccessCount)
		assert.False(t, outcome.Optimized)

		// Bulk delete error
		mock.bulkDeleteErr = errors.New("bulk delete failed")
		_, err = store.DeleteIDs(ctx, secCtx, []string{"E1"})
		require.Error(t, err)

		// FileObjectStorage
		testRoot, fos, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
		_ = testRoot
		storeFOS := auditStore{p: fos}
		cliCtx := WithCLIOperation(ctx)
		outcomeFOS, err := storeFOS.DeleteIDs(cliCtx, secCtx, []string{"nonexistent_1"})
		require.NoError(t, err)
		assert.True(t, outcomeFOS.Optimized)
	})

	t.Run("bulkDeleteOutcome_Helpers", func(t *testing.T) {
		// nil optimized
		outOptNil := bulkDeleteOutcomeFromOptimized(nil)
		assert.True(t, outOptNil.Optimized)

		// non-nil optimized with error struct
		outOpt := bulkDeleteOutcomeFromOptimized(&BulkDeleteResult{
			SuccessCount: 1,
			FailureCount: 1,
			Errors: []BulkOperationError{
				{ID: "E1", Error: errors.New("disk full")},
			},
		})
		assert.Equal(t, 1, outOpt.SuccessCount)
		assert.Equal(t, "disk full", outOpt.FirstMessage)

		// non-nil bulk with message only
		outBulkMsg := bulkDeleteOutcomeFromBulk(&BulkResult{
			SuccessCount: 2,
			FailureCount: 1,
			Errors: []BulkOperationError{
				{ID: "E2", Message: "timeout occurred"},
			},
		})
		assert.Equal(t, "timeout occurred", outBulkMsg.FirstMessage)

		// nil bulk
		outBulkNil := bulkDeleteOutcomeFromBulk(nil)
		assert.Equal(t, 0, outBulkNil.SuccessCount)
	})

	t.Run("AuditAggregationService_and_AuditMetricsCollector_SubStores", func(t *testing.T) {
		mock := &mockStorageWave12{}
		svc := &AuditAggregationService{storage: mock}

		assert.NotNil(t, svc.query())
		assert.NotNil(t, svc.metrics())
		assert.NotNil(t, svc.statuses())
		assert.NotNil(t, svc.deleter())

		collector := NewAuditMetricsCollector(mock)
		assert.NotNil(t, collector.metrics())
	})
}

type mockStorageWave8CustomErr struct {
	NoopObjectStorage
	listErr          error
	bulkUpdateResult *BulkResult
	bulkUpdateErr    error
	bulkDeleteResult *BulkResult
	bulkDeleteErr    error
}

func (m *mockStorageWave8CustomErr) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return &QueryResult{Objects: []map[string]any{}}, nil
}

func (m *mockStorageWave8CustomErr) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, items []BulkUpdateItem) (*BulkResult, error) {
	if m.bulkUpdateErr != nil {
		return nil, m.bulkUpdateErr
	}
	return m.bulkUpdateResult, nil
}

func (m *mockStorageWave8CustomErr) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	if m.bulkDeleteErr != nil {
		return nil, m.bulkDeleteErr
	}
	return m.bulkDeleteResult, nil
}

func TestStorageExtended_AuditWriter(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	testRoot, fos, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	opt := &audit.EventOptions{
		EventType:  EventTypeSystemConfigChange,
		Operation:  "write_event_test",
		TargetKind: "test",
		TargetID:   "T-1",
		Severity:   SeverityLow,
	}

	// With explicit project root
	err := fos.WriteEvent(ctx, testRoot, secCtx, opt)
	assert.NoError(t, err)

	// With empty project root (falls back to f.GetProjectRoot())
	err = fos.WriteEvent(ctx, "", secCtx, opt)
	assert.NoError(t, err)
}

func TestStorageExtended_AuditIDGenerator(t *testing.T) {
	ctx := context.Background()
	testRoot, fos, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	auditDir := filepath.Join(testRoot, paths.ProjectDataDir, "audit")
	require.NoError(t, fileutil.MkdirAll(auditDir, paths.DirPerm755))
	gen := GetAuditIDGenerator(ctx, auditDir, fos)
	require.NotNil(t, gen)

	// GenerateNextID
	id1, err := gen.GenerateNextID()
	require.NoError(t, err)
	assert.NotEmpty(t, id1)

	// GenerateBatchIDs
	ids, err := gen.GenerateBatchIDs(3)
	require.NoError(t, err)
	assert.Len(t, ids, 3)

	// GetLastSequence
	seq := gen.GetLastSequence()
	assert.GreaterOrEqual(t, seq, 0)

	// Reset
	gen.Reset()
	_ = testRoot
}

func TestStorageExtended_BucketingHelpers(t *testing.T) {
	t.Run("sanitizeCategory", func(t *testing.T) {
		assert.Equal(t, "user_profile", sanitizeCategory("user<profile>"))
		assert.Equal(t, "user_name", sanitizeCategory("  user:name  "))
		assert.Equal(t, "abc", sanitizeCategory("___abc___"))

		longStr := strings.Repeat("x", 150)
		sanitized := sanitizeCategory(longStr)
		assert.Len(t, sanitized, 100)
	})

	t.Run("countFilesInDirectory", func(t *testing.T) {
		tempDir := t.TempDir()

		// Non-existent directory
		_, err := countFilesInDirectory(filepath.Join(tempDir, "missing"))
		require.Error(t, err)

		// Create YAML, YML, TXT files and a subdirectory
		require.NoError(t, fileutil.WriteFile(filepath.Join(tempDir, "a.yaml"), []byte("a: 1"), paths.FilePerm644))
		require.NoError(t, fileutil.WriteFile(filepath.Join(tempDir, "b.yml"), []byte("b: 2"), paths.FilePerm644))
		require.NoError(t, fileutil.WriteFile(filepath.Join(tempDir, "c.txt"), []byte("txt"), paths.FilePerm644))
		require.NoError(t, fileutil.MkdirAll(filepath.Join(tempDir, "subdir"), paths.DirPerm755))

		count, err := countFilesInDirectory(tempDir)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})
}

func TestStorageExtended_AuditStreamMigrate(t *testing.T) {
	t.Run("copyFile", func(t *testing.T) {
		tempDir := t.TempDir()

		src := filepath.Join(tempDir, "src.txt")
		dst := filepath.Join(tempDir, "dst.txt")
		require.NoError(t, fileutil.WriteFile(src, []byte("hello stream"), paths.FilePerm644))

		err := copyFile(src, dst)
		require.NoError(t, err)

		content, err := fileutil.ReadFile(dst)
		require.NoError(t, err)
		assert.Equal(t, "hello stream", string(content))

		// Non-existent source
		err = copyFile(filepath.Join(tempDir, "missing.txt"), dst)
		require.Error(t, err)
	})

	t.Run("rewriteStreamRegistryPaths", func(t *testing.T) {
		tempDir := t.TempDir()

		// Non-existent registry file -> returns false, nil
		changed, err := rewriteStreamRegistryPaths(filepath.Join(tempDir, "missing.reg"), map[string]string{})
		require.NoError(t, err)
		assert.False(t, changed)

		// Create a sample registry file
		regPath := filepath.Join(tempDir, "registry.jsonl")
		lines := `{"id":"E1","loc":"/old/path/1.json::0"}
{"id":"E2","loc":"/old/path/2.json::10"}
{"id":"E3","loc":"/keep/path/3.json::20"}
`
		require.NoError(t, fileutil.WriteFile(regPath, []byte(lines), paths.FilePerm644))

		pathMap := map[string]string{
			"/old/path/1.json": "/new/path/1.json",
			"/old/path/2.json": "/new/path/2.json",
		}

		changed, err = rewriteStreamRegistryPaths(regPath, pathMap)
		require.NoError(t, err)
		assert.True(t, changed)

		// Read back
		newContent, err := fileutil.ReadFile(regPath)
		require.NoError(t, err)
		assert.Contains(t, string(newContent), "/new/path/1.json::0")
		assert.Contains(t, string(newContent), "/new/path/2.json::10")
		assert.Contains(t, string(newContent), "/keep/path/3.json::20")
		assert.NotContains(t, string(newContent), "/old/path/1.json")
	})

	t.Run("MigrateAuditStreamToCanonicalLocation", func(t *testing.T) {
		// Empty root
		moved, updated, err := MigrateAuditStreamToCanonicalLocation("")
		require.NoError(t, err)
		assert.Equal(t, 0, moved)
		assert.False(t, updated)

		tempDir := t.TempDir()

		// Non-existent legacy directory
		moved, updated, err = MigrateAuditStreamToCanonicalLocation(tempDir)
		require.NoError(t, err)
		assert.Equal(t, 0, moved)
		assert.False(t, updated)

		// Set up legacy directory with files
		legacyDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.AuditStreamsDir)
		require.NoError(t, fileutil.MkdirAll(legacyDir, paths.DirPerm755))

		// Add matching legacy file: audit_stream_2026-03-03.jsonl
		legFile1 := filepath.Join(legacyDir, "audit_stream_2026-03-03.jsonl")
		require.NoError(t, fileutil.WriteFile(legFile1, []byte(`{"id":"E1"}`), paths.FilePerm644))

		// Add non-matching file (subdir and wrong prefix)
		require.NoError(t, fileutil.Mkdir(filepath.Join(legacyDir, "some_dir"), paths.DirPerm755))
		require.NoError(t, fileutil.WriteFile(filepath.Join(legacyDir, "other.txt"), []byte("skip"), paths.FilePerm644))

		// Run migration
		moved, updated, err = MigrateAuditStreamToCanonicalLocation(tempDir)
		require.NoError(t, err)
		assert.Equal(t, 1, moved)

		// Verify file moved to canonical location
		canonicalDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.StreamsDir, objects.KindAuditEvent)
		canonicalFile := filepath.Join(canonicalDir, "2026-03-03_stream.json")
		assert.FileExists(t, canonicalFile)
	})
}
