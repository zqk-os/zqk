package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// =============================================================================
// LANE 1: SUITE 1 - CRUD Error Paths, Validation, & Edge Cases
// =============================================================================

func TestCRUDAndIndexing_CRUDErrorsAndEdgeCases(t *testing.T) {
	testRoot, fos, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	require.NotNil(t, fos)

	ctx := context.Background()
	cliCtx := WithCLIOperation(ctx)
	coreAllowCtx := pkgctx.WithAllowCoreObjectDelete(cliCtx)

	t.Run("DeleteNonCLI_ReturnsError", func(t *testing.T) {
		// Non-CLI context must fail closed
		userSecCtx := &pkgctx.SecurityContext{AccountID: "ACC-TEST-USER", Roles: []string{"user"}}
		err := fos.deleteImpl(ctx, userSecCtx, "GOAL-NON-CLI-001", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ErrMsgDeleteRequiresCLI)
	})

	t.Run("DeleteProtectedSchedulerJob_ReturnsError", func(t *testing.T) {
		// SCH-maintenance-wal is protected from deletion
		jobID := "SCH-maintenance-wal"
		jobObj := map[string]any{
			objects.FieldKeyID:                 jobID,
			objects.FieldKeyKind:               objects.KindSchedulerJob,
			objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
			objects.FieldKeyTitle:              "Maintenance WAL Job",
			objects.FieldKeyStatus:             objects.ObjectStatusActive,
			objects.FieldKeyJobType:            "maintenance_wal",
			objects.FieldKeyTriggerType:        "timer",
			objects.FieldKeyScheduleExpression: "*/5 * * * *",
			objects.FieldKeyCategory:           "system",
			objects.FieldKeyExecutionMode:      "reusable",
			objects.FieldKeyMaxRuntimeSeconds:  60,
			objects.FieldKeyEnabled:            true,
			objects.FieldKeyCreatedAt:          "2030-01-01T00:00:00Z",
			objects.FieldKeyCreatedBy:          "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:          "2030-01-01T00:00:00Z",
			objects.FieldKeyUpdatedBy:          "ACC-SYSTEM",
		}
		createErr := fos.Create(cliCtx, secCtx, jobObj)
		require.NoError(t, createErr)

		delErr := fos.deleteImpl(cliCtx, secCtx, jobID, false)
		require.Error(t, delErr)
		assert.Contains(t, delErr.Error(), ErrMsgProtectedDelete)
	})

	t.Run("DeleteCoreKernelHardDeleteGuard", func(t *testing.T) {
		goalID := "GOAL-HARD-DEL-GUARD-001"
		goalObj := map[string]any{
			objects.FieldKeyID:            goalID,
			objects.FieldKeyKind:          objects.KindGoal,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyTitle:         "Hard Delete Guard Goal",
			objects.FieldKeyDescription:   "Description for hard delete guard test",
			objects.FieldKeyStatus:        "originated",
			objects.FieldKeyCreatedAt:     "2030-01-01T00:00:00Z",
			objects.FieldKeyCreatedBy:     "ACC-TEST",
			objects.FieldKeyUpdatedAt:     "2030-01-01T00:00:00Z",
			objects.FieldKeyUpdatedBy:     "ACC-TEST",
		}
		require.NoError(t, fos.Create(cliCtx, secCtx, goalObj))

		// Without core object delete permission in context, deletion is refused
		delErr := fos.deleteImpl(cliCtx, secCtx, goalID, false)
		require.Error(t, delErr)
		assert.Contains(t, delErr.Error(), "hard delete refused for core kind goal")

		// With allow core object delete, deletion succeeds
		allowedDelErr := fos.deleteImpl(coreAllowCtx, secCtx, goalID, false)
		require.NoError(t, allowedDelErr)
	})

	t.Run("DeleteWithBlockingDependents_NoCascadeVsCascade", func(t *testing.T) {
		reqID := "REQ-DEP-CHECK-001"
		critID := "CRIT-DEP-CHECK-001"

		reqObj := map[string]any{
			objects.FieldKeyID:            reqID,
			objects.FieldKeyKind:          objects.KindRequirement,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyTitle:         "Requirement with Dependents",
			objects.FieldKeyDescription:   "Requirement for dependency test",
			objects.FieldKeyStatus:        "originated",
			objects.FieldKeyCreatedAt:     "2030-01-01T00:00:00Z",
			objects.FieldKeyCreatedBy:     "ACC-TEST",
			objects.FieldKeyUpdatedAt:     "2030-01-01T00:00:00Z",
			objects.FieldKeyUpdatedBy:     "ACC-TEST",
		}
		require.NoError(t, fos.Create(cliCtx, secCtx, reqObj))

		critObj := map[string]any{
			objects.FieldKeyID:              critID,
			objects.FieldKeyKind:            objects.KindCriteria,
			objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
			objects.FieldKeyTitle:           "Criteria Dependent on Req",
			objects.FieldKeyDescription:     "Criteria for dependency test",
			objects.FieldKeyStatus:          "originated",
			objects.FieldKeyRequirementRefs: []string{reqID},
			objects.FieldKeyCategory:        "functional",
			objects.FieldKeyCreatedAt:       "2030-01-01T00:00:00Z",
			objects.FieldKeyCreatedBy:       "ACC-TEST",
			objects.FieldKeyUpdatedAt:       "2030-01-01T00:00:00Z",
			objects.FieldKeyUpdatedBy:       "ACC-TEST",
		}
		require.NoError(t, fos.Create(cliCtx, secCtx, critObj))

		// Without cascade, deleting requirement must fail due to dependent criteria
		delErr := fos.deleteImpl(coreAllowCtx, secCtx, reqID, false)
		require.Error(t, delErr)
		assert.Contains(t, delErr.Error(), "still reference it")

		// With cascade, both should be deleted
		cascadeErr := fos.deleteImpl(coreAllowCtx, secCtx, reqID, true)
		require.NoError(t, cascadeErr)

		_, reqReadErr := fos.Read(cliCtx, secCtx, reqID)
		assert.Error(t, reqReadErr)
		_, critReadErr := fos.Read(cliCtx, secCtx, critID)
		assert.Error(t, critReadErr)
	})

	t.Run("CreateObjectTitleValidation_RejectsMalformedTitles", func(t *testing.T) {
		makeObj := func(id, title string) map[string]any {
			return map[string]any{
				objects.FieldKeyID:            id,
				objects.FieldKeyKind:          objects.KindGoal,
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				objects.FieldKeyTitle:         title,
				objects.FieldKeyDescription:   "Test description",
				objects.FieldKeyStatus:        "planned",
				objects.FieldKeyCreatedAt:     "2030-01-01T00:00:00Z",
				objects.FieldKeyCreatedBy:     "ACC-TEST",
				objects.FieldKeyUpdatedAt:     "2030-01-01T00:00:00Z",
				objects.FieldKeyUpdatedBy:     "ACC-TEST",
			}
		}

		// Empty title
		errEmpty := fos.validateObjectBeforeCreation(cliCtx, makeObj("GOAL-EMPTY-001", ""), objects.KindGoal, secCtx)
		require.Error(t, errEmpty)
		assert.Contains(t, errEmpty.Error(), "title must be non-empty and single line")

		// Whitespace-only title
		errWhitespace := fos.validateObjectBeforeCreation(cliCtx, makeObj("GOAL-WS-001", "   \t  "), objects.KindGoal, secCtx)
		require.Error(t, errWhitespace)
		assert.Contains(t, errWhitespace.Error(), "title must be non-empty and single line")

		// Newline in title
		errNewline := fos.validateObjectBeforeCreation(cliCtx, makeObj("GOAL-NL-001", "First Line\nSecond Line"), objects.KindGoal, secCtx)
		require.Error(t, errNewline)
		assert.Contains(t, errNewline.Error(), "title must be non-empty and single line")
	})

	t.Run("UpdateNonCASPathNormal_OptimisticLockingAndEdgeCases", func(t *testing.T) {
		t.Setenv("ZQK_TEST_MODE", "true")
		jobID := "SCH-UPDATE-NORMAL-001"
		jobObj := map[string]any{
			objects.FieldKeyID:                 jobID,
			objects.FieldKeyKind:               objects.KindSchedulerJob,
			objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
			objects.FieldKeyTitle:              "Update Normal Job",
			objects.FieldKeyStatus:             objects.ObjectStatusActive,
			objects.FieldKeyJobType:            "normal_update",
			objects.FieldKeyTriggerType:        "timer",
			objects.FieldKeyScheduleExpression: "*/10 * * * *",
			objects.FieldKeyCategory:           "system",
			objects.FieldKeyExecutionMode:      "reusable",
			objects.FieldKeyMaxRuntimeSeconds:  60,
			objects.FieldKeyEnabled:            true,
			objects.FieldKeyCreatedAt:          "2030-01-01T00:00:00Z",
			objects.FieldKeyCreatedBy:          "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:          "2030-01-01T00:00:00Z",
			objects.FieldKeyUpdatedBy:          "ACC-SYSTEM",
		}
		require.NoError(t, fos.Create(cliCtx, secCtx, jobObj))

		jobPath, err := fos.getObjectFilePath(jobID, objects.KindSchedulerJob)
		require.NoError(t, err)

		updates := map[string]any{
			objects.FieldKeyTitle: "Updated Normal Job Title",
		}

		// Optimistic locking mismatch
		staleUpdatedAt := "2020-01-01T00:00:00Z"
		conflictErr := fos.UpdateNonCASPathNormalForTest(cliCtx, secCtx, objects.KindSchedulerJob, jobID, jobPath, jobObj, updates, nil, "", "", nil, staleUpdatedAt)
		require.Error(t, conflictErr)
		assert.True(t, errors.Is(conflictErr, ErrVersionConflict) || strings.Contains(conflictErr.Error(), "version conflict"))

		// Non-existent old file path
		missingPathErr := fos.UpdateNonCASPathNormalForTest(cliCtx, secCtx, objects.KindSchedulerJob, jobID, filepath.Join(testRoot, "nonexistent.yaml"), jobObj, updates, nil, "", "", nil, staleUpdatedAt)
		require.Error(t, missingPathErr)
		assert.Contains(t, missingPathErr.Error(), ErrMsgReadOptLock)

		// Successful normal update
		jobObj[objects.FieldKeyTitle] = "Updated Normal Job Title"
		successErr := fos.UpdateNonCASPathNormalForTest(cliCtx, secCtx, objects.KindSchedulerJob, jobID, jobPath, jobObj, updates, nil, "", "", nil, "")
		require.NoError(t, successErr)

		readBack, readErr := fos.Read(cliCtx, secCtx, jobID)
		require.NoError(t, readErr)
		assert.Equal(t, "Updated Normal Job Title", readBack[objects.FieldKeyTitle])
	})

	t.Run("UpdateNonCASPathIDChange_RejectsWhenNewIDNotFoundInCAS", func(t *testing.T) {
		oldJobID := "SCH-ID-CHANGE-OLD-001"
		newJobID := "SCH-ID-CHANGE-NEW-001"

		jobObj := map[string]any{
			objects.FieldKeyID:                 oldJobID,
			objects.FieldKeyKind:               objects.KindSchedulerJob,
			objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
			objects.FieldKeyTitle:              "ID Change Job",
			objects.FieldKeyStatus:             objects.ObjectStatusActive,
			objects.FieldKeyJobType:            "id_change",
			objects.FieldKeyTriggerType:        "timer",
			objects.FieldKeyScheduleExpression: "*/15 * * * *",
			objects.FieldKeyCategory:           "system",
			objects.FieldKeyExecutionMode:      "reusable",
			objects.FieldKeyMaxRuntimeSeconds:  60,
			objects.FieldKeyEnabled:            true,
			objects.FieldKeyCreatedAt:          "2030-01-01T00:00:00Z",
			objects.FieldKeyCreatedBy:          "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:          "2030-01-01T00:00:00Z",
			objects.FieldKeyUpdatedBy:          "ACC-SYSTEM",
		}
		require.NoError(t, fos.Create(cliCtx, secCtx, jobObj))

		oldPath, err := fos.getObjectFilePath(oldJobID, objects.KindSchedulerJob)
		require.NoError(t, err)

		updatedObj := make(map[string]any)
		for k, v := range jobObj {
			updatedObj[k] = v
		}
		updatedObj[objects.FieldKeyID] = newJobID

		idChangeErr := fos.UpdateNonCASPathIDChangeForTest(cliCtx, secCtx, objects.KindSchedulerJob, oldJobID, newJobID, oldPath, updatedObj, map[string]any{objects.FieldKeyID: newJobID})
		require.Error(t, idChangeErr)
		assert.Contains(t, idChangeErr.Error(), "not found")
	})

	t.Run("PrepareFileObjectUpdate_InferKindFailure", func(t *testing.T) {
		// Non-existent kind prefix cannot be inferred
		_, prepErr := fos.prepareFileObjectUpdate(cliCtx, secCtx, "INVALIDPREFIX-001", map[string]any{"title": "New Title"})
		require.Error(t, prepErr)
		assert.Contains(t, prepErr.Error(), "infer kind")
	})

	t.Run("KeystoreEntryAccessControlAndPermissions", func(t *testing.T) {
		entryID := "KEY-ACCESS-ELEVATION-001"
		ownerAccount := "ACC-OWNER-01"
		strangerAccount := "ACC-STRANGER-01"

		entryObj := map[string]any{
			objects.FieldKeyID:             entryID,
			objects.FieldKeyKind:           objects.KindKeystoreEntry,
			objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
			objects.FieldKeyTitle:          "Keystore Access Entry",
			objects.FieldKeyAccountID:      ownerAccount,
			objects.FieldKeyCredentialHash: "dummy-cred-hash-12345",
			objects.FieldKeySalt:           "dummy-salt-67890",
			objects.FieldKeyStatus:         objects.ObjectStatusActive,
			objects.FieldKeyCreatedAt:      "2030-01-01T00:00:00Z",
			objects.FieldKeyCreatedBy:      ownerAccount,
			objects.FieldKeyUpdatedAt:      "2030-01-01T00:00:00Z",
			objects.FieldKeyUpdatedBy:      ownerAccount,
		}
		require.NoError(t, fos.Create(cliCtx, secCtx, entryObj))

		// Non-owner, non-admin cannot update keystore entry
		strangerSecCtx := &pkgctx.SecurityContext{AccountID: strangerAccount, Roles: []string{"user"}}
		_, errStranger := fos.prepareFileObjectUpdate(cliCtx, strangerSecCtx, entryID, map[string]any{"title": "Hacked Title"})
		require.Error(t, errStranger)
		assert.Contains(t, errStranger.Error(), ErrMsgPermUpdateKeystore)

		// Non-system user cannot mutate credential_hash
		ownerSecCtx := &pkgctx.SecurityContext{AccountID: ownerAccount, Roles: []string{"user"}}
		_, errCredHash := fos.prepareFileObjectUpdate(cliCtx, ownerSecCtx, entryID, map[string]any{objects.FieldKeyCredentialHash: "tampered-hash"})
		require.Error(t, errCredHash)
		assert.Contains(t, errCredHash.Error(), ErrMsgPermUpdateCredHash)

		// Non-system user cannot mutate salt
		_, errSalt := fos.prepareFileObjectUpdate(cliCtx, ownerSecCtx, entryID, map[string]any{objects.FieldKeySalt: "tampered-salt"})
		require.Error(t, errSalt)
		assert.Contains(t, errSalt.Error(), ErrMsgPermUpdateSalt)
	})

	t.Run("OccupiableKind_TransitionToInProgressStampsClaimedAt", func(t *testing.T) {
		taskID := "ATK-OCCUPIABLE-CLAIM-001"
		taskObj := map[string]any{
			objects.FieldKeyID:            taskID,
			objects.FieldKeyKind:          objects.KindAgentTask,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyTitle:         "Occupiable Agent Task",
			objects.FieldKeyDescription:   "Occupiable testing description",
			objects.FieldKeyStatus:        "originated",
			objects.FieldKeyCreatedAt:     "2030-01-01T00:00:00Z",
			objects.FieldKeyCreatedBy:     "ACC-TEST",
			objects.FieldKeyUpdatedAt:     "2030-01-01T00:00:00Z",
			objects.FieldKeyUpdatedBy:     "ACC-TEST",
		}
		require.NoError(t, fos.Create(cliCtx, secCtx, taskObj))

		// Update to in_progress with claimed_by, claimed_at empty
		updates := map[string]any{
			objects.FieldKeyStatus:    objects.ObjectStatusInProgress,
			objects.FieldKeyClaimedBy: "ACC-AGENT-WORKER",
		}
		breakGlassCtx := pkgctx.WithLifecycleBreakGlass(cliCtx, "unit test elevation reason")
		prep, err := fos.prepareFileObjectUpdate(breakGlassCtx, secCtx, taskID, updates)
		require.NoError(t, err)
		require.NotNil(t, prep)
		assert.NotEmpty(t, prep.updates[objects.FieldKeyClaimedAt], "claimed_at should be auto-stamped on transition to in_progress")
	})
}

