// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package rollback

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type cycle202MockStorage struct {
	mockRollbackStorage
	readFn   func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
	updateFn func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error
	listFn   func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error)
}

func (m *cycle202MockStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if m.readFn != nil {
		return m.readFn(ctx, secCtx, id)
	}
	return m.mockRollbackStorage.Read(ctx, secCtx, id)
}

func (m *cycle202MockStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, secCtx, id, updates)
	}
	return m.mockRollbackStorage.Update(ctx, secCtx, id, updates)
}

func (m *cycle202MockStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if m.listFn != nil {
		return m.listFn(ctx, secCtx, storageCtx, filter)
	}
	return m.mockRollbackStorage.List(ctx, secCtx, storageCtx, filter)
}

func TestCycle202_DefaultConfig_Variations(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if cfg.RetainCount <= 0 {
		t.Errorf("expected positive RetainCount, got %d", cfg.RetainCount)
	}
	if cfg.RetainDuration <= 0 {
		t.Errorf("expected positive RetainDuration, got %v", cfg.RetainDuration)
	}
}

func TestCycle202_SnapshotObjectStates_NilContextAndErrors(t *testing.T) {
	t.Parallel()
	mock := &cycle202MockStorage{
		mockRollbackStorage: mockRollbackStorage{
			objects: map[string]map[string]any{
				"item-1": {objects.FieldKeyID: "item-1", objects.FieldKeyKind: "backlog_item", objects.FieldKeyStatus: "ready"},
			},
		},
	}

	// 1. ctx == nil branch
	states, err := SnapshotObjectStates(nil, mock, []ObjectRef{{Kind: "backlog_item", ID: "item-1"}})
	if err != nil {
		t.Fatalf("unexpected error with nil context: %v", err)
	}
	if len(states) != 1 || states[0].ID != "item-1" {
		t.Fatalf("unexpected states: %+v", states)
	}

	// 2. Read error branch
	readErr := errors.New("storage read failure")
	errMock := &cycle202MockStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return nil, readErr
		},
	}
	statesErr, err := SnapshotObjectStates(context.Background(), errMock, []ObjectRef{{Kind: "backlog_item", ID: "item-1"}})
	if err == nil {
		t.Fatal("expected error from SnapshotObjectStates when Read fails, got nil")
	}
	if !errors.Is(err, readErr) {
		t.Fatalf("expected readErr, got %v", err)
	}
	if statesErr != nil {
		t.Fatalf("expected nil states on error, got %+v", statesErr)
	}

	// 3. Provider nil branch
	nilProvStates, err := SnapshotObjectStates(context.Background(), nil, []ObjectRef{{Kind: "backlog_item", ID: "item-1"}})
	if err != nil || nilProvStates != nil {
		t.Fatalf("expected nil states and nil error for nil provider, got %v, %v", nilProvStates, err)
	}

	// 4. Empty refs branch
	emptyRefsStates, err := SnapshotObjectStates(context.Background(), mock, nil)
	if err != nil || emptyRefsStates != nil {
		t.Fatalf("expected nil states and nil error for empty refs, got %v, %v", emptyRefsStates, err)
	}
}

func TestCycle202_NewStore_ErrorConditions(t *testing.T) {
	t.Parallel()
	// Empty project root
	store, err := NewStore("")
	if err == nil || store != nil {
		t.Fatalf("expected error for empty project root, got store=%v, err=%v", store, err)
	}

	// MkdirAll failure: create regular file at directory target
	tmpDir := t.TempDir()
	blockingFilePath := filepath.Join(tmpDir, paths.ProjectDataDir)
	writeErr := fileutil.WriteFile(blockingFilePath, []byte("blocker"), paths.FilePerm644)
	if writeErr != nil {
		t.Fatalf("failed to create blocking file: %v", writeErr)
	}

	store, err = NewStore(tmpDir)
	if err == nil || store != nil {
		t.Fatalf("expected error when directory creation fails, got store=%v, err=%v", store, err)
	}
}

