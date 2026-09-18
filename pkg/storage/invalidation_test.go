package storage

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestInvalidationShockwaveBus_SubscribeAndBroadcast(t *testing.T) {
	bus := NewInvalidationShockwaveBus()

	var receivedEvents []MutationEvent
	var mu sync.Mutex

	sub := InvalidationSubscriberFunc(func(ctx context.Context, event MutationEvent) error {
		mu.Lock()
		defer mu.Unlock()
		receivedEvents = append(receivedEvents, event)
		return nil
	})

	bus.Subscribe(sub)
	if bus.SubscribersCount() != 1 {
		t.Fatalf("expected 1 subscriber, got %d", bus.SubscribersCount())
	}

	ctx := context.Background()

	// Broadcast Put event
	putEvent := MutationEvent{
		Op:         MutationOpPut,
		Kind:       "backlog_item",
		ID:         "BLI-TEST-001",
		OldHash:    "oldhash123",
		NewHash:    "newhash456",
		Path:       "/path/to/bli.yaml",
		Timestamp:  time.Now(),
		ObjectData: map[string]any{"id": "BLI-TEST-001", "kind": "backlog_item"},
	}
	bus.Broadcast(ctx, putEvent)

	// Broadcast Delete event
	deleteEvent := MutationEvent{
		Op:         MutationOpDelete,
		Kind:       "backlog_item",
		ID:         "BLI-TEST-001",
		OldHash:    "newhash456",
		Path:       "/path/to/bli.yaml",
		Timestamp:  time.Now(),
	}
	bus.Broadcast(ctx, deleteEvent)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedEvents) != 2 {
		t.Fatalf("expected 2 received events, got %d", len(receivedEvents))
	}

	if receivedEvents[0].Op != MutationOpPut || receivedEvents[0].ID != "BLI-TEST-001" || receivedEvents[0].NewHash != "newhash456" {
		t.Errorf("unexpected put event: %+v", receivedEvents[0])
	}
	if receivedEvents[1].Op != MutationOpDelete || receivedEvents[1].ID != "BLI-TEST-001" || receivedEvents[1].OldHash != "newhash456" {
		t.Errorf("unexpected delete event: %+v", receivedEvents[1])
	}
}

func TestInvalidationShockwaveBus_Unsubscribe(t *testing.T) {
	bus := NewInvalidationShockwaveBus()

	var count int
	var mu sync.Mutex

	sub := InvalidationSubscriberFunc(func(ctx context.Context, event MutationEvent) error {
		mu.Lock()
		defer mu.Unlock()
		count++
		return nil
	})

	bus.Subscribe(sub)
	bus.Broadcast(context.Background(), MutationEvent{Op: MutationOpPut, ID: "1"})

	bus.Unsubscribe(sub)
	if bus.SubscribersCount() != 0 {
		t.Fatalf("expected 0 subscribers, got %d", bus.SubscribersCount())
	}

	bus.Broadcast(context.Background(), MutationEvent{Op: MutationOpPut, ID: "2"})

	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Fatalf("expected count 1 after unsubscribe, got %d", count)
	}
}

func TestInvalidationShockwaveBus_ErrorResilience(t *testing.T) {
	bus := NewInvalidationShockwaveBus()

	var secondSubCalled bool
	var mu sync.Mutex

	errSub := InvalidationSubscriberFunc(func(ctx context.Context, event MutationEvent) error {
		return errors.New("subscriber intentional failure")
	})

	secondSub := InvalidationSubscriberFunc(func(ctx context.Context, event MutationEvent) error {
		mu.Lock()
		defer mu.Unlock()
		secondSubCalled = true
		return nil
	})

	bus.Subscribe(errSub)
	bus.Subscribe(secondSub)

	bus.Broadcast(context.Background(), MutationEvent{
		Op:   MutationOpPut,
		Kind: "policy",
		ID:   "POL-001",
	})

	mu.Lock()
	defer mu.Unlock()
	if !secondSubCalled {
		t.Fatal("expected second subscriber to execute even when first subscriber fails")
	}
}

