package app_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"

	"github.com/lanceman/zqk/cmd/zqk-community/app"
)

// updateCountingStorage wraps a storage and counts Update calls (used to assert
// TryReuseSession does not touch the session; only PostRunE should touch).
type updateCountingStorage struct {
	storage.ObjectStorageProvider
	updateCount int64
}

func (c *updateCountingStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	atomic.AddInt64(&c.updateCount, 1)
	return c.ObjectStorageProvider.Update(ctx, secCtx, id, updates)
}

// TestSessionTouch_TryReuseSessionDoesNotTouch ensures TryReuseSession does not call
// TouchSession when reusing a session. Touching only in PostRunE avoids duplicate
// zqk_session update entries in the WAL (one per command instead of two).
func TestSessionTouch_TryReuseSessionDoesNotTouch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	testRoot, _ := setupIntegrationTestWithSpecsApp(t, "session-touch")
	if err := storage.EnsurePathAliasCacheReady(testRoot); err != nil {
		t.Fatalf("EnsurePathAliasCacheReady: %v", err)
	}

	sp, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, sp)
	t.Cleanup(func() {
		if err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, sp)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})
	wrapper := &updateCountingStorage{ObjectStorageProvider: sp}

	ctx := pkgctx.NewSystemContext()
	sessionID := app.StartZqkSession(ctx, testRoot, "first-command", pkgctx.SystemAccountID, wrapper)
	if sessionID == app.EmptyValue {
		t.Fatal("StartZqkSession returned empty")
	}

	// Flush CAS index so Read can find the session we just created (zqk_session is CAS)
	if q := caspkg.GetListingIndexWriteQueueForProjectRoot(testRoot); q != nil {
		_ = q.FlushAllWithDeadline(time.Now().Add(5 * time.Second))
	}

	app.WritePersistedSessionID(ctx, testRoot, sessionID, pkgctx.SystemAccountID, wrapper)

	countBefore := atomic.LoadInt64(&wrapper.updateCount)
	gotID, reused := app.TryReuseSession(ctx, testRoot, "second-command", pkgctx.SystemAccountID, wrapper)
	if !reused || gotID != sessionID {
		t.Fatalf("TryReuseSession: got (%q, %v), want (%q, true)", gotID, reused, sessionID)
	}
	countAfter := atomic.LoadInt64(&wrapper.updateCount)
	if countAfter != countBefore {
		t.Errorf("TryReuseSession must not call Update (touch); Update was called %d time(s). Only PostRunE should touch so WAL has one entry per command.",
			countAfter-countBefore)
	}
}