func TestCycle202_Store_Append_BoundaryConditions(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// 1. JSON Marshal error: unsupported type in Metadata
	invalidPoint := &RollbackPoint{
		ID:        "rb-marshal-fail",
		Timestamp: time.Now().UTC(),
		ScopeType: ScopeTypeMaintenance,
		ScopeID:   "task-1",
		Metadata: map[string]any{
			"unmarshalable": make(chan int),
		},
	}
	err = store.Append(invalidPoint)
	if err == nil {
		t.Fatal("expected Append to fail with JSON marshal error, got nil")
	}

	// 2. Line too large error: payload exceeding maxRollbackLineSize
	oversizedState := strings.Repeat("A", maxRollbackLineSize+1024)
	oversizedPoint := &RollbackPoint{
		ID:        "rb-oversized",
		Timestamp: time.Now().UTC(),
		ScopeType: ScopeTypeLifecycle,
		ScopeID:   "scope-1",
		ObjectStates: []ObjectState{
			{Kind: "backlog_item", ID: "item-big", State: map[string]any{"data": oversizedState}},
		},
	}
	err = store.Append(oversizedPoint)
	if err == nil {
		t.Fatal("expected Append to fail with line too large error, got nil")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected 'too large' in error, got: %v", err)
	}

	// 3. OpenFile failure: path is a directory
	storeDirTmp := t.TempDir()
	brokenStore, err := NewStore(storeDirTmp)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	mkdirErr := fileutil.MkdirAll(brokenStore.path, storeDirPerm)
	if mkdirErr != nil {
		t.Fatalf("failed to create directory at store file path: %v", mkdirErr)
	}
	validPoint := &RollbackPoint{
		ID:        "rb-valid-1",
		Timestamp: time.Now().UTC(),
		ScopeType: ScopeTypeLifecycle,
		ScopeID:   "scope-1",
	}
	err = brokenStore.Append(validPoint)
	if err == nil {
		t.Fatal("expected Append to fail when store path is a directory, got nil")
	}
}

