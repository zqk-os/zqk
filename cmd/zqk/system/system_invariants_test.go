package system

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

func executeCycle29Command(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cmd.SetContext(pkgctx.WithCommandOutputWriter(ctx, buf))
	err := cmd.Execute()
	return buf.String(), err
}

// TestCycle29_AutoFixBatchCmd_FlagsAndDefaults validates CRIT-1793632807358585000-41bf155a:
// Command layout, flag registration, default values, and unrecognized flag rejection.
func TestCycle29_AutoFixBatchCmd_FlagsAndDefaults(t *testing.T) {
	t.Parallel()

	cmd := NewAutoFixBatchCmd()
	require.NotNil(t, cmd)

	assert.Equal(t, "auto-fix-batch", cmd.Use)
	assert.True(t, cmd.Hidden, "auto-fix-batch should be hidden as an internal command")
	assert.Contains(t, cmd.Short, "Process a batch of auto-fixable issues")

	// Flag registration and defaults
	batchFileFlag := cmd.Flags().Lookup("batch-file")
	require.NotNil(t, batchFileFlag)
	assert.Equal(t, "", batchFileFlag.DefValue)

	projectRootFlag := cmd.Flags().Lookup("project-root")
	require.NotNil(t, projectRootFlag)
	assert.Equal(t, "", projectRootFlag.DefValue)

	chunkSizeFlag := cmd.Flags().Lookup("chunk-size")
	require.NotNil(t, chunkSizeFlag)
	assert.Equal(t, "0", chunkSizeFlag.DefValue)

	// Valid flag value setting
	require.NoError(t, cmd.Flags().Set("batch-file", "/tmp/autofix.json"))
	require.NoError(t, cmd.Flags().Set("project-root", "/tmp/repo"))
	require.NoError(t, cmd.Flags().Set("chunk-size", "42"))

	val, err := cmd.Flags().GetInt("chunk-size")
	require.NoError(t, err)
	assert.Equal(t, 42, val)

	// Unrecognized flag rejection
	cmdUnrecognized := NewAutoFixBatchCmd()
	_, err = executeCycle29Command(t, cmdUnrecognized, "--nonexistent-flag-xyz")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flag")
}

