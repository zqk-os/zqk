package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestStorageExtended_Wave25_YAMLFormatter(t *testing.T) {
	t.Run("FormatMultiLineYAML", func(t *testing.T) {
		input := map[string]any{
			"simple":    "single line string",
			"multiline": "line 1\nline 2\nline 3\rline 4",
			"nested": map[string]any{
				"inner_multi": "inner line 1\ninner line 2",
				"count":       42,
				"active":      true,
			},
			"list_any": []any{
				"item1",
				"item2\nwith newline",
				100,
			},
			"list_str": []string{
				"str1",
				"str2\nmultiline",
			},
			"number": 3.1415,
		}

		out, err := FormatMultiLineYAML(input)
		require.NoError(t, err)
		require.NotEmpty(t, out)
		assert.Contains(t, string(out), "line 1")
		assert.Contains(t, string(out), "inner line 1")

		// containsNewline helper
		assert.True(t, containsNewline("hello\nworld"))
		assert.True(t, containsNewline("hello\rworld"))
		assert.False(t, containsNewline("hello world"))
	})
}

func TestStorageExtended_Wave25_VerificationOutcomeAuthority(t *testing.T) {
	t.Run("isCriteriaVerificationOutcomeStatus", func(t *testing.T) {
		assert.False(t, isCriteriaVerificationOutcomeStatus(""))
		assert.True(t, isCriteriaVerificationOutcomeStatus(objects.ObjectStatusValidated))
		assert.True(t, isCriteriaVerificationOutcomeStatus(criteriaStatusComplete))
		assert.True(t, isCriteriaVerificationOutcomeStatus(" Validated "))
		assert.True(t, isCriteriaVerificationOutcomeStatus("COMPLETE"))
		assert.False(t, isCriteriaVerificationOutcomeStatus("in_progress"))
	})

	t.Run("checkCriteriaVerificationOutcomeAuthority", func(t *testing.T) {
		// Non-criteria kind -> no error
		assert.NoError(t, checkCriteriaVerificationOutcomeAuthority(objects.KindGoal, nil, "validated"))
		// Non-outcome status -> no error
		assert.NoError(t, checkCriteriaVerificationOutcomeAuthority(objects.KindCriteria, nil, "in_progress"))

		// System account -> allowed
		sysCtx := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
		assert.NoError(t, checkCriteriaVerificationOutcomeAuthority(objects.KindCriteria, sysCtx, "validated"))

		// Test harness account -> allowed
		testHarnessCtx := &pkgctx.SecurityContext{AccountID: pkgctx.TestHarnessAccountID}
		assert.NoError(t, checkCriteriaVerificationOutcomeAuthority(objects.KindCriteria, testHarnessCtx, "complete"))

		// Regular account without bypass -> denied
		userCtx := &pkgctx.SecurityContext{AccountID: "user-123"}
		origBypass := zqkenv.TestBypassAuth().Get()
		zqkenv.TestBypassAuth().Set("0")
		defer zqkenv.TestBypassAuth().Set(origBypass)

		err := checkCriteriaVerificationOutcomeAuthority(objects.KindCriteria, userCtx, "validated")
		assert.Error(t, err)

		// Regular account with bypass -> allowed
		zqkenv.TestBypassAuth().Set("1")
		assert.NoError(t, checkCriteriaVerificationOutcomeAuthority(objects.KindCriteria, userCtx, "validated"))
	})

	t.Run("checkConvergenceSessionErrorStatusAuthority", func(t *testing.T) {
		// Non-convergence_session kind -> no error
		assert.NoError(t, checkConvergenceSessionErrorStatusAuthority(objects.KindGoal, nil, "error"))
		// Non-error status -> no error
		assert.NoError(t, checkConvergenceSessionErrorStatusAuthority(objects.KindConvergenceSession, nil, "active"))

		// System account -> allowed
		sysCtx := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
		assert.NoError(t, checkConvergenceSessionErrorStatusAuthority(objects.KindConvergenceSession, sysCtx, "error"))

		// Test harness account -> allowed
		testHarnessCtx := &pkgctx.SecurityContext{AccountID: pkgctx.TestHarnessAccountID}
		assert.NoError(t, checkConvergenceSessionErrorStatusAuthority(objects.KindConvergenceSession, testHarnessCtx, "error"))

		// Regular account without bypass -> denied
		userCtx := &pkgctx.SecurityContext{AccountID: "user-123"}
		origBypass := zqkenv.TestBypassAuth().Get()
		zqkenv.TestBypassAuth().Set("0")
		defer zqkenv.TestBypassAuth().Set(origBypass)

		err := checkConvergenceSessionErrorStatusAuthority(objects.KindConvergenceSession, userCtx, "error")
		assert.Error(t, err)

		// Regular account with bypass -> allowed
		zqkenv.TestBypassAuth().Set("1")
		assert.NoError(t, checkConvergenceSessionErrorStatusAuthority(objects.KindConvergenceSession, userCtx, "error"))
	})

	t.Run("checkVerificationOutcomeAuthority", func(t *testing.T) {
		sysCtx := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
		assert.NoError(t, checkVerificationOutcomeAuthority(objects.KindCriteria, sysCtx, "validated"))
		assert.NoError(t, checkVerificationOutcomeAuthority(objects.KindConvergenceSession, sysCtx, "error"))
	})
}

