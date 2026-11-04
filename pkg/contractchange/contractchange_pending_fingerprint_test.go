package contractchange

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type mockStorageProvider struct {
	storage.NoopObjectStorage
	listFn   func(ctx context.Context, sec *pkgctx.SecurityContext, sCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error)
	readFn   func(ctx context.Context, sec *pkgctx.SecurityContext, id string) (map[string]any, error)
	updateFn func(ctx context.Context, sec *pkgctx.SecurityContext, id string, updates map[string]any) error
}

func (m *mockStorageProvider) List(ctx context.Context, sec *pkgctx.SecurityContext, sCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if m.listFn != nil {
		return m.listFn(ctx, sec, sCtx, filter)
	}
	return m.NoopObjectStorage.List(ctx, sec, sCtx, filter)
}

func (m *mockStorageProvider) Read(ctx context.Context, sec *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if m.readFn != nil {
		return m.readFn(ctx, sec, id)
	}
	return m.NoopObjectStorage.Read(ctx, sec, id)
}

func (m *mockStorageProvider) Update(ctx context.Context, sec *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, sec, id, updates)
	}
	return m.NoopObjectStorage.Update(ctx, sec, id, updates)
}

func TestFingerprintLifecyclePreconditions_NestedAndErrors(t *testing.T) {
	t.Parallel()

	// 1. Nested lifecycle resolution via WalkDir
	root := t.TempDir()
	nestedDir := filepath.Join(root, paths.ProcessInternalLifecyclesDir, "subpack")
	if err := fileutil.MkdirAll(nestedDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create nested lifecycles dir: %v", err)
	}
	yamlBody := "object_type: custom_entity\nstatuses:\n  - value: active\n"
	yamlPath := filepath.Join(nestedDir, "custom_entity_lifecycle.yaml")
	if err := fileutil.WriteFile(yamlPath, []byte(yamlBody), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write nested lifecycle: %v", err)
	}

	fp, err := FingerprintLifecyclePreconditions(root, "custom_entity")
	if err != nil {
		t.Fatalf("expected successful nested fingerprint, got err: %v", err)
	}
	if fp == "" {
		t.Fatalf("expected non-empty fingerprint for nested lifecycle")
	}

	// 2. Lifecycle path is a directory (ReadFile fails with non-NotExist error)
	badDirKind := "dir_kind"
	dirKindPath := filepath.Join(root, paths.ProcessInternalLifecyclesDir, badDirKind+"_lifecycle.yaml")
	if err := fileutil.MkdirAll(dirKindPath, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create directory in place of lifecycle file: %v", err)
	}
	_, err = FingerprintLifecyclePreconditions(root, badDirKind)
	if err == nil {
		t.Fatalf("expected error when lifecycle target is a directory")
	}
}