func TestCycle202_Store_List_CorruptedLines(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	p1 := &RollbackPoint{
		ID:        "rb-valid-1",
		Timestamp: time.Now().UTC(),
		ScopeType: ScopeTypeLifecycle,
		ScopeID:   "scope-1",
		ObjectStates: []ObjectState{
			{Kind: "goal", ID: "G-1", State: map[string]any{"status": "in_progress"}},
		},
	}
	err = store.Append(p1)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Append a corrupted line directly to the WAL file
	appendCorruptedLine(t, store.path, "NOT_VALID_JSON_LINE\n")

	p2 := &RollbackPoint{
		ID:        "rb-valid-2",
		Timestamp: time.Now().UTC().Add(time.Second),
		ScopeType: ScopeTypeMaintenance,
		ScopeID:   "scope-2",
	}
	err = store.Append(p2)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	metas, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(metas) != 2 {
		t.Fatalf("expected 2 valid metas (skipping corrupted line), got %d: %+v", len(metas), metas)
	}
	if metas[0].ID != "rb-valid-1" || metas[1].ID != "rb-valid-2" {
		t.Fatalf("unexpected metas returned: %+v", metas)
	}

	// Test store.List error when store.path is an unreadable directory
	brokenDir := t.TempDir()
	brokenStore, err := NewStore(brokenDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	err = fileutil.MkdirAll(brokenStore.path, storeDirPerm)
	if err != nil {
		t.Fatalf("MkdirAll for brokenStore path failed: %v", err)
	}
	_, err = brokenStore.List()
	if err == nil {
		t.Fatal("expected error from store.List when path is a directory, got nil")
	}

	// Test rollback.List error when store.List fails
	_, err = List(brokenDir, 10, 0)
	if err == nil {
		t.Fatal("expected error from rollback.List when store.List fails, got nil")
	}
}

func appendCorruptedLine(t *testing.T, filePath, content string) {
	t.Helper()
	f, err := fileutil.OpenFile(filePath, fileutil.O_WRONLY|fileutil.O_APPEND, storeFilePerm)
	if err != nil {
		t.Fatalf("OpenFile for corrupt injection failed: %v", err)
	}
	defer func() {
		closeErr := f.Close()
		if closeErr != nil {
			t.Fatalf("failed to close file: %v", closeErr)
		}
	}()
	_, writeErr := f.Write([]byte(content))
	if writeErr != nil {
		t.Fatalf("failed to write corrupt line: %v", writeErr)
	}
	syncErr := f.Sync()
	if syncErr != nil {
		t.Fatalf("failed to sync corrupt line: %v", syncErr)
	}
}

func TestCycle202_Store_Get_EdgeCases(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	p1 := &RollbackPoint{
		ID:        "rb-get-1",
		Timestamp: time.Now().UTC().Add(-2 * time.Minute),
		ScopeType: ScopeTypeLifecycle,
		ScopeID:   "scope-1",
	}
	err = store.Append(p1)
	if err != nil {
		t.Fatalf("Append p1 failed: %v", err)
	}

	appendCorruptedLine(t, store.path, "{invalid_json_line\n")

	p2 := &RollbackPoint{
		ID:        "rb-get-2",
		Timestamp: time.Now().UTC().Add(-1 * time.Minute),
		ScopeType: ScopeTypeMigration,
		ScopeID:   "scope-2",
	}
	err = store.Append(p2)
	if err != nil {
		t.Fatalf("Append p2 failed: %v", err)
	}

	p3 := &RollbackPoint{
		ID:        "rb-get-3",
		Timestamp: time.Now().UTC(),
		ScopeType: ScopeTypeMaintenance,
		ScopeID:   "scope-3",
	}
	err = store.Append(p3)
	if err != nil {
		t.Fatalf("Append p3 failed: %v", err)
	}

	// Test getting p1 (exercises scan continuation with out != nil for subsequent lines)
	got1, err := store.Get("rb-get-1")
	if err != nil || got1 == nil || got1.ID != "rb-get-1" {
		t.Fatalf("failed to get p1: got=%v, err=%v", got1, err)
	}

	// Test getting p2 across corrupted line
	got2, err := store.Get("rb-get-2")
	if err != nil || got2 == nil || got2.ID != "rb-get-2" {
		t.Fatalf("failed to get p2: got=%v, err=%v", got2, err)
	}

	// Test missing id
	missing, err := store.Get("rb-nonexistent")
	if err != nil || missing != nil {
		t.Fatalf("expected nil, nil for missing point, got=%v, err=%v", missing, err)
	}

	// Test store.Get error when path is a directory
	brokenDir := t.TempDir()
	brokenStore, err := NewStore(brokenDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	err = fileutil.MkdirAll(brokenStore.path, storeDirPerm)
	if err != nil {
		t.Fatalf("MkdirAll for brokenStore path failed: %v", err)
	}
	_, err = brokenStore.Get("rb-any")
	if err == nil {
		t.Fatal("expected error from store.Get when path is a directory, got nil")
	}
}

func TestCycle202_Store_Retain_DurationAndCountPruning(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// 1. Non-existent file retain returns nil
	err = store.Retain(10, time.Hour)
	if err != nil {
		t.Fatalf("Retain on non-existent file failed: %v", err)
	}

	// 2. Append points with different timestamps
	pOld := &RollbackPoint{
		ID:        "rb-old",
		Timestamp: time.Now().UTC().Add(-48 * time.Hour),
		ScopeType: ScopeTypeLifecycle,
		ScopeID:   "s-old",
	}
	err = store.Append(pOld)
	if err != nil {
		t.Fatalf("Append pOld failed: %v", err)
	}

	appendCorruptedLine(t, store.path, "corrupt_line_to_skip\n")

	pRecent1 := &RollbackPoint{
		ID:        "rb-recent-1",
		Timestamp: time.Now().UTC().Add(-10 * time.Minute),
		ScopeType: ScopeTypeLifecycle,
		ScopeID:   "s-1",
	}
	err = store.Append(pRecent1)
	if err != nil {
		t.Fatalf("Append pRecent1 failed: %v", err)
	}

	pRecent2 := &RollbackPoint{
		ID:        "rb-recent-2",
		Timestamp: time.Now().UTC().Add(-5 * time.Minute),
		ScopeType: ScopeTypeLifecycle,
		ScopeID:   "s-2",
	}
	err = store.Append(pRecent2)
	if err != nil {
		t.Fatalf("Append pRecent2 failed: %v", err)
	}

	// 3. Retain keeping all remaining (len(keep) >= len(all))
	err = store.Retain(10, 72*time.Hour)
	if err != nil {
		t.Fatalf("Retain with no-op pruning failed: %v", err)
	}

	// 4. Retain with duration cutoff (pruning pOld)
	err = store.Retain(10, 1*time.Hour)
	if err != nil {
		t.Fatalf("Retain with duration cutoff failed: %v", err)
	}
	metasAfterDuration, err := store.List()
	if err != nil {
		t.Fatalf("List after retain failed: %v", err)
	}
	if len(metasAfterDuration) != 2 {
		t.Fatalf("expected 2 points remaining after duration cutoff, got %d", len(metasAfterDuration))
	}

	// 5. Retain with keepCount pruning (pruning to 1 point)
	err = store.Retain(1, 1*time.Hour)
	if err != nil {
		t.Fatalf("Retain with keepCount failed: %v", err)
	}
	metasAfterCount, err := store.List()
	if err != nil {
		t.Fatalf("List after count retain failed: %v", err)
	}
	if len(metasAfterCount) != 1 || metasAfterCount[0].ID != "rb-recent-2" {
		t.Fatalf("expected only rb-recent-2 to remain, got %+v", metasAfterCount)
	}
}

func TestCycle202_Store_Retain_ErrorConditions(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Retain error when store path has unreadable permissions (not IsNotExist error)
	unreadableFile := filepath.Join(tmpDir, "unreadable_file")
	err := fileutil.WriteFile(unreadableFile, []byte("unreadable"), 0000)
	if err != nil {
		t.Fatalf("WriteFile unreadable failed: %v", err)
	}
	unreadableStore := &Store{path: unreadableFile, projectRoot: tmpDir}
	err = unreadableStore.Retain(5, time.Hour)
	if err == nil {
		t.Fatal("expected error when Retain opens an unreadable file, got nil")
	}

	// Retain error when tmpPath creation fails
	testRetainTmpCreateFailure(t)
}

func testRetainTmpCreateFailure(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	st, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	p1 := &RollbackPoint{ID: "p1", Timestamp: time.Now().UTC().Add(-10 * time.Minute)}
	p2 := &RollbackPoint{ID: "p2", Timestamp: time.Now().UTC()}
	err = st.Append(p1)
	if err != nil {
		t.Fatalf("Append p1 failed: %v", err)
	}
	err = st.Append(p2)
	if err != nil {
		t.Fatalf("Append p2 failed: %v", err)
	}

	// Create tmpPath as a directory so fileutil.Create(tmpPath) fails
	tmpPath := st.path + tmpFileSuffix
	err = fileutil.MkdirAll(tmpPath, storeDirPerm)
	if err != nil {
		t.Fatalf("MkdirAll for tmpPath failed: %v", err)
	}

	err = st.Retain(1, time.Hour)
	if err == nil {
		t.Fatal("expected error when tmpPath creation fails, got nil")
	}
}

func TestCycle202_Capture_ComprehensiveFlows(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// 1. getStates returns error
	getErr := errors.New("state retrieval failure")
	id, err := Capture(tmpDir, ScopeTypeLifecycle, "s-1", func() ([]ObjectState, error) {
		return nil, getErr
	})
	if err == nil || id != "" {
		t.Fatalf("expected wrapped error and empty id, got id=%q, err=%v", id, err)
	}
	if !strings.Contains(err.Error(), "rollback capture: get states") {
		t.Fatalf("expected wrap message, got: %v", err)
	}

	// 2. getStates returns empty slice
	id, err = Capture(tmpDir, ScopeTypeLifecycle, "s-2", func() ([]ObjectState, error) {
		return []ObjectState{}, nil
	})
	if err != nil || id != "" {
		t.Fatalf("expected nil error and empty id for empty states, got id=%q, err=%v", id, err)
	}

	// 3. Capture with invalid project root
	id, err = Capture("", ScopeTypeLifecycle, "s-3", func() ([]ObjectState, error) {
		return []ObjectState{{Kind: "goal", ID: "G-1"}}, nil
	})
	if err == nil || id != "" {
		t.Fatalf("expected error for empty project root, got id=%q, err=%v", id, err)
	}

	// 4. Capture with unwritable store directory
	blockingDir := t.TempDir()
	st, err := NewStore(blockingDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	err = fileutil.MkdirAll(st.path, storeDirPerm)
	if err != nil {
		t.Fatalf("failed to make store path a directory: %v", err)
	}
	id, err = Capture(blockingDir, ScopeTypeLifecycle, "s-4", func() ([]ObjectState, error) {
		return []ObjectState{{Kind: "goal", ID: "G-1"}}, nil
	})
	if err == nil || id != "" {
		t.Fatalf("expected error when append fails, got id=%q, err=%v", id, err)
	}

	// 5. Successful capture
	validStates := []ObjectState{
		{Kind: "goal", ID: "G-1", State: map[string]any{"status": "in_progress"}},
		{Kind: "requirement", ID: "REQ-1", State: map[string]any{"status": "active"}},
	}
	id, err = Capture(tmpDir, ScopeTypeLifecycle, "s-5", func() ([]ObjectState, error) {
		return validStates, nil
	})
	if err != nil {
		t.Fatalf("Capture failed: %v", err)
	}
	if !strings.HasPrefix(id, "rb-") {
		t.Fatalf("expected point ID prefix 'rb-', got %q", id)
	}

	point, err := Get(tmpDir, id)
	if err != nil || point == nil {
		t.Fatalf("Get after capture failed: point=%v, err=%v", point, err)
	}
	if len(point.ObjectStates) != 2 {
		t.Fatalf("expected 2 object states captured, got %d", len(point.ObjectStates))
	}
}

func TestCycle202_List_ComprehensiveFiltering(t *testing.T) {
	t.Parallel()
	// 1. Invalid project root
	metas, err := List("", 10, 0)
	if err == nil || metas != nil {
		t.Fatalf("expected error for empty project root, got metas=%v, err=%v", metas, err)
	}

	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Populate 5 points with staggered timestamps
	baseTime := time.Now().UTC()
	for i := 0; i < 5; i++ {
		p := &RollbackPoint{
			ID:        fmt.Sprintf("rb-filter-%d", i),
			Timestamp: baseTime.Add(-time.Duration(5-i) * time.Hour),
			ScopeType: ScopeTypeLifecycle,
			ScopeID:   fmt.Sprintf("s-%d", i),
			ObjectStates: []ObjectState{
				{Kind: "goal", ID: fmt.Sprintf("G-%d", i)},
			},
		}
		err = store.Append(p)
		if err != nil {
			t.Fatalf("Append %d failed: %v", i, err)
		}
	}

	// Filter with lastN=2 (triggers len(metas) > lastN)
	metasLast2, err := List(tmpDir, 2, 0)
	if err != nil {
		t.Fatalf("List with lastN failed: %v", err)
	}
	if len(metasLast2) != 2 {
		t.Fatalf("expected 2 metas, got %d", len(metasLast2))
	}
	if metasLast2[0].ID != "rb-filter-3" || metasLast2[1].ID != "rb-filter-4" {
		t.Fatalf("unexpected lastN slice: %+v", metasLast2)
	}

	// Filter with withinDuration (last 2.5 hours)
	metasWithin, err := List(tmpDir, 0, 150*time.Minute)
	if err != nil {
		t.Fatalf("List with withinDuration failed: %v", err)
	}
	if len(metasWithin) != 2 {
		t.Fatalf("expected 2 metas within 150m, got %d: %+v", len(metasWithin), metasWithin)
	}

	// Filter combined: lastN and withinDuration
	metasCombined, err := List(tmpDir, 1, 150*time.Minute)
	if err != nil {
		t.Fatalf("List combined failed: %v", err)
	}
	if len(metasCombined) != 1 || metasCombined[0].ID != "rb-filter-4" {
		t.Fatalf("expected 1 meta (rb-filter-4), got %+v", metasCombined)
	}
}

func TestCycle202_Get_EdgeCases(t *testing.T) {
	t.Parallel()
	// Invalid project root
	point, err := Get("", "rb-1")
	if err == nil || point != nil {
		t.Fatalf("expected error for empty project root, got point=%v, err=%v", point, err)
	}

	tmpDir := t.TempDir()
	point, err = Get(tmpDir, "rb-missing")
	if err != nil || point != nil {
		t.Fatalf("expected nil, nil for missing point, got point=%v, err=%v", point, err)
	}
}

func TestCycle202_Apply_ComprehensivePathways(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	validPoint := &RollbackPoint{
		ID:        "rb-apply-target",
		Timestamp: time.Now().UTC(),
		ScopeType: ScopeTypeLifecycle,
		ScopeID:   "s-1",
		ObjectStates: []ObjectState{
			{Kind: "goal", ID: "G-1", State: map[string]any{"status": "in_progress", "title": "Main Goal"}},
			{Kind: "requirement", ID: "R-1", State: map[string]any{"status": "active", "title": "Main Req"}},
		},
	}
	err = store.Append(validPoint)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// 1. ctx == nil branch and successful apply
	appliedUpdates := make(map[string]map[string]any)
	successMock := &cycle202MockStorage{
		updateFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
			appliedUpdates[id] = updates
			return nil
		},
	}
	err = Apply(nil, tmpDir, "rb-apply-target", successMock)
	if err != nil {
		t.Fatalf("Apply with nil context failed: %v", err)
	}
	if len(appliedUpdates) != 2 {
		t.Fatalf("expected 2 applied updates, got %d", len(appliedUpdates))
	}
	if appliedUpdates["G-1"]["status"] != "in_progress" || appliedUpdates["R-1"]["status"] != "active" {
		t.Fatalf("unexpected applied updates: %+v", appliedUpdates)
	}

	// 2. Point not found in store
	err = Apply(context.Background(), tmpDir, "rb-nonexistent", successMock)
	if err == nil {
		t.Fatal("expected error for non-existent rollback point, got nil")
	}
	if !strings.Contains(err.Error(), "rollback point not found") {
		t.Fatalf("expected 'rollback point not found' in error, got: %v", err)
	}

	// 3. Store Get failure (empty project root)
	err = Apply(context.Background(), "", "rb-apply-target", successMock)
	if err == nil {
		t.Fatal("expected error for invalid project root in Apply, got nil")
	}

	// 4. Update failure in storage provider
	updateFailErr := errors.New("write-behind persistence rejected")
	failMock := &cycle202MockStorage{
		updateFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
			if id == "R-1" {
				return updateFailErr
			}
			return nil
		},
	}
	err = Apply(context.Background(), tmpDir, "rb-apply-target", failMock)
	if err == nil {
		t.Fatal("expected error when storage Update fails, got nil")
	}
	if !strings.Contains(err.Error(), "rollback apply requirement/R-1") {
		t.Fatalf("expected error message mentioning requirement/R-1, got: %v", err)
	}
}

func TestCycle202_ApplyReconstruct_ComprehensivePathways(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	targetTime := time.Now().UTC().Add(-1 * time.Hour)

	// 1. Provider Read error
	readErr := errors.New("cas read failure")
	readFailMock := &cycle202MockStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return nil, readErr
		},
	}
	err := ApplyReconstruct(context.Background(), tmpDir, targetTime, []ObjectRef{{Kind: "goal", ID: "G-1"}}, readFailMock, nil)
	if err == nil {
		t.Fatal("expected error when Read fails, got nil")
	}
	if !strings.Contains(err.Error(), "rollback reconstruct read goal/G-1") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 2. Provider Read returns nil, nil (object not found)
	notFoundMock := &cycle202MockStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return nil, nil
		},
	}
	err = ApplyReconstruct(nil, tmpDir, targetTime, []ObjectRef{{Kind: "goal", ID: "G-missing"}}, notFoundMock, nil)
	if err == nil {
		t.Fatal("expected error when Read returns nil, got nil")
	}
	if !strings.Contains(err.Error(), "object G-missing not found") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 3. ReconstructStateAtTimestamp error (List journal entries fails)
	listErr := errors.New("journal scan failure")
	listFailMock := &cycle202MockStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{"id": id, "kind": "goal", "status": "draft"}, nil
		},
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
			return nil, listErr
		},
	}
	err = ApplyReconstruct(context.Background(), tmpDir, targetTime, []ObjectRef{{Kind: "goal", ID: "G-1"}}, listFailMock, nil)
	if err == nil {
		t.Fatal("expected error when ReconstructStateAtTimestamp fails, got nil")
	}
	if !strings.Contains(err.Error(), "rollback reconstruct goal/G-1") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 4. Provider Update fails
	updateErr := errors.New("cas update conflict")
	updateFailMock := &cycle202MockStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{"id": id, "kind": "goal", "status": "draft"}, nil
		},
		updateFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
			return updateErr
		},
	}
	err = ApplyReconstruct(context.Background(), tmpDir, targetTime, []ObjectRef{{Kind: "goal", ID: "G-1"}}, updateFailMock, nil)
	if err == nil {
		t.Fatal("expected error when Update fails, got nil")
	}
	if !strings.Contains(err.Error(), "rollback reconstruct apply goal/G-1") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 5. Successful ApplyReconstruct across multiple objects
	testApplyReconstructSuccess(t, tmpDir, targetTime)
}