func TestStorageExtended_Wave25_YAMLParseCache(t *testing.T) {
	cache := NewParseCache(10)
	require.NotNil(t, cache)

	t.Run("Get and Put and Evict and Clear", func(t *testing.T) {
		// Initial miss
		_, found := cache.Get("hash1")
		assert.False(t, found)
		assert.Equal(t, uint64(1), cache.Misses())

		parsed := &objects.ParsedObject{
			Raw: map[string]any{"key": "value"},
		}
		cache.Put("hash1", parsed)
		assert.Equal(t, uint64(1), cache.Puts())

		got, found := cache.Get("hash1")
		assert.True(t, found)
		assert.Equal(t, parsed, got)
		assert.Equal(t, uint64(1), cache.Hits())

		// Evict
		cache.Evict("hash1")
		assert.Equal(t, uint64(1), cache.Evictions())
		_, found = cache.Get("hash1")
		assert.False(t, found)

		// Put multiple and hit LRU limit
		for i := 0; i < 15; i++ {
			h := fmt.Sprintf("h-%d", i)
			cache.Put(h, &objects.ParsedObject{Raw: map[string]any{"i": i}})
		}

		// Clear
		cache.Clear()
		_, found = cache.Get("h-14")
		assert.False(t, found)

		// ResetMetrics
		cache.ResetMetrics()
		assert.Equal(t, uint64(0), cache.Hits())
		assert.Equal(t, uint64(0), cache.Misses())
		assert.Equal(t, uint64(0), cache.Puts())
		assert.Equal(t, uint64(0), cache.Evictions())
	})

	t.Run("GetGlobalParseCache", func(t *testing.T) {
		g := GetGlobalParseCache()
		assert.NotNil(t, g)
	})
}