func TestEmitForKind_EdgeCases(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	lcDir := filepath.Join(root, paths.ProcessInternalLifecyclesDir)
	if err := fileutil.MkdirAll(lcDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create lc dir: %v", err)
	}

	// 1. Kind has no lifecycle: returns nil without creating outbox
	if err := EmitForKind(root, "non_existent_kind", "reason"); err != nil {
		t.Fatalf("expected nil for missing lifecycle, got: %v", err)
	}

	// 2. Fingerprint error propagation (directory instead of lifecycle file)
	badKind := "bad_entity"
	if err := fileutil.MkdirAll(filepath.Join(lcDir, badKind+"_lifecycle.yaml"), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create bad entity dir: %v", err)
	}
	if err := EmitForKind(root, badKind, "reason"); err == nil {
		t.Fatalf("expected error for bad entity lifecycle directory")
	}

	// 3. Outbox path is a directory (ListPending fails)
	obDirRoot := t.TempDir()
	obDirLc := filepath.Join(obDirRoot, paths.ProcessInternalLifecyclesDir)
	_ = fileutil.MkdirAll(obDirLc, paths.DirPerm755)
	_ = fileutil.WriteFile(filepath.Join(obDirLc, "item_lifecycle.yaml"), []byte("kind: item"), paths.FilePerm644)
	obPath := outboxPath(obDirRoot)
	if err := fileutil.MkdirAll(obPath, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create directory at outbox path: %v", err)
	}
	if err := EmitForKind(obDirRoot, "item", "test"); err == nil {
		t.Fatalf("expected error when outbox path is a directory")
	}

	// 4. Consecutive emits for the same kind with DIFFERENT fingerprints (exercise deduplication loop break)
	diffRoot := t.TempDir()
	diffLcDir := filepath.Join(diffRoot, paths.ProcessInternalLifecyclesDir)
	_ = fileutil.MkdirAll(diffLcDir, paths.DirPerm755)
	lcFile := filepath.Join(diffLcDir, "evolving_kind_lifecycle.yaml")

	_ = fileutil.WriteFile(lcFile, []byte("version: 1"), paths.FilePerm644)
	if err := EmitForKind(diffRoot, "evolving_kind", ""); err != nil {
		t.Fatalf("first emit failed: %v", err)
	}

	// Change lifecycle content so fingerprint changes
	_ = fileutil.WriteFile(lcFile, []byte("version: 2"), paths.FilePerm644)
	if err := EmitForKind(diffRoot, "evolving_kind", "evolution_trigger"); err != nil {
		t.Fatalf("second emit with changed fingerprint failed: %v", err)
	}

	pending, err := ListPending(diffRoot)
	if err != nil {
		t.Fatalf("failed to list pending: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending events for evolving kind, got %d", len(pending))
	}
	if pending[0].Reason != "spec_origin_trigger" {
		t.Fatalf("expected default reason 'spec_origin_trigger', got %q", pending[0].Reason)
	}
	if pending[1].Reason != "evolution_trigger" {
		t.Fatalf("expected reason 'evolution_trigger', got %q", pending[1].Reason)
	}
}

func TestListPending_And_MarkConsumed_ErrorBranches(t *testing.T) {
	t.Parallel()

	// 1. ListPending returns error when outbox path is a directory
	dirRoot := t.TempDir()
	path := outboxPath(dirRoot)
	if err := fileutil.MkdirAll(path, paths.DirPerm755); err != nil {
		t.Fatalf("failed to make outbox path directory: %v", err)
	}
	_, err := ListPending(dirRoot)
	if err == nil {
		t.Fatalf("expected error from ListPending when outbox is a directory")
	}

	// 2. MarkConsumed on non-existent file returns nil
	nonExistentRoot := t.TempDir()
	if err := MarkConsumed(nonExistentRoot, []string{"CCE-some-id"}); err != nil {
		t.Fatalf("expected nil when file not exist, got: %v", err)
	}

	// 3. MarkConsumed returns error when outbox path is a directory
	if err := MarkConsumed(dirRoot, []string{"CCE-some-id"}); err == nil {
		t.Fatalf("expected error from MarkConsumed when outbox is a directory")
	}
}