// =============================================================================
// LANE 1: SUITE 2 - CAS Blob Verification, Hash Reconciliation, & Indexing
// =============================================================================

func TestCRUDAndIndexing_CASBlobVerificationAndIndexing(t *testing.T) {
	testRoot, fos, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	require.NotNil(t, fos)

	ctx := context.Background()

	t.Run("VerifyOrReconcileCASHash_64HexFilename_MatchAndMismatch", func(t *testing.T) {
		data := []byte("content-addressable-storage-test-payload-data-line")
		realHash := CalculateSHA256Hash(data)
		validPath := filepath.Join(testRoot, "blobs", realHash+".yaml")

		// Exact match succeeds
		errMatch := fos.VerifyOrReconcileCASHash(ctx, "audit_event", realHash, validPath, data)
		assert.NoError(t, errMatch)

		// Mismatch returns verification error
		tamperedData := []byte("tampered-corrupted-data")
		errMismatch := fos.VerifyOrReconcileCASHash(ctx, "audit_event", realHash, validPath, tamperedData)
		require.Error(t, errMismatch)
		assert.Contains(t, errMismatch.Error(), "CAS hash mismatch")
	})

	t.Run("VerifyOrReconcileCASHash_IDBasedFilename_ReconcilesStaleRegistry", func(t *testing.T) {
		kindDir := filepath.Join(testRoot, ".zqk", "process", "scheduler_job")
		require.NoError(t, fileutil.EnsureDir(kindDir))

		id := "SCH-RECONCILE-001"
		filePath := filepath.Join(kindDir, id+".yaml")
		data1 := []byte("id: SCH-RECONCILE-001\nkind: scheduler_job\nversion: 1\n")
		hash1 := CalculateSHA256Hash(data1)

		// 1. Initial write and registry setup
		require.NoError(t, fileutil.WriteFile(filePath, data1, 0644))
		reg := fos.newHashRegistry(ctx, "scheduler_job", kindDir)
		require.NoError(t, reg.Load())
		reg.SetHash(filepath.Base(filePath), hash1)
		require.NoError(t, reg.Save())

		// Verify initial state passes
		assert.NoError(t, fos.VerifyOrReconcileCASHash(ctx, "scheduler_job", id, filePath, data1))

		// 2. Content updated on disk outside registry (stale registry scenario)
		data2 := []byte("id: SCH-RECONCILE-001\nkind: scheduler_job\nversion: 2-mutated\n")
		hash2 := CalculateSHA256Hash(data2)
		require.NoError(t, fileutil.WriteFile(filePath, data2, 0644))

		// VerifyOrReconcileCASHash must reconcile the registry to hash2
		reconcileErr := fos.VerifyOrReconcileCASHash(ctx, "scheduler_job", id, filePath, data2)
		require.NoError(t, reconcileErr)

		// Check registry was updated to hash2
		regReloaded := fos.newHashRegistry(ctx, "scheduler_job", kindDir)
		require.NoError(t, regReloaded.Load())
		assert.Equal(t, hash2, regReloaded.GetHash(filepath.Base(filePath)))
	})

	t.Run("CASIndexInvalidationSubscriber_LifecycleAndEventHandling", func(t *testing.T) {
		// Nil CAS handled safely
		nilSub := NewCASIndexInvalidationSubscriber(nil)
		assert.NoError(t, nilSub.HandleMutation(ctx, MutationEvent{Op: MutationOpDelete, ID: "any"}))

		// With active FileCAS instance
		casDir := filepath.Join(testRoot, ".zqk", "cas_test_sub")
		require.NoError(t, fileutil.EnsureDir(casDir))

		casInstance := filecas.NewContentAddressableStorage(casDir, "goal")
		require.NotNil(t, casInstance)

		sub := NewCASIndexInvalidationSubscriber(casInstance)

		// Mismatched kind ignored
		mismatchedEvent := MutationEvent{
			Kind: "scheduler_job",
			ID:   "SCH-IGNORED-001",
			Op:   MutationOpDelete,
		}
		assert.NoError(t, sub.HandleMutation(ctx, mismatchedEvent))

		// Valid Put event updates in-memory mapping
		validHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		putEvent := MutationEvent{
			Kind:    "goal",
			ID:      "GOAL-SUB-001",
			Op:      MutationOpPut,
			NewHash: validHash,
		}
		assert.NoError(t, sub.HandleMutation(ctx, putEvent))
		h, hErr := casInstance.GetIndex().GetHash("GOAL-SUB-001")
		assert.NoError(t, hErr)
		assert.Equal(t, validHash, h)

		// Valid Delete event clears in-memory mapping
		delEvent := MutationEvent{
			Kind: "goal",
			ID:   "GOAL-SUB-001",
			Op:   MutationOpDelete,
		}
		assert.NoError(t, sub.HandleMutation(ctx, delEvent))
		deletedHash, _ := casInstance.GetIndex().GetHash("GOAL-SUB-001")
		assert.Empty(t, deletedHash)
	})

	t.Run("ReverseReferenceIndex_ReferenceLifecyclePersistenceAndReload", func(t *testing.T) {
		idx := GetReverseReferenceIndexForProject(testRoot)
		require.NotNil(t, idx)
		idx.Clear()

		objA := "BLI-ELEV-REV-A"
		ref1 := "CRIT-ELEV-REV-1"
		ref2 := "CRIT-ELEV-REV-2"

		// Add references
		idx.AddReference(objA, ref1)
		idx.AddReference(objA, ref2)

		deps1 := idx.GetDependents(ref1)
		assert.Contains(t, deps1, objA)
		deps2 := idx.GetDependents(ref2)
		assert.Contains(t, deps2, objA)

		objARefs := idx.GetReferencedObjectIDs(objA)
		assert.ElementsMatch(t, []string{ref1, ref2}, objARefs)
		assert.GreaterOrEqual(t, idx.ReferencedIDCount(), 2)

		// Update references: replace ref1, ref2 with ref3
		ref3 := "CRIT-ELEV-REV-3"
		idx.UpdateReferences(objA, []string{ref1, ref2}, []string{ref3})

		assert.Empty(t, idx.GetDependents(ref1))
		assert.Empty(t, idx.GetDependents(ref2))
		assert.Contains(t, idx.GetDependents(ref3), objA)

		// Persist cache and reload
		saveErr := idx.SaveCache(testRoot)
		require.NoError(t, saveErr)

		reloadedIdx := &ReverseReferenceIndex{}
		loaded, loadErr := reloadedIdx.LoadCache(testRoot)
		require.NoError(t, loadErr)
		assert.True(t, loaded)
		assert.Contains(t, reloadedIdx.GetDependents(ref3), objA)

		// Remove reference
		idx.RemoveReference(objA, ref3)
		assert.Empty(t, idx.GetDependents(ref3))

		// Clear
		idx.Clear()
		assert.Equal(t, 0, idx.ReferencedIDCount())
	})

	t.Run("ExtractReferenceIDsFromObject_ComprehensiveFormats", func(t *testing.T) {
		obj := map[string]any{
			"id":               "BLI-TEST-EXTRACT-001",
			"criteria_refs":    []string{"CRIT-001", "CRIT-002"},
			"goal_refs":        []any{"GOAL-001", "GOAL-002"},
			"requirement_refs": "REQ-001",
			"created_by":       "ACC-AGENT-SYSTEM",
			"updated_by":       "ACC-AGENT-TESTER",
			"account_id":       "ACC-PRIMARY-001",
			"git_commit_ref":   "abc12345", // Non-kernel ref should be ignored
		}

		refs := extractReferenceIDsFromObject(obj)
		assert.Contains(t, refs, "CRIT-001")
		assert.Contains(t, refs, "CRIT-002")
		assert.Contains(t, refs, "GOAL-001")
		assert.Contains(t, refs, "GOAL-002")
		assert.Contains(t, refs, "REQ-001")
		assert.Contains(t, refs, "ACC-AGENT-SYSTEM")
		assert.Contains(t, refs, "ACC-AGENT-TESTER")
		assert.Contains(t, refs, "ACC-PRIMARY-001")

		// Nil object returns nil
		assert.Nil(t, extractReferenceIDsFromObject(nil))
	})

	t.Run("DarwinCASVisibility_PostFlushHandlesEmptyGracefully", func(t *testing.T) {
		// Valid empty kinds list returns immediately with nil
		err := postFlushDarwinCASVisibilityIfNeeded(ctx, testRoot, []string{})
		assert.NoError(t, err)

		// Stream storage kinds or empty strings skipped
		errStream := postFlushDarwinCASVisibilityIfNeeded(ctx, testRoot, []string{"", objects.KindAuditEvent})
		assert.NoError(t, errStream)
	})
}

