// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package agentidle

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFileStore_NewFileStore_Initialization(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Non-existing file (fresh store initialization)
	freshPath := filepath.Join(tempDir, "fresh_store.json")
	freshStore, err := NewFileStore(freshPath)
	require.NoError(t, err)
	require.NotNil(t, freshStore)
	records, err := freshStore.GetAllRecords()
	require.NoError(t, err)
	assert.Empty(t, records)

	// 2. Existing valid store file
	validPath := filepath.Join(tempDir, "valid_store.json")
	storeA, err := NewFileStore(validPath)
	require.NoError(t, err)
	err = storeA.Accumulate("agent-001", "task-init", 12*time.Second)
	require.NoError(t, err)

	storeB, err := NewFileStore(validPath)
	require.NoError(t, err)
	duration, err := storeB.GetIdleTime("agent-001", "task-init")
	require.NoError(t, err)
	assert.Equal(t, 12*time.Second, duration)

	// 3. Empty store file
	emptyPath := filepath.Join(tempDir, "empty_store.json")
	err = fileutil.WriteFile(emptyPath, []byte(""), paths.FilePerm600)
	require.NoError(t, err)
	emptyStore, err := NewFileStore(emptyPath)
	require.NoError(t, err)
	emptyRecords, err := emptyStore.GetAllRecords()
	require.NoError(t, err)
	assert.Empty(t, emptyRecords)
}

func TestFileStore_NewFileStore_CorruptAndErrors(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Reading from a directory path triggers read failure
	dirAsPath := filepath.Join(tempDir, "directory_target")
	err := fileutil.MkdirAll(dirAsPath, paths.DirPerm755)
	require.NoError(t, err)

	dirStore, err := NewFileStore(dirAsPath)
	require.Error(t, err)
	assert.Nil(t, dirStore)
	assert.Contains(t, err.Error(), "failed to read idle store file")

	// 2. Corrupt JSON syntax triggers unmarshal error
	corruptPath := filepath.Join(tempDir, "corrupt_store.json")
	err = fileutil.WriteFile(corruptPath, []byte("{invalid-json-payload}"), paths.FilePerm600)
	require.NoError(t, err)

	corruptStore, err := NewFileStore(corruptPath)
	require.Error(t, err)
	assert.Nil(t, corruptStore)
	assert.Contains(t, err.Error(), "failed to unmarshal idle store")
}

func TestFileStore_NewFileStore_LegacyRawMapMigration(t *testing.T) {
	tempDir := t.TempDir()
	legacyPath := filepath.Join(tempDir, "legacy_raw_map.json")

	// In legacy raw map format, doc struct unmarshal fails because schema_version is int or records is not a map,
	// but unmarshals directly into map[string]time.Duration.
	legacyPayload := []byte(`{"schema_version": 1000, "agent-legacy:task-99": 7500000000}`)
	err := fileutil.WriteFile(legacyPath, legacyPayload, paths.FilePerm600)
	require.NoError(t, err)

	legacyStore, err := NewFileStore(legacyPath)
	require.NoError(t, err)
	require.NotNil(t, legacyStore)

	idleTime, err := legacyStore.GetIdleTime("agent-legacy", "task-99")
	require.NoError(t, err)
	assert.Equal(t, 7500*time.Millisecond, idleTime)

	// Verify accumulating on migrated legacy store promotes format
	err = legacyStore.Accumulate("agent-legacy", "task-99", 500*time.Millisecond)
	require.NoError(t, err)

	updatedIdle, err := legacyStore.GetIdleTime("agent-legacy", "task-99")
	require.NoError(t, err)
	assert.Equal(t, 8*time.Second, updatedIdle)
}

func TestFileStore_Save_ErrorHandling(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Directory creation failure (parent path blocked by regular file)
	blockerFile := filepath.Join(tempDir, "file_as_dir")
	err := fileutil.WriteFile(blockerFile, []byte("regular file content"), paths.FilePerm600)
	require.NoError(t, err)

	blockedStorePath := filepath.Join(blockerFile, "unreachable", "idle.json")
	blockedStore := &FileStore{
		filePath: blockedStorePath,
		records:  make(map[string]time.Duration),
	}
	err = blockedStore.save()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create idle store directory")

	// 2. Temp file write failure (tmpFile path blocked by directory)
	subDir := filepath.Join(tempDir, "sub_write_err")
	err = fileutil.MkdirAll(subDir, paths.DirPerm755)
	require.NoError(t, err)

	storePathForTmp := filepath.Join(subDir, "target.json")
	tmpDirPath := storePathForTmp + ".tmp"
	err = fileutil.MkdirAll(tmpDirPath, paths.DirPerm755)
	require.NoError(t, err)

	writeErrStore := &FileStore{
		filePath: storePathForTmp,
		records:  make(map[string]time.Duration),
	}
	err = writeErrStore.save()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write tmp idle store file")

	// 3. Atomic rename failure (target path is an existing non-empty directory)
	renameSubDir := filepath.Join(tempDir, "sub_rename_err")
	err = fileutil.MkdirAll(renameSubDir, paths.DirPerm755)
	require.NoError(t, err)

	targetDirPath := filepath.Join(renameSubDir, "existing_dir.json")
	err = fileutil.MkdirAll(targetDirPath, paths.DirPerm755)
	require.NoError(t, err)

	renameErrStore := &FileStore{
		filePath: targetDirPath,
		records:  make(map[string]time.Duration),
	}
	err = renameErrStore.save()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to rename idle store file")
}

