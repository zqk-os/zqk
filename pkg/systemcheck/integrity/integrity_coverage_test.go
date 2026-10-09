package integrity

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/kernelcas"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// errorIntegrityStorage mocks storage that returns errors for List and Exists.
type errorIntegrityStorage struct {
	storage.ObjectStorageProvider
	failList   bool
	failExists bool
}

func (e *errorIntegrityStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if e.failList {
		return nil, fmt.Errorf("list failed")
	}
	return &storage.QueryResult{}, nil
}

func (e *errorIntegrityStorage) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	if e.failExists {
		return false, fmt.Errorf("exists failed")
	}
	return false, nil
}

func TestBuildKernelIntegrityReport_Comprehensive(t *testing.T) {
	ctx := context.Background()
	sec := &pkgctx.SecurityContext{}

	// Case 1: Empty project root, zero dangling refs, no cache
	tmpDir1 := t.TempDir()
	store1 := &mockIntegrityStorage{
		objectsByKind: make(map[string][]map[string]any),
		existingIDs:   make(map[string]bool),
	}

	report1, err := BuildKernelIntegrityReport(ctx, tmpDir1, store1, sec)
	require.NoError(t, err)
	assert.True(t, report1.PipelineKindsOK)
	assert.Equal(t, 0, report1.DanglingRefCount)
	assert.True(t, report1.DanglingOK)
	assert.False(t, report1.ObjectCompliance.Available)
	assert.Equal(t, "membrane_only_no_check_cache", report1.KernelHealthyScope)
	assert.NotEmpty(t, report1.LifecycleMissing)

	// Case 2: Project root with lifecycle files, cache, and > 25 dangling references
	tmpDir2 := t.TempDir()
	lcDir := filepath.Join(tmpDir2, paths.ProcessDir, "_internal", "lifecycles")
	require.NoError(t, fileutil.MkdirAll(lcDir, paths.DirPerm755))

	// Write lifecycle files for all critical kinds
	for _, ck := range kernelcas.ListCriticalKinds() {
		lcPath := filepath.Join(lcDir, ck+"_lifecycle.yaml")
		require.NoError(t, fileutil.WriteFile(lcPath, []byte("status_transitions: []\n"), paths.FilePerm644))
	}

	// Create > 25 dangling hits to test DanglingSample truncation
	danglingObjs := make([]map[string]any, 0, 30)
	for i := 0; i < 30; i++ {
		danglingObjs = append(danglingObjs, map[string]any{
			objects.FieldKeyID: fmt.Sprintf("GOAL-%d", i),
			"goal_ref":         fmt.Sprintf("MISSING-%d", i),
		})
	}
	store2 := &mockIntegrityStorage{
		objectsByKind: map[string][]map[string]any{
			objects.KindGoal: danglingObjs,
		},
		existingIDs: make(map[string]bool),
	}

	// Add object compliance check cache
	precommitDir := filepath.Join(tmpDir2, paths.ProjectDataDir, paths.PreCommitDir)
	require.NoError(t, fileutil.MkdirAll(precommitDir, paths.DirPerm755))
	checkFile := filepath.Join(precommitDir, "system-check.json")
	var summary CheckSummaryFile
	summary.Summary.TotalObjects = 100
	summary.Summary.TotalIssues = 0
	summary.Summary.BlockingIssues = 0
	summary.Summary.PendingAutofixBatches = 0
	checkBytes, err := json.Marshal(summary)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(checkFile, checkBytes, paths.FilePerm644))

	report2, err := BuildKernelIntegrityReport(ctx, tmpDir2, store2, sec)
	require.NoError(t, err)
	assert.Equal(t, 30, report2.DanglingRefCount)
	assert.False(t, report2.DanglingOK)
	assert.Len(t, report2.DanglingSample, 25)
	assert.True(t, report2.ObjectCompliance.Available)
	assert.Equal(t, "membrane_and_instances", report2.KernelHealthyScope)
	assert.Greater(t, report2.LifecyclePresent, 0)
}

func TestLegacyCustomRulesGate_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Clean without Go validation directory -> binary distribution
	ok, note := LegacyCustomRulesGate(tmpDir)
	assert.True(t, ok)
	assert.Contains(t, note, "binary distribution")

	// 2. pkg/validation/go_validator_custom.go exists without legacy map
	validationDir := filepath.Join(tmpDir, "pkg", "validation")
	require.NoError(t, fileutil.MkdirAll(validationDir, paths.DirPerm755))
	customGo := filepath.Join(validationDir, "go_validator_custom.go")
	require.NoError(t, fileutil.WriteFile(customGo, []byte("package validation\n// modern code"), paths.FilePerm644))

	ok, note = LegacyCustomRulesGate(tmpDir)
	assert.True(t, ok)
	assert.Contains(t, note, "composed overlays only")

	// 3. pkg/validation/go_validator_custom.go contains customRuleValidators
	require.NoError(t, fileutil.WriteFile(customGo, []byte("package validation\nvar customRuleValidators = map[string]any{}\n"), paths.FilePerm644))
	ok, note = LegacyCustomRulesGate(tmpDir)
	assert.False(t, ok)
	assert.Contains(t, note, "legacy customRuleValidators map present")

	// 4. pkg/validation/go_validator_custom_rules.go exists -> forbidden
	rulesGo := filepath.Join(validationDir, "go_validator_custom_rules.go")
	require.NoError(t, fileutil.WriteFile(rulesGo, []byte("package validation"), paths.FilePerm644))
	ok, note = LegacyCustomRulesGate(tmpDir)
	assert.False(t, ok)
	assert.Contains(t, note, "must stay deleted")
}