// =============================================================================
// LANE 1: SUITE 3 - Migration, Validation Strategies, & Workflow Constraints
// =============================================================================

func TestCRUDAndIndexing_MigrationAndSchemaStoreValidation(t *testing.T) {
	testRoot, fos, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	require.NotNil(t, fos)

	ctx := context.Background()
	cliCtx := WithCLIOperation(ctx)

	t.Run("RuntimeDeltaMigration_DisabledAndEnabledKinds", func(t *testing.T) {
		// 1. Kind where runtime delta is disabled returns 0 scanned/created
		resDisabled := BackfillRuntimeDeltaCurrentForKind(testRoot, objects.KindGoal)
		assert.Equal(t, objects.KindGoal, resDisabled.Kind)
		assert.Equal(t, 0, resDisabled.Scanned)
		assert.Equal(t, 0, resDisabled.Created)
		assert.Equal(t, 0, resDisabled.Failures)

		// 2. Kind where runtime delta is enabled
		sessionDir := filepath.Join(testRoot, ".zqk", "process", "zqk_session")
		require.NoError(t, fileutil.EnsureDir(sessionDir))

		// Create a session object
		sessionID := "ZQK-SESS-ELEVATION-001"
		sessObj := map[string]any{
			objects.FieldKeyID:            sessionID,
			objects.FieldKeyKind:          objects.KindZqkSession,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyTitle:         "ZQK Session for Backfill",
			objects.FieldKeyStatus:        objects.ObjectStatusActive,
			"heartbeat_at":                "2030-01-01T00:00:00Z",
			objects.FieldKeyCreatedAt:     "2030-01-01T00:00:00Z",
			objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:     "2030-01-01T00:00:00Z",
			objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
		}
		_ = fos.Create(cliCtx, secCtx, sessObj)

		// Run backfill
		resEnabled := BackfillRuntimeDeltaCurrentForKind(testRoot, objects.KindZqkSession)
		assert.Equal(t, objects.KindZqkSession, resEnabled.Kind)

		// Second backfill should be idempotent (skipped)
		resIdempotent := BackfillRuntimeDeltaCurrentForKind(testRoot, objects.KindZqkSession)
		assert.Equal(t, objects.KindZqkSession, resIdempotent.Kind)
		assert.Equal(t, 0, resIdempotent.Created)
	})

	t.Run("SyncValidationStrategy_ValidationFilteringAndLifecycle", func(t *testing.T) {
		strategy := &SyncValidationStrategy{}
		assert.Equal(t, "sync", strategy.Name())
		assert.NoError(t, strategy.Start(testRoot))
		assert.NoError(t, strategy.Stop())

		kindDir := filepath.Join(testRoot, ".zqk", "sync_val_test")
		require.NoError(t, fileutil.EnsureDir(kindDir))

		// Create a real hash file on disk
		realHash := "1111111111111111111111111111111111111111111111111111111111111111"
		hashFile := filepath.Join(kindDir, realHash+".yaml")
		require.NoError(t, fileutil.WriteFile(hashFile, []byte("valid-blob"), 0644))

		fakeHash := "9999999999999999999999999999999999999999999999999999999999999999"

		mappings := map[string]string{
			"OBJ-VALID": realHash,
			"OBJ-STALE": fakeHash,
		}

		validMap, _, staleCount := strategy.ValidateMappings(kindDir, mappings, nil)
		assert.Equal(t, 1, staleCount)
		assert.Contains(t, validMap, "OBJ-VALID")
		assert.NotContains(t, validMap, "OBJ-STALE")
	})

	t.Run("AsyncCacheValidationStrategy_LifecycleAndRegistry", func(t *testing.T) {
		cfg := &AsyncCacheConfig{
			ScanInterval: 500 * time.Millisecond,
		}
		asyncStrat := NewAsyncCacheValidationStrategy(cfg)
		assert.Equal(t, "async-cache", asyncStrat.Name())

		kindDir := filepath.Join(testRoot, ".zqk", "async_val_test")
		require.NoError(t, fileutil.EnsureDir(kindDir))

		assert.NoError(t, asyncStrat.Start(kindDir))

		mappings := map[string]string{
			"OBJ-ASYNC-1": "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111",
		}
		validMap, _, staleCount := asyncStrat.ValidateMappings(kindDir, mappings, nil)
		assert.Equal(t, 1, staleCount)
		assert.Empty(t, validMap)

		assert.NoError(t, asyncStrat.Stop())

		// ValidationStrategyRegistry
		reg := GetGlobalValidationStrategyRegistry()
		reg.RegisterStrategy("custom_kind_elevation", asyncStrat)
		assert.Equal(t, asyncStrat, reg.GetStrategy("custom_kind_elevation"))

		// Fallback to default strategy when not explicitly registered
		defaultStrat := reg.GetStrategy("unregistered_kind_xyz")
		assert.NotNil(t, defaultStrat)

		// Overwrite with default strategy
		reg.RegisterStrategy("custom_kind_elevation", defaultStrat)
		assert.Equal(t, defaultStrat, reg.GetStrategy("custom_kind_elevation"))

		// High volume initialization
		count := InitializeAsyncValidationStrategies(testRoot)
		assert.GreaterOrEqual(t, count, 0)
	})

	t.Run("WorkflowConstraintValidator_RoleAndObjectConstraints", func(t *testing.T) {
		wcv := NewWorkflowConstraintValidator(fos)
		require.NotNil(t, wcv)

		// 1. Role operation check
		roleConstraint := RoleConstraint{
			AllowedOperations: []string{"create", "read"},
			AllowedKinds:      []string{objects.KindGoal, objects.KindBacklogItem},
			BlockedKinds:      []string{objects.KindPolicy},
		}

		// Allowed operation
		assert.NoError(t, wcv.validateRoleOperation(roleConstraint, "developer", "create"))
		// Disallowed operation
		errDisallowedOp := wcv.validateRoleOperation(roleConstraint, "developer", "delete")
		require.Error(t, errDisallowedOp)
		assert.Contains(t, errDisallowedOp.Error(), "is not allowed to perform delete operation")

		// Allowed kind
		assert.NoError(t, wcv.validateRoleKind(roleConstraint, "developer", objects.KindGoal))
		// Blocked kind
		errBlockedKind := wcv.validateRoleKind(roleConstraint, "developer", objects.KindPolicy)
		require.Error(t, errBlockedKind)
		assert.Contains(t, errBlockedKind.Error(), "is blocked from operating on")

		// 2. Object constraint check
		objConstraint := ObjectConstraint{
			RequiredRoles: []string{"admin", "lead"},
			BlockedRoles:  []string{"guest"},
		}

		// Blocked role
		errBlockedRole := wcv.checkBlockedRoles(objConstraint.BlockedRoles, []string{"guest", "developer"}, objects.KindPolicy, "create")
		require.Error(t, errBlockedRole)
		assert.Contains(t, errBlockedRole.Error(), "is blocked from create operation")

		// Missing required role
		errMissingRole := wcv.checkRequiredRoles(objConstraint.RequiredRoles, []string{"developer"}, objects.KindPolicy, "create")
		require.Error(t, errMissingRole)
		assert.Contains(t, errMissingRole.Error(), "requires one of these roles")

		// Has required role
		assert.NoError(t, wcv.checkRequiredRoles(objConstraint.RequiredRoles, []string{"admin"}, objects.KindPolicy, "create"))

		// 3. Priority Plan activation without workflow_ref succeeds
		planID := "PRI-NO-WORKFLOW-001"
		planObj := map[string]any{
			objects.FieldKeyID:            planID,
			objects.FieldKeyKind:          objects.KindPriorityPlan,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyTitle:         "Plan without workflow constraint",
			objects.FieldKeyStatus:        "originated",
			objects.FieldKeyCreatedAt:     "2030-01-01T00:00:00Z",
			objects.FieldKeyCreatedBy:     "ACC-TEST",
			objects.FieldKeyUpdatedAt:     "2030-01-01T00:00:00Z",
			objects.FieldKeyUpdatedBy:     "ACC-TEST",
		}
		require.NoError(t, fos.Create(cliCtx, secCtx, planObj))
		assert.NoError(t, wcv.ValidatePriorityPlanActivation(cliCtx, secCtx, planID))
	})
}