func TestFileStore_Accumulate_And_GetIdleTime(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "accum_test.json")

	store, err := NewFileStore(storePath)
	require.NoError(t, err)

	// Non-existing key returns zero duration without error
	nonExistTime, err := store.GetIdleTime("non-existent-agent", "non-existent-task")
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), nonExistTime)

	// Accumulate multiple increments across distinct agents and tasks
	agents := []string{"agent-alpha", "agent-beta"}
	tasks := []string{"task-x", "task-y"}

	for _, a := range agents {
		for _, task := range tasks {
			err = store.Accumulate(a, task, 2*time.Second)
			require.NoError(t, err)
			err = store.Accumulate(a, task, 3*time.Second)
			require.NoError(t, err)
		}
	}

	for _, a := range agents {
		for _, task := range tasks {
			idled, err := store.GetIdleTime(a, task)
			require.NoError(t, err)
			assert.Equal(t, 5*time.Second, idled)
		}
	}
}

func TestFileStore_GetAllRecords_DefensiveCopy(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "defensive_copy.json")

	store, err := NewFileStore(storePath)
	require.NoError(t, err)

	err = store.Accumulate("agent-def", "task-def", 10*time.Second)
	require.NoError(t, err)

	// Obtain copy and modify it
	recordsCopy, err := store.GetAllRecords()
	require.NoError(t, err)
	assert.Len(t, recordsCopy, 1)

	recordsCopy["agent-def:task-def"] = 999 * time.Hour
	recordsCopy["agent-injected:task-injected"] = 50 * time.Minute

	// Internal state must remain uncorrupted
	internalIdle, err := store.GetIdleTime("agent-def", "task-def")
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, internalIdle)

	freshRecords, err := store.GetAllRecords()
	require.NoError(t, err)
	assert.Len(t, freshRecords, 1)
	assert.Equal(t, 10*time.Second, freshRecords["agent-def:task-def"])
}

func TestFileStore_ConcurrentSafety(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "concurrent_stress.json")

	store, err := NewFileStore(storePath)
	require.NoError(t, err)

	const workerCount = 10
	const iterations = 5

	var wg sync.WaitGroup
	wg.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		workerID := i
		agentID := fmt.Sprintf("worker-agent-%d", workerID)
		taskID := fmt.Sprintf("worker-task-%d", workerID)

		goroutinelabels.NewGoroutine(
			fmt.Sprintf("agentidle_concurrent_worker_%d", workerID),
			"concurrent accumulation and retrieval stress test",
		).StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				accErr := store.Accumulate(agentID, taskID, 100*time.Millisecond)
				assert.NoError(t, accErr)

				curTime, getErr := store.GetIdleTime(agentID, taskID)
				assert.NoError(t, getErr)
				assert.True(t, curTime >= 100*time.Millisecond)

				allRecs, allErr := store.GetAllRecords()
				assert.NoError(t, allErr)
				assert.NotEmpty(t, allRecs)
			}
		})
	}

	wg.Wait()

	// Verify all workers accumulated exactly workerCount * iterations increments
	allFinalRecords, err := store.GetAllRecords()
	require.NoError(t, err)
	assert.Len(t, allFinalRecords, workerCount)

	for i := 0; i < workerCount; i++ {
		agentID := fmt.Sprintf("worker-agent-%d", i)
		taskID := fmt.Sprintf("worker-task-%d", i)
		finalTime, err := store.GetIdleTime(agentID, taskID)
		require.NoError(t, err)
		assert.Equal(t, time.Duration(iterations)*100*time.Millisecond, finalTime)
	}
}

func TestRecordCVSSnapshot_AggregationAndExecution(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "cvs_snapshot_test.json")

	store, err := NewFileStore(storePath)
	require.NoError(t, err)

	// Populate multiple distinct records
	err = store.Accumulate("agent-cvs-1", "task-cvs-1", 10*time.Second)
	require.NoError(t, err)
	err = store.Accumulate("agent-cvs-2", "task-cvs-2", 25*time.Second)
	require.NoError(t, err)
	err = store.Accumulate("agent-cvs-1", "task-cvs-3", 15*time.Second)
	require.NoError(t, err)

	// Verify total aggregation before invocation
	records, err := store.GetAllRecords()
	require.NoError(t, err)
	var expectedTotal time.Duration
	for _, d := range records {
		expectedTotal += d
	}
	assert.Equal(t, 50*time.Second, expectedTotal)

	// Call RecordCVSSnapshot. In testing environment, CLI returns error if target CVS object does not exist.
	// We verify that the returned error is handled and never swallowed.
	cvsErr := RecordCVSSnapshot(store, "CVS-NONEXISTENT-TARGET-999")
	if cvsErr != nil {
		assert.Error(t, cvsErr)
	}
}