func testApplyReconstructSuccess(t *testing.T, tmpDir string, targetTime time.Time) {
	t.Helper()
	reconstructed := make(map[string]map[string]any)
	successMock := &cycle202MockStorage{
		readFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{"id": id, "kind": "goal", "status": "restored"}, nil
		},
		updateFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
			reconstructed[id] = updates
			return nil
		},
	}

	refs := []ObjectRef{
		{Kind: "goal", ID: "G-1"},
		{Kind: "", ID: "G-empty-kind"}, // should be skipped
		{Kind: "goal", ID: ""},         // should be skipped
		{Kind: "requirement", ID: "R-1"},
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	err := ApplyReconstruct(nil, tmpDir, targetTime, refs, successMock, logger)
	if err != nil {
		t.Fatalf("ApplyReconstruct success flow failed: %v", err)
	}
	if len(reconstructed) != 2 {
		t.Fatalf("expected 2 reconstructed objects, got %d: %+v", len(reconstructed), reconstructed)
	}
	if _, ok := reconstructed["G-1"]; !ok {
		t.Fatal("expected G-1 in reconstructed objects")
	}
	if _, ok := reconstructed["R-1"]; !ok {
		t.Fatal("expected R-1 in reconstructed objects")
	}
}

func TestCycle202_Retain_ComprehensiveFlows(t *testing.T) {
	t.Parallel()
	// 1. Invalid project root
	err := Retain("")
	if err == nil {
		t.Fatal("expected error for empty project root in Retain, got nil")
	}

	// 2. Successful retain on valid directory
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		p := &RollbackPoint{
			ID:        fmt.Sprintf("rb-retain-%d", i),
			Timestamp: time.Now().UTC(),
			ScopeType: ScopeTypeLifecycle,
			ScopeID:   "s-1",
		}
		err = store.Append(p)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	err = Retain(tmpDir)
	if err != nil {
		t.Fatalf("Retain failed: %v", err)
	}
}

