package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
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
	if err := os.MkdirAll(filepath.Join(tmpDir, "docs", "process"), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	secCtx := pkgctx.NewSecurityContext("account:test", []string{"admin"}, []string{})

	prev := map[string]any{"id": "ITEM-1", "title": "old title"}
	upd := map[string]any{"id": "ITEM-1", "title": "new title"}

	// Call createChangeJournalEntry (production path)
	err := createChangeJournalEntry(ctx, tmpDir, "ITEM-1", "backlog_item", "", OpUpdate, prev, upd, secCtx, nil)
	if err != nil {
		t.Fatalf("createChangeJournalEntry unexpected error: %v", err)
	}

	cAfter, fAfter := GetChangeJournalHelperStats()
	if cAfter <= cInit && fAfter <= fInit {
		t.Fatalf("expected created/failed counter to increment, got cInit=%d cAfter=%d fInit=%d fAfter=%d", cInit, cAfter, fInit, fAfter)
	}
}