// TestCycle29_AutoFixBatchCmd_RequiredArgumentsAndValidation validates required argument invariants
// and fail-closed error handling when missing required flags.
func TestCycle29_AutoFixBatchCmd_RequiredArgumentsAndValidation(t *testing.T) {
	t.Parallel()

	// Missing --batch-file
	cmdNoBatch := NewAutoFixBatchCmd()
	err := runAutoFixBatch(cmdNoBatch, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--batch-file is required")

	// Provided --batch-file but missing --project-root
	cmdNoRoot := NewAutoFixBatchCmd()
	require.NoError(t, cmdNoRoot.Flags().Set("batch-file", "/tmp/fake.json"))
	err = runAutoFixBatch(cmdNoRoot, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--project-root is required")

	// Non-existent batch file with project root
	tempDir := t.TempDir()
	cmdNonExistent := NewAutoFixBatchCmd()
	require.NoError(t, cmdNonExistent.Flags().Set("batch-file", filepath.Join(tempDir, "missing.json")))
	require.NoError(t, cmdNonExistent.Flags().Set("project-root", tempDir))
	err = runAutoFixBatch(cmdNonExistent, nil)
	require.Error(t, err)
}

// TestCycle29_AutoFixProcessPendingCmd_FlagsAndDefaults tests NewAutoFixProcessPendingCmd
// flag invariants, default values, and directory resolution.
func TestCycle29_AutoFixProcessPendingCmd_FlagsAndDefaults(t *testing.T) {
	t.Parallel()

	cmd := NewAutoFixProcessPendingCmd()
	require.NotNil(t, cmd)

	assert.True(t, cmd.Hidden)

	rootFlag := cmd.Flags().Lookup("project-root")
	require.NotNil(t, rootFlag)
	assert.Equal(t, "", rootFlag.DefValue)

	syncFlag := cmd.Flags().Lookup("sync-glossary")
	require.NotNil(t, syncFlag)
	assert.Equal(t, "false", syncFlag.DefValue)

	applyFlag := cmd.Flags().Lookup("sync-glossary-apply")
	require.NotNil(t, applyFlag)
	assert.Equal(t, "false", applyFlag.DefValue)

	// Execution without autofix dir should return cleanly without error
	tempDir := t.TempDir()
	runCmd := NewAutoFixProcessPendingCmd()
	require.NoError(t, runCmd.Flags().Set("project-root", tempDir))
	err := runAutoFixProcessPending(runCmd, nil)
	assert.NoError(t, err)
}

// TestCycle29_AutoFixBatchProcessor_InvariantsAndEdgeCases tests batch normalization,
// chunking, nil-safety, and execution against empty or invalid batches.
func TestCycle29_AutoFixBatchProcessor_InvariantsAndEdgeCases(t *testing.T) {
	t.Parallel()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	proc := NewAutoFixBatchProcessor("/tmp/cycle29_proc", logger, nil)
	require.NotNil(t, proc)
	assert.Equal(t, "/tmp/cycle29_proc", proc.projectRoot)

	// normalizeAutoFixBatchIssues nil and empty
	assert.Nil(t, normalizeAutoFixBatchIssues(nil))

	emptyBatch := &AutoFixBatch{}
	assert.Nil(t, normalizeAutoFixBatchIssues(emptyBatch))

	// normalizeAutoFixBatchIssues flat issues
	flatBatch := &AutoFixBatch{
		Issues: []AutoFixBatchIssue{
			{ObjectID: "OBJ-1", ObjectKind: "test_kind", Issue: Issue{Category: "cat1", Message: "msg1"}},
		},
	}
	normalizedFlat := normalizeAutoFixBatchIssues(flatBatch)
	require.Len(t, normalizedFlat, 1)
	assert.Equal(t, "OBJ-1", normalizedFlat[0].ObjectID)

	// normalizeAutoFixBatchIssues grouped objects
	groupedBatch := &AutoFixBatch{
		Objects: []AutoFixBatchObject{
			{
				ObjectID:   "OBJ-2",
				ObjectKind: "kind2",
				FilePath:   "/path/to/obj2.yaml",
				Issues: []Issue{
					{Category: "catA", Message: "msgA"},
					{Category: "catB", Message: "msgB"},
				},
			},
		},
	}
	normalizedGrouped := normalizeAutoFixBatchIssues(groupedBatch)
	require.Len(t, normalizedGrouped, 2)
	assert.Equal(t, "OBJ-2", normalizedGrouped[0].ObjectID)
	assert.Equal(t, "catA", normalizedGrouped[0].Issue.Category)
	assert.Equal(t, "catB", normalizedGrouped[1].Issue.Category)

	// ProcessBatch on empty batch
	resEmpty := proc.ProcessBatch(nil, nil, emptyBatch, "BATCH-EMPTY", 10)
	require.NotNil(t, resEmpty)
	assert.Equal(t, 0, resEmpty.Processed)
	assert.Equal(t, 0, resEmpty.Fixed)
	assert.Equal(t, 0, resEmpty.Failed)

	// ProcessBatch on invalid project root with issues (fails gracefully without panic)
	badProc := NewAutoFixBatchProcessor("/nonexistent/cycle29/path", logger, nil)
	resBad := badProc.ProcessBatch(nil, nil, flatBatch, "BATCH-FAIL", -1) // negative chunk size tests default fallback
	require.NotNil(t, resBad)
	assert.Equal(t, 1, resBad.Failed)
	assert.NotEmpty(t, resBad.Errors)
}

// TestCycle29_AutoFixBatch_HelperUtilities tests utility functions in auto_fix_batch_cmd.go:
// log messages, json decoding, renaming, cache invalidation, and path resolution.
func TestCycle29_AutoFixBatch_HelperUtilities(t *testing.T) {
	t.Parallel()

	// autofixChunkCommitLogMessage
	msgSkipped := autofixChunkCommitLogMessage(0, 5)
	assert.Contains(t, msgSkipped, "skipped = no fix applied")

	msgCommitted := autofixChunkCommitLogMessage(3, 0)
	assert.Equal(t, "Autofix chunk committed", msgCommitted)

	msgCommittedWithSkipped := autofixChunkCommitLogMessage(3, 2)
	assert.Equal(t, "Autofix chunk committed", msgCommittedWithSkipped)

	msgZero := autofixChunkCommitLogMessage(0, 0)
	assert.Equal(t, "Autofix chunk committed", msgZero)

	// decodeAutoFixBatch
	validJSON := []byte(`{"batch_id":"BATCH-C29","status":"pending"}`)
	b, trailing, err := decodeAutoFixBatch(validJSON)
	require.NoError(t, err)
	assert.Equal(t, "BATCH-C29", b.BatchID)
	assert.False(t, trailing)

	trailingJSON := []byte(`{"batch_id":"BATCH-TRAIL","status":"pending"}{"trailing":true}`)
	bTrail, trailingFound, err := decodeAutoFixBatch(trailingJSON)
	require.NoError(t, err)
	assert.Equal(t, "BATCH-TRAIL", bTrail.BatchID)
	assert.True(t, trailingFound)

	invalidJSON := []byte(`{"batch_id":`)
	_, _, err = decodeAutoFixBatch(invalidJSON)
	assert.Error(t, err)

	// renameBatchFileAfterProcessing
	tempDir := t.TempDir()

	// Non-existent file should return nil
	assert.NoError(t, renameBatchFileAfterProcessing(filepath.Join(tempDir, "missing.json"), true))

	// Success rename
	batchFile1 := filepath.Join(tempDir, "AUTOFIX-1.json")
	require.NoError(t, fileutil.WriteFile(batchFile1, []byte("{}"), paths.FilePerm644))
	require.NoError(t, renameBatchFileAfterProcessing(batchFile1, true))
	assert.False(t, fileutil.Exists(batchFile1))
	assert.True(t, fileutil.Exists(filepath.Join(tempDir, "FIXED_AUTOFIX-1.json")))

	// Already prefixed with FIXED_ should be a no-op
	fixedFile := filepath.Join(tempDir, "FIXED_AUTOFIX-1.json")
	require.NoError(t, renameBatchFileAfterProcessing(fixedFile, true))
	assert.True(t, fileutil.Exists(fixedFile))

	// Failure rename
	batchFile2 := filepath.Join(tempDir, "AUTOFIX-2.json")
	require.NoError(t, fileutil.WriteFile(batchFile2, []byte("{}"), paths.FilePerm644))
	require.NoError(t, renameBatchFileAfterProcessing(batchFile2, false))
	assert.False(t, fileutil.Exists(batchFile2))
	assert.True(t, fileutil.Exists(filepath.Join(tempDir, "PROCESSED_AUTOFIX-2.json")))

	// Already prefixed with PROCESSED_ should be a no-op
	processedFile := filepath.Join(tempDir, "PROCESSED_AUTOFIX-2.json")
	require.NoError(t, renameBatchFileAfterProcessing(processedFile, false))
	assert.True(t, fileutil.Exists(processedFile))

	// resolveBatchIssueFilePath
	dummyExisting := filepath.Join(tempDir, "existing.yaml")
	require.NoError(t, fileutil.WriteFile(dummyExisting, []byte("data"), paths.FilePerm644))
	resolved, ok := resolveBatchIssueFilePath(nil, "OBJ-1", "kind", dummyExisting)
	assert.True(t, ok)
	assert.Equal(t, dummyExisting, resolved)

	resolvedMissing, ok := resolveBatchIssueFilePath(nil, "OBJ-1", "kind", "/tmp/does-not-exist.yaml")
	assert.False(t, ok)
	assert.Equal(t, "/tmp/does-not-exist.yaml", resolvedMissing)

	// invalidateStaleCachesForMissingObject nil-safety
	invalidateStaleCachesForMissingObject(nil, nil, "")
	invalidateStaleCachesForMissingObject(nil, nil, "OBJ-1")
	valCache := validation.NewValidationStateCache(tempDir, time.Hour)
	objCache := GetGlobalObjectIDCache()
	invalidateStaleCachesForMissingObject(objCache, valCache, "OBJ-TEST")
}

// TestCycle29_AutoFixProcessingLock_Exclusivity tests lock acquisition and release invariants.
func TestCycle29_AutoFixProcessingLock_Exclusivity(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	lock1, acquired, err := tryAcquireAutoFixProcessingLock(tempDir)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NotNil(t, lock1)

	// Second acquisition must fail
	lock2, acquired2, err2 := tryAcquireAutoFixProcessingLock(tempDir)
	require.NoError(t, err2)
	assert.False(t, acquired2)
	assert.Nil(t, lock2)

	// Release first lock
	require.NoError(t, lock1.Close())

	// Third acquisition after close succeeds
	lock3, acquired3, err3 := tryAcquireAutoFixProcessingLock(tempDir)
	require.NoError(t, err3)
	assert.True(t, acquired3)
	require.NotNil(t, lock3)
	_ = lock3.Close()

	// acquireExclusiveAutoFixCheckLock nil cmd or without flags
	unlock, err := acquireExclusiveAutoFixCheckLock(nil)
	assert.NoError(t, err)
	assert.Nil(t, unlock)

	cmdNoFlags := NewCheckCmd()
	unlock, err = acquireExclusiveAutoFixCheckLock(cmdNoFlags)
	assert.NoError(t, err)
	assert.Nil(t, unlock)
}

// TestCycle29_CASBatchRepair_InvariantsAndEdgeCases tests auto_fix_helpers_cas.go:
// file replacement safety, content resolution, index drift repairs, and missing hash indexing.
func TestCycle29_CASBatchRepair_InvariantsAndEdgeCases(t *testing.T) {
	t.Parallel()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	tempDir := t.TempDir()
	kindDir := filepath.Join(tempDir, "criteria")
	require.NoError(t, fileutil.MkdirAll(kindDir, paths.DirPerm755))

	// removeOldKindFileIfCASReplacementExists:
	// 1. Empty paths: no-op
	removeOldKindFileIfCASReplacementExists(logger, "CRIT-1", "", "", kindDir)

	// 2. Old and new path are identical: no-op
	samePath := filepath.Join(kindDir, "same.yaml")
	removeOldKindFileIfCASReplacementExists(logger, "CRIT-1", samePath, samePath, kindDir)

	// 3. New path does not exist on disk: refuses to remove old file
	oldPath := filepath.Join(kindDir, "old.yaml")
	require.NoError(t, fileutil.WriteFile(oldPath, []byte("legacy"), paths.FilePerm644))
	missingNew := filepath.Join(kindDir, "missing_new.yaml")
	removeOldKindFileIfCASReplacementExists(logger, "CRIT-1", oldPath, missingNew, kindDir)
	assert.True(t, fileutil.Exists(oldPath), "old path must NOT be deleted when replacement is missing")

	// 4. Old path is outside kindDir: refuses to remove
	outsideOld := filepath.Join(tempDir, "outside.yaml")
	require.NoError(t, fileutil.WriteFile(outsideOld, []byte("outside"), paths.FilePerm644))
	validNew := filepath.Join(kindDir, "valid_new.yaml")
	require.NoError(t, fileutil.WriteFile(validNew, []byte("cas"), paths.FilePerm644))
	removeOldKindFileIfCASReplacementExists(logger, "CRIT-1", outsideOld, validNew, kindDir)
	assert.True(t, fileutil.Exists(outsideOld), "old path outside kindDir must NOT be deleted")

	// 5. Valid replacement: oldPath is under kindDir and validNew exists -> oldPath is removed
	removeOldKindFileIfCASReplacementExists(logger, "CRIT-1", oldPath, validNew, kindDir)
	assert.False(t, fileutil.Exists(oldPath), "old path under kindDir must be deleted when replacement exists")

	// resolveCASFixFilePathAndContent:
	fixCtxExisting := &AutoFixContext{
		FilePath: validNew,
		Obj:      &parser.ParsedObject{ID: "CRIT-TEST"},
		Logger:   logger,
	}
	fPath, fKind, content, err := resolveCASFixFilePathAndContent(tempDir, fixCtxExisting, "criteria")
	require.NoError(t, err)
	assert.Equal(t, validNew, fPath)
	assert.Equal(t, "criteria", fKind)
	assert.Equal(t, []byte("cas"), content)

	// resolveCASFixFilePathAndContent on missing file with unknown ID returns error
	fixCtxMissing := &AutoFixContext{
		FilePath: filepath.Join(tempDir, "missing.yaml"),
		Obj:      &parser.ParsedObject{ID: "UNKNOWN-ID"},
		Logger:   logger,
	}
	_, _, _, err = resolveCASFixFilePathAndContent(tempDir, fixCtxMissing, "criteria")
	assert.Error(t, err)

	// fixCASIndexOutOfSync:
	// Empty project root
	fixCtxEmptyRoot := &AutoFixContext{
		Ctx:    cli.ContextForProjectRoot(""),
		Obj:    &parser.ParsedObject{ID: "CRIT-1"},
		Logger: logger,
	}
	fixed, msg := fixCASIndexOutOfSync(fixCtxEmptyRoot, "criteria", nil)
	assert.False(t, fixed)
	assert.Empty(t, msg)

	// Missing file
	fixCtxBadFile := &AutoFixContext{
		Ctx:      cli.ContextForProjectRoot(tempDir),
		FilePath: filepath.Join(tempDir, "nonexistent.yaml"),
		Obj:      &parser.ParsedObject{ID: "CRIT-1"},
		Logger:   logger,
	}
	fixed, msg = fixCASIndexOutOfSync(fixCtxBadFile, "criteria", nil)
	assert.False(t, fixed)
	assert.Empty(t, msg)

	// Empty file
	emptyFilePath := filepath.Join(tempDir, "empty.yaml")
	require.NoError(t, fileutil.WriteFile(emptyFilePath, []byte(""), paths.FilePerm644))
	fixCtxEmptyFile := &AutoFixContext{
		Ctx:      cli.ContextForProjectRoot(tempDir),
		FilePath: emptyFilePath,
		Obj:      &parser.ParsedObject{ID: "CRIT-1"},
		Logger:   logger,
	}
	fixed, msg = fixCASIndexOutOfSync(fixCtxEmptyFile, "criteria", nil)
	assert.False(t, fixed)
	assert.Empty(t, msg)

	// Unknown kind without inferred ID prefix
	fixCtxUnknownKind := &AutoFixContext{
		Ctx:      cli.ContextForProjectRoot(tempDir),
		FilePath: validNew,
		Obj:      &parser.ParsedObject{ID: "UNKNOWN-OBJ-ID"},
		Logger:   logger,
	}
	fixed, msg = fixCASIndexOutOfSync(fixCtxUnknownKind, "nonexistent_kind_xyz", nil)
	assert.False(t, fixed)
	assert.Empty(t, msg)

	// fixCASMissingHash:
	// Missing file on disk returns true (skipped so missing files don't fail loops)
	fixCtxMissingFile := &AutoFixContext{
		Ctx:      cli.ContextForProjectRoot(tempDir),
		FilePath: filepath.Join(tempDir, "missing_for_hash.yaml"),
		Obj:      &parser.ParsedObject{ID: "CRIT-MISSING-HASH"},
		Logger:   logger,
	}
	fixed, msg = fixCASMissingHash(fixCtxMissingFile, "criteria", nil)
	assert.True(t, fixed)
	assert.Contains(t, msg, "Skipped - file not found")

	// Empty file
	fixed, msg = fixCASMissingHash(fixCtxEmptyFile, "criteria", nil)
	assert.False(t, fixed)
	assert.Empty(t, msg)

	// Unknown kind
	fixed, msg = fixCASMissingHash(fixCtxUnknownKind, "nonexistent_kind_xyz", nil)
	assert.False(t, fixed)
	assert.Empty(t, msg)

	// removeStaleCASIndexEntry
	cas := storage.NewContentAddressableStorage(kindDir, "criteria")
	fixed, msg = removeStaleCASIndexEntry(fixCtxExisting, cas, "criteria", "CRIT-STALE")
	assert.True(t, fixed)
	assert.Contains(t, msg, "CRIT-STALE")
}

// TestCycle29_HashFixing_InvariantsAndEdgeCases tests auto_fix_helpers_hash_fixing.go:
// registry selection, nil safety, and immutable object hash repairs.
func TestCycle29_HashFixing_InvariantsAndEdgeCases(t *testing.T) {
	t.Parallel()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	tempDir := t.TempDir()

	testCmd := &cobra.Command{}
	testCmd.SetContext(context.Background())

	fixCtx := &AutoFixContext{
		Ctx:               cli.ContextForProjectRoot(tempDir),
		Cmd:               testCmd,
		Obj:               &parser.ParsedObject{ID: "BLI-1", Kind: "backlog_item"},
		FilePath:          filepath.Join(tempDir, "backlog", "BLI-1.yaml"),
		Kind:              "backlog_item",
		Logger:            logger,
		HashRegistryCache: &HashRegistryCacheType{cache: make(map[string]storage.HashRegistryProvider)},
	}

	// fixHashViaRegistry with nil registry returns false cleanly
	fixed, msg := fixHashViaRegistry(fixCtx, "backlog_item", "orig_hash", false, nil)
	assert.False(t, fixed)
	assert.Empty(t, msg)

	// selectRegistryForUpdate with nil registry creates or selects via file path
	reg := selectRegistryForUpdate(fixCtx, "backlog_item", nil)
	// Even if nil when file doesn't exist on disk, it must execute safely without panicking
	_ = reg

	// fixImmutableObjectHash on non-existent storage root returns false gracefully
	badCtx := &AutoFixContext{
		Ctx:      cli.ContextForProjectRoot("/nonexistent/cycle29/storage"),
		Obj:      &parser.ParsedObject{ID: "IMM-1", Kind: "audit_event"},
		FilePath: "/nonexistent/cycle29/storage/audit/event.yaml",
		Logger:   logger,
	}
	fixed, msg = fixImmutableObjectHash(badCtx, "audit_event", "hash123", true, nil)
	assert.False(t, fixed)
	assert.Empty(t, msg)
}
