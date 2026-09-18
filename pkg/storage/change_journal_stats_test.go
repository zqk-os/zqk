package storage

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestChangeJournal_CallbackAtomicPointer(t *testing.T) {
	// Do NOT use t.Parallel() on global state toggles
	initCb := getChangeJournalEventCallback()
	defer SetChangeJournalEventCallback(initCb)

	called := false
	dummyCb := func(
		ctx context.Context,
		projectRoot string,
		storage ObjectStorageProvider,
		operationID string,
		operationType string,
		status string,
		changeType string,
		objectRef string,
		kind string,
		objectID string,
		duration time.Duration,
		err error,
	) {
		called = true
	}

	SetChangeJournalEventCallback(dummyCb)
	gotCb := getChangeJournalEventCallback()
	if gotCb == nil {
		t.Fatalf("expected non-nil callback after SetChangeJournalEventCallback")
	}

	gotCb(context.Background(), "", nil, "", "", "", "", "", "", "", 0, nil)
	if !called {
		t.Fatalf("expected callback to be called")
	}

	SetChangeJournalEventCallback(nil)
	if getChangeJournalEventCallback() != nil {
		t.Fatalf("expected nil callback after SetChangeJournalEventCallback(nil)")
	}
}

func TestChangeJournal_LifetimeCounters(t *testing.T) {
	// Do NOT use t.Parallel() on global state toggles
	cInit, fInit := GetChangeJournalHelperStats()

	ctx := context.Background()
	tmpDir := t.TempDir()
	mustEnsureProcessSpecsLayout(t, tmpDir)
	fileStorage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}
	defer func() { _ = fileStorage.Shutdown(context.Background()) }()

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{})

	prev := map[string]any{objects.FieldKeyID: "BLI-1", objects.FieldKeyTitle: "old title"}
	upd := map[string]any{objects.FieldKeyID: "BLI-1", objects.FieldKeyTitle: "new title"}

	// Call createChangeJournalEntry (production path)
	err = createChangeJournalEntry(ctx, tmpDir, "BLI-1", "backlog_item", "", OpUpdate, prev, upd, secCtx, fileStorage)
	if err != nil {
		t.Fatalf("createChangeJournalEntry unexpected error: %v", err)
	}

	drainCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := DrainChangeJournalForRoot(drainCtx, tmpDir); err != nil {
		t.Fatalf("DrainChangeJournalForRoot error: %v", err)
	}

	cAfter, fAfter := GetChangeJournalHelperStats()
	t.Logf("After drain: cInit=%d cAfter=%d fInit=%d fAfter=%d", cInit, cAfter, fInit, fAfter)
	if cAfter <= cInit && fAfter <= fInit {
		t.Fatalf("expected created/failed counter to increment, got cInit=%d cAfter=%d fInit=%d fAfter=%d", cInit, cAfter, fInit, fAfter)
	}
}

func TestChangeJournal_GracefulDrain(t *testing.T) {
	ctx := context.Background()
	handler := &ChangeJournalShutdownHandler{}

	if handler.GetName() != "change_journal" {
		t.Errorf("unexpected name %s", handler.GetName())
	}
	if !handler.IsCritical() {
		t.Errorf("change journal must be critical queue")
	}

	drainCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := handler.Drain(drainCtx); err != nil {
		t.Fatalf("drain failed: %v", err)
	}
	if !handler.IsDrained() {
		t.Fatalf("expected handler to be drained, got pending=%d", handler.GetPendingCount())
	}
}