func TestApplyPending_ComprehensiveScenarios(t *testing.T) {
	t.Parallel()

	// 1. Outbox path is a directory -> ListPending fails -> ApplyPending returns error
	dirRoot := t.TempDir()
	if err := fileutil.MkdirAll(outboxPath(dirRoot), paths.DirPerm755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	_, err := ApplyPending(context.Background(), dirRoot, &mockStorageProvider{})
	if err == nil {
		t.Fatalf("expected error when ListPending fails in ApplyPending")
	}

	// 2. Multiple events with same kind in single pending batch -> seenKinds deduplication branch
	dupRoot := t.TempDir()
	dupLcDir := filepath.Join(dupRoot, paths.ProcessInternalLifecyclesDir)
	_ = fileutil.MkdirAll(dupLcDir, paths.DirPerm755)
	_ = fileutil.WriteFile(filepath.Join(dupLcDir, "other_lifecycle.yaml"), []byte("kind: other"), paths.FilePerm644)

	// Append two events of kind "other" directly to outbox
	ev1 := Event{
		SchemaVersion: "1",
		ID:            "CCE-0001-other",
		Kind:          "other",
		Fingerprint:   "fp1",
		CreatedAt:     "2026-01-01T00:00:00Z",
	}
	ev2 := Event{
		SchemaVersion: "1",
		ID:            "CCE-0002-other",
		Kind:          "other",
		Fingerprint:   "fp2",
		CreatedAt:     "2026-01-01T00:00:01Z",
	}
	_ = appendEvent(dupRoot, ev1)
	_ = appendEvent(dupRoot, ev2)

	res, err := ApplyPending(context.Background(), dupRoot, &mockStorageProvider{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.EventsConsumed != 2 {
		t.Fatalf("expected 2 consumed, got %d", res.EventsConsumed)
	}

	// 3. Provider.List returns error
	listErrRoot := t.TempDir()
	_ = appendEvent(listErrRoot, Event{
		SchemaVersion: "1",
		ID:            "CCE-0003-priority_plan",
		Kind:          objects.KindPriorityPlan,
		Fingerprint:   "fp3",
		CreatedAt:     "2026-01-01T00:00:00Z",
	})
	mockListErr := &mockStorageProvider{
		listFn: func(ctx context.Context, sec *pkgctx.SecurityContext, sCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
			return nil, errors.New("simulated storage list error")
		},
	}
	res, err = ApplyPending(context.Background(), listErrRoot, mockListErr)
	if err != nil {
		t.Fatalf("ApplyPending should swallow listPriorityPlansNeedingDemote error and log warning, got: %v", err)
	}
	if res.EventsConsumed != 1 {
		t.Fatalf("expected event to be consumed despite list error")
	}

	// 4. Provider.List returns nil QueryResult
	nilResRoot := t.TempDir()
	_ = appendEvent(nilResRoot, Event{
		SchemaVersion: "1",
		ID:            "CCE-0004-priority_plan",
		Kind:          objects.KindPriorityPlan,
		Fingerprint:   "fp4",
		CreatedAt:     "2026-01-01T00:00:00Z",
	})
	mockNilRes := &mockStorageProvider{
		listFn: func(ctx context.Context, sec *pkgctx.SecurityContext, sCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
			return nil, nil
		},
	}
	res, err = ApplyPending(context.Background(), nilResRoot, mockNilRes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.EventsConsumed != 1 || len(res.Demoted) != 0 {
		t.Fatalf("expected 1 consumed, 0 demoted, got: %+v", res)
	}

	// 5. listPriorityPlansNeedingDemote objects iteration filtering:
	// - nil object
	// - object with empty ID
	// - object with non-shovel/locked status
	// - object with dispatch identity
	// Followed by demote loop branches:
	// - Read error or nil object
	// - Read returns non-shovel/locked object
	// - Read returns object with dispatch identity
	// - Update returns error
	// - Update succeeds
	demoteScenarioRoot := t.TempDir()
	_ = appendEvent(demoteScenarioRoot, Event{
		SchemaVersion: "1",
		ID:            "CCE-0005-priority_plan",
		Kind:          objects.KindPriorityPlan,
		Fingerprint:   "fp5",
		CreatedAt:     "2026-01-01T00:00:00Z",
	})

	listObjs := []map[string]any{
		nil, // nil object
		{objects.FieldKeyID: "", objects.FieldKeyStatus: "active"},                                                                  // empty ID
		{objects.FieldKeyID: "plan-grooming", objects.FieldKeyStatus: "grooming"},                                                   // non shovel/locked
		{objects.FieldKeyID: "plan-with-team", objects.FieldKeyStatus: "active", objects.FieldKeyTeamConfigurationRef: "TC-TEAM-1"}, // has dispatch identity
		{objects.FieldKeyID: "plan-read-fail", objects.FieldKeyStatus: "active"},
		{objects.FieldKeyID: "plan-read-nil", objects.FieldKeyStatus: "active"},
		{objects.FieldKeyID: "plan-status-changed", objects.FieldKeyStatus: "active"},
		{objects.FieldKeyID: "plan-concurrent-team", objects.FieldKeyStatus: "active"},
		{objects.FieldKeyID: "plan-update-fail", objects.FieldKeyStatus: "active"},
		{objects.FieldKeyID: "plan-demoted-ok", objects.FieldKeyStatus: "active"},
	}

	readMap := map[string]map[string]any{
		"plan-status-changed":  {objects.FieldKeyID: "plan-status-changed", objects.FieldKeyStatus: "grooming"},
		"plan-concurrent-team": {objects.FieldKeyID: "plan-concurrent-team", objects.FieldKeyStatus: "active", objects.FieldKeyPersonaRefs: []string{"PER-1"}},
		"plan-update-fail":     {objects.FieldKeyID: "plan-update-fail", objects.FieldKeyStatus: "active"},
		"plan-demoted-ok":      {objects.FieldKeyID: "plan-demoted-ok", objects.FieldKeyStatus: "active"},
	}

	updated := map[string]map[string]any{}
	mockDemoteCases := &mockStorageProvider{
		listFn: func(ctx context.Context, sec *pkgctx.SecurityContext, sCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
			return &storage.QueryResult{Objects: listObjs}, nil
		},
		readFn: func(ctx context.Context, sec *pkgctx.SecurityContext, id string) (map[string]any, error) {
			if id == "plan-read-fail" {
				return nil, errors.New("simulated read failure")
			}
			if id == "plan-read-nil" {
				return nil, nil
			}
			return readMap[id], nil
		},
		updateFn: func(ctx context.Context, sec *pkgctx.SecurityContext, id string, updates map[string]any) error {
			if id == "plan-update-fail" {
				return errors.New("simulated update failure")
			}
			updated[id] = updates
			return nil
		},
	}

	res, err = ApplyPending(context.Background(), demoteScenarioRoot, mockDemoteCases)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.EventsConsumed != 1 {
		t.Fatalf("expected 1 event consumed, got %d", res.EventsConsumed)
	}
	if len(res.Demoted) != 1 || res.Demoted[0] != "plan-demoted-ok" {
		t.Fatalf("expected only plan-demoted-ok to be demoted, got: %v", res.Demoted)
	}
	if updated["plan-demoted-ok"][objects.FieldKeyStatus] != objects.ObjectStatusGrooming {
		t.Fatalf("expected status to be updated to grooming, got: %v", updated["plan-demoted-ok"])
	}

	// 6. MarkConsumed failure in ApplyPending
	markErrRoot := t.TempDir()
	_ = appendEvent(markErrRoot, Event{
		SchemaVersion: "1",
		ID:            "CCE-0006-backlog_item",
		Kind:          objects.KindBacklogItem,
		Fingerprint:   "fp6",
		CreatedAt:     "2026-01-01T00:00:00Z",
	})
	// Replace outbox file with a read-only directory or chmod 000 before MarkConsumed
	// Note: on unix, making the file a directory causes fileutil.ReadFile in MarkConsumed to fail
	obFilePath := outboxPath(markErrRoot)
	// We want ListPending to succeed, then MarkConsumed to fail.
	// We can remove the file and create a directory in its place.
	var customProvider *mockStorageProvider
	customProvider = &mockStorageProvider{
		listFn: func(ctx context.Context, sec *pkgctx.SecurityContext, sCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
			return &storage.QueryResult{}, nil
		},
	}
	// To reliably trigger MarkConsumed error:
	// We can write an un-writable directory or manipulate permissions:
	_ = os.Remove(obFilePath)
	_ = fileutil.MkdirAll(obFilePath, paths.DirPerm755)

	_, err = ApplyPending(context.Background(), markErrRoot, customProvider)
	if err == nil {
		t.Fatalf("expected ApplyPending to return error when MarkConsumed fails")
	}
}

func TestConcurrentOutboxOperations_RaceFree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	lcDir := filepath.Join(root, paths.ProcessInternalLifecyclesDir)
	_ = fileutil.MkdirAll(lcDir, paths.DirPerm755)

	kinds := []string{"kind_alpha", "kind_beta", "kind_gamma", "kind_delta"}
	for _, k := range kinds {
		body := "object_type: " + k + "\nstatuses:\n  - value: active\n"
		_ = fileutil.WriteFile(filepath.Join(lcDir, k+"_lifecycle.yaml"), []byte(body), paths.FilePerm644)
	}

	var wg sync.WaitGroup
	const workers = 8
	const iters = 10

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				k := kinds[(workerID+i)%len(kinds)]
				_ = EmitForKind(root, k, "concurrent_test")
				_, _ = ListPending(root)
			}
		}(w)
	}

	wg.Wait()

	pending, err := ListPending(root)
	if err != nil {
		t.Fatalf("failed to list pending after concurrent runs: %v", err)
	}
	if len(pending) == 0 {
		t.Fatalf("expected pending events after concurrent emits")
	}
}