func TestCycle202_Concurrency_ThreadSafety(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	const workerCount = 8
	const iterations = 15

	var wg sync.WaitGroup
	errCh := make(chan error, workerCount*iterations*2)

	// Appender goroutines
	for w := 0; w < workerCount/2; w++ {
		workerID := w
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_cycle202_appender", "concurrent snapshot appender").StartSimple(func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				p := &RollbackPoint{
					ID:        fmt.Sprintf("rb-conc-%d-%d", workerID, i),
					Timestamp: time.Now().UTC(),
					ScopeType: ScopeTypeLifecycle,
					ScopeID:   fmt.Sprintf("worker-%d", workerID),
					ObjectStates: []ObjectState{
						{Kind: "goal", ID: fmt.Sprintf("G-%d", workerID), State: map[string]any{"i": i}},
					},
				}
				appendErr := store.Append(p)
				if appendErr != nil {
					errCh <- errfmt.Errorf("concurrent append failed: %w", appendErr)
					return
				}
			}
		})
	}

	// Reader & Lister goroutines
	for w := workerCount / 2; w < workerCount; w++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_cycle202_reader", "concurrent snapshot reader").StartSimple(func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				metas, listErr := store.List()
				if listErr != nil {
					errCh <- errfmt.Errorf("concurrent list failed: %w", listErr)
					return
				}
				if len(metas) > 0 {
					_, getErr := store.Get(metas[0].ID)
					if getErr != nil {
						errCh <- errfmt.Errorf("concurrent get failed: %w", getErr)
						return
					}
				}
			}
		})
	}

	wg.Wait()
	close(errCh)

	for chErr := range errCh {
		t.Errorf("concurrency error: %v", chErr)
	}

	finalMetas, err := store.List()
	if err != nil {
		t.Fatalf("final List failed: %v", err)
	}
	expectedCount := (workerCount / 2) * iterations
	if len(finalMetas) != expectedCount {
		t.Fatalf("expected %d rollback points after concurrent appends, got %d", expectedCount, len(finalMetas))
	}
}
