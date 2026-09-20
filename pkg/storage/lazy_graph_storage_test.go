package storage

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestLazyGraphStorage(t *testing.T) {
	ctx := context.Background()
	tempDir, _, _ := setupTestingFactoryCompleteTestEnvironment(t)
	provider := &mockGraphProvider{enabled: true}
	l := NewLazyGraphStorage(provider, tempDir, true) // useMockGraph = true
	require.NotNil(t, l)

	err := l.ensureInitialized(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, l.real)

	secCtx := pkgctx.NewSecurityContext("system", []string{"admin"}, []string{"bypass_policy"})
	storageCtx := pkgctx.NewStorageContext()

	pool := l.GetPool()
	assert.NotNil(t, pool)

	// Unique ID: BLI-001 collides when graph/membranes share process state across tests.
	id := "BLI-lazy-" + strings.ReplaceAll(t.Name(), "/", "_")
	obj := map[string]any{objects.FieldKeyID: id, objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Test Title", objects.FieldKeyStatus: "roadmap", objects.FieldKeySchemaVersion: "1.0.0", objects.FieldKeyCategory: "development"}
	_ = l.Delete(WithTestHardDelete(ctx), secCtx, id, false) //nolint:errcheck // best-effort cleanup
	err = l.Create(ctx, secCtx, obj)
	assert.NoError(t, err)

	readObj, err := l.Read(ctx, secCtx, id)
	assert.NoError(t, err)
	assert.NotNil(t, readObj)

	err = l.Update(ctx, secCtx, id, map[string]any{objects.FieldKeyTitle: "updated title"})
	assert.NoError(t, err)

	err = l.Move(ctx, secCtx, id, "new_kind", false)
	assert.Error(t, err)

	err = l.Rename(ctx, secCtx, id, "new1", false)
	assert.Error(t, err)

	err = l.Delete(WithTestHardDelete(ctx), secCtx, id, false)
	assert.NoError(t, err)

	exists, err := l.Exists(ctx, secCtx, id)
	assert.NoError(t, err)
	assert.False(t, exists)

	count, err := l.Count(ctx, secCtx, ListFilter{})
	assert.NoError(t, err)
	assert.Equal(t, 0, count)

	err = l.Shutdown(ctx)
	assert.NoError(t, err)

	_, err = l.List(ctx, secCtx, storageCtx, ListFilter{})
	assert.NoError(t, err)

	_, err = l.Query(ctx, secCtx, storageCtx, Query{})
	assert.Error(t, err) // Cypher/Vector expected

	_, err = l.Aggregate(ctx, secCtx, storageCtx, ListFilter{}, nil)
	assert.Error(t, err) // at least one aggregation required

	_, err = l.Search(ctx, secCtx, storageCtx, SearchQuery{})
	assert.NoError(t, err)

	_, err = l.GetRelated(ctx, secCtx, id, "rel", 1)
	assert.Error(t, err)

	_, err = l.GetPath(ctx, secCtx, "BLI-003", "BLI-004")
	assert.NoError(t, err)

	_, err = l.GetNeighbors(ctx, secCtx, id, "out")
	assert.Error(t, err)

	_, err = l.BulkCreate(ctx, secCtx, []map[string]any{obj})
	assert.NoError(t, err)

	_, err = l.BulkUpdate(ctx, secCtx, []BulkUpdateItem{})
	assert.NoError(t, err)

	_, err = l.BulkGet(ctx, secCtx, []string{id})
	assert.NoError(t, err)

	_, err = l.BulkDelete(ctx, secCtx, []string{id}, false)
	assert.NoError(t, err)

	_, err = l.BeginTransaction(ctx)
	assert.NoError(t, err)
}

func TestLazyGraphStorage_NotAvailable(t *testing.T) {
	ctx := context.Background()
	// No provider, no socket, no mock -> should fail initialization
	l := NewLazyGraphStorage(nil, "/tmp/non/existent", false)

	err := l.ensureInitialized(ctx)
	assert.Error(t, err)

	secCtx := pkgctx.NewSecurityContext("system", []string{"admin"}, []string{"bypass_policy"})

	// Check if methods return ErrGraphNotAvailable or init err
	err = l.Create(ctx, secCtx, nil)
	assert.Error(t, err)
}
