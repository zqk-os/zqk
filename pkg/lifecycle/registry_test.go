package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
)

func TestCloseLifecycleWAL_EmptyOrMissing(t *testing.T) {
	if err := CloseLifecycleWAL(""); err != nil {
		t.Fatalf("expected nil error on empty projectRoot, got: %v", err)
	}

	if err := CloseLifecycleWAL("/nonexistent/path"); err != nil {
		t.Fatalf("expected nil error on missing projectRoot, got: %v", err)
	}
}

func TestCloseLifecycleWAL_ClosesAndRemoves(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := GetOrCreateLifecycleWAL(tmpDir)
	if err != nil {
		t.Fatalf("failed to create WAL: %v", err)
	}
	if w == nil {
		t.Fatalf("expected non-nil WAL")
	}

	walCacheMu.RLock()
	cached := walCache[tmpDir]
	walCacheMu.RUnlock()
	if cached != w {
		t.Fatalf("expected WAL to be cached")
	}

	if err := CloseLifecycleWAL(tmpDir); err != nil {
		t.Fatalf("CloseLifecycleWAL failed: %v", err)
	}

	walCacheMu.RLock()
	cachedAfter := walCache[tmpDir]
	walCacheMu.RUnlock()
	if cachedAfter != nil {
		t.Fatalf("expected WAL to be removed from cache")
	}
}

func TestResetLifecycleWALCache(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	_, err := GetOrCreateLifecycleWAL(tmpDir1)
	if err != nil {
		t.Fatalf("failed to create WAL 1: %v", err)
	}
	_, err = GetOrCreateLifecycleWAL(tmpDir2)
	if err != nil {
		t.Fatalf("failed to create WAL 2: %v", err)
	}

	if err := ResetLifecycleWALCache(); err != nil {
		t.Fatalf("ResetLifecycleWALCache failed: %v", err)
	}

	walCacheMu.RLock()
	count := len(walCache)
	walCacheMu.RUnlock()
	if count != 0 {
		t.Fatalf("expected empty cache after reset, got len %d", count)
	}
}

func TestPollLifecycleWAL_ContextAlreadyCanceled(t *testing.T) {
	tmpDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	called := false
	PollLifecycleWAL(ctx, tmpDir, 10*time.Millisecond, func(ev *LifecycleEvent) {
		called = true
	}, nil)

	if called {
		t.Fatalf("expected onEvent not to be called with canceled context")
	}

	walPath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.WalDir)
	if _, err := os.Stat(walPath); !os.IsNotExist(err) {
		t.Fatalf("expected WAL directory not to be created when context is canceled before start")
	}
}