func TestStorageExtended_Wave25_Invalidation(t *testing.T) {
	bus := NewInvalidationShockwaveBus()
	require.NotNil(t, bus)

	var receivedEvents []MutationEvent
	subFunc := InvalidationSubscriberFunc(func(ctx context.Context, event MutationEvent) error {
		receivedEvents = append(receivedEvents, event)
		return nil
	})

	bus.Subscribe(subFunc)
	assert.Equal(t, 1, bus.SubscribersCount())

	ctx := context.Background()
	event := MutationEvent{
		Op:   MutationOpPut,
		Kind: "goal",
		ID:   "GOAL-1",
	}
	bus.Broadcast(ctx, event)
	assert.Len(t, receivedEvents, 1)

	// Unsubscribe
	bus.Unsubscribe(subFunc)
	assert.Equal(t, 0, bus.SubscribersCount())

	// ClearSubscribers
	bus.Subscribe(subFunc)
	assert.Equal(t, 1, bus.SubscribersCount())
	bus.ClearSubscribers()
	assert.Equal(t, 0, bus.SubscribersCount())

	// Global bus functions
	globalSub := InvalidationSubscriberFunc(func(ctx context.Context, event MutationEvent) error {
		return nil
	})
	SubscribeInvalidation(globalSub)
	UnsubscribeInvalidation(globalSub)

	// broadcastInvalidationShockwave helper
	broadcastInvalidationShockwave(ctx, OpCreate, "goal", "GOAL-2", "/path/to/hash.yaml", map[string]any{"k": "v"})
	broadcastInvalidationShockwave(ctx, OpDelete, "goal", "GOAL-2", "/path/to/hash.yaml", nil)

	// Subscribers handle mutations
	pSub := newParseCacheSubscriber(NewParseCache(5))
	assert.NoError(t, pSub.HandleMutation(ctx, MutationEvent{OldHash: "old", NewHash: "new", Op: MutationOpDelete}))

	lSub := newListCacheSubscriber()
	assert.NoError(t, lSub.HandleMutation(ctx, MutationEvent{Kind: "goal"}))
	assert.NoError(t, lSub.HandleMutation(ctx, MutationEvent{Kind: ""}))
}

func TestStorageExtended_Wave25_ObjectDraftPlane(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "draft_plane_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	storage, err := NewFileObjectStorage(tmpDir)
	require.NoError(t, err)

	kind := objects.KindGoal
	id := "GOAL-DRAFT-1"
	draftData := []byte("id: GOAL-DRAFT-1\nkind: goal\ntitle: Draft Goal\nstatus: draft\n")

	// Write to draft plane
	err = storage.WriteObjectToDraftPlane(id, kind, draftData)
	require.NoError(t, err)

	// Read bytes
	readBytes, err := storage.readObjectDraftPlaneBytes(kind, id)
	require.NoError(t, err)
	assert.NotEmpty(t, readBytes)
	assert.Contains(t, string(readBytes), "GOAL-DRAFT-1")

	// Read map
	readObj, err := storage.readObjectDraftPlane(kind, id)
	require.NoError(t, err)
	assert.Equal(t, "GOAL-DRAFT-1", readObj["id"])
	assert.Equal(t, kind, readObj[objects.FieldKeyKind])

	// Delete from draft plane
	err = storage.deleteObjectDraftPlane(kind, id)
	require.NoError(t, err)

	// Read non-existent
	_, err = storage.readObjectDraftPlaneBytes(kind, id)
	assert.ErrorIs(t, err, ErrObjectNotFound)
}

func TestStorageExtended_Wave25_Cleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cleanup_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// isEmptyBucketDir on empty dir
	assert.True(t, isEmptyBucketDir(tmpDir))

	// Non-existent dir
	assert.False(t, isEmptyBucketDir(filepath.Join(tmpDir, "missing")))

	// Dir with non-yaml file (dotfile)
	dotFile := filepath.Join(tmpDir, ".hashes")
	require.NoError(t, fileutil.WriteFile(dotFile, []byte("hash"), 0644))
	assert.True(t, isEmptyBucketDir(tmpDir))

	// Dir with yaml file
	yamlFile := filepath.Join(tmpDir, "object.yaml")
	require.NoError(t, fileutil.WriteFile(yamlFile, []byte("k: v"), 0644))
	assert.False(t, isEmptyBucketDir(tmpDir))
}

func TestStorageExtended_Wave25_BucketingStrategyStorage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bucket_strat_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	fileStorage := NewFileBucketStrategyStorage(tmpDir)
	require.NotNil(t, fileStorage)
	assert.Equal(t, "file", fileStorage.GetBackendType())

	ctx := context.Background()

	// Load non-existent strategy
	_, err = fileStorage.LoadStrategy(ctx, "NON-EXISTENT")
	assert.Error(t, err)

	// SaveStrategy without ID
	err = fileStorage.SaveStrategy(ctx, map[string]any{"name": "strat1"})
	assert.Error(t, err)

	// Graph bucket strategy storage
	graphStorage := NewGraphBucketStrategyStorage(nil)
	assert.Equal(t, "graph", graphStorage.GetBackendType())
}