func TestMembraneCoverageGate_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()

	pathsToCheck := []string{
		filepath.Join("pkg", "storage", "object_storage_file_create.go"),
		filepath.Join("pkg", "storage", "object_storage_file_update.go"),
		filepath.Join("pkg", "storage", "object_storage_file_delete.go"),
		filepath.Join("pkg", "storage", "object_storage_file_transaction.go"),
		filepath.Join("pkg", "storage", "object_storage_graph_crud.go"),
		filepath.Join("cmd", "zqk", "system", "state_restore.go"),
		filepath.Join("cmd", "zqk", "system", "check_impl_output.go"),
	}

	// 1. Missing first file -> returns true, nil
	ok, gaps := MembraneCoverageGate(tmpDir)
	assert.True(t, ok)
	assert.Empty(t, gaps)

	// 2. Create first file, but leave others missing
	firstFile := filepath.Join(tmpDir, pathsToCheck[0])
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(firstFile), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(firstFile, []byte("package storage\n// kernelcas.Run\n"), paths.FilePerm644))

	ok, gaps = MembraneCoverageGate(tmpDir)
	assert.False(t, ok)
	assert.Len(t, gaps, len(pathsToCheck)-1)

	// 3. Create all files, but one file lacks needles
	for _, rel := range pathsToCheck[1:] {
		full := filepath.Join(tmpDir, rel)
		require.NoError(t, fileutil.MkdirAll(filepath.Dir(full), paths.DirPerm755))
		require.NoError(t, fileutil.WriteFile(full, []byte("package test\n// empty\n"), paths.FilePerm644))
	}
	ok, gaps = MembraneCoverageGate(tmpDir)
	assert.False(t, ok)
	assert.NotEmpty(t, gaps)

	// 4. Create all files with needle
	for _, rel := range pathsToCheck {
		full := filepath.Join(tmpDir, rel)
		require.NoError(t, fileutil.WriteFile(full, []byte("package test\n// kernelcas.Run\n"), paths.FilePerm644))
	}
	ok, gaps = MembraneCoverageGate(tmpDir)
	assert.True(t, ok)
	assert.Empty(t, gaps)
}

func TestCriticalKindList_Comprehensive(t *testing.T) {
	kinds := CriticalKindList()
	assert.NotEmpty(t, kinds)
	assert.Contains(t, kinds, objects.KindGoal)
	assert.Contains(t, kinds, objects.KindBacklogItem)
}

func TestScanDanglingRefs_Comprehensive(t *testing.T) {
	ctx := context.Background()
	sec := &pkgctx.SecurityContext{}

	// 1. Error in store.List
	errStore := &errorIntegrityStorage{failList: true}
	hits := ScanDanglingRefs(ctx, sec, errStore, 0)
	assert.Empty(t, hits)

	// 2. Store with objects having empty ID, non-ref fields, and existing refs
	mockStore := &mockIntegrityStorage{
		objectsByKind: map[string][]map[string]any{
			objects.KindGoal: {
				{
					objects.FieldKeyID: "", // empty id should be skipped
					"goal_ref":         "SOME-REF",
				},
				{
					objects.FieldKeyID: "GOAL-1",
					"title":            "Regular Title", // non-ref field should be skipped
					"goal_ref":         "GOAL-TARGET",   // exists in store
					"dependencies":     []string{"DEP-MISSING-1", "DEP-MISSING-2"},
				},
			},
		},
		existingIDs: map[string]bool{
			"GOAL-TARGET": true,
		},
	}

	// Scan with sampleLimit = 1
	limitedHits := ScanDanglingRefs(ctx, sec, mockStore, 1)
	assert.Len(t, limitedHits, 1)
	assert.Equal(t, "GOAL-1", limitedHits[0].ObjectID)
	assert.Equal(t, "dependencies", limitedHits[0].Field)

	// Scan with unlimited sample
	allHits := ScanDanglingRefs(ctx, sec, mockStore, 0)
	assert.Len(t, allHits, 2)
}