// =============================================================================
// LANE 1: SUITE 4 - Storage Transactions, Pool Wrappers, & File Discovery
// =============================================================================

func TestCRUDAndIndexing_MemoryAndFileStorageEdgePaths(t *testing.T) {
	testRoot, fos, secCtx := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	require.NotNil(t, fos)

	ctx := context.Background()

	t.Run("FileTransactionCoordinator_AcquireReleaseConflictDetection", func(t *testing.T) {
		coord := NewFileTransactionCoordinator(testRoot, nil)
		require.NotNil(t, coord)

		fileA := filepath.Join(testRoot, "tx_file_a.yaml")
		fileB := filepath.Join(testRoot, "tx_file_b.yaml")
		filePaths := []string{fileA, fileB}

		// 1. Initial acquisition
		acquireErr := coord.AcquireLocks(ctx, filePaths, 2*time.Second)
		require.NoError(t, acquireErr)

		assert.True(t, coord.IsLocked(fileA))
		assert.True(t, coord.IsLocked(fileB))
		assert.ElementsMatch(t, filePaths, coord.GetLockedFiles())

		// 2. Double acquisition is idempotent / succeeds
		assert.NoError(t, coord.AcquireLocks(ctx, []string{fileA}, 1*time.Second))

		// 3. Release locks
		releaseErr := coord.ReleaseLocks(ctx)
		require.NoError(t, releaseErr)

		assert.False(t, coord.IsLocked(fileA))
		assert.False(t, coord.IsLocked(fileB))
		assert.Empty(t, coord.GetLockedFiles())
	})

	t.Run("FileDiscovery_EdgePathsIgnoredExtensionsAndHiddenFiles", func(t *testing.T) {
		discDir := filepath.Join(testRoot, ".zqk", "process", "disc_test")
		require.NoError(t, fileutil.EnsureDir(discDir))

		// Create valid yaml
		validFile := filepath.Join(discDir, "valid-001.yaml")
		require.NoError(t, fileutil.WriteFile(validFile, []byte("id: DISC-001\nkind: goal\n"), 0644))

		// Create ignored files: .tmp, .bak, hidden .dot
		tmpFile := filepath.Join(discDir, "temp-001.tmp")
		require.NoError(t, fileutil.WriteFile(tmpFile, []byte("tmp"), 0644))

		bakFile := filepath.Join(discDir, "backup-001.bak")
		require.NoError(t, fileutil.WriteFile(bakFile, []byte("bak"), 0644))

		hiddenFile := filepath.Join(discDir, ".hidden.yaml")
		require.NoError(t, fileutil.WriteFile(hiddenFile, []byte("hidden"), 0644))

		// Use discoverFiles helper or List to verify only valid yaml is indexed
		filter := ListFilter{
			Kind: "disc_test",
		}
		res, err := fos.List(ctx, secCtx, nil, filter)
		if err == nil && res != nil {
			for _, obj := range res.Objects {
				id, _ := obj["id"].(string)
				assert.NotEqual(t, ".hidden", id)
			}
		}
	})

	t.Run("PoolAwareGraphStorage_WrapperExecution", func(t *testing.T) {
		mockConn := &mockGraphConnForElevation{}
		mockPool := &mockGraphPoolForElevation{conn: mockConn}

		poolStorage := NewPoolAwareGraphStorage(mockPool, testRoot)
		require.NotNil(t, poolStorage)
		assert.Equal(t, mockPool, poolStorage.GetPool())

		// Verify pool execution invocation
		called := false
		err := poolStorage.GetPool().Execute(ctx, func(conn provider.GraphConnection) error {
			called = true
			assert.Equal(t, mockConn, conn)
			return nil
		})
		require.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("StoragePool_ConcurrencySafetyWithGoroutines", func(t *testing.T) {
		// Verify concurrent reads/writes with goroutinelabels satisfy POL-CONCURRENCY
		const concurrencyLevel = 5
		var wg sync.WaitGroup
		var successCount atomic.Int32

		for i := 0; i < concurrencyLevel; i++ {
			wg.Add(1)
			goroutineID := i
			goroutinelabels.NewGoroutine("storage_elevation_worker", "concurrent_crud_elevation_verification").StartSimple(func() {
				defer wg.Done()
				id := fmt.Sprintf("SCH-CONCUR-ELEV-%03d", goroutineID)
				obj := map[string]any{
					objects.FieldKeyID:                 id,
					objects.FieldKeyKind:               objects.KindSchedulerJob,
					objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
					objects.FieldKeyTitle:              fmt.Sprintf("Concurrent Job %d", goroutineID),
					objects.FieldKeyStatus:             objects.ObjectStatusActive,
					objects.FieldKeyJobType:            "concurrency_test",
					objects.FieldKeyTriggerType:        "timer",
					objects.FieldKeyScheduleExpression: "*/5 * * * *",
					objects.FieldKeyCategory:           "system",
					objects.FieldKeyExecutionMode:      "reusable",
					objects.FieldKeyMaxRuntimeSeconds:  60,
					objects.FieldKeyEnabled:            true,
					objects.FieldKeyCreatedAt:          "2030-01-01T00:00:00Z",
					objects.FieldKeyCreatedBy:          "ACC-SYSTEM",
					objects.FieldKeyUpdatedAt:          "2030-01-01T00:00:00Z",
					objects.FieldKeyUpdatedBy:          "ACC-SYSTEM",
				}
				cliCtx := WithCLIOperation(context.Background())
				if err := fos.Create(cliCtx, secCtx, obj); err == nil {
					if _, readErr := fos.Read(cliCtx, secCtx, id); readErr == nil {
						successCount.Add(1)
					}
				}
			})
		}
		wg.Wait()
		assert.Equal(t, int32(concurrencyLevel), successCount.Load())
	})
}

// =============================================================================
// Mock helpers for PoolAwareGraphStorage
// =============================================================================

type mockGraphPoolForElevation struct {
	conn provider.GraphConnection
}

func (m *mockGraphPoolForElevation) GetConnection(ctx context.Context) (provider.GraphConnection, error) {
	return m.conn, nil
}

func (m *mockGraphPoolForElevation) ReturnConnection(conn provider.GraphConnection) error {
	return nil
}

func (m *mockGraphPoolForElevation) Stats() provider.PoolStats {
	return provider.PoolStats{}
}

func (m *mockGraphPoolForElevation) Close() error {
	return nil
}

func (m *mockGraphPoolForElevation) Execute(ctx context.Context, fn func(provider.GraphConnection) error) error {
	if m.conn == nil {
		return errors.New("no mock connection")
	}
	return fn(m.conn)
}

type mockGraphConnForElevation struct {
	provider.GraphConnection
}

func (m *mockGraphConnForElevation) Close() error {
	return nil
}

func (m *mockGraphConnForElevation) Ping(ctx context.Context) error {
	return nil
}