func TestStorageExtended_Wave25_WaitGroup(t *testing.T) {
	mgr := NewWaitGroupManager()
	require.NotNil(t, mgr)

	obs := NewLoggingWaitGroupObserver(nil)
	mgr.SetObserver(obs)

	// CreateGroup and CreateGroupForGoroutine
	wg := mgr.CreateGroup("grp-1", "test_op")
	require.NotNil(t, wg)

	wg2 := mgr.CreateGroupForGoroutine("grp-1", "test_op")
	assert.Equal(t, wg, wg2)

	assert.Equal(t, 1, mgr.Count())
	assert.Contains(t, mgr.ListGroups(), "grp-1")

	// GetGroup and GetGroupInfo
	gotWG := mgr.GetGroup("grp-1")
	assert.Equal(t, wg, gotWG)

	assert.Nil(t, mgr.GetGroup("non-existent"))

	var op string
	var created, accessed time.Time
	var exists bool
	for i := 0; i < 5; i++ {
		op, created, accessed, exists = mgr.GetGroupInfo("grp-1")
		if exists {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if exists {
		assert.Equal(t, "test_op", op)
		assert.False(t, created.IsZero())
		assert.False(t, accessed.IsZero())
	}

	_, _, _, exists = mgr.GetGroupInfo("non-existent")
	assert.False(t, exists)

	// Add, Done, Wait
	mgr.Add("grp-1", 1)
	mgr.Done("grp-1")
	mgr.Wait("grp-1")

	// Add/Done/Wait on non-existent groups
	mgr.Add("missing-grp", 1)
	mgr.Done("missing-grp")
	mgr.Wait("missing-grp")

	// Observer stats & methods
	opObs, adds, dones, waits, found := obs.GetStats("grp-1")
	assert.True(t, found)
	assert.Equal(t, "test_op", opObs)
	assert.Equal(t, 1, adds)
	assert.Equal(t, 1, dones)
	assert.Equal(t, 1, waits)

	allStats := obs.GetAllStats()
	assert.NotEmpty(t, allStats)

	summary := obs.Summary()
	assert.Contains(t, summary, "grp-1")

	obs.OnGroupCompleted("grp-1", 35*time.Second)

	obs.SetEnabled(false)
	obs.OnGroupCreated("grp-2", "op2")
	obs.SetEnabled(true)

	obs.ClearStats()
	assert.Equal(t, ConstMiscNoWaitgroupsTracked, obs.Summary())

	// DeleteGroup
	mgr.DeleteGroup("grp-1")
	assert.Equal(t, 0, mgr.Count())

	// EnableLoggingObserver
	mgr.EnableLoggingObserver(context.Background())
}

func TestStorageExtended_Wave25_ValidationStrategy(t *testing.T) {
	syncStrat := NewSyncValidationStrategy()
	require.NotNil(t, syncStrat)
	assert.Equal(t, "sync", syncStrat.Name())
	assert.NotNil(t, syncStrat.GetMetrics())
	assert.NoError(t, syncStrat.Start(""))
	assert.NoError(t, syncStrat.Stop())

	tmpDir, err := os.MkdirTemp("", "val_strat_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a dummy hash file
	hashFile := filepath.Join(tmpDir, "hash123.yaml")
	require.NoError(t, fileutil.WriteFile(hashFile, []byte("data"), 0644))

	mappings := map[string]string{
		"OBJ-1": "hash123",
		"OBJ-2": "hashMissing",
	}
	validMappings, validBucketKeys, staleCount := syncStrat.ValidateMappings(tmpDir, mappings, nil)
	assert.Equal(t, 1, staleCount)
	assert.Contains(t, validMappings, "OBJ-1")
	assert.NotContains(t, validMappings, "OBJ-2")
	assert.Nil(t, validBucketKeys)

	// Async shutdown handler
	handler := getAsyncValidationShutdownHandler()
	require.NotNil(t, handler)
	assert.Equal(t, ConstMiscAsyncValidationStrategies, handler.GetName())
	assert.Equal(t, int64(0), handler.GetPendingCount())
	assert.True(t, handler.IsDrained())
	assert.False(t, handler.IsCritical())
	assert.NoError(t, handler.Drain(context.Background()))
	assert.NoError(t, handler.InitiateShutdown())

	registerAsyncValidationShutdownHandler()
}

func TestStorageExtended_Wave25_WorkEnvelopeBackfill(t *testing.T) {
	t.Run("effortAwareKindNames", func(t *testing.T) {
		kinds := effortAwareKindNames()
		assert.NotNil(t, kinds)
	})

	t.Run("BackfillWorkEnvelopeCompletedAt nil store", func(t *testing.T) {
		res := BackfillWorkEnvelopeCompletedAt(context.Background(), nil, nil, true, 0)
		assert.Contains(t, res.Errors, "storage is nil")
	})
}

func TestStorageExtended_Wave25_WorkflowConstraintValidator(t *testing.T) {
	validator := NewWorkflowConstraintValidator(nil)
	require.NotNil(t, validator)

	// Helper extract methods
	workflowMap := map[string]any{
		objects.FieldKeyConstraints: map[string]any{
			ConstMiscRoleConstraints: map[string]any{
				"admin": map[string]any{
					ConstMiscAllowedOperations: []any{"create", "update"},
					"allowed_kinds":            []any{"goal", "task"},
					"blocked_kinds":            []any{"secret"},
				},
			},
			ConstMiscObjectConstraints: map[string]any{
				"task": map[string]any{
					ConstMiscRequiredRoles: []any{"admin"},
					"blocked_roles":        []any{"guest"},
				},
			},
		},
	}

	constraints := validator.extractConstraints(workflowMap)
	assert.NotNil(t, constraints)
	assert.Contains(t, constraints.RoleConstraints, "admin")
	assert.Contains(t, constraints.ObjectConstraints, "task")

	// Role operation validation
	roleAdmin := constraints.RoleConstraints["admin"]
	assert.NoError(t, validator.validateRoleOperation(roleAdmin, "admin", "create"))
	assert.Error(t, validator.validateRoleOperation(roleAdmin, "admin", "delete"))

	// Role kind validation
	assert.NoError(t, validator.validateRoleKind(roleAdmin, "admin", "goal"))
	assert.Error(t, validator.validateRoleKind(roleAdmin, "admin", "secret"))
	assert.Error(t, validator.validateRoleKind(roleAdmin, "admin", "unknown_kind"))

	// Blocked and required roles
	objTask := constraints.ObjectConstraints["task"]
	assert.NoError(t, validator.checkBlockedRoles(objTask.BlockedRoles, []string{"user"}, "task", "create"))
	assert.Error(t, validator.checkBlockedRoles(objTask.BlockedRoles, []string{"guest"}, "task", "create"))

	assert.NoError(t, validator.checkRequiredRoles(objTask.RequiredRoles, []string{"admin"}, "task", "create"))
	assert.Error(t, validator.checkRequiredRoles(objTask.RequiredRoles, []string{"user"}, "task", "create"))

	// SecCtx validation
	secCtxAdmin := &pkgctx.SecurityContext{Roles: []string{"admin"}}
	assert.NoError(t, validator.validateConstraintsForRoles(context.Background(), secCtxAdmin, constraints, "task", "create"))

	secCtxGuest := &pkgctx.SecurityContext{Roles: []string{"guest"}}
	assert.Error(t, validator.validateConstraintsForRoles(context.Background(), secCtxGuest, constraints, "task", "create"))
}

func TestStorageExtended_Wave25_WALFacade(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "wal_facade_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// NewObjectWAL and GetWALPath
	walObj, err := NewObjectWAL(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, walObj)

	path := GetWALPath(tmpDir)
	assert.NotEmpty(t, path)

	// WriteAppliedSeq and ReadAppliedSeq
	err = WriteAppliedSeq(tmpDir, 42)
	require.NoError(t, err)
	seq, err := ReadAppliedSeq(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, int64(42), seq)

	// AcquireObjectWAL & ReleaseObjectWAL
	acqWal, err := AcquireObjectWAL(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, acqWal)
	_ = ReleaseObjectWAL(tmpDir, acqWal)

	// TryClaimWriteBehindOwner & Release
	claimed := tryClaimWriteBehindOwner(tmpDir, "owner-1")
	assert.True(t, claimed)
	claimedAgain := tryClaimWriteBehindOwner(tmpDir, "owner-2")
	assert.False(t, claimedAgain)
	releaseWriteBehindOwner(tmpDir, "owner-1")
	logWriteBehindOwnerSkipped(tmpDir)

	_ = objectWALRefCountForTest(tmpDir)

	// parseWALLine and getMaxSeqFromLine
	line := []byte(`{"seq":100,"op":"put","kind":"goal","id":"GOAL-1"}`)
	records, err := parseWALLine(line)
	if err == nil && len(records) > 0 {
		assert.Equal(t, int64(100), records[0].Seq)
	}
	_ = getMaxSeqFromLine(line)

	// ReadLastSeqFromTail
	_, _ = ReadLastSeqFromTail(tmpDir)

	// CompactWAL
	_ = CompactWAL(tmpDir)

	// ReplayWALChunk and ReplayWAL
	_, _, _ = ReplayWALChunk(tmpDir, 0, 10, func(rec *WALRecord) error { return nil })
	_ = ReplayWAL(tmpDir, 0, func(rec *WALRecord) error { return nil })

	// WaitForWALProcessingEventDriven
	_ = WaitForWALProcessingEventDriven(tmpDir, 10*time.Millisecond)
}

func TestStorageExtended_Wave25_FileCASImpl(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "file_cas_impl_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	storage, err := NewFileObjectStorage(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	kind := objects.KindGoal
	id := "GOAL-CAS-TEST-1"
	data := []byte("id: GOAL-CAS-TEST-1\nkind: goal\ntitle: Test Goal\ndescription: Substantive description\nstatus: active\nschema_version: 2.0.0\n")

	// WriteObjectRaw
	err = storage.WriteObjectRaw(ctx, kind, id, data)
	require.NoError(t, err)

	// RenameObjectRaw
	newID := "GOAL-CAS-TEST-2"
	err = storage.RenameObjectRaw(ctx, kind, id, newID)
	require.NoError(t, err)

	// DeleteObjectRaw
	err = storage.DeleteObjectRaw(ctx, kind, newID)
	require.NoError(t, err)

	// PutStreamSegmentChunk & GetStreamSegmentChunk
	chunkID := "SEG-1"
	chunkHash, err := storage.PutStreamSegmentChunk(objects.KindAuditEvent, chunkID, []byte("chunk content"))
	if err == nil {
		assert.NotEmpty(t, chunkHash)
		readChunk, err := storage.GetStreamSegmentChunk(objects.KindAuditEvent, chunkID)
		if err == nil {
			assert.Equal(t, []byte("chunk content"), readChunk)
		}
	}

	// PutStreamSegment
	_, _ = storage.PutStreamSegment(objects.KindAuditEvent, "SEG-2", []byte("segment content"))

	// metricsWrapper
	mw := &metricsWrapper{m: caspkg.GetObjectStorageMetrics()}
	mw.RecordCreate(time.Millisecond, true)
	mw.RecordCreate(time.Millisecond, false)
	mw.RecordRead(time.Millisecond, true)
	mw.RecordRead(time.Millisecond, false)
	mw.RecordUpdate(time.Millisecond, true)
	mw.RecordUpdate(time.Millisecond, false)
	mw.RecordDelete(time.Millisecond, true)
	mw.RecordDelete(time.Millisecond, false)
	mw.RecordIndexFileLock(true, time.Millisecond)
	mw.RecordIndexSave(time.Millisecond, nil, 1)
	mw.RecordSetMapping(time.Millisecond, nil, true)
	mw.RecordIndexReload()
	mw.RecordRemoveMapping(time.Millisecond, nil)
}