func TestInvalidationShockwaveBus_ParseCacheIntegration(t *testing.T) {
	parseCache := NewParseCache(100)
	sub := newParseCacheSubscriber(parseCache)

	bus := NewInvalidationShockwaveBus()
	bus.Subscribe(sub)

	oldHash := "abc123def456"
	newHash := "789xyz"
	parsedObj := &objects.ParsedObject{
		Raw: map[string]any{"id": "BLI-TEST-AST", "kind": "backlog_item"},
	}

	parseCache.Put(oldHash, parsedObj)

	// Verify it's present in parseCache
	if _, hit := parseCache.Get(oldHash); !hit {
		t.Fatal("expected parseCache hit for oldHash")
	}

	// Dispatch mutation event with OldHash
	bus.Broadcast(context.Background(), MutationEvent{
		Op:      MutationOpPut,
		Kind:    "backlog_item",
		ID:      "BLI-TEST-AST",
		OldHash: oldHash,
		NewHash: newHash,
	})

	// Verify oldHash was evicted
	if _, hit := parseCache.Get(oldHash); hit {
		t.Fatal("expected parseCache miss for oldHash after shockwave broadcast")
	}
}

func TestInvalidation_StorageBroadcast(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "zqk-invalidation-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	mustEnsureProcessSpecsLayout(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		_ = RunProjectTestTeardown(opts)
	})

	var mu sync.Mutex
	var events []MutationEvent
	testSub := InvalidationSubscriberFunc(func(ctx context.Context, event MutationEvent) error {
		mu.Lock()
		defer mu.Unlock()
		if event.ID == "BLI-INV-TEST" {
			events = append(events, event)
		}
		return nil
	})

	globalBus := GetGlobalInvalidationBus()
	globalBus.Subscribe(testSub)
	defer globalBus.Unsubscribe(testSub)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := WithTestHardDelete(context.Background())

	// 1. Test Create broadcasts Put event
	objID := "BLI-INV-TEST"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Invalidation Test Item",
		objects.FieldKeyDescription:   "Testing shockwave bus broadcast",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	if err := storage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}

	mu.Lock()
	if len(events) < 1 {
		mu.Unlock()
		t.Fatalf("expected at least 1 event after Create, got %d", len(events))
	}
	lastEvent := events[len(events)-1]
	mu.Unlock()

	if lastEvent.Op != MutationOpPut || lastEvent.ID != objID || lastEvent.Kind != "backlog_item" {
		t.Fatalf("unexpected event after Create: %+v", lastEvent)
	}

	// 2. Test Update broadcasts Put event
	updates := map[string]any{
		objects.FieldKeyTitle: "Updated Invalidation Title",
	}
	if err := storage.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}

	mu.Lock()
	if len(events) < 2 {
		mu.Unlock()
		t.Fatalf("expected at least 2 events after Update, got %d", len(events))
	}
	lastEvent = events[len(events)-1]
	mu.Unlock()

	if lastEvent.Op != MutationOpPut || lastEvent.ID != objID {
		t.Fatalf("unexpected event after Update: %+v", lastEvent)
	}

	// 3. Test Delete broadcasts Delete event
	if err := storage.Delete(ctx, secCtx, objID, false); err != nil {
		t.Fatalf("failed to delete object: %v", err)
	}

	mu.Lock()
	if len(events) < 3 {
		mu.Unlock()
		t.Fatalf("expected at least 3 events after Delete, got %d", len(events))
	}
	lastEvent = events[len(events)-1]
	mu.Unlock()

	if lastEvent.Op != MutationOpDelete || lastEvent.ID != objID {
		t.Fatalf("unexpected event after Delete: %+v", lastEvent)
	}
}