func TestBuildUnlinkUpdates_Comprehensive(t *testing.T) {
	missing := map[string]struct{}{
		"TARGET-1": {},
		"TARGET-2": {},
	}

	// 1. Missing field in object
	obj := map[string]any{"other_field": "val"}
	_, err := BuildUnlinkUpdates(obj, "missing_field", missing)
	require.Error(t, err)

	// 2. Single ref (`_ref` and not `_refs`)
	// a. Target is missing -> returns FieldUnset
	singleObj := map[string]any{"parent_ref": "TARGET-1"}
	updates, err := BuildUnlinkUpdates(singleObj, "parent_ref", missing)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"parent_ref": storage.FieldUnset}, updates)

	// b. Target is NOT missing -> returns nil, nil
	presentObj := map[string]any{"parent_ref": "TARGET-EXISTS"}
	updates, err = BuildUnlinkUpdates(presentObj, "parent_ref", missing)
	require.NoError(t, err)
	assert.Nil(t, updates)

	// c. Single ref value is not string -> returns nil, nil
	nonStringObj := map[string]any{"parent_ref": 12345}
	updates, err = BuildUnlinkUpdates(nonStringObj, "parent_ref", missing)
	require.NoError(t, err)
	assert.Nil(t, updates)

	// 3. Multi ref (`_refs`)
	// a. All targets missing -> FieldUnset
	allMissingObj := map[string]any{
		"criteria_refs": []string{"TARGET-1", "TARGET-2"},
	}
	updates, err = BuildUnlinkUpdates(allMissingObj, "criteria_refs", missing)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"criteria_refs": storage.FieldUnset}, updates)

	// b. Some targets kept -> returns kept slice
	mixedObj := map[string]any{
		"criteria_refs": []string{"TARGET-1", "TARGET-KEEP"},
	}
	updates, err = BuildUnlinkUpdates(mixedObj, "criteria_refs", missing)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"criteria_refs": []string{"TARGET-KEEP"}}, updates)
}

func TestObjectCompliance_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Multiple summary candidates with mtimes and corrupt files
	precommitDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.PreCommitDir)
	logsDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.LogsDir)
	require.NoError(t, fileutil.MkdirAll(precommitDir, paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(logsDir, paths.DirPerm755))

	// Write empty/stub summary file (TotalObjects == 0) -> should be skipped
	stubFile := filepath.Join(precommitDir, "system-check.json")
	var stub CheckSummaryFile
	stub.Summary.TotalObjects = 0
	stubBytes, _ := json.Marshal(stub)
	require.NoError(t, fileutil.WriteFile(stubFile, stubBytes, paths.FilePerm644))

	// Write corrupt JSON to logsDir -> should be skipped
	corruptFile := filepath.Join(logsDir, "system-check-autofix-dangling.json")
	require.NoError(t, fileutil.WriteFile(corruptFile, []byte("{corrupt json"), paths.FilePerm644))

	// Write valid older file in logsDir
	validFile1 := filepath.Join(logsDir, "system-check.json")
	var valid1 CheckSummaryFile
	valid1.Summary.TotalObjects = 20
	valid1.Summary.TotalIssues = 5
	valid1.Summary.BlockingIssues = 1
	validBytes1, _ := json.Marshal(valid1)
	require.NoError(t, fileutil.WriteFile(validFile1, validBytes1, paths.FilePerm644))
	t1 := time.Now().Add(-1 * time.Hour)
	_ = fileutil.Chtimes(validFile1, t1, t1)

	// Snapshot should pick validFile1 despite stub in precommitDir
	snap := LoadObjectComplianceSnapshot(tmpDir)
	assert.True(t, snap.Available)
	assert.Equal(t, 20, snap.TotalObjects)
	assert.False(t, snap.ObjectComplianceOK) // blocking issues = 1

	// 2. ReadObjectComplianceHistoryTail edge cases
	histPath := KernelObjectComplianceHistoryPath(tmpDir)
	// Write history with blank lines and corrupt lines
	histContent := "\n\n{\"total_issues\": 5}\nnot json\n{\"total_issues\": 3}\n"
	require.NoError(t, fileutil.WriteFile(histPath, []byte(histContent), paths.FilePerm644))

	tail := ReadObjectComplianceHistoryTail(histPath, 10)
	assert.Len(t, tail, 2)

	// Nonexistent history tail
	assert.Nil(t, ReadObjectComplianceHistoryTail(filepath.Join(tmpDir, "missing.jsonl"), 5))

	// Penultimate with < 2 lines
	shortHist := filepath.Join(tmpDir, "short.jsonl")
	require.NoError(t, fileutil.WriteFile(shortHist, []byte("{\"total_issues\": 1}\n"), paths.FilePerm644))
	assert.Nil(t, ReadPenultimateObjectComplianceHistory(shortHist))

	// 3. AppendObjectComplianceHistory directory error
	conflictFile := filepath.Join(tmpDir, "conflict")
	require.NoError(t, fileutil.WriteFile(conflictFile, []byte("conflict"), paths.FilePerm644))
	err := AppendObjectComplianceHistory(conflictFile, ObjectComplianceHistoryEntry{
		TotalIssues: 1,
	})
	require.Error(t, err)
}