func TestGetListReadCongruence(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "zqk-get-list-parity-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	mustEnsureProcessSpecsLayout(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		_ = RunProjectTestTeardown(opts)
	})

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := WithTestHardDelete(context.Background())
	storageCtx := &pkgctx.StorageContext{}

	objID := "BLI-PARITY-TEST"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Parity Test Item",
		objects.FieldKeyDescription:   "Testing read congruence between get and list",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	// 1. Create CAS-visible object and verify get/list congruence on initial state
	CreateCASVisible(t, storage, ctx, secCtx, obj, objects.ObjectStatusPlanned)

	getObj, err := storage.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if getObj[objects.FieldKeyStatus] != objects.ObjectStatusPlanned {
		t.Fatalf("expected get status %s, got %v", objects.ObjectStatusPlanned, getObj[objects.FieldKeyStatus])
	}

	listRes, err := storage.List(ctx, secCtx, storageCtx, ListFilter{Kind: "backlog_item"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	var foundInList map[string]any
	for _, o := range listRes.Objects {
		if o[objects.FieldKeyID] == objID {
			foundInList = o
			break
		}
	}
	if foundInList == nil {
		t.Fatalf("object %s not found in list results", objID)
	}
	if foundInList[objects.FieldKeyStatus] != getObj[objects.FieldKeyStatus] {
		t.Fatalf("get/list status divergence: get=%v list=%v", getObj[objects.FieldKeyStatus], foundInList[objects.FieldKeyStatus])
	}

	// 2. Update status to in_progress and verify immediate get/list congruence with zero lag
	updates := map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
	}
	if err := storage.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}

	getUpdated, err := storage.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("get after update failed: %v", err)
	}
	if getUpdated[objects.FieldKeyStatus] != objects.ObjectStatusInProgress {
		t.Fatalf("expected get status %s, got %v", objects.ObjectStatusInProgress, getUpdated[objects.FieldKeyStatus])
	}

	listUpdated, err := storage.List(ctx, secCtx, storageCtx, ListFilter{Kind: "backlog_item"})
	if err != nil {
		t.Fatalf("list after update failed: %v", err)
	}
	foundInList = nil
	for _, o := range listUpdated.Objects {
		if o[objects.FieldKeyID] == objID {
			foundInList = o
			break
		}
	}
	if foundInList == nil {
		t.Fatalf("object %s not found in list results after update", objID)
	}
	if foundInList[objects.FieldKeyStatus] != getUpdated[objects.FieldKeyStatus] {
		t.Fatalf("get/list status divergence after update: get=%v list=%v", getUpdated[objects.FieldKeyStatus], foundInList[objects.FieldKeyStatus])
	}
}

// TestObjectIDCache_SSOTLocator verifies CRIT-CACHE-SSOT-ID-LOCATOR-001:
// The Object ID Cache acts as the authoritative locator SSOT. Query resolvers
// and downstream caches must resolve object presence and content hash locators via the ID cache.
func TestObjectIDCache_SSOTLocator(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "zqk-ssot-locator-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	mustEnsureProcessSpecsLayout(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		_ = RunProjectTestTeardown(opts)
	})

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := WithTestHardDelete(context.Background())

	objID := "BLI-SSOT-LOCATOR-001"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "SSOT Locator Test Item",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	CreateCASVisible(t, storage, ctx, secCtx, obj, objects.ObjectStatusPlanned)

	// Verify that the authoritative locator resolves the object presence and path
	resolvedPath, err := storage.GetFilePathForObject(objID, "backlog_item")
	if err != nil {
		t.Fatalf("GetFilePathForObject failed: %v", err)
	}
	if resolvedPath == "" {
		t.Fatalf("expected non-empty resolved path for %s", objID)
	}
	if _, statErr := fileutil.Stat(resolvedPath); statErr != nil {
		t.Fatalf("resolved path %s does not exist on disk: %v", resolvedPath, statErr)
	}
}

